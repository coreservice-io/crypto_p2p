package peer

import (
	"time"

	"github.com/coreservice-io/crypto_p2p/wire/msg"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

const pingInterval = 1 * time.Minute

func (p *Peer) pingHandler() {

	p.peer_mgr.RegHandler(msg.CMD_PING, handlePingMsg)
	p.peer_mgr.RegHandler(msg.CMD_PONG, handlePongMsg)

	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

out:
	for {
		select {
		case <-pingTicker.C:
			nonce, err := wirebase.RandomUint64()
			if err != nil {
				log.Errorln("Not sending ping to %s: %v", p, err)
				continue
			}

			p.statsMtx.Lock()
			p.lastPingNonce = nonce
			p.lastPingTime = time.Now()
			p.statsMtx.Unlock()

			p.QueueMessage(msg.NewMsgPing(nonce), nil)

		case <-p.quit:
			break out
		}
	}
}

func handlePingMsg(m msg.Message, p *Peer) error {

	message := m.(*msg.MsgPing)
	p.QueueMessage(msg.NewMsgPong(message.Nonce), nil)

	return nil
}

func handlePongMsg(m msg.Message, p *Peer) error {

	message := m.(*msg.MsgPong)

	p.statsMtx.Lock()
	if p.lastPingNonce != 0 && message.Nonce == p.lastPingNonce {
		p.lastPingMicros = time.Since(p.lastPingTime).Nanoseconds()
		p.lastPingMicros /= 1000 // convert to usec.
		p.lastPingNonce = 0
	}
	p.statsMtx.Unlock()

	return nil
}

func (p *Peer) pingHandler2() {

	p.peer_mgr.RegHandler(msg.CMD_PING, handlePingMsg2)

	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

out:
	for {
		select {
		case <-pingTicker.C:
			nonce, err := wirebase.RandomUint64()
			if err != nil {
				log.Errorln("Not sending ping to %s: %v", p, err)
				continue
			}

			p.SendWithTimeout(msg.CMD_PING, nonce.bytes, 8)
			if err != nil {
				log.Errorln("Not sending ping to %s: %v", p, err)
				continue
			}

		case <-p.quit:
			break out
		}
	}
}

func handlePingMsg2(m msg.Message, p *Peer) error {

	message := m.(*msg.MsgPing)

	p.writeMessage(omsg.msg)

	p.QueueMessageResp(msg.NewMsgPong(message.Nonce), nil)

	return nil
}
