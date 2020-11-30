package rpchandlers

import (
	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/app/rpc/rpccontext"
	"github.com/kaspanet/kaspad/domain/consensus/model"
	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/domain/consensus/utils/hashes"
	"github.com/kaspanet/kaspad/infrastructure/network/netadapter/router"
)

// HandleGetBlocks handles the respectively named RPC command
func HandleGetBlocks(context *rpccontext.Context, _ *router.Router, request appmessage.Message) (appmessage.Message, error) {
	getBlocksRequest := request.(*appmessage.GetBlocksRequestMessage)
	startingBlockHash := getBlocksRequest.LowHash

	var lowHash *externalapi.DomainHash
	if len(startingBlockHash) > 0 {
		hash, err := hashes.FromString(getBlocksRequest.LowHash)
		if err != nil {
			errorMessage := &appmessage.GetBlocksResponseMessage{}
			errorMessage.Error = appmessage.RPCErrorf("Hash could not be parsed: %s", err)
			return errorMessage, nil
		}
		lowHash = hash
	} else {
		lowHash = context.Config.NetParams().GenesisHash
	}

	_, err := context.Domain.Consensus().GetBlock(lowHash)
	if err != nil {
		errorMessage := &appmessage.GetBlockResponseMessage{}
		errorMessage.Error = appmessage.RPCErrorf("Block %s not found", lowHash)
		return errorMessage, nil
	}

	blockHashes, err := context.Domain.Consensus().GetHashesBetween(lowHash, model.VirtualBlockHash)
	if err != nil {
		return nil, err
	}

	lastBlockHash := blockHashes[len(blockHashes)-1]
	if len(blockHashes) == 0 || lastBlockHash != model.VirtualBlockHash {
		return &appmessage.GetBlockResponseMessage{
			Error: appmessage.RPCErrorf("Unable to process input data"),
		}, nil
	}

	// remove virtual block hash from the end
	blockHashes = blockHashes[:len(blockHashes)-1]
	if len(blockHashes) == 0 {
		return appmessage.NewGetBlocksResponseMessage(nil, nil), nil
	}

	blockVerboseData := make([]*appmessage.BlockVerboseData, len(blockHashes))
	blockHashesStr := make([]string, len(blockHashes))
	if getBlocksRequest.IncludeBlockVerboseData {
		for i, blockHash := range blockHashes {
			block, err := context.Domain.Consensus().GetBlock(blockHash)
			if err != nil {
				return nil, err
			}
			verboseBlock, err := context.BuildBlockVerboseData(block, true)
			if err != nil {
				return nil, err
			}
			blockVerboseData[i] = verboseBlock
			blockHashesStr[i] = blockHash.String()
		}
	} else {
		for i, blockHash := range blockHashes {
			blockHashesStr[i] = blockHash.String()
		}
	}

	response := appmessage.NewGetBlocksResponseMessage(blockHashesStr, blockVerboseData)
	return response, nil
}
