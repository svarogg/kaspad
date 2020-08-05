// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package difficulty

import (
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/blockwindow"
	"github.com/kaspanet/kaspad/consensus/virtualblock"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/bigintpool"
)

type DifficultyManager struct {
	params  *dagconfig.Params
	virtual *virtualblock.VirtualBlock

	// powMaxBits defines the highest allowed proof of work value for a
	// block in compact form.
	powMaxBits uint32
}

func NewManager(params *dagconfig.Params, virtual *virtualblock.VirtualBlock) *DifficultyManager {
	return &DifficultyManager{
		params:     params,
		virtual:    virtual,
		powMaxBits: util.BigToCompact(params.PowMax),
	}
}

// RequiredDifficulty calculates the required difficulty for a
// block given its bluest parent.
func (d *DifficultyManager) RequiredDifficulty(bluestParent *blocknode.BlockNode) uint32 {
	// Genesis block.
	if bluestParent == nil || bluestParent.BlueScore() < d.params.DifficultyAdjustmentWindowSize+1 {
		return d.powMaxBits
	}

	// Fetch window of dag.difficultyAdjustmentWindowSize + 1 so we can have dag.difficultyAdjustmentWindowSize block intervals
	timestampsWindow := blockwindow.BlueBlockWindow(bluestParent, d.params.DifficultyAdjustmentWindowSize+1)
	windowMinTimestamp, windowMaxTimeStamp := timestampsWindow.MinMaxTimestamps()

	// Remove the last block from the window so to calculate the average target of dag.difficultyAdjustmentWindowSize blocks
	targetsWindow := timestampsWindow[:d.params.DifficultyAdjustmentWindowSize]

	// Calculate new target difficulty as:
	// averageWindowTarget * (windowMinTimestamp / (targetTimePerBlock * windowSize))
	// The result uses integer division which means it will be slightly
	// rounded down.
	newTarget := bigintpool.Acquire(0)
	defer bigintpool.Release(newTarget)
	windowTimeStampDifference := bigintpool.Acquire(windowMaxTimeStamp - windowMinTimestamp)
	defer bigintpool.Release(windowTimeStampDifference)
	targetTimePerBlock := bigintpool.Acquire(d.params.TargetTimePerBlock.Milliseconds())
	defer bigintpool.Release(targetTimePerBlock)
	difficultyAdjustmentWindowSize := bigintpool.Acquire(int64(d.params.DifficultyAdjustmentWindowSize))
	defer bigintpool.Release(difficultyAdjustmentWindowSize)

	targetsWindow.AverageTarget(newTarget)
	newTarget.
		Mul(newTarget, windowTimeStampDifference).
		Div(newTarget, targetTimePerBlock).
		Div(newTarget, difficultyAdjustmentWindowSize)
	if newTarget.Cmp(d.params.PowMax) > 0 {
		return d.powMaxBits
	}
	newTargetBits := util.BigToCompact(newTarget)
	return newTargetBits
}

// NextRequiredDifficulty calculates the required difficulty for a block that will
// be built on top of the current tips.
func (d *DifficultyManager) NextRequiredDifficulty() uint32 {
	difficulty := d.RequiredDifficulty(d.virtual.Parents().Bluest())
	return difficulty
}
