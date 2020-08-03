package transaction

import (
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/txscript"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/util/subnetworkid"
	"github.com/kaspanet/kaspad/wire"
	"testing"
)

func TestCheckTransactionSanity(t *testing.T) {
	tests := []struct {
		name                   string
		numInputs              uint32
		numOutputs             uint32
		outputValue            uint64
		nodeSubnetworkID       subnetworkid.SubnetworkID
		txSubnetworkData       *txSubnetworkData
		extraModificationsFunc func(*wire.MsgTx)
		expectedErr            error
	}{
		{"good one", 1, 1, 1, *subnetworkid.SubnetworkIDNative, nil, nil, nil},
		{"no inputs", 0, 1, 1, *subnetworkid.SubnetworkIDNative, nil, nil, common.NewRuleError(common.ErrNoTxInputs, "")},
		{"no outputs", 1, 0, 1, *subnetworkid.SubnetworkIDNative, nil, nil, nil},
		{"too much sompi in one output", 1, 1, util.MaxSompi + 1,
			*subnetworkid.SubnetworkIDNative,
			nil,
			nil,
			common.NewRuleError(common.ErrBadTxOutValue, "")},
		{"too much sompi in total outputs", 1, 2, util.MaxSompi - 1,
			*subnetworkid.SubnetworkIDNative,
			nil,
			nil,
			common.NewRuleError(common.ErrBadTxOutValue, "")},
		{"duplicate inputs", 2, 1, 1,
			*subnetworkid.SubnetworkIDNative,
			nil,
			func(tx *wire.MsgTx) { tx.TxIn[1].PreviousOutpoint.Index = 0 },
			common.NewRuleError(common.ErrDuplicateTxInputs, "")},
		{"1 input coinbase",
			1,
			1,
			1,
			*subnetworkid.SubnetworkIDNative,
			&txSubnetworkData{subnetworkid.SubnetworkIDCoinbase, 0, nil},
			nil,
			nil},
		{"no inputs coinbase",
			0,
			1,
			1,
			*subnetworkid.SubnetworkIDNative,
			&txSubnetworkData{subnetworkid.SubnetworkIDCoinbase, 0, nil},
			nil,
			nil},
		{"too long payload coinbase",
			1,
			1,
			1,
			*subnetworkid.SubnetworkIDNative,
			&txSubnetworkData{subnetworkid.SubnetworkIDCoinbase, 0, make([]byte, MaxCoinbasePayloadLen+1)},
			nil,
			common.NewRuleError(common.ErrBadCoinbasePayloadLen, "")},
		{"non-zero gas in Kaspa", 1, 1, 0,
			*subnetworkid.SubnetworkIDNative,
			&txSubnetworkData{subnetworkid.SubnetworkIDNative, 1, []byte{}},
			nil,
			common.NewRuleError(common.ErrInvalidGas, "")},
		{"non-zero gas in subnetwork registry", 1, 1, 0,
			*subnetworkid.SubnetworkIDNative,
			&txSubnetworkData{subnetworkid.SubnetworkIDNative, 1, []byte{}},
			nil,
			common.NewRuleError(common.ErrInvalidGas, "")},
		{"non-zero payload in Kaspa", 1, 1, 0,
			*subnetworkid.SubnetworkIDNative,
			&txSubnetworkData{subnetworkid.SubnetworkIDNative, 0, []byte{1}},
			nil,
			common.NewRuleError(common.ErrInvalidPayload, "")},
		{"payload in subnetwork registry isn't 8 bytes", 1, 1, 0,
			*subnetworkid.SubnetworkIDNative,
			&txSubnetworkData{subnetworkid.SubnetworkIDNative, 0, []byte{1, 2, 3, 4, 5, 6, 7}},
			nil,
			common.NewRuleError(common.ErrInvalidPayload, "")},
		{"payload in other subnetwork isn't 0 bytes", 1, 1, 0,
			subnetworkid.SubnetworkID{123},
			&txSubnetworkData{&subnetworkid.SubnetworkID{234}, 0, []byte{1}},
			nil,
			common.NewRuleError(common.ErrInvalidPayload, "")},
		{"invalid payload hash", 1, 1, 0,
			subnetworkid.SubnetworkID{123},
			&txSubnetworkData{&subnetworkid.SubnetworkID{123}, 0, []byte{1}},
			func(tx *wire.MsgTx) {
				tx.PayloadHash = &daghash.Hash{}
			},
			common.NewRuleError(common.ErrInvalidPayloadHash, "")},
		{"invalid payload hash in native subnetwork", 1, 1, 0,
			*subnetworkid.SubnetworkIDNative,
			nil,
			func(tx *wire.MsgTx) {
				tx.PayloadHash = daghash.DoubleHashP(tx.Payload)
			},
			common.NewRuleError(common.ErrInvalidPayloadHash, "")},
	}

	for _, test := range tests {
		tx := createTxForTest(test.numInputs, test.numOutputs, test.outputValue, test.txSubnetworkData)

		if test.extraModificationsFunc != nil {
			test.extraModificationsFunc(tx)
		}

		err := CheckTransactionSanity(util.NewTx(tx), &test.nodeSubnetworkID)
		if e := common.CheckRuleError(err, test.expectedErr); e != nil {
			t.Errorf("TestCheckTransactionSanity: '%s': %v", test.name, e)
			continue
		}
	}
}

type txSubnetworkData struct {
	subnetworkID *subnetworkid.SubnetworkID
	Gas          uint64
	Payload      []byte
}

func createTxForTest(numInputs uint32, numOutputs uint32, outputValue uint64, subnetworkData *txSubnetworkData) *wire.MsgTx {
	txIns := []*wire.TxIn{}
	txOuts := []*wire.TxOut{}

	for i := uint32(0); i < numInputs; i++ {
		txIns = append(txIns, &wire.TxIn{
			PreviousOutpoint: *wire.NewOutpoint(&daghash.TxID{}, i),
			SignatureScript:  []byte{},
			Sequence:         wire.MaxTxInSequenceNum,
		})
	}

	for i := uint32(0); i < numOutputs; i++ {
		txOuts = append(txOuts, &wire.TxOut{
			ScriptPubKey: OpTrueScript,
			Value:        outputValue,
		})
	}

	if subnetworkData != nil {
		return wire.NewSubnetworkMsgTx(wire.TxVersion, txIns, txOuts, subnetworkData.subnetworkID, subnetworkData.Gas, subnetworkData.Payload)
	}

	return wire.NewNativeMsgTx(wire.TxVersion, txIns, txOuts)
}

// OpTrueScript is script returning TRUE
var OpTrueScript = []byte{txscript.OpTrue}
