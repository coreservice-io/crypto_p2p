package msg

import (
	"io"

	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

type MsgPing struct {
	Nonce uint64
}

func (msg *MsgPing) Command() uint32 {
	return CMD_PING
}

func (msg *MsgPing) Encode(w io.Writer, pver uint32) error {
	err := wirebase.WriteElement(w, msg.Nonce)
	if err != nil {
		return err
	}

	return nil
}

func (msg *MsgPing) Decode(r io.Reader, pver uint32) error {
	err := wirebase.ReadElement(r, &msg.Nonce)
	if err != nil {
		return err
	}

	return nil
}

func NewMsgPing(nonce uint64) *MsgPing {
	return &MsgPing{
		Nonce: nonce,
	}
}

type MsgPong struct {
	Nonce uint64
}

func (msg *MsgPong) Command() uint32 {
	return CMD_PONG
}

func (msg *MsgPong) Encode(w io.Writer, pver uint32) error {
	return wirebase.WriteElement(w, msg.Nonce)
}

func (msg *MsgPong) Decode(r io.Reader, pver uint32) error {
	return wirebase.ReadElement(r, &msg.Nonce)
}

// NewMsgPong returns a new pong message that confirms to the Message
// interface.  See MsgPong for details.
func NewMsgPong(nonce uint64) *MsgPong {
	return &MsgPong{
		Nonce: nonce,
	}
}
