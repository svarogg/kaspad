package ibd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaspanet/kaspad/app/appmessage"
	peerpkg "github.com/kaspanet/kaspad/app/protocol/peer"
	"github.com/kaspanet/kaspad/domain/blockdag"
	"github.com/kaspanet/kaspad/domain/dagconfig"
	"github.com/kaspanet/kaspad/infrastructure/db/dbaccess"
	"github.com/kaspanet/kaspad/infrastructure/network/netadapter"
	"github.com/kaspanet/kaspad/infrastructure/network/netadapter/router"
	"github.com/kaspanet/kaspad/util/daghash"
)

func TestHandleIBD(t *testing.T) {
	tempDir := os.TempDir()

	dbPath := filepath.Join(tempDir, "TestHandleIBD")
	_ = os.RemoveAll(dbPath)
	databaseContext, err := dbaccess.New(dbPath)
	if err != nil {
		t.Fatalf("error creating db: %s", err)
	}
	defer func() {
		databaseContext.Close()
		os.RemoveAll(dbPath)
	}()

	dagConfig := &blockdag.Config{
		DatabaseContext: databaseContext,
		DAGParams:       &dagconfig.SimnetParams,
		TimeSource:      blockdag.NewTimeSource(),
	}

	dag, err := blockdag.New(dagConfig)
	if err != nil {
		t.Fatalf("HandleIBD: %s", err)
	}

	ctx := &mockContext{dag: dag}
	incomingRoute := router.NewRoute()
	outgoingRoute := router.NewRoute()
	peer := peerpkg.New(&netadapter.NetConnection{})

	go func() {
		err := HandleIBD(ctx, incomingRoute, outgoingRoute, peer)
		if err != nil {
			t.Fatalf("HandleIBD: %s", err)
		}
	}()

	peer.StartIBD()
	block := blockdag.PrepareAndProcessBlockForTest(t, dag, []*daghash.Hash{dagConfig.DAGParams.GenesisHash}, nil)
	err = incomingRoute.Enqueue(appmessage.NewMsgBlockLocator([]*daghash.Hash{block.BlockHash()}))
	if err != nil {
		t.Fatalf("HandleIBD: %s", err)
	}

	_, err = outgoingRoute.Dequeue()
	if err != nil {
		t.Fatalf("HandleIBD: %s", err)
	}
}
