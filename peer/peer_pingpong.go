package peer

import (
	"time"

	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
	"github.com/coreservice-io/crypto_p2p/wire/wmsg"
)

const (
	// is the interval of time to wait in between sending ping messages.
	pingInterval = 2 * time.Minute
)

// periodically pings the peer.
// It must be run as a goroutine.
func (p *Peer) pingHandler() {
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

			p.QueueMessage(wmsg.NewMsgPing(nonce), nil)

		case <-p.quit:
			break out
		}
	}
}

// invoked when a peer receives a pong message.
// It updates the ping statistics as required for recent clients.
func (p *Peer) handlePongMsg(msg *wmsg.MsgPong) {
	// Arguably we could use a buffered channel here sending data
	// in a fifo manner whenever we send a ping, or a list keeping track of
	// the times of each ping. For now we just make a best effort and
	// only record stats if it was for the last ping sent.
	// Any preceding and overlapping pings will be ignored. It is unlikely to occur
	// without large usage of the ping rpc call since we ping infrequently
	// enough that if they overlap we would have timed out the peer.
	p.statsMtx.Lock()
	if p.lastPingNonce != 0 && msg.Nonce == p.lastPingNonce {
		p.lastPingMicros = time.Since(p.lastPingTime).Nanoseconds()
		p.lastPingMicros /= 1000 // convert to usec.
		p.lastPingNonce = 0
	}
	p.statsMtx.Unlock()
}
