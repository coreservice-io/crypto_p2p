package cmd

import (
	"errors"

	"github.com/coreservice-io/crypto_p2p/msg"
)

type CmdHandShake_REQ struct {
	Net_magic uint32
	Port      uint32
}

func (cmd_req *CmdHandShake_REQ) Decode(from []byte) error {
	if len(from) != 8 {
		return errors.New("len error")
	}
	cmd_req.Net_magic = msg.DecodeUint32(from[0:4])
	cmd_req.Port = msg.DecodeUint32(from[4:8])
	return nil
}

func (cmd_req *CmdHandShake_REQ) Encode() []byte {
	result := make([]byte, 8)
	msg.EncodeUint32(cmd_req.Net_magic, result[0:4])
	msg.EncodeUint32(cmd_req.Port, result[4:8])
	return result
}

func (cmd_req *CmdHandShake_REQ) Cmd() uint32 {
	return CMD_HANDSHAKE
}

///////////////////////////

type CmdHandShake_RESP struct {
	Net_magic uint32
}

func (cmd_req *CmdHandShake_RESP) Encode() []byte {
	result := make([]byte, 4)
	msg.EncodeUint32(cmd_req.Net_magic, result)
	return result
}

func (cmd_resp *CmdHandShake_RESP) Decode(from []byte) error {
	if len(from) != 4 {
		return errors.New("len error")
	}
	cmd_resp.Net_magic = msg.DecodeUint32(from)
	return nil
}
