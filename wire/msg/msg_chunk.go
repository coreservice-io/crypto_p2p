package msg

// max chunks number for a whole message
const MSG_CHUNK_LIMIT = 1024

// max data size for a single chunk
const MSG_CHUNK_BODY_LIMIT = 1024 * 8 // 8KB

const MSG_CHUNK_HEADER_SIZE = 20 //bytes

// messageHeader defines the header structure for all protocol messages.
type MsgChunkHeader struct {
	cmd        uint32 // 4 bytes
	id         uint32 // 4 bytes a unique random number which assigned by request side
	body_len   uint32 // 4 bytes
	chunk_size uint32 // 4 bytes
	chunk_idx  uint32 // 4 bytes
}
