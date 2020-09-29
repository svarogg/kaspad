package rpcclient

import "github.com/kaspanet/kaspad/app/appmessage"

// GetVersion sends an RPC request respective to the function's name and returns the RPC server's response
func (c *RPCClient) GetVersion() (*appmessage.GetVersionResponseMessage, error) {
	err := c.rpcRouter.outgoingRoute().Enqueue(appmessage.NewGetVersionRequestMessage())
	if err != nil {
		return nil, err
	}
	response, err := c.route(appmessage.CmdGetVersionResponseMessage).DequeueWithTimeout(c.timeout)
	if err != nil {
		return nil, err
	}
	getVersionResponse := response.(*appmessage.GetVersionResponseMessage)
	if getVersionResponse.Error != nil {
		return nil, c.convertRPCError(getVersionResponse.Error)
	}
	return getVersionResponse, nil
}
