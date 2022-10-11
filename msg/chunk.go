package msg

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"sync"
	"time"
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
	Cmd        uint32 // 4 bytes
	Req_id     uint32 // 4 bytes a unique random number which assigned by request side
	Body_len   uint32 // 4 bytes
	Chunk_size uint32 // 4 bytes
	Chunk_idx  uint32 // 4 bytes
}

type MsgChunk struct {
	Header *MsgChunkHeader
	body   []byte
}

func (msg_chunk *MsgChunk) Recycle() {
	if msg_chunk.body != nil {
		chunk_body_pool.Put(msg_chunk.body)
	}
}

// split content to chunks and write it to connection
// if timeout_secs<=0 then timeout won't be used
func WriteMsgChunks(conn net.Conn, req_id uint32, cmd uint32, content []byte, timeout_secs int) error {

	content_len := len(content)

	if content_len > MSG_CHUNK_LIMIT*MSG_CHUNK_BODY_SIZE_LIMIT {
		return errors.New("content over limit")
	}

	chunks := int(math.Ceil(float64(content_len) / MSG_CHUNK_BODY_SIZE_LIMIT))
	if chunks == 0 {
		chunks = 1
	}

	if timeout_secs > 0 {
		err := conn.SetWriteDeadline(time.Now().Add(time.Duration(timeout_secs) * time.Second))
		if err != nil {
			return err
		}
	}

	for i := 0; i < chunks; i++ {

		start_p := i * MSG_CHUNK_BODY_SIZE_LIMIT
		end_p := (i + 1) * MSG_CHUNK_BODY_SIZE_LIMIT

		if end_p >= content_len {
			end_p = content_len
		}

		_, w_err := conn.Write((&MsgChunkHeader{
			Cmd:        cmd,
			Req_id:     req_id,
			Body_len:   uint32(end_p - start_p),
			Chunk_size: uint32(chunks),
			Chunk_idx:  uint32(i),
		}).encodeMsgChunkHeader())

		if w_err != nil {
			return w_err
		}

		if end_p > 0 {
			_, w_err = conn.Write(content[start_p:end_p])
			if w_err != nil {
				return w_err
			}
		}

	}

	return nil

}

func ReadMsgChunk(conn net.Conn) (*MsgChunk, error) {

	msg_chunk := &MsgChunk{}

	//read header
	header_buff := chunk_header_pool.Get().([]byte)
	defer chunk_header_pool.Put(header_buff)

	_, err := io.ReadFull(conn, header_buff)
	if err != nil {
		return nil, err
	}

	header, err := decodeMsgChunkHeader(header_buff)
	if err != nil {
		return nil, err
	}

	if header.Chunk_idx > MSG_CHUNK_LIMIT {
		return nil, errors.New("chunk_idx overlimit [MSG_CHUNK_LIMIT]")
	}

	msg_chunk.Header = header
	//read body
	if msg_chunk.Header.Body_len > 0 {
		msg_chunk.body = chunk_body_pool.Get().([]byte)
		_, err := io.ReadFull(conn, msg_chunk.body[:msg_chunk.Header.Body_len])
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
		Cmd:        binary.LittleEndian.Uint32(header_bytes[0:4]),
		Req_id:     binary.LittleEndian.Uint32(header_bytes[4:8]),
		Body_len:   binary.LittleEndian.Uint32(header_bytes[8:12]),
		Chunk_size: binary.LittleEndian.Uint32(header_bytes[12:16]),
		Chunk_idx:  binary.LittleEndian.Uint32(header_bytes[16:20]),
	}, nil
}

func (msg_h *MsgChunkHeader) encodeMsgChunkHeader() []byte {

	result := make([]byte, MSG_CHUNK_HEADER_SIZE)

	EncodeUint32(msg_h.Cmd, result[0:4])
	EncodeUint32(msg_h.Req_id, result[4:8])
	EncodeUint32(msg_h.Body_len, result[8:12])
	EncodeUint32(msg_h.Chunk_size, result[12:16])
	EncodeUint32(msg_h.Chunk_idx, result[16:20])

	return result
}
