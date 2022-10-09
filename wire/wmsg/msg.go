package wmsg

import (
	"fmt"

	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

const CMD_TEST_INFO = 4
const CMD_TEST_CHAT = 5

// Commands used in message headers which describe the type of message.
const (
	CMD_VERSION = iota
	CMD_VERACK
	CMD_REJECT
	CMD_PING
	CMD_PONG
	CMD_SENDADDR
	CMD_MSGDATA
)

// ErrUnknownMessage is the error returned when decoding an unknown message.
var ErrUnknownMessage = fmt.Errorf("received unknown message")

// ErrInvalidHandshake is the error returned when a peer sends us a known
// message that does not belong in the version-verack handshake.
var ErrInvalidHandshake = fmt.Errorf("invalid message during handshake")

// makeEmptyMessage creates a message of the appropriate concrete type based
// on the command.
func MakeEmptyMessage(command uint32) (wirebase.Message, error) {
	var msg wirebase.Message
	switch command {
	case CMD_VERSION:
		msg = &MsgVersion{}

	case CMD_VERACK:
		msg = &MsgVerAck{}

	case CMD_PING:
		msg = &MsgPing{}

	case CMD_PONG:
		msg = &MsgPong{}

	case CMD_REJECT:
		msg = &MsgReject{}

	case CMD_SENDADDR:
		msg = &MsgSendAddr{}

	default:
		return nil, ErrUnknownMessage
	}
	return msg, nil
}
