package peer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
	"github.com/coreservice-io/crypto_p2p/wire/wmsg"

	"github.com/decred/dcrd/lru"
)

const (
	// the lowest protocol version that a peer may support.
	MinAcceptableProtocolVersion = 0

	// is the duration of inactivity before we timeout a
	// peer that hasn't completed the initial version negotiation.
	negotiateTimeout = 30 * time.Second

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

// minUint32 is a helper function to return the minimum of two uint32s.
// This avoids a math import and the need to cast to floats.
func minUint32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// Config is the struct to hold configuration options useful to Peer.
type Config struct {
	NetMagic wirebase.NetMagic

	// specifies the protocol version to use and advertise.
	ProtocolVersion uint32

	// callback functions to be invoked on receiving peer messages.
	OnMessageHook MessageWatcher

	// AllowSelfConns is only used to allow the tests to bypass the self
	// connection detecting and disconnect logic since they intentionally
	// do so for testing purposes.
	AllowSelfConns bool
}

// NOTE: The overall data flow of a peer is split into 3 goroutines.
// Inbound messages are read via the inHandler goroutine
// and generally dispatched to their own handler.
// For inbound data-related messages, the data is handled by the corresponding
// message handlers.
// The data flow for outbound messages is split into 2 goroutines, queueHandler and outHandler.
// The first, queueHandler, is used as a way for external entities to queue messages,
// by way of the QueueMessage function,
// quickly regardless of whether the peer is currently sending or not.
// It acts as the traffic cop between the external world and the actual
// goroutine which writes to the network socket.

// Peer provides a basic concurrent safe peer for handling communications
// via the peer-to-peer protocol.
// It provides full duplex reading and writing,
// automatic handling of the initial handshake process,
// querying of usage statistics and other information about the remote peer such
// as its address, user agent, and protocol version, output message queuing,
// inventory trickling, and the ability to dynamically register and unregister
// callbacks for handling protocol messages.
//
// Outbound messages are typically queued via QueueMessage.
// QueueMessage is intended for all messages.
// However, some helper functions for pushing messages
// of specific types that typically require common special handling are
// provided as a convenience.
type Peer struct {
	inbound bool

	// These fields are set at creation time and never modified,
	// so they are safe to read from concurrently without a mutex.
	literalAddr string
	cfg         Config

	conn     net.Conn
	ioreader io.Reader
	iowriter io.Writer

	connected   int32
	connectedAt time.Time

	longMessages  sync.Map
	requestRouter sync.Map

	flagsMtx        sync.Mutex       // protects the peer flags below
	na              *wire.NetAddress // Y
	protocolVersion uint32           // Y negotiated protocol version

	// These fields keep track of statistics for the peer and are protected
	// by the statsMtx mutex.
	statsMtx sync.RWMutex

	lastPingNonce  uint64    // Set to nonce if we have a pending ping.
	lastPingTime   time.Time // Time we sent last ping.
	lastPingMicros int64     // Time for last ping to return.

	stallControl  chan stallControlMsg
	outputQueue   chan outMsg   // msg -> sending queue
	sendQueue     chan outMsg   // sending queue -> send
	sendDoneQueue chan struct{} // sending done notify
	queueQuit     chan struct{} // if queue routine quit
	inQuit        chan struct{} // if inHandler routine quit
	outQuit       chan struct{} // if outHandler routine quit
	quit          chan struct{} // if peer quit
}

// String returns the peer's address and directionality as a human-readable string.
func (p *Peer) String() string {
	return fmt.Sprintf("%s (%s)", p.literalAddr, directionString(p.inbound))
}

// Addr returns the peer address.
//
// This function is safe for concurrent access.
func (p *Peer) Addr() string {
	// The address doesn't change after initialization, therefore it is not
	// protected by a mutex.
	return p.literalAddr
}

// returns whether the peer is inbound.
func (p *Peer) Inbound() bool {
	return p.inbound
}

// ProtocolVersion returns the negotiated peer protocol version.
func (p *Peer) ProtocolVersion() uint32 {
	// p.flagsMtx.Lock()
	return p.protocolVersion
}

// returns a new base peer based on the inbound flag.  This
// is used by the NewInboundPeer and NewOutboundPeer functions to perform base
// setup needed by both types of peers.
func newPeerBase(origCfg *Config, inbound bool) *Peer {
	// Default to the max supported protocol version if not specified by the caller.
	cfg := *origCfg // Copy to avoid mutating caller.

	p := Peer{
		inbound:       inbound,
		stallControl:  make(chan stallControlMsg, 1), // nonblocking sync
		outputQueue:   make(chan outMsg, 50),
		sendQueue:     make(chan outMsg, 1),   // nonblocking sync
		sendDoneQueue: make(chan struct{}, 1), // nonblocking sync
		inQuit:        make(chan struct{}),
		queueQuit:     make(chan struct{}),
		outQuit:       make(chan struct{}),
		quit:          make(chan struct{}),
		cfg:           cfg, // Copy so caller can't mutate.
	}
	return &p
}

// returns a new inbound peer.
// Use Start to begin processing incoming and outgoing messages.
func NewInboundPeer(cfg *Config) *Peer {
	return newPeerBase(cfg, true)
}

// returns a new outbound peer.
// connecting to anything other than an ipv4 or
// ipv6 address will fail and may cause a nil-pointer-dereference.
// This includes hostnames and onion services.
func NewOutboundPeer(cfg *Config, remoteAddr string) (*Peer, error) {
	p := newPeerBase(cfg, false)
	p.literalAddr = remoteAddr

	host, portStr, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return nil, err
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	// If host is an onion hidden service or a hostname,
	// it is likely that a nil-pointer-dereference will occur.
	p.na = wire.NetAddressFromBytes(
		time.Now(), net.ParseIP(host), uint16(port),
	)

	return p, nil
}

// start begins processing input and output messages.
func (p *Peer) start() error {
	log.Tracef("Starting peer %s", p)

	negotiateErr := make(chan error, 1)
	go func() {
		if p.inbound {
			negotiateErr <- p.negotiateInboundProtocol()
		} else {
			negotiateErr <- p.negotiateOutboundProtocol()
		}
	}()

	select {
	case err := <-negotiateErr:
		if err != nil {
			p.Disconnect()
			return err
		}
	case <-time.After(negotiateTimeout):
		p.Disconnect()
		return errors.New("protocol negotiation timeout")
	}
	log.Debugf("Connected to %s", p.Addr())

	// The protocol has been negotiated successfully
	// so start processing input and output messages.
	go p.stallHandler()
	go p.inHandler()
	go p.queueHandler()
	go p.outHandler()

	// light business
	go p.pingHandler()

	return nil
}

func (p *Peer) Send(msg_cmd uint32, raw_msg []byte) ([]byte, error) {

	p_msg := &wmsg.MsgData{
		Id:         rand.Uint32(),
		Msg_cmd:    msg_cmd,
		Msg_chunks: make(chan int32, wirebase.MSG_DATA_CHUNKS_NUM+1), // +1 for eof
		Msg_buffer: bytes.NewBuffer([]byte{}),
	}

	p.requestRouter.Store(p_msg.Id, p_msg)
	defer p.requestRouter.Delete(p_msg.Id)

	// encode the message and send chunk by chunk with writetime out
	p.QueueMessage(wmsg.NewMsgData(raw_msg), nil)

	// after send finished,
	// start to receive from msg_buffer chunk by chunk with readtimeout
	r, r_err := p_msg.Receive()
	if r_err != nil {
		return nil, r_err
	}

	return r, nil
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
