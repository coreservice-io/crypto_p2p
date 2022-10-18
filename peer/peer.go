package peer

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire/msg"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"

	"github.com/decred/dcrd/lru"
)

const (
	// the lowest protocol version that a peer may support.
	MinAcceptVersion = 0

	// is the duration of inactivity before we time out a peer.
	idleTimeout = 5 * time.Minute

	// stallTickInterval is the interval of time between each check for
	// stalled peers.
	stallTickInterval = 15 * time.Second

	// is the base maximum amount of time messages that
	// expect a response will wait before disconnecting the peer for
	// stalling.  The deadlines are adjusted for callback running times and
	// only checked on each stall tick interval.
	stallResponseTimeout = 30 * time.Second
)

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

type Config struct {
	NetMagic uint32

	ProtocolVersion uint32

	AllowSelfConns bool
}

type Peer struct {
	inbound bool

	// These fields are set at creation time and never modified,
	// so they are safe to read from concurrently without a mutex.
	literalAddr string
	cfg         Config

	conn net.Conn

	connected int32

	longMessages  sync.Map
	request_handlers sync.Map

	peer_mgr *PeerMgr

	na              *wire.NetAddress
	protocolVersion uint32          
	allowSelfConns bool

	// These fields keep track of statistics for the peer and are protected
	// by the statsMtx mutex.
	// statsMtx sync.RWMutex

	// lastPingNonce  uint64    // Set to nonce if we have a pending ping.
	// lastPingTime   time.Time // Time we sent last ping.
	// lastPingMicros int64     // Time for last ping to return.

	stallControl  chan stallControlMsg
	outputQueue   chan outMsg   // msg -> sending queue
	sendQueue     chan outMsg   // sending queue -> send
	sendDoneQueue chan struct{} // sending done notify
	queueQuit     chan struct{} // if queue routine quit
	inQuit        chan struct{} // if inHandler routine quit
	outQuit       chan struct{} // if outHandler routine quit
	quit          chan struct{} // if peer quit
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

func newPeer(origCfg *Config, inbound bool) *Peer {
	return &Peer{
		inbound:       inbound,
		stallControl:  make(chan stallControlMsg, 1), // nonblocking sync
		outputQueue:   make(chan outMsg, 50),
		sendQueue:     make(chan outMsg, 1),   // nonblocking sync
		sendDoneQueue: make(chan struct{}, 1), // nonblocking sync
		inQuit:        make(chan struct{}),
		queueQuit:     make(chan struct{}),
		outQuit:       make(chan struct{}),
		quit:          make(chan struct{}),
		allowSelfConns: true,
		cfg:           *origCfg, // Copy so caller can't mutate.
	}
}

func NewInboundPeer(cfg *Config) *Peer {
	return newPeer(cfg, true)
}

func NewOutboundPeer(cfg *Config, remoteAddr string) (*Peer, error) {
	p := newPeer(cfg, false)
	p.literalAddr = remoteAddr

	host, portStr, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return nil, err
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	p.na = wire.NetAddressFromBytes(
		time.Now(), net.ParseIP(host), uint16(port),
	)

	return p, nil
}

func (p *Peer) start() error {
	log.Traceln("Starting peer %s", p)

	err := p.handshake()
	if err != nil {
		return err
	}

	log.Debugln("Connected to %s", p.Addr())

	// go p.stallHandler()
	go p.inHandler()
	go p.outHandler()
	// go p.queueHandler()

	// light business
	go p.pingHandler()

	return nil
}

func (p *Peer) start2() error {
	p.RegRequestHandler(cmd.CMD_HANDSHAKE, func(req []byte) []byte {
		cmd_req := &cmd.CmdHandShake_REQ{}
		err := cmd_req.Decode(req)
		if err == nil || cmd_req.Net_magic == peer_mgr.net_magic {
			p.boosted <- struct{}{}
		}

		return (&cmd.CmdHandShake_RESP{Net_magic: peer_mgr.net_magic}).Encode()
	})

	err := p.StartMsgLoop()
	if err != nil {
		peer_mgr.log.Errorln(err)
	}

	time.After(time.Duration(PEER_BOOST_TIMEOUT_SECS) * time.Second):
	peer_mgr.RemovePeer(p)

	select {
	case <-p.boosted:
		idleTimer.Stop()
		peer_mgr.log.Infoln("inbound peer boosted,ipv4:", p.pna.ipv4)
		//reg more functions
		//reg ping/pong hearbeat
		//........
	}
}

func (p *Peer) Send(cmd uint32, content []byte) ([]byte, error) {
	return p.SendWithTimeout(cmd, content, DEFAULT_REQUEST_TIMEOUT_SECS)
}

func (p *Peer) SendWithTimeout(cmd uint32, data []byte, timeout_secs uint32) ([]byte, error) {

	if timeout_secs <= 0 || timeout_secs > REQUEST_TIMEOUT_SECS_MAX {
		return nil, errors.New("timeout_secs must within range (0, REQUEST_TIMEOUT_SECS_MAX] ")
	}

	p_msg := &msg.MsgData{
		Id:         rand.Uint32(),
		Msg_cmd:    cmd,
		Resp_buffer:				bytes.NewBuffer([]byte{}),
		resp_done:	make(chan bool, 1)
	}

	p.request_handlers.Store(p_msg.Id, p_msg)
	defer p.request_handlers.Delete(p_msg.Id)

	doneChan := make(chan struct{})

	p.QueueMessage(msg.NewMsgData(data), doneChan)

out:
	for {
		select {
		// case <-doneChan:
		// 	isSend=true
		// 	continue out
		case done := <-p_resp.resp_done:
			if !done {
				return nil, errors.New("failed")
			}
			return p_resp.buffer.Body.Bytes(), nil

		case <-time.After(time.Second * timeout_secs):
			if !isSend {
			fmt.Printf("occur timeout")
			return timeouts
			}
		}
	}

	return r, nil
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
