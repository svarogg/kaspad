package consensus

import (
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/dbaccess"
	"github.com/kaspanet/kaspad/util/subnetworkid"
)

type Config struct {
	DAGParams       *dagconfig.Params
	SubnetworkID    *subnetworkid.SubnetworkID
	DatabaseContext *dbaccess.DatabaseContext

	BlockLocatorManager    BlockLocatorManager
	BlockNodeStore         BlockNodeStore
	CoinbaseManager        CoinbaseManager
	DelayedBlockManager    DelayedBlockManager
	DifficultyManager      DifficultyManager
	FinalityManager        FinalityManager
	GHOSTDAGManager        GHOSTDAGManager
	IndexManager           IndexManager
	MultiSetManager        MultiSetManager
	NotificationManager    NotificationManager
	PastMedianTimeManager  PastMedianTimeManager
	ReachabilityTree       ReachabilityTree
	SequenceLockCalculator SequenceLockCalculator
	SigCache               SigCache
	SubnetworkManager      SubnetworkManager
	TimeSource             TimeSource
	UTXOManager            UTXOManager
	UTXODiffStore          UTXODiffStore
	ValidationManager      ValidationManager
	VirtualBlock           VirtualBlock
}
