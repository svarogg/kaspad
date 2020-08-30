package ibd

import (
	"github.com/kaspanet/kaspad/domain/blockdag"
	"time"
)

const (
	dequeueTimeout = 2 * time.Second
)

type mockContext struct {
	dag *blockdag.BlockDAG
}

func (mc *mockContext) DAG() *blockdag.BlockDAG {
	return mc.dag
}

func (mc *mockContext) StartIBDIfRequired() {

}
