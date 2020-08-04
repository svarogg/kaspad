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
	"github.com/kaspanet/kaspad/consensus/merkle"
	"github.com/kaspanet/kaspad/consensus/multiset"
	"github.com/kaspanet/kaspad/consensus/notifications"
	"github.com/kaspanet/kaspad/consensus/orphanedblocks"
	"github.com/kaspanet/kaspad/consensus/pastmediantime"
	"github.com/kaspanet/kaspad/consensus/reachability"
	"github.com/kaspanet/kaspad/consensus/sequencelock"
	"github.com/kaspanet/kaspad/consensus/subnetworks"
	"github.com/kaspanet/kaspad/consensus/syncrate"
	"github.com/kaspanet/kaspad/consensus/timesource"
	"github.com/kaspanet/kaspad/consensus/utxo"
	"github.com/kaspanet/kaspad/consensus/utxodiffstore"
	"github.com/kaspanet/kaspad/consensus/validation/merklevalidation"
	"github.com/kaspanet/kaspad/consensus/virtualblock"
	"math"
	"sync"
	"time"

	"github.com/kaspanet/kaspad/util/mstime"

	"github.com/kaspanet/kaspad/dbaccess"

	"github.com/pkg/errors"

	"github.com/kaspanet/kaspad/util/subnetworkid"

	"github.com/kaspanet/go-secp256k1"
	"github.com/kaspanet/kaspad/consensus/txscript"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/wire"
)

const (
	// isDAGCurrentMaxDiff is the number of blocks from the network tips (estimated by timestamps) for the current
	// to be considered not synced
	isDAGCurrentMaxDiff = 40_000
)

// BlockDAG provides functions for working with the kaspa block DAG.
// It includes functionality such as rejecting duplicate blocks, ensuring blocks
// follow all rules, and orphan handling.
type BlockDAG struct {
	// The following fields are set when the instance is created and can't
	// be changed afterwards, so there is no need to protect them with a
	// separate mutex.
	Params                 *dagconfig.Params
	databaseContext        *dbaccess.DatabaseContext
	timeSource             timesource.TimeSource
	sigCache               *txscript.SigCache
	indexManager           IndexManager
	genesis                *blocknode.BlockNode
	notifier               *notifications.ConsensusNotifier
	coinbase               *coinbase.Coinbase
	ghostdag               *ghostdag.GHOSTDAG
	blockLocatorFactory    *blocklocator.BlockLocatorFactory
	difficulty             *difficulty.Difficulty
	pastMedianTimeFactory  *pastmediantime.PastMedianTimeFactory
	syncRate               *syncrate.SyncRate
	sequenceLockCalculator *sequencelock.SequenceLockCalculator
	finalityManager        *finality.FinalityManager

	// dagLock protects concurrent access to the vast majority of the
	// fields in this struct below this point.
	dagLock sync.RWMutex

	utxoLock sync.RWMutex

	// index and virtual are related to the memory block index. They both
	// have their own locks, however they are often also protected by the
	// DAG lock to help prevent logic races when blocks are being processed.

	// index houses the entire block index in memory. The block index is
	// a tree-shaped structure.
	blockNodeStore *blocknode.BlockNodeStore

	// blockCount holds the number of blocks in the DAG
	blockCount uint64

	// virtual tracks the current tips.
	virtual *virtualblock.VirtualBlock

	// subnetworkID holds the subnetwork ID of the DAG
	subnetworkID *subnetworkid.SubnetworkID

	orphanedBlocks *orphanedblocks.OrphanedBlocks
	delayedBlocks  *delayedblocks.DelayedBlocks

	// The following fields are used to determine if certain warnings have
	// already been shown.
	//
	// unknownRulesWarned refers to warnings due to unknown rules being
	// activated.
	//
	// unknownVersionsWarned refers to warnings due to unknown versions
	// being mined.
	unknownRulesWarned    bool
	unknownVersionsWarned bool

	utxoDiffStore   *utxodiffstore.UTXODiffStore
	multisetManager *multiset.MultiSetManager

	reachabilityTree *reachability.ReachabilityTree
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

	blockNodeStore := blocknode.NewBlockNodeStore(params)
	dag := &BlockDAG{
		Params:                params,
		databaseContext:       config.DatabaseContext,
		timeSource:            config.TimeSource,
		sigCache:              config.SigCache,
		indexManager:          config.IndexManager,
		blockNodeStore:        blockNodeStore,
		delayedBlocks:         delayedblocks.New(),
		blockCount:            0,
		subnetworkID:          config.SubnetworkID,
		notifier:              notifications.New(),
		coinbase:              coinbase.New(config.DatabaseContext, params),
		pastMedianTimeFactory: pastmediantime.NewPastMedianTimeFactory(params),
		syncRate:              syncrate.NewSyncRate(params),
	}

	dag.multisetManager = multiset.NewMultiSetManager()
	dag.reachabilityTree = reachability.NewReachabilityTree(blockNodeStore, params)
	dag.ghostdag = ghostdag.NewGHOSTDAG(dag.reachabilityTree, params, dag.timeSource)
	dag.virtual = virtualblock.NewVirtualBlock(dag.ghostdag, params, dag.blockNodeStore, nil)
	dag.blockLocatorFactory = blocklocator.NewBlockLocatorFactory(dag.blockNodeStore, params)
	dag.utxoDiffStore = utxodiffstore.NewUTXODiffStore(dag.databaseContext, blockNodeStore, dag.virtual)
	dag.difficulty = difficulty.NewDifficulty(params, dag.virtual)
	dag.sequenceLockCalculator = sequencelock.NewSequenceLockCalculator(dag.virtual, dag.pastMedianTimeFactory)
	dag.orphanedBlocks = orphanedblocks.New(blockNodeStore)
	dag.finalityManager = finality.New(params, blockNodeStore, dag.virtual, dag.reachabilityTree, dag.utxoDiffStore, config.DatabaseContext)

	// Initialize the DAG state from the passed database. When the db
	// does not yet contain any DAG state, both it and the DAG state
	// will be initialized to contain only the genesis block.
	err := dag.initDAGState()
	if err != nil {
		return nil, err
	}

	// Initialize and catch up all of the currently active optional indexes
	// as needed.
	if config.IndexManager != nil {
		err = config.IndexManager.Init(dag, dag.databaseContext)
		if err != nil {
			return nil, err
		}
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

// IsKnownBlock returns whether or not the DAG instance has the block represented
// by the passed hash. This includes checking the various places a block can
// be in, like part of the DAG or the orphan pool.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) IsKnownBlock(hash *daghash.Hash) bool {
	return dag.IsInDAG(hash) || dag.orphanedBlocks.IsKnownOrphan(hash) || dag.delayedBlocks.IsKnownDelayed(hash) || dag.IsKnownInvalid(hash)
}

// AreKnownBlocks returns whether or not the DAG instances has all blocks represented
// by the passed hashes. This includes checking the various places a block can
// be in, like part of the DAG or the orphan pool.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) AreKnownBlocks(hashes []*daghash.Hash) bool {
	for _, hash := range hashes {
		haveBlock := dag.IsKnownBlock(hash)
		if !haveBlock {
			return false
		}
	}

	return true
}

// IsKnownInvalid returns whether the passed hash is known to be an invalid block.
// Note that if the block is not found this method will return false.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) IsKnownInvalid(hash *daghash.Hash) bool {
	node, ok := dag.blockNodeStore.LookupNode(hash)
	if !ok {
		return false
	}
	return dag.blockNodeStore.NodeStatus(node).KnownInvalid()
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

	if err := dag.validateGasLimit(block); err != nil {
		return nil, err
	}

	newBlockPastUTXO, txsAcceptanceData, newBlockFeeData, newBlockMultiSet, err :=
		dag.verifyAndBuildUTXO(node, block.Transactions(), fastAdd)
	if err != nil {
		return nil, errors.Wrapf(err, "error verifying UTXO for %s", node)
	}

	err = dag.coinbase.ValidateCoinbaseTransaction(node, block, txsAcceptanceData)
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

	err = dag.multisetManager.FlushToDB(dbTx)
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

	// Allow the index manager to call each of the currently active
	// optional indexes with the block being connected so they can
	// update themselves accordingly.
	if dag.indexManager != nil {
		err := dag.indexManager.ConnectBlock(dbTx, block.Hash(), txsAcceptanceData)
		if err != nil {
			return err
		}
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
	dag.multisetManager.ClearNewEntries()

	return nil
}

func (dag *BlockDAG) validateGasLimit(block *util.Block) error {
	var currentSubnetworkID *subnetworkid.SubnetworkID
	var currentSubnetworkGasLimit uint64
	var currentGasUsage uint64
	var err error

	// We assume here that transactions are ordered by subnetworkID,
	// since it was already validated in checkTransactionSanity
	for _, tx := range block.Transactions() {
		msgTx := tx.MsgTx()

		// In native and Built-In subnetworks all txs must have Gas = 0, and that was already validated in checkTransactionSanity
		// Therefore - no need to check them here.
		if msgTx.SubnetworkID.IsEqual(subnetworkid.SubnetworkIDNative) || msgTx.SubnetworkID.IsBuiltIn() {
			continue
		}

		if !msgTx.SubnetworkID.IsEqual(currentSubnetworkID) {
			currentSubnetworkID = &msgTx.SubnetworkID
			currentGasUsage = 0
			currentSubnetworkGasLimit, err = subnetworks.GasLimit(dag.databaseContext, currentSubnetworkID)
			if err != nil {
				return errors.Errorf("Error getting gas limit for subnetworkID '%s': %s", currentSubnetworkID, err)
			}
		}

		newGasUsage := currentGasUsage + msgTx.Gas
		if newGasUsage < currentGasUsage { // check for overflow
			str := fmt.Sprintf("Block gas usage in subnetwork with ID %s has overflown", currentSubnetworkID)
			return common.NewRuleError(common.ErrInvalidGas, str)
		}
		if newGasUsage > currentSubnetworkGasLimit {
			str := fmt.Sprintf("Block wastes too much gas in subnetwork with ID %s", currentSubnetworkID)
			return common.NewRuleError(common.ErrInvalidGas, str)
		}

		currentGasUsage = newGasUsage
	}

	return nil
}

// NextBlockCoinbaseTransaction prepares the coinbase transaction for the next mined block
//
// This function CAN'T be called with the DAG lock held.
func (dag *BlockDAG) NextBlockCoinbaseTransaction(scriptPubKey []byte, extraData []byte) (*util.Tx, error) {
	dag.dagLock.RLock()
	defer dag.dagLock.RUnlock()

	return dag.NextBlockCoinbaseTransactionNoLock(scriptPubKey, extraData)
}

// NextBlockCoinbaseTransactionNoLock prepares the coinbase transaction for the next mined block
//
// This function MUST be called with the DAG read-lock held
func (dag *BlockDAG) NextBlockCoinbaseTransactionNoLock(scriptPubKey []byte, extraData []byte) (*util.Tx, error) {
	txsAcceptanceData, err := dag.TxsAcceptedByVirtual()
	if err != nil {
		return nil, err
	}
	return dag.coinbase.ExpectedCoinbaseTransaction(&dag.virtual.BlockNode, txsAcceptanceData, scriptPubKey, extraData)
}

// NextAcceptedIDMerkleRootNoLock prepares the acceptedIDMerkleRoot for the next mined block
//
// This function MUST be called with the DAG read-lock held
func (dag *BlockDAG) NextAcceptedIDMerkleRootNoLock() (*daghash.Hash, error) {
	txsAcceptanceData, err := dag.TxsAcceptedByVirtual()
	if err != nil {
		return nil, err
	}

	return merkle.CalculateAcceptedIDMerkleRoot(txsAcceptanceData), nil
}

// TxsAcceptedByVirtual retrieves transactions accepted by the current virtual block
//
// This function MUST be called with the DAG read-lock held
func (dag *BlockDAG) TxsAcceptedByVirtual() (common.MultiBlockTxsAcceptanceData, error) {
	_, _, txsAcceptanceData, err := dag.pastUTXO(&dag.virtual.BlockNode)
	return txsAcceptanceData, err
}

// TxsAcceptedByBlockHash retrieves transactions accepted by the given block
//
// This function MUST be called with the DAG read-lock held
func (dag *BlockDAG) TxsAcceptedByBlockHash(blockHash *daghash.Hash) (common.MultiBlockTxsAcceptanceData, error) {
	node, ok := dag.blockNodeStore.LookupNode(blockHash)
	if !ok {
		return nil, errors.Errorf("Couldn't find block %s", blockHash)
	}
	_, _, txsAcceptanceData, err := dag.pastUTXO(node)
	return txsAcceptanceData, err
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

	dag.multisetManager.SetMultiset(node.Hash(), newBlockMultiset)

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
	dag.utxoLock.Lock()
	defer dag.utxoLock.Unlock()
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

	feeData, err := dag.checkConnectToPastUTXO(node, pastUTXO, transactions, fastAdd)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	multiset, err = dag.multisetManager.CalcMultiset(node, txsAcceptanceData, selectedParentPastUTXO)
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

// Now returns the adjusted time according to
// dag.timeSource. See TimeSource.Now for
// more details.
func (dag *BlockDAG) Now() mstime.Time {
	return dag.timeSource.Now()
}

// selectedTip returns the current selected tip for the DAG.
// It will return nil if there is no tip.
func (dag *BlockDAG) selectedTip() *blocknode.BlockNode {
	return dag.virtual.SelectedParent()
}

// SelectedTipHeader returns the header of the current selected tip for the DAG.
// It will return nil if there is no tip.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) SelectedTipHeader() *wire.BlockHeader {
	selectedTip := dag.selectedTip()
	if selectedTip == nil {
		return nil
	}

	return selectedTip.Header()
}

// SelectedTipHash returns the hash of the current selected tip for the DAG.
// It will return nil if there is no tip.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) SelectedTipHash() *daghash.Hash {
	selectedTip := dag.selectedTip()
	if selectedTip == nil {
		return nil
	}

	return selectedTip.Hash()
}

// UTXOSet returns the DAG's UTXO set
func (dag *BlockDAG) UTXOSet() *utxo.FullUTXOSet {
	return dag.virtual.UTXOSet()
}

// CalcPastMedianTime returns the past median time of the DAG.
func (dag *BlockDAG) CalcPastMedianTime() mstime.Time {
	return dag.PastMedianTime(dag.virtual.Tips().Bluest())
}

// GetUTXOEntry returns the requested unspent transaction output. The returned
// instance must be treated as immutable since it is shared by all callers.
//
// This function is safe for concurrent access. However, the returned entry (if
// any) is NOT.
func (dag *BlockDAG) GetUTXOEntry(outpoint wire.Outpoint) (*utxo.UTXOEntry, bool) {
	return dag.virtual.UTXOSet().Get(outpoint)
}

// BlueScoreByBlockHash returns the blue score of a block with the given hash.
func (dag *BlockDAG) BlueScoreByBlockHash(hash *daghash.Hash) (uint64, error) {
	node, ok := dag.blockNodeStore.LookupNode(hash)
	if !ok {
		return 0, errors.Errorf("block %s is unknown", hash)
	}

	return node.BlueScore(), nil
}

// BluesByBlockHash returns the blues of the block for the given hash.
func (dag *BlockDAG) BluesByBlockHash(hash *daghash.Hash) ([]*daghash.Hash, error) {
	node, ok := dag.blockNodeStore.LookupNode(hash)
	if !ok {
		return nil, errors.Errorf("block %s is unknown", hash)
	}

	hashes := make([]*daghash.Hash, len(node.Blues()))
	for i, blue := range node.Blues() {
		hashes[i] = blue.Hash()
	}

	return hashes, nil
}

// BlockConfirmationsByHash returns the confirmations number for a block with the
// given hash. See blockConfirmations for further details.
//
// This function is safe for concurrent access
func (dag *BlockDAG) BlockConfirmationsByHash(hash *daghash.Hash) (uint64, error) {
	dag.dagLock.RLock()
	defer dag.dagLock.RUnlock()

	return dag.BlockConfirmationsByHashNoLock(hash)
}

// BlockConfirmationsByHashNoLock is lock free version of BlockConfirmationsByHash
//
// This function is unsafe for concurrent access.
func (dag *BlockDAG) BlockConfirmationsByHashNoLock(hash *daghash.Hash) (uint64, error) {
	if hash.IsEqual(&daghash.ZeroHash) {
		return 0, nil
	}

	node, ok := dag.blockNodeStore.LookupNode(hash)
	if !ok {
		return 0, errors.Errorf("block %s is unknown", hash)
	}

	return dag.blockConfirmations(node)
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

// SelectedTipBlueScore returns the blue score of the selected tip. Returns zero
// if we hadn't accepted the genesis block yet.
func (dag *BlockDAG) SelectedTipBlueScore() uint64 {
	selectedTip := dag.selectedTip()
	if selectedTip == nil {
		return 0
	}
	return selectedTip.BlueScore()
}

// VirtualBlueScore returns the blue score of the current virtual block
func (dag *BlockDAG) VirtualBlueScore() uint64 {
	return dag.virtual.BlueScore()
}

// BlockCount returns the number of blocks in the DAG
func (dag *BlockDAG) BlockCount() uint64 {
	return dag.blockCount
}

// TipHashes returns the hashes of the DAG's tips
func (dag *BlockDAG) TipHashes() []*daghash.Hash {
	return dag.virtual.Tips().Hashes()
}

// CurrentBits returns the bits of the tip with the lowest bits, which also means it has highest difficulty.
func (dag *BlockDAG) CurrentBits() uint32 {
	tips := dag.virtual.Tips()
	minBits := uint32(math.MaxUint32)
	for tip := range tips {
		if minBits > tip.Header().Bits {
			minBits = tip.Header().Bits
		}
	}
	return minBits
}

// HeaderByHash returns the block header identified by the given hash or an
// error if it doesn't exist.
func (dag *BlockDAG) HeaderByHash(hash *daghash.Hash) (*wire.BlockHeader, error) {
	node, ok := dag.blockNodeStore.LookupNode(hash)
	if !ok {
		err := errors.Errorf("block %s is not known", hash)
		return &wire.BlockHeader{}, err
	}

	return node.Header(), nil
}

// ChildHashesByHash returns the child hashes of the block with the given hash in the
// DAG.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) ChildHashesByHash(hash *daghash.Hash) ([]*daghash.Hash, error) {
	node, ok := dag.blockNodeStore.LookupNode(hash)
	if !ok {
		str := fmt.Sprintf("block %s is not in the DAG", hash)
		return nil, common.ErrNotInDAG(str)

	}

	return node.Children().Hashes(), nil
}

// SelectedParentHash returns the selected parent hash of the block with the given hash in the
// DAG.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) SelectedParentHash(blockHash *daghash.Hash) (*daghash.Hash, error) {
	node, ok := dag.blockNodeStore.LookupNode(blockHash)
	if !ok {
		str := fmt.Sprintf("block %s is not in the DAG", blockHash)
		return nil, common.ErrNotInDAG(str)

	}

	if node.SelectedParent() == nil {
		return nil, nil
	}
	return node.SelectedParent().Hash(), nil
}

// antiPastHashesBetween returns the hashes of the blocks between the
// lowHash's antiPast and highHash's antiPast, or up to the provided
// max number of block hashes.
//
// This function MUST be called with the DAG state lock held (for reads).
func (dag *BlockDAG) antiPastHashesBetween(lowHash, highHash *daghash.Hash, maxHashes uint64) ([]*daghash.Hash, error) {
	nodes, err := dag.antiPastBetween(lowHash, highHash, maxHashes)
	if err != nil {
		return nil, err
	}
	hashes := make([]*daghash.Hash, len(nodes))
	for i, node := range nodes {
		hashes[i] = node.Hash()
	}
	return hashes, nil
}

// antiPastBetween returns the blockNodes between the lowHash's antiPast
// and highHash's antiPast, or up to the provided max number of blocks.
//
// This function MUST be called with the DAG state lock held (for reads).
func (dag *BlockDAG) antiPastBetween(lowHash, highHash *daghash.Hash, maxEntries uint64) ([]*blocknode.BlockNode, error) {
	lowNode, ok := dag.blockNodeStore.LookupNode(lowHash)
	if !ok {
		return nil, errors.Errorf("Couldn't find low hash %s", lowHash)
	}
	highNode, ok := dag.blockNodeStore.LookupNode(highHash)
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
		isCurrentAncestorOfLowNode, err := dag.isInPast(current, lowNode)
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

func (dag *BlockDAG) isInPast(this *blocknode.BlockNode, other *blocknode.BlockNode) (bool, error) {
	return dag.reachabilityTree.IsInPast(this, other)
}

// AntiPastHashesBetween returns the hashes of the blocks between the
// lowHash's antiPast and highHash's antiPast, or up to the provided
// max number of block hashes.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) AntiPastHashesBetween(lowHash, highHash *daghash.Hash, maxHashes uint64) ([]*daghash.Hash, error) {
	dag.dagLock.RLock()
	defer dag.dagLock.RUnlock()
	hashes, err := dag.antiPastHashesBetween(lowHash, highHash, maxHashes)
	if err != nil {
		return nil, err
	}
	return hashes, nil
}

// antiPastHeadersBetween returns the headers of the blocks between the
// lowHash's antiPast and highHash's antiPast, or up to the provided
// max number of block headers.
//
// This function MUST be called with the DAG state lock held (for reads).
func (dag *BlockDAG) antiPastHeadersBetween(lowHash, highHash *daghash.Hash, maxHeaders uint64) ([]*wire.BlockHeader, error) {
	nodes, err := dag.antiPastBetween(lowHash, highHash, maxHeaders)
	if err != nil {
		return nil, err
	}
	headers := make([]*wire.BlockHeader, len(nodes))
	for i, node := range nodes {
		headers[i] = node.Header()
	}
	return headers, nil
}

// GetTopHeaders returns the top wire.MaxBlockHeadersPerMsg block headers ordered by blue score.
func (dag *BlockDAG) GetTopHeaders(highHash *daghash.Hash, maxHeaders uint64) ([]*wire.BlockHeader, error) {
	highNode := &dag.virtual.BlockNode
	if highHash != nil {
		var ok bool
		highNode, ok = dag.blockNodeStore.LookupNode(highHash)
		if !ok {
			return nil, errors.Errorf("Couldn't find the high hash %s in the dag", highHash)
		}
	}
	headers := make([]*wire.BlockHeader, 0, highNode.BlueScore())
	queue := blocknode.NewDownHeap()
	queue.PushSet(highNode.Parents())

	visited := blocknode.NewBlockNodeSet()
	for i := uint32(0); queue.Len() > 0 && uint64(len(headers)) < maxHeaders; i++ {
		current := queue.Pop()
		if !visited.Contains(current) {
			visited.Add(current)
			headers = append(headers, current.Header())
			queue.PushSet(current.Parents())
		}
	}
	return headers, nil
}

// Lock locks the DAG's UTXO set for writing.
func (dag *BlockDAG) Lock() {
	dag.dagLock.Lock()
}

// Unlock unlocks the DAG's UTXO set for writing.
func (dag *BlockDAG) Unlock() {
	dag.dagLock.Unlock()
}

// RLock locks the DAG's UTXO set for reading.
func (dag *BlockDAG) RLock() {
	dag.dagLock.RLock()
}

// RUnlock unlocks the DAG's UTXO set for reading.
func (dag *BlockDAG) RUnlock() {
	dag.dagLock.RUnlock()
}

// AntiPastHeadersBetween returns the headers of the blocks between the
// lowHash's antiPast and highHash's antiPast, or up to
// wire.MaxBlockHeadersPerMsg block headers.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) AntiPastHeadersBetween(lowHash, highHash *daghash.Hash, maxHeaders uint64) ([]*wire.BlockHeader, error) {
	dag.dagLock.RLock()
	defer dag.dagLock.RUnlock()
	headers, err := dag.antiPastHeadersBetween(lowHash, highHash, maxHeaders)
	if err != nil {
		return nil, err
	}
	return headers, nil
}

// SubnetworkID returns the node's subnetwork ID
func (dag *BlockDAG) SubnetworkID() *subnetworkid.SubnetworkID {
	return dag.subnetworkID
}

// ForEachHash runs the given fn on every hash that's currently known to
// the DAG.
//
// This function is NOT safe for concurrent access. It is meant to be
// used either on initialization or when the dag lock is held for reads.
func (dag *BlockDAG) ForEachHash(fn func(hash daghash.Hash) error) error {
	return dag.blockNodeStore.ForEachHash(fn)
}

func (dag *BlockDAG) addDelayedBlock(block *util.Block, delay time.Duration) error {
	processTime := dag.Now().Add(delay)
	log.Debugf("Adding block to delayed blocks queue (block hash: %s, process time: %s)", block.Hash().String(), processTime)

	dag.delayedBlocks.Add(block, processTime)

	return dag.processDelayedBlocks()
}

// processDelayedBlocks loops over all delayed blocks and processes blocks which are due.
// This method is invoked after processing a block (ProcessBlock method).
func (dag *BlockDAG) processDelayedBlocks() error {
	// Check if the delayed block with the earliest process time should be processed
	for dag.delayedBlocks.Len() > 0 {
		earliestDelayedBlockProcessTime := dag.delayedBlocks.Peek().ProcessTime()
		if earliestDelayedBlockProcessTime.After(dag.Now()) {
			break
		}
		delayedBlock := dag.delayedBlocks.Pop()
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

// IndexManager provides a generic interface that is called when blocks are
// connected to the DAG for the purpose of supporting optional indexes.
type IndexManager interface {
	// Init is invoked during DAG initialize in order to allow the index
	// manager to initialize itself and any indexes it is managing.
	Init(*BlockDAG, *dbaccess.DatabaseContext) error

	// ConnectBlock is invoked when a new block has been connected to the
	// DAG.
	ConnectBlock(dbContext *dbaccess.TxContext, blockHash *daghash.Hash, acceptedTxsData common.MultiBlockTxsAcceptanceData) error
}

// Config is a descriptor which specifies the blockDAG instance configuration.
type Config struct {
	// Interrupt specifies a channel the caller can close to signal that
	// long running operations, such as catching up indexes or performing
	// database migrations, should be interrupted.
	//
	// This field can be nil if the caller does not desire the behavior.
	Interrupt <-chan struct{}

	// DAGParams identifies which DAG parameters the DAG is associated
	// with.
	//
	// This field is required.
	DAGParams *dagconfig.Params

	// TimeSource defines the time source to use for things such as
	// block processing and determining whether or not the DAG is current.
	TimeSource timesource.TimeSource

	// SigCache defines a signature cache to use when when validating
	// signatures. This is typically most useful when individual
	// transactions are already being validated prior to their inclusion in
	// a block such as what is usually done via a transaction memory pool.
	//
	// This field can be nil if the caller is not interested in using a
	// signature cache.
	SigCache *txscript.SigCache

	// IndexManager defines an index manager to use when initializing the
	// DAG and connecting blocks.
	//
	// This field can be nil if the caller does not wish to make use of an
	// index manager.
	IndexManager IndexManager

	// SubnetworkID identifies which subnetwork the DAG is associated
	// with.
	//
	// This field is required.
	SubnetworkID *subnetworkid.SubnetworkID

	// DatabaseContext is the context in which all database queries related to
	// this DAG are going to run.
	DatabaseContext *dbaccess.DatabaseContext
}

// initBlockNode returns a new block node for the given block header and parents, and the
// anticone of its selected parent (parent with highest blue score).
// selectedParentAnticone is used to update reachability data we store for future reachability queries.
// This function is NOT safe for concurrent access.
func (dag *BlockDAG) initBlockNode(blockHeader *wire.BlockHeader, parents blocknode.BlockNodeSet) (node *blocknode.BlockNode, selectedParentAnticone []*blocknode.BlockNode) {
	return dag.ghostdag.InitBlockNode(blockHeader, parents)
}

func (dag *BlockDAG) Notifier() *notifications.ConsensusNotifier {
	return dag.notifier
}

// PastMedianTime returns the median time of the previous few blocks
// prior to, and including, the block node.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) PastMedianTime(node *blocknode.BlockNode) mstime.Time {
	return dag.pastMedianTimeFactory.PastMedianTime(node)
}

// GasLimit returns the gas limit of a registered subnetwork. If the subnetwork does not
// exist this method returns an error.
func (dag *BlockDAG) GasLimit(subnetworkID *subnetworkid.SubnetworkID) (uint64, error) {
	return subnetworks.GasLimit(dag.databaseContext, subnetworkID)
}

// IsInSelectedParentChain returns whether or not a block hash is found in the selected
// parent chain. Note that this method returns an error if the given blockHash does not
// exist within the block node store.
func (dag *BlockDAG) IsInSelectedParentChain(blockHash *daghash.Hash) (bool, error) {
	return dag.virtual.IsInSelectedParentChain(blockHash)
}

// SelectedParentChain returns the selected parent chain starting from blockHash (exclusive)
// up to the virtual (exclusive). If blockHash is nil then the genesis block is used. If
// blockHash is not within the select parent chain, go down its own selected parent chain,
// while collecting each block hash in removedChainHashes, until reaching a block within
// the main selected parent chain.
func (dag *BlockDAG) SelectedParentChain(blockHash *daghash.Hash) ([]*daghash.Hash, []*daghash.Hash, error) {
	return dag.virtual.SelectedParentChain(blockHash)
}

// BlockLocatorFromHashes returns a block locator from high and low hash.
// See BlockLocator for details on the algorithm used to create a block locator.
func (dag *BlockDAG) BlockLocatorFromHashes(highHash, lowHash *daghash.Hash) (blocklocator.BlockLocator, error) {
	return dag.blockLocatorFactory.BlockLocatorFromHashes(highHash, lowHash)
}

// FindNextLocatorBoundaries returns the lowest unknown block locator, hash
// and the highest known block locator hash. This is used to create the
// next block locator to find the highest shared known chain block with the
// sync peer.
func (dag *BlockDAG) FindNextLocatorBoundaries(locator blocklocator.BlockLocator) (highHash, lowHash *daghash.Hash) {
	return dag.blockLocatorFactory.FindNextLocatorBoundaries(locator)
}

// NextRequiredDifficulty calculates the required difficulty for a block that will
// be built on top of the current tips.
func (dag *BlockDAG) NextRequiredDifficulty() uint32 {
	return dag.difficulty.NextRequiredDifficulty()
}

// IsSyncRateBelowThreshold checks whether the sync rate
// is below the expected threshold.
func (dag *BlockDAG) IsSyncRateBelowThreshold(maxDeviation float64) bool {
	return dag.syncRate.IsSyncRateBelowThreshold(maxDeviation)
}

// CalcSequenceLock computes a relative lock-time SequenceLock for the passed
// transaction using the passed UTXOSet to obtain the past median time
// for blocks in which the referenced inputs of the transactions were included
// within. The generated SequenceLock lock can be used in conjunction with a
// block height, and adjusted median block time to determine if all the inputs
// referenced within a transaction have reached sufficient maturity allowing
// the candidate transaction to be included in a block.
func (dag *BlockDAG) CalcSequenceLock(tx *util.Tx, utxoSet utxo.UTXOSet) (*sequencelock.SequenceLock, error) {
	return dag.sequenceLockCalculator.CalcSequenceLock(tx, utxoSet)
}

// IsKnownOrphan returns whether the passed hash is currently a known orphan.
// Keep in mind that only a limited number of orphans are held onto for a
// limited amount of time, so this function must not be used as an absolute
// way to test if a block is an orphan block. A full block (as opposed to just
// its hash) must be passed to ProcessBlock for that purpose. However, calling
// ProcessBlock with an orphan that already exists results in an error, so this
// function provides a mechanism for a caller to intelligently detect *recent*
// duplicate orphans and react accordingly.
func (dag *BlockDAG) IsKnownOrphan(hash *daghash.Hash) bool {
	return dag.orphanedBlocks.IsKnownOrphan(hash)
}

// GetOrphanMissingAncestorHashes returns all of the missing parents in the orphan's sub-DAG
func (dag *BlockDAG) GetOrphanMissingAncestorHashes(orphanHash *daghash.Hash) []*daghash.Hash {
	return dag.orphanedBlocks.GetOrphanMissingAncestorHashes(orphanHash)
}

// IsKnownFinalizedBlock returns whether the block is below the finality point.
// IsKnownFinalizedBlock might be false-negative because node finality status is
// updated in a separate goroutine. To get a definite answer if a block
// is finalized or not, use dag.CheckFinalityViolation.
func (dag *BlockDAG) IsKnownFinalizedBlock(blockHash *daghash.Hash) bool {
	return dag.finalityManager.IsKnownFinalizedBlock(blockHash)
}

// FinalityInterval is the interval that determines the finality window of the DAG.
func (dag *BlockDAG) FinalityInterval() uint64 {
	return dag.finalityManager.FinalityInterval()
}

// LastFinalityPointHash returns the hash of the last finality point
func (dag *BlockDAG) LastFinalityPointHash() *daghash.Hash {
	return dag.finalityManager.LastFinalityPointHash()
}
