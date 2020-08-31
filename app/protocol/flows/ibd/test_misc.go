package ibd

import (
	"time"

	"github.com/kaspanet/kaspad/domain/blockdag"
	"github.com/kaspanet/kaspad/util"
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

func (mc *mockContext) OnNewBlock(block *util.Block) error {
	return nil
}

func (mc *mockContext) StartIBDIfRequired() {

}

func (mc *mockContext) FinishIBD() {

}
