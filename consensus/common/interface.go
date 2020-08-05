package common

import "github.com/kaspanet/kaspad/util/mstime"

type BlockLocatorManager interface {
}

type BlockNodeStore interface {
}

type CoinbaseManager interface {
}

type DelayedBlockManager interface {
}

type DifficultyManager interface {
}

type FinalityManager interface {
}

type GHOSTDAGManager interface {
}

type MultiSetManager interface {
}

type NotificationManager interface {
}

type OrphanedBlockManager interface {
}

type PastMedianTimeManager interface {
}

type ReachabilityTree interface {
}

type SequenceLockCalculator interface {
}

type SubnetworkManager interface {
}

type SyncRateManager interface {
}

type TimeSource interface {
	Now() mstime.Time
}

type UTXODiffStore interface {
}

type VirtualBlock interface {
}
