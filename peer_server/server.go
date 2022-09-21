package main

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/coreservice-io/crypto_p2p/addrmgr"
	"github.com/coreservice-io/crypto_p2p/connmgr"
	"github.com/coreservice-io/crypto_p2p/peer"
	"github.com/coreservice-io/crypto_p2p/protocol"
	"github.com/coreservice-io/log"
	"github.com/decred/dcrd/lru"
)

type server struct {
	addrManager *addrmgr.AddrManager
	connManager *connmgr.ConnManager

	newPeers  chan *serverPeer
	donePeers chan *serverPeer
	banPeers  chan *serverPeer
	query     chan interface{}

	wg   sync.WaitGroup
	quit chan struct{}

	log log.Logger
}

type serverPeer struct {
	feeFilter int64

	*peer.Peer

	connReq    *connmgr.ConnReq
	server     *server
	persistent bool

	addressesMtx   sync.RWMutex
	knownAddresses lru.Cache
	banScore       connmgr.DynamicBanScore
	quit           chan struct{}
}

func newServerPeer(s *server, isPersistent bool) *serverPeer {
	return &serverPeer{
		server:         s,
		persistent:     isPersistent,
		knownAddresses: lru.NewCache(5000),
		quit:           make(chan struct{}),
	}
}

func (sp *serverPeer) addKnownAddresses(addresses []*protocol.NetAddress) {
	sp.addressesMtx.Lock()
	for _, na := range addresses {
		sp.knownAddresses.Add(addrmgr.NetAddressKey(na))
	}
	sp.addressesMtx.Unlock()
}

// addressKnown true if the given address is already known to the peer.
func (sp *serverPeer) addressKnown(na *protocol.NetAddress) bool {
	sp.addressesMtx.RLock()
	exists := sp.knownAddresses.Contains(addrmgr.NetAddressKey(na))
	sp.addressesMtx.RUnlock()
	return exists
}

type simpleAddr struct {
	net, addr string
}

func (a simpleAddr) String() string {
	return a.addr
}

func (a simpleAddr) Network() string {
	return a.net
}

func parseListeners(addrs []string) ([]net.Addr, error) {
	netAddrs := make([]net.Addr, 0, len(addrs))
	for _, addr := range addrs {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		if host == "" {
			netAddrs = append(netAddrs, simpleAddr{net: "tcp4", addr: addr})
			continue
		}

		ip := net.ParseIP(host)
		if ip == nil {
			return nil, fmt.Errorf("'%s' is not a valid IP address", host)
		}

		if ip.To4() == nil {
			return nil, fmt.Errorf("'%s' is not a valid IPv4 address", host)
		} else {
			netAddrs = append(netAddrs, simpleAddr{net: "tcp4", addr: addr})
		}
	}
	return netAddrs, nil
}

func initListeners(amgr *addrmgr.AddrManager, listenAddrs []string) ([]net.Listener, error) {
	netAddrs, err := parseListeners(listenAddrs)
	if err != nil {
		return nil, err
	}

	listeners := make([]net.Listener, 0, len(netAddrs))
	for _, addr := range netAddrs {
		listener, err := net.Listen(addr.Network(), addr.String())
		if err != nil {
			fmt.Errorf("Can't listen on %s: %v", addr, err)
			continue
		}
		listeners = append(listeners, listener)
	}

	for _, listener := range listeners {
		addr := listener.Addr().String()
		err := addLocalAddress(amgr, addr)
		if err != nil {
			fmt.Errorf("Skipping address %s: %v", addr, err)
		}
	}

	return listeners, nil
}

func addLocalAddress(addrMgr *addrmgr.AddrManager, addr string) error {
	// host, portStr, err := net.SplitHostPort(addr)
	// if err != nil {
	//	return err
	// }
	// port, err := strconv.ParseUint(portStr, 10, 16)
	// if err != nil {
	// 	return err
	// }

	// netAddr, err := addrMgr.HostToNetAddress(host, uint16(port))
	// if err != nil {
	// 	return err
	// }

	// addrMgr.AddLocalAddress(netAddr, addrmgr.BoundPrio)

	return nil
}

type getOutboundGroup struct {
	key   string
	reply chan int
}

func (s *server) OutboundGroupCount(key string) int {
	replyChan := make(chan int)
	s.query <- getOutboundGroup{key: key, reply: replyChan}
	return <-replyChan
}

func (s *server) inboundPeerConnected(conn net.Conn) {
	sp := newServerPeer(s, false)
	sp.Peer = peer.NewInboundPeer(newPeerConfig(sp))
	// sp.AssociateConnection(conn)
	go s.peerDoneHandler(sp)
}

func (s *server) outboundPeerConnected(c *connmgr.ConnReq, conn net.Conn) {
	sp := newServerPeer(s, c.Permanent)
	p, err := peer.NewOutboundPeer(newPeerConfig(sp), c.Addr.String())
	if err != nil {
		s.log.Debugln("create outbound peer failed %s: %v", c.Addr, err)
		if c.Permanent {
			s.connManager.Disconnect(c.ID())
		} else {
			s.connManager.Remove(c.ID())
			go s.connManager.NewConnReq()
		}
		return
	}
	sp.Peer = p
	sp.connReq = c

	go s.peerDoneHandler(sp)
}

func (s *server) peerDoneHandler(sp *serverPeer) {
	sp.WaitForDisconnect()
	s.donePeers <- sp
	close(sp.quit)
}

func newPeerConfig(sp *serverPeer) *peer.Config {
	return nil
}

func addrStringToNetAddr(addr string) (net.Addr, error) {
	host, strPort, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	port, err := strconv.Atoi(strPort)
	if err != nil {
		return nil, err
	}

	// Skip an IP address.
	if ip := net.ParseIP(host); ip != nil {
		return &net.TCPAddr{
			IP:   ip,
			Port: port,
		}, nil
	}

	// Look up with the parsed host.
	ips, err := msndLookup(host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no address for %s", host)
	}

	return &net.TCPAddr{
		IP:   ips[0],
		Port: port,
	}, nil
}

func newServer(listenAddrs []string, interrupt <-chan struct{}) (*server, error) {
	// instance address manager.
	addrMgr := addrmgr.New(cfg.DataPath, msndLookup)

	var listeners []net.Listener
	var err error
	listeners, err = initListeners(addrMgr, listenAddrs)
	if err != nil {
		return nil, err
	}

	if len(listeners) == 0 {
		return nil, errors.New("listen address is not valid")
	}

	s := server{
		addrManager: addrMgr,

		newPeers:  make(chan *serverPeer, cfg.MaxPeers),
		donePeers: make(chan *serverPeer, cfg.MaxPeers),
		banPeers:  make(chan *serverPeer, cfg.MaxPeers),
		query:     make(chan interface{}),
		quit:      make(chan struct{}),
	}

	var newAddressFunc func() (net.Addr, error)
	if len(cfg.ConnectPeers) == 0 {
		newAddressFunc = func() (net.Addr, error) {
			// TODO: try from am address list
			/*for tries := 0; tries < 100; tries++ {
				addr := s.addrManager.GetAddress()
				if addr == nil {
					break
				}

				if s.OutboundGroupCount(addr.na) != 0 {
					continue
				}

				// failed 30 times only allow recent 10mins
				if tries < 30 && time.Since(addr.LastAttempt()) < 10*time.Minute {
					continue
				}

				// Mark an attempt for the valid address.
				s.addrManager.Attempt(addr.NetAddress())

				addrString := addrmgr.NetAddressKey(addr.NetAddress())
				return addrStringToNetAddr(addrString)
			}*/

			return nil, errors.New("no valid connect address")
		}
	}

	targetOutbound := defaultTargetOutbound
	if cfg.MaxPeers < targetOutbound {
		targetOutbound = cfg.MaxPeers
	}

	// instance connection manager.
	cmgr, err := connmgr.New(&connmgr.Config{
		Listeners:      listeners,
		OnAccept:       s.inboundPeerConnected,
		RetryDuration:  connectionRetryInterval,
		TargetOutbound: uint32(targetOutbound),
		Dial:           msndDial,
		OnConnection:   s.outboundPeerConnected,
		GetNewAddress:  newAddressFunc,
	})
	if err != nil {
		return nil, err
	}
	s.connManager = cmgr

	// start persistent peers.
	permanentPeers := cfg.ConnectPeers
	if len(permanentPeers) == 0 {
		permanentPeers = cfg.AddPeers
	}
	for _, addr := range permanentPeers {
		netAddr, err := addrStringToNetAddr(addr)
		if err != nil {
			return nil, err
		}

		go s.connManager.Connect(&connmgr.ConnReq{
			Addr:      netAddr,
			Permanent: true,
		})
	}

	return &s, nil
}

func (s *server) Stop() error {
	s.log.Infoln("Server stop done")
	return nil
}

func (s *server) Start() error {
	s.log.Infoln("Server start...")
	return nil
}

func (s *server) WaitForShutdown() error {
	s.log.Infoln("Server wait for shutdown")
	return nil
}
