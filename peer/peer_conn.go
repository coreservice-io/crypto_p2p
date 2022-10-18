package peer

import (
	"bytes"
	"errors"
	"net"
	"sync/atomic"

	"github.com/coreservice-io/crypto_p2p/wire"
	"github.com/coreservice-io/crypto_p2p/wire/msg"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

func (p *Peer) Connected() bool {
	return atomic.LoadInt32(&p.connected) != 0
}

func (p *Peer) AttachConn(conn net.Conn) error {
	if !atomic.CompareAndSwapInt32(&p.connected, 0, 1) {
		return errors.New("Already Attach Conn")
	}
	p.conn = conn

	if p.inbound {
		addr := p.conn.RemoteAddr()
		p.literalAddr = addr.String()

		na, err := wire.NewNetAddress(addr)
		if err != nil {
			p.Close()
			return err
		}
		p.na = na
	}

	if err := p.start(); err != nil {
		p.Close()
		return err
	}
	return nil
}

func (p *Peer) Close() {
	if atomic.CompareAndSwapInt32(&p.connected, 1, 0) {
		log.Debugf("Disconnecting %s", p)
		p.conn.Close()
	}

	close(p.quit)
}

func (p *Peer) WaitForClose() {
	<-p.quit
}

func (p *Peer) readMsgBytes() (uint32, []byte, error) {
	hdr, msg_payload, err := wirebase.ReadMessage(p.conn, p.longMessages)
	if err != nil {
		return 0, nil, err
	}

	return (*hdr).Cmd(), msg_payload, err
}

func (p *Peer) writeMsgBytes(cmd uint32, data []byte, timeout_secs int) error {
	if atomic.LoadInt32(&p.connected) != 1 {
		return nil
	}

	_, err := wirebase.WriteMessage(p.conn, cmd, data, timeout_secs)

	return err
}

func (p *Peer) readMessage() (msg.Message, error) {
	hdr, msg_payload, err := wirebase.ReadMessage(p.conn, p.longMessages)
	if err != nil {
		return nil, err
	}

	message, err := p.peer_mgr.message_handler.MakeEmptyMessage((*hdr).Cmd())
	if err != nil {
		return message, err
	}
	// Decode message.
	pr := bytes.NewBuffer(msg_payload)

	pver := p.ProtocolVersion()
	err = message.Decode(pr, pver)
	if err != nil {
		return nil, err
	}

	return message, nil
}

func (p *Peer) writeMessage(message msg.Message, timeout_secs int) error {
	if atomic.LoadInt32(&p.connected) != 1 {
		return nil
	}

	var bw bytes.Buffer

	pver := p.ProtocolVersion()
	err := message.Encode(&bw, pver)
	if err != nil {
		return err
	}
	payload := bw.Bytes()

	_, err = wirebase.WriteMessage(p.conn, message.Command(), payload, timeout_secs)

	return err
}
