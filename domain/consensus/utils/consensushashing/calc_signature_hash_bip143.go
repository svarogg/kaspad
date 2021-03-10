package consensushashing

import (
	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/domain/consensus/utils/constants"
	"github.com/kaspanet/kaspad/domain/consensus/utils/hashes"
	"github.com/kaspanet/kaspad/domain/consensus/utils/serialization"
	"github.com/pkg/errors"
)

// SigHashType represents hash type bits at the end of a signature.
type SigHashType uint32

// Hash type bits from the end of a signature.
const (
	SigHashAll          SigHashType = 0x1
	SigHashNone         SigHashType = 0x2
	SigHashSingle       SigHashType = 0x3
	SigHashAnyOneCanPay SigHashType = 0x80

	// SigHashMask defines the number of bits of the hash type which is used
	// to identify which outputs are signed.
	SigHashMask = 0x1f
)

func (sht SigHashType) isSigHashAll() bool {
	return sht&SigHashMask == SigHashAll
}
func (sht SigHashType) isSigHashNone() bool {
	return sht&SigHashMask == SigHashNone
}
func (sht SigHashType) isSigHashSingle() bool {
	return sht&SigHashMask == SigHashSingle
}
func (sht SigHashType) isSigHashAnyOneCanPay() bool {
	return sht&SigHashAnyOneCanPay == SigHashAnyOneCanPay
}

type SighashReusedValues struct {
	previousOutputsHash *externalapi.DomainHash
	sequencesHash       *externalapi.DomainHash
	outputsHash         *externalapi.DomainHash
	payloadHash         *externalapi.DomainHash
}

// CalcSignatureHash will, given a script and hash type calculate the signature hash
// to be used for signing and verification.
// This returns error only if one of the provided parameters are invalid.
func CalcSignatureHash_BIP143(tx *externalapi.DomainTransaction, inputIndex int, hashType SigHashType,
	reusedValues *SighashReusedValues) (*externalapi.DomainHash, *SighashReusedValues, error) {

	txIn := tx.Inputs[inputIndex]
	prevScriptPublicKey := txIn.UTXOEntry.ScriptPublicKey()

	if prevScriptPublicKey.Version > constants.MaxScriptPublicKeyVersion {
		return nil, nil, errors.Errorf("Script version is unkown.")
	}
	return calcSignatureHash_BIP143(tx, inputIndex, txIn, prevScriptPublicKey, hashType, reusedValues)
}

func calcSignatureHash_BIP143(tx *externalapi.DomainTransaction, inputIndex int, txIn *externalapi.DomainTransactionInput,
	prevScriptPublicKey *externalapi.ScriptPublicKey, hashType SigHashType, reusedValues *SighashReusedValues) (
	*externalapi.DomainHash, *SighashReusedValues, error) {

	hashWriter := hashes.NewTransactionSigningHashWriter()
	infallibleWriteElement(hashWriter, tx.Version)

	// TODO: PreviousOutputsHash

	sequencesHash := getSequencesHash(tx, hashType, reusedValues)
	infallibleWriteElement(hashWriter, sequencesHash)

	infallibleWriteElement(hashWriter, txIn.PreviousOutpoint.TransactionID)
	infallibleWriteElement(hashWriter, txIn.PreviousOutpoint.Index)

	infallibleWriteElement(hashWriter, prevScriptPublicKey.Version)
	infallibleWriteElement(hashWriter, prevScriptPublicKey.Script)

	infallibleWriteElement(hashWriter, txIn.UTXOEntry.Amount())

	infallibleWriteElement(hashWriter, txIn.Sequence)

	outputsHash := getOutputsHash(tx, inputIndex, hashType, reusedValues)
	infallibleWriteElement(hashWriter, outputsHash)

	infallibleWriteElement(hashWriter, tx.LockTime)

	payloadHash := getPayloadHash(tx, reusedValues)
	infallibleWriteElement(hashWriter, payloadHash)

	infallibleWriteElement(hashWriter, hashType)

	return hashWriter.Finalize(), nil, nil
}

func getSequencesHash(tx *externalapi.DomainTransaction, hashType SigHashType, reusedValues *SighashReusedValues) *externalapi.DomainHash {
	if hashType.isSigHashSingle() || hashType.isSigHashAnyOneCanPay() || hashType.isSigHashNone() {
		return externalapi.NewZeroHash()
	}

	if reusedValues.sequencesHash == nil {

		hashWriter := hashes.NewTransactionSigningHashWriter()
		for _, txIn := range tx.Inputs {
			infallibleWriteElement(hashWriter, txIn.Sequence)
		}
		reusedValues.sequencesHash = hashWriter.Finalize()
	}
	return reusedValues.sequencesHash
}

func getOutputsHash(tx *externalapi.DomainTransaction, inputIndex int, hashType SigHashType, reusedValues *SighashReusedValues) *externalapi.DomainHash {
	// SigHashNone: return zero-hash
	if hashType.isSigHashNone() {
		return externalapi.NewZeroHash()
	}

	// SigHashSingle: If the relevant output exists - return it's hash, otherwise return zero-hash
	if hashType.isSigHashSingle() {
		if len(tx.Outputs) >= inputIndex {
			return externalapi.NewZeroHash()
		}
		hashWriter := hashes.NewTransactionSigningHashWriter()
		hashTxOut(hashWriter, tx.Outputs[inputIndex])
		return hashWriter.Finalize()
	}

	// SigHashAll: Return hash of all outputs. Re-use hash if available.
	if reusedValues.outputsHash == nil {
		hashWriter := hashes.NewTransactionSigningHashWriter()
		for _, txOut := range tx.Outputs {
			hashTxOut(hashWriter, txOut)
		}
		reusedValues.outputsHash = hashWriter.Finalize()
	}

	return reusedValues.outputsHash
}

func hashTxOut(hashWriter hashes.HashWriter, txOut *externalapi.DomainTransactionOutput) {
	infallibleWriteElement(hashWriter, txOut.Value)
	infallibleWriteElement(hashWriter, txOut.ScriptPublicKey.Version)
	infallibleWriteElement(hashWriter, txOut.ScriptPublicKey.Script)
}

func getPayloadHash(tx *externalapi.DomainTransaction, reusedValues *SighashReusedValues) *externalapi.DomainHash {
	if reusedValues.payloadHash == nil {
		hashWriter := hashes.NewTransactionSigningHashWriter()
		infallibleWriteElement(hashWriter, tx.Payload)
		reusedValues.payloadHash = hashWriter.Finalize()
	}
	return reusedValues.payloadHash
}

func infallibleWriteElement(hashWriter hashes.HashWriter, element interface{}) {
	err := serialization.WriteElement(hashWriter, element)
	if err != nil {
		// It seems like this could only happen if the writer returned an error.
		// and this writer should never return an error (no allocations or possible failures)
		// the only non-writer error path here is unknown types in `WriteElement`
		panic(errors.Wrap(err, "TransactionHashForSigning() failed. this should never fail for structurally-valid transactions"))
	}
}
