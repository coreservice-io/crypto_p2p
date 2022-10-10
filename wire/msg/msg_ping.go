package msg

import (
	"io"
)

type MsgPing struct {
	Msg
}

func (msg *MsgPing) Decode(r io.Reader, pver uint32) error {
	return nil
}

func (msg *MsgPing) Encode(w io.Writer, pver uint32) error {
	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgPing) Command() uint32 {
	return CMD_PING
}

// NewMsgPing returns a new ping message that confirms to the Message
// interface.
// See MsgPing for details.
func NewMsgPing(nonce uint64) *MsgPing {
	return &MsgPing{}
}
