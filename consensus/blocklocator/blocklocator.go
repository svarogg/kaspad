package blocklocator

import (
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/reachability"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
)

// BlockLocator is used to help locate a specific block. The algorithm for
// building the block locator is to add block hashes in reverse order on the
// block's selected parent chain until the desired stop block is reached.
// In order to keep the list of locator hashes to a reasonable number of entries,
// the step between each entry is doubled each loop iteration to exponentially
// decrease the number of hashes as a function of the distance from the block
// being located.
//
// For example, assume a selected parent chain with IDs as depicted below, and the
// stop block is genesis:
// 	genesis -> 1 -> 2 -> ... -> 15 -> 16  -> 17  -> 18
//
// The block locator for block 17 would be the hashes of blocks:
//  [17 16 14 11 7 2 genesis]
type BlockLocator []*daghash.Hash

type BlockLocatorManager struct {
	blockNodeStore   *blocknode.BlockNodeStore
	reachabilityTree *reachability.ReachabilityTree
	params           *dagconfig.Params
}

func NewManager(blockNodeStore *blocknode.BlockNodeStore,
	reachabilityTree *reachability.ReachabilityTree, params *dagconfig.Params) *BlockLocatorManager {

	return &BlockLocatorManager{
		blockNodeStore:   blockNodeStore,
		reachabilityTree: reachabilityTree,
		params:           params,
	}
}

// BlockLocatorFromHashes returns a block locator from high and low hash.
// See BlockLocator for details on the algorithm used to create a block locator.
func (blf *BlockLocatorManager) BlockLocatorFromHashes(highHash, lowHash *daghash.Hash) (BlockLocator, error) {
	highNode, ok := blf.blockNodeStore.LookupNode(highHash)
	if !ok {
		return nil, errors.Errorf("block %s is unknown", highHash)
	}

	lowNode, ok := blf.blockNodeStore.LookupNode(lowHash)
	if !ok {
		return nil, errors.Errorf("block %s is unknown", lowHash)
	}

	return blf.blockLocator(highNode, lowNode)
}

// blockLocator returns a block locator for the passed high and low nodes.
// See the BlockLocator type comments for more details.
func (blf *BlockLocatorManager) blockLocator(highNode, lowNode *blocknode.BlockNode) (BlockLocator, error) {
	// We use the selected parent of the high node, so the
	// block locator won't contain the high node.
	highNode = highNode.SelectedParent()

	node := highNode
	step := uint64(1)
	locator := make(BlockLocator, 0)
	for node != nil {
		locator = append(locator, node.Hash())

		// Nothing more to add once the low node has been added.
		if node.BlueScore() <= lowNode.BlueScore() {
			if node != lowNode {
				return nil, errors.Errorf("highNode and lowNode are " +
					"not in the same selected parent chain.")
			}
			break
		}

		// Calculate blueScore of previous node to include ensuring the
		// final node is lowNode.
		nextBlueScore := node.BlueScore() - step
		if nextBlueScore < lowNode.BlueScore() {
			nextBlueScore = lowNode.BlueScore()
		}

		// walk backwards through the nodes to the correct ancestor.
		node = node.SelectedAncestor(nextBlueScore)

		// Double the distance between included hashes.
		step *= 2
	}

	return locator, nil
}

// FindNextLocatorBoundaries returns the lowest unknown block locator, hash
// and the highest known block locator hash. This is used to create the
// next block locator to find the highest shared known chain block with the
// sync peer.
func (blf *BlockLocatorManager) FindNextLocatorBoundaries(locator BlockLocator) (highHash, lowHash *daghash.Hash) {
	// Find the most recent locator block hash in the DAG. In the case none of
	// the hashes in the locator are in the DAG, fall back to the genesis block.
	lowNode, _ := blf.blockNodeStore.LookupNode(blf.params.GenesisHash)
	nextBlockLocatorIndex := int64(len(locator) - 1)
	for i, hash := range locator {
		node, ok := blf.blockNodeStore.LookupNode(hash)
		if ok {
			lowNode = node
			nextBlockLocatorIndex = int64(i) - 1
			break
		}
	}
	if nextBlockLocatorIndex < 0 {
		return nil, lowNode.Hash()
	}
	return locator[nextBlockLocatorIndex], lowNode.Hash()
}

// AntiPastHashesBetween returns the hashes of the blocks between the
// lowHash's antiPast and highHash's antiPast, or up to the provided
// max number of block hashes.
func (blf *BlockLocatorManager) AntiPastHashesBetween(lowHash, highHash *daghash.Hash, maxHashes uint64) ([]*daghash.Hash, error) {
	nodes, err := blf.antiPastBetween(lowHash, highHash, maxHashes)
	if err != nil {
		return nil, err
	}
	hashes := make([]*daghash.Hash, len(nodes))
	for i, node := range nodes {
		hashes[i] = node.Hash()
	}
	return hashes, nil
}

// AntiPastHeadersBetween returns the headers of the blocks between the
// lowHash's antiPast and highHash's antiPast, or up to the provided
// max number of block headers.
func (blf *BlockLocatorManager) AntiPastHeadersBetween(lowHash, highHash *daghash.Hash, maxHeaders uint64) ([]*wire.BlockHeader, error) {
	nodes, err := blf.antiPastBetween(lowHash, highHash, maxHeaders)
	if err != nil {
		return nil, err
	}
	headers := make([]*wire.BlockHeader, len(nodes))
	for i, node := range nodes {
		headers[i] = node.Header()
	}
	return headers, nil
}

// antiPastBetween returns the blockNodes between the lowHash's antiPast
// and highHash's antiPast, or up to the provided max number of blocks.
//
// This function MUST be called with the DAG state lock held (for reads).
func (blf *BlockLocatorManager) antiPastBetween(lowHash, highHash *daghash.Hash, maxEntries uint64) ([]*blocknode.BlockNode, error) {
	lowNode, ok := blf.blockNodeStore.LookupNode(lowHash)
	if !ok {
		return nil, errors.Errorf("Couldn't find low hash %s", lowHash)
	}
	highNode, ok := blf.blockNodeStore.LookupNode(highHash)
	if !ok {
		return nil, errors.Errorf("Couldn't find high hash %s", highHash)
	}
	if lowNode.BlueScore() >= highNode.BlueScore() {
		return nil, errors.Errorf("Low hash blueScore >= high hash blueScore (%d >= %d)",
			lowNode.BlueScore(), highNode.BlueScore())
	}

	// In order to get no more then maxEntries blocks from the
	// future of the lowNode (including itself), we iterate the
	// selected parent chain of the highNode and stop once we reach
	// highNode.blueScore-lowNode.blueScore+1 <= maxEntries. That
	// stop point becomes the new highNode.
	// Using blueScore as an approximation is considered to be
	// fairly accurate because we presume that most DAG blocks are
	// blue.
	for highNode.BlueScore()-lowNode.BlueScore()+1 > maxEntries {
		highNode = highNode.SelectedParent()
	}

	// Collect every node in highNode's past (including itself) but
	// NOT in the lowNode's past (excluding itself) into an up-heap
	// (a heap sorted by blueScore from lowest to greatest).
	visited := blocknode.NewBlockNodeSet()
	candidateNodes := blocknode.NewUpHeap()
	queue := blocknode.NewDownHeap()
	queue.Push(highNode)
	for queue.Len() > 0 {
		current := queue.Pop()
		if visited.Contains(current) {
			continue
		}
		visited.Add(current)
		isCurrentAncestorOfLowNode, err := blf.reachabilityTree.IsInPast(current, lowNode)
		if err != nil {
			return nil, err
		}
		if isCurrentAncestorOfLowNode {
			continue
		}
		candidateNodes.Push(current)
		for parent := range current.Parents() {
			queue.Push(parent)
		}
	}

	// Pop candidateNodes into a slice. Since candidateNodes is
	// an up-heap, it's guaranteed to be ordered from low to high
	nodesLen := int(maxEntries)
	if candidateNodes.Len() < nodesLen {
		nodesLen = candidateNodes.Len()
	}
	nodes := make([]*blocknode.BlockNode, nodesLen)
	for i := 0; i < nodesLen; i++ {
		nodes[i] = candidateNodes.Pop()
	}
	return nodes, nil
}
