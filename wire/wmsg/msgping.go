package wmsg

import (
	"io"

	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

// MsgPing implements the Message interface and represents a bitcoin ping
// message.
//
// it contains an identifier which can be
// returned in the pong message to determine network timing.
//
// The payload for this message just consists of a nonce used for identifying
// it later.
type MsgPing struct {
	// Unique value associated with message that is used to identify
	// specific ping message.
	Nonce uint64
}

func (msg *MsgPing) Decode(r io.Reader, pver uint32) error {
	err := wirebase.ReadElement(r, &msg.Nonce)
	if err != nil {
		return err
	}

	return nil
}

func (msg *MsgPing) Encode(w io.Writer, pver uint32) error {
	err := wirebase.WriteElement(w, msg.Nonce)
	if err != nil {
		return err
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgPing) Command() string {
	return CmdPing
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgPing) MaxPayloadLength(pver uint32) uint32 {
	plen := uint32(8)
	return plen
}

// NewMsgPing returns a new bitcoin ping message that conforms to the Message
// interface.  See MsgPing for details.
func NewMsgPing(nonce uint64) *MsgPing {
	return &MsgPing{
		Nonce: nonce,
	}
}
