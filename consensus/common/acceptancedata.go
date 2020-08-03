package common

import (
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
)

// TxAcceptanceData stores a transaction together with an indication
// if it was accepted or not by some block
type TxAcceptanceData struct {
	Tx         *util.Tx
	IsAccepted bool
}

// BlockTxsAcceptanceData stores all transactions in a block with an indication
// if they were accepted or not by some other block
type BlockTxsAcceptanceData struct {
	BlockHash        daghash.Hash
	TxAcceptanceData []TxAcceptanceData
}

// MultiBlockTxsAcceptanceData stores data about which transactions were accepted by a block
// It's a slice of the block's blues block IDs and their transaction acceptance data
type MultiBlockTxsAcceptanceData []BlockTxsAcceptanceData

// FindAcceptanceData finds the BlockTxsAcceptanceData that matches blockHash
func (data MultiBlockTxsAcceptanceData) FindAcceptanceData(blockHash *daghash.Hash) (*BlockTxsAcceptanceData, bool) {
	for _, acceptanceData := range data {
		if acceptanceData.BlockHash.IsEqual(blockHash) {
			return &acceptanceData, true
		}
	}
	return nil, false
}
