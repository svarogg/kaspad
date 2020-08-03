package transaction

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/subnetworks"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/util/subnetworkid"
	"github.com/kaspanet/kaspad/wire"
	"math"
)

const (
	// MaxCoinbasePayloadLen is the maximum length a coinbase payload can be.
	MaxCoinbasePayloadLen = 150
)

// CheckTransactionSanity performs some preliminary checks on a transaction to
// ensure it is sane. These checks are context free.
func CheckTransactionSanity(tx *util.Tx, subnetworkID *subnetworkid.SubnetworkID) error {
	isCoinbase := tx.IsCoinBase()
	// A transaction must have at least one input.
	msgTx := tx.MsgTx()
	if !isCoinbase && len(msgTx.TxIn) == 0 {
		return common.NewRuleError(common.ErrNoTxInputs, "transaction has no inputs")
	}

	// Ensure the transaction amounts are in range. Each transaction
	// output must not be negative or more than the max allowed per
	// transaction. Also, the total of all outputs must abide by the same
	// restrictions. All amounts in a transaction are in a unit value known
	// as a sompi. One kaspa is a quantity of sompi as defined by the
	// SompiPerKaspa constant.
	var totalSompi uint64
	for _, txOut := range msgTx.TxOut {
		sompi := txOut.Value
		if sompi > util.MaxSompi {
			str := fmt.Sprintf("transaction output value of %d is "+
				"higher than max allowed value of %d", sompi,
				util.MaxSompi)
			return common.NewRuleError(common.ErrBadTxOutValue, str)
		}

		// Binary arithmetic guarantees that any overflow is detected and reported.
		// This is impossible for Kaspa, but perhaps possible if an alt increases
		// the total money supply.
		newTotalSompi := totalSompi + sompi
		if newTotalSompi < totalSompi {
			str := fmt.Sprintf("total value of all transaction "+
				"outputs exceeds max allowed value of %d",
				util.MaxSompi)
			return common.NewRuleError(common.ErrBadTxOutValue, str)
		}
		totalSompi = newTotalSompi
		if totalSompi > util.MaxSompi {
			str := fmt.Sprintf("total value of all transaction "+
				"outputs is %d which is higher than max "+
				"allowed value of %d", totalSompi,
				util.MaxSompi)
			return common.NewRuleError(common.ErrBadTxOutValue, str)
		}
	}

	// Check for duplicate transaction inputs.
	existingTxOut := make(map[wire.Outpoint]struct{})
	for _, txIn := range msgTx.TxIn {
		if _, exists := existingTxOut[txIn.PreviousOutpoint]; exists {
			return common.NewRuleError(common.ErrDuplicateTxInputs, "transaction "+
				"contains duplicate inputs")
		}
		existingTxOut[txIn.PreviousOutpoint] = struct{}{}
	}

	// Coinbase payload length must not exceed the max length.
	if isCoinbase {
		payloadLen := len(msgTx.Payload)
		if payloadLen > MaxCoinbasePayloadLen {
			str := fmt.Sprintf("coinbase transaction payload length "+
				"of %d is out of range (max: %d)",
				payloadLen, MaxCoinbasePayloadLen)
			return common.NewRuleError(common.ErrBadCoinbasePayloadLen, str)
		}
	} else {
		// Previous transaction outputs referenced by the inputs to this
		// transaction must not be null.
		for _, txIn := range msgTx.TxIn {
			if isNullOutpoint(&txIn.PreviousOutpoint) {
				return common.NewRuleError(common.ErrBadTxInput, "transaction "+
					"input refers to previous output that "+
					"is null")
			}
		}
	}

	// Check payload's hash
	if !msgTx.SubnetworkID.IsEqual(subnetworkid.SubnetworkIDNative) {
		payloadHash := daghash.DoubleHashH(msgTx.Payload)
		if !msgTx.PayloadHash.IsEqual(&payloadHash) {
			return common.NewRuleError(common.ErrInvalidPayloadHash, "invalid payload hash")
		}
	} else if msgTx.PayloadHash != nil {
		return common.NewRuleError(common.ErrInvalidPayloadHash, "unexpected non-empty payload hash in native subnetwork")
	}

	// Transactions in native, registry and coinbase subnetworks must have Gas = 0
	if (msgTx.SubnetworkID.IsEqual(subnetworkid.SubnetworkIDNative) ||
		msgTx.SubnetworkID.IsBuiltIn()) &&
		msgTx.Gas > 0 {

		return common.NewRuleError(common.ErrInvalidGas, "transaction in the native or "+
			"registry subnetworks has gas > 0 ")
	}

	if msgTx.SubnetworkID.IsEqual(subnetworkid.SubnetworkIDRegistry) {
		err := subnetworks.ValidateSubnetworkRegistryTransaction(msgTx)
		if err != nil {
			return err
		}
	}

	if msgTx.SubnetworkID.IsEqual(subnetworkid.SubnetworkIDNative) &&
		len(msgTx.Payload) > 0 {

		return common.NewRuleError(common.ErrInvalidPayload,
			"transaction in the native subnetwork includes a payload")
	}

	// If we are a partial node, only transactions on built in subnetworks
	// or our own subnetwork may have a payload
	isLocalNodeFull := subnetworkID == nil
	shouldTxBeFull := msgTx.SubnetworkID.IsBuiltIn() ||
		msgTx.SubnetworkID.IsEqual(subnetworkID)
	if !isLocalNodeFull && !shouldTxBeFull && len(msgTx.Payload) > 0 {
		return common.NewRuleError(common.ErrInvalidPayload,
			"transaction that was expected to be partial has a payload "+
				"with length > 0")
	}

	return nil
}

// isNullOutpoint determines whether or not a previous transaction outpoint
// is set.
func isNullOutpoint(outpoint *wire.Outpoint) bool {
	if outpoint.Index == math.MaxUint32 && outpoint.TxID == daghash.ZeroTxID {
		return true
	}
	return false
}
