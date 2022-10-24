package peer

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire/msg"
)

const pingInterval = 1 * time.Minute

func (p *Peer) pingHandler() {

	llog.Traceln("start ping %v", p)

	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

out:
	for {
		select {
		case <-pingTicker.C:
			nonce := uint64(rand.Int63())

			b := make([]byte, 8)
			binary.LittleEndian.PutUint64(b, nonce)

			llog.Traceln("sending ping %v", p)

			res, err := p.SendWithTimeout(msg.CMD_PING, b, 10)
			if err != nil {
				llog.Errorln("can't ping to %s: %v", p, err)
				p.Close()
				break
			}

			pnonce := binary.LittleEndian.Uint64(res)
			llog.Traceln("receive %s: receive %v. %v", nonce, pnonce, p)

		case <-p.quit:
			break out
		}
	}
}

func handlePingMsg(msg_payload []byte, ctx *msg.HandlerMsgCtx, p *Peer) error {

	pnonce := binary.LittleEndian.Uint64(msg_payload)

	llog.Traceln(fmt.Sprintf("%s recieve ping: %v", p, pnonce))

	ctx.Send(msg_payload)

	return nil
}
