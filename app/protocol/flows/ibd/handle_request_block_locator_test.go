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

	t.Run("SimpleCall", func(t *testing.T) {
		go func() {
			err = HandleRequestBlockLocator(ctx, incomingRoute, outgoingRoute)
			if err != nil {
				t.Fatalf("HandleRequestBlockLocator: %s", err)
			}
		}()

		err = incomingRoute.Enqueue(appmessage.NewMsgRequestBlockLocator(secondBlock.BlockHash(), firstBlock.BlockHash()))
		if err != nil {
			t.Fatalf("HandleRequestBlockLocator: %s", err)
		}

		_, err = outgoingRoute.DequeueWithTimeout(dequeueTimeout)
		if err != nil {
			t.Fatalf("HandleRequestBlockLocator: %s", err)
		}
	})

	t.Run("CheckOutgoingMessageType", func(t *testing.T) {
		go func() {
			err = HandleRequestBlockLocator(ctx, incomingRoute, outgoingRoute)
			if err != nil {
				t.Fatalf("HandleRequestBlockLocator: %s", err)
			}
		}()

		err = incomingRoute.Enqueue(appmessage.NewMsgRequestBlockLocator(secondBlock.BlockHash(), firstBlock.BlockHash()))
		if err != nil {
			t.Fatalf("HandleRequestBlockLocator: %s", err)
		}

		msg, err := outgoingRoute.DequeueWithTimeout(dequeueTimeout)
		if err != nil {
			t.Fatalf("HandleRequestBlockLocator: %s", err)
		}

		if _, ok := msg.(*appmessage.MsgBlockLocator); !ok {
			t.Fatalf("HandleRequestBlockLocator: expected %s, got %s", appmessage.CmdBlockLocator, msg.Command())
		}
	})

	t.Run("CallMultipleTimes", func(t *testing.T) {
		go func() {
			err = HandleRequestBlockLocator(ctx, incomingRoute, outgoingRoute)
			if err != nil {
				t.Fatalf("HandleRequestBlockLocator: %s", err)
			}
		}()

		for i := 0; i < callTimes; i++ {
			err = incomingRoute.Enqueue(appmessage.NewMsgRequestBlockLocator(secondBlock.BlockHash(), firstBlock.BlockHash()))
			if err != nil {
				t.Fatalf("HandleRequestBlockLocator: %s", err)
			}

			_, err = outgoingRoute.DequeueWithTimeout(dequeueTimeout)
			if err != nil {
				t.Fatalf("HandleRequestBlockLocator: %s", err)
			}
		}
	})

	t.Run("CallOnClosedRoute", func(t *testing.T) {
		closedRoute := router.NewRoute()
		closedRoute.Close()
		err = HandleRequestBlockLocator(ctx, closedRoute, outgoingRoute)
		if err == nil {
			t.Fatal("HandleRequestBlockLocator: expected error, got nil")
		}
	})

	t.Run("CallOnNilRoutes", func(t *testing.T) {
		err = HandleRequestBlockLocator(ctx, nil, nil)
		if err == nil {
			t.Fatal("HandleRequestBlockLocator: expected error, got nil")
		}
	})

	t.Run("CallWithEnqueuedInvalidMessage", func(t *testing.T) {
		go func() {
			err = HandleRequestBlockLocator(ctx, incomingRoute, outgoingRoute)
			if err == nil {
				t.Fatal("HandleRequestBlockLocator: expected error, got nil")
			}
		}()

		err = incomingRoute.Enqueue(appmessage.NewMsgPing(1))
		if err != nil {
			t.Fatalf("HandleRequestBlockLocator: %s", err)
		}

		_, err = outgoingRoute.DequeueWithTimeout(dequeueTimeout)
		if err != nil {
			t.Fatalf("HandleRequestBlockLocator: %s", err)
		}
	})

}
