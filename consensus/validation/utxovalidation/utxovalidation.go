package utxovalidation

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/coinbase"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/pastmediantime"
	"github.com/kaspanet/kaspad/consensus/sequencelock"
	"github.com/kaspanet/kaspad/consensus/txscript"
	"github.com/kaspanet/kaspad/consensus/utxo"
	"github.com/kaspanet/kaspad/consensus/validation/scriptvalidation"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/sigcache"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
)

const (
	// the following are used when calculating a transaction's mass

	// MassPerTxByte is the number of grams that any byte
	// adds to a transaction.
	MassPerTxByte = 1

	// MassPerScriptPubKeyByte is the number of grams that any
	// scriptPubKey byte adds to a transaction.
	MassPerScriptPubKeyByte = 10

	// MassPerSigOp is the number of grams that any
	// signature operation adds to a transaction.
	MassPerSigOp = 10000
)

// checkConnectToPastUTXO performs several checks to confirm connecting the passed
// block to the DAG represented by the passed view does not violate any rules.
//
// An example of some of the checks performed are ensuring connecting the block
// would not cause any duplicate transaction hashes for old transactions that
// aren't already fully spent, double spends, exceeding the maximum allowed
// signature operations per block, invalid values in relation to the expected
// block subsidy, or fail transaction script validation.
//
// It also returns the feeAccumulator for this block.
//
// This function MUST be called with the dag state lock held (for writes).
func CheckConnectToPastUTXO(block *blocknode.BlockNode, pastUTXO utxo.UTXOSet,
	transactions []*util.Tx, fastAdd bool, params *dagconfig.Params, sigCache *sigcache.SigCache,
	pastMedianTimeFactory *pastmediantime.PastMedianTimeManager,
	sequenceLockCalculator *sequencelock.SequenceLockCalculator) (coinbase.CompactFeeData, error) {

	if !fastAdd {
		err := ensureNoDuplicateTx(pastUTXO, transactions)
		if err != nil {
			return nil, err
		}

		err = checkDoubleSpendsWithBlockPast(pastUTXO, transactions)
		if err != nil {
			return nil, err
		}

		if err := validateBlockMass(pastUTXO, transactions); err != nil {
			return nil, err
		}
	}

	// Perform several checks on the inputs for each transaction. Also
	// accumulate the total fees. This could technically be combined with
	// the loop above instead of running another loop over the transactions,
	// but by separating it we can avoid running the more expensive (though
	// still relatively cheap as compared to running the scripts) checks
	// against all the inputs when the signature operations are out of
	// bounds.
	// In addition - add all fees into a fee accumulator, to be stored and checked
	// when validating descendants' coinbase transactions.
	var totalFees uint64
	compactFeeFactory := coinbase.NewCompactFeeFactory()

	for _, tx := range transactions {
		txFee, err := CheckTransactionInputsAndCalulateFee(tx, block.BlueScore(), pastUTXO,
			params, fastAdd)
		if err != nil {
			return nil, err
		}

		// Sum the total fees and ensure we don't overflow the
		// accumulator.
		lastTotalFees := totalFees
		totalFees += txFee
		if totalFees < lastTotalFees {
			return nil, common.NewRuleError(common.ErrBadFees, "total fees for block "+
				"overflows accumulator")
		}

		err = compactFeeFactory.Add(txFee)
		if err != nil {
			return nil, errors.Errorf("error adding tx %s fee to compactFeeFactory: %s", tx.ID(), err)
		}
	}
	feeData, err := compactFeeFactory.Data()
	if err != nil {
		return nil, errors.Errorf("error getting bytes of fee data: %s", err)
	}

	if !fastAdd {
		scriptFlags := txscript.ScriptNoFlags

		// We obtain the MTP of the *previous* block (unless it's genesis block)
		// in order to determine if transactions in the current block are final.
		medianTime := block.Header().Timestamp
		if !block.IsGenesis() {
			medianTime = pastMedianTimeFactory.PastMedianTime(block.SelectedParent())
		}

		// We also enforce the relative sequence number based
		// lock-times within the inputs of all transactions in this
		// candidate block.
		for _, tx := range transactions {
			// A transaction can only be included within a block
			// once the sequence locks of *all* its inputs are
			// active.
			sequenceLock, err := sequenceLockCalculator.CalcSequenceLockForBlock(block, pastUTXO, tx)
			if err != nil {
				return nil, err
			}
			if !sequenceLock.IsActive(block.BlueScore(),
				medianTime) {
				str := fmt.Sprintf("block contains " +
					"transaction whose input sequence " +
					"locks are not met")
				return nil, common.NewRuleError(common.ErrUnfinalizedTx, str)
			}
		}

		// Now that the inexpensive checks are done and have passed, verify the
		// transactions are actually allowed to spend the coins by running the
		// expensive SCHNORR signature check scripts. Doing this last helps
		// prevent CPU exhaustion attacks.
		err := scriptvalidation.CheckBlockScripts(block.Hash(), pastUTXO, transactions, scriptFlags, sigCache)
		if err != nil {
			return nil, err
		}
	}
	return feeData, nil
}

// ensureNoDuplicateTx ensures blocks do not contain duplicate transactions which
// 'overwrite' older transactions that are not fully spent. This prevents an
// attack where a coinbase and all of its dependent transactions could be
// duplicated to effectively revert the overwritten transactions to a single
// confirmation thereby making them vulnerable to a double spend.
//
// For more details, see http://r6.ca/blog/20120206T005236Z.html.
//
// This function MUST be called with the dag state lock held (for reads).
func ensureNoDuplicateTx(utxoSet utxo.UTXOSet, transactions []*util.Tx) error {
	// Fetch utxos for all of the transaction ouputs in this block.
	// Typically, there will not be any utxos for any of the outputs.
	fetchSet := make(map[wire.Outpoint]struct{})
	for _, tx := range transactions {
		prevOut := wire.Outpoint{TxID: *tx.ID()}
		for txOutIdx := range tx.MsgTx().TxOut {
			prevOut.Index = uint32(txOutIdx)
			fetchSet[prevOut] = struct{}{}
		}
	}

	// Duplicate transactions are only allowed if the previous transaction
	// is fully spent.
	for outpoint := range fetchSet {
		if _, ok := utxoSet.Get(outpoint); ok {
			str := fmt.Sprintf("tried to overwrite transaction %s "+
				"that is not fully spent", outpoint.TxID)
			return common.NewRuleError(common.ErrOverwriteTx, str)
		}
	}

	return nil
}

// checkDoubleSpendsWithBlockPast checks that each block transaction
// has a corresponding UTXO in the block pastUTXO.
func checkDoubleSpendsWithBlockPast(pastUTXO utxo.UTXOSet, blockTransactions []*util.Tx) error {
	for _, tx := range blockTransactions {
		if tx.IsCoinBase() {
			continue
		}

		for _, txIn := range tx.MsgTx().TxIn {
			if _, ok := pastUTXO.Get(txIn.PreviousOutpoint); !ok {
				return common.NewRuleError(common.ErrMissingTxOut, fmt.Sprintf("missing transaction "+
					"output %s in the utxo set", txIn.PreviousOutpoint))
			}
		}
	}

	return nil
}

func validateBlockMass(pastUTXO utxo.UTXOSet, transactions []*util.Tx) error {
	_, err := CalcBlockMass(pastUTXO, transactions)
	return err
}

// CalcBlockMass sums up and returns the "mass" of a block. See CalcTxMass
// for further details.
func CalcBlockMass(pastUTXO utxo.UTXOSet, transactions []*util.Tx) (uint64, error) {
	totalMass := uint64(0)
	for _, tx := range transactions {
		txMass, err := CalcTxMassFromUTXOSet(tx, pastUTXO)
		if err != nil {
			return 0, err
		}
		totalMass += txMass

		// We could potentially overflow the accumulator so check for
		// overflow as well.
		if totalMass < txMass || totalMass > wire.MaxMassPerBlock {
			str := fmt.Sprintf("block has total mass %d, which is "+
				"above the allowed limit of %d", totalMass, wire.MaxMassPerBlock)
			return 0, common.NewRuleError(common.ErrBlockMassTooHigh, str)
		}
	}
	return totalMass, nil
}

// CalcTxMassFromUTXOSet calculates the transaction mass based on the
// UTXO set in its past.
//
// See CalcTxMass for more details.
func CalcTxMassFromUTXOSet(tx *util.Tx, utxoSet utxo.UTXOSet) (uint64, error) {
	if tx.IsCoinBase() {
		return CalcTxMass(tx, nil), nil
	}
	previousScriptPubKeys := make([][]byte, len(tx.MsgTx().TxIn))
	for txInIndex, txIn := range tx.MsgTx().TxIn {
		entry, ok := utxoSet.Get(txIn.PreviousOutpoint)
		if !ok {
			str := fmt.Sprintf("output %s referenced from "+
				"transaction %s input %d either does not exist or "+
				"has already been spent", txIn.PreviousOutpoint,
				tx.ID(), txInIndex)
			return 0, common.NewRuleError(common.ErrMissingTxOut, str)
		}
		previousScriptPubKeys[txInIndex] = entry.ScriptPubKey()
	}
	return CalcTxMass(tx, previousScriptPubKeys), nil
}

// CalcTxMass sums up and returns the "mass" of a transaction. This number
// is an approximation of how many resources (CPU, RAM, etc.) it would take
// to process the transaction.
// The following properties are considered in the calculation:
// * The transaction length in bytes
// * The length of all output scripts in bytes
// * The count of all input sigOps
func CalcTxMass(tx *util.Tx, previousScriptPubKeys [][]byte) uint64 {
	txSize := tx.MsgTx().SerializeSize()

	if tx.IsCoinBase() {
		return uint64(txSize * MassPerTxByte)
	}

	scriptPubKeySize := 0
	for _, txOut := range tx.MsgTx().TxOut {
		scriptPubKeySize += len(txOut.ScriptPubKey)
	}

	sigOpsCount := 0
	for txInIndex, txIn := range tx.MsgTx().TxIn {
		// Count the precise number of signature operations in the
		// referenced public key script.
		sigScript := txIn.SignatureScript
		isP2SH := txscript.IsPayToScriptHash(previousScriptPubKeys[txInIndex])
		sigOpsCount += txscript.GetPreciseSigOpCount(sigScript, previousScriptPubKeys[txInIndex], isP2SH)
	}

	return uint64(txSize*MassPerTxByte +
		scriptPubKeySize*MassPerScriptPubKeyByte +
		sigOpsCount*MassPerSigOp)
}

// CheckTransactionInputsAndCalulateFee performs a series of checks on the inputs to a
// transaction to ensure they are valid. An example of some of the checks
// include verifying all inputs exist, ensuring the block reward seasoning
// requirements are met, detecting double spends, validating all values and fees
// are in the legal range and the total output amount doesn't exceed the input
// amount. As it checks the inputs, it also calculates the total fees for the
// transaction and returns that value.
//
// NOTE: The transaction MUST have already been sanity checked with the
// CheckTransactionSanity function prior to calling this function.
func CheckTransactionInputsAndCalulateFee(tx *util.Tx, txBlueScore uint64, utxoSet utxo.UTXOSet, dagParams *dagconfig.Params, fastAdd bool) (
	txFeeInSompi uint64, err error) {

	// Coinbase transactions have no standard inputs to validate.
	if tx.IsCoinBase() {
		return 0, nil
	}

	txID := tx.ID()
	var totalSompiIn uint64
	for txInIndex, txIn := range tx.MsgTx().TxIn {
		// Ensure the referenced input transaction is available.
		entry, ok := utxoSet.Get(txIn.PreviousOutpoint)
		if !ok {
			str := fmt.Sprintf("output %s referenced from "+
				"transaction %s input %d either does not exist or "+
				"has already been spent", txIn.PreviousOutpoint,
				tx.ID(), txInIndex)
			return 0, common.NewRuleError(common.ErrMissingTxOut, str)
		}

		if !fastAdd {
			if err = validateCoinbaseMaturity(dagParams, entry, txBlueScore, txIn); err != nil {
				return 0, err
			}
		}

		// Ensure the transaction amounts are in range. Each of the
		// output values of the input transactions must not be negative
		// or more than the max allowed per transaction. All amounts in
		// a transaction are in a unit value known as a sompi. One
		// kaspa is a quantity of sompi as defined by the
		// SompiPerKaspa constant.
		originTxSompi := entry.Amount()
		if originTxSompi > util.MaxSompi {
			str := fmt.Sprintf("transaction output value of %s is "+
				"higher than max allowed value of %d",
				util.Amount(originTxSompi),
				util.MaxSompi)
			return 0, common.NewRuleError(common.ErrBadTxOutValue, str)
		}

		// The total of all outputs must not be more than the max
		// allowed per transaction. Also, we could potentially overflow
		// the accumulator so check for overflow.
		lastSompiIn := totalSompiIn
		totalSompiIn += originTxSompi
		if totalSompiIn < lastSompiIn ||
			totalSompiIn > util.MaxSompi {
			str := fmt.Sprintf("total value of all transaction "+
				"inputs is %d which is higher than max "+
				"allowed value of %d", totalSompiIn,
				util.MaxSompi)
			return 0, common.NewRuleError(common.ErrBadTxOutValue, str)
		}
	}

	// Calculate the total output amount for this transaction. It is safe
	// to ignore overflow and out of range errors here because those error
	// conditions would have already been caught by checkTransactionSanity.
	var totalSompiOut uint64
	for _, txOut := range tx.MsgTx().TxOut {
		totalSompiOut += txOut.Value
	}

	// Ensure the transaction does not spend more than its inputs.
	if totalSompiIn < totalSompiOut {
		str := fmt.Sprintf("total value of all transaction inputs for "+
			"transaction %s is %d which is less than the amount "+
			"spent of %d", txID, totalSompiIn, totalSompiOut)
		return 0, common.NewRuleError(common.ErrSpendTooHigh, str)
	}

	txFeeInSompi = totalSompiIn - totalSompiOut
	return txFeeInSompi, nil
}

func validateCoinbaseMaturity(dagParams *dagconfig.Params, entry *utxo.UTXOEntry, txBlueScore uint64, txIn *wire.TxIn) error {
	// Ensure the transaction is not spending coins which have not
	// yet reached the required coinbase maturity.
	if entry.IsCoinbase() {
		originBlueScore := entry.BlockBlueScore()
		blueScoreSincePrev := txBlueScore - originBlueScore
		if blueScoreSincePrev < dagParams.BlockCoinbaseMaturity {
			str := fmt.Sprintf("tried to spend coinbase "+
				"transaction output %s from blue score %d "+
				"to blue score %d before required maturity "+
				"of %d", txIn.PreviousOutpoint,
				originBlueScore, txBlueScore,
				dagParams.BlockCoinbaseMaturity)
			return common.NewRuleError(common.ErrImmatureSpend, str)
		}
	}
	return nil
}

// ValidateTxMass makes sure that the given transaction's mass does not exceed
// the maximum allowed limit. Currently, it is equivalent to the block mass limit.
// See CalcTxMass for further details.
func ValidateTxMass(tx *util.Tx, utxoSet utxo.UTXOSet) error {
	txMass, err := CalcTxMassFromUTXOSet(tx, utxoSet)
	if err != nil {
		return err
	}
	if txMass > wire.MaxMassPerBlock {
		str := fmt.Sprintf("tx %s has mass %d, which is above the "+
			"allowed limit of %d", tx.ID(), txMass, wire.MaxMassPerBlock)
		return common.NewRuleError(common.ErrTxMassTooHigh, str)
	}
	return nil
}
