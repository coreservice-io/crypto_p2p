package wmsg

import (
	"io"
)

// defines a sendaddr message which is used for a peer
// to signal support for receiving ADDRV2 messages.
// It implements the Message interface.
//
// This message has no payload.
type MsgSendAddr struct{}

// decodes r using the protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgSendAddr) Decode(r io.Reader, pver uint32) error {
	return nil
}

// encodes the receiver to w using the protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgSendAddr) Encode(w io.Writer, pver uint32) error {
	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgSendAddr) Command() uint32 {
	return CMD_SENDADDR
}

// returns a new sendaddr message that confirms to the
// Message interface.
func NewMsgSendAddr() *MsgSendAddr {
	return &MsgSendAddr{}
}
