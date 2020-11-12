package rpc

import (
	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/app/protocol"
	"github.com/kaspanet/kaspad/app/rpc/rpccontext"
	"github.com/kaspanet/kaspad/domain/blockdag"
	"github.com/kaspanet/kaspad/domain/blockdag/indexers"
	"github.com/kaspanet/kaspad/domain/mempool"
	"github.com/kaspanet/kaspad/domain/mining"
	"github.com/kaspanet/kaspad/infrastructure/config"
	"github.com/kaspanet/kaspad/infrastructure/network/addressmanager"
	"github.com/kaspanet/kaspad/infrastructure/network/connmanager"
	"github.com/kaspanet/kaspad/infrastructure/network/netadapter"
	"github.com/kaspanet/kaspad/infrastructure/network/netadapter/router"
)

// Handler is the interface for the RPC handlers
type Handler interface {
	Execute(router *router.Router, request appmessage.Message) (appmessage.Message, error)
}

type handlerInContext struct {
	context *rpccontext.Context
	handler handlerFunc
}

func (hic *handlerInContext) Execute(router *router.Router, request appmessage.Message) (appmessage.Message, error) {
	return hic.handler(hic.context, router, request)
}

// Manager is an RPC manager
type Manager struct {
	context  *rpccontext.Context
	handlers map[appmessage.MessageCommand]Handler
}

// NewManager creates a new RPC Manager
func NewManager(
	cfg *config.Config,
	netAdapter *netadapter.NetAdapter,
	dag *blockdag.BlockDAG,
	protocolManager *protocol.Manager,
	connectionManager *connmanager.ConnectionManager,
	blockTemplateGenerator *mining.BlkTmplGenerator,
	mempool *mempool.TxPool,
	addressManager *addressmanager.AddressManager,
	acceptanceIndex *indexers.AcceptanceIndex,
	shutDownChan chan<- struct{}) *Manager {

	manager := Manager{
		context: rpccontext.NewContext(
			cfg,
			netAdapter,
			dag,
			protocolManager,
			connectionManager,
			blockTemplateGenerator,
			mempool,
			addressManager,
			acceptanceIndex,
			shutDownChan,
		),
	}

	for command, handler := range rpcHandlers {
		handlerInContext := handlerInContext{
			context: manager.context,
			handler: handler,
		}
		manager.RegisterHandler(command, &handlerInContext)
	}

	netAdapter.SetRPCRouterInitializer(manager.routerInitializer)
	return &manager
}

// NotifyTransactionAddedToMempool notifies the manager that a transaction has been added to the mempool
func (m *Manager) NotifyTransactionAddedToMempool() {
	m.context.BlockTemplateState.NotifyMempoolTx()
}

// Context returns RPC context
func (m *Manager) Context() *rpccontext.Context {
	return m.context
}
