package peer

import (
	"sync"

	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

type PeerManager struct {
	MsgHandlers sync.Map //msg_cmd => MsgHandlerFunc
}

type MsgHandlerFunc func(wirebase.Message, *Peer) error

var peer_manager *PeerManager = &PeerManager{}

func GetPeerManager() *PeerManager {
	return peer_manager
}

func (pm *PeerManager) RegHandler(msg_cmd uint32, handler MsgHandlerFunc) {
	pm.MsgHandlers.Store(msg_cmd, handler)
}

func (pm *PeerManager) GetHandler(msg_cmd uint32) MsgHandlerFunc {
	if h_i, ok := pm.MsgHandlers.Load(msg_cmd); ok {
		return h_i.(MsgHandlerFunc)
	}
	return nil
}
