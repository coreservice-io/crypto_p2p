package wmsg

import (
	"bytes"
	"fmt"
	"io"
	"strings"
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
	ProtocolVersion int32

	// Time the message was generated.  This is encoded as an int64 on the wire.
	Timestamp time.Time

	// Address of the remote peer.
	AddrYou [32]byte

	// Address of the local peer.
	AddrMe [32]byte

	// Unique value associated with message that is used to detect self
	// connections.
	Nonce uint64

	// The user agent that generated messsage.  This is a encoded as a varString
	// on the wire.  This has a max length of MaxUserAgentLen.
	UserAgent string
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
		userAgent, err := wirebase.ReadVarString(buf, pver)
		if err != nil {
			return err
		}
		err = validateUserAgent(userAgent)
		if err != nil {
			return err
		}
		msg.UserAgent = userAgent
	}

	return nil
}

func (msg *MsgVersion) Encode(w io.Writer, pver uint32) error {
	err := validateUserAgent(msg.UserAgent)
	if err != nil {
		return err
	}

	err = wirebase.WriteElements(w, msg.ProtocolVersion, msg.Timestamp.Unix())
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

	err = wirebase.WriteVarString(w, pver, msg.UserAgent)
	if err != nil {
		return err
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgVersion) Command() string {
	return CmdVersion
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgVersion) MaxPayloadLength(pver uint32) uint32 {
	// XXX: <= 106 different

	// Protocol version 4 bytes + services 8 bytes + timestamp 8 bytes +
	// remote and local net addresses + nonce 8 bytes +
	// length of user agent (varInt) + max allowed useragent length
	return 33 + (30 * 2) +
		wirebase.MaxVarIntPayload + MaxUserAgentLen
}

// returns a new version message that conforms to the
// Message interface using the passed parameters and defaults for the remaining
// fields.
func NewMsgVersion(me [32]byte, you [32]byte, nonce uint64, protocolVersion int32) *MsgVersion {

	// Limit the timestamp to one second precision since the protocol
	// doesn't support better.
	return &MsgVersion{
		ProtocolVersion: protocolVersion,
		Timestamp:       time.Unix(time.Now().Unix(), 0),
		AddrYou:         you,
		AddrMe:          me,
		Nonce:           nonce,
		UserAgent:       DefaultUserAgent,
	}
}

// validateUserAgent checks userAgent length against MaxUserAgentLen
func validateUserAgent(userAgent string) error {
	if len(userAgent) > MaxUserAgentLen {
		str := fmt.Sprintf("user agent too long [len %v, max %v]",
			len(userAgent), MaxUserAgentLen)
		return wirebase.NewMessageError("MsgVersion", str)
	}
	return nil
}

// AddUserAgent adds a user agent to the user agent string for the version message.
// The version string is not defined to any strict format, although
// it is recommended to use the form "major.minor.revision" e.g. "2.6.41".
func (msg *MsgVersion) AddUserAgent(name string, version string,
	comments ...string) error {

	newUserAgent := fmt.Sprintf("%s:%s", name, version)
	if len(comments) != 0 {
		newUserAgent = fmt.Sprintf("%s(%s)", newUserAgent,
			strings.Join(comments, "; "))
	}
	newUserAgent = fmt.Sprintf("%s%s/", msg.UserAgent, newUserAgent)
	err := validateUserAgent(newUserAgent)
	if err != nil {
		return err
	}
	msg.UserAgent = newUserAgent
	return nil
}
