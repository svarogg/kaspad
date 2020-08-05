package consensus

import (
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/dbaccess"
	"github.com/kaspanet/kaspad/sigcache"
	"github.com/kaspanet/kaspad/util/subnetworkid"
)

type Config struct {
	DAGParams       *dagconfig.Params
	SubnetworkID    *subnetworkid.SubnetworkID
	DatabaseContext *dbaccess.DatabaseContext
	SigCache        *sigcache.SigCache

	BlockLocatorManager    BlockLocatorManager
	BlockNodeStore         BlockNodeStore
	CoinbaseManager        CoinbaseManager
	DelayedBlockManager    DelayedBlockManager
	DifficultyManager      DifficultyManager
	FinalityManager        FinalityManager
	GHOSTDAGManager        GHOSTDAGManager
	MultiSetManager        MultiSetManager
	NotificationManager    NotificationManager
	OrphanedBlockManager   OrphanedBlockManager
	PastMedianTimeManager  PastMedianTimeManager
	ReachabilityTree       ReachabilityTree
	SequenceLockCalculator SequenceLockCalculator
	SyncRateManager        SyncRateManager
	TimeSource             TimeSource
	UTXODiffStore          UTXODiffStore
	VirtualBlock           VirtualBlock
}
