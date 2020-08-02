package blocknode

import "github.com/kaspanet/kaspad/util/daghash"

func NewBlockNodeForTest(hash *daghash.Hash) *BlockNode {
	return &BlockNode{hash: hash}
}
