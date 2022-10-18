package msg

import (
	"io"
)

type MsgVerAck struct{}

func (msg *MsgVerAck) Decode(r io.Reader, pver uint32) error {
	return nil
}

func (msg *MsgVerAck) Encode(w io.Writer, pver uint32) error {
	return nil
}

func (msg *MsgVerAck) Command() uint32 {
	return CMD_VERACK
}

func NewMsgVerAck() *MsgVerAck {
	return &MsgVerAck{}
}
