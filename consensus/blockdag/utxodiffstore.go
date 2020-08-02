package blockdag

import (
	"bytes"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/utxo"
	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
	"io"

	"github.com/kaspanet/kaspad/dbaccess"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/util/locks"
)

type blockUTXODiffData struct {
	diff      *utxo.UTXODiff
	diffChild *blocknode.BlockNode
}

type utxoDiffStore struct {
	dag    *BlockDAG
	dirty  map[*blocknode.BlockNode]struct{}
	loaded map[*blocknode.BlockNode]*blockUTXODiffData
	mtx    *locks.PriorityMutex
}

func newUTXODiffStore(dag *BlockDAG) *utxoDiffStore {
	return &utxoDiffStore{
		dag:    dag,
		dirty:  make(map[*blocknode.BlockNode]struct{}),
		loaded: make(map[*blocknode.BlockNode]*blockUTXODiffData),
		mtx:    locks.NewPriorityMutex(),
	}
}

func (diffStore *utxoDiffStore) setBlockDiff(node *blocknode.BlockNode, diff *utxo.UTXODiff) error {
	diffStore.mtx.HighPriorityWriteLock()
	defer diffStore.mtx.HighPriorityWriteUnlock()
	// load the diff data from DB to diffStore.loaded
	_, err := diffStore.diffDataByBlockNode(node)
	if dbaccess.IsNotFoundError(err) {
		diffStore.loaded[node] = &blockUTXODiffData{}
	} else if err != nil {
		return err
	}

	diffStore.loaded[node].diff = diff
	diffStore.setBlockAsDirty(node)
	return nil
}

func (diffStore *utxoDiffStore) setBlockDiffChild(node *blocknode.BlockNode, diffChild *blocknode.BlockNode) error {
	diffStore.mtx.HighPriorityWriteLock()
	defer diffStore.mtx.HighPriorityWriteUnlock()
	// load the diff data from DB to diffStore.loaded
	_, err := diffStore.diffDataByBlockNode(node)
	if err != nil {
		return err
	}

	diffStore.loaded[node].diffChild = diffChild
	diffStore.setBlockAsDirty(node)
	return nil
}

func (diffStore *utxoDiffStore) removeBlocksDiffData(dbContext dbaccess.Context, nodes []*blocknode.BlockNode) error {
	for _, node := range nodes {
		err := diffStore.removeBlockDiffData(dbContext, node)
		if err != nil {
			return err
		}
	}
	return nil
}

func (diffStore *utxoDiffStore) removeBlockDiffData(dbContext dbaccess.Context, node *blocknode.BlockNode) error {
	diffStore.mtx.LowPriorityWriteLock()
	defer diffStore.mtx.LowPriorityWriteUnlock()
	delete(diffStore.loaded, node)
	err := dbaccess.RemoveDiffData(dbContext, node.Hash())
	if err != nil {
		return err
	}
	return nil
}

func (diffStore *utxoDiffStore) setBlockAsDirty(node *blocknode.BlockNode) {
	diffStore.dirty[node] = struct{}{}
}

func (diffStore *utxoDiffStore) diffDataByBlockNode(node *blocknode.BlockNode) (*blockUTXODiffData, error) {
	if diffData, ok := diffStore.loaded[node]; ok {
		return diffData, nil
	}
	diffData, err := diffStore.diffDataFromDB(node.Hash())
	if err != nil {
		return nil, err
	}
	diffStore.loaded[node] = diffData
	return diffData, nil
}

func (diffStore *utxoDiffStore) diffByNode(node *blocknode.BlockNode) (*utxo.UTXODiff, error) {
	diffStore.mtx.HighPriorityReadLock()
	defer diffStore.mtx.HighPriorityReadUnlock()
	diffData, err := diffStore.diffDataByBlockNode(node)
	if err != nil {
		return nil, err
	}
	return diffData.diff, nil
}

func (diffStore *utxoDiffStore) diffChildByNode(node *blocknode.BlockNode) (*blocknode.BlockNode, error) {
	diffStore.mtx.HighPriorityReadLock()
	defer diffStore.mtx.HighPriorityReadUnlock()
	diffData, err := diffStore.diffDataByBlockNode(node)
	if err != nil {
		return nil, err
	}
	return diffData.diffChild, nil
}

func (diffStore *utxoDiffStore) diffDataFromDB(hash *daghash.Hash) (*blockUTXODiffData, error) {
	serializedBlockDiffData, err := dbaccess.FetchUTXODiffData(diffStore.dag.databaseContext, hash)
	if err != nil {
		return nil, err
	}

	return diffStore.deserializeBlockUTXODiffData(serializedBlockDiffData)
}

func (diffStore *utxoDiffStore) deserializeBlockUTXODiffData(serializedDiffData []byte) (*blockUTXODiffData, error) {
	diffData := &blockUTXODiffData{}
	r := bytes.NewBuffer(serializedDiffData)

	var hasDiffChild bool
	err := wire.ReadElement(r, &hasDiffChild)
	if err != nil {
		return nil, err
	}

	if hasDiffChild {
		hash := &daghash.Hash{}
		err := wire.ReadElement(r, hash)
		if err != nil {
			return nil, err
		}

		var ok bool
		diffData.diffChild, ok = diffStore.dag.blockNodeStore.LookupNode(hash)
		if !ok {
			return nil, errors.Errorf("block %s does not exist in the DAG", hash)
		}
	}

	diffData.diff, err = utxo.DeserializeUTXODiff(r)
	if err != nil {
		return nil, err
	}

	return diffData, nil
}

// flushToDB writes all dirty diff data to the database.
func (diffStore *utxoDiffStore) flushToDB(dbContext *dbaccess.TxContext) error {
	diffStore.mtx.HighPriorityWriteLock()
	defer diffStore.mtx.HighPriorityWriteUnlock()
	if len(diffStore.dirty) == 0 {
		return nil
	}

	// Allocate a buffer here to avoid needless allocations/grows
	// while writing each entry.
	buffer := &bytes.Buffer{}
	for node := range diffStore.dirty {
		buffer.Reset()
		diffData := diffStore.loaded[node]
		err := storeDiffData(dbContext, buffer, node.Hash(), diffData)
		if err != nil {
			return err
		}
	}
	return nil
}

func (diffStore *utxoDiffStore) clearDirtyEntries() {
	diffStore.dirty = make(map[*blocknode.BlockNode]struct{})
}

// maxBlueScoreDifferenceToKeepLoaded is the maximum difference
// between the virtual's blueScore and a BlockNode's blueScore
// under which to keep diff data loaded in memory.
var maxBlueScoreDifferenceToKeepLoaded uint64 = 100

// clearOldEntries removes entries whose blue score is lower than
// virtual.blueScore - maxBlueScoreDifferenceToKeepLoaded. Note
// that tips are not removed either even if their blue score is
// lower than the above.
func (diffStore *utxoDiffStore) clearOldEntries() {
	diffStore.mtx.HighPriorityWriteLock()
	defer diffStore.mtx.HighPriorityWriteUnlock()

	virtualBlueScore := diffStore.dag.VirtualBlueScore()
	minBlueScore := virtualBlueScore - maxBlueScoreDifferenceToKeepLoaded
	if maxBlueScoreDifferenceToKeepLoaded > virtualBlueScore {
		minBlueScore = 0
	}

	tips := diffStore.dag.virtual.tips()

	toRemove := make(map[*blocknode.BlockNode]struct{})
	for node := range diffStore.loaded {
		if node.BlueScore() < minBlueScore && !tips.Contains(node) {
			toRemove[node] = struct{}{}
		}
	}
	for node := range toRemove {
		delete(diffStore.loaded, node)
	}
}

// storeDiffData stores the UTXO diff data to the database.
// This overwrites the current entry if there exists one.
func storeDiffData(dbContext dbaccess.Context, w *bytes.Buffer, hash *daghash.Hash, diffData *blockUTXODiffData) error {
	// To avoid a ton of allocs, use the io.Writer
	// instead of allocating one. We expect the buffer to
	// already be initialized and, in most cases, to already
	// be large enough to accommodate the serialized data
	// without growing.
	err := serializeBlockUTXODiffData(w, diffData)
	if err != nil {
		return err
	}

	return dbaccess.StoreUTXODiffData(dbContext, hash, w.Bytes())
}

// serializeBlockUTXODiffData serializes diff data in the following format:
// 	Name         | Data type | Description
//  ------------ | --------- | -----------
// 	hasDiffChild | Boolean   | Indicates if a diff child exist
//  diffChild    | Hash      | The diffChild's hash. Empty if hasDiffChild is true.
//  diff		 | UTXODiff  | The diff data's diff
func serializeBlockUTXODiffData(w io.Writer, diffData *blockUTXODiffData) error {
	hasDiffChild := diffData.diffChild != nil
	err := wire.WriteElement(w, hasDiffChild)
	if err != nil {
		return err
	}
	if hasDiffChild {
		err := wire.WriteElement(w, diffData.diffChild.Hash())
		if err != nil {
			return err
		}
	}

	err = utxo.SerializeUTXODiff(w, diffData.diff)
	if err != nil {
		return err
	}

	return nil
}
