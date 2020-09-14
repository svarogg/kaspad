package selectedtip

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

	t.Run("SimpleCall", func(t *testing.T) {
		peer := peerpkg.New(&netadapter.NetConnection{})

		go func() {
			err = RequestSelectedTip(ctx, incomingRoute, outgoingRoute, peer)
			if err != nil {
				t.Fatalf("RequestSelectedTip: %s", err)
			}
		}()

		peer.RequestSelectedTipIfRequired()
		_, err := outgoingRoute.DequeueWithTimeout(dequeueTimeout)
		if err != nil {
			t.Fatalf("RequestSelectedTip: %s", err)
		}

		err = incomingRoute.Enqueue(appmessage.NewMsgSelectedTip(nil))
		if err != nil {
			t.Fatalf("RequestSelectedTip: %s", err)
		}
	})

	t.Run("CheckOutgoingMessageType", func(t *testing.T) {
		peer := peerpkg.New(&netadapter.NetConnection{})

		go func() {
			err = RequestSelectedTip(ctx, incomingRoute, outgoingRoute, peer)
			if err != nil {
				t.Fatalf("RequestSelectedTip: %s", err)
			}
		}()

		peer.RequestSelectedTipIfRequired()
		msg, err := outgoingRoute.Dequeue()
		if err != nil {
			t.Fatalf("RequestSelectedTip: %s", err)
		}

		if _, ok := msg.(*appmessage.MsgRequestSelectedTip); !ok {
			t.Fatalf("RequestSelectedTip: expected %s, got %s", appmessage.CmdRequestSelectedTip, msg.Command())
		}

		err = incomingRoute.Enqueue(appmessage.NewMsgSelectedTip(nil))
		if err != nil {
			t.Fatalf("RequestSelectedTip: %s", err)
		}
	})

	t.Run("CallMultipleTimes", func(t *testing.T) {
		peer := peerpkg.New(&netadapter.NetConnection{})
		const minGetSelectedTipInterval = time.Minute
		go func() {
			err = RequestSelectedTip(ctx, incomingRoute, outgoingRoute, peer)
			if err != nil {
				t.Fatalf("RequestSelectedTip: %s", err)
			}
		}()

		for i := 0; i < callTimes; i++ {
			peer.RequestSelectedTipIfRequired()
			_, err := outgoingRoute.DequeueWithTimeout(dequeueTimeout)
			if err != nil {
				t.Fatalf("RequestSelectedTip: %s", err)
			}

			err = incomingRoute.Enqueue(appmessage.NewMsgSelectedTip(nil))
			if err != nil {
				t.Fatalf("RequestSelectedTip: %s", err)
			}

			// because of RequestSelectedTipIfRequired internal interval
			time.Sleep(minGetSelectedTipInterval)
		}
	})

	t.Run("CallOnClosedRoute", func(t *testing.T) {
		peer := peerpkg.New(&netadapter.NetConnection{})
		closedRoute := router.NewRoute()
		closedRoute.Close()

		go func() {
			err = RequestSelectedTip(ctx, incomingRoute, outgoingRoute, peer)
			if err == nil {
				t.Fatal("RequestSelectedTip: expected error, got nil")
			}
		}()

		peer.RequestSelectedTipIfRequired()
		_, err := outgoingRoute.DequeueWithTimeout(dequeueTimeout)
		if err != nil {
			t.Fatalf("RequestSelectedTip: %s", err)
		}

	})

	t.Run("CallOnNilRoutes", func(t *testing.T) {
		peer := peerpkg.New(&netadapter.NetConnection{})

		go func() {
			err = RequestSelectedTip(ctx, nil, nil, peer)
			if err == nil {
				t.Fatal("RequestSelectedTip: expected error, got nil")
			}
		}()

		peer.RequestSelectedTipIfRequired()
		_, err = outgoingRoute.DequeueWithTimeout(dequeueTimeout)
		if err != nil {
			t.Fatalf("RequestSelectedTip: %s", err)
		}
	})

	t.Run("CallOnNilPeer", func(t *testing.T) {
		err = RequestSelectedTip(ctx, incomingRoute, outgoingRoute, nil)
		if err == nil {
			t.Fatal("RequestSelectedTip: expected error, got nil")
		}
	})
}
