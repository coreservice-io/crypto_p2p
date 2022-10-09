package wmsg

import (
	"io"
)

// defines a verack message which is used for a peer to
// acknowledge a version message (MsgVersion) after it has used the information
// to negotiate parameters.  It implements the Message interface.
//
// This message has no payload.
type MsgVerAck struct{}

func (msg *MsgVerAck) Decode(r io.Reader, pver uint32) error {
	return nil
}

func (msg *MsgVerAck) Encode(w io.Writer, pver uint32) error {
	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgVerAck) Command() uint32 {
	return CMD_VERACK
}

// returns a new verack message that confirms to the
// Message interface.
func NewMsgVerAck() *MsgVerAck {
	return &MsgVerAck{}
}
