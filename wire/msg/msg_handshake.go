package msg

import (
	"encoding/binary"
	"errors"
)

type MsgHandShakeOut struct {
	Net_magic uint32
	Port      uint32
}

func (msg *MsgHandShakeOut) Command() uint32 {
	return CMD_HANDSHAKE
}

func (msg *MsgHandShakeOut) Encode() []byte {
	result := make([]byte, 8)
	binary.LittleEndian.PutUint32(result[0:4], msg.Net_magic)
	binary.LittleEndian.PutUint32(result[4:8], msg.Port)
	return result
}

func (msg *MsgHandShakeOut) Decode(from []byte) error {
	if len(from) != 8 {
		return errors.New("len error")
	}
	msg.Net_magic = binary.LittleEndian.Uint32(from[0:4])
	msg.Port = binary.LittleEndian.Uint32(from[4:8])
	return nil
}

///////////////////////////

type MsgHandShakeIn struct {
	Net_magic uint32
}

func (msg *MsgHandShakeIn) Encode() []byte {
	result := make([]byte, 4)
	binary.LittleEndian.PutUint32(result, msg.Net_magic)
	return result
}

func (msg *MsgHandShakeIn) Decode(from []byte) error {
	if len(from) != 4 {
		return errors.New("len error")
	}
	msg.Net_magic = binary.LittleEndian.Uint32(from)
	return nil
}
