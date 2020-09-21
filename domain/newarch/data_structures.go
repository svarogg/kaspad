package newarch

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
	Add(GhostdagDatum) // TODO
}

type BlockStatuses interface {
}

type ReachabilityStore interface {
}

type ConsensusState interface {
}
type UTXODiffs interface {
}

type BlockHeaders interface {
}
