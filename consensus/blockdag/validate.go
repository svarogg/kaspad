// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockdag

import (
	"github.com/kaspanet/kaspad/consensus/common"
	"github.com/kaspanet/kaspad/consensus/validation/blockvalidation"
	"github.com/kaspanet/kaspad/consensus/validation/utxovalidation"
	"github.com/kaspanet/kaspad/util"
	"github.com/pkg/errors"
)

// CheckConnectBlockTemplateNoLock fully validates that connecting the passed block to
// the DAG does not violate any consensus rules, aside from the proof of
// work requirement. The block must connect to the current tip of the main dag.
func (dag *BlockDAG) CheckConnectBlockTemplateNoLock(block *util.Block) error {

	// Skip the proof of work check as this is just a block template.
	flags := common.BFNoPoWCheck

	header := block.MsgBlock().Header

	delay, err := blockvalidation.CheckBlockSanity(block, dag.Params, dag.subnetworkID, dag.timeSource, flags)
	if err != nil {
		return err
	}

	if delay != 0 {
		return errors.Errorf("Block timestamp is too far in the future")
	}

	parents, err := lookupParentNodes(block, dag)
	if err != nil {
		return err
	}

	err = blockvalidation.CheckBlockContext(dag.difficulty, dag.pastMedianTimeFactory, dag.reachabilityTree, block, parents, flags)
	if err != nil {
		return err
	}

	templateNode, _ := dag.initBlockNode(&header, dag.virtual.Tips())

	_, err = utxovalidation.CheckConnectToPastUTXO(templateNode,
		dag.UTXOSet(), block.Transactions(), false, dag.Params, dag.sigCache, dag.pastMedianTimeFactory, dag.sequenceLockCalculator)

	return err
}
