package peer

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync/atomic"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
	"github.com/coreservice-io/crypto_p2p/wire/wmsg"
)

// waits for the next message to arrive from the remote peer.
// If the next message is not a version message or the version is not
// acceptable then return an error.
func (p *Peer) readRemoteVersionMsg() error {
	// Read their version message.
	remoteMsg, _, err := p.readMessage(wirebase.LatestEncoding)
	if err != nil {
		return err
	}

	// Notify and disconnect clients if the first message is not a version message.
	msg, ok := remoteMsg.(*wmsg.MsgVersion)
	if !ok {
		reason := "a version message must precede all others"
		rejectMsg := wmsg.NewMsgReject(msg.Command(), wmsg.RejectMalformed, reason)
		_ = p.writeMessage(rejectMsg, wirebase.LatestEncoding)
		return errors.New(reason)
	}

	// Detect self connections.
	if !p.cfg.AllowSelfConns && sentNonces.Contains(msg.Nonce) {
		return errors.New("disconnecting peer connected to self")
	}

	// Negotiate the protocol version and set the services to what the remote peer advertised.
	p.flagsMtx.Lock()
	p.advertisedProtoVer = uint32(msg.ProtocolVersion)
	p.protocolVersion = minUint32(p.protocolVersion, p.advertisedProtoVer)
	p.versionKnown = true
	p.flagsMtx.Unlock()
	log.Debugf("Negotiated protocol version %d for peer %s", p.protocolVersion, p)

	// Updating a bunch of stats including block based stats, and the
	// peer's time offset.
	p.statsMtx.Lock()
	p.timeOffset = msg.Timestamp.Unix() - time.Now().Unix()
	p.statsMtx.Unlock()

	// Set the peer's ID, user agent, and potentially the flag which
	// specifies the witness support is enabled.
	p.flagsMtx.Lock()
	p.id = atomic.AddInt32(&nodeCount, 1)
	p.userAgent = msg.UserAgent
	p.flagsMtx.Unlock()

	// Notify and disconnect clients that have a protocol version that is too old.
	if uint32(msg.ProtocolVersion) < MinAcceptableProtocolVersion {
		// Send a reject message indicating the protocol version is
		// obsolete and wait for the message to be sent before disconnecting.
		reason := fmt.Sprintf("protocol version must be %d or greater", MinAcceptableProtocolVersion)
		rejectMsg := wmsg.NewMsgReject(msg.Command(), wmsg.RejectObsolete, reason)
		_ = p.writeMessage(rejectMsg, wirebase.LatestEncoding)
		return errors.New(reason)
	}

	return nil
}

// processRemoteVerAckMsg takes the verack from the remote peer and handles it.
func (p *Peer) processRemoteVerAckMsg(msg *wmsg.MsgVerAck) {
	p.flagsMtx.Lock()
	p.verAckReceived = true
	p.flagsMtx.Unlock()
}

// creates a version message that can be used to send to the remote peer.
func (p *Peer) localVersionMsg() (*wmsg.MsgVersion, error) {

	theirNA := p.na

	// If we are behind a proxy and the connection comes from the proxy then
	// we return an unroutable address as their address. This is to prevent
	// leaking the tor proxy address.
	if p.cfg.Proxy != "" {
		proxyaddress, _, err := net.SplitHostPort(p.cfg.Proxy)
		// invalid proxy means poorly configured, be on the safe side.
		if err != nil || p.na.Addr.String() == proxyaddress {
			emptyIp := net.IP([]byte{0, 0, 0, 0})
			theirNA = wire.NewNetAddressIPPort(emptyIp, 0)
		}
	}

	// Create a wire.NetAddress to use as the
	// "addrme" in the version message.
	//
	// Older nodes previously added the IP and port information to the
	// address manager which proved to be unreliable as an inbound
	// connection from a peer didn't necessarily mean the peer itself
	// accepted inbound connections.
	//
	// Also, the timestamp is unused in the version message.
	ourNA := &wire.NetAddress{}

	// Generate a unique nonce for this peer so self connections can be detected.
	// This is accomplished by adding it to a size-limited map of recently seen nonces.
	nonce := uint64(rand.Int63())
	sentNonces.Add(nonce)

	// Version message.
	msg := wmsg.NewMsgVersion(ourNA.ToBytes(), theirNA.ToBytes(), nonce, int32(p.cfg.ProtocolVersion))
	msg.AddUserAgent(p.cfg.UserAgentName, p.cfg.UserAgentVersion, []string{""}...)

	return msg, nil
}

// writes our version message to the remote peer.
func (p *Peer) writeLocalVersionMsg() error {
	versionMsg, err := p.localVersionMsg()
	if err != nil {
		return err
	}

	return p.writeMessage(versionMsg, wirebase.LatestEncoding)
}

// writes our sendaddr message to the remote peer if the
// peer supports protocol version 70016 and above.
func (p *Peer) writeSendAddrMsg(pver uint32) error {
	sendAddrMsg := wmsg.NewMsgSendAddr()
	return p.writeMessage(sendAddrMsg, wirebase.LatestEncoding)
}

// waits until desired negotiation messages are received,
// recording the remote peer's preference for sendaddr as an example.
// The list of negotiated features can be expanded in the future. If a
// verack is received, negotiation stops and the connection is live.
func (p *Peer) waitToFinishNegotiation(pver uint32) error {
	// There are several possible messages that can be received here.
	// We could immediately receive verack and be done with the handshake.
	// We could receive sendaddr and still have to wait for verack. Or we
	// can receive unknown messages before and after sendaddr and still
	// have to wait for verack.
	for {
		remoteMsg, _, err := p.readMessage(wirebase.LatestEncoding)
		if err == wmsg.ErrUnknownMessage {
			continue
		} else if err != nil {
			return err
		}

		switch m := remoteMsg.(type) {
		case *wmsg.MsgSendAddr:
			return nil
		case *wmsg.MsgVerAck:
			// Receiving a verack means we are done with the
			// handshake.
			p.processRemoteVerAckMsg(m)
			return nil
		default:
			// This is triggered if the peer sends, for example, a
			// GETDATA message during this negotiation.
			return wmsg.ErrInvalidHandshake
		}
	}
}

// performs the negotiation protocol for an inbound peer.
// The events should occur in the following order, otherwise an error is returned:
//
//   1. Remote peer sends their version.
//   2. We send our version.
//   3. We send sendaddrv.
//   4. We send our verack.
//   5. Wait until sendaddrv or verack is received. Unknown messages are
//      skipped as it could be wtxidrelay or a different message in the future
//   6. If remote peer sent sendaddr above, wait until receipt of verack.
func (p *Peer) negotiateInboundProtocol() error {
	if err := p.readRemoteVersionMsg(); err != nil {
		return err
	}

	if err := p.writeLocalVersionMsg(); err != nil {
		return err
	}

	var protoVersion uint32
	p.flagsMtx.Lock()
	protoVersion = p.protocolVersion
	p.flagsMtx.Unlock()

	if err := p.writeSendAddrMsg(protoVersion); err != nil {
		return err
	}

	err := p.writeMessage(wmsg.NewMsgVerAck(), wirebase.LatestEncoding)
	if err != nil {
		return err
	}

	// Finish the negotiation by waiting for negotiable messages or verack.
	return p.waitToFinishNegotiation(protoVersion)
}

// performs the negotiation protocol for an outbound peer.
// The events should occur in the following order, otherwise an error is returned:
//
//   1. We send our version.
//   2. Remote peer sends their version.
//   3. We send sendaddrv2.
//   4. We send our verack.
//   5. We wait to receive sendaddrv2 or verack, skipping unknown messages as
//      in the inbound case.
//   6. If sendaddr was received, wait for receipt of verack.
func (p *Peer) negotiateOutboundProtocol() error {
	if err := p.writeLocalVersionMsg(); err != nil {
		return err
	}

	if err := p.readRemoteVersionMsg(); err != nil {
		return err
	}

	var protoVersion uint32
	p.flagsMtx.Lock()
	protoVersion = p.protocolVersion
	p.flagsMtx.Unlock()

	if err := p.writeSendAddrMsg(protoVersion); err != nil {
		return err
	}

	err := p.writeMessage(wmsg.NewMsgVerAck(), wirebase.LatestEncoding)
	if err != nil {
		return err
	}

	// Finish the negotiation by waiting for negotiable messages or verack.
	return p.waitToFinishNegotiation(protoVersion)
}
