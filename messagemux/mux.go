package messagemux

import (
	"github.com/kaspanet/kaspad/p2pserver"
	"github.com/kaspanet/kaspad/wire"
)

type mux struct {
	server        *p2pserver.Server
	connection    *p2pserver.Connection
	msgTypeToFlow map[string]func()
}

func New(server *p2pserver.Server, connection *p2pserver.Connection) Mux {
	m := &mux{server: server, connection: connection}

	return m
}

func (m mux) AddFlow(msgTypes []string, ch chan<- wire.Message) {
	panic("implement me")
}

func (m mux) Stop() {
	panic("implement me")
}
