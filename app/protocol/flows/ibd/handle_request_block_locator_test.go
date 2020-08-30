package ibd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/domain/blockdag"
	"github.com/kaspanet/kaspad/domain/dagconfig"
	"github.com/kaspanet/kaspad/infrastructure/db/dbaccess"
	"github.com/kaspanet/kaspad/infrastructure/network/netadapter/router"
	"github.com/kaspanet/kaspad/util/daghash"
)

func TestHandleRequestBlockLocator(t *testing.T) {
	tempDir := os.TempDir()

	dbPath := filepath.Join(tempDir, "TestHandleRequestBlockLocator")
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
		t.Fatalf("HandleRequestBlockLocator: %s", err)
	}

	firstBlock := blockdag.PrepareAndProcessBlockForTest(t, dag, []*daghash.Hash{dagConfig.DAGParams.GenesisHash}, nil)
	secondBlock := blockdag.PrepareAndProcessBlockForTest(t, dag, []*daghash.Hash{firstBlock.BlockHash()}, nil)

	ctx := &mockContext{dag: dag}
	incomingRoute := router.NewRoute()
	outgoingRoute := router.NewRoute()
	err = incomingRoute.Enqueue(appmessage.NewMsgRequestBlockLocator(secondBlock.BlockHash(), firstBlock.BlockHash()))
	if err != nil {
		t.Fatalf("HandleRequestBlockLocator: %s", err)
	}

	go func() {
		err = HandleRequestBlockLocator(ctx, incomingRoute, outgoingRoute)
		if err != nil {
			t.Fatalf("HandleRequestBlockLocator: %s", err)
		}
	}()

	_, err = outgoingRoute.DequeueWithTimeout(dequeueTimeout)
	if err != nil {
		t.Fatalf("HandleRequestBlockLocator: %s", err)
	}
}
