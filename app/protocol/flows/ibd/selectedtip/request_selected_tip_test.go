package selectedtip

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
)

func TestRequestSelectedTip(t *testing.T) {
	tempDir := os.TempDir()

	dbPath := filepath.Join(tempDir, "TestRequestSelectedTip")
	_ = os.RemoveAll(dbPath)
	databaseContext, err := dbaccess.New(dbPath)
	if err != nil {
		t.Fatalf("error creating db: %s", err)
	}
	defer func() {
		databaseContext.Close()
		os.RemoveAll(dbPath)
	}()

	dag, err := blockdag.New(&blockdag.Config{
		DatabaseContext: databaseContext,
		DAGParams:       &dagconfig.SimnetParams,
		TimeSource:      blockdag.NewTimeSource(),
	})
	if err != nil {
		t.Fatalf("RequestSelectedTip: %s", err)
	}

	ctx := &mockContext{dag: dag}
	incomingRoute := router.NewRoute()
	outgoingRoute := router.NewRoute()
	peer := peerpkg.New(&netadapter.NetConnection{})

	go func() {
		err = RequestSelectedTip(ctx, incomingRoute, outgoingRoute, peer)
		if err != nil {
			t.Fatalf("RequestSelectedTip: %s", err)
		}
	}()

	peer.RequestSelectedTipIfRequired()
	_, err = outgoingRoute.Dequeue()
	if err != nil {
		t.Fatalf("RequestSelectedTip: %s", err)
	}

	err = incomingRoute.Enqueue(appmessage.NewMsgSelectedTip(nil))
	if err != nil {
		t.Fatalf("RequestSelectedTip: %s", err)
	}
}
