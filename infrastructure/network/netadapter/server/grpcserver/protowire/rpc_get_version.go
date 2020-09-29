package protowire

import "github.com/kaspanet/kaspad/app/appmessage"

func (x *KaspadMessage_GetVersionRequest) toAppMessage() (appmessage.Message, error) {
	return &appmessage.GetVersionRequestMessage{}, nil
}

func (x *KaspadMessage_GetVersionRequest) fromAppMessage(_ *appmessage.GetVersionRequestMessage) error {
	x.GetVersionRequest = &GetVersionRequestMessage{}
	return nil
}

func (x *KaspadMessage_GetVersionResponse) toAppMessage() (appmessage.Message, error) {
	var err *appmessage.RPCError
	if x.GetVersionResponse.Error != nil {
		err = &appmessage.RPCError{Message: x.GetVersionResponse.Error.Message}
	}
	return &appmessage.GetVersionResponseMessage{
		Version: x.GetVersionResponse.Version,
		Error:   err,
	}, nil
}

func (x *KaspadMessage_GetVersionResponse) fromAppMessage(message *appmessage.GetVersionResponseMessage) error {
	var err *RPCError
	if message.Error != nil {
		err = &RPCError{Message: message.Error.Message}
	}
	x.GetVersionResponse = &GetVersionResponseMessage{
		Version: message.Version,
		Error:   err,
	}
	return nil
}
