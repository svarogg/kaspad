// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockdag

import (
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/difficulty"
	"github.com/kaspanet/kaspad/consensus/ghostdag"
	"github.com/kaspanet/kaspad/consensus/pastmediantime"
	"github.com/kaspanet/kaspad/consensus/reachability"
	"github.com/kaspanet/kaspad/consensus/timesource"
	"github.com/kaspanet/kaspad/consensus/virtualblock"
	"testing"

	"github.com/kaspanet/kaspad/util/mstime"

	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/wire"
)

// TestSetCoinbaseMaturity makes the ability to set the coinbase maturity
// available when running tests.
func (dag *BlockDAG) TestSetCoinbaseMaturity(maturity uint64) {
	dag.Params.BlockCoinbaseMaturity = maturity
}

// newTestDAG returns a DAG that is usable for syntetic tests. It is
// important to note that this DAG has no database associated with it, so
// it is not usable with all functions and the tests must take care when making
// use of it.
func newTestDAG(params *dagconfig.Params) *BlockDAG {
	blockNodeStore := blocknode.NewStore(params)
	dag := &BlockDAG{
		Params:         params,
		timeSource:     timesource.New(),
		blockNodeStore: blockNodeStore,
	}
	dag.reachabilityTree = reachability.NewReachabilityTree(blockNodeStore, params)
	dag.ghostdagManager = ghostdag.NewManager(dag.reachabilityTree, params, dag.timeSource)
	dag.pastMedianTimeManager = pastmediantime.NewManager(params)

	// Create a genesis block node and block blockNodeStore populated with it
	// on the above fake DAG.
	dag.genesis, _ = dag.initBlockNode(&params.GenesisBlock.Header, blocknode.NewBlockNodeSet())
	blockNodeStore.AddNode(dag.genesis)

	dag.virtual = virtualblock.New(dag.ghostdagManager, dag.Params, dag.blockNodeStore, blocknode.BlockNodeSetFromSlice(dag.genesis))
	dag.difficultyManager = difficulty.NewManager(params, dag.virtual)

	return dag
}

// newTestNode creates a block node connected to the passed parent with the
// provided fields populated and fake values for the other fields.
func newTestNode(dag *BlockDAG, parents blocknode.BlockNodeSet, blockVersion int32, bits uint32, timestamp mstime.Time) *blocknode.BlockNode {
	// Make up a header and create a block node from it.
	header := &wire.BlockHeader{
		Version:              blockVersion,
		ParentHashes:         parents.Hashes(),
		Bits:                 bits,
		Timestamp:            timestamp,
		HashMerkleRoot:       &daghash.ZeroHash,
		AcceptedIDMerkleRoot: &daghash.ZeroHash,
		UTXOCommitment:       &daghash.ZeroHash,
	}
	node, _ := dag.initBlockNode(header, parents)
	return node
}

func addNodeAsChildToParents(node *blocknode.BlockNode) {
	for parent := range node.Parents() {
		parent.Children().Add(node)
	}
}

func prepareAndProcessBlockByParentMsgBlocks(t *testing.T, dag *BlockDAG, parents ...*wire.MsgBlock) *wire.MsgBlock {
	parentHashes := make([]*daghash.Hash, len(parents))
	for i, parent := range parents {
		parentHashes[i] = parent.BlockHash()
	}
	return PrepareAndProcessBlockForTest(t, dag, parentHashes, nil)
}

func nodeByMsgBlock(t *testing.T, dag *BlockDAG, block *wire.MsgBlock) *blocknode.BlockNode {
	node, ok := dag.blockNodeStore.LookupNode(block.BlockHash())
	if !ok {
		t.Fatalf("couldn't find block node with hash %s", block.BlockHash())
	}
	return node
}

type fakeTimeSource struct {
	time mstime.Time
}

func (fts *fakeTimeSource) Now() mstime.Time {
	return fts.time
}

func newFakeTimeSource(fakeTime mstime.Time) common.TimeSource {
	return &fakeTimeSource{time: fakeTime}
}
