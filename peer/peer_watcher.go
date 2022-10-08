package peer

import (
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

// defines callback function pointers to invoke with message listeners for a peer.
type MessageWatcher struct {
	OnRead func(p *Peer, bytesRead int, msg wirebase.Message, err error)

	OnWrite func(p *Peer, bytesWritten int, msg wirebase.Message, err error)
}
