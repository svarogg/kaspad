package blockdag

import (
	"strings"

	"github.com/kaspanet/kaspad/util/daghash"
)

// BlockNodeSet implements a basic unsorted set of blocks
type BlockNodeSet map[*BlockNode]struct{}

// NewBlockNodeSet creates a new, empty BlockNodeSet
func NewBlockNodeSet() BlockNodeSet {
	return map[*BlockNode]struct{}{}
}

// BlockNodeSetFromSlice converts a slice of blockNodes into an unordered set represented as map
func BlockNodeSetFromSlice(nodes ...*BlockNode) BlockNodeSet {
	set := NewBlockNodeSet()
	for _, node := range nodes {
		set.Add(node)
	}
	return set
}

// Add adds a BlockNode to this BlockNodeSet
func (bs BlockNodeSet) Add(node *BlockNode) {
	bs[node] = struct{}{}
}

// Remove removes a BlockNode from this BlockNodeSet, if exists
// Does nothing if this set does not contain the BlockNode
func (bs BlockNodeSet) Remove(node *BlockNode) {
	delete(bs, node)
}

// Clone clones this block set
func (bs BlockNodeSet) Clone() BlockNodeSet {
	clone := NewBlockNodeSet()
	for node := range bs {
		clone.Add(node)
	}
	return clone
}

// Subtract returns the difference between the BlockNodeSet and another BlockNodeSet
func (bs BlockNodeSet) Subtract(other BlockNodeSet) BlockNodeSet {
	diff := NewBlockNodeSet()
	for node := range bs {
		if !other.Contains(node) {
			diff.Add(node)
		}
	}
	return diff
}

// AddSet adds all blockNodes in other set to this set
func (bs BlockNodeSet) AddSet(other BlockNodeSet) {
	for node := range other {
		bs.Add(node)
	}
}

// AddSlice adds provided slice to this set
func (bs BlockNodeSet) AddSlice(slice []*BlockNode) {
	for _, node := range slice {
		bs.Add(node)
	}
}

// Union returns a BlockNodeSet that contains all blockNodes included in this set,
// the other set, or both
func (bs BlockNodeSet) Union(other BlockNodeSet) BlockNodeSet {
	union := bs.Clone()

	union.AddSet(other)

	return union
}

// Contains returns true iff this set contains node
func (bs BlockNodeSet) Contains(node *BlockNode) bool {
	_, ok := bs[node]
	return ok
}

// Hashes returns the hashes of the blockNodes in this set.
func (bs BlockNodeSet) Hashes() []*daghash.Hash {
	hashes := make([]*daghash.Hash, 0, len(bs))
	for node := range bs {
		hashes = append(hashes, node.hash)
	}
	daghash.Sort(hashes)
	return hashes
}

func (bs BlockNodeSet) String() string {
	nodeStrs := make([]string, 0, len(bs))
	for node := range bs {
		nodeStrs = append(nodeStrs, node.String())
	}
	return strings.Join(nodeStrs, ",")
}

func (bs BlockNodeSet) Bluest() *BlockNode {
	var bluestNode *BlockNode
	var maxScore uint64
	for node := range bs {
		if bluestNode == nil ||
			node.blueScore > maxScore ||
			(node.blueScore == maxScore && daghash.Less(node.hash, bluestNode.hash)) {
			bluestNode = node
			maxScore = node.blueScore
		}
	}
	return bluestNode
}
