package netadapter

import (
	"fmt"

	"github.com/kaspanet/kaspad/netadapter/server"
)

// NetConnection is a wrapper to a server connection for use by services external to NetAdapter
type NetConnection struct {
	connection server.Connection

	// ID returns the ID associated with this connection
	ID string
}

func newNetConnection(connection server.Connection, id string) *NetConnection {
	return &NetConnection{
		connection: connection,
		ID:         id,
	}
}

func (c *NetConnection) String() string {
	return fmt.Sprintf("<%s: %s>", c.ID, c.connection)
}

// Address returns the address associated with this connection
func (c *NetConnection) Address() string {
	return c.connection.Address().String()
}

// SetOnInvalidMessageHandler sets a handler function
// for invalid messages
func (c *NetConnection) SetOnInvalidMessageHandler(onInvalidMessageHandler server.OnInvalidMessageHandler) {
	c.connection.SetOnInvalidMessageHandler(onInvalidMessageHandler)
}
