package delayedblocks

import (
	"github.com/kaspanet/kaspad/consensus/timesource"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/util/mstime"
	"time"
)

// DelayedBlockManager is a list of all delayed blocks. We are maintaining this
// list for the case where a new block with a valid timestamp points to a delayed block.
// In that case we will delay the processing of the child block so it would be processed
// after its parent.
type DelayedBlockManager struct {
	timeSource timesource.TimeSource

	delayedBlocks      map[daghash.Hash]*DelayedBlock
	delayedBlocksQueue delayedBlocksHeap
}

func New(timeSource timesource.TimeSource) *DelayedBlockManager {
	return &DelayedBlockManager{
		timeSource: timeSource,

		delayedBlocks:      make(map[daghash.Hash]*DelayedBlock),
		delayedBlocksQueue: newDelayedBlocksHeap(),
	}
}

func (db *DelayedBlockManager) Add(block *util.Block, processTime mstime.Time) {
	delayedBlock := &DelayedBlock{
		block:       block,
		processTime: processTime,
	}

	db.delayedBlocks[*block.Hash()] = delayedBlock
	db.delayedBlocksQueue.Push(delayedBlock)
}

// popDelayedBlock removes the topmost (delayed block with earliest process time) of the queue and returns it.
func (db *DelayedBlockManager) Pop() *DelayedBlock {
	delayedBlock := db.delayedBlocksQueue.pop()
	delete(db.delayedBlocks, *delayedBlock.block.Hash())
	return delayedBlock
}

func (db *DelayedBlockManager) Peek() *DelayedBlock {
	return db.delayedBlocksQueue.peek()
}

func (db *DelayedBlockManager) Len() int {
	return db.delayedBlocksQueue.Len()
}

func (db *DelayedBlockManager) IsKnownDelayed(hash *daghash.Hash) bool {
	_, ok := db.delayedBlocks[*hash]
	return ok
}

// MaxDelayOfParents returns the maximum delay of the given block hashes.
// Note that delay could be 0, but isDelayed will return true. This is the case where the parent process time is due.
func (db *DelayedBlockManager) MaxDelayOfParents(parentHashes []*daghash.Hash) (delay time.Duration, isDelayed bool) {
	for _, parentHash := range parentHashes {
		if delayedParent, exists := db.delayedBlocks[*parentHash]; exists {
			isDelayed = true
			parentDelay := delayedParent.ProcessTime().Sub(db.timeSource.Now())
			if parentDelay > delay {
				delay = parentDelay
			}
		}
	}

	return delay, isDelayed
}

// DelayedBlock represents a block which has a delayed timestamp and will be processed at processTime
type DelayedBlock struct {
	block       *util.Block
	processTime mstime.Time
}

func (db *DelayedBlock) Block() *util.Block {
	return db.block
}

func (db *DelayedBlock) ProcessTime() mstime.Time {
	return db.processTime
}
