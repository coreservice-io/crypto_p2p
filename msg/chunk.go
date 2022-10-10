package msg

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
)

// max chunks number for a whole message
const MSG_CHUNK_LIMIT = 1024

// max data size for a single chunk
const MSG_CHUNK_BODY_SIZE_LIMIT = 1024 * 1 // 1kb for buffer pool efficiency

const MSG_CHUNK_HEADER_SIZE = 20 //bytes

// buffer pool
var chunk_header_pool = sync.Pool{
	New: func() interface{} {
		return make([]byte, MSG_CHUNK_HEADER_SIZE)
	},
}

var chunk_body_pool = sync.Pool{
	New: func() interface{} {
		return make([]byte, MSG_CHUNK_BODY_SIZE_LIMIT)
	},
}

// messageHeader defines the header structure for all protocol messages.
type MsgChunkHeader struct {
	cmd        uint32 // 4 bytes
	req_id     uint32 // 4 bytes a unique random number which assigned by request side
	body_len   uint32 // 4 bytes
	chunk_size uint32 // 4 bytes
	chunk_idx  uint32 // 4 bytes
}

type MsgChunk struct {
	header *MsgChunkHeader
	body   []byte
}

func (msg_chunk *MsgChunk) Cmd() uint32 {
	return msg_chunk.header.cmd
}

func (msg_chunk *MsgChunk) Recycle() {
	if msg_chunk.body != nil {
		chunk_body_pool.Put(msg_chunk.body)
	}
}

func ReadMsgChunk(reader io.Reader) (*MsgChunk, error) {

	msg_chunk := &MsgChunk{}

	//read header
	header_buff := chunk_header_pool.Get().([]byte)
	defer chunk_header_pool.Put(header_buff)

	_, err := io.ReadFull(reader, header_buff)
	if err != nil {
		return nil, err
	}

	header, err := decodeMsgChunkHeader(header_buff)
	if err != nil {
		return nil, err
	}

	if header.chunk_idx > MSG_CHUNK_LIMIT {
		return nil, errors.New("chunk_idx overlimit [MSG_CHUNK_LIMIT]")
	}

	msg_chunk.header = header
	//read body
	if msg_chunk.header.body_len > 0 {
		msg_chunk.body = chunk_body_pool.Get().([]byte)
		_, err := io.ReadFull(reader, msg_chunk.body[:msg_chunk.header.body_len])
		if err != nil {
			msg_chunk.Recycle()
			return nil, err
		}
	}

	return msg_chunk, nil
}

func decodeMsgChunkHeader(header_bytes []byte) (*MsgChunkHeader, error) {
	if len(header_bytes) != MSG_CHUNK_HEADER_SIZE {
		return nil, errors.New("header size err")
	}

	return &MsgChunkHeader{
		cmd:        binary.LittleEndian.Uint32(header_bytes[0:4]),
		req_id:     binary.LittleEndian.Uint32(header_bytes[4:8]),
		body_len:   binary.LittleEndian.Uint32(header_bytes[8:12]),
		chunk_size: binary.LittleEndian.Uint32(header_bytes[12:16]),
		chunk_idx:  binary.LittleEndian.Uint32(header_bytes[16:20]),
	}, nil

}
