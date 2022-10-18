package peer

import (
	"time"

	"github.com/coreservice-io/crypto_p2p/wire/msg"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

const (
	pingInterval = 2 * time.Minute
)

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
				log.Errorf("Not sending ping to %s: %v", p, err)
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

	// Arguably we could use a buffered channel here sending data
	// in a fifo manner whenever we send a ping, or a list keeping track of
	// the times of each ping. For now we just make a best effort and
	// only record stats if it was for the last ping sent.
	// Any preceding and overlapping pings will be ignored. It is unlikely to occur
	// without large usage of the ping rpc call since we ping infrequently
	// enough that if they overlap we would have timed out the peer.
	p.statsMtx.Lock()
	if p.lastPingNonce != 0 && message.Nonce == p.lastPingNonce {
		p.lastPingMicros = time.Since(p.lastPingTime).Nanoseconds()
		p.lastPingMicros /= 1000 // convert to usec.
		p.lastPingNonce = 0
	}
	p.statsMtx.Unlock()

	return nil
}
