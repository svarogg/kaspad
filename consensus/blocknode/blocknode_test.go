package blocknode

import (
	"testing"
)

func TestAncestorErrors(t *testing.T) {
	node := BlockNode{blueScore: 2}
	node.blueScore = 2
	ancestor := node.SelectedAncestor(3)
	if ancestor != nil {
		t.Errorf("TestAncestorErrors: Ancestor() unexpectedly returned a node. Expected: <nil>")
	}
}
