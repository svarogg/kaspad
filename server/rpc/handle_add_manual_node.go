package rpc

import (
	"github.com/kaspanet/kaspad/rpcmodel"
	"github.com/kaspanet/kaspad/util/network"
)

// handleAddManualNode handles addManualNode commands.
func handleAddManualNode(s *Server, cmd interface{}, closeChan <-chan struct{}) (interface{}, error) {
	c := cmd.(*rpcmodel.ConnectCmd)

	oneTry := c.OneTry != nil && *c.OneTry

	addr, err := network.NormalizeAddress(c.Target, s.cfg.DAGParams.DefaultPort)
	if err != nil {
		return nil, &rpcmodel.RPCError{
			Code:    rpcmodel.ErrRPCInvalidParameter,
			Message: err.Error(),
		}
	}

	if oneTry {
		s.cfg.ConnMgr.Connect(addr, false)
	} else {
		s.cfg.ConnMgr.Connect(addr, true)
	}

	// no data returned unless an error.
	return nil, nil
}
