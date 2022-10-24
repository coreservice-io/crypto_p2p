package msg

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

type MsgVersion struct {
	ProtocolVersion uint32
	Magic           uint32
	Port            uint32
	Nonce           uint64
}

func (msg *MsgVersion) Encode(w io.Writer, pver uint32) error {

	result := make([]byte, 20)

	binary.LittleEndian.PutUint32(result[0:4], msg.ProtocolVersion)
	binary.LittleEndian.PutUint32(result[4:8], msg.Magic)
	binary.LittleEndian.PutUint32(result[8:12], msg.Port)
	binary.LittleEndian.PutUint64(result[12:20], msg.Nonce)

	w.Write(result)

	return nil
}

func (msg *MsgVersion) Decode(r io.Reader, pver uint32) error {
	buf, ok := r.(*bytes.Buffer)
	if !ok {
		return fmt.Errorf("MsgVersion.Decode reader is not a *bytes.Buffer")
	}

	var err error

	err = wirebase.ReadElements(buf, &msg.ProtocolVersion)
	if err != nil {
		return err
	}

	err = wirebase.ReadElements(buf, &msg.Magic)
	if err != nil {
		return err
	}

	err = wirebase.ReadElements(buf, &msg.Port)
	if err != nil {
		return err
	}

	err = wirebase.ReadElements(buf, &msg.Nonce)
	if err != nil {
		return err
	}

	return nil
}

func (msg *MsgVersion) Command() uint32 {
	return CMD_VERSION
}

func NewMsgVersion(magic uint32, port uint32, nonce uint64, protocolVersion uint32) *MsgVersion {

	return &MsgVersion{
		ProtocolVersion: protocolVersion,
		Magic:           magic,
		Port:            port,
		Nonce:           nonce,
	}
}
