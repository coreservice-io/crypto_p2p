package wirebase

import (
	"io"
)

const (
	// MaxVarIntPayload is the maximum payload size for a variable length integer.
	MaxVarIntPayload = 9
)

// CommandSize is the fixed size of all commands in the common message
// header.  Shorter commands must be zero padded.
const CommandSize = 12

// MaxMessagePayload is the maximum bytes a message can be regardless of other
// individual limits imposed by messages themselves.
const MaxMessagePayload = (1024 * 1024 * 1) // Unit MB

// represents which network a message belongs to.
type NetMagic uint32

// MessageEncoding represents the wire message encoding format to be used.
type MessageEncoding uint32

const (
	// BaseEncoding encodes all messages in the default format specified
	// for the wire protocol.
	BaseEncoding MessageEncoding = 1 << iota

	// WitnessEncoding encodes all messages other than transaction messages
	WitnessEncoding
)

// LatestEncoding is the most recently specified encoding for the wire
// protocol.
var LatestEncoding = WitnessEncoding

// Message is an interface that describes a message.  A type that
// implements Message has complete control over the representation of its data
// and may therefore contain additional or fewer fields than those which
// are used directly in the protocol encoded message.
type Message interface {
	Command() string
	MaxPayloadLength(uint32) uint32
	Decode(io.Reader, uint32) error
	Encode(io.Writer, uint32) error
}

// MessageHeaderSize is the number of bytes in a message header.
// network (magic) 4 bytes + command 12 bytes + payload length 4 bytes +
// checksum 4 bytes.
const MessageHeaderSize = 24

// messageHeader defines the header structure for all protocol messages.
type messageHeader struct {
	magic    uint32  // 4 bytes
	command  string  // 12 bytes
	length   uint32  // 4 bytes
	checksum [4]byte // 4 bytes
}

type MakeEmptyMessage func(command string) (Message, error)
