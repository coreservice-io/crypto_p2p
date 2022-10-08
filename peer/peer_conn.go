package peer

import (
	"bytes"
	"fmt"
	"sync/atomic"

	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
	"github.com/coreservice-io/crypto_p2p/wire/wmsg"

	"github.com/davecgh/go-spew/spew"
)

// Connected returns whether or not the peer is currently connected.
// This function is safe for concurrent access.
func (p *Peer) Connected() bool {
	return atomic.LoadInt32(&p.connected) != 0 &&
		atomic.LoadInt32(&p.disconnect) == 0
}

// Disconnect disconnects the peer by closing the connection.  Calling this
// function when the peer is already disconnected or in the process of
// disconnecting will have no effect.
func (p *Peer) Disconnect() {
	if !atomic.CompareAndSwapInt32(&p.disconnect, 0, 1) {
		return
	}

	log.Tracef("Disconnecting %s", p)
	if atomic.LoadInt32(&p.connected) != 0 {
		p.conn.Close()
	}
	close(p.quit)
}

// reads the next message from the peer with logging.
func (p *Peer) readMessage(enc wirebase.MessageEncoding) (wirebase.Message, []byte, error) {
	n, msg, buf, err := wirebase.ReadMessageWithEncodingN(p.conn,
		p.ProtocolVersion(), p.cfg.NetMagic, wmsg.MakeEmptyMessage, enc)
	atomic.AddUint64(&p.bytesReceived, uint64(n))

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
	if atomic.LoadInt32(&p.disconnect) != 0 {
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
	atomic.AddUint64(&p.bytesSent, uint64(n))

	if p.cfg.OnMessageHook.OnWrite != nil {
		p.cfg.OnMessageHook.OnWrite(p, n, msg, err)
	}
	return err
}
