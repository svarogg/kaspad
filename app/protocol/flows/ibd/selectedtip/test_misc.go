package selectedtip

import (
	"time"

	"github.com/kaspanet/kaspad/domain/blockdag"
)

const (
	dequeueTimeout = 2 * time.Second
	callTimes      = 5
)

type mockContext struct {
	dag *blockdag.BlockDAG
}

func (mc *mockContext) DAG() *blockdag.BlockDAG {
	return mc.dag
}

func (mc *mockContext) StartIBDIfRequired() {

}
