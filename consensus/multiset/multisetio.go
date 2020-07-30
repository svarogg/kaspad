package multiset

import (
	"encoding/binary"
	"github.com/kaspanet/go-secp256k1"
	"io"
)

var (
	byteOrder = binary.LittleEndian
)

// SerializeMultiset serializes an ECMH multiset.
func SerializeMultiset(w io.Writer, ms *secp256k1.MultiSet) error {
	serialized := ms.Serialize()
	err := binary.Write(w, byteOrder, serialized)
	if err != nil {
		return err
	}
	return nil
}

// DeserializeMultiset deserializes an EMCH multiset.
func DeserializeMultiset(r io.Reader) (*secp256k1.MultiSet, error) {
	serialized := &secp256k1.SerializedMultiSet{}
	err := binary.Read(r, byteOrder, serialized[:])
	if err != nil {
		return nil, err
	}
	return secp256k1.DeserializeMultiSet(serialized)
}
