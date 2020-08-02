package blocknode

import (
	"bytes"
	"encoding/binary"
	"github.com/kaspanet/kaspad/consensus/blockstatus"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/dbaccess"
	"github.com/kaspanet/kaspad/util/binaryserializer"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
	"io"
)

var (
	// byteOrder is the preferred byte order used for serializing numeric
	// fields for storage in the database.
	byteOrder = binary.LittleEndian
)

// BlockIndexKey generates the binary key for an entry in the block index
// bucket. The key is composed of the block blue score encoded as a big-endian
// 64-bit unsigned int followed by the 32 byte block hash.
// The blue score component is important for iteration order.
func BlockIndexKey(blockHash *daghash.Hash, blueScore uint64) []byte {
	indexKey := make([]byte, daghash.HashSize+8)
	binary.BigEndian.PutUint64(indexKey[0:8], blueScore)
	copy(indexKey[8:daghash.HashSize+8], blockHash[:])
	return indexKey
}

func (bi *BlockNodeStore) Init(dbContext *dbaccess.DatabaseContext) (
	unprocessedBlockNodes []*BlockNode, blockCount uint64, err error) {

	blockIndexCursor, err := dbaccess.BlockIndexCursor(dbContext)
	if err != nil {
		return nil, 0, err
	}
	defer blockIndexCursor.Close()

	blockCount = 0
	for blockIndexCursor.Next() {
		serializedDBNode, err := blockIndexCursor.Value()
		if err != nil {
			return nil, 0, err
		}
		node, err := bi.deserializeBlockNode(serializedDBNode)
		if err != nil {
			return nil, 0, err
		}

		// Check to see if this node had been stored in the the block DB
		// but not yet accepted. If so, add it to a slice to be processed later.
		if node.Status() == blockstatus.StatusDataStored {
			unprocessedBlockNodes = append(unprocessedBlockNodes, node)
			continue
		}

		// If the node is known to be invalid add it as-is to the block
		// index and continue.
		if node.Status().KnownInvalid() {
			bi.addNode(node)
			continue
		}

		if blockCount == 0 {
			if !node.Hash().IsEqual(bi.dagParams.GenesisHash) {
				return nil, 0, errors.Errorf("Expected "+
					"first entry in block index to be genesis block, "+
					"found %s", node.Hash())
			}
		} else {
			if len(node.Parents()) == 0 {
				return nil, 0, errors.Errorf("block %s "+
					"has no parents but it's not the genesis block", node.Hash())
			}
		}

		// Add the node to its parents children, connect it,
		// and add it to the block index.
		node.UpdateParentsChildren()
		bi.addNode(node)

		blockCount++
	}
	return unprocessedBlockNodes, blockCount, nil
}

// deserializeBlockNode parses a value in the block index bucket and returns a block node.
func (bi *BlockNodeStore) deserializeBlockNode(blockRow []byte) (*BlockNode, error) {
	buffer := bytes.NewReader(blockRow)

	var header wire.BlockHeader
	err := header.Deserialize(buffer)
	if err != nil {
		return nil, err
	}

	node := &BlockNode{
		hash:                 header.BlockHash(),
		version:              header.Version,
		bits:                 header.Bits,
		nonce:                header.Nonce,
		timestamp:            header.Timestamp.UnixMilliseconds(),
		hashMerkleRoot:       header.HashMerkleRoot,
		acceptedIDMerkleRoot: header.AcceptedIDMerkleRoot,
		utxoCommitment:       header.UTXOCommitment,
	}

	node.children = NewBlockNodeSet()
	node.parents = NewBlockNodeSet()

	for _, hash := range header.ParentHashes {
		parent, ok := bi.LookupNode(hash)
		if !ok {
			return nil, errors.Errorf("deserializeBlockNode: Could "+
				"not find parent %s for block %s", hash, header.BlockHash())
		}
		node.Parents().Add(parent)
	}

	statusByte, err := buffer.ReadByte()
	if err != nil {
		return nil, err
	}
	node.status = blockstatus.BlockStatus(statusByte)

	selectedParentHash := &daghash.Hash{}
	if _, err := io.ReadFull(buffer, selectedParentHash[:]); err != nil {
		return nil, err
	}

	// Because genesis doesn't have selected parent, it's serialized as zero hash
	if !selectedParentHash.IsEqual(&daghash.ZeroHash) {
		var ok bool
		node.selectedParent, ok = bi.LookupNode(selectedParentHash)
		if !ok {
			return nil, errors.Errorf("block %s does not exist in the DAG", selectedParentHash)
		}
	}

	node.blueScore, err = binaryserializer.Uint64(buffer, byteOrder)
	if err != nil {
		return nil, err
	}

	bluesCount, err := wire.ReadVarInt(buffer)
	if err != nil {
		return nil, err
	}

	node.blues = make([]*BlockNode, bluesCount)
	for i := uint64(0); i < bluesCount; i++ {
		hash := &daghash.Hash{}
		if _, err := io.ReadFull(buffer, hash[:]); err != nil {
			return nil, err
		}

		var ok bool
		node.blues[i], ok = bi.LookupNode(hash)
		if !ok {
			return nil, errors.Errorf("block %s does not exist in the DAG", selectedParentHash)
		}
	}

	bluesAnticoneSizesLen, err := wire.ReadVarInt(buffer)
	if err != nil {
		return nil, err
	}

	node.bluesAnticoneSizes = make(map[*BlockNode]dagconfig.KType)
	for i := uint64(0); i < bluesAnticoneSizesLen; i++ {
		hash := &daghash.Hash{}
		if _, err := io.ReadFull(buffer, hash[:]); err != nil {
			return nil, err
		}
		bluesAnticoneSize, err := binaryserializer.Uint8(buffer)
		if err != nil {
			return nil, err
		}
		blue, ok := bi.LookupNode(hash)
		if !ok {
			return nil, errors.Errorf("couldn't find block with hash %s", hash)
		}
		node.bluesAnticoneSizes[blue] = dagconfig.KType(bluesAnticoneSize)
	}

	return node, nil
}

func SerializeBlockNode(node *BlockNode) ([]byte, error) {
	w := bytes.NewBuffer(make([]byte, 0, wire.MaxBlockHeaderPayload+1))
	header := node.Header()
	err := header.Serialize(w)
	if err != nil {
		return nil, err
	}

	err = w.WriteByte(byte(node.status))
	if err != nil {
		return nil, err
	}

	// Because genesis doesn't have selected parent, it's serialized as zero hash
	selectedParentHash := &daghash.ZeroHash
	if node.selectedParent != nil {
		selectedParentHash = node.selectedParent.hash
	}
	_, err = w.Write(selectedParentHash[:])
	if err != nil {
		return nil, err
	}

	err = binaryserializer.PutUint64(w, byteOrder, node.blueScore)
	if err != nil {
		return nil, err
	}

	err = wire.WriteVarInt(w, uint64(len(node.blues)))
	if err != nil {
		return nil, err
	}

	for _, blue := range node.blues {
		_, err = w.Write(blue.hash[:])
		if err != nil {
			return nil, err
		}
	}

	err = wire.WriteVarInt(w, uint64(len(node.bluesAnticoneSizes)))
	if err != nil {
		return nil, err
	}
	for blue, blueAnticoneSize := range node.bluesAnticoneSizes {
		_, err = w.Write(blue.hash[:])
		if err != nil {
			return nil, err
		}

		err = binaryserializer.PutUint8(w, uint8(blueAnticoneSize))
		if err != nil {
			return nil, err
		}
	}
	return w.Bytes(), nil
}
