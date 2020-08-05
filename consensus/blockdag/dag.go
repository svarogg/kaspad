// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockdag

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/blocklocator"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/coinbase"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/delayedblocks"
	"github.com/kaspanet/kaspad/consensus/difficulty"
	"github.com/kaspanet/kaspad/consensus/finality"
	"github.com/kaspanet/kaspad/consensus/ghostdag"
	"github.com/kaspanet/kaspad/consensus/multiset"
	"github.com/kaspanet/kaspad/consensus/notifications"
	"github.com/kaspanet/kaspad/consensus/orphanedblocks"
	"github.com/kaspanet/kaspad/consensus/pastmediantime"
	"github.com/kaspanet/kaspad/consensus/reachability"
	"github.com/kaspanet/kaspad/consensus/sequencelock"
	"github.com/kaspanet/kaspad/consensus/subnetworks"
	"github.com/kaspanet/kaspad/consensus/syncrate"
	"github.com/kaspanet/kaspad/consensus/utxo"
	"github.com/kaspanet/kaspad/consensus/utxodiffstore"
	"github.com/kaspanet/kaspad/consensus/validation/blockvalidation"
	"github.com/kaspanet/kaspad/consensus/validation/merklevalidation"
	"github.com/kaspanet/kaspad/consensus/validation/utxovalidation"
	"github.com/kaspanet/kaspad/consensus/virtualblock"
	"github.com/kaspanet/kaspad/sigcache"
	"sync"
	"time"

	"github.com/kaspanet/kaspad/dbaccess"

	"github.com/pkg/errors"

	"github.com/kaspanet/kaspad/util/subnetworkid"

	"github.com/kaspanet/go-secp256k1"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/wire"
)

// BlockDAG provides functions for working with the kaspa block DAG.
// It includes functionality such as rejecting duplicate blocks, ensuring blocks
// follow all rules, and orphan handling.
type BlockDAG struct {
	Params          *dagconfig.Params
	subnetworkID    *subnetworkid.SubnetworkID
	databaseContext *dbaccess.DatabaseContext
	sigCache        *sigcache.SigCache

	timeSource             common.TimeSource
	notificationManager    *notifications.NotificationManager
	coinbaseManager        *coinbase.CoinbaseManager
	ghostdagManager        *ghostdag.GHOSTDAGManager
	blockLocatorManager    *blocklocator.BlockLocatorManager
	difficultyManager      *difficulty.DifficultyManager
	pastMedianTimeManager  *pastmediantime.PastMedianTimeManager
	syncRateManager        *syncrate.SyncRateManager
	sequenceLockCalculator *sequencelock.SequenceLockCalculator
	finalityManager        *finality.FinalityManager
	blockNodeStore         *blocknode.BlockNodeStore
	virtual                *virtualblock.VirtualBlock
	orphanedBlockManager   *orphanedblocks.OrphanedBlockManager
	delayedBlockManager    *delayedblocks.DelayedBlockManager
	utxoDiffStore          *utxodiffstore.UTXODiffStore
	multiSetManager        *multiset.MultiSetManager
	reachabilityTree       *reachability.ReachabilityTree

	// dagLock protects concurrent access to the vast majority of the
	// fields in this struct below this point.
	dagLock sync.RWMutex

	genesis *blocknode.BlockNode

	// blockCount holds the number of blocks in the DAG
	blockCount uint64
}

// New returns a BlockDAG instance using the provided configuration details.
func New(config *Config) (*BlockDAG, error) {
	// Enforce required config fields.
	if config.DAGParams == nil {
		return nil, errors.New("BlockDAG.New DAG parameters nil")
	}
	if config.TimeSource == nil {
		return nil, errors.New("BlockDAG.New timesource is nil")
	}
	if config.DatabaseContext == nil {
		return nil, errors.New("BlockDAG.DatabaseContext timesource is nil")
	}

	params := config.DAGParams

	blockNodeStore := blocknode.NewStore(params)
	dag := &BlockDAG{
		Params:                params,
		databaseContext:       config.DatabaseContext,
		timeSource:            config.TimeSource,
		sigCache:              config.SigCache,
		blockNodeStore:        blockNodeStore,
		delayedBlockManager:   delayedblocks.New(config.TimeSource),
		blockCount:            0,
		subnetworkID:          config.SubnetworkID,
		notificationManager:   notifications.NewManager(),
		coinbaseManager:       coinbase.NewManager(config.DatabaseContext, params),
		pastMedianTimeManager: pastmediantime.NewManager(params),
		syncRateManager:       syncrate.NewManager(params),
	}

	dag.multiSetManager = multiset.NewManager()
	dag.reachabilityTree = reachability.NewReachabilityTree(blockNodeStore, params)
	dag.ghostdagManager = ghostdag.NewManager(dag.reachabilityTree, params, dag.timeSource)
	dag.virtual = virtualblock.New(dag.ghostdagManager, params, dag.blockNodeStore, nil)
	dag.blockLocatorManager = blocklocator.NewManager(dag.blockNodeStore, dag.reachabilityTree, params)
	dag.utxoDiffStore = utxodiffstore.New(dag.databaseContext, blockNodeStore, dag.virtual)
	dag.difficultyManager = difficulty.NewManager(params, dag.virtual)
	dag.sequenceLockCalculator = sequencelock.NewCalculator(dag.virtual, dag.pastMedianTimeManager)
	dag.orphanedBlockManager = orphanedblocks.NewManager(blockNodeStore)
	dag.finalityManager = finality.NewManager(params, blockNodeStore, dag.virtual, dag.reachabilityTree, dag.utxoDiffStore, config.DatabaseContext)

	// Initialize the DAG state from the passed database. When the db
	// does not yet contain any DAG state, both it and the DAG state
	// will be initialized to contain only the genesis block.
	err := dag.initDAGState()
	if err != nil {
		return nil, err
	}

	genesis, ok := blockNodeStore.LookupNode(params.GenesisHash)

	if !ok {
		genesisBlock := util.NewBlock(dag.Params.GenesisBlock)
		// To prevent the creation of a new err variable unintentionally so the
		// defered function above could read err - declare isOrphan and isDelayed explicitly.
		var isOrphan, isDelayed bool
		isOrphan, isDelayed, err = dag.ProcessBlock(genesisBlock, common.BFNone)
		if err != nil {
			return nil, err
		}
		if isDelayed {
			return nil, errors.New("genesis block shouldn't be in the future")
		}
		if isOrphan {
			return nil, errors.New("genesis block is unexpectedly orphan")
		}
		genesis, ok = blockNodeStore.LookupNode(params.GenesisHash)
		if !ok {
			return nil, errors.New("genesis is not found in the DAG after it was proccessed")
		}
	}

	// Save a reference to the genesis block.
	dag.genesis = genesis

	selectedTip := dag.selectedTip()
	log.Infof("DAG state (blue score %d, hash %s)",
		selectedTip.BlueScore(), selectedTip.Hash())

	return dag, nil
}

// addBlock handles adding the passed block to the DAG.
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: Avoids several expensive transaction validation operations.
//
// This function MUST be called with the DAG state lock held (for writes).
func (dag *BlockDAG) addBlock(node *blocknode.BlockNode,
	block *util.Block, selectedParentAnticone []*blocknode.BlockNode, flags common.BehaviorFlags) (*common.ChainUpdates, error) {
	// Skip checks if node has already been fully validated.
	fastAdd := flags&common.BFFastAdd == common.BFFastAdd || dag.blockNodeStore.NodeStatus(node).KnownValid()

	// Connect the block to the DAG.
	chainUpdates, err := dag.connectBlock(node, block, selectedParentAnticone, fastAdd)
	if err != nil {
		if errors.As(err, &common.RuleError{}) {
			dag.blockNodeStore.SetStatusFlags(node, blocknode.StatusValidateFailed)

			dbTx, err := dag.databaseContext.NewTx()
			if err != nil {
				return nil, err
			}
			defer dbTx.RollbackUnlessClosed()
			err = dag.blockNodeStore.FlushToDB(dbTx)
			if err != nil {
				return nil, err
			}
			err = dbTx.Commit()
			if err != nil {
				return nil, err
			}
		}
		return nil, err
	}
	dag.blockCount++
	return chainUpdates, nil
}

// connectBlock handles connecting the passed node/block to the DAG.
//
// This function MUST be called with the DAG state lock held (for writes).
func (dag *BlockDAG) connectBlock(node *blocknode.BlockNode,
	block *util.Block, selectedParentAnticone []*blocknode.BlockNode, fastAdd bool) (*common.ChainUpdates, error) {

	if err := dag.finalityManager.CheckFinalityViolation(node); err != nil {
		return nil, err
	}

	if err := blockvalidation.ValidateGasLimit(dag.databaseContext, block); err != nil {
		return nil, err
	}

	newBlockPastUTXO, txsAcceptanceData, newBlockFeeData, newBlockMultiSet, err :=
		dag.verifyAndBuildUTXO(node, block.Transactions(), fastAdd)
	if err != nil {
		return nil, errors.Wrapf(err, "error verifying UTXO for %s", node)
	}

	err = dag.coinbaseManager.ValidateCoinbaseTransaction(node, block, txsAcceptanceData)
	if err != nil {
		return nil, err
	}

	// Apply all changes to the DAG.
	virtualUTXODiff, chainUpdates, err :=
		dag.applyDAGChanges(node, newBlockPastUTXO, newBlockMultiSet, selectedParentAnticone)
	if err != nil {
		// Since all validation logic has already ran, if applyDAGChanges errors out,
		// this means we have a problem in the internal structure of the DAG - a problem which is
		// irrecoverable, and it would be a bad idea to attempt adding any more blocks to the DAG.
		// Therefore - in such cases we panic.
		panic(err)
	}

	err = dag.saveChangesFromBlock(block, virtualUTXODiff, txsAcceptanceData, newBlockFeeData)
	if err != nil {
		return nil, err
	}

	return chainUpdates, nil
}

func (dag *BlockDAG) saveChangesFromBlock(block *util.Block, virtualUTXODiff *utxo.UTXODiff,
	txsAcceptanceData common.MultiBlockTxsAcceptanceData, feeData coinbase.CompactFeeData) error {

	dbTx, err := dag.databaseContext.NewTx()
	if err != nil {
		return err
	}
	defer dbTx.RollbackUnlessClosed()

	err = dag.blockNodeStore.FlushToDB(dbTx)
	if err != nil {
		return err
	}

	err = dag.utxoDiffStore.FlushToDB(dbTx)
	if err != nil {
		return err
	}

	err = dag.reachabilityTree.StoreState(dbTx)
	if err != nil {
		return err
	}

	err = dag.multiSetManager.FlushToDB(dbTx)
	if err != nil {
		return err
	}

	// Update DAG state.
	state := &dagState{
		TipHashes:         dag.TipHashes(),
		LastFinalityPoint: dag.finalityManager.LastFinalityPointHash(),
		LocalSubnetworkID: dag.subnetworkID,
	}
	err = saveDAGState(dbTx, state)
	if err != nil {
		return err
	}

	// Update the UTXO set using the diffSet that was melded into the
	// full UTXO set.
	err = utxo.UpdateUTXOSet(dbTx, virtualUTXODiff)
	if err != nil {
		return err
	}

	// Scan all accepted transactions and register any subnetwork registry
	// transaction. If any subnetwork registry transaction is not well-formed,
	// fail the entire block.
	err = subnetworks.RegisterSubnetworks(dbTx, block.Transactions())
	if err != nil {
		return err
	}

	// Apply the fee data into the database
	err = dbaccess.StoreFeeData(dbTx, block.Hash(), feeData)
	if err != nil {
		return err
	}

	err = dbTx.Commit()
	if err != nil {
		return err
	}

	dag.blockNodeStore.ClearDirtyEntries()
	dag.utxoDiffStore.ClearDirtyEntries()
	dag.utxoDiffStore.ClearOldEntries()
	dag.reachabilityTree.ClearDirtyEntries()
	dag.multiSetManager.ClearNewEntries()

	return nil
}

// applyDAGChanges does the following:
// 1. Connects each of the new block's parents to the block.
// 2. Adds the new block to the DAG's tips.
// 3. Updates the DAG's full UTXO set.
// 4. Updates each of the tips' utxoDiff.
// 5. Applies the new virtual's blue score to all the unaccepted UTXOs
// 6. Adds the block to the reachability structures
// 7. Adds the multiset of the block to the multiset store.
// 8. Updates the finality point of the DAG (if required).
//
// It returns the diff in the virtual block's UTXO set.
//
// This function MUST be called with the DAG state lock held (for writes).
func (dag *BlockDAG) applyDAGChanges(node *blocknode.BlockNode, newBlockPastUTXO utxo.UTXOSet,
	newBlockMultiset *secp256k1.MultiSet, selectedParentAnticone []*blocknode.BlockNode) (
	virtualUTXODiff *utxo.UTXODiff, chainUpdates *common.ChainUpdates, err error) {

	// Add the block to the reachability tree
	err = dag.reachabilityTree.AddBlock(node, selectedParentAnticone, dag.SelectedTipBlueScore())
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed adding block to the reachability tree")
	}

	dag.multiSetManager.SetMultiset(node.Hash(), newBlockMultiset)

	if err = dag.updateParents(node, newBlockPastUTXO); err != nil {
		return nil, nil, errors.Wrapf(err, "failed updating parents of %s", node)
	}

	// Update the virtual block's parents (the DAG tips) to include the new block.
	chainUpdates = dag.virtual.AddTip(node)

	// Build a UTXO set for the new virtual block
	newVirtualUTXO, _, _, err := dag.pastUTXO(&dag.virtual.BlockNode)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not restore past UTXO for virtual")
	}

	// Apply new utxoDiffs to all the tips
	err = updateTipsUTXO(dag, newVirtualUTXO)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed updating the tips' UTXO")
	}

	// It is now safe to meld the UTXO set to base.
	diffSet := newVirtualUTXO.(*utxo.DiffUTXOSet)
	virtualUTXODiff = diffSet.UTXODiff
	err = dag.meldVirtualUTXO(diffSet)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed melding the virtual UTXO")
	}

	dag.blockNodeStore.SetStatusFlags(node, blocknode.StatusValid)

	// And now we can update the finality point of the DAG (if required)
	dag.finalityManager.UpdateFinalityPoint()

	return virtualUTXODiff, chainUpdates, nil
}

func (dag *BlockDAG) meldVirtualUTXO(newVirtualUTXODiffSet *utxo.DiffUTXOSet) error {
	return newVirtualUTXODiffSet.MeldToBase()
}

// verifyAndBuildUTXO verifies all transactions in the given block and builds its UTXO
// to save extra traversals it returns the transactions acceptance data, the compactFeeData
// for the new block and its multiset.
func (dag *BlockDAG) verifyAndBuildUTXO(node *blocknode.BlockNode, transactions []*util.Tx, fastAdd bool) (
	newBlockUTXO utxo.UTXOSet, txsAcceptanceData common.MultiBlockTxsAcceptanceData, newBlockFeeData coinbase.CompactFeeData, multiset *secp256k1.MultiSet, err error) {

	pastUTXO, selectedParentPastUTXO, txsAcceptanceData, err := dag.pastUTXO(node)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	err = merklevalidation.ValidateAcceptedIDMerkleRoot(node, txsAcceptanceData)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	feeData, err := utxovalidation.CheckConnectToPastUTXO(node, pastUTXO, transactions, fastAdd, dag.Params, dag.sigCache, dag.pastMedianTimeManager, dag.sequenceLockCalculator)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	multiset, err = dag.multiSetManager.CalcMultiset(node, txsAcceptanceData, selectedParentPastUTXO)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	calculatedMultisetHash := daghash.Hash(*multiset.Finalize())
	if !calculatedMultisetHash.IsEqual(node.UTXOCommitment()) {
		str := fmt.Sprintf("block %s UTXO commitment is invalid - block "+
			"header indicates %s, but calculated value is %s", node.Hash(),
			node.UTXOCommitment(), calculatedMultisetHash)
		return nil, nil, nil, nil, common.NewRuleError(common.ErrBadUTXOCommitment, str)
	}

	return pastUTXO, txsAcceptanceData, feeData, multiset, nil
}

func genesisPastUTXO(virtual *virtualblock.VirtualBlock) (utxo.UTXOSet, error) {
	// The genesis has no past UTXO, so we create an empty UTXO
	// set by creating a diff UTXO set with the virtual UTXO
	// set, and adding all of its entries in toRemove
	diff := utxo.NewUTXODiff()
	for outpoint, entry := range virtual.UTXOSet().UTXOCollection {
		err := diff.RemoveEntry(outpoint, entry)
		if err != nil {
			return nil, err
		}
	}
	genesisPastUTXO := utxo.UTXOSet(utxo.NewDiffUTXOSet(virtual.UTXOSet(), diff))
	return genesisPastUTXO, nil
}

func (dag *BlockDAG) fetchBlueBlocks(node *blocknode.BlockNode) ([]*util.Block, error) {
	blueBlocks := make([]*util.Block, len(node.Blues()))
	for i, blueBlockNode := range node.Blues() {
		blueBlock, err := dag.fetchBlockByHash(blueBlockNode.Hash())
		if err != nil {
			return nil, err
		}

		blueBlocks[i] = blueBlock
	}
	return blueBlocks, nil
}

// applyBlueBlocks adds all transactions in the blue blocks to the selectedParent's past UTXO set
// Purposefully ignoring failures - these are just unaccepted transactions
// Writing down which transactions were accepted or not in txsAcceptanceData
func (dag *BlockDAG) applyBlueBlocks(node *blocknode.BlockNode, selectedParentPastUTXO utxo.UTXOSet, blueBlocks []*util.Block) (
	pastUTXO utxo.UTXOSet, multiBlockTxsAcceptanceData common.MultiBlockTxsAcceptanceData, err error) {

	pastUTXO = selectedParentPastUTXO.(*utxo.DiffUTXOSet).CloneWithoutBase()
	multiBlockTxsAcceptanceData = make(common.MultiBlockTxsAcceptanceData, len(blueBlocks))

	// Add blueBlocks to multiBlockTxsAcceptanceData in topological order. This
	// is so that anyone who iterates over it would process blocks (and transactions)
	// in their order of appearance in the DAG.
	for i := 0; i < len(blueBlocks); i++ {
		blueBlock := blueBlocks[i]
		transactions := blueBlock.Transactions()
		blockTxsAcceptanceData := common.BlockTxsAcceptanceData{
			BlockHash:        *blueBlock.Hash(),
			TxAcceptanceData: make([]common.TxAcceptanceData, len(transactions)),
		}
		isSelectedParent := i == 0

		for j, tx := range blueBlock.Transactions() {
			var isAccepted bool

			// Coinbase transaction outputs are added to the UTXO
			// only if they are in the selected parent chain.
			if !isSelectedParent && tx.IsCoinBase() {
				isAccepted = false
			} else {
				isAccepted, err = pastUTXO.AddTx(tx.MsgTx(), node.BlueScore())
				if err != nil {
					return nil, nil, err
				}
			}
			blockTxsAcceptanceData.TxAcceptanceData[j] = common.TxAcceptanceData{Tx: tx, IsAccepted: isAccepted}
		}
		multiBlockTxsAcceptanceData[i] = blockTxsAcceptanceData
	}

	return pastUTXO, multiBlockTxsAcceptanceData, nil
}

// updateParents adds this block to the children sets of its parents
// and updates the diff of any parent whose DiffChild is this block
func (dag *BlockDAG) updateParents(node *blocknode.BlockNode, newBlockUTXO utxo.UTXOSet) error {
	node.UpdateParentsChildren()
	return dag.updateParentsDiffs(node, newBlockUTXO)
}

// updateParentsDiffs updates the diff of any parent whose DiffChild is this block
func (dag *BlockDAG) updateParentsDiffs(node *blocknode.BlockNode, newBlockUTXO utxo.UTXOSet) error {
	virtualDiffFromNewBlock, err := dag.virtual.UTXOSet().DiffFrom(newBlockUTXO)
	if err != nil {
		return err
	}

	err = dag.utxoDiffStore.SetBlockDiff(node, virtualDiffFromNewBlock)
	if err != nil {
		return err
	}

	for parent := range node.Parents() {
		diffChild, err := dag.utxoDiffStore.DiffChildByNode(parent)
		if err != nil {
			return err
		}
		if diffChild == nil {
			parentPastUTXO, err := dag.restorePastUTXO(parent)
			if err != nil {
				return err
			}
			err = dag.utxoDiffStore.SetBlockDiffChild(parent, node)
			if err != nil {
				return err
			}
			diff, err := newBlockUTXO.DiffFrom(parentPastUTXO)
			if err != nil {
				return err
			}
			err = dag.utxoDiffStore.SetBlockDiff(parent, diff)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// pastUTXO returns the UTXO of a given block's past
// To save traversals over the blue blocks, it also returns the transaction acceptance data for
// all blue blocks
func (dag *BlockDAG) pastUTXO(node *blocknode.BlockNode) (
	pastUTXO, selectedParentPastUTXO utxo.UTXOSet, bluesTxsAcceptanceData common.MultiBlockTxsAcceptanceData, err error) {

	if node.IsGenesis() {
		genesisPastUTXO, err := genesisPastUTXO(dag.virtual)
		if err != nil {
			return nil, nil, nil, err
		}
		return genesisPastUTXO, nil, common.MultiBlockTxsAcceptanceData{}, nil
	}

	selectedParentPastUTXO, err = dag.restorePastUTXO(node.SelectedParent())
	if err != nil {
		return nil, nil, nil, err
	}

	blueBlocks, err := dag.fetchBlueBlocks(node)
	if err != nil {
		return nil, nil, nil, err
	}

	pastUTXO, bluesTxsAcceptanceData, err = dag.applyBlueBlocks(node, selectedParentPastUTXO, blueBlocks)
	if err != nil {
		return nil, nil, nil, err
	}

	return pastUTXO, selectedParentPastUTXO, bluesTxsAcceptanceData, nil
}

// restorePastUTXO restores the UTXO of a given block from its diff
func (dag *BlockDAG) restorePastUTXO(node *blocknode.BlockNode) (utxo.UTXOSet, error) {
	stack := []*blocknode.BlockNode{}

	// Iterate over the chain of diff-childs from node till virtual and add them
	// all into a stack
	for current := node; current != nil; {
		stack = append(stack, current)
		var err error
		current, err = dag.utxoDiffStore.DiffChildByNode(current)
		if err != nil {
			return nil, err
		}
	}

	// Start with the top item in the stack, going over it top-to-bottom,
	// applying the UTXO-diff one-by-one.
	topNode, stack := stack[len(stack)-1], stack[:len(stack)-1] // pop the top item in the stack
	topNodeDiff, err := dag.utxoDiffStore.DiffByNode(topNode)
	if err != nil {
		return nil, err
	}
	accumulatedDiff := topNodeDiff.Clone()

	for i := len(stack) - 1; i >= 0; i-- {
		diff, err := dag.utxoDiffStore.DiffByNode(stack[i])
		if err != nil {
			return nil, err
		}
		// Use withDiffInPlace, otherwise copying the diffs again and again create a polynomial overhead
		err = accumulatedDiff.WithDiffInPlace(diff)
		if err != nil {
			return nil, err
		}
	}

	return utxo.NewDiffUTXOSet(dag.virtual.UTXOSet(), accumulatedDiff), nil
}

// updateTipsUTXO builds and applies new diff UTXOs for all the DAG's tips
func updateTipsUTXO(dag *BlockDAG, virtualUTXO utxo.UTXOSet) error {
	for tip := range dag.virtual.Parents() {
		tipPastUTXO, err := dag.restorePastUTXO(tip)
		if err != nil {
			return err
		}
		diff, err := virtualUTXO.DiffFrom(tipPastUTXO)
		if err != nil {
			return err
		}
		err = dag.utxoDiffStore.SetBlockDiff(tip, diff)
		if err != nil {
			return err
		}
	}

	return nil
}

// UTXOConfirmations returns the confirmations for the given outpoint, if it exists
// in the DAG's UTXO set.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) UTXOConfirmations(outpoint *wire.Outpoint) (uint64, bool) {
	dag.dagLock.RLock()
	defer dag.dagLock.RUnlock()

	utxoEntry, ok := dag.GetUTXOEntry(*outpoint)
	if !ok {
		return 0, false
	}
	confirmations := dag.SelectedTipBlueScore() - utxoEntry.BlockBlueScore() + 1

	return confirmations, true
}

// blockConfirmations returns the current confirmations number of the given node
// The confirmations number is defined as follows:
// * If the node is in the selected tip red set	-> 0
// * If the node is the selected tip			-> 1
// * Otherwise									-> selectedTip.blueScore - acceptingBlock.blueScore + 2
func (dag *BlockDAG) blockConfirmations(node *blocknode.BlockNode) (uint64, error) {
	acceptingBlock, err := dag.acceptingBlock(node)
	if err != nil {
		return 0, err
	}

	// if acceptingBlock is nil, the node is red
	if acceptingBlock == nil {
		return 0, nil
	}

	return dag.selectedTip().BlueScore() - acceptingBlock.BlueScore() + 1, nil
}

// acceptingBlock finds the node in the selected-parent chain that had accepted
// the given node
func (dag *BlockDAG) acceptingBlock(node *blocknode.BlockNode) (*blocknode.BlockNode, error) {
	// Return an error if the node is the virtual block
	if node == &dag.virtual.BlockNode {
		return nil, errors.New("cannot get acceptingBlock for virtual")
	}

	// If the node is a chain-block itself, the accepting block is its chain-child
	isNodeInSelectedParentChain, err := dag.virtual.IsInSelectedParentChain(node.Hash())
	if err != nil {
		return nil, err
	}
	if isNodeInSelectedParentChain {
		if len(node.Children()) == 0 {
			// If the node is the selected tip, it doesn't have an accepting block
			return nil, nil
		}
		for child := range node.Children() {
			isChildInSelectedParentChain, err := dag.virtual.IsInSelectedParentChain(child.Hash())
			if err != nil {
				return nil, err
			}
			if isChildInSelectedParentChain {
				return child, nil
			}
		}
		return nil, errors.Errorf("chain block %s does not have a chain child", node.Hash())
	}

	// Find the only chain block that may contain the node in its blues
	candidateAcceptingBlock := dag.virtual.OldestChainBlockWithBlueScoreGreaterThan(node.BlueScore())

	// if no candidate is found, it means that the node has same or more
	// blue score than the selected tip and is found in its anticone, so
	// it doesn't have an accepting block
	if candidateAcceptingBlock == nil {
		return nil, nil
	}

	// candidateAcceptingBlock is the accepting block only if it actually contains
	// the node in its blues
	for _, blue := range candidateAcceptingBlock.Blues() {
		if blue == node {
			return candidateAcceptingBlock, nil
		}
	}

	// Otherwise, the node is red or in the selected tip anticone, and
	// doesn't have an accepting block
	return nil, nil
}

func (dag *BlockDAG) addDelayedBlock(block *util.Block, delay time.Duration) error {
	processTime := dag.Now().Add(delay)
	log.Debugf("Adding block to delayed blocks queue (block hash: %s, process time: %s)", block.Hash().String(), processTime)

	dag.delayedBlockManager.Add(block, processTime)

	return dag.processDelayedBlocks()
}

// processDelayedBlocks loops over all delayed blocks and processes blocks which are due.
// This method is invoked after processing a block (ProcessBlock method).
func (dag *BlockDAG) processDelayedBlocks() error {
	// Check if the delayed block with the earliest process time should be processed
	for dag.delayedBlockManager.Len() > 0 {
		earliestDelayedBlockProcessTime := dag.delayedBlockManager.Peek().ProcessTime()
		if earliestDelayedBlockProcessTime.After(dag.Now()) {
			break
		}
		delayedBlock := dag.delayedBlockManager.Pop()
		_, _, err := dag.processBlockNoLock(delayedBlock.Block(), common.BFAfterDelay)
		if err != nil {
			log.Errorf("Error while processing delayed block (block %s)", delayedBlock.Block().Hash().String())
			// Rule errors should not be propagated as they refer only to the delayed block,
			// while this function runs in the context of another block
			if !errors.As(err, &common.RuleError{}) {
				return err
			}
		}
		log.Debugf("Processed delayed block (block %s)", delayedBlock.Block().Hash().String())
	}

	return nil
}

// Config is a descriptor which specifies the blockDAG instance configuration.
type Config struct {
	// DAGParams identifies which DAG parameters the DAG is associated
	// with.
	//
	// This field is required.
	DAGParams *dagconfig.Params

	// TimeSource defines the time source to use for things such as
	// block processing and determining whether or not the DAG is current.
	TimeSource common.TimeSource

	// SigCache defines a signature cache to use when when validating
	// signatures. This is typically most useful when individual
	// transactions are already being validated prior to their inclusion in
	// a block such as what is usually done via a transaction memory pool.
	//
	// This field can be nil if the caller is not interested in using a
	// signature cache.
	SigCache *sigcache.SigCache

	// SubnetworkID identifies which subnetwork the DAG is associated
	// with.
	//
	// This field is required.
	SubnetworkID *subnetworkid.SubnetworkID

	// DatabaseContext is the context in which all database queries related to
	// this DAG are going to run.
	DatabaseContext *dbaccess.DatabaseContext
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

	err = blockvalidation.CheckBlockContext(dag.difficultyManager, dag.pastMedianTimeManager, dag.reachabilityTree, block, parents, flags)
	if err != nil {
		return err
	}

	templateNode, _ := dag.initBlockNode(&header, dag.virtual.Tips())

	_, err = utxovalidation.CheckConnectToPastUTXO(templateNode,
		dag.UTXOSet(), block.Transactions(), false, dag.Params, dag.sigCache, dag.pastMedianTimeManager, dag.sequenceLockCalculator)

	return err
}
