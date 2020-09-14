package ibd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaspanet/kaspad/util/daghash"
	"github.com/pkg/errors"

	"github.com/kaspanet/kaspad/app/appmessage"
	peerpkg "github.com/kaspanet/kaspad/app/protocol/peer"
	"github.com/kaspanet/kaspad/domain/blockdag"
	"github.com/kaspanet/kaspad/domain/dagconfig"
	"github.com/kaspanet/kaspad/infrastructure/db/dbaccess"
	"github.com/kaspanet/kaspad/infrastructure/network/netadapter"
	"github.com/kaspanet/kaspad/infrastructure/network/netadapter/router"
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
	peer := peerpkg.New(&netadapter.NetConnection{})

	t.Run("SimpleCall", func(t *testing.T) {
		incomingRoute := router.NewRoute()
		outgoingRoute := router.NewRoute()

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

		_, err = outgoingRoute.DequeueWithTimeout(dequeueTimeout)
		if err != nil {
			t.Fatalf("HandleIBD: %s", err)
		}
	})

	t.Run("CheckOutgoingMessageType", func(t *testing.T) {
		incomingRoute := router.NewRoute()
		outgoingRoute := router.NewRoute()

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

		for {
			msg, err := outgoingRoute.DequeueWithTimeout(dequeueTimeout)
			switch {
			case errors.Cause(err) == router.ErrTimeout:
				return
			case err != nil:
				t.Fatalf("HandleIBD: %s", err)
			}

			switch msg.(type) {
			case *appmessage.MsgRequestIBDBlocks:
				continue
			case *appmessage.MsgRequestNextIBDBlocks:
				continue
			case *appmessage.MsgRequestBlockLocator:
				continue
			default:
				t.Fatalf("HandleIBD: expected %s, %s or %s, got %s",
					appmessage.CmdRequestIBDBlocks, appmessage.CmdRequestNextIBDBlocks,
					appmessage.CmdRequestBlockLocator, msg.Command())
			}
		}
	})

	t.Run("CallOnClosedRoute", func(t *testing.T) {
		outgoingRoute := router.NewRoute()
		closedRoute := router.NewRoute()
		closedRoute.Close()

		go func() {
			err := HandleIBD(ctx, closedRoute, outgoingRoute, peer)
			if err == nil {
				t.Fatalf("HandleIBD: expected error, got nil")
			}
		}()

		peer.StartIBD()
		outgoingRoute.DequeueWithTimeout(dequeueTimeout)
	})

	t.Run("CallOnNilRoutes", func(t *testing.T) {
		go func() {
			err := HandleIBD(ctx, nil, nil, peer)
			if err == nil {
				t.Fatalf("HandleIBD: expected error, got nil")
			}
		}()

		peer.StartIBD()
		// just to wait until HandleIBD return error
		<-time.After(dequeueTimeout)
	})

	t.Run("CallOnNilPeer", func(t *testing.T) {
		incomingRoute := router.NewRoute()
		outgoingRoute := router.NewRoute()

		err := HandleIBD(ctx, incomingRoute, outgoingRoute, nil)
		if err == nil {
			t.Fatalf("HandleIBD: expected error, got nil")
		}
	})

	t.Run("CallWithEnqueuedInvalidMessage", func(t *testing.T) {
		incomingRoute := router.NewRoute()
		outgoingRoute := router.NewRoute()

		go func() {
			err := HandleIBD(ctx, incomingRoute, outgoingRoute, peer)
			if err == nil {
				t.Fatalf("HandleIBD: expected error, got nil")
			}
		}()

		err = incomingRoute.Enqueue(appmessage.NewMsgPing(1))
		if err != nil {
			t.Fatalf("HandleIBD: %s", err)
		}

		peer.StartIBD()
		outgoingRoute.DequeueWithTimeout(dequeueTimeout)
	})
}
