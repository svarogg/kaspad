package newarch

// TODO: We pass hash everywhere, and it takes a lot of ram if we copy it, and if we don't copy it we need to check if
// it's safe to do pointer comparison.

import (
	"github.com/kaspanet/go-secp256k1"
	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/domain/blockdag"
	"github.com/kaspanet/kaspad/util/daghash"
)

type PruningPoint interface {
	Set(hash *daghash.Hash)
	Get() *daghash.Hash
}

type AcceptanceIndex interface {
	AddBlock(hash *daghash.Hash, txsAcceptanceData blockdag.MultiBlockTxsAcceptanceData)
	GetAcceptanceData(hash *daghash.Hash)
}

type MultisetStore interface {
	Set(hash *daghash.Hash)
	Get() secp256k1.MultiSet
}

type MsgBlocks interface {
	Add(block *appmessage.MsgBlock)
	Get(hash *daghash.Hash)
	DeleteBeforeHash(hash daghash.Hash)
}

type BlockRelations interface {
	Add(header appmessage.BlockHeader)
	IsChildOf(a, b daghash.Hash) bool
	IsParentOf(a, b daghash.Hash) bool
	Parents(hash daghash.Hash) []*daghash.Hash
	Children(hash daghash.Hash) []*daghash.Hash
}

type GhostdagData interface {
	Add(_ BlockGhostDAGData)
	Get(hash daghash.Hash) BlockGhostDAGData
}

type BlockGhostDAGData interface {
	BlueScore() uint64
	Blues() []*daghash.Hash
	Reds() []*daghash.Hash
	SelectedParent() *daghash.Hash
}

type blockStatus uint32
type BlockStatuses interface {
	Set(hash daghash.Hash, status blockStatus)
	Get(hash daghash.Hash) blockStatus
}

type ReachabilityStore interface {
	Set(hash daghash.Hash, data reachabilityData)
	Get(hash daghash.Hash) reachabilityData
}
type reachabilityData interface {
	// TBD
}

type UTXODiff interface {
	// TBD
}

type ConsensusState interface {
	UpdateWithDiff(diff UTXODiff)
	GetUTXOByOutpoint(outpoint appmessage.Outpoint) blockdag.UTXOEntry
}

type UTXODiffs interface {
	Set(hash daghash.Hash, diff UTXODiff)
	Get(hash daghash.Hash)
}

type BlockHeaders interface {
	Set(hash daghash.Hash, header appmessage.BlockHeader)
	Get(hash daghash.Hash) appmessage.BlockHeader
}
