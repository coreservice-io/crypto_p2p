package peer

import (
	"errors"
	"net"
	"time"

	"github.com/coreservice-io/crypto_p2p/protocol"
	"github.com/coreservice-io/log"
)

const (
	shakeTimeout = 30 * time.Second
)

type Peer struct {
	conn    net.Conn
	addr    string
	cfg     Config
	inbound bool
	log     log.Logger
	quit    chan struct{}

	/*
		stallControl  chan stallControlMsg
		outputQueue   chan outMsg
		sendQueue     chan outMsg
		sendDoneQueue chan struct{}
		outputInvChan chan *wire.InvVect
		inQuit        chan struct{}
		queueQuit     chan struct{}
		outQuit       chan struct{}
		quit          chan struct{}
	*/
}

type Config struct {
	listeners Messagelistners // callback when recv peer data
}

type Messagelistners struct {
	OnGetAddr func(p *Peer, msg *protocol.MsgGetAddr)
	OnAddr    func(p *Peer, msg *protocol.MsgAddr)
	OnPing    func(p *Peer, msg *protocol.MsgPing)
	OnPong    func(p *Peer, msg *protocol.MsgPong)
	OnVersion func(p *Peer, msg *protocol.MsgVersion)
}

func newPeerBase(config *Config, inbound bool) *Peer {
	return &Peer{
		cfg:     *config,
		inbound: inbound,
	}
}

func NewInboundPeer(cfg *Config) *Peer {
	return newPeerBase(cfg, true)
}

func NewOutboundPeer(cfg *Config, addr string) (*Peer, error) {
	p := newPeerBase(cfg, false)
	p.addr = addr

	/* host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	if cfg.HostToNetAddress != nil {
		na, err := cfg.HostToNetAddress(host, uint16(port), 0)
		if err != nil {
			return nil, err
		}
		p.na = na
	} */

	return p, nil
}

func (p *Peer) shakeInbound() error {
	return nil
}

func (p *Peer) shakeOutbound() error {
	return nil
}

// start begins processing input and output messages.
func (p *Peer) start() error {
	p.log.Traceln("Starting peer %s", p)

	shakeErr := make(chan error)
	go func() {
		if p.inbound {
			shakeErr <- p.shakeInbound()
		} else {
			shakeErr <- p.shakeInbound()
		}
	}()

	select {
	case err := <-shakeErr:
		if err != nil {
			return err
		}
	case <-time.After(shakeTimeout):
		return errors.New("shake timeout")
	}
	p.log.Debugln("Connected to %s", p.addr)

	// go p.stallHandler()  // stall detection
	// go p.inHandler()     // incoming messages, callback with swith-case msg type.
	// go p.queueHandler()  // queuing of outgoing data, sendQueue send msg to outHandler.
	// go p.outHandler()    // outgoing messages, read queue msg and write to peer. notify stallHandler.
	// go p.pingHandler()   // periodically pings, QueueMessage ping with nonce.

	// p.QueueMessage(protocol.NewMsgVersion(), nil)
	return nil
}

func (p *Peer) WaitForDisconnect() {
	<-p.quit
}
