package peer

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire/msg"
	"github.com/coreservice-io/crypto_p2p/wire/wmsg"
)

func (p *Peer) readRemoteVersionMsg(magic uint32) error {
	remoteMsg, err := p.readMessage()
	if err != nil {
		return err
	}

	message, ok := remoteMsg.(*msg.MsgVersion)
	if !ok {
		return errors.New("Negotiated error: " +
			"a version message must precede all others")
	}

	if message.Magic != uint32(magic) {
		return errors.New("Negotiated error: " +
			fmt.Sprintf("message from other network [%v]", message.Magic))
	}

	// Detect self connections.
	if !p.allowSelfConns && sentNonces.Contains(message.Nonce) {
		return errors.New("Negotiated error: " +
			"disconnecting peer connected to self")
	}

	// Negotiate the protocol version and set the services to what the remote peer advertised.
	p.protocolVersion = minUint32(p.protocolVersion, uint32(message.ProtocolVersion))
	log.Debugf("Negotiated protocol version %d for peer %s", p.protocolVersion, p)

	if uint32(message.ProtocolVersion) < MinAcceptVersion {
		reason := fmt.Sprintf("protocol version must be %d or greater", MinAcceptVersion)
		// rejectMsg := msg.NewMsgReject(message.Command(), wmsg.RejectObsolete, reason)
		// _ = p.writeMessage(rejectMsg)
		return errors.New("Negotiated error: " +
			reason)
	}

	return nil
}

func (p *Peer) processRemoteVerAckMsg(msg *wmsg.MsgVerAck) {

}

func (p *Peer) writeLocalVersionMsg(magic uint32) error {

	nonce := uint64(rand.Int63())
	sentNonces.Add(nonce)

	versionMsg := msg.NewMsgVersion(magic, uint32(p.na.Port), nonce, p.cfg.ProtocolVersion)

	return p.writeMessage(versionMsg, 8)
}

func (p *Peer) waitToFinishHandShake() error {

	for {
		remoteMsg, err := p.readMessage()
		if err == wmsg.ErrUnknownMessage {
			continue
		} else if err != nil {
			return err
		}

		switch m := remoteMsg.(type) {
		case *msg.MsgVerAck:
			// Receiving a verack means we are done with the handshake.
			p.processRemoteVerAckMsg(m)
			return nil
		default:
			return fmt.Errorf("invalid message during handshake")
		}
	}
}

func (p *Peer) handshakeIn(magic uint32) error {
	if err := p.readRemoteVersionMsg(magic); err != nil {
		return err
	}

	if err := p.writeLocalVersionMsg(magic); err != nil {
		return err
	}

	if err := p.writeMessage(wmsg.NewMsgSendAddr()); err != nil {
		return err
	}

	err := p.writeMessage(wmsg.NewMsgVerAck())
	if err != nil {
		return err
	}

	return p.waitToFinishHandShake()
}

func (p *Peer) handshakeOut(magic uint32) error {
	if err := p.writeLocalVersionMsg(magic); err != nil {
		return err
	}

	if err := p.readRemoteVersionMsg(magic); err != nil {
		return err
	}

	if err := p.writeMessage(wmsg.NewMsgSendAddr()); err != nil {
		return err
	}

	err := p.writeMessage(wmsg.NewMsgVerAck())
	if err != nil {
		return err
	}

	return p.waitToFinishHandShake()
}

func (p *Peer) handshake() error {
	hsErr := make(chan error, 1)
	go func() {
		if p.inbound {
			hsErr <- p.handshakeIn()
		} else {
			hsErr <- p.handshakeOut()
		}
	}()

	select {
	case err := <-hsErr:
		if err != nil {
			p.Close()
			return err
		}
	case <-time.After(PEER_HAND_SHAKE_TIMEOUT):
		p.Close()
		return errors.New("protocol hand shake timeout")
	}

	return nil
}
