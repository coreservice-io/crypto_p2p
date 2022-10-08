package peer

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"

	"github.com/decred/dcrd/lru"
)

const (
	// the max protocol version the peer supports.
	MaxAcceptableProtocolVersion = 100

	// the lowest protocol version that a connected peer may support.
	MinAcceptableProtocolVersion = 0

	// outputBufferSize is the number of elements the output channels use.
	outputBufferSize = 50

	// pingInterval is the interval of time to wait in between sending ping
	// messages.
	pingInterval = 2 * time.Minute

	// negotiateTimeout is the duration of inactivity before we timeout a
	// peer that hasn't completed the initial version negotiation.
	negotiateTimeout = 30 * time.Second

	// idleTimeout is the duration of inactivity before we time out a peer.
	idleTimeout = 5 * time.Minute

	// stallTickInterval is the interval of time between each check for
	// stalled peers.
	stallTickInterval = 15 * time.Second

	// stallResponseTimeout is the base maximum amount of time messages that
	// expect a response will wait before disconnecting the peer for
	// stalling.  The deadlines are adjusted for callback running times and
	// only checked on each stall tick interval.
	stallResponseTimeout = 30 * time.Second
)

var (
	// nodeCount is the total number of peer connections made since startup
	// and is used to assign an id to a peer.
	nodeCount int32

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

	// returns the netaddress for the given host. This can be
	// nil in  which case the host will be parsed as an IP address.
	HostToNetAddress HostToNetAddrFunc

	// Proxy indicates a proxy is being used for connections.  The only
	// effect this has is to prevent leaking the tor proxy address, so it
	// only needs to specified if using a tor proxy.
	Proxy string

	// UserAgentName specifies the user agent name to advertise.
	// It is highly recommended to specify this value.
	UserAgentName string

	// UserAgentVersion specifies the user agent version to advertise.
	// It is highly recommended to specify this value and that it follows the
	// form "major.minor.revision" e.g. "2.6.41".
	UserAgentVersion string

	NetMagic wirebase.NetMagic

	// ProtocolVersion specifies the maximum protocol version to use and advertise.
	//   This field can be omitted in which case
	// peer.MaxAcceptableProtocolVersion will be used.
	ProtocolVersion uint32

	// callback functions to be invoked on receiving peer messages.
	OnMessageHook MessageWatcher

	// AllowSelfConns is only used to allow the tests to bypass the self
	// connection detecting and disconnect logic since they intentionally
	// do so for testing purposes.
	AllowSelfConns bool
}

// func which takes a host, port and returns the netaddress.
type HostToNetAddrFunc func(host string, port uint16) (*wire.NetAddress, error)

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
// via the peer-to-peer protocol.  It provides full duplex
// reading and writing, automatic handling of the initial handshake process,
// querying of usage statistics and other information about the remote peer such
// as its address, user agent, and protocol version, output message queuing,
// inventory trickling, and the ability to dynamically register and unregister
// callbacks for handling protocol messages.
//
// Outbound messages are typically queued via QueueMessage or QueueInventory.
// QueueMessage is intended for all messages.
// QueueInventory, on the other hand, is only intended for relaying inventory
// as it employs a trickling mechanism to batch the inventory together.
// However, some helper functions for pushing messages
// of specific types that typically require common special handling are
// provided as a convenience.
type Peer struct {
	// The following variables must only be used atomically.
	bytesReceived uint64
	bytesSent     uint64
	lastRecv      int64
	lastSend      int64

	// TO combine
	connected  int32 // Y
	disconnect int32 // Y

	inbound bool // Y

	// These fields are set at creation time and never modified,
	// so they are safe to read from concurrently without a mutex.
	literalAddr string // Y
	cfg         Config // Y

	conn net.Conn // Y

	flagsMtx             sync.Mutex       // protects the peer flags below
	na                   *wire.NetAddress // Y
	id                   int32
	userAgent            string
	versionKnown         bool   // Y
	advertisedProtoVer   uint32 // Y protocol version advertised by remote
	protocolVersion      uint32 // Y negotiated protocol version
	sendHeadersPreferred bool   // peer sent a sendheaders message
	verAckReceived       bool
	sendAddrV2           bool

	wireEncoding wirebase.MessageEncoding

	// These fields keep track of statistics for the peer and are protected
	// by the statsMtx mutex.
	statsMtx       sync.RWMutex
	timeOffset     int64
	timeConnected  time.Time
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
	quit          chan struct{} // if peer quit, disconnect used
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

// Inbound returns whether the peer is inbound.
func (p *Peer) Inbound() bool {
	return p.inbound
}

// LastPingNonce returns the last ping nonce of the remote peer.
func (p *Peer) LastPingNonce() uint64 {
	// p.statsMtx.RLock()
	// p.statsMtx.RUnlock()

	return p.lastPingNonce
}

// LastPingTime returns the last ping time of the remote peer.
func (p *Peer) LastPingTime() time.Time {
	return p.lastPingTime
}

// LastPingMicros returns the last ping micros of the remote peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastPingMicros() int64 {
	// p.statsMtx.RLock()
	return p.lastPingMicros
}

// VersionKnown returns the whether or not the version of a peer is known locally.
func (p *Peer) VersionKnown() bool {
	// p.flagsMtx.Lock()
	return p.versionKnown
}

// VerAckReceived returns whether or not a verack message was received by the peer.
func (p *Peer) VerAckReceived() bool {
	// p.flagsMtx.Lock()
	return p.verAckReceived
}

// ProtocolVersion returns the negotiated peer protocol version.
func (p *Peer) ProtocolVersion() uint32 {
	// p.flagsMtx.Lock()
	return p.protocolVersion
}

// WaitForDisconnect waits until the peer has completely disconnected and all
// resources are cleaned up.  This will happen if either the local or remote
// side has been disconnected or the peer is forcibly disconnected via
// Disconnect.
func (p *Peer) WaitForDisconnect() {
	<-p.quit
}

// returns a new base peer based on the inbound flag.  This
// is used by the NewInboundPeer and NewOutboundPeer functions to perform base
// setup needed by both types of peers.
func newPeerBase(origCfg *Config, inbound bool) *Peer {
	// Default to the max supported protocol version if not specified by the caller.
	cfg := *origCfg // Copy to avoid mutating caller.
	// if cfg.ProtocolVersion == 0 {
	// 	cfg.ProtocolVersion = MaxAcceptableProtocolVersion
	// }

	p := Peer{
		inbound:       inbound,
		stallControl:  make(chan stallControlMsg, 1), // nonblocking sync
		outputQueue:   make(chan outMsg, outputBufferSize),
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

	// Negotiate the protocol within the specified negotiateTimeout.
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

	// The protocol has been negotiated successfully so start processing input
	// and output messages.
	go p.stallHandler()
	go p.inHandler()
	go p.queueHandler()
	go p.outHandler()
	go p.pingHandler()

	return nil
}

// AssociateConnection associates the given conn to the peer.
// Calling this function when the peer is already connected will have no effect.
func (p *Peer) AssociateConnection(conn net.Conn) {
	// Already connected?
	if !atomic.CompareAndSwapInt32(&p.connected, 0, 1) {
		return
	}

	p.conn = conn
	p.timeConnected = time.Now()

	if p.inbound {

		addr := p.conn.RemoteAddr()
		p.literalAddr = addr.String()

		// Set up a NetAddress for the peer to be used with AddrManager.
		// We only do this inbound because outbound set this up at connection time
		// and no point recomputing.
		na, err := wire.NewNetAddress(addr)
		if err != nil {
			log.Errorf("Cannot create remote net address: %v", err)
			p.Disconnect()
			return
		}

		p.na = na
	}

	go func() {
		if err := p.start(); err != nil {
			log.Debugf("Cannot start peer %v: %v", p, err)
			p.Disconnect()
		}
	}()
}

// returns a new inbound peer. Use Start to begin
// processing incoming and outgoing messages.
func NewInboundPeer(cfg *Config) *Peer {
	return newPeerBase(cfg, true)
}

// returns a new outbound peer. If the Config argument
// does not set HostToNetAddress, connecting to anything other than an ipv4 or
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

	if cfg.HostToNetAddress != nil {
		na, err := cfg.HostToNetAddress(host, uint16(port))
		if err != nil {
			return nil, err
		}
		p.na = na
	} else {
		// If host is an onion hidden service or a hostname,
		// it is likely that a nil-pointer-dereference will occur.
		// The caller should set HostToNetAddress if connecting to these.
		p.na = wire.NetAddressFromBytes(
			time.Now(), net.ParseIP(host), uint16(port),
		)
	}

	return p, nil
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
