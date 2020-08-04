package blockvalidation

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/pastmediantime"
	"github.com/kaspanet/kaspad/consensus/validation/transactionvalidation"
	"github.com/kaspanet/kaspad/util"
)

func ValidateAllTxsFinalized(block *util.Block, node *blocknode.BlockNode, bluestParent *blocknode.BlockNode,
	pastMedianTimeFactory *pastmediantime.PastMedianTimeFactory) error {

	blockTime := block.MsgBlock().Header.Timestamp
	if !block.IsGenesis() {
		blockTime = pastMedianTimeFactory.PastMedianTime(bluestParent)
	}

	// Ensure all transactions in the block are finalized.
	for _, tx := range block.Transactions() {
		if !transactionvalidation.IsFinalizedTransaction(tx, node.BlueScore(), blockTime) {
			str := fmt.Sprintf("block contains unfinalized "+
				"transaction %s", tx.ID())
			return common.NewRuleError(common.ErrUnfinalizedTx, str)
		}
	}

	return nil
}
