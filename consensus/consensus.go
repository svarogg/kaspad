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
	blockNodeStore := blocknode.NewBlockNodeStore(params)
	delayedBlockManager := delayedblocks.New(timeSource)
	notificationManager := notifications.New()
	coinbaseManager := coinbase.New(dbContext, params)
	pastMedianTimeManager := pastmediantime.NewPastMedianTimeFactory(params)
	syncRateManager := syncrate.NewSyncRate(params)
	multiSetManager := multiset.NewMultiSetManager()
	reachabilityTree := reachability.NewReachabilityTree(blockNodeStore, params)
	ghostdagManager := ghostdag.NewGHOSTDAG(reachabilityTree, params, timeSource)
	virtualBlock := virtualblock.NewVirtualBlock(ghostdagManager, params, blockNodeStore, nil)
	blockLocatorManager := blocklocator.NewBlockLocatorFactory(blockNodeStore, reachabilityTree, params)
	utxoDiffStore := utxodiffstore.NewUTXODiffStore(dbContext, blockNodeStore, virtualBlock)
	difficultyManager := difficulty.NewDifficulty(params, virtualBlock)
	sequenceLockCalculator := sequencelock.NewSequenceLockCalculator(virtualBlock, pastMedianTimeManager)
	orphanedBlockManager := orphanedblocks.New(blockNodeStore)
	finalityManager := finality.New(params, blockNodeStore, virtualBlock, reachabilityTree, utxoDiffStore, dbContext)

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
