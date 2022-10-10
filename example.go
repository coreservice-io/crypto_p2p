package main

import (
	"github.com/coreservice-io/crypto_p2p/peer"
	"github.com/coreservice-io/logrus_log"
)

const peer_version = 1

func main() {

	llog, err := logrus_log.New("./logs", 1, 20, 30)
	if err != nil {
		panic(err.Error())
	}

	peer_mgr, _ := peer.NewPeerMgr(peer_version, []string{}, llog)
	llog.Infoln("peerMgr created , peer_version:", peer_mgr.Version())

	//
}
