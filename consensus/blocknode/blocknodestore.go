// Copyright (c) 2015-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blocknode

import (
	"github.com/kaspanet/kaspad/consensus/blockstatus"
	"github.com/kaspanet/kaspad/dbaccess"
	"sync"

	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/util/daghash"
)

// BlockNodeStore provides facilities for keeping track of an in-memory store of the
// block DAG.
type BlockNodeStore struct {
	dagParams *dagconfig.Params

	sync.RWMutex
	blockNodes map[daghash.Hash]*BlockNode
	dirty      map[*BlockNode]struct{}
}

// NewBlockNodeStore returns a new empty instance of a blockNode store. The store will
// be dynamically populated as block nodes are loaded from the database and
// manually added.
func NewBlockNodeStore(dagParams *dagconfig.Params) *BlockNodeStore {
	return &BlockNodeStore{
		dagParams:  dagParams,
		blockNodes: make(map[daghash.Hash]*BlockNode),
		dirty:      make(map[*BlockNode]struct{}),
	}
}

// HaveNode returns whether or not the block store contains the provided hash.
//
// This function is safe for concurrent access.
func (bi *BlockNodeStore) HaveNode(hash *daghash.Hash) bool {
	bi.RLock()
	defer bi.RUnlock()
	_, hasBlock := bi.blockNodes[*hash]
	return hasBlock
}

// LookupNode returns the block node identified by the provided hash. It will
// return nil if there is no entry for the hash.
//
// This function is safe for concurrent access.
func (bi *BlockNodeStore) LookupNode(hash *daghash.Hash) (*BlockNode, bool) {
	bi.RLock()
	defer bi.RUnlock()
	node, ok := bi.blockNodes[*hash]
	return node, ok
}

// AddNode adds the provided node to the store and marks it as dirty.
// Duplicate entries are not checked so it is up to caller to avoid adding them.
//
// This function is safe for concurrent access.
func (bi *BlockNodeStore) AddNode(node *BlockNode) {
	bi.Lock()
	defer bi.Unlock()
	bi.addNode(node)
	bi.dirty[node] = struct{}{}
}

// addNode adds the provided node to the store, but does not mark it as
// dirty. This can be used while initializing the block index.
//
// This function is NOT safe for concurrent access.
func (bi *BlockNodeStore) addNode(node *BlockNode) {
	bi.blockNodes[*node.Hash()] = node
}

// NodeStatus provides concurrent-safe access to the status field of a node.
//
// This function is safe for concurrent access.
func (bi *BlockNodeStore) NodeStatus(node *BlockNode) blockstatus.BlockStatus {
	bi.RLock()
	defer bi.RUnlock()
	status := node.Status()
	return status
}

// SetStatusFlags flips the provided status flags on the block node to on,
// regardless of whether they were on or off previously. This does not unset any
// flags currently on.
//
// This function is safe for concurrent access.
func (bi *BlockNodeStore) SetStatusFlags(node *BlockNode, flags blockstatus.BlockStatus) {
	bi.Lock()
	defer bi.Unlock()
	node.AddStatus(flags)
	bi.dirty[node] = struct{}{}
}

// UnsetStatusFlags flips the provided status flags on the block node to off,
// regardless of whether they were on or off previously.
//
// This function is safe for concurrent access.
func (bi *BlockNodeStore) UnsetStatusFlags(node *BlockNode, flags blockstatus.BlockStatus) {
	bi.Lock()
	defer bi.Unlock()
	node.RemoveStatus(flags)
	bi.dirty[node] = struct{}{}
}

// FlushToDB writes all dirty block nodes to the database.
func (bi *BlockNodeStore) FlushToDB(dbContext *dbaccess.TxContext) error {
	bi.Lock()
	defer bi.Unlock()
	if len(bi.dirty) == 0 {
		return nil
	}

	for node := range bi.dirty {
		serializedBlockNode, err := SerializeBlockNode(node)
		if err != nil {
			return err
		}
		key := BlockIndexKey(node.Hash(), node.BlueScore())
		err = dbaccess.StoreIndexBlock(dbContext, key, serializedBlockNode)
		if err != nil {
			return err
		}
	}
	return nil
}

func (bi *BlockNodeStore) ClearDirtyEntries() {
	bi.dirty = make(map[*BlockNode]struct{})
}

// ForEachHash runs the given fn on every hash in the store
//
// This function is NOT safe for concurrent access. It is meant to be
// used either on initialization or when the dag lock is held for reads.
func (bi *BlockNodeStore) ForEachHash(fn func(hash daghash.Hash) error) error {
	for hash := range bi.blockNodes {
		err := fn(hash)
		if err != nil {
			return err
		}
	}
	return nil
}
