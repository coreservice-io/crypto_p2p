package wirebase

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
)

const TMU_MAX_SIZE = 1024 * 8 // 8KB

// number for maxium size of a single message.
const TMU_MAX_NUM = 1024

const TMU_HEADER_SIZE = 20

const TMU_MAX_PAYLOAD_SIZE = TMU_MAX_SIZE - TMU_HEADER_SIZE

// the maximum bytes of a single message 8MB
const MSG_PAYLOAD_MAX_SIZE = (TMU_MAX_PAYLOAD_SIZE * TMU_MAX_NUM)

type TmuHeader struct {
	cmd     uint32
	length  uint32
	id      uint32
	tmu_idx uint32
	tmu_num uint32
}

func (hdr *TmuHeader) Cmd() uint32 {
	return hdr.cmd
}

func (hdr *TmuHeader) Id() uint32 {
	return hdr.id
}

func (hdr *TmuHeader) SeqNo() uint32 {
	return hdr.tmu_idx
}

func (hdr *TmuHeader) SeqLen() uint32 {
	return hdr.tmu_num
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

func ReadTmuHeader(iostream io.Reader) (int, *TmuHeader, error) {

	header_bytes := tmu_header_pool.Get().([]byte)
	defer tmu_header_pool.Put(header_bytes)

	n, err := io.ReadFull(iostream, header_bytes[:])
	if err != nil {
		return n, nil, err
	}

	if len(header_bytes) != TMU_HEADER_SIZE {
		return n, nil, errors.New("header size err")
	}

	hdr := &TmuHeader{
		cmd:     binary.LittleEndian.Uint32(header_bytes[0:4]),
		length:  binary.LittleEndian.Uint32(header_bytes[4:8]),
		id:      binary.LittleEndian.Uint32(header_bytes[8:12]),
		tmu_num: binary.LittleEndian.Uint32(header_bytes[12:16]),
		tmu_idx: binary.LittleEndian.Uint32(header_bytes[16:20]),
	}

	return n, hdr, nil
}

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

func WriteMessage(w io.Writer, id uint32, cmd uint32, data []byte) (int, error) {

	data_len := len(data)

	if data_len > MSG_PAYLOAD_MAX_SIZE {
		str := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload is %d bytes",
			data_len, MSG_PAYLOAD_MAX_SIZE)
		return 0, NewMessageError("WriteMessage", str)
	}

	chunks := int(math.Ceil(float64(data_len) / (TMU_MAX_PAYLOAD_SIZE)))
	if chunks == 0 {
		chunks = 1
	}

	totalBytes := 0

	for i := 0; i < chunks; i++ {
		idx := i + 1
		start_p := i * TMU_MAX_PAYLOAD_SIZE
		end_p := (i + 1) * TMU_MAX_PAYLOAD_SIZE
		if end_p >= data_len {
			end_p = data_len
		}
		tmu_payload := data[start_p:end_p]

		hdr := &TmuHeader{
			id:      id,
			cmd:     cmd,
			tmu_num: uint32(chunks),
			tmu_idx: uint32(idx),
		}

		n, err := writeTmu(w, hdr, tmu_payload)
		if err != nil {
			return 0, err
		}

		totalBytes += n
	}

	return totalBytes, nil
}

func CombineTmuWithMap(message_map *sync.Map,
	hdr *TmuHeader, payload []byte) ([]byte, error) {
	if hdr.tmu_num == 1 {
		return payload, nil
	}

	if hdr.tmu_num >= hdr.tmu_idx {
		longBuf, exists := message_map.Load(hdr.id)
		if !exists {
			longBuf = bytes.NewBuffer(make([]byte, 0))
			message_map.Store(hdr.id, longBuf)
		}

		return CombineTmu(longBuf.(*bytes.Buffer), hdr, payload)
	}

	// not build msg
	return nil, nil
}

func CombineTmu(longBuf *bytes.Buffer,
	hdr *TmuHeader, payload []byte) ([]byte, error) {

	longBuf.Write(payload)

	if hdr.tmu_num == 1 {
		return payload, nil
	}

	// last one
	if hdr.tmu_num == hdr.tmu_idx {
		payload := longBuf.Bytes()
		return payload, nil
	}

	// not build msg
	return nil, nil
}

func writeTmu(iostream io.Writer, hdr *TmuHeader, payload []byte) (int, error) {

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

func ReadTrafficUnit(iostream io.Reader) (*TmuHeader, []byte, error) {

	_, tmu_hdr, tmu_payload, err := readTmu(iostream)
	if err != nil {
		return nil, tmu_payload, err
	}

	return tmu_hdr, tmu_payload, err
}

func readTmu(iostream io.Reader) (int, *TmuHeader, []byte, error) {

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
		return totalBytes, nil, nil, fmt.Errorf(str)
	}

	if hdr.tmu_num > 1 &&
		(hdr.tmu_idx > hdr.tmu_num || hdr.tmu_idx == 0) {
		str := fmt.Sprintf("Tmu payload idx is wrong - tmu idx %d(%d).",
			hdr.tmu_num, hdr.tmu_idx)
		return totalBytes, nil, nil, fmt.Errorf(str)
	}

	payload := make([]byte, hdr.length)
	n, err = io.ReadFull(iostream, payload)
	totalBytes += n
	if err != nil {
		return totalBytes, nil, nil, err
	}

	return totalBytes, hdr, payload, nil
}
