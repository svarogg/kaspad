package coinbase

import (
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/dbaccess"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/coinbasepayload"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/util/subnetworkid"
	"github.com/kaspanet/kaspad/util/txsort"
	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
)

var (
	// baseSubsidy is the starting subsidy amount for mined blocks. This
	// value is halved every SubsidyHalvingInterval blocks.
	baseSubsidy uint64 = 50 * util.SompiPerKaspa
)

type Coinbase struct {
	dbContext *dbaccess.DatabaseContext
	params    *dagconfig.Params
}

func New(dbContext *dbaccess.DatabaseContext, params *dagconfig.Params) *Coinbase {
	return &Coinbase{
		dbContext: dbContext,
		params:    params,
	}
}

// getBluesFeeData returns the CompactFeeData for all nodes' blues,
// used to calculate the fees this BlockNode needs to pay
func (cb *Coinbase) getBluesFeeData(node *blocknode.BlockNode) (map[daghash.Hash]CompactFeeData, error) {
	bluesFeeData := make(map[daghash.Hash]CompactFeeData)

	for _, blueBlock := range node.Blues() {
		feeData, err := dbaccess.FetchFeeData(cb.dbContext, blueBlock.Hash())
		if err != nil {
			return nil, err
		}

		bluesFeeData[*blueBlock.Hash()] = feeData
	}

	return bluesFeeData, nil
}

// The following functions deal with building and validating the coinbase transaction

func (cb *Coinbase) ValidateCoinbaseTransaction(node *blocknode.BlockNode, block *util.Block, txsAcceptanceData common.MultiBlockTxsAcceptanceData) error {
	if node.IsGenesis() {
		return nil
	}
	blockCoinbaseTx := block.CoinbaseTransaction().MsgTx()
	_, scriptPubKey, extraData, err := coinbasepayload.DeserializeCoinbasePayload(blockCoinbaseTx)
	if errors.Is(err, coinbasepayload.ErrIncorrectScriptPubKeyLen) {
		return common.NewRuleError(common.ErrBadCoinbaseTransaction, err.Error())
	}
	if err != nil {
		return err
	}
	expectedCoinbaseTransaction, err := cb.ExpectedCoinbaseTransaction(node, txsAcceptanceData, scriptPubKey, extraData)
	if err != nil {
		return err
	}

	if !expectedCoinbaseTransaction.Hash().IsEqual(block.CoinbaseTransaction().Hash()) {
		return common.NewRuleError(common.ErrBadCoinbaseTransaction, "Coinbase transaction is not built as expected")
	}

	return nil
}

// ExpectedCoinbaseTransaction returns the coinbase transaction for the current block
func (cb *Coinbase) ExpectedCoinbaseTransaction(node *blocknode.BlockNode, txsAcceptanceData common.MultiBlockTxsAcceptanceData, scriptPubKey []byte, extraData []byte) (*util.Tx, error) {
	bluesFeeData, err := cb.getBluesFeeData(node)
	if err != nil {
		return nil, err
	}

	var txIns []*wire.TxIn
	var txOuts []*wire.TxOut

	for _, blue := range node.Blues() {
		txOut, err := cb.coinbaseOutputForBlueBlock(blue, txsAcceptanceData, bluesFeeData)
		if err != nil {
			return nil, err
		}
		if txOut != nil {
			txOuts = append(txOuts, txOut)
		}
	}
	payload, err := coinbasepayload.SerializeCoinbasePayload(node.BlueScore(), scriptPubKey, extraData)
	if err != nil {
		return nil, err
	}
	coinbaseTx := wire.NewSubnetworkMsgTx(wire.TxVersion, txIns, txOuts, subnetworkid.SubnetworkIDCoinbase, 0, payload)
	sortedCoinbaseTx := txsort.Sort(coinbaseTx)
	return util.NewTx(sortedCoinbaseTx), nil
}

// coinbaseOutputForBlueBlock calculates the output that should go into the coinbase transaction of blueBlock
// If blueBlock gets no fee - returns nil for txOut
func (cb *Coinbase) coinbaseOutputForBlueBlock(blueBlock *blocknode.BlockNode,
	txsAcceptanceData common.MultiBlockTxsAcceptanceData,
	feeData map[daghash.Hash]CompactFeeData) (*wire.TxOut, error) {

	blockTxsAcceptanceData, ok := txsAcceptanceData.FindAcceptanceData(blueBlock.Hash())
	if !ok {
		return nil, errors.Errorf("No txsAcceptanceData for block %s", blueBlock.Hash())
	}
	blockFeeData, ok := feeData[*blueBlock.Hash()]
	if !ok {
		return nil, errors.Errorf("No feeData for block %s", blueBlock.Hash())
	}

	if len(blockTxsAcceptanceData.TxAcceptanceData) != blockFeeData.Len() {
		return nil, errors.Errorf(
			"length of accepted transaction Data(%d) and fee Data(%d) is not equal for block %s",
			len(blockTxsAcceptanceData.TxAcceptanceData), blockFeeData.Len(), blueBlock.Hash())
	}

	totalFees := uint64(0)
	feeIterator := blockFeeData.iterator()

	for _, txAcceptanceData := range blockTxsAcceptanceData.TxAcceptanceData {
		fee, err := feeIterator.next()
		if err != nil {
			return nil, errors.Errorf("Error retrieving fee from CompactFeeData iterator: %s", err)
		}
		if txAcceptanceData.IsAccepted {
			totalFees += fee
		}
	}

	totalReward := CalcBlockSubsidy(blueBlock.BlueScore(), cb.params.SubsidyReductionInterval) + totalFees

	if totalReward == 0 {
		return nil, nil
	}

	// the ScriptPubKey for the coinbase is parsed from the coinbase payload
	_, scriptPubKey, _, err := coinbasepayload.DeserializeCoinbasePayload(blockTxsAcceptanceData.TxAcceptanceData[0].Tx.MsgTx())
	if err != nil {
		return nil, err
	}

	txOut := &wire.TxOut{
		Value:        totalReward,
		ScriptPubKey: scriptPubKey,
	}

	return txOut, nil
}

// CalcBlockSubsidy returns the subsidy amount a block at the provided blue score
// should have. This is mainly used for determining how much the coinbase for
// newly generated blocks awards as well as validating the coinbase for blocks
// has the expected value.
//
// The subsidy is halved every SubsidyReductionInterval blocks. Mathematically
// this is: baseSubsidy / 2^(blueScore/SubsidyReductionInterval)
//
// At the target block generation rate for the main network, this is
// approximately every 4 years.
func CalcBlockSubsidy(blueScore uint64, subsidyReductionInterval uint64) uint64 {
	if subsidyReductionInterval == 0 {
		return baseSubsidy
	}

	// Equivalent to: baseSubsidy / 2^(blueScore/subsidyHalvingInterval)
	return baseSubsidy >> uint(blueScore/subsidyReductionInterval)
}
