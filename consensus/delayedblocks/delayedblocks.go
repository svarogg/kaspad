package delayedblocks

import (
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/util/mstime"
)

// DelayedBlocks is a list of all delayed blocks. We are maintaining this
// list for the case where a new block with a valid timestamp points to a delayed block.
// In that case we will delay the processing of the child block so it would be processed
// after its parent.
type DelayedBlocks struct {
	delayedBlocks      map[daghash.Hash]*DelayedBlock
	delayedBlocksQueue delayedBlocksHeap
}

func New() *DelayedBlocks {
	return &DelayedBlocks{
		delayedBlocks:      make(map[daghash.Hash]*DelayedBlock),
		delayedBlocksQueue: newDelayedBlocksHeap(),
	}
}

func (db *DelayedBlocks) Add(block *util.Block, processTime mstime.Time) {
	delayedBlock := &DelayedBlock{
		block:       block,
		processTime: processTime,
	}

	db.delayedBlocks[*block.Hash()] = delayedBlock
	db.delayedBlocksQueue.Push(delayedBlock)
}

// popDelayedBlock removes the topmost (delayed block with earliest process time) of the queue and returns it.
func (db *DelayedBlocks) Pop() *DelayedBlock {
	delayedBlock := db.delayedBlocksQueue.pop()
	delete(db.delayedBlocks, *delayedBlock.block.Hash())
	return delayedBlock
}

func (db *DelayedBlocks) Peek() *DelayedBlock {
	return db.delayedBlocksQueue.peek()
}

func (db *DelayedBlocks) Len() int {
	return db.delayedBlocksQueue.Len()
}

func (db *DelayedBlocks) IsKnownDelayed(hash *daghash.Hash) bool {
	_, ok := db.delayedBlocks[*hash]
	return ok
}

func (db *DelayedBlocks) Get(hash *daghash.Hash) (*DelayedBlock, bool) {
	block, ok := db.delayedBlocks[*hash]
	return block, ok
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
