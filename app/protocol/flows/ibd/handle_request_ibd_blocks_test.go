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

func TestHandleRequestIBDBlocks(t *testing.T) {
	tempDir := os.TempDir()

	dbPath := filepath.Join(tempDir, "TestHandleRequestIBDBlocks")
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
		t.Fatalf("HandleRequestIBDBlocks: %s", err)
	}

	firstBlock := blockdag.PrepareAndProcessBlockForTest(t, dag, []*daghash.Hash{dagConfig.DAGParams.GenesisHash}, nil)
	secondBlock := blockdag.PrepareAndProcessBlockForTest(t, dag, []*daghash.Hash{firstBlock.BlockHash()}, nil)
	_ = secondBlock
	ctx := &mockContext{dag: dag}
	incomingRoute := router.NewRoute()
	outgoingRoute := router.NewRoute()

	t.Run("SimpleCall", func(t *testing.T) {
		go func() {
			err := HandleRequestIBDBlocks(ctx, incomingRoute, outgoingRoute)
			if err != nil {
				t.Fatalf("HandleRequestIBDBlocks: %s", err)
			}
		}()

		err = incomingRoute.Enqueue(appmessage.NewMsgRequstIBDBlocks(firstBlock.BlockHash(), secondBlock.BlockHash()))
		if err != nil {
			t.Fatalf("HandleRequestIBDBlocks: %s", err)
		}

		for {
			msg, err := outgoingRoute.Dequeue()
			if err != nil {
				t.Fatalf("HandleRequestIBDBlocks: %s", err)
			}

			_, ok := msg.(*appmessage.MsgDoneIBDBlocks)
			if ok {
				return
			}
		}
	})

	t.Run("CheckOutgoingMessageType", func(t *testing.T) {
		go func() {
			err := HandleRequestIBDBlocks(ctx, incomingRoute, outgoingRoute)
			if err != nil {
				t.Fatalf("HandleRequestIBDBlocks: %s", err)
			}
		}()

		err = incomingRoute.Enqueue(appmessage.NewMsgRequstIBDBlocks(firstBlock.BlockHash(), secondBlock.BlockHash()))
		if err != nil {
			t.Fatalf("HandleRequestIBDBlocks: %s", err)
		}

		for {
			msg, err := outgoingRoute.Dequeue()
			if err != nil {
				t.Fatalf("HandleRequestIBDBlocks: %s", err)
			}

			switch msg.(type) {
			case *appmessage.MsgDoneIBDBlocks:
				return
			case *appmessage.MsgIBDBlock:
				continue
			default:
				t.Fatalf("HandleRequestIBDBlocks: expected %s or %s, got %s",
					appmessage.CmdDoneIBDBlocks, appmessage.CmdIBDBlock, msg.Command())
			}
		}
	})

	t.Run("CallMultipleTimes", func(t *testing.T) {
		go func() {
			err := HandleRequestIBDBlocks(ctx, incomingRoute, outgoingRoute)
			if err != nil {
				t.Fatalf("HandleRequestIBDBlocks: %s", err)
			}
		}()

		for i := 0; i < callTimes; i++ {
			err = incomingRoute.Enqueue(appmessage.NewMsgRequstIBDBlocks(firstBlock.BlockHash(), secondBlock.BlockHash()))
			if err != nil {
				t.Fatalf("HandleRequestIBDBlocks: %s", err)
			}

			for {
				msg, err := outgoingRoute.DequeueWithTimeout(dequeueTimeout)
				if err != nil {
					t.Fatalf("HandleRequestIBDBlocks: %s", err)
				}

				_, ok := msg.(*appmessage.MsgDoneIBDBlocks)
				if ok {
					break
				}
			}
		}
	})

	t.Run("CallOnClosedRoute", func(t *testing.T) {
		closedRoute := router.NewRoute()
		closedRoute.Close()
		err := HandleRequestIBDBlocks(ctx, closedRoute, outgoingRoute)
		if err == nil {
			t.Fatalf("HandleRequestIBDBlocks: expected error, got nil")
		}
	})

	t.Run("CallOnNilRoutes", func(t *testing.T) {
		err := HandleRequestIBDBlocks(ctx, nil, nil)
		if err == nil {
			t.Fatalf("HandleRequestIBDBlocks: expected error, got nil")
		}
	})

	t.Run("CallWithEnqueuedInvalidMessage", func(t *testing.T) {
		err = incomingRoute.Enqueue(appmessage.NewMsgPing(1))
		if err != nil {
			t.Fatalf("HandleRequestIBDBlocks: %s", err)
		}

		err := HandleRequestIBDBlocks(ctx, incomingRoute, outgoingRoute)
		if err == nil {
			t.Fatalf("HandleRequestIBDBlocks: expected error, got nil")
		}
	})

}
