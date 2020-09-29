package appmessage

// GetVersionRequestMessage is an appmessage corresponding to
// its respective RPC message
type GetVersionRequestMessage struct {
	baseMessage
}

// Command returns the protocol command string for the message
func (msg *GetVersionRequestMessage) Command() MessageCommand {
	return CmdGetVersionRequestMessage
}

// NewGetVersionRequestMessage returns a instance of the message
func NewGetVersionRequestMessage() *GetVersionRequestMessage {
	return &GetVersionRequestMessage{}
}

// GetVersionResponseMessage is an appmessage corresponding to
// its respective RPC message
type GetVersionResponseMessage struct {
	baseMessage
	Version string

	Error *RPCError
}

// Command returns the protocol command string for the message
func (msg *GetVersionResponseMessage) Command() MessageCommand {
	return CmdGetVersionResponseMessage
}

// NewGetVersionResponseMessage returns a instance of the message
func NewGetVersionResponseMessage(version string) *GetVersionResponseMessage {
	return &GetVersionResponseMessage{
		Version: version,
	}
}
