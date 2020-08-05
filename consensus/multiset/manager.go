package multiset

import (
	"github.com/kaspanet/go-secp256k1"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/utxo"
	"github.com/kaspanet/kaspad/dbaccess"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
)

type MultiSetManager struct {
	store *MultisetStore
}

func NewManager() *MultiSetManager {
	return &MultiSetManager{
		store: NewMultisetStore(),
	}
}

// CalcMultiset returns the multiset of the past UTXO of the given block.
func (msm *MultiSetManager) CalcMultiset(node *blocknode.BlockNode, acceptanceData common.MultiBlockTxsAcceptanceData,
	selectedParentPastUTXO utxo.UTXOSet) (*secp256k1.MultiSet, error) {

	return msm.PastUTXOMultiSet(node, acceptanceData, selectedParentPastUTXO)
}

func (msm *MultiSetManager) PastUTXOMultiSet(node *blocknode.BlockNode, acceptanceData common.MultiBlockTxsAcceptanceData,
	selectedParentPastUTXO utxo.UTXOSet) (*secp256k1.MultiSet, error) {

	ms, err := msm.SelectedParentMultiset(node)
	if err != nil {
		return nil, err
	}

	for _, blockAcceptanceData := range acceptanceData {
		for _, txAcceptanceData := range blockAcceptanceData.TxAcceptanceData {
			if !txAcceptanceData.IsAccepted {
				continue
			}

			tx := txAcceptanceData.Tx.MsgTx()

			var err error
			ms, err = msm.AddTxToMultiset(ms, tx, selectedParentPastUTXO, node.BlueScore())
			if err != nil {
				return nil, err
			}
		}
	}
	return ms, nil
}

// SelectedParentMultiset returns the multiset of the node's selected
// parent. If the node is the genesis BlockNode then it does not have
// a selected parent, in which case return a new, empty multiset.
func (msm *MultiSetManager) SelectedParentMultiset(node *blocknode.BlockNode) (*secp256k1.MultiSet, error) {
	if node.IsGenesis() {
		return secp256k1.NewMultiset(), nil
	}

	ms, err := msm.store.MultisetByBlockHash(node.SelectedParent().Hash())
	if err != nil {
		return nil, err
	}

	return ms, nil
}

func (msm *MultiSetManager) AddTxToMultiset(ms *secp256k1.MultiSet, tx *wire.MsgTx, pastUTXO utxo.UTXOSet, blockBlueScore uint64) (*secp256k1.MultiSet, error) {
	for _, txIn := range tx.TxIn {
		entry, ok := pastUTXO.Get(txIn.PreviousOutpoint)
		if !ok {
			return nil, errors.Errorf("Couldn't find entry for outpoint %s", txIn.PreviousOutpoint)
		}

		var err error
		ms, err = utxo.RemoveUTXOFromMultiset(ms, entry, &txIn.PreviousOutpoint)
		if err != nil {
			return nil, err
		}
	}

	isCoinbase := tx.IsCoinBase()
	for i, txOut := range tx.TxOut {
		outpoint := *wire.NewOutpoint(tx.TxID(), uint32(i))
		entry := utxo.NewUTXOEntry(txOut, isCoinbase, blockBlueScore)

		var err error
		ms, err = utxo.AddUTXOToMultiset(ms, entry, &outpoint)
		if err != nil {
			return nil, err
		}
	}
	return ms, nil
}

func (msm *MultiSetManager) SetMultiset(blockHash *daghash.Hash, ms *secp256k1.MultiSet) {
	msm.store.SetMultiset(blockHash, ms)
}

// FlushToDB writes all new multiset data to the database.
func (msm *MultiSetManager) FlushToDB(dbContext *dbaccess.TxContext) error {
	return msm.store.FlushToDB(dbContext)
}

func (msm *MultiSetManager) ClearNewEntries() {
	msm.store.ClearNewEntries()
}

func (msm *MultiSetManager) Init(dbContext dbaccess.Context) error {
	return msm.store.Init(dbContext)
}
