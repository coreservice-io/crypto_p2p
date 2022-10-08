package peer

import (
	"bytes"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
	"github.com/coreservice-io/crypto_p2p/wire/wmsg"

	"github.com/davecgh/go-spew/spew"
)

func (p *Peer) Connected() bool {
	return atomic.LoadInt32(&p.connected) != 0
}

// attach the given conn to the peer.
func (p *Peer) AttachConnection(conn net.Conn) {
	if !atomic.CompareAndSwapInt32(&p.connected, 0, 1) {
		return
	}

	p.conn = conn
	p.connectedAt = time.Now()

	if p.inbound {

		addr := p.conn.RemoteAddr()
		p.literalAddr = addr.String()

		// Set up a NetAddress for the peer to be used with AddrManager.
		// We only do this inbound because outbound set this up at connection time
		// and no point recomputing.
		na, err := wire.NewNetAddress(addr)
		if err != nil {
			log.Errorf("Cannot create remote net address: %v", err)
			p.Disconnect()
			return
		}

		p.na = na
	}

	go func() {
		if err := p.start(); err != nil {
			log.Debugf("Cannot start peer %v: %v", p, err)
			p.Disconnect()
		}
	}()
}

func (p *Peer) Disconnect() {
	if !atomic.CompareAndSwapInt32(&p.connected, 1, 0) {
		return
	}

	log.Tracef("Disconnecting %s", p)
	p.conn.Close()
	close(p.quit)
}

// waits until the peer has completely disconnected and all
// resources are cleaned up.
// This will happen if either the local or remote side has been
// disconnected or the peer is forcibly disconnected via Disconnect.
func (p *Peer) WaitForDisconnect() {
	<-p.quit
}

// reads the next message from the peer with logging.
func (p *Peer) readMessage(enc wirebase.MessageEncoding) (wirebase.Message, []byte, error) {
	n, msg, buf, err := wirebase.ReadMessageWithEncodingN(p.conn,
		p.ProtocolVersion(), p.cfg.NetMagic, wmsg.MakeEmptyMessage, enc)

	if p.cfg.OnMessageHook.OnRead != nil {
		p.cfg.OnMessageHook.OnRead(p, n, msg, err)
	}

	if err != nil {
		return nil, nil, err
	}

	// Use closures to log expensive operations so they are only run when
	// the logging level requires it.
	log.Debugf("%v", newLogClosure(func() string {
		// Debug summary of message.
		summary := messageSummary(msg)
		if len(summary) > 0 {
			summary = " (" + summary + ")"
		}
		return fmt.Sprintf("Received %v%s from %s",
			msg.Command(), summary, p)
	}))
	log.Tracef("%v", newLogClosure(func() string {
		return spew.Sdump(msg)
	}))
	log.Tracef("%v", newLogClosure(func() string {
		return spew.Sdump(buf)
	}))

	return msg, buf, nil
}

// writeMessage sends a bitcoin message to the peer with logging.
func (p *Peer) writeMessage(msg wirebase.Message, enc wirebase.MessageEncoding) error {
	// Don't do anything if we're disconnecting.
	if atomic.LoadInt32(&p.connected) != 1 {
		return nil
	}

	// Use closures to log expensive operations so they are only run when
	// the logging level requires it.
	log.Debugf("%v", newLogClosure(func() string {
		// Debug summary of message.
		summary := messageSummary(msg)
		if len(summary) > 0 {
			summary = " (" + summary + ")"
		}
		return fmt.Sprintf("Sending %v%s to %s", msg.Command(),
			summary, p)
	}))
	log.Tracef("%v", newLogClosure(func() string {
		return spew.Sdump(msg)
	}))
	log.Tracef("%v", newLogClosure(func() string {
		var buf bytes.Buffer
		_, err := wirebase.WriteMessageWithEncodingN(&buf, msg,
			p.ProtocolVersion(), p.cfg.NetMagic, enc)
		if err != nil {
			return err.Error()
		}
		return spew.Sdump(buf.Bytes())
	}))

	// Write the message to the peer.
	n, err := wirebase.WriteMessageWithEncodingN(p.conn, msg,
		p.ProtocolVersion(), p.cfg.NetMagic, enc)

	if p.cfg.OnMessageHook.OnWrite != nil {
		p.cfg.OnMessageHook.OnWrite(p, n, msg, err)
	}
	return err
}
