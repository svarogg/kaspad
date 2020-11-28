package consensusserialization

import (
	"io"

	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/domain/consensus/utils/hashes"
	"github.com/kaspanet/kaspad/domain/consensus/utils/transactionhelper"
	"github.com/kaspanet/kaspad/util/binaryserializer"
	"github.com/pkg/errors"
)

// txEncoding is a bitmask defining which transaction fields we
// want to encode and which to ignore.
type txEncoding uint8

const (
	txEncodingFull txEncoding = 0

	// TODO: Consider if we need to ever exclude the payload, or use a different approach to partial nodes
	// where we'll get rid of PayloadHash field, never exclude the payload when hashing, and provide
	// partial nodes with their relevant block region with a merkle proof.
	txEncodingExcludePayload txEncoding = 1 << iota

	txEncodingExcludeSignatureScript
)

// TransactionHashForSigning hashes the transaction and the given hash type in a way that is intended for
// signatures.
func TransactionHashForSigning(tx *externalapi.DomainTransaction, hashType uint32) *externalapi.DomainHash {
	// Encode the header and double sha256 everything prior to the number of
	// transactions.
	writer := hashes.NewHashWriter()
	err := SerializeTransaction(writer, tx, txEncodingExcludePayload)
	if err != nil {
		// It seems like this could only happen if the writer returned an error.
		// and this writer should never return an error (no allocations or possible failures)
		// the only non-writer error path here is unknown types in `WriteElement`
		panic(errors.Wrap(err, "TransactionHashForSigning() failed. this should never fail for structurally-valid transactions"))
	}

	err = WriteElement(writer, hashType)
	if err != nil {
		panic(errors.Wrap(err, "this should never happen. SHA256's digest should never return an error"))
	}

	return writer.Finalize()
}

// TransactionHash returns the transaction hash.
func TransactionHash(tx *externalapi.DomainTransaction) *externalapi.DomainHash {
	// Encode the header and double sha256 everything prior to the number of
	// transactions.
	writer := hashes.NewHashWriter()
	err := SerializeTransaction(writer, tx, txEncodingExcludePayload)
	if err != nil {
		// It seems like this could only happen if the writer returned an error.
		// and this writer should never return an error (no allocations or possible failures)
		// the only non-writer error path here is unknown types in `WriteElement`
		panic(errors.Wrap(err, "TransactionHash() failed. this should never fail for structurally-valid transactions"))
	}

	return writer.Finalize()
}

// TransactionID generates the Hash for the transaction without the signature script and payload field.
func TransactionID(tx *externalapi.DomainTransaction) *externalapi.DomainTransactionID {

	// If transaction ID is already cached, return it
	if tx.ID != nil {
		return tx.ID
	}

	// Encode the transaction, replace signature script with zeroes, cut off
	// payload and calculate double sha256 on the result.
	var encodingFlags txEncoding
	if !transactionhelper.IsCoinBase(tx) {
		encodingFlags = txEncodingExcludeSignatureScript | txEncodingExcludePayload
	}
	writer := hashes.NewHashWriter()
	err := SerializeTransaction(writer, tx, encodingFlags)
	if err != nil {
		// this writer never return errors (no allocations or possible failures) so errors can only come from validity checks,
		// and we assume we never construct malformed transactions.
		panic(errors.Wrap(err, "TransactionID() failed. this should never fail for structurally-valid transactions"))
	}
	transactionID := externalapi.DomainTransactionID(*writer.Finalize())

	tx.ID = &transactionID

	return tx.ID
}

// SerializeTransaction serializes the transaction.
func SerializeTransaction(w io.Writer, tx *externalapi.DomainTransaction, encodingFlags txEncoding) error {
	err := binaryserializer.PutUint32(w, littleEndian, uint32(tx.Version))
	if err != nil {
		return err
	}

	count := uint64(len(tx.Inputs))
	err = WriteElement(w, count)
	if err != nil {
		return err
	}

	for _, ti := range tx.Inputs {
		err = writeTransactionInput(w, ti, encodingFlags)
		if err != nil {
			return err
		}
	}

	count = uint64(len(tx.Outputs))
	err = WriteElement(w, count)
	if err != nil {
		return err
	}

	for _, output := range tx.Outputs {
		err = writeTxOut(w, output)
		if err != nil {
			return err
		}
	}

	err = binaryserializer.PutUint64(w, littleEndian, tx.LockTime)
	if err != nil {
		return err
	}

	_, err = w.Write(tx.SubnetworkID[:])
	if err != nil {
		return err
	}

	err = binaryserializer.PutUint64(w, littleEndian, tx.Gas)
	if err != nil {
		return err
	}

	err = WriteElement(w, &tx.PayloadHash)
	if err != nil {
		return err
	}

	if encodingFlags&txEncodingExcludePayload != txEncodingExcludePayload {
		err = writeVarBytes(w, tx.Payload)
		if err != nil {
			return err
		}
	} else {
		err = writeVarBytes(w, []byte{})
		if err != nil {
			return err
		}
	}

	return nil
}

// writeTransactionInput encodes ti to the kaspa protocol encoding for a transaction
// input to w.
func writeTransactionInput(w io.Writer, ti *externalapi.DomainTransactionInput, encodingFlags txEncoding) error {
	err := writeOutpoint(w, &ti.PreviousOutpoint)
	if err != nil {
		return err
	}

	if encodingFlags&txEncodingExcludeSignatureScript != txEncodingExcludeSignatureScript {
		err = writeVarBytes(w, ti.SignatureScript)
	} else {
		err = writeVarBytes(w, []byte{})
	}
	if err != nil {
		return err
	}

	return binaryserializer.PutUint64(w, littleEndian, ti.Sequence)
}

func writeOutpoint(w io.Writer, outpoint *externalapi.DomainOutpoint) error {
	_, err := w.Write(outpoint.TransactionID[:])
	if err != nil {
		return err
	}

	return binaryserializer.PutUint32(w, littleEndian, outpoint.Index)
}

func writeVarBytes(w io.Writer, data []byte) error {
	dataLength := uint64(len(data))
	err := WriteElement(w, dataLength)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

func writeTxOut(w io.Writer, to *externalapi.DomainTransactionOutput) error {
	err := binaryserializer.PutUint64(w, littleEndian, to.Value)
	if err != nil {
		return err
	}

	return writeVarBytes(w, to.ScriptPublicKey)
}

func DeserializeTransaction(r io.Reader) (*externalapi.DomainTransaction, error) {
	txVersion, err := binaryserializer.Uint32(r, littleEndian)
	if err != nil {
		return nil, err
	}

	var inputsCount uint64
	err = readElement(r, &inputsCount)
	if err != nil {
		return nil, err
	}

	inputs := make([]*externalapi.DomainTransactionInput, inputsCount)
	for i := uint64(0); i < inputsCount; i++ {
		inputs[i], err = readTransactionInput(r)
		if err != nil {
			return nil, err
		}
	}

	var outputsCount uint64
	err = readElement(r, &outputsCount)
	if err != nil {
		return nil, err
	}
	outputs := make([]*externalapi.DomainTransactionOutput, inputsCount)
	for i := uint64(0); i < inputsCount; i++ {
		outputs[i], err = readTransactionOutput(r)
		if err != nil {
			return nil, err
		}
	}

	lockTime, err := binaryserializer.Uint64(r, littleEndian)
	if err != nil {
		return nil, err
	}

	var subnetworkID externalapi.DomainSubnetworkID
	_, err = io.ReadFull(r, subnetworkID[:])
	if err != nil {
		return nil, err
	}

	gas, err := binaryserializer.Uint64(r, littleEndian)
	if err != nil {
		return nil, err
	}

	var payloadHash externalapi.DomainHash
	err = readElement(r, &payloadHash)
	if err != nil {
		return nil, err
	}

	payload, err := readVarBytes(r)
	if err != nil {
		return nil, err
	}

	return &externalapi.DomainTransaction{
		Version:      int32(txVersion),
		Inputs:       inputs,
		Outputs:      outputs,
		LockTime:     lockTime,
		SubnetworkID: subnetworkID,
		Gas:          gas,
		PayloadHash:  payloadHash,
		Payload:      payload,
	}, nil
}

func readTransactionInput(r io.Reader) (*externalapi.DomainTransactionInput, error) {
	previousOutpoint, err := readOutpoint(r)
	if err != nil {
		return nil, err
	}

	signatureScript, err := readVarBytes(r)
	if err != nil {
		return nil, err
	}

	sequence, err := binaryserializer.Uint64(r, littleEndian)
	if err != nil {
		return nil, err
	}

	return &externalapi.DomainTransactionInput{
		PreviousOutpoint: *previousOutpoint,
		SignatureScript:  signatureScript,
		Sequence:         sequence,
	}, nil
}

func readOutpoint(r io.Reader) (*externalapi.DomainOutpoint, error) {
	var transactionID externalapi.DomainTransactionID
	_, err := io.ReadFull(r, transactionID[:])
	if err != nil {
		return nil, err
	}

	index, err := binaryserializer.Uint32(r, littleEndian)
	if err != nil {
		return nil, err
	}

	return &externalapi.DomainOutpoint{
		TransactionID: transactionID,
		Index:         index,
	}, err
}

func readVarBytes(r io.Reader) ([]byte, error) {
	var dataLength uint64
	err := readElement(r, &dataLength)
	if err != nil {
		return nil, err
	}

	data := make([]byte, dataLength)
	_, err = io.ReadFull(r, data)
	return data, err
}

func readTransactionOutput(r io.Reader) (*externalapi.DomainTransactionOutput, error) {
	value, err := binaryserializer.Uint64(r, littleEndian)
	if err != nil {
		return nil, err
	}

	scriptPublicKey, err := readVarBytes(r)
	if err != nil {
		return nil, err
	}

	return &externalapi.DomainTransactionOutput{
		Value:           value,
		ScriptPublicKey: scriptPublicKey,
	}, nil
}
