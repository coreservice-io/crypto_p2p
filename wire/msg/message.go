package msg

import (
	"errors"
	"fmt"
	"io"
)

// pre-define cmd for message
const CMD_RESP = 0 //special one
const CMD_VERSION = 1
const CMD_VERACK = 2
const CMD_PING = 3
const CMD_PONG = 4
const CMD_HANDSHAKE = 5

var ErrUnknownMessage = fmt.Errorf("received unknown message")

type Message interface {
	Command() uint32
	Decode(io.Reader, uint32) error
	Encode(io.Writer, uint32) error
}

type MessageHandler struct {
	handlers map[uint32]Message
}

func InitMessageHandler() (*MessageHandler, error) {
	mh := &MessageHandler{
		handlers: make(map[uint32]Message),
	}
	return mh, nil
}

func (hm *MessageHandler) MakeEmptyMessage(cmd uint32) (Message, error) {
	var message Message
	switch cmd {
	case CMD_VERSION:
		message = &MsgVersion{}

	case CMD_VERACK:
		message = &MsgVerAck{}

	case CMD_PING:
		message = &MsgPing{}

	case CMD_PONG:
		message = &MsgPong{}

	default:
		message := hm.GetMessage(cmd)
		if message == nil {
			return nil, ErrUnknownMessage
		}
	}
	return message, nil
}

func (hm *MessageHandler) RegMessage(message Message) error {
	if _, ok := hm.handlers[message.Command()]; ok {
		return errors.New("cmd handler overlap")
	}
	hm.handlers[message.Command()] = message
	return nil
}

func (hm *MessageHandler) GetMessage(cmd uint32) Message {
	message, ok := hm.handlers[cmd]
	if !ok {
		return nil
	}

	return message
}
