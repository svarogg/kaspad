// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockdag

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/validation/blockvalidation"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/pkg/errors"
	"time"

	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
)

// IsInDAG determines whether a block with the given hash exists in
// the DAG.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) IsInDAG(hash *daghash.Hash) bool {
	return dag.blockNodeStore.HaveNode(hash)
}

// processOrphans determines if there are any orphans which depend on the passed
// block hash (they are no longer orphans if true) and potentially accepts them.
// It repeats the process for the newly accepted blocks (to detect further
// orphans which may no longer be orphans) until there are no more.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to maybeAcceptBlock.
//
// This function MUST be called with the DAG state lock held (for writes).
func (dag *BlockDAG) processOrphans(hash *daghash.Hash, flags common.BehaviorFlags) error {
	// Start with processing at least the passed hash. Leave a little room
	// for additional orphan blocks that need to be processed without
	// needing to grow the array in the common case.
	processHashes := make([]*daghash.Hash, 0, 10)
	processHashes = append(processHashes, hash)
	for len(processHashes) > 0 {
		// Pop the first hash to process from the slice.
		processHash := processHashes[0]
		processHashes[0] = nil // Prevent GC leak.
		processHashes = processHashes[1:]

		// Look up all orphans that are parented by the block we just
		// accepted.  An indexing for loop is
		// intentionally used over a range here as range does not
		// reevaluate the slice on each iteration nor does it adjust the
		// index for the modified slice.
		for i := 0; i < len(dag.orphanedBlocks.OrphanParents(processHash)); i++ {
			orphan := dag.orphanedBlocks.OrphanParents(processHash)[i]
			if orphan == nil {
				log.Warnf("Found a nil entry at index %d in the "+
					"orphan dependency list for block %s", i,
					processHash)
				continue
			}

			// Skip this orphan if one or more of its parents are
			// still missing.
			_, err := lookupParentNodes(orphan.Block, dag)
			if err != nil {
				var ruleErr common.RuleError
				if ok := errors.As(err, &ruleErr); ok && ruleErr.ErrorCode == common.ErrParentBlockUnknown {
					continue
				}
				return err
			}

			// Remove the orphan from the orphan pool.
			orphanHash := orphan.Block.Hash()
			dag.orphanedBlocks.RemoveOrphanBlock(orphan)
			i--

			// Potentially accept the block into the block DAG.
			err = dag.maybeAcceptBlock(orphan.Block, flags|common.BFWasUnorphaned)
			if err != nil {
				// Since we don't want to reject the original block because of
				// a bad unorphaned child, only return an error if it's not a RuleError.
				if !errors.As(err, &common.RuleError{}) {
					return err
				}
				log.Warnf("Verification failed for orphan block %s: %s", orphanHash, err)
			}

			// Add this block to the list of blocks to process so
			// any orphan blocks that depend on this block are
			// handled too.
			processHashes = append(processHashes, orphanHash)
		}
	}
	return nil
}

// ProcessBlock is the main workhorse for handling insertion of new blocks into
// the block DAG. It includes functionality such as rejecting duplicate
// blocks, ensuring blocks follow all rules, orphan handling, and insertion into
// the block DAG.
//
// When no errors occurred during processing, the first return value indicates
// whether or not the block is an orphan.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) ProcessBlock(block *util.Block, flags common.BehaviorFlags) (isOrphan bool, isDelayed bool, err error) {
	dag.dagLock.Lock()
	defer dag.dagLock.Unlock()
	return dag.processBlockNoLock(block, flags)
}

func (dag *BlockDAG) processBlockNoLock(block *util.Block, flags common.BehaviorFlags) (isOrphan bool, isDelayed bool, err error) {
	isAfterDelay := flags&common.BFAfterDelay == common.BFAfterDelay
	wasBlockStored := flags&common.BFWasStored == common.BFWasStored
	disallowDelay := flags&common.BFDisallowDelay == common.BFDisallowDelay
	disallowOrphans := flags&common.BFDisallowOrphans == common.BFDisallowOrphans

	blockHash := block.Hash()
	log.Tracef("Processing block %s", blockHash)

	// The block must not already exist in the DAG.
	if dag.IsInDAG(blockHash) && !wasBlockStored {
		return false, false, errors.Errorf("already have block %s", blockHash)
	}

	// The block must not already exist as an orphan.
	if dag.orphanedBlocks.IsKnownOrphan(blockHash) {
		str := fmt.Sprintf("already have block (orphan) %s", blockHash)
		return false, false, common.NewRuleError(common.ErrDuplicateBlock, str)
	}

	if dag.delayedBlocks.IsKnownDelayed(blockHash) {
		str := fmt.Sprintf("already have block (delayed) %s", blockHash)
		return false, false, common.NewRuleError(common.ErrDuplicateBlock, str)
	}

	if !isAfterDelay {
		// Perform preliminary sanity checks on the block and its transactions.
		delay, err := blockvalidation.CheckBlockSanity(block, dag.Params, dag.subnetworkID, dag.timeSource, flags)
		if err != nil {
			return false, false, err
		}

		if delay != 0 && disallowDelay {
			str := fmt.Sprintf("Cannot process blocks beyond the allowed time offset while the BFDisallowDelay flag is raised %s", blockHash)
			return false, true, common.NewRuleError(common.ErrDelayedBlockIsNotAllowed, str)
		}

		if delay != 0 {
			err = dag.addDelayedBlock(block, delay)
			if err != nil {
				return false, false, err
			}
			return false, true, nil
		}
	}

	var missingParents []*daghash.Hash
	for _, parentHash := range block.MsgBlock().Header.ParentHashes {
		if !dag.IsInDAG(parentHash) {
			missingParents = append(missingParents, parentHash)
		}
	}
	if len(missingParents) > 0 && disallowOrphans {
		str := fmt.Sprintf("Cannot process orphan blocks while the BFDisallowOrphans flag is raised %s", blockHash)
		return false, false, common.NewRuleError(common.ErrOrphanBlockIsNotAllowed, str)
	}

	// Handle the case of a block with a valid timestamp(non-delayed) which points to a delayed block.
	delay, isParentDelayed := dag.delayedBlocks.MaxDelayOfParents(missingParents)
	if isParentDelayed {
		// Add Millisecond to ensure that parent process time will be after its child.
		delay += time.Millisecond
		err := dag.addDelayedBlock(block, delay)
		if err != nil {
			return false, false, err
		}
		return false, true, err
	}

	// Handle orphan blocks.
	if len(missingParents) > 0 {
		// Some orphans during netsync are a normal part of the process, since the anticone
		// of the chain-split is never explicitly requested.
		// Therefore, if we are during netsync - don't report orphans to default logs.
		//
		// The number K*2 was chosen since in peace times anticone is limited to K blocks,
		// while some red block can make it a bit bigger, but much more than that indicates
		// there might be some problem with the netsync process.
		if flags&common.BFIsSync == common.BFIsSync && dagconfig.KType(dag.orphanedBlocks.Len()) < dag.Params.K*2 {
			log.Debugf("Adding orphan block %s. This is normal part of netsync process", blockHash)
		} else {
			log.Infof("Adding orphan block %s", blockHash)
		}
		dag.orphanedBlocks.AddOrphanBlock(block)

		return true, false, nil
	}

	// The block has passed all context independent checks and appears sane
	// enough to potentially accept it into the block DAG.
	err = dag.maybeAcceptBlock(block, flags)
	if err != nil {
		return false, false, err
	}

	// Accept any orphan blocks that depend on this block (they are
	// no longer orphans) and repeat for those accepted blocks until
	// there are no more.
	err = dag.processOrphans(blockHash, flags)
	if err != nil {
		return false, false, err
	}

	if !isAfterDelay {
		err = dag.processDelayedBlocks()
		if err != nil {
			return false, false, err
		}
	}

	dag.syncRate.AddBlockProcessingTimestamp()

	log.Debugf("Accepted block %s", blockHash)

	return false, false, nil
}
