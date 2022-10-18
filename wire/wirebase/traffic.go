package wirebase

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"sync"
)

const TMU_MAX_SIZE = 1024 * 8 // 8KB

// number for maxium size of a single message.
const TMU_MAX_NUM = 1024

// the maximum bytes of a single message 8MB
const MSG_PAYLOAD_MAX_SIZE = (TMU_MAX_SIZE * TMU_MAX_NUM)

const TMU_HEADER_SIZE = 20

type tmuHeader struct {
	cmd     uint32
	length  uint32
	id      uint32
	tmu_idx uint32
	tmu_num uint32
}

func (hdr *tmuHeader) Cmd() uint32 {
	return hdr.cmd
}

func (hdr *tmuHeader) Id() uint32 {
	return hdr.id
}

var tmu_header_pool = sync.Pool{
	New: func() interface{} {
		return make([]byte, TMU_HEADER_SIZE)
	},
}

var tmu_body_pool = sync.Pool{
	New: func() interface{} {
		return make([]byte, TMU_MAX_SIZE)
	},
}

func ReadTmuHeader(iostream io.Reader) (int, *tmuHeader, error) {
	// var headerBytes [TMU_HEADER_SIZE]byte

	header_bytes := tmu_header_pool.Get().([]byte)
	defer tmu_header_pool.Put(header_bytes)

	n, err := io.ReadFull(iostream, header_bytes[:])
	if err != nil {
		return n, nil, err
	}

	if len(header_bytes) != TMU_HEADER_SIZE {
		return n, nil, errors.New("header size err")
	}

	// decoding header
	hdr := &tmuHeader{
		cmd:     binary.LittleEndian.Uint32(header_bytes[0:4]),
		length:  binary.LittleEndian.Uint32(header_bytes[4:8]),
		id:      binary.LittleEndian.Uint32(header_bytes[8:12]),
		tmu_num: binary.LittleEndian.Uint32(header_bytes[12:16]),
		tmu_idx: binary.LittleEndian.Uint32(header_bytes[16:20]),
	}

	return n, hdr, nil
}

// reads n bytes from reader r in chunks and discards the read bytes.
// This is used to skip payloads when various errors occur and helps
// prevent rogue nodes from causing massive memory allocation through
// forging header length.
func discardInput(r io.Reader, n uint32) {
	maxSize := uint32(10 * 1024) // 10k at a time
	numReads := n / maxSize
	bytesRemaining := n % maxSize
	if n > 0 {
		buf := make([]byte, maxSize)
		for i := uint32(0); i < numReads; i++ {
			io.ReadFull(r, buf)
		}
	}
	if bytesRemaining > 0 {
		buf := make([]byte, bytesRemaining)
		io.ReadFull(r, buf)
	}
}

func WriteMessage(w io.Writer, cmd uint32, data []byte, timeout_secs int) (int, error) {

	data_len := len(data)

	if data_len > MSG_PAYLOAD_MAX_SIZE {
		str := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload is %d bytes",
			data_len, MSG_PAYLOAD_MAX_SIZE)
		return 0, NewMessageError("WriteMessage", str)
	}

	chunks := int(math.Ceil(float64(data_len) / TMU_MAX_SIZE))
	if chunks == 0 {
		chunks = 1
	}

	totalBytes := 0

	id := rand.Uint32()

	for i := 0; i < chunks; i++ {
		start_p := i * TMU_MAX_SIZE
		end_p := (i + 1) * TMU_MAX_SIZE
		if end_p >= data_len {
			end_p = data_len
		}
		tmu_payload := data[start_p:end_p]

		hdr := &tmuHeader{
			id:      id,
			cmd:     cmd,
			tmu_num: uint32(chunks),
			tmu_idx: uint32(i + 1),
		}

		n, err := WriteTmu(w, hdr, tmu_payload)
		if err != nil {
			return 0, err
		}

		totalBytes += n
	}

	return totalBytes, nil
}

func ReadMessage(iostream io.Reader, longMessages sync.Map) (*tmuHeader, []byte, error) {

	for {
		_, tmu_hdr, tmu_payload, err := ReadTmu(iostream)
		if err != nil {
			return nil, tmu_payload, err
		}

		msg_payload, err := CombineTmu(longMessages, tmu_hdr, tmu_payload)
		if err != nil {
			return nil, nil, err
		}

		if msg_payload != nil {
			return tmu_hdr, msg_payload, err
		}
	}
}

func CombineTmu(longMessages sync.Map,
	hdr *tmuHeader, payload []byte) ([]byte, error) {
	if hdr.tmu_num == 1 {
		return payload, nil
	}

	if hdr.tmu_num >= hdr.tmu_idx {
		longBuf, exists := longMessages.Load(hdr.id)
		if !exists {
			longBuf = bytes.NewBuffer(payload)
			longMessages.Store(hdr.id, longBuf)
		} else {
			longBuf.(*bytes.Buffer).Write(payload)
		}
	}

	// last one
	if hdr.tmu_num == hdr.tmu_idx {
		longBuf, exists := longMessages.Load(hdr.id)
		if !exists {
			longBuf = bytes.NewBuffer(payload)
			longMessages.Store(hdr.id, longBuf)
		} else {
			longBuf.(*bytes.Buffer).Write(payload)
		}

		payload = longBuf.(*bytes.Buffer).Bytes()

		return payload, nil
	}

	// not build msg
	return nil, nil
}

func WriteTmu(iostream io.Writer, hdr *tmuHeader, payload []byte) (int, error) {

	lenp := len(payload)

	hdr.length = uint32(lenp)

	if lenp > TMU_MAX_SIZE {
		str := fmt.Sprintf("tmu payload is too large - encoded "+
			"%d bytes, but maximum tmu payload is %d bytes",
			lenp, TMU_MAX_SIZE)
		return 0, NewMessageError("WriteTmu", str)
	}

	// hw := bytes.NewBuffer(make([]byte, 0, TMU_HEADER_SIZE))
	// writeElements(hw, hdr.id, hdr.cmd, uint32(lenp),
	// 	hdr.tmu_num, hdr.tmu_idx)

	result := make([]byte, TMU_HEADER_SIZE)
	// hw := bytes.NewBuffer(make([]byte, 0, TMU_HEADER_SIZE))

	binary.LittleEndian.PutUint32(result[0:4], hdr.cmd)
	binary.LittleEndian.PutUint32(result[4:8], hdr.length)
	binary.LittleEndian.PutUint32(result[8:12], hdr.id)
	binary.LittleEndian.PutUint32(result[12:16], hdr.tmu_num)
	binary.LittleEndian.PutUint32(result[16:20], hdr.tmu_idx)

	totalBytes := 0

	n, err := iostream.Write(result)
	totalBytes += n
	if err != nil {
		return totalBytes, err
	}

	if len(payload) > 0 {
		n, err = iostream.Write(payload)
		totalBytes += n
	}

	return totalBytes, err
}

func ReadTmu(iostream io.Reader) (int, *tmuHeader, []byte, error) {

	totalBytes := 0
	n, hdr, err := ReadTmuHeader(iostream)
	totalBytes += n
	if err != nil {
		return totalBytes, nil, nil, err
	}

	if hdr.length > TMU_MAX_SIZE {
		str := fmt.Sprintf("Tmu payload is too large - header "+
			"indicates %d bytes, but max message payload is %d bytes.",
			hdr.length, TMU_MAX_SIZE)
		return totalBytes, nil, nil, NewMessageError("ReadTmu", str)
	}

	if hdr.tmu_num > 1 &&
		(hdr.tmu_idx > hdr.tmu_num || hdr.tmu_idx == 0) {
		str := fmt.Sprintf("Tmu payload idx is wrong - tmu idx %d(%d).",
			hdr.tmu_num, hdr.tmu_idx)
		return totalBytes, nil, nil, NewMessageError("ReadTmu", str)
	}

	err := isCmdValid(hdr.cmd)
	if err != nil {
		discardInput(iostream, hdr.length)
		return totalBytes, nil, nil, err
	}

	payload := make([]byte, hdr.length)
	n, err = io.ReadFull(iostream, payload)
	totalBytes += n
	if err != nil {
		return totalBytes, nil, nil, err
	}

	return totalBytes, hdr, payload, nil
}
