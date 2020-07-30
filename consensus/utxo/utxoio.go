package utxo

import (
	"bytes"
	"encoding/binary"
	"github.com/kaspanet/kaspad/dbaccess"
	"github.com/kaspanet/kaspad/util/binaryserializer"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
	"io"
)

// OutpointIndexByteOrder is the byte order for serializing the outpoint index.
// It uses big endian to ensure that when outpoint is used as database key, the
// keys will be iterated in an ascending order by the outpoint index.
var OutpointIndexByteOrder = binary.BigEndian

var OutpointSerializeSize = daghash.TxIDSize + 4

var (
	// byteOrder is the preferred byte order used for serializing numeric
	// fields for storage in the database.
	byteOrder = binary.LittleEndian
)

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

func DeserializeUTXODiff(r io.Reader) (*UTXODiff, error) {
	diff := &UTXODiff{}

	var err error
	diff.toAdd, err = deserializeUTXOCollection(r)
	if err != nil {
		return nil, err
	}

	diff.toRemove, err = deserializeUTXOCollection(r)
	if err != nil {
		return nil, err
	}

	return diff, nil
}

func deserializeUTXOCollection(r io.Reader) (utxoCollection, error) {
	count, err := wire.ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	collection := utxoCollection{}
	for i := uint64(0); i < count; i++ {
		utxoEntry, outpoint, err := deserializeUTXO(r)
		if err != nil {
			return nil, err
		}
		collection.add(*outpoint, utxoEntry)
	}
	return collection, nil
}

func deserializeUTXO(r io.Reader) (*UTXOEntry, *wire.Outpoint, error) {
	outpoint, err := DeserializeOutpoint(r)
	if err != nil {
		return nil, nil, err
	}

	utxoEntry, err := deserializeUTXOEntry(r)
	if err != nil {
		return nil, nil, err
	}
	return utxoEntry, outpoint, nil
}

// SerializeUTXODiff serializes UTXODiff by serializing
// UTXODiff.toAdd, UTXODiff.toRemove and UTXODiff.Multiset one after the other.
func SerializeUTXODiff(w io.Writer, diff *UTXODiff) error {
	err := serializeUTXOCollection(w, diff.toAdd)
	if err != nil {
		return err
	}

	err = serializeUTXOCollection(w, diff.toRemove)
	if err != nil {
		return err
	}

	return nil
}

// serializeUTXOCollection serializes utxoCollection by iterating over
// the utxo entries and serializing them and their corresponding outpoint
// prefixed by a varint that indicates their size.
func serializeUTXOCollection(w io.Writer, collection utxoCollection) error {
	err := wire.WriteVarInt(w, uint64(len(collection)))
	if err != nil {
		return err
	}
	for outpoint, utxoEntry := range collection {
		err := SerializeUTXO(w, utxoEntry, &outpoint)
		if err != nil {
			return err
		}
	}
	return nil
}

// SerializeUTXO serializes a utxo entry-outpoint pair
func SerializeUTXO(w io.Writer, entry *UTXOEntry, outpoint *wire.Outpoint) error {
	err := SerializeOutpoint(w, outpoint)
	if err != nil {
		return err
	}

	err = serializeUTXOEntry(w, entry)
	if err != nil {
		return err
	}
	return nil
}

// p2pkhUTXOEntrySerializeSize is the serialized size for a P2PKH UTXO entry.
// 8 bytes (header code) + 8 bytes (amount) + varint for script pub key length of 25 (for P2PKH) + 25 bytes for P2PKH script.
var p2pkhUTXOEntrySerializeSize = 8 + 8 + wire.VarIntSerializeSize(25) + 25

// serializeUTXOEntry encodes the entry to the given io.Writer and use compression if useCompression is true.
// The compression format is described in detail above.
func serializeUTXOEntry(w io.Writer, entry *UTXOEntry) error {
	// Encode the blueScore.
	err := binaryserializer.PutUint64(w, byteOrder, entry.blockBlueScore)
	if err != nil {
		return err
	}

	// Encode the packedFlags.
	err = binaryserializer.PutUint8(w, uint8(entry.packedFlags))
	if err != nil {
		return err
	}

	err = binaryserializer.PutUint64(w, byteOrder, entry.Amount())
	if err != nil {
		return err
	}

	err = wire.WriteVarInt(w, uint64(len(entry.ScriptPubKey())))
	if err != nil {
		return err
	}

	_, err = w.Write(entry.ScriptPubKey())
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// deserializeUTXOEntry decodes a UTXO entry from the passed reader
// into a new UTXOEntry. If isCompressed is used it will decompress
// the entry according to the format that is described in detail
// above.
func deserializeUTXOEntry(r io.Reader) (*UTXOEntry, error) {
	// Deserialize the blueScore.
	blockBlueScore, err := binaryserializer.Uint64(r, byteOrder)
	if err != nil {
		return nil, err
	}

	// Decode the packedFlags.
	packedFlags, err := binaryserializer.Uint8(r)
	if err != nil {
		return nil, err
	}

	entry := &UTXOEntry{
		blockBlueScore: blockBlueScore,
		packedFlags:    txoFlags(packedFlags),
	}

	entry.amount, err = binaryserializer.Uint64(r, byteOrder)
	if err != nil {
		return nil, err
	}

	scriptPubKeyLen, err := wire.ReadVarInt(r)
	if err != nil {
		return nil, err
	}

	entry.scriptPubKey = make([]byte, scriptPubKeyLen)
	_, err = r.Read(entry.scriptPubKey)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return entry, nil
}

// UpdateUTXOSet updates the UTXO set in the database based on the provided
// UTXO diff.
func UpdateUTXOSet(dbContext dbaccess.Context, virtualUTXODiff *UTXODiff) error {
	outpointBuff := bytes.NewBuffer(make([]byte, OutpointSerializeSize))
	for outpoint := range virtualUTXODiff.toRemove {
		outpointBuff.Reset()
		err := SerializeOutpoint(outpointBuff, &outpoint)
		if err != nil {
			return err
		}

		key := outpointBuff.Bytes()
		err = dbaccess.RemoveFromUTXOSet(dbContext, key)
		if err != nil {
			return err
		}
	}

	// We are preallocating for P2PKH entries because they are the most common ones.
	// If we have entries with a compressed script bigger than P2PKH's, the buffer will grow.
	utxoEntryBuff := bytes.NewBuffer(make([]byte, p2pkhUTXOEntrySerializeSize))

	for outpoint, entry := range virtualUTXODiff.toAdd {
		utxoEntryBuff.Reset()
		outpointBuff.Reset()
		// Serialize and store the UTXO entry.
		err := serializeUTXOEntry(utxoEntryBuff, entry)
		if err != nil {
			return err
		}
		serializedEntry := utxoEntryBuff.Bytes()

		err = SerializeOutpoint(outpointBuff, &outpoint)
		if err != nil {
			return err
		}

		key := outpointBuff.Bytes()
		err = dbaccess.AddToUTXOSet(dbContext, key, serializedEntry)
		if err != nil {
			return err
		}
	}

	return nil
}

func InitUTXOSet(dbContext dbaccess.Context) (*FullUTXOSet, error) {
	fullUTXOCollection := make(utxoCollection)
	cursor, err := dbaccess.UTXOSetCursor(dbContext)
	if err != nil {
		return nil, err
	}
	defer cursor.Close()

	for cursor.Next() {
		// Deserialize the outpoint
		key, err := cursor.Key()
		if err != nil {
			return nil, err
		}
		outpoint, err := DeserializeOutpoint(bytes.NewReader(key.Suffix()))
		if err != nil {
			return nil, err
		}

		// Deserialize the utxo entry
		value, err := cursor.Value()
		if err != nil {
			return nil, err
		}
		entry, err := deserializeUTXOEntry(bytes.NewReader(value))
		if err != nil {
			return nil, err
		}

		fullUTXOCollection[*outpoint] = entry
	}

	return newFullUTXOSetFromUTXOCollection(fullUTXOCollection)
}
