package transactionvalidation

import (
	"github.com/kaspanet/kaspad/consensus/txscript"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/mstime"
	"math"
)

// IsFinalizedTransaction determines whether or not a transaction is finalized.
func IsFinalizedTransaction(tx *util.Tx, blockBlueScore uint64, blockTime mstime.Time) bool {
	msgTx := tx.MsgTx()

	// Lock time of zero means the transaction is finalized.
	lockTime := msgTx.LockTime
	if lockTime == 0 {
		return true
	}

	// The lock time field of a transaction is either a block blue score at
	// which the transaction is finalized or a timestamp depending on if the
	// value is before the txscript.LockTimeThreshold. When it is under the
	// threshold it is a block blue score.
	blockTimeOrBlueScore := int64(0)
	if lockTime < txscript.LockTimeThreshold {
		blockTimeOrBlueScore = int64(blockBlueScore)
	} else {
		blockTimeOrBlueScore = blockTime.UnixMilliseconds()
	}
	if int64(lockTime) < blockTimeOrBlueScore {
		return true
	}

	// At this point, the transaction's lock time hasn't occurred yet, but
	// the transaction might still be finalized if the sequence number
	// for all transaction inputs is maxed out.
	for _, txIn := range msgTx.TxIn {
		if txIn.Sequence != math.MaxUint64 {
			return false
		}
	}
	return true
}
