package sequencelock

import (
	"github.com/kaspanet/kaspad/util/mstime"
	"testing"
)

// TODO: Fix this test
//// TestCalcSequenceLock tests the LockTimeToSequence function, and the
//// CalcSequenceLock method of a DAG instance. The tests exercise several
//// combinations of inputs to the CalcSequenceLock function in order to ensure
//// the returned SequenceLocks are correct for each test instance.
//func TestCalcSequenceLock(t *testing.T) {
//	netParams := &dagconfig.SimnetParams
//
//	blockVersion := int32(0x10000000)
//
//	// Generate enough synthetic blocks for the rest of the test
//	dag := newTestDAG(netParams)
//	node := dag.selectedTip()
//	blockTime := node.Header().Timestamp
//	numBlocksToGenerate := 5
//	for i := 0; i < numBlocksToGenerate; i++ {
//		blockTime = blockTime.Add(time.Second)
//		node = newTestNode(dag, blocknode.BlockNodeSetFromSlice(node), blockVersion, 0, blockTime)
//		dag.blockNodeStore.AddNode(node)
//		dag.virtual.SetTips(blocknode.BlockNodeSetFromSlice(node))
//	}
//
//	// Create a utxo view with a fake utxo for the inputs used in the
//	// transactions created below. This utxo is added such that it has an
//	// age of 4 blocks.
//	msgTx := wire.NewNativeMsgTx(wire.TxVersion, nil, []*wire.TxOut{{ScriptPubKey: nil, Value: 10}})
//	targetTx := util.NewTx(msgTx)
//	utxoSet := utxo.NewFullUTXOSet()
//	blueScore := uint64(numBlocksToGenerate) - 4
//	if isAccepted, err := utxoSet.AddTx(targetTx.MsgTx(), blueScore); err != nil {
//		t.Fatalf("AddTx unexpectedly failed. Error: %s", err)
//	} else if !isAccepted {
//		t.Fatalf("AddTx unexpectedly didn't add tx %s", targetTx.ID())
//	}
//
//	// Create a utxo that spends the fake utxo created above for use in the
//	// transactions created in the tests. It has an age of 4 blocks. Note
//	// that the sequence lock heights are always calculated from the same
//	// point of view that they were originally calculated from for a given
//	// utxo. That is to say, the height prior to it.
//	outpoint := wire.Outpoint{
//		TxID:  *targetTx.ID(),
//		Index: 0,
//	}
//	prevUtxoBlueScore := uint64(numBlocksToGenerate) - 4
//
//	// Obtain the past median time from the PoV of the input created above.
//	// The past median time for the input is the past median time from the PoV
//	// of the block *prior* to the one that included it.
//	medianTime := dag.PastMedianTime(node.RelativeAncestor(5)).UnixMilliseconds()
//
//	// The median time calculated from the PoV of the best block in the
//	// test DAG. For unconfirmed inputs, this value will be used since
//	// the MTP will be calculated from the PoV of the yet-to-be-mined
//	// block.
//	nextMedianTime := dag.PastMedianTime(node).UnixMilliseconds()
//	nextBlockBlueScore := int32(numBlocksToGenerate) + 1
//
//	// Add an additional transaction which will serve as our unconfirmed
//	// output.
//	unConfTx := wire.NewNativeMsgTx(wire.TxVersion, nil, []*wire.TxOut{{ScriptPubKey: nil, Value: 5}})
//	unConfUtxo := wire.Outpoint{
//		TxID:  *unConfTx.TxID(),
//		Index: 0,
//	}
//	if isAccepted, err := utxoSet.AddTx(unConfTx, utxo.UnacceptedBlueScore); err != nil {
//		t.Fatalf("AddTx unexpectedly failed. Error: %s", err)
//	} else if !isAccepted {
//		t.Fatalf("AddTx unexpectedly didn't add tx %s", unConfTx.TxID())
//	}
//
//	tests := []struct {
//		name    string
//		tx      *wire.MsgTx
//		utxoSet utxo.UTXOSet
//		mempool bool
//		want    *SequenceLock
//	}{
//		// A transaction with a single input with max sequence number.
//		// This sequence number has the high bit set, so sequence locks
//		// should be disabled.
//		{
//			name:    "single input, max sequence number",
//			tx:      wire.NewNativeMsgTx(1, []*wire.TxIn{{PreviousOutpoint: outpoint, Sequence: wire.MaxTxInSequenceNum}}, nil),
//			utxoSet: utxoSet,
//			want: &SequenceLock{
//				Milliseconds:   -1,
//				BlockBlueScore: -1,
//			},
//		},
//		// A transaction with a single input whose lock time is
//		// expressed in seconds. However, the specified lock time is
//		// below the required floor for time based lock times since
//		// they have time granularity of 524288 milliseconds. As a result, the
//		// milliseconds lock-time should be just before the median time of
//		// the targeted block.
//		{
//			name:    "single input, milliseconds lock time below time granularity",
//			tx:      wire.NewNativeMsgTx(1, []*wire.TxIn{{PreviousOutpoint: outpoint, Sequence: LockTimeToSequence(true, 2)}}, nil),
//			utxoSet: utxoSet,
//			want: &SequenceLock{
//				Milliseconds:   medianTime - 1,
//				BlockBlueScore: -1,
//			},
//		},
//		// A transaction with a single input whose lock time is
//		// expressed in seconds. The number of seconds should be 1048575
//		// milliseconds after the median past time of the DAG.
//		{
//			name:    "single input, 1048575 milliseconds after median time",
//			tx:      wire.NewNativeMsgTx(1, []*wire.TxIn{{PreviousOutpoint: outpoint, Sequence: LockTimeToSequence(true, 1048576)}}, nil),
//			utxoSet: utxoSet,
//			want: &SequenceLock{
//				Milliseconds:   medianTime + 1048575,
//				BlockBlueScore: -1,
//			},
//		},
//		// A transaction with multiple inputs. The first input has a
//		// lock time expressed in seconds. The second input has a
//		// sequence lock in blocks with a value of 4. The last input
//		// has a sequence number with a value of 5, but has the disable
//		// bit set. So the first lock should be selected as it's the
//		// latest lock that isn't disabled.
//		{
//			name: "multiple varied inputs",
//			tx: wire.NewNativeMsgTx(1,
//				[]*wire.TxIn{{
//					PreviousOutpoint: outpoint,
//					Sequence:         LockTimeToSequence(true, 2621440),
//				}, {
//					PreviousOutpoint: outpoint,
//					Sequence:         LockTimeToSequence(false, 4),
//				}, {
//					PreviousOutpoint: outpoint,
//					Sequence: LockTimeToSequence(false, 5) |
//						wire.SequenceLockTimeDisabled,
//				}},
//				nil),
//			utxoSet: utxoSet,
//			want: &SequenceLock{
//				Milliseconds:   medianTime + (5 << wire.SequenceLockTimeGranularity) - 1,
//				BlockBlueScore: int64(prevUtxoBlueScore) + 3,
//			},
//		},
//		// Transaction with a single input. The input's sequence number
//		// encodes a relative lock-time in blocks (3 blocks). The
//		// sequence lock should  have a value of -1 for seconds, but a
//		// height of 2 meaning it can be included at height 3.
//		{
//			name:    "single input, lock-time in blocks",
//			tx:      wire.NewNativeMsgTx(1, []*wire.TxIn{{PreviousOutpoint: outpoint, Sequence: LockTimeToSequence(false, 3)}}, nil),
//			utxoSet: utxoSet,
//			want: &SequenceLock{
//				Milliseconds:   -1,
//				BlockBlueScore: int64(prevUtxoBlueScore) + 2,
//			},
//		},
//		// A transaction with two inputs with lock times expressed in
//		// seconds. The selected sequence lock value for seconds should
//		// be the time further in the future.
//		{
//			name: "two inputs, lock-times in seconds",
//			tx: wire.NewNativeMsgTx(1, []*wire.TxIn{{
//				PreviousOutpoint: outpoint,
//				Sequence:         LockTimeToSequence(true, 5242880),
//			}, {
//				PreviousOutpoint: outpoint,
//				Sequence:         LockTimeToSequence(true, 2621440),
//			}}, nil),
//			utxoSet: utxoSet,
//			want: &SequenceLock{
//				Milliseconds:   medianTime + (10 << wire.SequenceLockTimeGranularity) - 1,
//				BlockBlueScore: -1,
//			},
//		},
//		// A transaction with two inputs with lock times expressed in
//		// blocks. The selected sequence lock value for blocks should
//		// be the height further in the future, so a height of 10
//		// indicating it can be included at height 11.
//		{
//			name: "two inputs, lock-times in blocks",
//			tx: wire.NewNativeMsgTx(1,
//				[]*wire.TxIn{{
//					PreviousOutpoint: outpoint,
//					Sequence:         LockTimeToSequence(false, 1),
//				}, {
//					PreviousOutpoint: outpoint,
//					Sequence:         LockTimeToSequence(false, 11),
//				}},
//				nil),
//			utxoSet: utxoSet,
//			want: &SequenceLock{
//				Milliseconds:   -1,
//				BlockBlueScore: int64(prevUtxoBlueScore) + 10,
//			},
//		},
//		// A transaction with multiple inputs. Two inputs are time
//		// based, and the other two are block based. The lock lying
//		// further into the future for both inputs should be chosen.
//		{
//			name: "four inputs, two lock-times in time, two lock-times in blocks",
//			tx: wire.NewNativeMsgTx(1,
//				[]*wire.TxIn{{
//					PreviousOutpoint: outpoint,
//					Sequence:         LockTimeToSequence(true, 2621440),
//				}, {
//					PreviousOutpoint: outpoint,
//					Sequence:         LockTimeToSequence(true, 6815744),
//				}, {
//					PreviousOutpoint: outpoint,
//					Sequence:         LockTimeToSequence(false, 3),
//				}, {
//					PreviousOutpoint: outpoint,
//					Sequence:         LockTimeToSequence(false, 9),
//				}},
//				nil),
//			utxoSet: utxoSet,
//			want: &SequenceLock{
//				Milliseconds:   medianTime + (13 << wire.SequenceLockTimeGranularity) - 1,
//				BlockBlueScore: int64(prevUtxoBlueScore) + 8,
//			},
//		},
//		// A transaction with a single unconfirmed input. As the input
//		// is confirmed, the height of the input should be interpreted
//		// as the height of the *next* block. So, a 2 block relative
//		// lock means the sequence lock should be for 1 block after the
//		// *next* block height, indicating it can be included 2 blocks
//		// after that.
//		{
//			name:    "single input, unconfirmed, lock-time in blocks",
//			tx:      wire.NewNativeMsgTx(1, []*wire.TxIn{{PreviousOutpoint: unConfUtxo, Sequence: LockTimeToSequence(false, 2)}}, nil),
//			utxoSet: utxoSet,
//			mempool: true,
//			want: &SequenceLock{
//				Milliseconds:   -1,
//				BlockBlueScore: int64(nextBlockBlueScore) + 1,
//			},
//		},
//		// A transaction with a single unconfirmed input. The input has
//		// a time based lock, so the lock time should be based off the
//		// MTP of the *next* block.
//		{
//			name:    "single input, unconfirmed, lock-time in milliseoncds",
//			tx:      wire.NewNativeMsgTx(1, []*wire.TxIn{{PreviousOutpoint: unConfUtxo, Sequence: LockTimeToSequence(true, 1048576)}}, nil),
//			utxoSet: utxoSet,
//			mempool: true,
//			want: &SequenceLock{
//				Milliseconds:   nextMedianTime + 1048575,
//				BlockBlueScore: -1,
//			},
//		},
//	}
//
//	t.Logf("Running %v SequenceLock tests", len(tests))
//	for _, test := range tests {
//		utilTx := util.NewTx(test.tx)
//		seqLock, err := dag.CalcSequenceLock(utilTx, utxoSet, test.mempool)
//		if err != nil {
//			t.Fatalf("test '%s', unable to calc sequence lock: %v", test.name, err)
//		}
//
//		if seqLock.Milliseconds != test.want.Milliseconds {
//			t.Fatalf("test '%s' got %v milliseconds want %v milliseconds",
//				test.name, seqLock.Milliseconds, test.want.Milliseconds)
//		}
//		if seqLock.BlockBlueScore != test.want.BlockBlueScore {
//			t.Fatalf("test '%s' got blue score of %v want blue score of %v ",
//				test.name, seqLock.BlockBlueScore, test.want.BlockBlueScore)
//		}
//	}
//}

// TestSequenceLocksActive tests the IsActive function to ensure it
// works as expected in all possible combinations/scenarios.
func TestSequenceLocksActive(t *testing.T) {
	seqLock := func(blueScore int64, milliseconds int64) *SequenceLock {
		return &SequenceLock{
			Milliseconds:   milliseconds,
			BlockBlueScore: blueScore,
		}
	}

	tests := []struct {
		seqLock        *SequenceLock
		blockBlueScore uint64
		mtp            mstime.Time

		want bool
	}{
		// Block based sequence lock with equal block blue score.
		{seqLock: seqLock(1000, -1), blockBlueScore: 1001, mtp: mstime.UnixMilliseconds(9), want: true},

		// Time based sequence lock with mtp past the absolute time.
		{seqLock: seqLock(-1, 30), blockBlueScore: 2, mtp: mstime.UnixMilliseconds(31), want: true},

		// Block based sequence lock with current blue score below seq lock block blue score.
		{seqLock: seqLock(1000, -1), blockBlueScore: 90, mtp: mstime.UnixMilliseconds(9), want: false},

		// Time based sequence lock with current time before lock time.
		{seqLock: seqLock(-1, 30), blockBlueScore: 2, mtp: mstime.UnixMilliseconds(29), want: false},

		// Block based sequence lock at the same blue score, so shouldn't yet be active.
		{seqLock: seqLock(1000, -1), blockBlueScore: 1000, mtp: mstime.UnixMilliseconds(9), want: false},

		// Time based sequence lock with current time equal to lock time, so shouldn't yet be active.
		{seqLock: seqLock(-1, 30), blockBlueScore: 2, mtp: mstime.UnixMilliseconds(30), want: false},
	}

	t.Logf("Running %d sequence locks tests", len(tests))
	for i, test := range tests {
		got := test.seqLock.IsActive(test.blockBlueScore, test.mtp)
		if got != test.want {
			t.Fatalf("IsActive #%d got %v want %v", i,
				got, test.want)
		}
	}
}
