package selectedtip

import (
	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/domain/blockdag"
	"github.com/kaspanet/kaspad/domain/dagconfig"
	"github.com/kaspanet/kaspad/infrastructure/db/dbaccess"
	"github.com/kaspanet/kaspad/infrastructure/network/netadapter/router"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleRequestSelectedTip(t *testing.T) {
	tempDir := os.TempDir()

	dbPath := filepath.Join(tempDir, "TestHandleRequestSelectedTip")
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
		DAGParams:  &dagconfig.SimnetParams,
		TimeSource: blockdag.NewTimeSource(),
	})
	if err != nil {
		t.Fatalf("HandleRequestSelectedTip: %s", err)
	}

	ctx := &mockContext{dag: dag}
	incomingRoute := router.NewRoute()
	outgoingRoute := router.NewRoute()
	err = incomingRoute.Enqueue(appmessage.NewMsgRequestSelectedTip())
	if err != nil {
		t.Fatalf("HandleRequestSelectedTip: %s", err)
	}

	go func() {
		err = HandleRequestSelectedTip(ctx, incomingRoute, outgoingRoute)
		if err != nil {
			t.Fatalf("HandleRequestSelectedTip: %s", err)
		}
	}()

	_, err = outgoingRoute.DequeueWithTimeout(dequeueTimeout)
	if err != nil {
		t.Fatalf("HandleRequestSelectedTip: %s", err)
	}
}
