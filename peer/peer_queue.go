package peer

import (
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire/msg"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

func (p *Peer) isAllowedReadError(err error) bool {
	if _, ok := err.(*wirebase.MessageError); !ok {
		return false
	}

	host, _, err := net.SplitHostPort(p.literalAddr)
	if err != nil {
		return false
	}

	if host != "127.0.0.1" && host != "localhost" {
		return false
	}

	return true
}

func (p *Peer) shouldHandleReadError(err error) bool {
	if atomic.LoadInt32(&p.connected) != 1 {
		return false
	}

	if err == io.EOF {
		return false
	}
	if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
		return false
	}

	return true
}

func (p *Peer) MsgCheckError(err error) bool {
	if err != nil {
		if p.isAllowedReadError(err) {
			llog.Debugln("Allowed test error from %s: %v", p, err)
			return true
		}

		if err == msg.ErrUnknownMessage {
			llog.Debugln("Received unknown message from %s: %v", p, err)
			return true
		}

		if p.shouldHandleReadError(err) {
			errMsg := fmt.Sprintf("Can't read message from %s: %v", p, err)
			if err != io.ErrUnexpectedEOF {
				llog.Debugln(errMsg)
			}
		}

		return false
	}

	return true
}

func (p *Peer) maybeAddDeadline(pendingResponses map[uint32]time.Time, msg_id uint32) {
	stallResponseTimeout := 30 * time.Second

	deadline := time.Now().Add(stallResponseTimeout)
	pendingResponses[msg_id] = deadline
}

func (p *Peer) stallHandler() {

	pendingResponses := make(map[uint32]time.Time)

	stallTickInterval := 15 * time.Second

	stallTicker := time.NewTicker(stallTickInterval)
	defer stallTicker.Stop()

out:
	for {
		select {
		case tmu_hdr := <-p.stallControl:

			msg_id := tmu_hdr.Id()
			if tmu_hdr.SeqNo() != tmu_hdr.SeqLen() {
				p.maybeAddDeadline(pendingResponses, msg_id)
			} else {
				delete(pendingResponses, msg_id)
			}

		case <-stallTicker.C:
			now := time.Now()

			for msg_id, deadline := range pendingResponses {
				if now.Before(deadline) {
					continue
				}

				// timeout occure
				p.message_map.Delete(msg_id)
				delete(pendingResponses, msg_id)

				break
			}

		case <-p.inQuit:
			break out
		}
	}

cleanup:
	for {
		select {
		case <-p.stallControl:
		default:
			break cleanup
		}
	}
	llog.Traceln(fmt.Sprintf("stall handler done for %s", p))
}

func (p *Peer) inHandler() {

	llog.Traceln("start inHandler")

	// message loop
out:
	for atomic.LoadInt32(&p.connected) == 1 {

		tmu_hdr, tmu_payload, err := wirebase.ReadTrafficUnit(p.conn)
		if !p.MsgCheckError(err) {
			break out
		}

		if tmu_hdr.Cmd() == msg.CMD_RESP {
			err = p.recieve_response_handle(tmu_hdr, tmu_payload)
		} else {
			err = p.recieve_request_handle(tmu_hdr, tmu_payload)
		}
		if !p.MsgCheckError(err) {
			break out
		}
	}

	p.Close()

	close(p.inQuit)
	llog.Traceln("ihandler done. %v", p)
}

func (p *Peer) recieve_response_handle(tmu_hdr *wirebase.TmuHeader, tmu_payload []byte) error {

	r_msg, ok := p.request_map.Load(tmu_hdr.Id())
	if !ok {
		return fmt.Errorf("unexpected message of head %v", tmu_hdr)
	}

	msgOutWrap := r_msg.(*msg.MsgOutWrap)

	msg_payload, err := wirebase.CombineTmu(msgOutWrap.Resp, tmu_hdr, tmu_payload)
	if err != nil {
		return err
	}
	if msg_payload == nil {
		return nil
	}

	msgOutWrap.Knock <- true
	return nil
}

func (p *Peer) recieve_request_handle(tmu_hdr *wirebase.TmuHeader, tmu_payload []byte) error {

	p.stallControl <- tmu_hdr

	msg_payload, err := wirebase.CombineTmuWithMap(p.message_map, tmu_hdr, tmu_payload)
	if err != nil {
		return err
	}
	if msg_payload == nil {
		return nil
	}

	cmd := tmu_hdr.Cmd()

	if handler := p.peer_mgr.GetHandler(cmd); handler != nil {
		go func() {
			ctx := msg.NewHandlerMsgCtx()

			err = handler(msg_payload, ctx, p)
			if err != nil {
				return
			}

			out_resp := msg.NewMsgRespWrap(tmu_hdr.Id(), msg.CMD_RESP, ctx.Req)

			p.SendMessage(out_resp)
		}()
	} else {
		llog.Debugln("Received unhandled message of type %d from %v", cmd, p)
	}

	return nil
}

func (p *Peer) shouldLogWriteError(err error) bool {
	if atomic.LoadInt32(&p.connected) != 1 {
		return false
	}

	if err == io.EOF {
		return false
	}
	if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
		return false
	}

	return true
}

func (p *Peer) SendMessage(message *msg.MsgOutWrap) {

	err := p.writeMsgOutWrap(message)
	if err != nil {
		p.Close()
		if p.shouldLogWriteError(err) {
			llog.Errorln("Failed to send message to %s: %v", p, err)
		}
	}
}
