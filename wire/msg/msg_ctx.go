package msg

import (
	"bytes"
	"math/rand"
)

type MsgOutWrap struct {
	id    uint32
	Cmd   uint32
	Req   []byte
	Resp  *bytes.Buffer
	Knock chan bool
}

func (m *MsgOutWrap) Id() uint32 {
	return m.id
}

func NewEmptyMsgReqWrap(cmd uint32, data []byte) *MsgOutWrap {
	return &MsgOutWrap{
		id:    rand.Uint32(),
		Cmd:   cmd,
		Req:   data,
		Resp:  bytes.NewBuffer([]byte{}),
		Knock: make(chan bool, 1),
	}
}

func NewMsgRespWrap(id uint32, cmd uint32, data []byte) *MsgOutWrap {
	return &MsgOutWrap{
		id:  id,
		Cmd: cmd,
		Req: data,
	}
}

type HandlerMsgCtx struct {
	Req []byte
}

func NewHandlerMsgCtx() *HandlerMsgCtx {
	return &HandlerMsgCtx{}
}

func (ctx *HandlerMsgCtx) Send(data []byte) *HandlerMsgCtx {
	ctx.Req = data
	return ctx
}
