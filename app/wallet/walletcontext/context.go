package walletcontext

import (
	"github.com/kaspanet/kaspad/app/wallet/walletnotification"
	"github.com/kaspanet/kaspad/domain/blockdag/indexers"
	routerpkg "github.com/kaspanet/kaspad/infrastructure/network/netadapter/router"
)

// Context represents the Wallet handler context
type Context interface {
	AcceptanceIndex() *indexers.AcceptanceIndex
	Listener(router *routerpkg.Router) (*walletnotification.Listener, error)
}
