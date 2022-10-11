package msg

import "encoding/binary"

func EncodeUint32(from uint32, to []byte) {
	binary.LittleEndian.PutUint32(to, from)
}

func DecodeUint32(from []byte) uint32 {
	return binary.LittleEndian.Uint32(from)
}
