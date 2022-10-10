package msg

import "io"

// Commands used in message headers which describe the type of message.

const CMD_RESP = 0
const CMD_VERSION = 1
const CMD_PING = 2
const CMD_SENDPORT = 3

// readElement reads the next sequence of bytes from r using little endian
// depending on the concrete type of element pointed to.
// func ReadElement(content []byte, element interface{}) error {
// 	// Attempt to read the element based on the concrete type via fast
// 	// type assertions first.
// 	switch element.(type) {

// 	case *int16:
// 		element = binary.LittleEndian.Uint16(content)
// 		return nil

// 	case *uint32:
// 		element = binary.LittleEndian.Uint32(content)
// 		return nil

// 	case *uint64:
// 		element = binary.LittleEndian.Uint64(content)
// 		return nil

// 	default:
// 		return errors.New("type error")
// 	}
// }

// Message is an interface that describes a message.  A type that
// implements Message has complete control over the representation of its data
// and may therefore contain additional or fewer fields than those which
// are used directly in the protocol encoded message.
type MsgI interface {
	Command() uint32
	Encode() ([]byte, error)
}

type Msg struct {
	Headr []byte
	Body  []byte
}

func (msg *Msg) Recycle() {

}

func NewMsg(reader io.Reader) (MsgI, error) {

}
