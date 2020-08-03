package blockwindow

import (
	"github.com/kaspanet/kaspad/consensus/blocknode"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/bigintpool"
	"github.com/pkg/errors"
	"math"
	"math/big"
	"sort"
)

type BlockWindow []*blocknode.BlockNode

// BlueBlockWindow returns a BlockWindow of the given size that contains the
// blues in the past of startindNode, sorted by GHOSTDAG order.
// If the number of blues in the past of startingNode is less then windowSize,
// the window will be padded by genesis blocks to achieve a size of windowSize.
func BlueBlockWindow(startingNode *blocknode.BlockNode, windowSize uint64) BlockWindow {
	window := make(BlockWindow, 0, windowSize)
	currentNode := startingNode
	for uint64(len(window)) < windowSize && currentNode.SelectedParent() != nil {
		if currentNode.SelectedParent() != nil {
			for _, blue := range currentNode.Blues() {
				window = append(window, blue)
				if uint64(len(window)) == windowSize {
					break
				}
			}
			currentNode = currentNode.SelectedParent()
		}
	}

	if uint64(len(window)) < windowSize {
		genesis := currentNode
		for uint64(len(window)) < windowSize {
			window = append(window, genesis)
		}
	}

	return window
}

func (bw BlockWindow) MinMaxTimestamps() (min, max int64) {
	min = math.MaxInt64
	max = 0
	for _, node := range bw {
		if node.Timestamp() < min {
			min = node.Timestamp()
		}
		if node.Timestamp() > max {
			max = node.Timestamp()
		}
	}
	return
}

func (bw BlockWindow) AverageTarget(averageTarget *big.Int) {
	averageTarget.SetInt64(0)

	target := bigintpool.Acquire(0)
	defer bigintpool.Release(target)
	for _, node := range bw {
		util.CompactToBigWithDestination(node.Bits(), target)
		averageTarget.Add(averageTarget, target)
	}

	windowLen := bigintpool.Acquire(int64(len(bw)))
	defer bigintpool.Release(windowLen)
	averageTarget.Div(averageTarget, windowLen)
}

func (bw BlockWindow) MedianTimestamp() (int64, error) {
	if len(bw) == 0 {
		return 0, errors.New("Cannot calculate median timestamp for an empty block window")
	}
	timestamps := make([]int64, len(bw))
	for i, node := range bw {
		timestamps[i] = node.Timestamp()
	}
	sort.Sort(timeSorter(timestamps))
	return timestamps[len(timestamps)/2], nil
}
