package wmsg

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

// MaxUserAgentLen is the maximum allowed length for the user agent field in a
// version message (MsgVersion).
const MaxUserAgentLen = 256

// DefaultUserAgent for wire in the stack
const DefaultUserAgent = "/wire:0.0.1/"

// implements the Message interface and represents a version
// message.  It is used for a peer to advertise itself as soon as an outbound
// connection is made.  The remote peer then uses this information along with
// its own to negotiate.  The remote peer must then respond with a version
// message of its own containing the negotiated values followed by a verack
// message (MsgVerAck).  This exchange must take place before any further
// communication is allowed to proceed.
type MsgVersion struct {
	// Version of the protocol the node is using.
	ProtocolVersion uint32

	// Time the message was generated.  This is encoded as an int64 on the wire.
	Timestamp time.Time

	// Address of the remote peer.
	AddrYou [32]byte

	// Address of the local peer.
	AddrMe [32]byte

	// Unique value associated with message that is used to detect self connections.
	Nonce uint64
}

func (msg *MsgVersion) Decode(r io.Reader, pver uint32) error {
	buf, ok := r.(*bytes.Buffer)
	if !ok {
		return fmt.Errorf("MsgVersion.Decode reader is not a *bytes.Buffer")
	}

	var timestamp int64
	err := wirebase.ReadElements(buf, &msg.ProtocolVersion, &timestamp)
	if err != nil {
		return err
	}
	msg.Timestamp = time.Time(time.Unix(timestamp, 0))

	err = wirebase.ReadElements(buf, &msg.AddrYou, &msg.AddrMe)
	if err != nil {
		return err
	}

	if buf.Len() > 0 {
		err = wirebase.ReadElement(buf, &msg.Nonce)
		if err != nil {
			return err
		}
	}

	if buf.Len() > 0 {
		_, err := wirebase.ReadVarString(buf, pver)
		if err != nil {
			return err
		}
	}

	return nil
}

func (msg *MsgVersion) Encode(w io.Writer, pver uint32) error {

	err := wirebase.WriteElements(w, msg.ProtocolVersion, msg.Timestamp.Unix())
	if err != nil {
		return err
	}

	err = wirebase.WriteElements(w, &msg.AddrYou, &msg.AddrMe)
	if err != nil {
		return err
	}

	err = wirebase.WriteElement(w, msg.Nonce)
	if err != nil {
		return err
	}

	err = wirebase.WriteVarString(w, pver, "")
	if err != nil {
		return err
	}

	return nil
}

// Command returns the protocol command string for the message.
// This is part of the Message interface implementation.
func (msg *MsgVersion) Command() uint32 {
	return CMD_VERSION
}

// returns a new version message that confirms to the
// Message interface using the passed parameters and defaults for the remaining
// fields.
func NewMsgVersion(me [32]byte, you [32]byte, nonce uint64, protocolVersion uint32) *MsgVersion {

	// Limit the timestamp to one second precision since the protocol
	// doesn't support better.
	return &MsgVersion{
		ProtocolVersion: protocolVersion,
		Timestamp:       time.Unix(time.Now().Unix(), 0),
		AddrYou:         you,
		AddrMe:          me,
		Nonce:           nonce,
	}
}
