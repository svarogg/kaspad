package blocknode

import (
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/kaspanet/kaspad/util/mstime"
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

// This test is to ensure the size BlueAnticoneSizesSize is serialized to the size of KType.
// We verify that by serializing and deserializing the block while making sure that we stay within the expected range.
func TestBlueAnticoneSizesSize(t *testing.T) {
	k := dagconfig.KType(0)
	k--

	if k < dagconfig.KType(0) {
		t.Fatalf("KType must be unsigned")
	}

	blockHeader := dagconfig.SimnetParams.GenesisBlock.Header
	node := NewBlockNode(&blockHeader, NewBlockNodeSet(), mstime.Now())
	fakeBlue := &BlockNode{hash: &daghash.Hash{1}}

	blockNodeStore := NewStore(&dagconfig.SimnetParams)
	blockNodeStore.AddNode(fakeBlue)

	// Setting maxKType to maximum value of KType.
	// As we verify above that KType is unsigned we can be sure that maxKType is indeed the maximum value of KType.
	maxKType := ^dagconfig.KType(0)
	node.bluesAnticoneSizes[fakeBlue] = maxKType
	serializedNode, _ := SerializeBlockNode(node)
	deserializedNode, _ := blockNodeStore.deserializeBlockNode(serializedNode)
	if deserializedNode.bluesAnticoneSizes[fakeBlue] != maxKType {
		t.Fatalf("TestBlueAnticoneSizesSize: BlueAnticoneSize should not change when deserializing. Expected: %v but got %v",
			maxKType, deserializedNode.bluesAnticoneSizes[fakeBlue])
	}
}
