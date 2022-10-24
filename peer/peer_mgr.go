package peer

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire"
	"github.com/coreservice-io/crypto_p2p/wire/msg"
)

const PEER_HAND_SHAKE_TIMEOUT_SEC = 15

type PeerConfig struct {
	version       uint32
	magic         uint32
	shake_timeout time.Duration
}

type PeerMgr struct {
	ib_peers sync.Map
	ob_peers sync.Map

	message_handler *sync.Map

	config     PeerConfig
	listen     string
	seed_addrs []wire.PeerAddr
}

type MessageHandlerFunc func([]byte, *msg.HandlerMsgCtx, *Peer) error

func InitPeerMgr(version uint32, net_magic uint32, listen string, seeds string) (*PeerMgr, error) {

	seed_addrs, err := wire.ParsePeerAddr(seeds)
	if err != nil {
		return nil, err
	}

	pmCfg := &PeerConfig{
		version:       version,
		magic:         net_magic,
		shake_timeout: PEER_HAND_SHAKE_TIMEOUT_SEC,
	}

	peer_mgr := &PeerMgr{
		config:          *pmCfg,
		listen:          listen,
		seed_addrs:      seed_addrs,
		message_handler: new(sync.Map),
	}
	return peer_mgr, nil
}

func (pm *PeerMgr) Seeds() []wire.PeerAddr {
	return pm.seed_addrs
}

func (pm *PeerMgr) IbPeers() *sync.Map {
	return &pm.ib_peers
}

func (pm *PeerMgr) Start() error {

	pm.initStartupHandler()

	pm.setupInboundServer(pm.listen)

	pm.setupOutboundPeer()
	return nil
}

func (pm *PeerMgr) initStartupHandler() {
	llog.Traceln("init handler when startup")

	pm.RegHandler(msg.CMD_PING, handlePingMsg)
}

func (pm *PeerMgr) setupInboundServer(listenAddr string) error {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}

	go func() {

		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			p := NewInboundPeer()
			err = p.AttachConn(conn, &pm.config)
			if err != nil {
				return
			}

			err = pm.AddPeer(p)
			if err != nil {
				p.Close()
				return
			}
		}
	}()

	return nil
}

func (pm *PeerMgr) setupOutboundPeer() error {

	for _, addr := range pm.seed_addrs {

		p, err := NewOutboundPeer(addr.String())
		if err != nil {
			llog.Errorln(fmt.Sprintf("NewOutboundPeer error. Reason: %v", err))
			continue
		}

		conn, err := net.Dial("tcp", p.Addr())
		if err != nil {
			llog.Errorln(fmt.Sprintf("[peer] Outbound error. Reason: %v", err))
			continue
		}

		p.AttachConn(conn, &pm.config)

		err = pm.AddPeer(p)
		if err != nil {
			p.Close()
			llog.Errorln(fmt.Sprintf("[peer] Outbound error. Reason: %v", err))
		}
	}

	return nil
}

func (pm *PeerMgr) AddPeer(p *Peer) error {

	var peer_pool *sync.Map
	if p.inbound {
		peer_pool = &pm.ib_peers
	} else {
		peer_pool = &pm.ob_peers
	}

	p.PutMgr(pm)
	if _, exists := peer_pool.LoadOrStore(p.na.Host(), p); exists {
		return errors.New("ip overlap")
	}

	return nil
}

func (pm *PeerMgr) RemovePeer(p *Peer) {
	var peer_pool *sync.Map
	if p.inbound {
		peer_pool = &pm.ib_peers
	} else {
		peer_pool = &pm.ob_peers
	}

	peer_pool.Delete(p.na.Host())
	p.Close()
}

func (pm *PeerMgr) RegHandler(msg_cmd uint32, handler MessageHandlerFunc) {
	pm.message_handler.Store(msg_cmd, handler)

	// if _, ok := hm.handlers[msg_cmd]; ok {
	// 	return errors.New("cmd handler overlap")
	// }
	// hm.handlers[msg_cmd] = message
	// return nil
}

func (pm *PeerMgr) GetHandler(msg_cmd uint32) MessageHandlerFunc {
	if h_i, ok := pm.message_handler.Load(msg_cmd); ok {
		llog.Traceln(fmt.Sprintf("find handler for cmd %d", msg_cmd))

		return h_i.(MessageHandlerFunc)
	}

	llog.Traceln(fmt.Sprintf("can't find handler for cmd %d", msg_cmd))

	return nil
}
