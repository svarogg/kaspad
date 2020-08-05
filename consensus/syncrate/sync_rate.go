package syncrate

import (
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/util/mstime"
	"time"
)

const syncRateWindowDuration = 15 * time.Minute

type SyncRateManager struct {
	params *dagconfig.Params

	recentBlockProcessingTimestamps []mstime.Time
	startTime                       mstime.Time
}

func NewManager(params *dagconfig.Params) *SyncRateManager {
	return &SyncRateManager{
		params:                          params,
		recentBlockProcessingTimestamps: nil,
		startTime:                       mstime.Now(),
	}
}

// AddBlockProcessingTimestamp adds the last block processing timestamp in order to measure the recent sync rate.
//
// This function MUST be called with the DAG state lock held (for writes).
func (sr *SyncRateManager) AddBlockProcessingTimestamp() {
	now := mstime.Now()
	sr.recentBlockProcessingTimestamps = append(sr.recentBlockProcessingTimestamps, now)
	sr.removeNonRecentTimestampsFromRecentBlockProcessingTimestamps()
}

// removeNonRecentTimestampsFromRecentBlockProcessingTimestamps removes timestamps older than syncRateWindowDuration
// from dag.recentBlockProcessingTimestamps
//
// This function MUST be called with the DAG state lock held (for writes).
func (sr *SyncRateManager) removeNonRecentTimestampsFromRecentBlockProcessingTimestamps() {
	sr.recentBlockProcessingTimestamps = sr.recentBlockProcessingTimestampsRelevantWindow()
}

func (sr *SyncRateManager) recentBlockProcessingTimestampsRelevantWindow() []mstime.Time {
	minTime := mstime.Now().Add(-syncRateWindowDuration)
	windowStartIndex := len(sr.recentBlockProcessingTimestamps)
	for i, processTime := range sr.recentBlockProcessingTimestamps {
		if processTime.After(minTime) {
			windowStartIndex = i
			break
		}
	}
	return sr.recentBlockProcessingTimestamps[windowStartIndex:]
}

// syncRate returns the rate of processed
// blocks in the last syncRateWindowDuration
// duration.
func (sr *SyncRateManager) syncRate() float64 {
	return float64(len(sr.recentBlockProcessingTimestampsRelevantWindow())) / syncRateWindowDuration.Seconds()
}

// IsSyncRateBelowThreshold checks whether the sync rate
// is below the expected threshold.
func (sr *SyncRateManager) IsSyncRateBelowThreshold(maxDeviation float64) bool {
	if sr.uptime() < syncRateWindowDuration {
		return false
	}

	return sr.syncRate() < 1/sr.params.TargetTimePerBlock.Seconds()*maxDeviation
}

func (sr *SyncRateManager) uptime() time.Duration {
	return mstime.Now().Sub(sr.startTime)
}
