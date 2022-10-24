package msg

import (
	"fmt"
)

// pre-define cmd for message
const CMD_RESP = 0 //special one
const CMD_VERSION = 1
const CMD_VERACK = 2
const CMD_PING = 3
const CMD_PONG = 4
const CMD_HANDSHAKE = 5

var ErrUnknownMessage = fmt.Errorf("received unknown message")

var ErrTimeoutMessage = fmt.Errorf("response timeout")

type MsgResp struct {
	data []byte
}

type MsgVerAck struct{}
