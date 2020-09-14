package selectedtip

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/domain/blockdag"
	"github.com/kaspanet/kaspad/domain/dagconfig"
	"github.com/kaspanet/kaspad/infrastructure/db/dbaccess"
	"github.com/kaspanet/kaspad/infrastructure/network/netadapter/router"
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
		DAGParams:       &dagconfig.SimnetParams,
		TimeSource:      blockdag.NewTimeSource(),
	})
	if err != nil {
		t.Fatalf("HandleRequestSelectedTip: %s", err)
	}

	ctx := &mockContext{dag: dag}
	incomingRoute := router.NewRoute()
	outgoingRoute := router.NewRoute()

	t.Run("SimpleCall", func(t *testing.T) {
		go func() {
			err = HandleRequestSelectedTip(ctx, incomingRoute, outgoingRoute)
			if err != nil {
				t.Fatalf("HandleRequestSelectedTip: %s", err)
			}
		}()

		err = incomingRoute.Enqueue(appmessage.NewMsgRequestSelectedTip())
		if err != nil {
			t.Fatalf("HandleRequestSelectedTip: %s", err)
		}

		_, err = outgoingRoute.DequeueWithTimeout(dequeueTimeout)
		if err != nil {
			t.Fatalf("HandleRequestSelectedTip: %s", err)
		}
	})

	t.Run("CheckOutgoingMessageType", func(t *testing.T) {
		go func() {
			err = HandleRequestSelectedTip(ctx, incomingRoute, outgoingRoute)
			if err != nil {
				t.Fatalf("HandleRequestSelectedTip: %s", err)
			}
		}()

		err = incomingRoute.Enqueue(appmessage.NewMsgRequestSelectedTip())
		if err != nil {
			t.Fatalf("HandleRequestSelectedTip: %s", err)
		}

		msg, err := outgoingRoute.DequeueWithTimeout(dequeueTimeout)
		if err != nil {
			t.Fatalf("HandleRequestSelectedTip: %s", err)
		}

		if _, ok := msg.(*appmessage.MsgSelectedTip); !ok {
			t.Fatalf("HandleRequestSelectedTip: expected %s, got %s", appmessage.CmdSelectedTip, msg.Command())
		}
	})

	t.Run("CallMultipleTimes", func(t *testing.T) {
		go func() {
			err = HandleRequestSelectedTip(ctx, incomingRoute, outgoingRoute)
			if err != nil {
				t.Fatalf("HandleRequestSelectedTip: %s", err)
			}
		}()

		for i := 0; i < callTimes; i++ {
			err = incomingRoute.Enqueue(appmessage.NewMsgRequestSelectedTip())
			if err != nil {
				t.Fatalf("HandleRequestSelectedTip: %s", err)
			}

			_, err := outgoingRoute.DequeueWithTimeout(dequeueTimeout)
			if err != nil {
				t.Fatalf("HandleRequestSelectedTip: %s", err)
			}
		}
	})

	t.Run("CallOnClosedRoute", func(t *testing.T) {
		closedRoute := router.NewRoute()
		closedRoute.Close()
		err = HandleRequestSelectedTip(ctx, closedRoute, outgoingRoute)
		if err == nil {
			t.Fatal("HandleRequestSelectedTip: expected error, got nil")
		}
	})

	t.Run("CallOnNilRoutes", func(t *testing.T) {
		err := HandleRequestSelectedTip(ctx, nil, nil)
		if err == nil {
			t.Fatal("HandleRequestSelectedTip: expected err, got nil")
		}
	})

	t.Run("CallWithEnqueuedInvalidMessage", func(t *testing.T) {
		go func() {
			err = HandleRequestSelectedTip(ctx, incomingRoute, outgoingRoute)
			if err == nil {
				t.Fatal("HandleRequestSelectedTip: expected err, got nil")
			}
		}()

		err := incomingRoute.Enqueue(appmessage.NewMsgPing(1))
		if err != nil {
			t.Fatalf("HandleRequestSelectedTip: %s", err)
		}

		_, err = outgoingRoute.DequeueWithTimeout(dequeueTimeout)
		if err != nil {
			t.Fatalf("HandleRequestSelectedTip: %s", err)
		}
	})
}
