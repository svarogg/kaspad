package consensus

import (
	"github.com/kaspanet/kaspad/config"
	"github.com/kaspanet/kaspad/consensus/blocklocator"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/coinbase"
	"github.com/kaspanet/kaspad/consensus/delayedblocks"
	"github.com/kaspanet/kaspad/consensus/difficulty"
	"github.com/kaspanet/kaspad/consensus/finality"
	"github.com/kaspanet/kaspad/consensus/ghostdag"
	"github.com/kaspanet/kaspad/consensus/multiset"
	"github.com/kaspanet/kaspad/consensus/notifications"
	"github.com/kaspanet/kaspad/consensus/orphanedblocks"
	"github.com/kaspanet/kaspad/consensus/pastmediantime"
	"github.com/kaspanet/kaspad/consensus/reachability"
	"github.com/kaspanet/kaspad/consensus/sequencelock"
	"github.com/kaspanet/kaspad/consensus/syncrate"
	"github.com/kaspanet/kaspad/consensus/timesource"
	"github.com/kaspanet/kaspad/consensus/utxodiffstore"
	"github.com/kaspanet/kaspad/consensus/virtualblock"
	"github.com/kaspanet/kaspad/dbaccess"
	"github.com/kaspanet/kaspad/sigcache"
)

type Consensus struct {
	config *Config
}

func New(config *config.Config, dbContext *dbaccess.DatabaseContext, sigCache *sigcache.SigCache) *Consensus {
	params := config.NetParams()
	subnetworkID := config.SubnetworkID

	timeSource := timesource.New()
	blockNodeStore := blocknode.NewStore(params)
	delayedBlockManager := delayedblocks.New(timeSource)
	notificationManager := notifications.NewManager()
	coinbaseManager := coinbase.NewManager(dbContext, params)
	pastMedianTimeManager := pastmediantime.NewManager(params)
	syncRateManager := syncrate.NewManager(params)
	multiSetManager := multiset.NewManager()
	reachabilityTree := reachability.NewReachabilityTree(blockNodeStore, params)
	ghostdagManager := ghostdag.NewManager(reachabilityTree, params, timeSource)
	virtualBlock := virtualblock.New(ghostdagManager, params, blockNodeStore, nil)
	blockLocatorManager := blocklocator.NewManager(blockNodeStore, reachabilityTree, params)
	utxoDiffStore := utxodiffstore.New(dbContext, blockNodeStore, virtualBlock)
	difficultyManager := difficulty.NewManager(params, virtualBlock)
	sequenceLockCalculator := sequencelock.NewCalculator(virtualBlock, pastMedianTimeManager)
	orphanedBlockManager := orphanedblocks.NewManager(blockNodeStore)
	finalityManager := finality.NewManager(params, blockNodeStore, virtualBlock, reachabilityTree, utxoDiffStore, dbContext)

	consensusConfig := &Config{
		DAGParams:       params,
		SubnetworkID:    subnetworkID,
		DatabaseContext: dbContext,
		SigCache:        sigCache,

		BlockLocatorManager:    blockLocatorManager,
		BlockNodeStore:         blockNodeStore,
		CoinbaseManager:        coinbaseManager,
		DelayedBlockManager:    delayedBlockManager,
		DifficultyManager:      difficultyManager,
		FinalityManager:        finalityManager,
		GHOSTDAGManager:        ghostdagManager,
		MultiSetManager:        multiSetManager,
		NotificationManager:    notificationManager,
		OrphanedBlockManager:   orphanedBlockManager,
		PastMedianTimeManager:  pastMedianTimeManager,
		ReachabilityTree:       reachabilityTree,
		SequenceLockCalculator: sequenceLockCalculator,
		SyncRateManager:        syncRateManager,
		TimeSource:             timeSource,
		UTXODiffStore:          utxoDiffStore,
		VirtualBlock:           virtualBlock,
	}

	return &Consensus{config: consensusConfig}
}
