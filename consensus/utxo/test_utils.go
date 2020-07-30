package utxo

import (
	"compress/bzip2"
	"encoding/binary"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/wire"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LoadUTXOSet returns a utxo view loaded from a file.
func LoadUTXOSet(filename string) (UTXOSet, error) {
	// The utxostore file format is:
	// <tx hash><output index><serialized utxo len><serialized utxo>
	//
	// The output index and serialized utxo len are little endian uint32s
	// and the serialized utxo uses the format described in dagio.go.

	filename = filepath.Join("testdata", filename)
	fi, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	// Choose read based on whether the file is compressed or not.
	var r io.Reader
	if strings.HasSuffix(filename, ".bz2") {
		r = bzip2.NewReader(fi)
	} else {
		r = fi
	}
	defer fi.Close()

	utxoSet := NewFullUTXOSet()
	for {
		// Tx ID of the utxo entry.
		var txID daghash.TxID
		_, err := io.ReadAtLeast(r, txID[:], len(txID[:]))
		if err != nil {
			// Expected EOF at the right offset.
			if err == io.EOF {
				break
			}
			return nil, err
		}

		// Output index of the utxo entry.
		var index uint32
		err = binary.Read(r, binary.LittleEndian, &index)
		if err != nil {
			return nil, err
		}

		// Num of serialized utxo entry bytes.
		var numBytes uint32
		err = binary.Read(r, binary.LittleEndian, &numBytes)
		if err != nil {
			return nil, err
		}

		// Deserialize the UTXO entry and add it to the UTXO set.
		entry, err := deserializeUTXOEntry(r)
		if err != nil {
			return nil, err
		}
		utxoSet.utxoCollection[wire.Outpoint{TxID: txID, Index: index}] = entry
	}

	return utxoSet, nil
}
