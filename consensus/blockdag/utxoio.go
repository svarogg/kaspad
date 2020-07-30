package blockdag

import (
	"bytes"
	"github.com/kaspanet/kaspad/consensus/utxo"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
	"io"
)

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
		err := wire.WriteElement(w, diffData.diffChild.hash)
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
		diffData.diffChild, ok = diffStore.dag.index.LookupNode(hash)
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
