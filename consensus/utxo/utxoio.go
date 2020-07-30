package utxo

import (
	"encoding/binary"
	"github.com/kaspanet/kaspad/util/binaryserializer"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/wire"
	"io"
)

// OutpointIndexByteOrder is the byte order for serializing the outpoint index.
// It uses big endian to ensure that when outpoint is used as database key, the
// keys will be iterated in an ascending order by the outpoint index.
var OutpointIndexByteOrder = binary.BigEndian

var OutpointSerializeSize = daghash.TxIDSize + 4

func SerializeOutpoint(w io.Writer, outpoint *wire.Outpoint) error {
	_, err := w.Write(outpoint.TxID[:])
	if err != nil {
		return err
	}

	return binaryserializer.PutUint32(w, OutpointIndexByteOrder, outpoint.Index)
}

// DeserializeOutpoint decodes an outpoint from the passed serialized byte
// slice into a new wire.Outpoint using a format that is suitable for long-
// term storage. This format is described in detail above.
func DeserializeOutpoint(r io.Reader) (*wire.Outpoint, error) {
	outpoint := &wire.Outpoint{}
	_, err := r.Read(outpoint.TxID[:])
	if err != nil {
		return nil, err
	}

	outpoint.Index, err = binaryserializer.Uint32(r, OutpointIndexByteOrder)
	if err != nil {
		return nil, err
	}

	return outpoint, nil
}
