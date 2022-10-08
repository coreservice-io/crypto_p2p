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

// decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgSendAddr) Decode(r io.Reader, pver uint32) error {
	return nil
}

// encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgSendAddr) Encode(w io.Writer, pver uint32) error {
	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgSendAddr) Command() string {
	return CmdSendAddr
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgSendAddr) MaxPayloadLength(pver uint32) uint32 {
	return 0
}

// returns a new sendaddr message that conforms to the
// Message interface.
func NewMsgSendAddr() *MsgSendAddr {
	return &MsgSendAddr{}
}
