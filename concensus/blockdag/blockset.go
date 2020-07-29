package blockdag

import (
	"strings"

	"github.com/kaspanet/kaspad/util/daghash"
)

// BlockSet implements a basic unsorted set of blocks
type BlockSet map[*blockNode]struct{}

// NewBlockSet creates a new, empty BlockSet
func NewBlockSet() BlockSet {
	return map[*blockNode]struct{}{}
}

// BlockSetFromSlice converts a slice of blockNodes into an unordered set represented as map
func BlockSetFromSlice(nodes ...*blockNode) BlockSet {
	set := NewBlockSet()
	for _, node := range nodes {
		set.Add(node)
	}
	return set
}

// Add adds a blockNode to this BlockSet
func (bs BlockSet) Add(node *blockNode) {
	bs[node] = struct{}{}
}

// Remove removes a blockNode from this BlockSet, if exists
// Does nothing if this set does not contain the blockNode
func (bs BlockSet) Remove(node *blockNode) {
	delete(bs, node)
}

// Clone clones thie block set
func (bs BlockSet) Clone() BlockSet {
	clone := NewBlockSet()
	for node := range bs {
		clone.Add(node)
	}
	return clone
}

// Subtract returns the difference between the BlockSet and another BlockSet
func (bs BlockSet) Subtract(other BlockSet) BlockSet {
	diff := NewBlockSet()
	for node := range bs {
		if !other.Contains(node) {
			diff.Add(node)
		}
	}
	return diff
}

// AddSet adds all blockNodes in other set to this set
func (bs BlockSet) AddSet(other BlockSet) {
	for node := range other {
		bs.Add(node)
	}
}

// AddSlice adds provided slice to this set
func (bs BlockSet) AddSlice(slice []*blockNode) {
	for _, node := range slice {
		bs.Add(node)
	}
}

// Union returns a BlockSet that contains all blockNodes included in this set,
// the other set, or both
func (bs BlockSet) Union(other BlockSet) BlockSet {
	union := bs.Clone()

	union.AddSet(other)

	return union
}

// Contains returns true iff this set contains node
func (bs BlockSet) Contains(node *blockNode) bool {
	_, ok := bs[node]
	return ok
}

// Hashes returns the hashes of the blockNodes in this set.
func (bs BlockSet) Hashes() []*daghash.Hash {
	hashes := make([]*daghash.Hash, 0, len(bs))
	for node := range bs {
		hashes = append(hashes, node.hash)
	}
	daghash.Sort(hashes)
	return hashes
}

func (bs BlockSet) String() string {
	nodeStrs := make([]string, 0, len(bs))
	for node := range bs {
		nodeStrs = append(nodeStrs, node.String())
	}
	return strings.Join(nodeStrs, ",")
}

func (bs BlockSet) Bluest() *blockNode {
	var bluestNode *blockNode
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
