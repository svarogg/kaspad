// Copyright (c) 2015-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockdag

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/blockstatus"
	"github.com/kaspanet/kaspad/consensus/utxo"
	"io"

	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/dbaccess"
	"github.com/pkg/errors"

	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/binaryserializer"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/util/subnetworkid"
	"github.com/kaspanet/kaspad/wire"
)

var (
	// byteOrder is the preferred byte order used for serializing numeric
	// fields for storage in the database.
	byteOrder = binary.LittleEndian
)

// ErrNotInDAG signifies that a block hash that is not in the
// DAG was requested.
type ErrNotInDAG string

// Error implements the error interface.
func (e ErrNotInDAG) Error() string {
	return string(e)
}

// IsNotInDAGErr returns whether or not the passed error is an
// ErrNotInDAG error.
func IsNotInDAGErr(err error) bool {
	var notInDAGErr ErrNotInDAG
	return errors.As(err, &notInDAGErr)
}

type dagState struct {
	TipHashes         []*daghash.Hash
	LastFinalityPoint *daghash.Hash
	LocalSubnetworkID *subnetworkid.SubnetworkID
}

// serializeDAGState returns the serialization of the DAG state.
// This is data to be stored in the DAG state bucket.
func serializeDAGState(state *dagState) ([]byte, error) {
	return json.Marshal(state)
}

// deserializeDAGState deserializes the passed serialized DAG state.
// This is data stored in the DAG state bucket and is updated after
// every block is connected to the DAG.
func deserializeDAGState(serializedData []byte) (*dagState, error) {
	var state *dagState
	err := json.Unmarshal(serializedData, &state)
	if err != nil {
		return nil, err
	}

	return state, nil
}

// saveDAGState uses an existing database context to store the latest
// tip hashes of the DAG.
func saveDAGState(dbContext dbaccess.Context, state *dagState) error {
	serializedDAGState, err := serializeDAGState(state)
	if err != nil {
		return err
	}

	return dbaccess.StoreDAGState(dbContext, serializedDAGState)
}

// createDAGState initializes the DAG state to the
// genesis block and the node's local subnetwork id.
func (dag *BlockDAG) createDAGState(localSubnetworkID *subnetworkid.SubnetworkID) error {
	return saveDAGState(dag.databaseContext, &dagState{
		TipHashes:         []*daghash.Hash{dag.Params.GenesisHash},
		LastFinalityPoint: dag.Params.GenesisHash,
		LocalSubnetworkID: localSubnetworkID,
	})
}

// initDAGState attempts to load and initialize the DAG state from the
// database. When the db does not yet contain any DAG state, both it and the
// DAG state are initialized to the genesis block.
func (dag *BlockDAG) initDAGState() error {
	// Fetch the stored DAG state from the database. If it doesn't exist,
	// it means that kaspad is running for the first time.
	serializedDAGState, err := dbaccess.FetchDAGState(dag.databaseContext)
	if dbaccess.IsNotFoundError(err) {
		// Initialize the database and the DAG state to the genesis block.
		return dag.createDAGState(dag.subnetworkID)
	}
	if err != nil {
		return err
	}

	dagState, err := deserializeDAGState(serializedDAGState)
	if err != nil {
		return err
	}

	err = dag.validateLocalSubnetworkID(dagState)
	if err != nil {
		return err
	}

	log.Debugf("Loading block index...")
	unprocessedBlockNodes, err := dag.initBlockIndex()
	if err != nil {
		return err
	}

	log.Debugf("Loading reachability data...")
	err = dag.reachabilityTree.init(dag.databaseContext)
	if err != nil {
		return err
	}

	log.Debugf("Loading multiset data...")
	err = dag.multisetStore.Init(dag.databaseContext)
	if err != nil {
		return err
	}

	log.Debugf("Loading UTXO set...")
	dag.virtual.utxoSet, err = utxo.InitUTXOSet(dag.databaseContext)
	if err != nil {
		return errors.Wrap(err, "Error loading UTXOSet")
	}

	log.Debugf("Applying the stored tips to the virtual block...")
	err = dag.initVirtualBlockTips(dagState)
	if err != nil {
		return err
	}

	log.Debugf("Setting the last finality point...")
	var ok bool
	dag.lastFinalityPoint, ok = dag.index.LookupNode(dagState.LastFinalityPoint)
	if !ok {
		return errors.Errorf("finality point block %s "+
			"does not exist in the DAG", dagState.LastFinalityPoint)
	}
	dag.finalizeNodesBelowFinalityPoint(false)

	log.Debugf("Processing unprocessed blockNodes...")
	err = dag.processUnprocessedBlockNodes(unprocessedBlockNodes)
	if err != nil {
		return err
	}

	log.Infof("DAG state initialized.")

	return nil
}

func (dag *BlockDAG) validateLocalSubnetworkID(state *dagState) error {
	if !state.LocalSubnetworkID.IsEqual(dag.subnetworkID) {
		return errors.Errorf("Cannot start kaspad with subnetwork ID %s because"+
			" its database is already built with subnetwork ID %s. If you"+
			" want to switch to a new database, please reset the"+
			" database by starting kaspad with --reset-db flag", dag.subnetworkID, state.LocalSubnetworkID)
	}
	return nil
}

func (dag *BlockDAG) initBlockIndex() (unprocessedBlockNodes []*blocknode.BlockNode, err error) {
	blockIndexCursor, err := dbaccess.BlockIndexCursor(dag.databaseContext)
	if err != nil {
		return nil, err
	}
	defer blockIndexCursor.Close()
	for blockIndexCursor.Next() {
		serializedDBNode, err := blockIndexCursor.Value()
		if err != nil {
			return nil, err
		}
		node, err := dag.deserializeBlockNode(serializedDBNode)
		if err != nil {
			return nil, err
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
			dag.index.AddNode(node)
			continue
		}

		if dag.blockCount == 0 {
			if !node.Hash().IsEqual(dag.Params.GenesisHash) {
				return nil, errors.Errorf("Expected "+
					"first entry in block index to be genesis block, "+
					"found %s", node.Hash())
			}
		} else {
			if len(node.Parents()) == 0 {
				return nil, errors.Errorf("block %s "+
					"has no parents but it's not the genesis block", node.Hash())
			}
		}

		// Add the node to its parents children, connect it,
		// and add it to the block index.
		node.UpdateParentsChildren()
		dag.index.AddNode(node)

		dag.blockCount++
	}
	return unprocessedBlockNodes, nil
}

func (dag *BlockDAG) initVirtualBlockTips(state *dagState) error {
	tips := blocknode.NewBlockNodeSet()
	for _, tipHash := range state.TipHashes {
		tip, ok := dag.index.LookupNode(tipHash)
		if !ok {
			return errors.Errorf("cannot find "+
				"DAG tip %s in block index", state.TipHashes)
		}
		tips.Add(tip)
	}
	dag.virtual.SetTips(tips)
	return nil
}

func (dag *BlockDAG) processUnprocessedBlockNodes(unprocessedBlockNodes []*blocknode.BlockNode) error {
	for _, node := range unprocessedBlockNodes {
		// Check to see if the block exists in the block DB. If it
		// doesn't, the database has certainly been corrupted.
		blockExists, err := dbaccess.HasBlock(dag.databaseContext, node.Hash())
		if err != nil {
			return errors.Wrapf(err, "HasBlock "+
				"for block %s failed: %s", node.Hash(), err)
		}
		if !blockExists {
			return errors.Errorf("block %s "+
				"exists in block index but not in block db", node.Hash())
		}

		// Attempt to accept the block.
		block, err := dag.fetchBlockByHash(node.Hash())
		if err != nil {
			return err
		}
		isOrphan, isDelayed, err := dag.ProcessBlock(block, BFWasStored)
		if err != nil {
			log.Warnf("Block %s, which was not previously processed, "+
				"failed to be accepted to the DAG: %s", node.Hash(), err)
			continue
		}

		// If the block is an orphan or is delayed then it couldn't have
		// possibly been written to the block index in the first place.
		if isOrphan {
			return errors.Errorf("Block %s, which was not "+
				"previously processed, turned out to be an orphan, which is "+
				"impossible.", node.Hash())
		}
		if isDelayed {
			return errors.Errorf("Block %s, which was not "+
				"previously processed, turned out to be delayed, which is "+
				"impossible.", node.Hash())
		}
	}
	return nil
}

// deserializeBlockNode parses a value in the block index bucket and returns a block node.
func (dag *BlockDAG) deserializeBlockNode(blockRow []byte) (*blocknode.BlockNode, error) {
	buffer := bytes.NewReader(blockRow)

	var header wire.BlockHeader
	err := header.Deserialize(buffer)
	if err != nil {
		return nil, err
	}

	node := &blocknode.BlockNode{
		hash:                 header.BlockHash(),
		version:              header.Version,
		bits:                 header.Bits,
		nonce:                header.Nonce,
		timestamp:            header.Timestamp.UnixMilliseconds(),
		hashMerkleRoot:       header.HashMerkleRoot,
		acceptedIDMerkleRoot: header.AcceptedIDMerkleRoot,
		utxoCommitment:       header.UTXOCommitment,
	}

	node.children = blocknode.NewBlockNodeSet()
	node.parents = blocknode.NewBlockNodeSet()

	for _, hash := range header.ParentHashes {
		parent, ok := dag.index.LookupNode(hash)
		if !ok {
			return nil, errors.Errorf("deserializeBlockNode: Could "+
				"not find parent %s for block %s", hash, header.BlockHash())
		}
		node.parents.Add(parent)
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
		node.selectedParent, ok = dag.index.LookupNode(selectedParentHash)
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

	node.blues = make([]*blocknode.BlockNode, bluesCount)
	for i := uint64(0); i < bluesCount; i++ {
		hash := &daghash.Hash{}
		if _, err := io.ReadFull(buffer, hash[:]); err != nil {
			return nil, err
		}

		var ok bool
		node.blues[i], ok = dag.index.LookupNode(hash)
		if !ok {
			return nil, errors.Errorf("block %s does not exist in the DAG", selectedParentHash)
		}
	}

	bluesAnticoneSizesLen, err := wire.ReadVarInt(buffer)
	if err != nil {
		return nil, err
	}

	node.bluesAnticoneSizes = make(map[*blocknode.BlockNode]dagconfig.KType)
	for i := uint64(0); i < bluesAnticoneSizesLen; i++ {
		hash := &daghash.Hash{}
		if _, err := io.ReadFull(buffer, hash[:]); err != nil {
			return nil, err
		}
		bluesAnticoneSize, err := binaryserializer.Uint8(buffer)
		if err != nil {
			return nil, err
		}
		blue, ok := dag.index.LookupNode(hash)
		if !ok {
			return nil, errors.Errorf("couldn't find block with hash %s", hash)
		}
		node.bluesAnticoneSizes[blue] = dagconfig.KType(bluesAnticoneSize)
	}

	return node, nil
}

// fetchBlockByHash retrieves the raw block for the provided hash,
// deserializes it, and returns a util.Block of it.
func (dag *BlockDAG) fetchBlockByHash(hash *daghash.Hash) (*util.Block, error) {
	blockBytes, err := dbaccess.FetchBlock(dag.databaseContext, hash)
	if err != nil {
		return nil, err
	}
	return util.NewBlockFromBytes(blockBytes)
}

func storeBlock(dbContext *dbaccess.TxContext, block *util.Block) error {
	blockBytes, err := block.Bytes()
	if err != nil {
		return err
	}
	return dbaccess.StoreBlock(dbContext, block.Hash(), blockBytes)
}

func blockHashFromBlockIndexKey(BlockIndexKey []byte) (*daghash.Hash, error) {
	return daghash.NewHash(BlockIndexKey[8 : daghash.HashSize+8])
}

// BlockByHash returns the block from the DAG with the given hash.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) BlockByHash(hash *daghash.Hash) (*util.Block, error) {
	// Lookup the block hash in block index and ensure it is in the DAG
	node, ok := dag.index.LookupNode(hash)
	if !ok {
		str := fmt.Sprintf("block %s is not in the DAG", hash)
		return nil, ErrNotInDAG(str)
	}

	block, err := dag.fetchBlockByHash(node.hash)
	if err != nil {
		return nil, err
	}
	return block, err
}

// BlockHashesFrom returns a slice of blocks starting from lowHash
// ordered by blueScore. If lowHash is nil then the genesis block is used.
//
// This method MUST be called with the DAG lock held
func (dag *BlockDAG) BlockHashesFrom(lowHash *daghash.Hash, limit int) ([]*daghash.Hash, error) {
	blockHashes := make([]*daghash.Hash, 0, limit)
	if lowHash == nil {
		lowHash = dag.genesis.hash

		// If we're starting from the beginning we should include the
		// genesis hash in the result
		blockHashes = append(blockHashes, dag.genesis.hash)
	}
	if !dag.IsInDAG(lowHash) {
		return nil, errors.Errorf("block %s not found", lowHash)
	}
	blueScore, err := dag.BlueScoreByBlockHash(lowHash)
	if err != nil {
		return nil, err
	}

	key := blockIndexKey(lowHash, blueScore)
	cursor, err := dbaccess.BlockIndexCursorFrom(dag.databaseContext, key)
	if dbaccess.IsNotFoundError(err) {
		return nil, errors.Wrapf(err, "block %s not in block index", lowHash)
	}
	if err != nil {
		return nil, err
	}
	defer cursor.Close()

	for cursor.Next() && len(blockHashes) < limit {
		key, err := cursor.Key()
		if err != nil {
			return nil, err
		}
		blockHash, err := blockHashFromBlockIndexKey(key.Suffix())
		if err != nil {
			return nil, err
		}
		blockHashes = append(blockHashes, blockHash)
	}

	return blockHashes, nil
}
