package wmsg

import (
	"io"

	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

// MsgPong implements the Message interface and represents a pong
// message which is used primarily to confirm that a connection is still valid
// in response to a ping message (MsgPing).
//
// This message was not added until protocol versions AFTER BIP0031Version.
type MsgPong struct {
	// Unique value associated with message that is used to identify
	// specific ping message.
	Nonce uint64
}

func (msg *MsgPong) Decode(r io.Reader, pver uint32) error {
	return wirebase.ReadElement(r, &msg.Nonce)
}

func (msg *MsgPong) Encode(w io.Writer, pver uint32) error {
	return wirebase.WriteElement(w, msg.Nonce)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgPong) Command() uint32 {
	return CMD_PONG
}

// NewMsgPong returns a new pong message that confirms to the Message
// interface.  See MsgPong for details.
func NewMsgPong(nonce uint64) *MsgPong {
	return &MsgPong{
		Nonce: nonce,
	}
}
