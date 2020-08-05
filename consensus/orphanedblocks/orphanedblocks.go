package orphanedblocks

import (
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/util/mstime"
	"sync"
	"time"
)

const (
	// maxOrphanBlocks is the maximum number of orphan blocks that can be
	// queued.
	maxOrphanBlocks = 100
)

// OrphanBlock represents a block that we don't yet have the parent for. It
// is a normal block plus an Expiration time to prevent caching the orphan
// forever.
type OrphanBlock struct {
	Block      *util.Block
	Expiration mstime.Time
}

type OrphanedBlockManager struct {
	sync.RWMutex

	blockNodeStore *blocknode.BlockNodeStore

	orphans       map[daghash.Hash]*OrphanBlock
	orphanParents map[daghash.Hash][]*OrphanBlock
	newestOrphan  *OrphanBlock
}

func NewManager(blockNodeStore *blocknode.BlockNodeStore) *OrphanedBlockManager {
	return &OrphanedBlockManager{
		blockNodeStore: blockNodeStore,

		orphans:       make(map[daghash.Hash]*OrphanBlock),
		orphanParents: make(map[daghash.Hash][]*OrphanBlock),
	}
}

// IsKnownOrphan returns whether the passed hash is currently a known orphan.
// Keep in mind that only a limited number of orphans are held onto for a
// limited amount of time, so this function must not be used as an absolute
// way to test if a block is an orphan block. A full block (as opposed to just
// its hash) must be passed to ProcessBlock for that purpose. However, calling
// ProcessBlock with an orphan that already exists results in an error, so this
// function provides a mechanism for a caller to intelligently detect *recent*
// duplicate orphans and react accordingly.
func (ob *OrphanedBlockManager) IsKnownOrphan(hash *daghash.Hash) bool {
	ob.RLock()
	defer ob.RUnlock()
	_, exists := ob.orphans[*hash]

	return exists
}

// GetOrphanMissingAncestorHashes returns all of the missing parents in the orphan's sub-DAG
func (ob *OrphanedBlockManager) GetOrphanMissingAncestorHashes(orphanHash *daghash.Hash) []*daghash.Hash {
	ob.RLock()
	defer ob.RUnlock()

	missingAncestorsHashes := make([]*daghash.Hash, 0)

	visited := make(map[daghash.Hash]bool)
	queue := []*daghash.Hash{orphanHash}
	for len(queue) > 0 {
		var current *daghash.Hash
		current, queue = queue[0], queue[1:]
		if !visited[*current] {
			visited[*current] = true
			orphan, orphanExists := ob.orphans[*current]
			if orphanExists {
				queue = append(queue, orphan.Block.MsgBlock().Header.ParentHashes...)
			} else {
				if !ob.blockNodeStore.HaveNode(current) && current != orphanHash {
					missingAncestorsHashes = append(missingAncestorsHashes, current)
				}
			}
		}
	}
	return missingAncestorsHashes
}

// RemoveOrphanBlock removes the passed orphan block from the orphan pool and
// previous orphan index.
func (ob *OrphanedBlockManager) RemoveOrphanBlock(orphan *OrphanBlock) {
	ob.Lock()
	defer ob.Unlock()

	// Remove the orphan block from the orphan pool.
	orphanHash := orphan.Block.Hash()
	delete(ob.orphans, *orphanHash)

	// Remove the reference from the previous orphan index too.
	for _, parentHash := range orphan.Block.MsgBlock().Header.ParentHashes {
		// An indexing for loop is intentionally used over a range here as range
		// does not reevaluate the slice on each iteration nor does it adjust the
		// index for the modified slice.
		orphans := ob.orphanParents[*parentHash]
		for i := 0; i < len(orphans); i++ {
			hash := orphans[i].Block.Hash()
			if hash.IsEqual(orphanHash) {
				orphans = append(orphans[:i], orphans[i+1:]...)
				i--
			}
		}

		// Remove the map entry altogether if there are no longer any orphans
		// which depend on the parent hash.
		if len(orphans) == 0 {
			delete(ob.orphanParents, *parentHash)
			continue
		}

		ob.orphanParents[*parentHash] = orphans
	}
}

// AddOrphanBlock adds the passed block (which is already determined to be
// an orphan prior calling this function) to the orphan pool. It lazily cleans
// up any expired blocks so a separate cleanup poller doesn't need to be run.
// It also imposes a maximum limit on the number of outstanding orphan
// blocks and will remove the oldest received orphan block if the limit is
// exceeded.
func (ob *OrphanedBlockManager) AddOrphanBlock(block *util.Block) {
	// Remove expired orphan blocks.
	for _, oBlock := range ob.orphans {
		if mstime.Now().After(oBlock.Expiration) {
			ob.RemoveOrphanBlock(oBlock)
			continue
		}

		// Update the newest orphan block pointer so it can be discarded
		// in case the orphan pool fills up.
		if ob.newestOrphan == nil || oBlock.Block.Timestamp().After(ob.newestOrphan.Block.Timestamp()) {
			ob.newestOrphan = oBlock
		}
	}

	// Limit orphan blocks to prevent memory exhaustion.
	if len(ob.orphans)+1 > maxOrphanBlocks {
		// If the new orphan is newer than the newest orphan on the orphan
		// pool, don't add it.
		if block.Timestamp().After(ob.newestOrphan.Block.Timestamp()) {
			return
		}
		// Remove the newest orphan to make room for the added one.
		ob.RemoveOrphanBlock(ob.newestOrphan)
		ob.newestOrphan = nil
	}

	// Protect concurrent access. This is intentionally done here instead
	// of near the top since RemoveOrphanBlock does its own locking and
	// the range iterator is not invalidated by removing map entries.
	ob.Lock()
	defer ob.Unlock()

	// Insert the block into the orphan map with an Expiration time
	// 1 hour from now.
	expiration := mstime.Now().Add(time.Hour)
	oBlock := &OrphanBlock{
		Block:      block,
		Expiration: expiration,
	}
	ob.orphans[*block.Hash()] = oBlock

	// Add to parent hash lookup index for faster dependency lookups.
	for _, parentHash := range block.MsgBlock().Header.ParentHashes {
		ob.orphanParents[*parentHash] = append(ob.orphanParents[*parentHash], oBlock)
	}
}

func (ob *OrphanedBlockManager) OrphanParents(parentHash *daghash.Hash) []*OrphanBlock {
	prevOrphan := ob.orphanParents[*parentHash]
	return prevOrphan
}

func (ob *OrphanedBlockManager) Len() int {
	return len(ob.orphans)
}
