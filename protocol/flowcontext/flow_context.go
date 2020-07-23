package flowcontext

import (
	"github.com/kaspanet/go-secp256k1"
	"github.com/kaspanet/kaspad/addrmgr"
	"github.com/kaspanet/kaspad/blockdag"
	"github.com/kaspanet/kaspad/config"
	"github.com/kaspanet/kaspad/connmanager"
	"github.com/kaspanet/kaspad/mempool"
	"github.com/kaspanet/kaspad/netadapter"
	"github.com/kaspanet/kaspad/protocol/flows/blockrelay"
	"github.com/kaspanet/kaspad/protocol/flows/relaytransactions"
	peerpkg "github.com/kaspanet/kaspad/protocol/peer"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/daghash"
	"sync"
	"time"
)

// FlowContext holds state that is relevant to more than one flow or one peer, and allows communication between
// different flows that can be associated to different peers.
type FlowContext struct {
	cfg               *config.Config
	netAdapter        *netadapter.NetAdapter
	txPool            *mempool.TxPool
	dag               *blockdag.BlockDAG
	addressManager    *addrmgr.AddrManager
	connectionManager *connmanager.ConnectionManager

	transactionsToRebroadcastLock sync.Mutex
	transactionsToRebroadcast     map[daghash.TxID]*util.Tx
	lastRebroadcastTime           time.Time
	sharedRequestedTransactions   *relaytransactions.SharedRequestedTransactions

	sharedRequestedBlocks *blockrelay.SharedRequestedBlocks

	isInIBD       uint32
	startIBDMutex sync.Mutex
	ibdPeer       *peerpkg.Peer

	peers      map[string]*peerpkg.Peer
	peersMutex sync.RWMutex

	selfPrivateKey *secp256k1.PrivateKey
	selfPublicKey  *secp256k1.SchnorrPublicKey
}

// New returns a new instance of FlowContext.
func New(cfg *config.Config, dag *blockdag.BlockDAG, addressManager *addrmgr.AddrManager,
	txPool *mempool.TxPool, netAdapter *netadapter.NetAdapter,
	connectionManager *connmanager.ConnectionManager, peerPrivateKey *secp256k1.PrivateKey,
	peerPublicKey *secp256k1.SchnorrPublicKey) *FlowContext {

	return &FlowContext{
		cfg:                         cfg,
		netAdapter:                  netAdapter,
		dag:                         dag,
		addressManager:              addressManager,
		connectionManager:           connectionManager,
		txPool:                      txPool,
		selfPrivateKey:              peerPrivateKey,
		selfPublicKey:               peerPublicKey,
		sharedRequestedTransactions: relaytransactions.NewSharedRequestedTransactions(),
		sharedRequestedBlocks:       blockrelay.NewSharedRequestedBlocks(),
		peers:                       make(map[string]*peerpkg.Peer),
		transactionsToRebroadcast:   make(map[daghash.TxID]*util.Tx),
	}
}
