package pastmediantime

import (
	"fmt"
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/consensus/blockwindow"
	"github.com/kaspanet/kaspad/dagconfig"
	"github.com/kaspanet/kaspad/util/mstime"
)

type PastMedianTimeFactory struct {
	params *dagconfig.Params
}

func NewPastMedianTimeFactory(params *dagconfig.Params) *PastMedianTimeFactory {
	return &PastMedianTimeFactory{
		params: params,
	}
}

// PastMedianTime returns the median time of the previous few blocks
// prior to, and including, the block node.
func (pmtf *PastMedianTimeFactory) PastMedianTime(node *blocknode.BlockNode) mstime.Time {
	window := blockwindow.BlueBlockWindow(node, 2*pmtf.params.TimestampDeviationTolerance-1)
	medianTimestamp, err := window.MedianTimestamp()
	if err != nil {
		panic(fmt.Sprintf("blueBlockWindow: %s", err))
	}
	return mstime.UnixMilliseconds(medianTimestamp)
}
