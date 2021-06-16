package mempool

import (
	"fmt"

	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/domain/consensus/utils/consensushashing"
)

// this function MUST be called with the mempool mutex locked for reads
func (mp *mempool) validateTransactionInIsolation(transaction *externalapi.DomainTransaction) error {
	transactionID := consensushashing.TransactionID(transaction)
	if _, ok := mp.transactionsPool.allTransactions[*transactionID]; ok {
		return transactionRuleError(RejectDuplicate,
			fmt.Sprintf("transaction %s is already in the mempool", transactionID))
	}

	if !mp.config.AcceptNonStandard {
		if err := mp.checkTransactionStandardInIsolation(transaction); err != nil {
			// Attempt to extract a reject code from the error so
			// it can be retained. When not possible, fall back to
			// a non standard error.
			rejectCode, found := extractRejectCode(err)
			if !found {
				rejectCode = RejectNonstandard
			}
			str := fmt.Sprintf("transaction %s is not standard: %s", transactionID, err)
			return transactionRuleError(rejectCode, str)
		}
	}

	if err := mp.mempoolUTXOSet.checkDoubleSpends(transaction); err != nil {
		return err
	}

	return nil
}

// this function MUST be called with the mempool mutex locked for writes
func (mp *mempool) validateTransactionInContext(transaction *externalapi.DomainTransaction) error {
	transactionID := consensushashing.TransactionID(transaction)
	if transaction.Mass > mp.config.MaximumMassAcceptedByBlock {
		return transactionRuleError(RejectInvalid, fmt.Sprintf("transaction %s mass is %d which is "+
			"higher than the maxmimum of %d", transactionID,
			transaction.Mass, mp.config.MaximumMassAcceptedByBlock))
	}

	if !mp.config.AcceptNonStandard {
		err := mp.checkTransactionStandardInContext(transaction)
		if err != nil {
			// Attempt to extract a reject code from the error so
			// it can be retained. When not possible, fall back to
			// a non standard error.
			rejectCode, found := extractRejectCode(err)
			if !found {
				rejectCode = RejectNonstandard
			}
			str := fmt.Sprintf("transaction inputs %s are not standard: %s",
				consensushashing.TransactionID(transaction), err)
			return transactionRuleError(rejectCode, str)
		}
	}

	return nil
}
