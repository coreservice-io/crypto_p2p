package peer

// import (
// 	"net"
// 	"sync"

// 	"github.com/btcsuite/btcd/peer"
// 	"github.com/coreservice-io/log"
// )

// type PeerMgr struct {
// 	log           log.Logger
// 	peer_version  uint32
// 	inbound_peers sync.Map
// 	outboud_peers sync.Map
// 	seeds         []PeerAddr
// }

// var pm *PeerMgr

// func InitPeerMgr(peer_version uint32, seeds []string, log log.Logger) (*PeerMgr, error) {

// 	//decode seeds =>PeerAddr here
// 	return &PeerMgr{peer_version: peer_version, log: log}, nil
// }

// func GetPeerMgr() *PeerMgr {
// 	return pm
// }

// func (peer_mgr *PeerMgr) Version() uint32 {
// 	return peer_mgr.peer_version
// }

// func (peer_mgr *PeerMgr) AddPeer(p *Peer) {
// 	if p.inbound {
// 		peer_mgr.inbound_peers.Store(p.id, p)
// 	} else {
// 		peer_mgr.outboud_peers.Store(p.id, p)
// 	}
// }

// func (peer_mgr *PeerMgr) RemovePeer(p *Peer) {
// 	if p.inbound {
// 		peer_mgr.inbound_peers.Delete(p.id)
// 	} else {
// 		peer_mgr.outboud_peers.Delete(p.id)
// 	}
// 	p.Close()
// }

// func (peer_mgr *PeerMgr) startServer(listenAddr string) error {
// 	listener, err := net.Listen("tcp", listenAddr)
// 	if err != nil {
// 		peer_mgr.log.Errorln(err)
// 		return err
// 	}
// 	go func() {
// 		conn, err := listener.Accept()
// 		if err != nil {
// 			return
// 		}

// 		inbound_peer := peer.NewPeer()

// 		// Create and start the inbound peer.
// 		//p := peer.NewInboundPeer(cfg)
// 		//p.AttachConnection(conn)
// 	}()

// 	return nil
// }
