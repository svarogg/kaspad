// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package virtualblock

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/ghostdag"
	"github.com/kaspanet/kaspad/consensus/utxo"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/pkg/errors"
	"sync"
)

// VirtualBlock is a virtual block whose parents are the tips of the DAG.
type VirtualBlock struct {
	mtx            sync.Mutex
	ghostdag       *ghostdag.GHOSTDAG
	params         *dagconfig.Params
	blockNodeStore *blocknode.BlockNodeStore
	utxoSet        *utxo.FullUTXOSet
	blocknode.BlockNode

	// selectedParentChainSet is a block set that includes all the blocks
	// that belong to the chain of selected parents from the virtual block.
	selectedParentChainSet blocknode.BlockNodeSet

	// selectedParentChainSlice is an ordered slice that includes all the
	// blocks that belong the the chain of selected parents from the
	// virtual block.
	selectedParentChainSlice []*blocknode.BlockNode
}

// NewVirtualBlock creates and returns a new VirtualBlock.
func NewVirtualBlock(ghostdag *ghostdag.GHOSTDAG, params *dagconfig.Params,
	blockNodeStore *blocknode.BlockNodeStore, tips blocknode.BlockNodeSet) *VirtualBlock {

	// The mutex is intentionally not held since this is a constructor.
	var virtual VirtualBlock
	virtual.ghostdag = ghostdag
	virtual.params = params
	virtual.blockNodeStore = blockNodeStore
	virtual.utxoSet = utxo.NewFullUTXOSet()
	virtual.selectedParentChainSet = blocknode.NewBlockNodeSet()
	virtual.selectedParentChainSlice = nil
	virtual.setTips(tips)

	return &virtual
}

func (v *VirtualBlock) SetUTXOSet(utxoSet *utxo.FullUTXOSet) {
	v.utxoSet = utxoSet
}

// setTips replaces the tips of the virtual block with the blocks in the
// given BlockNodeSet. This only differs from the exported version in that it
// is up to the caller to ensure the lock is held.
//
// This function MUST be called with the view mutex locked (for writes).
func (v *VirtualBlock) setTips(tips blocknode.BlockNodeSet) *common.ChainUpdates {
	oldSelectedParent := v.SelectedParent()
	node, _ := v.ghostdag.InitBlockNode(nil, tips)
	v.BlockNode = *node
	return v.updateSelectedParentSet(oldSelectedParent)
}

// updateSelectedParentSet updates the selectedParentSet to match the
// new selected parent of the virtual block.
// Every time the new selected parent is not a child of
// the old one, it updates the selected path by removing from
// the path blocks that are selected ancestors of the old selected
// parent and are not selected ancestors of the new one, and adding
// blocks that are selected ancestors of the new selected parent
// and aren't selected ancestors of the old one.
func (v *VirtualBlock) updateSelectedParentSet(oldSelectedParent *blocknode.BlockNode) *common.ChainUpdates {
	var intersectionNode *blocknode.BlockNode
	nodesToAdd := make([]*blocknode.BlockNode, 0)
	for node := v.BlockNode.SelectedParent(); intersectionNode == nil && node != nil; node = node.SelectedParent() {
		if v.selectedParentChainSet.Contains(node) {
			intersectionNode = node
		} else {
			nodesToAdd = append(nodesToAdd, node)
		}
	}

	if intersectionNode == nil && oldSelectedParent != nil {
		panic("updateSelectedParentSet: Cannot find intersection node. The block index may be corrupted.")
	}

	// Remove the nodes in the set from the oldSelectedParent down to the intersectionNode
	// Also, save the hashes of the removed blocks to removedChainBlockHashes
	removeCount := 0
	var removedChainBlockHashes []*daghash.Hash
	if intersectionNode != nil {
		for node := oldSelectedParent; !node.Hash().IsEqual(intersectionNode.Hash()); node = node.SelectedParent() {
			v.selectedParentChainSet.Remove(node)
			removedChainBlockHashes = append(removedChainBlockHashes, node.Hash())
			removeCount++
		}
	}
	// Remove the last removeCount nodes from the slice
	v.selectedParentChainSlice = v.selectedParentChainSlice[:len(v.selectedParentChainSlice)-removeCount]

	// Reverse nodesToAdd, since we collected them in reverse order
	for left, right := 0, len(nodesToAdd)-1; left < right; left, right = left+1, right-1 {
		nodesToAdd[left], nodesToAdd[right] = nodesToAdd[right], nodesToAdd[left]
	}
	// Add the nodes to the set and to the slice
	// Also, save the hashes of the added blocks to addedChainBlockHashes
	var addedChainBlockHashes []*daghash.Hash
	for _, node := range nodesToAdd {
		v.selectedParentChainSet.Add(node)
		addedChainBlockHashes = append(addedChainBlockHashes, node.Hash())
	}
	v.selectedParentChainSlice = append(v.selectedParentChainSlice, nodesToAdd...)

	return &common.ChainUpdates{
		RemovedChainBlockHashes: removedChainBlockHashes,
		AddedChainBlockHashes:   addedChainBlockHashes,
	}
}

// SetTips replaces the tips of the virtual block with the blocks in the
// given BlockNodeSet.
//
// This function is safe for concurrent access.
func (v *VirtualBlock) SetTips(tips blocknode.BlockNodeSet) {
	v.mtx.Lock()
	defer v.mtx.Unlock()
	v.setTips(tips)
}

// addTip adds the given tip to the set of tips in the virtual block.
// All former tips that happen to be the given tips parents are removed
// from the set. This only differs from the exported version in that it
// is up to the caller to ensure the lock is held.
//
// This function MUST be called with the view mutex locked (for writes).
func (v *VirtualBlock) addTip(newTip *blocknode.BlockNode) *common.ChainUpdates {
	updatedTips := v.Tips().Clone()
	for parent := range newTip.Parents() {
		updatedTips.Remove(parent)
	}

	updatedTips.Add(newTip)
	return v.setTips(updatedTips)
}

// AddTip adds the given tip to the set of tips in the virtual block.
// All former tips that happen to be the given tip's parents are removed
// from the set.
//
// This function is safe for concurrent access.
func (v *VirtualBlock) AddTip(newTip *blocknode.BlockNode) *common.ChainUpdates {
	v.mtx.Lock()
	defer v.mtx.Unlock()
	return v.addTip(newTip)
}

// Tips returns the current tip block nodes for the DAG. It will return
// an empty BlockNodeSet if there is no tip.
//
// This function is safe for concurrent access.
func (v *VirtualBlock) Tips() blocknode.BlockNodeSet {
	return v.Parents()
}

func (v *VirtualBlock) UTXOSet() *utxo.FullUTXOSet {
	return v.utxoSet
}

// OldestChainBlockWithBlueScoreGreaterThan finds the oldest chain block with a blue score
// greater than blueScore. If no such block exists, this method returns nil
func (v *VirtualBlock) OldestChainBlockWithBlueScoreGreaterThan(blueScore uint64) *blocknode.BlockNode {
	chainBlockIndex, ok := util.SearchSlice(len(v.selectedParentChainSlice), func(i int) bool {
		selectedPathNode := v.selectedParentChainSlice[i]
		return selectedPathNode.BlueScore() > blueScore
	})
	if !ok {
		return nil
	}
	return v.selectedParentChainSlice[chainBlockIndex]
}

// IsInSelectedParentChain returns whether or not a block hash is found in the selected
// parent chain. Note that this method returns an error if the given blockHash does not
// exist within the block node store.
func (v *VirtualBlock) IsInSelectedParentChain(blockHash *daghash.Hash) (bool, error) {
	blockNode, ok := v.blockNodeStore.LookupNode(blockHash)
	if !ok {
		str := fmt.Sprintf("block %s is not in the DAG", blockHash)
		return false, common.ErrNotInDAG(str)
	}
	return v.selectedParentChainSet.Contains(blockNode), nil
}

// SelectedParentChain returns the selected parent chain starting from blockHash (exclusive)
// up to the virtual (exclusive). If blockHash is nil then the genesis block is used. If
// blockHash is not within the select parent chain, go down its own selected parent chain,
// while collecting each block hash in removedChainHashes, until reaching a block within
// the main selected parent chain.
func (v *VirtualBlock) SelectedParentChain(blockHash *daghash.Hash) ([]*daghash.Hash, []*daghash.Hash, error) {
	if blockHash == nil {
		blockHash = v.params.GenesisHash
	}
	if !v.blockNodeStore.HaveNode(blockHash) {
		return nil, nil, errors.Errorf("blockHash %s does not exist in the DAG", blockHash)
	}

	// If blockHash is not in the selected parent chain, go down its selected parent chain
	// until we find a block that is in the main selected parent chain.
	var removedChainHashes []*daghash.Hash
	isBlockInSelectedParentChain, err := v.IsInSelectedParentChain(blockHash)
	if err != nil {
		return nil, nil, err
	}
	for !isBlockInSelectedParentChain {
		removedChainHashes = append(removedChainHashes, blockHash)

		node, ok := v.blockNodeStore.LookupNode(blockHash)
		if !ok {
			return nil, nil, errors.Errorf("block %s does not exist in the DAG", blockHash)
		}
		blockHash = node.SelectedParent().Hash()

		isBlockInSelectedParentChain, err = v.IsInSelectedParentChain(blockHash)
		if err != nil {
			return nil, nil, err
		}
	}

	// Find the index of the blockHash in the selectedParentChainSlice
	blockHashIndex := len(v.selectedParentChainSlice) - 1
	for blockHashIndex >= 0 {
		node := v.selectedParentChainSlice[blockHashIndex]
		if node.Hash().IsEqual(blockHash) {
			break
		}
		blockHashIndex--
	}

	// Copy all the addedChainHashes starting from blockHashIndex (exclusive)
	addedChainHashes := make([]*daghash.Hash, len(v.selectedParentChainSlice)-blockHashIndex-1)
	for i, node := range v.selectedParentChainSlice[blockHashIndex+1:] {
		addedChainHashes[i] = node.Hash()
	}

	return removedChainHashes, addedChainHashes, nil
}
