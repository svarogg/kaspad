package blockdag

import (
	"reflect"
	"testing"

	"github.com/kaspanet/kaspad/util/daghash"
)

func TestHashes(t *testing.T) {
	bs := BlockNodeSetFromSlice(
		&BlockNode{
			hash: &daghash.Hash{3},
		},
		&BlockNode{
			hash: &daghash.Hash{1},
		},
		&BlockNode{
			hash: &daghash.Hash{0},
		},
		&BlockNode{
			hash: &daghash.Hash{2},
		},
	)

	expected := []*daghash.Hash{
		{0},
		{1},
		{2},
		{3},
	}

	hashes := bs.Hashes()
	if !daghash.AreEqual(hashes, expected) {
		t.Errorf("TestHashes: hashes order is %s but expected %s", hashes, expected)
	}
}

func TestBlockSetSubtract(t *testing.T) {
	node1 := &BlockNode{hash: &daghash.Hash{10}}
	node2 := &BlockNode{hash: &daghash.Hash{20}}
	node3 := &BlockNode{hash: &daghash.Hash{30}}

	tests := []struct {
		name           string
		setA           BlockNodeSet
		setB           BlockNodeSet
		expectedResult BlockNodeSet
	}{
		{
			name:           "both sets empty",
			setA:           BlockNodeSetFromSlice(),
			setB:           BlockNodeSetFromSlice(),
			expectedResult: BlockNodeSetFromSlice(),
		},
		{
			name:           "subtract an empty set",
			setA:           BlockNodeSetFromSlice(node1),
			setB:           BlockNodeSetFromSlice(),
			expectedResult: BlockNodeSetFromSlice(node1),
		},
		{
			name:           "subtract from empty set",
			setA:           BlockNodeSetFromSlice(),
			setB:           BlockNodeSetFromSlice(node1),
			expectedResult: BlockNodeSetFromSlice(),
		},
		{
			name:           "subtract unrelated set",
			setA:           BlockNodeSetFromSlice(node1),
			setB:           BlockNodeSetFromSlice(node2),
			expectedResult: BlockNodeSetFromSlice(node1),
		},
		{
			name:           "typical case",
			setA:           BlockNodeSetFromSlice(node1, node2),
			setB:           BlockNodeSetFromSlice(node2, node3),
			expectedResult: BlockNodeSetFromSlice(node1),
		},
	}

	for _, test := range tests {
		result := test.setA.Subtract(test.setB)
		if !reflect.DeepEqual(result, test.expectedResult) {
			t.Errorf("BlockNodeSet.subtract: unexpected result in test '%s'. "+
				"Expected: %v, got: %v", test.name, test.expectedResult, result)
		}
	}
}

func TestBlockSetAddSet(t *testing.T) {
	node1 := &BlockNode{hash: &daghash.Hash{10}}
	node2 := &BlockNode{hash: &daghash.Hash{20}}
	node3 := &BlockNode{hash: &daghash.Hash{30}}

	tests := []struct {
		name           string
		setA           BlockNodeSet
		setB           BlockNodeSet
		expectedResult BlockNodeSet
	}{
		{
			name:           "both sets empty",
			setA:           BlockNodeSetFromSlice(),
			setB:           BlockNodeSetFromSlice(),
			expectedResult: BlockNodeSetFromSlice(),
		},
		{
			name:           "add an empty set",
			setA:           BlockNodeSetFromSlice(node1),
			setB:           BlockNodeSetFromSlice(),
			expectedResult: BlockNodeSetFromSlice(node1),
		},
		{
			name:           "add to empty set",
			setA:           BlockNodeSetFromSlice(),
			setB:           BlockNodeSetFromSlice(node1),
			expectedResult: BlockNodeSetFromSlice(node1),
		},
		{
			name:           "add already added member",
			setA:           BlockNodeSetFromSlice(node1, node2),
			setB:           BlockNodeSetFromSlice(node1),
			expectedResult: BlockNodeSetFromSlice(node1, node2),
		},
		{
			name:           "typical case",
			setA:           BlockNodeSetFromSlice(node1, node2),
			setB:           BlockNodeSetFromSlice(node2, node3),
			expectedResult: BlockNodeSetFromSlice(node1, node2, node3),
		},
	}

	for _, test := range tests {
		test.setA.AddSet(test.setB)
		if !reflect.DeepEqual(test.setA, test.expectedResult) {
			t.Errorf("BlockNodeSet.AddSet: unexpected result in test '%s'. "+
				"Expected: %v, got: %v", test.name, test.expectedResult, test.setA)
		}
	}
}

func TestBlockSetAddSlice(t *testing.T) {
	node1 := &BlockNode{hash: &daghash.Hash{10}}
	node2 := &BlockNode{hash: &daghash.Hash{20}}
	node3 := &BlockNode{hash: &daghash.Hash{30}}

	tests := []struct {
		name           string
		set            BlockNodeSet
		slice          []*BlockNode
		expectedResult BlockNodeSet
	}{
		{
			name:           "add empty slice to empty set",
			set:            BlockNodeSetFromSlice(),
			slice:          []*BlockNode{},
			expectedResult: BlockNodeSetFromSlice(),
		},
		{
			name:           "add an empty slice",
			set:            BlockNodeSetFromSlice(node1),
			slice:          []*BlockNode{},
			expectedResult: BlockNodeSetFromSlice(node1),
		},
		{
			name:           "add to empty set",
			set:            BlockNodeSetFromSlice(),
			slice:          []*BlockNode{node1},
			expectedResult: BlockNodeSetFromSlice(node1),
		},
		{
			name:           "add already added member",
			set:            BlockNodeSetFromSlice(node1, node2),
			slice:          []*BlockNode{node1},
			expectedResult: BlockNodeSetFromSlice(node1, node2),
		},
		{
			name:           "typical case",
			set:            BlockNodeSetFromSlice(node1, node2),
			slice:          []*BlockNode{node2, node3},
			expectedResult: BlockNodeSetFromSlice(node1, node2, node3),
		},
	}

	for _, test := range tests {
		test.set.AddSlice(test.slice)
		if !reflect.DeepEqual(test.set, test.expectedResult) {
			t.Errorf("BlockNodeSet.AddSlice: unexpected result in test '%s'. "+
				"Expected: %v, got: %v", test.name, test.expectedResult, test.set)
		}
	}
}

func TestBlockSetUnion(t *testing.T) {
	node1 := &BlockNode{hash: &daghash.Hash{10}}
	node2 := &BlockNode{hash: &daghash.Hash{20}}
	node3 := &BlockNode{hash: &daghash.Hash{30}}

	tests := []struct {
		name           string
		setA           BlockNodeSet
		setB           BlockNodeSet
		expectedResult BlockNodeSet
	}{
		{
			name:           "both sets empty",
			setA:           BlockNodeSetFromSlice(),
			setB:           BlockNodeSetFromSlice(),
			expectedResult: BlockNodeSetFromSlice(),
		},
		{
			name:           "union against an empty set",
			setA:           BlockNodeSetFromSlice(node1),
			setB:           BlockNodeSetFromSlice(),
			expectedResult: BlockNodeSetFromSlice(node1),
		},
		{
			name:           "union from an empty set",
			setA:           BlockNodeSetFromSlice(),
			setB:           BlockNodeSetFromSlice(node1),
			expectedResult: BlockNodeSetFromSlice(node1),
		},
		{
			name:           "union with subset",
			setA:           BlockNodeSetFromSlice(node1, node2),
			setB:           BlockNodeSetFromSlice(node1),
			expectedResult: BlockNodeSetFromSlice(node1, node2),
		},
		{
			name:           "typical case",
			setA:           BlockNodeSetFromSlice(node1, node2),
			setB:           BlockNodeSetFromSlice(node2, node3),
			expectedResult: BlockNodeSetFromSlice(node1, node2, node3),
		},
	}

	for _, test := range tests {
		result := test.setA.Union(test.setB)
		if !reflect.DeepEqual(result, test.expectedResult) {
			t.Errorf("BlockNodeSet.union: unexpected result in test '%s'. "+
				"Expected: %v, got: %v", test.name, test.expectedResult, result)
		}
	}
}
