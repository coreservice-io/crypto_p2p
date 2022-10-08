package wmsg

import (
	"fmt"

	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

// Commands used in message headers which describe the type of message.
const (
	CmdVersion  = "version"
	CmdVerAck   = "verack"
	CmdReject   = "reject"
	CmdPing     = "ping"
	CmdPong     = "pong"
	CmdSendAddr = "sendaddr"
)

// ErrUnknownMessage is the error returned when decoding an unknown message.
var ErrUnknownMessage = fmt.Errorf("received unknown message")

// ErrInvalidHandshake is the error returned when a peer sends us a known
// message that does not belong in the version-verack handshake.
var ErrInvalidHandshake = fmt.Errorf("invalid message during handshake")

// makeEmptyMessage creates a message of the appropriate concrete type based
// on the command.
func MakeEmptyMessage(command string) (wirebase.Message, error) {
	var msg wirebase.Message
	switch command {
	case CmdVersion:
		msg = &MsgVersion{}

	case CmdVerAck:
		msg = &MsgVerAck{}

	case CmdPing:
		msg = &MsgPing{}

	case CmdPong:
		msg = &MsgPong{}

	case CmdReject:
		msg = &MsgReject{}

	case CmdSendAddr:
		msg = &MsgSendAddr{}

	default:
		return nil, ErrUnknownMessage
	}
	return msg, nil
}
