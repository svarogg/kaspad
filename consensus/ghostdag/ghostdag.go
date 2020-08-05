package ghostdag

import (
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/reachability"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
	"sort"
)

type GHOSTDAGManager struct {
	reachabilityTree *reachability.ReachabilityTree
	params           *dagconfig.Params
	timeSource       common.TimeSource
}

func NewManager(reachabilityTree *reachability.ReachabilityTree,
	params *dagconfig.Params, timeSource common.TimeSource) *GHOSTDAGManager {

	return &GHOSTDAGManager{
		reachabilityTree: reachabilityTree,
		params:           params,
		timeSource:       timeSource,
	}
}

// ghostdag runs the GHOSTDAG protocol and updates newNode.blues,
// newNode.selectedParent and newNode.bluesAnticoneSizes accordingly.
// The function updates newNode.blues by iterating over the blocks in
// the anticone of newNode.selectedParent (which is the parent with the
// highest blue score) and adds any block to newNode.blues if by adding
// it to newNode.blues these conditions will not be violated:
//
// 1) |anticone-of-candidate-block ∩ blue-set-of-newNode| ≤ K
//
// 2) For every blue block in blue-set-of-newNode:
//    |(anticone-of-blue-block ∩ blue-set-newNode) ∪ {candidate-block}| ≤ K.
//    We validate this condition by maintaining a map bluesAnticoneSizes for
//    each block which holds all the blue anticone sizes that were affected by
//    the new added blue blocks.
//    So to find out what is |anticone-of-blue ∩ blue-set-of-newNode| we just iterate in
//    the selected parent chain of newNode until we find an existing entry in
//    bluesAnticoneSizes.
//
// For further details see the article https://eprint.iacr.org/2018/104.pdf
func (g *GHOSTDAGManager) Run(newNode *blocknode.BlockNode) (selectedParentAnticone []*blocknode.BlockNode, err error) {
	newNode.SetSelectedParent(newNode.Parents().Bluest())
	newNode.SetBluesAnticoneSizes(make(map[*blocknode.BlockNode]dagconfig.KType))
	newNode.BluesAnticoneSizes()[newNode.SelectedParent()] = 0
	newNode.SetBlues([]*blocknode.BlockNode{newNode.SelectedParent()})
	selectedParentAnticone, err = g.selectedParentAnticone(newNode)
	if err != nil {
		return nil, err
	}

	sort.Slice(selectedParentAnticone, func(i, j int) bool {
		return selectedParentAnticone[i].Less(selectedParentAnticone[j])
	})

	for _, blueCandidate := range selectedParentAnticone {
		candidateBluesAnticoneSizes := make(map[*blocknode.BlockNode]dagconfig.KType)
		var candidateAnticoneSize dagconfig.KType
		possiblyBlue := true

		// Iterate over all blocks in the blue set of newNode that are not in the past
		// of blueCandidate, and check for each one of them if blueCandidate potentially
		// enlarges their blue anticone to be over K, or that they enlarge the blue anticone
		// of blueCandidate to be over K.
		for chainBlock := newNode; possiblyBlue; chainBlock = chainBlock.SelectedParent() {
			// If blueCandidate is in the future of chainBlock, it means
			// that all remaining blues are in the past of chainBlock and thus
			// in the past of blueCandidate. In this case we know for sure that
			// the anticone of blueCandidate will not exceed K, and we can mark
			// it as blue.
			//
			// newNode is always in the future of blueCandidate, so there's
			// no point in checking it.
			if chainBlock != newNode {
				if isAncestorOfBlueCandidate, err := g.isInPast(chainBlock, blueCandidate); err != nil {
					return nil, err
				} else if isAncestorOfBlueCandidate {
					break
				}
			}

			for _, block := range chainBlock.Blues() {
				// Skip blocks that exist in the past of blueCandidate.
				if isAncestorOfBlueCandidate, err := g.isInPast(block, blueCandidate); err != nil {
					return nil, err
				} else if isAncestorOfBlueCandidate {
					continue
				}

				candidateBluesAnticoneSizes[block], err = blocknode.BlueAnticoneSize(block, newNode)
				if err != nil {
					return nil, err
				}
				candidateAnticoneSize++

				if candidateAnticoneSize > g.params.K {
					// k-cluster violation: The candidate's blue anticone exceeded k
					possiblyBlue = false
					break
				}

				if candidateBluesAnticoneSizes[block] == g.params.K {
					// k-cluster violation: A block in candidate's blue anticone already
					// has k blue blocks in its own anticone
					possiblyBlue = false
					break
				}

				// This is a sanity check that validates that a blue
				// block's blue anticone is not already larger than K.
				if candidateBluesAnticoneSizes[block] > g.params.K {
					return nil, errors.New("found blue anticone size larger than k")
				}
			}
		}

		if possiblyBlue {
			// No k-cluster violation found, we can now set the candidate block as blue
			newNode.SetBlues(append(newNode.Blues(), blueCandidate))
			newNode.BluesAnticoneSizes()[blueCandidate] = candidateAnticoneSize
			for blue, blueAnticoneSize := range candidateBluesAnticoneSizes {
				newNode.BluesAnticoneSizes()[blue] = blueAnticoneSize + 1
			}

			// The maximum length of node.blues can be K+1 because
			// it contains the selected parent.
			if dagconfig.KType(len(newNode.Blues())) == g.params.K+1 {
				break
			}
		}
	}

	newNode.SetBlueScore(newNode.SelectedParent().BlueScore() + uint64(len(newNode.Blues())))
	return selectedParentAnticone, nil
}

// selectedParentAnticone returns the blocks in the anticone of the selected parent of the given node.
// The function work as follows.
// We start by adding all parents of the node (other than the selected parent) to a process queue.
// For each node in the queue:
//   we check whether it is in the past of the selected parent.
//   If not, we add the node to the resulting anticone-set and queue it for processing.
func (g *GHOSTDAGManager) selectedParentAnticone(node *blocknode.BlockNode) ([]*blocknode.BlockNode, error) {
	anticoneSet := blocknode.NewBlockNodeSet()
	var anticoneSlice []*blocknode.BlockNode
	selectedParentPast := blocknode.NewBlockNodeSet()
	var queue []*blocknode.BlockNode
	// Queueing all parents (other than the selected parent itself) for processing.
	for parent := range node.Parents() {
		if parent == node.SelectedParent() {
			continue
		}
		anticoneSet.Add(parent)
		anticoneSlice = append(anticoneSlice, parent)
		queue = append(queue, parent)
	}
	for len(queue) > 0 {
		var current *blocknode.BlockNode
		current, queue = queue[0], queue[1:]
		// For each parent of a the current node we check whether it is in the past of the selected parent. If not,
		// we add the it to the resulting anticone-set and queue it for further processing.
		for parent := range current.Parents() {
			if anticoneSet.Contains(parent) || selectedParentPast.Contains(parent) {
				continue
			}
			isAncestorOfSelectedParent, err := g.isInPast(parent, node.SelectedParent())
			if err != nil {
				return nil, err
			}
			if isAncestorOfSelectedParent {
				selectedParentPast.Add(parent)
				continue
			}
			anticoneSet.Add(parent)
			anticoneSlice = append(anticoneSlice, parent)
			queue = append(queue, parent)
		}
	}
	return anticoneSlice, nil
}

func (g *GHOSTDAGManager) isInPast(this *blocknode.BlockNode, other *blocknode.BlockNode) (bool, error) {
	return g.reachabilityTree.IsInPast(this, other)
}

// InitBlockNode returns a new block node for the given block header and parents, and the
// anticone of its selected parent (parent with highest blue score).
// selectedParentAnticone is used to update reachability data we store for future reachability queries.
// This function is NOT safe for concurrent access.
func (g *GHOSTDAGManager) InitBlockNode(blockHeader *wire.BlockHeader, parents blocknode.BlockNodeSet) (
	node *blocknode.BlockNode, selectedParentAnticone []*blocknode.BlockNode) {

	node = blocknode.NewBlockNode(blockHeader, parents, g.timeSource.Now())

	if len(node.Parents()) == 0 {
		return node, nil
	}

	selectedParentAnticone, err := g.Run(node)
	if err != nil {
		panic(errors.Wrap(err, "unexpected error in GHOSTDAG"))
	}
	return node, selectedParentAnticone
}
