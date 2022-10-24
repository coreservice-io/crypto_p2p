package peer

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync/atomic"

	"github.com/coreservice-io/crypto_p2p/wire"
	"github.com/coreservice-io/crypto_p2p/wire/msg"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

func (p *Peer) Connected() bool {
	return atomic.LoadInt32(&p.connected) != 0
}

func (p *Peer) AttachConn(conn net.Conn, cfg *PeerConfig) error {
	if !atomic.CompareAndSwapInt32(&p.connected, 0, 1) {
		return errors.New("Already Attach Conn")
	}

	p.loadCfg(cfg)

	p.conn = conn

	if p.inbound {
		addr := p.conn.RemoteAddr()
		p.literalAddr = addr.String()

		var err error
		p.na, err = wire.NewPeerAddress(addr)
		if err != nil {
			p.Close()
			return err
		}
	}

	if err := p.start(); err != nil {
		llog.Errorln(fmt.Sprintf("%v Start Error. Reason: %s", p, err))
		p.Close()
		return err
	}
	return nil
}

func (p *Peer) Close() {
	if atomic.CompareAndSwapInt32(&p.connected, 1, 0) {
		llog.Traceln("Disconnecting %s", p)
		p.conn.Close()
	}

	close(p.quit)
}

func (p *Peer) WaitForClose() {
	<-p.quit
}

func (p *Peer) readTrafficUnit() (*wirebase.TmuHeader, []byte, error) {
	hdr, tmu_payload, err := wirebase.ReadTrafficUnit(p.conn)

	return hdr, tmu_payload, err
}

func (p *Peer) writeMessage(cmd uint32, payload []byte) error {
	if atomic.LoadInt32(&p.connected) != 1 {
		return nil
	}

	id := rand.Uint32()

	_, err := wirebase.WriteMessage(p.conn, id, cmd, payload)

	return err
}

func (p *Peer) writeMsgOutWrap(msgOutWrap *msg.MsgOutWrap) error {
	if atomic.LoadInt32(&p.connected) != 1 {
		return nil
	}

	payload := msgOutWrap.Req

	llog.Traceln(fmt.Sprintf("sending msg cmd %d", msgOutWrap.Cmd))

	_, err := wirebase.WriteMessage(p.conn, msgOutWrap.Id(), msgOutWrap.Cmd, payload)

	return err
}
