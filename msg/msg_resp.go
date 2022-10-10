package msg

import (
	"bytes"
	"errors"
)

type MsgResp struct {
	connect_chunk_idx uint32
	Body              bytes.Buffer
}

func (msg_resp *MsgResp) ConnectChunk(msg_chunk *MsgChunk) error {

	if msg_resp.connect_chunk_idx != msg_chunk.header.chunk_idx {
		return errors.New("ConnectChunk idx not match")
	}

	if msg_chunk.header.body_len > 0 {
		_, err := msg_resp.Body.Write(msg_chunk.body)
		return err
	}

	return nil
}
