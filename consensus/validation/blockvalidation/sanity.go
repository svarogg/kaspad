package blockvalidation

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/merkle"
	"github.com/kaspanet/kaspad/consensus/timesource"
	"github.com/kaspanet/kaspad/consensus/validation/transactionvalidation"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/util/subnetworkid"
	"github.com/kaspanet/kaspad/wire"
	"sort"
	"time"
)

// CheckBlockSanity performs some preliminary checks on a block to ensure it is
// sane before continuing with block processing. These checks are context free.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkBlockHeaderSanity.
func CheckBlockSanity(block *util.Block, params *dagconfig.Params,
	subnetworkID *subnetworkid.SubnetworkID, timeSource timesource.TimeSource,
	flags common.BehaviorFlags) (time.Duration, error) {

	delay, err := checkBlockHeaderSanity(block, params, timeSource, flags)
	if err != nil {
		return 0, err
	}
	err = checkBlockContainsAtLeastOneTransaction(block)
	if err != nil {
		return 0, err
	}
	err = checkBlockContainsLessThanMaxBlockMassTransactions(block)
	if err != nil {
		return 0, err
	}
	err = checkFirstBlockTransactionIsCoinbase(block)
	if err != nil {
		return 0, err
	}
	err = checkBlockContainsOnlyOneCoinbase(block)
	if err != nil {
		return 0, err
	}
	err = checkBlockTransactionOrder(block)
	if err != nil {
		return 0, err
	}
	err = checkNoNonNativeTransactions(block, params)
	if err != nil {
		return 0, err
	}
	err = checkBlockTransactionSanity(block, subnetworkID)
	if err != nil {
		return 0, err
	}
	err = checkBlockHashMerkleRoot(block)
	if err != nil {
		return 0, err
	}

	// The following check will be fairly quick since the transaction IDs
	// are already cached due to building the merkle tree above.
	err = checkBlockDuplicateTransactions(block)
	if err != nil {
		return 0, err
	}

	err = checkBlockDoubleSpends(block)
	if err != nil {
		return 0, err
	}
	return delay, nil
}

// checkBlockHeaderSanity performs some preliminary checks on a block header to
// ensure it is sane before continuing with processing. These checks are
// context free.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkProofOfWork.
func checkBlockHeaderSanity(block *util.Block, params *dagconfig.Params,
	timeSource timesource.TimeSource, flags common.BehaviorFlags) (delay time.Duration, err error) {

	// Ensure the proof of work bits in the block header is in min/max range
	// and the block hash is less than the target value described by the
	// bits.
	header := &block.MsgBlock().Header
	err = checkProofOfWork(header, params, flags)
	if err != nil {
		return 0, err
	}

	if len(header.ParentHashes) == 0 {
		if !header.BlockHash().IsEqual(params.GenesisHash) {
			return 0, common.NewRuleError(common.ErrNoParents, "block has no parents")
		}
	} else {
		err = checkBlockParentsOrder(header)
		if err != nil {
			return 0, err
		}
	}

	// Ensure the block time is not too far in the future. If it's too far, return
	// the duration of time that should be waited before the block becomes valid.
	// This check needs to be last as it does not return an error but rather marks the
	// header as delayed (and valid).
	maxTimestamp := timeSource.Now().Add(time.Duration(params.TimestampDeviationTolerance) * params.TargetTimePerBlock)
	if header.Timestamp.After(maxTimestamp) {
		return header.Timestamp.Sub(maxTimestamp), nil
	}

	return 0, nil
} // checkProofOfWork ensures the block header bits which indicate the target
// difficulty is in min/max range and that the block hash is less than the
// target difficulty as claimed.
//
// The flags modify the behavior of this function as follows:
//  - BFNoPoWCheck: The check to ensure the block hash is less than the target
//    difficulty is not performed.
func checkProofOfWork(header *wire.BlockHeader, params *dagconfig.Params, flags common.BehaviorFlags) error {
	// The target difficulty must be larger than zero.
	target := util.CompactToBig(header.Bits)
	if target.Sign() <= 0 {
		str := fmt.Sprintf("block target difficulty of %064x is too low",
			target)
		return common.NewRuleError(common.ErrUnexpectedDifficulty, str)
	}

	// The target difficulty must be less than the maximum allowed.
	if target.Cmp(params.PowMax) > 0 {
		str := fmt.Sprintf("block target difficulty of %064x is "+
			"higher than max of %064x", target, params.PowMax)
		return common.NewRuleError(common.ErrUnexpectedDifficulty, str)
	}

	// The block hash must be less than the claimed target unless the flag
	// to avoid proof of work checks is set.
	if flags&common.BFNoPoWCheck != common.BFNoPoWCheck {
		// The block hash must be less than the claimed target.
		hash := header.BlockHash()
		hashNum := daghash.HashToBig(hash)
		if hashNum.Cmp(target) > 0 {
			str := fmt.Sprintf("block hash of %064x is higher than "+
				"expected max of %064x", hashNum, target)
			return common.NewRuleError(common.ErrHighHash, str)
		}
	}

	return nil
}

//checkBlockParentsOrder ensures that the block's parents are ordered by hash
func checkBlockParentsOrder(header *wire.BlockHeader) error {
	sortedHashes := make([]*daghash.Hash, header.NumParentBlocks())
	for i, hash := range header.ParentHashes {
		sortedHashes[i] = hash
	}
	sort.Slice(sortedHashes, func(i, j int) bool {
		return daghash.Less(sortedHashes[i], sortedHashes[j])
	})
	if !daghash.AreEqual(header.ParentHashes, sortedHashes) {
		return common.NewRuleError(common.ErrWrongParentsOrder, "block parents are not ordered by hash")
	}
	return nil
}

func checkBlockContainsAtLeastOneTransaction(block *util.Block) error {
	transactions := block.Transactions()
	numTx := len(transactions)
	if numTx == 0 {
		return common.NewRuleError(common.ErrNoTransactions, "block does not contain "+
			"any transactions")
	}
	return nil
}

func checkBlockContainsLessThanMaxBlockMassTransactions(block *util.Block) error {
	// A block must not have more transactions than the max block mass or
	// else it is certainly over the block mass limit.
	transactions := block.Transactions()
	numTx := len(transactions)
	if numTx > wire.MaxMassPerBlock {
		str := fmt.Sprintf("block contains too many transactions - "+
			"got %d, max %d", numTx, wire.MaxMassPerBlock)
		return common.NewRuleError(common.ErrBlockMassTooHigh, str)
	}
	return nil
}

func checkFirstBlockTransactionIsCoinbase(block *util.Block) error {
	transactions := block.Transactions()
	if !transactions[util.CoinbaseTransactionIndex].IsCoinBase() {
		return common.NewRuleError(common.ErrFirstTxNotCoinbase, "first transaction in "+
			"block is not a coinbase")
	}
	return nil
}

func checkBlockContainsOnlyOneCoinbase(block *util.Block) error {
	transactions := block.Transactions()
	for i, tx := range transactions[util.CoinbaseTransactionIndex+1:] {
		if tx.IsCoinBase() {
			str := fmt.Sprintf("block contains second coinbase at "+
				"index %d", i+2)
			return common.NewRuleError(common.ErrMultipleCoinbases, str)
		}
	}
	return nil
}

func checkBlockTransactionOrder(block *util.Block) error {
	transactions := block.Transactions()
	for i, tx := range transactions[util.CoinbaseTransactionIndex+1:] {
		if i != 0 && subnetworkid.Less(&tx.MsgTx().SubnetworkID, &transactions[i].MsgTx().SubnetworkID) {
			return common.NewRuleError(common.ErrTransactionsNotSorted, "transactions must be sorted by subnetwork")
		}
	}
	return nil
}

func checkNoNonNativeTransactions(block *util.Block, params *dagconfig.Params) error {
	// Disallow non-native/coinbase subnetworks in networks that don't allow them
	if !params.EnableNonNativeSubnetworks {
		transactions := block.Transactions()
		for _, tx := range transactions {
			if !(tx.MsgTx().SubnetworkID.IsEqual(subnetworkid.SubnetworkIDNative) ||
				tx.MsgTx().SubnetworkID.IsEqual(subnetworkid.SubnetworkIDCoinbase)) {
				return common.NewRuleError(common.ErrInvalidSubnetwork, "non-native/coinbase subnetworks are not allowed")
			}
		}
	}
	return nil
}

func checkBlockTransactionSanity(block *util.Block, subnetworkID *subnetworkid.SubnetworkID) error {
	transactions := block.Transactions()
	for _, tx := range transactions {
		err := transactionvalidation.CheckTransactionSanity(tx, subnetworkID)
		if err != nil {
			return err
		}
	}
	return nil
}

func checkBlockHashMerkleRoot(block *util.Block) error {
	// Build merkle tree and ensure the calculated merkle root matches the
	// entry in the block header. This also has the effect of caching all
	// of the transaction hashes in the block to speed up future hash
	// checks.
	hashMerkleTree := merkle.BuildHashMerkleTreeStore(block.Transactions())
	calculatedHashMerkleRoot := hashMerkleTree.Root()
	if !block.MsgBlock().Header.HashMerkleRoot.IsEqual(calculatedHashMerkleRoot) {
		str := fmt.Sprintf("block hash merkle root is invalid - block "+
			"header indicates %s, but calculated value is %s",
			block.MsgBlock().Header.HashMerkleRoot, calculatedHashMerkleRoot)
		return common.NewRuleError(common.ErrBadMerkleRoot, str)
	}
	return nil
}

func checkBlockDuplicateTransactions(block *util.Block) error {
	existingTxIDs := make(map[daghash.TxID]struct{})
	transactions := block.Transactions()
	for _, tx := range transactions {
		id := tx.ID()
		if _, exists := existingTxIDs[*id]; exists {
			str := fmt.Sprintf("block contains duplicate "+
				"transaction %s", id)
			return common.NewRuleError(common.ErrDuplicateTx, str)
		}
		existingTxIDs[*id] = struct{}{}
	}
	return nil
}

func checkBlockDoubleSpends(block *util.Block) error {
	usedOutpoints := make(map[wire.Outpoint]*daghash.TxID)
	transactions := block.Transactions()
	for _, tx := range transactions {
		for _, txIn := range tx.MsgTx().TxIn {
			if spendingTxID, exists := usedOutpoints[txIn.PreviousOutpoint]; exists {
				str := fmt.Sprintf("transaction %s spends "+
					"outpoint %s that was already spent by "+
					"transaction %s in this block", tx.ID(), txIn.PreviousOutpoint, spendingTxID)
				return common.NewRuleError(common.ErrDoubleSpendInSameBlock, str)
			}
			usedOutpoints[txIn.PreviousOutpoint] = tx.ID()
		}
	}
	return nil
}
