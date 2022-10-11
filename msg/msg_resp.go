package msg

import (
	"bytes"
	"errors"
)

type MsgBuffer struct {
	connect_chunk_idx uint32
	Body              bytes.Buffer
}

// return (finished , error happens)
func (msg_buffer *MsgBuffer) ConnectChunk(msg_chunk *MsgChunk) (bool, error) {

	if msg_buffer.connect_chunk_idx != msg_chunk.Header.Chunk_idx {
		return false, errors.New("ConnectChunk idx not match")
	}

	if msg_chunk.Header.Body_len > 0 {
		_, err := msg_buffer.Body.Write(msg_chunk.body)
		if err != nil {
			return false, err
		}
	}

	msg_buffer.connect_chunk_idx++

	if msg_chunk.Header.Chunk_idx == msg_chunk.Header.Chunk_size-1 {
		return true, nil
	} else {
		return false, nil
	}
}
