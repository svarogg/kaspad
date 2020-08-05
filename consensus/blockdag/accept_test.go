package blockdag

import (
	"errors"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/test"
	"path/filepath"
	"testing"

	"github.com/kaspanet/kaspad/dagconfig"
)

func TestMaybeAcceptBlockErrors(t *testing.T) {
	// Create a new database and DAG instance to run tests against.
	dag, teardownFunc, err := DAGSetup("TestMaybeAcceptBlockErrors", true, Config{
		DAGParams: &dagconfig.SimnetParams,
	})
	if err != nil {
		t.Fatalf("TestMaybeAcceptBlockErrors: Failed to setup DAG instance: %v", err)
	}
	defer teardownFunc()

	dag.TestSetCoinbaseMaturity(0)

	// Test rejecting the block if its parents are missing
	orphanBlockFile := "blk_3B.dat"
	loadedBlocks, err := test.LoadBlocks(filepath.Join("../test/", orphanBlockFile))
	if err != nil {
		t.Fatalf("TestMaybeAcceptBlockErrors: "+
			"Error loading file '%s': %s\n", orphanBlockFile, err)
	}
	block := loadedBlocks[0]

	err = dag.maybeAcceptBlock(block, common.BFNone)
	if err == nil {
		t.Errorf("TestMaybeAcceptBlockErrors: rejecting the block if its parents are missing: "+
			"Expected: %s, got: <nil>", common.ErrParentBlockUnknown)
	}
	var ruleErr common.RuleError
	if ok := errors.As(err, &ruleErr); !ok {
		t.Errorf("TestMaybeAcceptBlockErrors: rejecting the block if its parents are missing: "+
			"Expected RuleError but got %s", err)
	} else if ruleErr.ErrorCode != common.ErrParentBlockUnknown {
		t.Errorf("TestMaybeAcceptBlockErrors: rejecting the block if its parents are missing: "+
			"Unexpected error code. Want: %s, got: %s", common.ErrParentBlockUnknown, ruleErr.ErrorCode)
	}

	// Test rejecting the block if its parents are invalid
	blocksFile := "blk_0_to_4.dat"
	blocks, err := test.LoadBlocks(filepath.Join("../test/", blocksFile))
	if err != nil {
		t.Fatalf("TestMaybeAcceptBlockErrors: "+
			"Error loading file '%s': %s\n", blocksFile, err)
	}

	// Add a valid block and mark it as invalid
	block1 := blocks[1]
	isOrphan, isDelayed, err := dag.ProcessBlock(block1, common.BFNone)
	if err != nil {
		t.Fatalf("TestMaybeAcceptBlockErrors: Valid block unexpectedly returned an error: %s", err)
	}
	if isDelayed {
		t.Fatalf("TestMaybeAcceptBlockErrors: block 1 is too far in the future")
	}
	if isOrphan {
		t.Fatalf("TestMaybeAcceptBlockErrors: incorrectly returned block 1 is an orphan")
	}
	blockNode1, ok := dag.blockNodeStore.LookupNode(block1.Hash())
	if !ok {
		t.Fatalf("block %s does not exist in the DAG", block1.Hash())
	}
	dag.blockNodeStore.SetStatusFlags(blockNode1, blocknode.StatusValidateFailed)

	block2 := blocks[2]
	err = dag.maybeAcceptBlock(block2, common.BFNone)
	if err == nil {
		t.Errorf("TestMaybeAcceptBlockErrors: rejecting the block if its parents are invalid: "+
			"Expected: %s, got: <nil>", common.ErrInvalidAncestorBlock)
	}
	if ok := errors.As(err, &ruleErr); !ok {
		t.Errorf("TestMaybeAcceptBlockErrors: rejecting the block if its parents are invalid: "+
			"Expected RuleError but got %s", err)
	} else if ruleErr.ErrorCode != common.ErrInvalidAncestorBlock {
		t.Errorf("TestMaybeAcceptBlockErrors: rejecting the block if its parents are invalid: "+
			"Unexpected error. Want: %s, got: %s", common.ErrInvalidAncestorBlock, ruleErr.ErrorCode)
	}

	// Set block1's status back to valid for next tests
	dag.blockNodeStore.UnsetStatusFlags(blockNode1, blocknode.StatusValidateFailed)

	// Test rejecting the block due to bad context
	originalBits := block2.MsgBlock().Header.Bits
	block2.MsgBlock().Header.Bits = 0
	err = dag.maybeAcceptBlock(block2, common.BFNone)
	if err == nil {
		t.Errorf("TestMaybeAcceptBlockErrors: rejecting the block due to bad context: "+
			"Expected: %s, got: <nil>", common.ErrUnexpectedDifficulty)
	}
	if ok := errors.As(err, &ruleErr); !ok {
		t.Errorf("TestMaybeAcceptBlockErrors: rejecting the block due to bad context: "+
			"Expected RuleError but got %s", err)
	} else if ruleErr.ErrorCode != common.ErrUnexpectedDifficulty {
		t.Errorf("TestMaybeAcceptBlockErrors: rejecting the block due to bad context: "+
			"Unexpected error. Want: %s, got: %s", common.ErrUnexpectedDifficulty, ruleErr.ErrorCode)
	}

	// Set block2's bits back to valid for next tests
	block2.MsgBlock().Header.Bits = originalBits
}
