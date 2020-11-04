package rpchandlers

import (
	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/app/rpc/rpccontext"
	"github.com/kaspanet/kaspad/infrastructure/network/netadapter/router"
)

// HandleNotifyUTXOOfAddressChanged handles the respectively named RPC command
func HandleNotifyUTXOOfAddressChanged(context *rpccontext.Context, router *router.Router, request appmessage.Message) (appmessage.Message, error) {
	listener, err := context.NotificationManager.Listener(router)
	if err != nil {
		return nil, err
	}

	requestMessage := request.(*appmessage.NotifyUTXOOfAddressChangedRequestMessage)
	listener.PropagateUTXOOfAddressChangedNotifications(requestMessage.Addresses)

	response := appmessage.NewNotifyUTXOOfAddressChangedResponseMessage()
	return response, nil
}
