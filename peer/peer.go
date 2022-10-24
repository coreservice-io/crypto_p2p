package peer

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire"
	"github.com/coreservice-io/crypto_p2p/wire/msg"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"

	"github.com/decred/dcrd/lru"
)

const (
	// the lowest protocol version that a peer may support.
	MinAcceptVersion = 0

	// is the duration of inactivity before we time out a peer.
	idleTimeout = 5 * time.Minute
)

const DEFAULT_REQUEST_TIMEOUT_SECS = 60
const REQUEST_TIMEOUT_SECS_MAX = 600 //request max timeout

var (
	// sentNonces houses the unique nonces that are generated when pushing
	// version messages that are used to detect self connections.
	sentNonces = lru.NewCache(50)
)

func minUint32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

type Peer struct {
	inbound bool

	// These fields are set at creation time and never modified,
	// so they are safe to read from concurrently without a mutex.
	literalAddr string

	conn net.Conn

	connected int32

	message_map *sync.Map
	request_map *sync.Map

	peer_mgr *PeerMgr

	na *wire.PeerAddr

	// config
	magic           uint32
	protocolVersion uint32
	shake_timeout   time.Duration
	allowSelfConns  bool

	stallControl chan *wirebase.TmuHeader
	inQuit       chan struct{} // if inHandler routine quit
	quit         chan struct{} // if peer quit
}

func (p *Peer) loadCfg(pmCfg *PeerConfig) {
	p.magic = pmCfg.magic
	p.protocolVersion = pmCfg.version
	p.shake_timeout = pmCfg.shake_timeout
}

func (p *Peer) PutMgr(pm *PeerMgr) {
	p.peer_mgr = pm
}

func (p *Peer) String() string {
	return fmt.Sprintf("%s (%s)", p.literalAddr, directionString(p.inbound))
}

func directionString(inbound bool) string {
	if inbound {
		return "inbound"
	}
	return "outbound"
}

func (p *Peer) Addr() string {
	return p.literalAddr
}

func (p *Peer) Inbound() bool {
	return p.inbound
}

func (p *Peer) ProtocolVersion() uint32 {
	return p.protocolVersion
}

func newPeer(inbound bool) *Peer {
	return &Peer{
		inbound:     inbound,
		message_map: new(sync.Map),
		request_map: new(sync.Map),

		stallControl:   make(chan *wirebase.TmuHeader, 1), // nonblocking sync
		inQuit:         make(chan struct{}),
		quit:           make(chan struct{}),
		allowSelfConns: true,
	}
}

func NewInboundPeer() *Peer {
	return newPeer(true)
}

func NewOutboundPeer(remoteAddr string) (*Peer, error) {
	p := newPeer(false)
	p.literalAddr = remoteAddr

	host, portStr, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return nil, err
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	p.na, _ = wire.NewPeerAddrStr(
		string(host),
		uint16(port),
	)

	return p, nil
}

func (p *Peer) start() error {
	llog.Traceln("Starting peer %v", p)

	err := p.handshake()
	if err != nil {
		return err
	}

	llog.Traceln("Connected to %v", p)

	go p.stallHandler()
	go p.inHandler()

	// light business
	go p.pingHandler()

	return nil
}

func (p *Peer) Send(cmd uint32, content []byte) ([]byte, error) {
	return p.SendWithTimeout(cmd, content, DEFAULT_REQUEST_TIMEOUT_SECS)
}

func (p *Peer) SendWithTimeout(cmd uint32, data []byte, timeout_secs uint32) ([]byte, error) {

	if timeout_secs <= 0 || timeout_secs > REQUEST_TIMEOUT_SECS_MAX {
		return nil, errors.New("timeout_secs must within range (0, REQUEST_TIMEOUT_SECS_MAX] ")
	}

	msg_reqWrap := msg.NewEmptyMsgReqWrap(cmd, data)

	p.request_map.Store(msg_reqWrap.Id(), msg_reqWrap)
	defer p.request_map.Delete(msg_reqWrap.Id())

	p.SendMessage(msg_reqWrap)

	select {
	case done := <-msg_reqWrap.Knock:
		if !done {
			return nil, errors.New("failed")
		}
		return msg_reqWrap.Resp.Bytes(), nil

	case <-time.After(time.Second * time.Duration(timeout_secs)):
		return nil, msg.ErrTimeoutMessage
	}
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
