// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockdag

import (
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/blockwindow"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/bigintpool"
)

// requiredDifficulty calculates the required difficulty for a
// block given its bluest parent.
func (dag *BlockDAG) requiredDifficulty(bluestParent *blocknode.BlockNode) uint32 {
	// Genesis block.
	if bluestParent == nil || bluestParent.BlueScore() < dag.Params.DifficultyAdjustmentWindowSize+1 {
		return dag.powMaxBits
	}

	// Fetch window of dag.difficultyAdjustmentWindowSize + 1 so we can have dag.difficultyAdjustmentWindowSize block intervals
	timestampsWindow := blockwindow.BlueBlockWindow(bluestParent, dag.Params.DifficultyAdjustmentWindowSize+1)
	windowMinTimestamp, windowMaxTimeStamp := timestampsWindow.MinMaxTimestamps()

	// Remove the last block from the window so to calculate the average target of dag.difficultyAdjustmentWindowSize blocks
	targetsWindow := timestampsWindow[:dag.Params.DifficultyAdjustmentWindowSize]

	// Calculate new target difficulty as:
	// averageWindowTarget * (windowMinTimestamp / (targetTimePerBlock * windowSize))
	// The result uses integer division which means it will be slightly
	// rounded down.
	newTarget := bigintpool.Acquire(0)
	defer bigintpool.Release(newTarget)
	windowTimeStampDifference := bigintpool.Acquire(windowMaxTimeStamp - windowMinTimestamp)
	defer bigintpool.Release(windowTimeStampDifference)
	targetTimePerBlock := bigintpool.Acquire(dag.Params.TargetTimePerBlock.Milliseconds())
	defer bigintpool.Release(targetTimePerBlock)
	difficultyAdjustmentWindowSize := bigintpool.Acquire(int64(dag.Params.DifficultyAdjustmentWindowSize))
	defer bigintpool.Release(difficultyAdjustmentWindowSize)

	targetsWindow.AverageTarget(newTarget)
	newTarget.
		Mul(newTarget, windowTimeStampDifference).
		Div(newTarget, targetTimePerBlock).
		Div(newTarget, difficultyAdjustmentWindowSize)
	if newTarget.Cmp(dag.Params.PowMax) > 0 {
		return dag.powMaxBits
	}
	newTargetBits := util.BigToCompact(newTarget)
	return newTargetBits
}

// NextRequiredDifficulty calculates the required difficulty for a block that will
// be built on top of the current tips.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) NextRequiredDifficulty() uint32 {
	difficulty := dag.requiredDifficulty(dag.virtual.Parents().Bluest())
	return difficulty
}
