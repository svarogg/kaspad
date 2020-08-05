package consensus

import (
	"github.com/kaspanet/kaspad/consensus/common"
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

	BlockLocatorManager    common.BlockLocatorManager
	BlockNodeStore         common.BlockNodeStore
	CoinbaseManager        common.CoinbaseManager
	DelayedBlockManager    common.DelayedBlockManager
	DifficultyManager      common.DifficultyManager
	FinalityManager        common.FinalityManager
	GHOSTDAGManager        common.GHOSTDAGManager
	MultiSetManager        common.MultiSetManager
	NotificationManager    common.NotificationManager
	OrphanedBlockManager   common.OrphanedBlockManager
	PastMedianTimeManager  common.PastMedianTimeManager
	ReachabilityTree       common.ReachabilityTree
	SequenceLockCalculator common.SequenceLockCalculator
	SyncRateManager        common.SyncRateManager
	TimeSource             common.TimeSource
	UTXODiffStore          common.UTXODiffStore
	VirtualBlock           common.VirtualBlock
}
