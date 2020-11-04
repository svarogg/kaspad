package rpchandlers

import (
	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/app/rpc/rpccontext"
	"github.com/kaspanet/kaspad/infrastructure/network/netadapter/router"
)

// HandleNotifyTransactionAdded handles the respectively named RPC command
func HandleNotifyTransactionAdded(context *rpccontext.Context, router *router.Router, request appmessage.Message) (appmessage.Message, error) {
	listener, err := context.NotificationManager.Listener(router)
	if err != nil {
		return nil, err
	}

	transactionAddedRequestMessage := request.(*appmessage.NotifyTransactionAddedRequestMessage)
	listener.PropagateTransactionAddedNotifications(transactionAddedRequestMessage.Transaction.TxHash())

	response := appmessage.NewNotifyTransactionAddedResponseMessage()
	return response, nil
}
