package finality

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/reachability"
	"github.com/kaspanet/kaspad/consensus/utxodiffstore"
	"github.com/kaspanet/kaspad/consensus/virtualblock"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/dbaccess"
	"github.com/kaspanet/kaspad/util/daghash"
)

type FinalityManager struct {
	params           *dagconfig.Params
	blockNodeStore   *blocknode.BlockNodeStore
	virtual          *virtualblock.VirtualBlock
	reachabilityTree *reachability.ReachabilityTree
	utxoDiffStore    *utxodiffstore.UTXODiffStore
	dbContext        *dbaccess.DatabaseContext

	lastFinalityPoint *blocknode.BlockNode
}

func NewManager(params *dagconfig.Params, blockNodeStore *blocknode.BlockNodeStore,
	virtual *virtualblock.VirtualBlock, reachabilityTree *reachability.ReachabilityTree,
	utxoDiffStore *utxodiffstore.UTXODiffStore, dbContext *dbaccess.DatabaseContext) *FinalityManager {

	return &FinalityManager{
		params:           params,
		blockNodeStore:   blockNodeStore,
		virtual:          virtual,
		reachabilityTree: reachabilityTree,
		utxoDiffStore:    utxoDiffStore,
		dbContext:        dbContext,
	}
}

// FinalityInterval is the interval that determines the finality window of the DAG.
func (fm *FinalityManager) FinalityInterval() uint64 {
	return uint64(fm.params.FinalityDuration / fm.params.TargetTimePerBlock)
}

// CheckFinalityViolation checks the new block does not violate the finality rules
// specifically - the new block selectedParent chain should contain the old finality point.
func (fm *FinalityManager) CheckFinalityViolation(newNode *blocknode.BlockNode) error {
	// the genesis block can not violate finality rules
	if newNode.IsGenesis() {
		return nil
	}

	// Because newNode doesn't have reachability data we
	// need to check if the last finality point is in the
	// selected parent chain of newNode.selectedParent, so
	// we explicitly check if newNode.selectedParent is
	// the finality point.
	if fm.lastFinalityPoint == newNode.SelectedParent() {
		return nil
	}

	isInSelectedChain, err := fm.isInSelectedParentChainOf(fm.lastFinalityPoint, newNode.SelectedParent())
	if err != nil {
		return err
	}

	if !isInSelectedChain {
		return common.NewRuleError(common.ErrFinality, "the last finality point is not in the selected parent chain of this block")
	}
	return nil
}

// isInSelectedParentChainOf returns whether `node` is in the selected parent chain of `other`.
func (fm *FinalityManager) isInSelectedParentChainOf(node *blocknode.BlockNode, other *blocknode.BlockNode) (bool, error) {
	// By definition, a node is not in the selected parent chain of itself.
	if node == other {
		return false, nil
	}

	return fm.reachabilityTree.IsReachabilityTreeAncestorOf(node, other)
}

// UpdateFinalityPoint updates the dag's last finality point if necessary.
func (fm *FinalityManager) UpdateFinalityPoint() {
	selectedTip := fm.virtual.SelectedParent()
	// if the selected tip is the genesis block - it should be the new finality point
	if selectedTip.IsGenesis() {
		fm.lastFinalityPoint = selectedTip
		return
	}
	// We are looking for a new finality point only if the new block's finality score is higher
	// by 2 than the existing finality point's
	if fm.FinalityScore(selectedTip) < fm.FinalityScore(fm.lastFinalityPoint)+2 {
		return
	}

	var currentNode *blocknode.BlockNode
	for currentNode = selectedTip.SelectedParent(); ; currentNode = currentNode.SelectedParent() {
		// We look for the first node in the selected parent chain that has a higher finality score than the last finality point.
		if fm.FinalityScore(currentNode.SelectedParent()) == fm.FinalityScore(fm.lastFinalityPoint) {
			break
		}
	}
	fm.lastFinalityPoint = currentNode
	spawn("dag.finalizeNodesBelowFinalityPoint", func() {
		fm.finalizeNodesBelowFinalityPoint(true)
	})
}

func (fm *FinalityManager) finalizeNodesBelowFinalityPoint(deleteDiffData bool) {
	queue := make([]*blocknode.BlockNode, 0, len(fm.lastFinalityPoint.Parents()))
	for parent := range fm.lastFinalityPoint.Parents() {
		queue = append(queue, parent)
	}
	var nodesToDelete []*blocknode.BlockNode
	if deleteDiffData {
		nodesToDelete = make([]*blocknode.BlockNode, 0, fm.FinalityInterval())
	}
	for len(queue) > 0 {
		var current *blocknode.BlockNode
		current, queue = queue[0], queue[1:]
		if !current.IsFinalized() {
			current.SetFinalized(true)
			if deleteDiffData {
				nodesToDelete = append(nodesToDelete, current)
			}
			for parent := range current.Parents() {
				queue = append(queue, parent)
			}
		}
	}
	if deleteDiffData {
		err := fm.utxoDiffStore.RemoveBlocksDiffData(fm.dbContext, nodesToDelete)
		if err != nil {
			panic(fmt.Sprintf("Error removing diff data from utxoDiffStore: %s", err))
		}
	}
}

// IsKnownFinalizedBlock returns whether the block is below the finality point.
// IsKnownFinalizedBlock might be false-negative because node finality status is
// updated in a separate goroutine. To get a definite answer if a block
// is finalized or not, use dag.CheckFinalityViolation.
func (fm *FinalityManager) IsKnownFinalizedBlock(blockHash *daghash.Hash) bool {
	node, ok := fm.blockNodeStore.LookupNode(blockHash)
	return ok && node.IsFinalized()
}

// LastFinalityPointHash returns the hash of the last finality point
func (fm *FinalityManager) LastFinalityPointHash() *daghash.Hash {
	if fm.lastFinalityPoint == nil {
		return nil
	}
	return fm.lastFinalityPoint.Hash()
}

func (fm *FinalityManager) FinalityScore(node *blocknode.BlockNode) uint64 {
	return node.BlueScore() / fm.FinalityInterval()
}

func (fm *FinalityManager) SetLastFinalityPoint(lastFinalityPoint *blocknode.BlockNode) {
	fm.lastFinalityPoint = lastFinalityPoint
	fm.finalizeNodesBelowFinalityPoint(false)
}
