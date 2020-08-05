package blockdag

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/blocklocator"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/merkle"
	"github.com/kaspanet/kaspad/consensus/notifications"
	"github.com/kaspanet/kaspad/consensus/sequencelock"
	"github.com/kaspanet/kaspad/consensus/subnetworks"
	"github.com/kaspanet/kaspad/consensus/utxo"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/util/mstime"
	"github.com/kaspanet/kaspad/util/subnetworkid"
	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
	"math"
)

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

func (dag *BlockDAG) isInPast(this *blocknode.BlockNode, other *blocknode.BlockNode) (bool, error) {
	return dag.reachabilityTree.IsInPast(this, other)
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

// initBlockNode returns a new block node for the given block header and parents, and the
// anticone of its selected parent (parent with highest blue score).
// selectedParentAnticone is used to update reachability data we store for future reachability queries.
// This function is NOT safe for concurrent access.
func (dag *BlockDAG) initBlockNode(blockHeader *wire.BlockHeader, parents blocknode.BlockNodeSet) (node *blocknode.BlockNode, selectedParentAnticone []*blocknode.BlockNode) {
	return dag.ghostdag.InitBlockNode(blockHeader, parents)
}

func (dag *BlockDAG) Notifier() *notifications.NotificationManager {
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

// AntiPastHashesBetween returns the hashes of the blocks between the
// lowHash's antiPast and highHash's antiPast, or up to the provided
// max number of block hashes.
func (dag *BlockDAG) AntiPastHashesBetween(lowHash, highHash *daghash.Hash, maxHashes uint64) ([]*daghash.Hash, error) {
	return dag.blockLocatorFactory.AntiPastHashesBetween(lowHash, highHash, maxHashes)
}

// antiPastHeadersBetween returns the headers of the blocks between the
// lowHash's antiPast and highHash's antiPast, or up to the provided
// max number of block headers.
func (dag *BlockDAG) AntiPastHeadersBetween(lowHash, highHash *daghash.Hash, maxHeaders uint64) ([]*wire.BlockHeader, error) {
	return dag.blockLocatorFactory.AntiPastHeadersBetween(lowHash, highHash, maxHeaders)
}

// CheckConnectBlockTemplate fully validates that connecting the passed block to
// the DAG does not violate any consensus rules, aside from the proof of
// work requirement.
//
// This function is safe for concurrent access.
func (dag *BlockDAG) CheckConnectBlockTemplate(block *util.Block) error {
	dag.dagLock.RLock()
	defer dag.dagLock.RUnlock()
	return dag.CheckConnectBlockTemplateNoLock(block)
}
