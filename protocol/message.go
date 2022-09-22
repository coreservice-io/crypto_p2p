package protocol

import (
	"io"
	"net"
)

const (
	CmdVersion = "version"
	CmdGetAddr = "getaddr"
	CmdAddr    = "addr"
	CmdPing    = "ping"
	CmdPong    = "pong"
)

type Message interface {
	Decode(io.Reader, uint32) error
	Encode(io.Writer, uint32) error
	Command() string
	MaxPayloadLength(uint32) uint32
}

//////////////////////////////////////////////////////////////////
type MsgGetAddr struct{}

func NewMsgGetAddr() *MsgGetAddr {
	return &MsgGetAddr{}
}

func (msg *MsgGetAddr) Decode(r io.Reader, pver uint32) error {
	return nil
}

func (msg *MsgGetAddr) Encode(w io.Writer, pver uint32) error {
	return nil
}

func (msg *MsgGetAddr) Command() string {
	return CmdGetAddr
}

func (msg *MsgGetAddr) MaxPayloadLength(pver uint32) uint32 {
	return 0
}

////////////////////////////////////////////////////////////////
type MsgAddr struct {
	AddrList []*NetAddress
}

type NetAddress struct {
	Addr net.Addr
	Port uint16
}

func NewMsgAddr() *MsgAddr {
	return &MsgAddr{}
}

func (msg *MsgAddr) Decode(r io.Reader, pver uint32) error {
	return nil
}

func (msg *MsgAddr) Encode(w io.Writer, pver uint32) error {
	return nil
}

func (msg *MsgAddr) Command() string {
	return CmdAddr
}

func (msg *MsgAddr) MaxPayloadLength(pver uint32) uint32 {
	return 0
}

//////////////////////////////////////////////////////////////////
type MsgVersion struct {
	Version int32
}

func NewMsgVersion() *MsgVersion {
	return &MsgVersion{}
}

func (msg *MsgVersion) Decode(r io.Reader, pver uint32) error {
	return nil
}

func (msg *MsgVersion) Encode(w io.Writer, pver uint32) error {
	return nil
}

func (msg *MsgVersion) Command() string {
	return CmdVersion
}

func (msg *MsgVersion) MaxPayloadLength(pver uint32) uint32 {
	return 0
}

//////////////////////////////////////////////////////////////////
type MsgPing struct {
	Nonce uint64
}

func NewMsgPing() *MsgPing {
	return &MsgPing{}
}

func (msg *MsgPing) Decode(r io.Reader, pver uint32) error {
	return nil
}

func (msg *MsgPing) Encode(w io.Writer, pver uint32) error {
	return nil
}

func (msg *MsgPing) Command() string {
	return CmdPing
}

func (msg *MsgPing) MaxPayloadLength(pver uint32) uint32 {
	return 0
}

//////////////////////////////////////////////////////////////////
type MsgPong struct {
	Nonce uint64
}

func NewMsgPong() *MsgPong {
	return &MsgPong{}
}

func (msg *MsgPong) Decode(r io.Reader, pver uint32) error {
	return nil
}

func (msg *MsgPong) Encode(w io.Writer, pver uint32) error {
	return nil
}

func (msg *MsgPong) Command() string {
	return CmdPong
}

func (msg *MsgPong) MaxPayloadLength(pver uint32) uint32 {
	return 0
}
