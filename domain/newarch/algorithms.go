package newarch

import (
	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/domain/blockdag"
	"github.com/kaspanet/kaspad/util/daghash"
)

type BlockIndex interface {
	Exists(hash *daghash.Hash) bool
}

type Pruning interface {
	PruneIfNeeded()
}

type Ghostdag interface {
	ghostdag(hash *daghash.Hash)
}

func newGhostdagImpl(_ Reachability, _ GhostdagData, _ BlockRelations) Ghostdag {
	return nil
}

type Difficulty interface {
	ValidateDifficulty(hash *daghash.Hash) error
	NextDifficulty() uint32 // bits
}

type UTXOSetVerifer interface {
	ValidateBlockTransactions(block *appmessage.MsgBlock)
	UpdateBlockUTXOSet(block *appmessage.MsgBlock) error               // TODO: Should somehow get the output from the above validation function
	GetUTXOByOutpoint(outpoint appmessage.Outpoint) blockdag.UTXOEntry // should be used only in mempool
}

func newUTXOSetVerifierImpl(_ MsgBlocks, _ UTXODiffs) UTXOSetVerifer {
	return nil
}

type Reachability interface {
	AddBlock(hash *daghash.Hash)
	IsInPastOf(a, b *daghash.Hash) bool
}

func newReachabilityImpl(_ ReachabilityStore, _ GhostdagData) {
}

type ConstructBlockTemplate interface {
	Construct(parents []*daghash.Hash, transactions []*appmessage.MsgTx) (appmessage.MsgBlock, error)
}

type InsertBlock interface {
	Insert(block *appmessage.MsgBlock) error
}

type Finality interface {
	validateFinality(hash *daghash.Hash) error
}

func newFinalityImpl(_ BlockRelations, _ GhostdagData, _ Reachability, _ BlockStatuses) Finality {
	return nil
}

type LimitVerifer interface {
	CheckBlockMass() error
	CheckMergeSetMass() error
	CheckMergeSetSize() error
	CheckMergeDepth() error
}
