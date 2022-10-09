package wirebase

import (
	"io"
)

const (
	// MaxVarIntPayload is the maximum payload size for a variable length integer.
	MaxVarIntPayload = 9
)

const MSG_PAYLOAD_MAX_LEN = 1024 * 8 // 8KB

// number for maxium size of a single message.
const MSG_DATA_CHUNKS_NUM = 1024

// the maximum bytes of a single message 8MB
const MSG_MAX_PAYLOAD_SIZE = (MSG_PAYLOAD_MAX_LEN * MSG_DATA_CHUNKS_NUM)

// represents which network a message belongs to.
type NetMagic uint32

// Message is an interface that describes a message.  A type that
// implements Message has complete control over the representation of its data
// and may therefore contain additional or fewer fields than those which
// are used directly in the protocol encoded message.
type Message interface {
	Command() uint32
	Decode(io.Reader, uint32) error
	Encode(io.Writer, uint32) error
}

// the number of bytes in a message header.
// network (magic) 4 bytes + command 4 bytes +
// payload length 4 bytes
const MSG_HEADER_SIZE = 28

// messageHeader defines the header structure for all protocol messages.
type messageHeader struct {
	magic uint32 // 4 bytes
	id    uint32 // 4 bytes a unique random number which assigned by request side
	// respid  uint32 // 4 bytes
	command uint32 // 4 bytes
	length  uint32 // 4 bytes
	flag    uint32 // 4 bytes
	ck_idx  uint32
	ck_size uint32
}

type MakeEmptyMessage func(command uint32) (Message, error)
