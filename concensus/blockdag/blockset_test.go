package blockdag

import (
	"reflect"
	"testing"

	"github.com/kaspanet/kaspad/util/daghash"
)

func TestHashes(t *testing.T) {
	bs := BlockSetFromSlice(
		&blockNode{
			hash: &daghash.Hash{3},
		},
		&blockNode{
			hash: &daghash.Hash{1},
		},
		&blockNode{
			hash: &daghash.Hash{0},
		},
		&blockNode{
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
	node1 := &blockNode{hash: &daghash.Hash{10}}
	node2 := &blockNode{hash: &daghash.Hash{20}}
	node3 := &blockNode{hash: &daghash.Hash{30}}

	tests := []struct {
		name           string
		setA           BlockSet
		setB           BlockSet
		expectedResult BlockSet
	}{
		{
			name:           "both sets empty",
			setA:           BlockSetFromSlice(),
			setB:           BlockSetFromSlice(),
			expectedResult: BlockSetFromSlice(),
		},
		{
			name:           "subtract an empty set",
			setA:           BlockSetFromSlice(node1),
			setB:           BlockSetFromSlice(),
			expectedResult: BlockSetFromSlice(node1),
		},
		{
			name:           "subtract from empty set",
			setA:           BlockSetFromSlice(),
			setB:           BlockSetFromSlice(node1),
			expectedResult: BlockSetFromSlice(),
		},
		{
			name:           "subtract unrelated set",
			setA:           BlockSetFromSlice(node1),
			setB:           BlockSetFromSlice(node2),
			expectedResult: BlockSetFromSlice(node1),
		},
		{
			name:           "typical case",
			setA:           BlockSetFromSlice(node1, node2),
			setB:           BlockSetFromSlice(node2, node3),
			expectedResult: BlockSetFromSlice(node1),
		},
	}

	for _, test := range tests {
		result := test.setA.Subtract(test.setB)
		if !reflect.DeepEqual(result, test.expectedResult) {
			t.Errorf("BlockSet.subtract: unexpected result in test '%s'. "+
				"Expected: %v, got: %v", test.name, test.expectedResult, result)
		}
	}
}

func TestBlockSetAddSet(t *testing.T) {
	node1 := &blockNode{hash: &daghash.Hash{10}}
	node2 := &blockNode{hash: &daghash.Hash{20}}
	node3 := &blockNode{hash: &daghash.Hash{30}}

	tests := []struct {
		name           string
		setA           BlockSet
		setB           BlockSet
		expectedResult BlockSet
	}{
		{
			name:           "both sets empty",
			setA:           BlockSetFromSlice(),
			setB:           BlockSetFromSlice(),
			expectedResult: BlockSetFromSlice(),
		},
		{
			name:           "add an empty set",
			setA:           BlockSetFromSlice(node1),
			setB:           BlockSetFromSlice(),
			expectedResult: BlockSetFromSlice(node1),
		},
		{
			name:           "add to empty set",
			setA:           BlockSetFromSlice(),
			setB:           BlockSetFromSlice(node1),
			expectedResult: BlockSetFromSlice(node1),
		},
		{
			name:           "add already added member",
			setA:           BlockSetFromSlice(node1, node2),
			setB:           BlockSetFromSlice(node1),
			expectedResult: BlockSetFromSlice(node1, node2),
		},
		{
			name:           "typical case",
			setA:           BlockSetFromSlice(node1, node2),
			setB:           BlockSetFromSlice(node2, node3),
			expectedResult: BlockSetFromSlice(node1, node2, node3),
		},
	}

	for _, test := range tests {
		test.setA.AddSet(test.setB)
		if !reflect.DeepEqual(test.setA, test.expectedResult) {
			t.Errorf("BlockSet.AddSet: unexpected result in test '%s'. "+
				"Expected: %v, got: %v", test.name, test.expectedResult, test.setA)
		}
	}
}

func TestBlockSetAddSlice(t *testing.T) {
	node1 := &blockNode{hash: &daghash.Hash{10}}
	node2 := &blockNode{hash: &daghash.Hash{20}}
	node3 := &blockNode{hash: &daghash.Hash{30}}

	tests := []struct {
		name           string
		set            BlockSet
		slice          []*blockNode
		expectedResult BlockSet
	}{
		{
			name:           "add empty slice to empty set",
			set:            BlockSetFromSlice(),
			slice:          []*blockNode{},
			expectedResult: BlockSetFromSlice(),
		},
		{
			name:           "add an empty slice",
			set:            BlockSetFromSlice(node1),
			slice:          []*blockNode{},
			expectedResult: BlockSetFromSlice(node1),
		},
		{
			name:           "add to empty set",
			set:            BlockSetFromSlice(),
			slice:          []*blockNode{node1},
			expectedResult: BlockSetFromSlice(node1),
		},
		{
			name:           "add already added member",
			set:            BlockSetFromSlice(node1, node2),
			slice:          []*blockNode{node1},
			expectedResult: BlockSetFromSlice(node1, node2),
		},
		{
			name:           "typical case",
			set:            BlockSetFromSlice(node1, node2),
			slice:          []*blockNode{node2, node3},
			expectedResult: BlockSetFromSlice(node1, node2, node3),
		},
	}

	for _, test := range tests {
		test.set.AddSlice(test.slice)
		if !reflect.DeepEqual(test.set, test.expectedResult) {
			t.Errorf("BlockSet.AddSlice: unexpected result in test '%s'. "+
				"Expected: %v, got: %v", test.name, test.expectedResult, test.set)
		}
	}
}

func TestBlockSetUnion(t *testing.T) {
	node1 := &blockNode{hash: &daghash.Hash{10}}
	node2 := &blockNode{hash: &daghash.Hash{20}}
	node3 := &blockNode{hash: &daghash.Hash{30}}

	tests := []struct {
		name           string
		setA           BlockSet
		setB           BlockSet
		expectedResult BlockSet
	}{
		{
			name:           "both sets empty",
			setA:           BlockSetFromSlice(),
			setB:           BlockSetFromSlice(),
			expectedResult: BlockSetFromSlice(),
		},
		{
			name:           "union against an empty set",
			setA:           BlockSetFromSlice(node1),
			setB:           BlockSetFromSlice(),
			expectedResult: BlockSetFromSlice(node1),
		},
		{
			name:           "union from an empty set",
			setA:           BlockSetFromSlice(),
			setB:           BlockSetFromSlice(node1),
			expectedResult: BlockSetFromSlice(node1),
		},
		{
			name:           "union with subset",
			setA:           BlockSetFromSlice(node1, node2),
			setB:           BlockSetFromSlice(node1),
			expectedResult: BlockSetFromSlice(node1, node2),
		},
		{
			name:           "typical case",
			setA:           BlockSetFromSlice(node1, node2),
			setB:           BlockSetFromSlice(node2, node3),
			expectedResult: BlockSetFromSlice(node1, node2, node3),
		},
	}

	for _, test := range tests {
		result := test.setA.Union(test.setB)
		if !reflect.DeepEqual(result, test.expectedResult) {
			t.Errorf("BlockSet.union: unexpected result in test '%s'. "+
				"Expected: %v, got: %v", test.name, test.expectedResult, result)
		}
	}
}
