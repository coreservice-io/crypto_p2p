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

const PEER_HAND_SHAKE_TIMEOUT = 15 * time.Second

type PeerMgr struct {
	ib_peers sync.Map
	ob_peers sync.Map

	MsgHandlers     sync.Map
	message_handler *msg.MessageHandler

	// config
	version    uint32
	listen     string
	seed_addrs []wire.PeerAddr
}

type MessageHandlerFunc func(msg.Message, *Peer) error

func InitPeerMgr(version uint32, net_magic uint32, listen string, seeds string) (*PeerMgr, error) {

	seed_addrs, err := wire.ParsePeerAddr(seeds)
	if err != nil {
		return nil, err
	}

	message_handler, err := msg.InitMessageHandler()
	if err != nil {
		return nil, err
	}
	message_handler.RegMessage(&msg.MsgPing{})

	peer_mgr := &PeerMgr{
		version:         version,
		listen:          listen,
		seed_addrs:      seed_addrs,
		message_handler: message_handler,
	}
	return peer_mgr, nil
}

func (pm *PeerMgr) Seeds() []wire.PeerAddr {
	return pm.seed_addrs
}

// register a msg
func (pm *PeerMgr) RegRequestHandler(message msg.Message, callback MessageHandlerFunc) error {
	if err := pm.message_handler.RegMessage(message); err != nil {
		return errors.New("cmd handler overlap")
	}
	return nil
}

func (pm *PeerMgr) Start() error {

	pm.setupInboundServer(pm.listen)

	pm.setupOutboundPeer()
	return nil
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

			p := NewInboundPeer(nil)
			err = p.AttachConn(conn)
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
		p, err := NewOutboundPeer(nil, addr.String())
		if err != nil {
			fmt.Printf("NewOutboundPeer: error %v\n", err)
			continue
		}

		// Establish the connection to the peer address.
		conn, err := net.Dial("tcp", p.Addr())
		if err != nil {
			fmt.Printf("net.Dial: error %v\n", err)
			continue
		}
		p.AttachConn(conn)
		err = pm.AddPeer(p)
		if err != nil {
			p.Close()
			fmt.Printf("net.Dial: error %v\n", err)
		}
	}

	return nil
}

func (pm *PeerMgr) AddPeer(p *Peer) error {

	var peer_pool sync.Map
	if p.inbound {
		peer_pool = pm.ib_peers
	} else {
		peer_pool = pm.ob_peers
	}

	if _, exists := peer_pool.LoadOrStore(p.na.host, p); exists {
		return errors.New("ip overlap")
	}

	return nil
}

func (pm *PeerMgr) RemovePeer(p *Peer) {
	var peer_pool sync.Map
	if p.inbound {
		peer_pool = pm.ib_peers
	} else {
		peer_pool = pm.ob_peers
	}

	peer_pool.Delete(p.na.host)
	p.Close()
}

func (pm *PeerMgr) RegHandler(msg_cmd uint32, handler MessageHandlerFunc) {
	pm.MsgHandlers.Store(msg_cmd, handler)
}

func (pm *PeerMgr) GetHandler(msg_cmd uint32) MessageHandlerFunc {
	if h_i, ok := pm.MsgHandlers.Load(msg_cmd); ok {
		return h_i.(MessageHandlerFunc)
	}
	return nil
}
