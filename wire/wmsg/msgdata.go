package wmsg

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

type MsgData struct {
	Id      uint32
	Msg_cmd uint32
	//incoming message chunk size [chunk1_size,chunk2_size....] , -1 for eof of message
	Msg_chunks chan int32
	Msg_buffer *bytes.Buffer
	Msg_err    bool

	Chunks_Idx  int32
	Chunks_Size int32

	content []byte
}

func (msg *MsgData) Decode(r io.Reader, pver uint32) error {
	return nil
}

func (msg *MsgData) Encode(w io.Writer, pver uint32) error {
	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgData) Command() uint32 {
	return CMD_MSGDATA
}

// returns a new verack message that confirms to the
// Message interface.
func NewMsgData(content []byte) *MsgData {
	return &MsgData{
		content: content,
	}
}

func (msg *MsgData) Receive() ([]byte, error) {

	total_chunks := 0
	eof := false

	for {

		select {
		case msg_chunk_size := <-msg.Msg_chunks:

			if msg_chunk_size == -1 {
				eof = true
			} else {
				total_chunks++
			}

		case <-time.After(10 * time.Second):
			return nil, errors.New("timeout occure")
		}

		if total_chunks > wirebase.MSG_DATA_CHUNKS_NUM {
			return nil, errors.New("msg chunk exceed limit, total chunks:" + strconv.Itoa(total_chunks))
		}

		if eof {
			if msg.Msg_err {
				return nil, errors.New(msg.Msg_buffer.String())
			}
			return msg.Msg_buffer.Bytes(), nil
		}
	}
}
