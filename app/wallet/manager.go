package wallet

import (
	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/app/rpc"
	"github.com/kaspanet/kaspad/app/wallet/wallethandlers"
	"github.com/kaspanet/kaspad/app/wallet/walletnotification"
	"github.com/kaspanet/kaspad/domain/blockdag/indexers"
	routerpkg "github.com/kaspanet/kaspad/infrastructure/network/netadapter/router"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
)

type sender interface {
	RegisterHandler(command appmessage.MessageCommand, rpcHandler rpc.Handler)
}

var walletHandlers = map[appmessage.MessageCommand]wallethandlers.HandlerFunc{
	appmessage.CmdNotifyBlockAddedRequestMessage:           wallethandlers.HandleNotifyBlockAdded,
	appmessage.CmdNotifyTransactionAddedRequestMessage:     wallethandlers.HandleNotifyTransactionAdded,
	appmessage.CmdNotifyUTXOOfAddressChangedRequestMessage: wallethandlers.HandleNotifyUTXOOfAddressChanged,
	appmessage.CmdNotifyChainChangedRequestMessage:         wallethandlers.HandleNotifyChainChanged,
	appmessage.CmdNotifyFinalityConflictsRequestMessage:    wallethandlers.HandleNotifyFinalityConflicts,
}

// Manager is an wallet manager
type Manager struct {
	rpcManager          *rpc.Manager
	notificationManager *walletnotification.Manager
}

// NewManager creates a new wallet Manager
func NewManager(rpcManager *rpc.Manager) *Manager {
	return &Manager{
		rpcManager:          rpcManager,
		notificationManager: walletnotification.NewNotificationManager(),
	}
}

// RegisterWalletHandlers register all wallet manger handlers to the sender
func (m *Manager) RegisterWalletHandlers(handlerSender sender) {
	for command, handler := range walletHandlers {
		handlerInContext := wallethandlers.HandlerInContext{
			Context: m,
			Handler: handler,
		}
		handlerSender.RegisterHandler(command, &handlerInContext)
	}
}

// AcceptanceIndex returns AcceptanceIndex instance
func (m *Manager) AcceptanceIndex() *indexers.AcceptanceIndex {
	return m.rpcManager.Context().AcceptanceIndex
}

// Listener retrieves the listener registered with the given router
func (m *Manager) Listener(router *routerpkg.Router) (*walletnotification.Listener, error) {
	return m.notificationManager.Listener(router)
}

// NotifyBlockAddedToDAG notifies the manager that a block has been added to the DAG
func (m *Manager) NotifyBlockAddedToDAG(block *util.Block) error {
	m.rpcManager.Context().BlockTemplateState.NotifyBlockAdded(block)

	blockAddedNotification := appmessage.NewBlockAddedNotificationMessage(block.MsgBlock())

	err := m.notificationManager.NotifyBlockAdded(blockAddedNotification)
	if err != nil {
		return err
	}

	err = m.notificationManager.NotifyTransactionAdded(block.Transactions())
	if err != nil {
		return err
	}

	return nil
}

// NotifyUTXOOfAddressChanged notifies the manager that a associated utxo set with address was changed
func (m *Manager) NotifyUTXOOfAddressChanged(addresses []string) error {
	notification := appmessage.NewUTXOOfAddressChangedNotificationMessage(addresses)
	return m.notificationManager.NotifyUTXOOfAddressChanged(notification)
}

// NotifyChainChanged notifies the manager that the DAG's selected parent chain has changed
func (m *Manager) NotifyChainChanged(removedChainBlockHashes []*daghash.Hash, addedChainBlockHashes []*daghash.Hash) error {
	addedChainBlocks, err := m.rpcManager.Context().CollectChainBlocks(addedChainBlockHashes)
	if err != nil {
		return err
	}
	removedChainBlockHashStrings := make([]string, len(removedChainBlockHashes))
	for i, removedChainBlockHash := range removedChainBlockHashes {
		removedChainBlockHashStrings[i] = removedChainBlockHash.String()
	}
	notification := appmessage.NewChainChangedNotificationMessage(removedChainBlockHashStrings, addedChainBlocks)
	return m.notificationManager.NotifyChainChanged(notification)
}

// NotifyFinalityConflict notifies the manager that there's a finality conflict in the DAG
func (m *Manager) NotifyFinalityConflict(violatingBlockHash string) error {
	notification := appmessage.NewFinalityConflictNotificationMessage(violatingBlockHash)
	return m.notificationManager.NotifyFinalityConflict(notification)
}

// NotifyFinalityConflictResolved notifies the manager that a finality conflict in the DAG has been resolved
func (m *Manager) NotifyFinalityConflictResolved(finalityBlockHash string) error {
	notification := appmessage.NewFinalityConflictResolvedNotificationMessage(finalityBlockHash)
	return m.notificationManager.NotifyFinalityConflictResolved(notification)
}
