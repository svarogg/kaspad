package merklevalidation

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/merkle"
)

func ValidateAcceptedIDMerkleRoot(node *blocknode.BlockNode, txsAcceptanceData common.MultiBlockTxsAcceptanceData) error {
	if node.IsGenesis() {
		return nil
	}

	calculatedAccepetedIDMerkleRoot := merkle.CalculateAcceptedIDMerkleRoot(txsAcceptanceData)
	header := node.Header()
	if !header.AcceptedIDMerkleRoot.IsEqual(calculatedAccepetedIDMerkleRoot) {
		str := fmt.Sprintf("block accepted ID merkle root is invalid - block "+
			"header indicates %s, but calculated value is %s",
			header.AcceptedIDMerkleRoot, calculatedAccepetedIDMerkleRoot)
		return common.NewRuleError(common.ErrBadMerkleRoot, str)
	}
	return nil
}
