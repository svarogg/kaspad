package common

import "github.com/kaspanet/kaspad/util/daghash"

// ChainUpdates represents the updates made to the selected parent chain after
// a block had been added to the DAG.
type ChainUpdates struct {
	RemovedChainBlockHashes []*daghash.Hash
	AddedChainBlockHashes   []*daghash.Hash
}
