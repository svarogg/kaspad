// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockdag

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/validation/blockvalidation"
	"github.com/kaspanet/kaspad/consensus/validation/utxovalidation"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
)

// checkBlockHeaderContext performs several validation checks on the block header
// which depend on its position within the block dag.
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: No checks are performed.
//
// This function MUST be called with the dag state lock held (for writes).
func (dag *BlockDAG) checkBlockHeaderContext(header *wire.BlockHeader, bluestParent *blocknode.BlockNode, fastAdd bool) error {
	if !fastAdd {
		if err := dag.validateDifficulty(header, bluestParent); err != nil {
			return err
		}

		if err := validateMedianTime(dag, header, bluestParent); err != nil {
			return err
		}
	}
	return nil
}

func validateMedianTime(dag *BlockDAG, header *wire.BlockHeader, bluestParent *blocknode.BlockNode) error {
	if !header.IsGenesis() {
		// Ensure the timestamp for the block header is not before the
		// median time of the last several blocks (medianTimeBlocks).
		medianTime := dag.PastMedianTime(bluestParent)
		if header.Timestamp.Before(medianTime) {
			str := fmt.Sprintf("block timestamp of %s is not after expected %s", header.Timestamp, medianTime)
			return common.NewRuleError(common.ErrTimeTooOld, str)
		}
	}

	return nil
}

func (dag *BlockDAG) validateDifficulty(header *wire.BlockHeader, bluestParent *blocknode.BlockNode) error {
	// Ensure the difficulty specified in the block header matches
	// the calculated difficulty based on the previous block and
	// difficulty retarget rules.
	expectedDifficulty := dag.difficulty.RequiredDifficulty(bluestParent)
	blockDifficulty := header.Bits
	if blockDifficulty != expectedDifficulty {
		str := fmt.Sprintf("block difficulty of %d is not the expected value of %d", blockDifficulty, expectedDifficulty)
		return common.NewRuleError(common.ErrUnexpectedDifficulty, str)
	}

	return nil
}

// validateParents validates that no parent is an ancestor of another parent, and no parent is finalized
func (dag *BlockDAG) validateParents(blockHeader *wire.BlockHeader, parents blocknode.BlockNodeSet) error {
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

			isAncestorOf, err := dag.isInPast(parentA, parentB)
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

// checkBlockContext peforms several validation checks on the block which depend
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
func (dag *BlockDAG) checkBlockContext(block *util.Block, parents blocknode.BlockNodeSet, flags common.BehaviorFlags) error {
	bluestParent := parents.Bluest()
	fastAdd := flags&common.BFFastAdd == common.BFFastAdd

	err := dag.validateParents(&block.MsgBlock().Header, parents)
	if err != nil {
		return err
	}

	// Perform all block header related validation checks.
	header := &block.MsgBlock().Header
	if err = dag.checkBlockHeaderContext(header, bluestParent, fastAdd); err != nil {
		return err
	}

	return nil
}

// CheckConnectBlockTemplate fully validates that connecting the passed block to
// the DAG does not violate any consensus rules, aside from the proof of
// work requirement.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) CheckConnectBlockTemplate(block *util.Block) error {
	dag.dagLock.RLock()
	defer dag.dagLock.RUnlock()
	return dag.CheckConnectBlockTemplateNoLock(block)
}

// CheckConnectBlockTemplateNoLock fully validates that connecting the passed block to
// the DAG does not violate any consensus rules, aside from the proof of
// work requirement. The block must connect to the current tip of the main dag.
func (dag *BlockDAG) CheckConnectBlockTemplateNoLock(block *util.Block) error {

	// Skip the proof of work check as this is just a block template.
	flags := common.BFNoPoWCheck

	header := block.MsgBlock().Header

	delay, err := blockvalidation.CheckBlockSanity(block, dag.Params, dag.subnetworkID, dag.timeSource, flags)
	if err != nil {
		return err
	}

	if delay != 0 {
		return errors.Errorf("Block timestamp is too far in the future")
	}

	parents, err := lookupParentNodes(block, dag)
	if err != nil {
		return err
	}

	err = dag.checkBlockContext(block, parents, flags)
	if err != nil {
		return err
	}

	templateNode, _ := dag.initBlockNode(&header, dag.virtual.Tips())

	_, err = utxovalidation.CheckConnectToPastUTXO(templateNode,
		dag.UTXOSet(), block.Transactions(), false, dag.Params, dag.sigCache, dag.pastMedianTimeFactory, dag.sequenceLockCalculator)

	return err
}
