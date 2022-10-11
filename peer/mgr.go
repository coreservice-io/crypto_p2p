package peer

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/coreservice-io/crypto_p2p/cmd"
	"github.com/coreservice-io/log"
)

const PEER_BOOST_TIMEOUT_SECS = 15

type PeerMgr struct {
	log           log.Logger
	peer_version  uint32
	inbound_peers sync.Map
	outboud_peers sync.Map
	seeds         []PeerAddr
	net_magic     uint32
}

var pm *PeerMgr

func InitPeerMgr(peer_version uint32, net_magic uint32, seeds []string, log log.Logger) (*PeerMgr, error) {

	//decode seeds =>PeerAddr here
	return &PeerMgr{peer_version: peer_version, log: log}, nil
}

func GetPeerMgr() *PeerMgr {
	return pm
}

func (peer_mgr *PeerMgr) Version() uint32 {
	return peer_mgr.peer_version
}

func (peer_mgr *PeerMgr) AddPeer(p *Peer) error {
	if p.inbound {
		if _, loaded := peer_mgr.inbound_peers.LoadOrStore(p.addr.ipv4, p); loaded {
			return errors.New("ip overlap")
		}
	} else {
		if _, loaded := peer_mgr.outboud_peers.LoadOrStore(p.addr.ipv4, p); loaded {
			return errors.New("ip overlap")
		}
	}

	return nil
}

func (peer_mgr *PeerMgr) RemovePeer(p *Peer) {
	if p.inbound {
		peer_mgr.inbound_peers.Delete(p.addr.ipv4)
	} else {
		peer_mgr.outboud_peers.Delete(p.addr.ipv4)
	}
	p.Close()
}

func (peer_mgr *PeerMgr) startServer(listenAddr string) error {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		peer_mgr.log.Errorln(err)
		return err
	}

	go func() {

		for {

			conn, err := listener.Accept()
			if err != nil {
				return
			}

			peer_mgr.SetupInboundPeer(conn)
		}

	}()

	return nil
}

func (peer_mgr *PeerMgr) SetupInboundPeer(conn net.Conn) error {

	inbound_peer := NewPeer()
	inbound_peer.inbound = true

	conn_addr, err := ParsePeerAddr(conn.RemoteAddr().String())
	if err != nil {
		return err
	}
	inbound_peer.addr = conn_addr
	inbound_peer.SetConn(conn)

	err = peer_mgr.AddPeer(inbound_peer)
	if err != nil {
		return err
	}

	//////////////////////////////
	//handshake and further setup
	go func() {

		inbound_peer.RegRequestHandler(cmd.CMD_HANDSHAKE, func(req []byte) []byte {
			cmd_req := &cmd.CmdHandShake_REQ{}
			err := cmd_req.Decode(req)
			if err == nil || cmd_req.Net_magic == peer_mgr.net_magic {
				inbound_peer.boosted <- struct{}{}
			}

			return (&cmd.CmdHandShake_RESP{Net_magic: peer_mgr.net_magic}).Encode()
		})

		err := inbound_peer.Start()
		if err != nil {
			peer_mgr.log.Errorln(err)
		}

		select {
		case <-time.After(time.Duration(PEER_BOOST_TIMEOUT_SECS) * time.Second):
			peer_mgr.RemovePeer(inbound_peer)

		case <-inbound_peer.boosted:
			peer_mgr.log.Infoln("inbound peer boosted,ipv4:", inbound_peer.addr.ipv4)
			//reg more functions
			//reg ping/pong hearbeat
			//........
		}

	}()

	return nil

}
