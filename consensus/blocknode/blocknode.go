// Copyright (c) 2015-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blocknode

import (
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/util/mstime"
	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
	"math"
)

// BlockNode represents a block within the block DAG. The DAG is stored into
// the block database.
type BlockNode struct {
	// NOTE: Additions, deletions, or modifications to the order of the
	// definitions in this struct should not be changed without considering
	// how it affects alignment on 64-bit platforms. The current order is
	// specifically crafted to result in minimal padding. There will be
	// hundreds of thousands of these in memory, so a few extra bytes of
	// padding adds up.

	// parents is the parent blocks for this node.
	parents BlockNodeSet

	// selectedParent is the selected parent for this node.
	// The selected parent is the parent that if chosen will maximize the blue score of this block
	selectedParent *BlockNode

	// children are all the blocks that refer to this block as a parent
	children BlockNodeSet

	// blues are all blue blocks in this block's worldview that are in its selected parent anticone
	blues []*BlockNode

	// blueScore is the count of all the blue blocks in this block's past
	blueScore uint64

	// bluesAnticoneSizes is a map holding the set of blues affected by this block and their
	// modified blue anticone size.
	bluesAnticoneSizes map[*BlockNode]dagconfig.KType

	// hash is the double sha 256 of the block.
	hash *daghash.Hash

	// Some fields from block headers to aid in  reconstructing headers
	// from memory. These must be treated as immutable and are intentionally
	// ordered to avoid padding on 64-bit platforms.
	version              int32
	bits                 uint32
	nonce                uint64
	timestamp            int64
	hashMerkleRoot       *daghash.Hash
	acceptedIDMerkleRoot *daghash.Hash
	utxoCommitment       *daghash.Hash

	// status is a bitfield representing the validation state of the block. The
	// status field, unlike the other fields, may be written to and so should
	// only be accessed using the concurrent-safe NodeStatus method on
	// blockIndex once the node has been added to the global index.
	status BlockStatus

	// isFinalized determines whether the node is below the finality point.
	isFinalized bool
}

func NewBlockNode(blockHeader *wire.BlockHeader, parents BlockNodeSet, timestamp mstime.Time) *BlockNode {
	node := &BlockNode{
		parents:            parents,
		children:           make(BlockNodeSet),
		blueScore:          math.MaxUint64, // Initialized to the max value to avoid collisions with the genesis block
		timestamp:          timestamp.UnixMilliseconds(),
		bluesAnticoneSizes: make(map[*BlockNode]dagconfig.KType),
	}

	// blockHeader is nil only for the virtual block
	if blockHeader != nil {
		node.hash = blockHeader.BlockHash()
		node.version = blockHeader.Version
		node.bits = blockHeader.Bits
		node.nonce = blockHeader.Nonce
		node.timestamp = blockHeader.Timestamp.UnixMilliseconds()
		node.hashMerkleRoot = blockHeader.HashMerkleRoot
		node.acceptedIDMerkleRoot = blockHeader.AcceptedIDMerkleRoot
		node.utxoCommitment = blockHeader.UTXOCommitment
	} else {
		node.hash = &daghash.ZeroHash
	}

	// The genesis block is defined to have a blueScore of 0
	if len(node.Parents()) == 0 {
		node.blueScore = 0
	}

	return node
}

// UpdateParentsChildren updates the node's parents to point to new node
func (node *BlockNode) UpdateParentsChildren() {
	for parent := range node.parents {
		parent.children.Add(node)
	}
}

func (node *BlockNode) Less(other *BlockNode) bool {
	if node.blueScore == other.blueScore {
		return daghash.Less(node.hash, other.hash)
	}

	return node.blueScore < other.blueScore
}

// Header constructs a block header from the node and returns it.
//
// This function is safe for concurrent access.
func (node *BlockNode) Header() *wire.BlockHeader {
	// No lock is needed because all accessed fields are immutable.
	return &wire.BlockHeader{
		Version:              node.version,
		ParentHashes:         node.ParentHashes(),
		HashMerkleRoot:       node.hashMerkleRoot,
		AcceptedIDMerkleRoot: node.acceptedIDMerkleRoot,
		UTXOCommitment:       node.utxoCommitment,
		Timestamp:            node.Time(),
		Bits:                 node.bits,
		Nonce:                node.nonce,
	}
}

// SelectedAncestor returns the ancestor block node at the provided blue score by following
// the selected-parents chain backwards from this node. The returned block will be nil when a
// blue score is requested that is higher than the blue score of the passed node.
//
// This function is safe for concurrent access.
func (node *BlockNode) SelectedAncestor(blueScore uint64) *BlockNode {
	if blueScore > node.blueScore {
		return nil
	}

	n := node
	for n != nil && n.blueScore > blueScore {
		n = n.selectedParent
	}

	return n
}

// RelativeAncestor returns the ancestor block node a relative 'distance' of
// blue blocks before this node. This is equivalent to calling Ancestor with
// the node's blue score minus provided distance.
//
// This function is safe for concurrent access.
func (node *BlockNode) RelativeAncestor(distance uint64) *BlockNode {
	return node.SelectedAncestor(node.blueScore - distance)
}

func (node *BlockNode) ParentHashes() []*daghash.Hash {
	return node.parents.Hashes()
}

// IsGenesis returns if the current block is the genesis block
func (node *BlockNode) IsGenesis() bool {
	return len(node.parents) == 0
}

// String returns a string that contains the block hash.
func (node BlockNode) String() string {
	return node.hash.String()
}

func (node *BlockNode) Time() mstime.Time {
	return mstime.UnixMilliseconds(node.timestamp)
}

func (node *BlockNode) Status() BlockStatus {
	return node.status
}

func (node *BlockNode) SetStatus(status BlockStatus) {
	node.status = status
}

func (node *BlockNode) AddStatus(status BlockStatus) {
	node.status |= status
}

func (node *BlockNode) RemoveStatus(status BlockStatus) {
	node.status &^= status
}

func (node *BlockNode) Hash() *daghash.Hash {
	return node.hash
}

func (node *BlockNode) BlueScore() uint64 {
	return node.blueScore
}

func (node *BlockNode) SelectedParent() *BlockNode {
	return node.selectedParent
}

func (node *BlockNode) Blues() []*BlockNode {
	return node.blues
}

func (node *BlockNode) Timestamp() int64 {
	return node.timestamp
}

func (node *BlockNode) Bits() uint32 {
	return node.bits
}

func (node *BlockNode) Parents() BlockNodeSet {
	return node.parents
}

func (node *BlockNode) IsFinalized() bool {
	return node.isFinalized
}

func (node *BlockNode) SetFinalized(isFinalized bool) {
	node.isFinalized = isFinalized
}

func (node *BlockNode) UTXOCommitment() *daghash.Hash {
	return node.utxoCommitment
}

func (node *BlockNode) Children() BlockNodeSet {
	return node.children
}

func (node *BlockNode) Version() int32 {
	return node.version
}

func (node *BlockNode) SetSelectedParent(selectedParent *BlockNode) {
	node.selectedParent = selectedParent
}

func (node *BlockNode) SetBluesAnticoneSizes(bluesAnticoneSizes map[*BlockNode]dagconfig.KType) {
	node.bluesAnticoneSizes = bluesAnticoneSizes
}

func (node *BlockNode) BluesAnticoneSizes() map[*BlockNode]dagconfig.KType {
	return node.bluesAnticoneSizes
}

func (node *BlockNode) SetBlues(blues []*BlockNode) {
	node.blues = blues
}

func (node *BlockNode) SetBlueScore(blueScore uint64) {
	node.blueScore = blueScore
}

// BlueAnticoneSize returns the blue anticone size of 'block' from the worldview of 'context'.
// Expects 'block' to be in the blue set of 'context'
func BlueAnticoneSize(block, context *BlockNode) (dagconfig.KType, error) {
	for current := context; current != nil; current = current.SelectedParent() {
		if blueAnticoneSize, ok := current.bluesAnticoneSizes[block]; ok {
			return blueAnticoneSize, nil
		}
	}
	return 0, errors.Errorf("block %s is not in blue set of %s", block.Hash(), context.Hash())
}
