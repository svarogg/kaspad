package blockvalidation

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/difficulty"
	"github.com/kaspanet/kaspad/consensus/pastmediantime"
	"github.com/kaspanet/kaspad/consensus/reachability"
	"github.com/kaspanet/kaspad/consensus/validation/transactionvalidation"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/wire"
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

// CheckBlockContext peforms several validation checks on the block which depend
// on its position within the block DAG.
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: The transaction are not checked to see if they are finalized
//    and the somewhat expensive BIP0034 validation is not performed.
//
// The flags are also passed to checkBlockHeaderContext. See its documentation
// for how the flags modify its behavior.
//
// This function MUST be called with the dag state lock held (for writes).
func CheckBlockContext(difficulty *difficulty.Difficulty,
	pastMedianTimeFactory *pastmediantime.PastMedianTimeFactory,
	reachabilityTree *reachability.ReachabilityTree,
	block *util.Block, parents blocknode.BlockNodeSet, flags common.BehaviorFlags) error {

	bluestParent := parents.Bluest()
	fastAdd := flags&common.BFFastAdd == common.BFFastAdd

	err := validateParents(reachabilityTree, &block.MsgBlock().Header, parents)
	if err != nil {
		return err
	}

	// Perform all block header related validation checks.
	header := &block.MsgBlock().Header
	if err = checkBlockHeaderContext(difficulty, pastMedianTimeFactory, header, bluestParent, fastAdd); err != nil {
		return err
	}

	return nil
}

// validateParents validates that no parent is an ancestor of another parent, and no parent is finalized
func validateParents(reachabilityTree *reachability.ReachabilityTree,
	blockHeader *wire.BlockHeader, parents blocknode.BlockNodeSet) error {

	for parentA := range parents {
		// isFinalized might be false-negative because node finality status is
		// updated in a separate goroutine. This is why later the block is
		// checked more thoroughly on the finality rules in dag.checkFinalityViolation.
		if parentA.IsFinalized() {
			return common.NewRuleError(common.ErrFinality, fmt.Sprintf("block %s is a finalized "+
				"parent of block %s", parentA.Hash(), blockHeader.BlockHash()))
		}

		for parentB := range parents {
			if parentA == parentB {
				continue
			}

			isAncestorOf, err := reachabilityTree.IsInPast(parentA, parentB)
			if err != nil {
				return err
			}
			if isAncestorOf {
				return common.NewRuleError(common.ErrInvalidParentsRelation, fmt.Sprintf("block %s is both a parent of %s and an"+
					" ancestor of another parent %s",
					parentA.Hash(),
					blockHeader.BlockHash(),
					parentB.Hash(),
				))
			}
		}
	}
	return nil
}

// checkBlockHeaderContext performs several validation checks on the block header
// which depend on its position within the block dag.
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: No checks are performed.
//
// This function MUST be called with the dag state lock held (for writes).
func checkBlockHeaderContext(difficulty *difficulty.Difficulty,
	pastMedianTimeFactory *pastmediantime.PastMedianTimeFactory,
	header *wire.BlockHeader, bluestParent *blocknode.BlockNode, fastAdd bool) error {

	if !fastAdd {
		if err := validateDifficulty(difficulty, header, bluestParent); err != nil {
			return err
		}

		if err := validateMedianTime(pastMedianTimeFactory, header, bluestParent); err != nil {
			return err
		}
	}
	return nil
}

func validateDifficulty(difficulty *difficulty.Difficulty, header *wire.BlockHeader, bluestParent *blocknode.BlockNode) error {
	// Ensure the difficulty specified in the block header matches
	// the calculated difficulty based on the previous block and
	// difficulty retarget rules.
	expectedDifficulty := difficulty.RequiredDifficulty(bluestParent)
	blockDifficulty := header.Bits
	if blockDifficulty != expectedDifficulty {
		str := fmt.Sprintf("block difficulty of %d is not the expected value of %d", blockDifficulty, expectedDifficulty)
		return common.NewRuleError(common.ErrUnexpectedDifficulty, str)
	}

	return nil
}

func validateMedianTime(pastMedianTimeFactory *pastmediantime.PastMedianTimeFactory, header *wire.BlockHeader, bluestParent *blocknode.BlockNode) error {
	if !header.IsGenesis() {
		// Ensure the timestamp for the block header is not before the
		// median time of the last several blocks (medianTimeBlocks).
		medianTime := pastMedianTimeFactory.PastMedianTime(bluestParent)
		if header.Timestamp.Before(medianTime) {
			str := fmt.Sprintf("block timestamp of %s is not after expected %s", header.Timestamp, medianTime)
			return common.NewRuleError(common.ErrTimeTooOld, str)
		}
	}

	return nil
}
