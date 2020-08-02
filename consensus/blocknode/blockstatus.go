package blocknode

// BlockStatus is a bit field representing the validation state of the block.
type BlockStatus byte

const (
	// StatusDataStored indicates that the block's payload is stored on disk.
	StatusDataStored BlockStatus = 1 << iota

	// StatusValid indicates that the block has been fully validated.
	StatusValid

	// StatusValidateFailed indicates that the block has failed validation.
	StatusValidateFailed

	// StatusInvalidAncestor indicates that one of the block's ancestors has
	// has failed validation, thus the block is also invalid.
	StatusInvalidAncestor
)

// KnownValid returns whether the block is known to be valid. This will return
// false for a valid block that has not been fully validated yet.
func (status BlockStatus) KnownValid() bool {
	return status&StatusValid != 0
}

// KnownInvalid returns whether the block is known to be invalid. This may be
// because the block itself failed validation or any of its ancestors is
// invalid. This will return false for invalid blocks that have not been proven
// invalid yet.
func (status BlockStatus) KnownInvalid() bool {
	return status&(StatusValidateFailed|StatusInvalidAncestor) != 0
}
