package peer

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/coreservice-io/crypto_p2p/wire/msg"
)

func (p *Peer) readRemoteVersionMsg(magic uint32) error {
	hdr, tmu_payload, err := p.readTrafficUnit()
	if err != nil {
		return err
	}

	if hdr.Cmd() != msg.CMD_VERSION {
		return errors.New("Negotiated error: " +
			"a version message must precede all others")
	}

	v_msg := &msg.MsgVersion{}

	pr := bytes.NewBuffer(tmu_payload)
	pver := p.ProtocolVersion()
	err = v_msg.Decode(pr, pver)
	if err != nil {
		return err
	}

	if v_msg.Magic != uint32(magic) {
		return errors.New("Negotiated error: " +
			fmt.Sprintf("message from other network [%v]", v_msg.Magic))
	}

	// Detect self connections.
	if !p.allowSelfConns && sentNonces.Contains(v_msg.Nonce) {
		return errors.New("Negotiated error: " +
			"disconnecting peer connected to self")
	}

	// Negotiate the protocol version and set the services to what the remote peer advertised.
	p.protocolVersion = minUint32(p.protocolVersion, uint32(v_msg.ProtocolVersion))
	llog.Traceln("Negotiated protocol version %d for peer %s", p.protocolVersion, p)

	if uint32(v_msg.ProtocolVersion) < MinAcceptVersion {
		return errors.New("Negotiated error: " +
			fmt.Sprintf("protocol version must be %d or greater", MinAcceptVersion))
	}

	return nil
}

func (p *Peer) writeLocalVersionMsg(magic uint32) error {

	nonce := uint64(rand.Int63())
	sentNonces.Add(nonce)

	versionMsg := msg.NewMsgVersion(magic, uint32(p.na.Port()), nonce, p.protocolVersion)

	var bw bytes.Buffer

	pver := p.ProtocolVersion()
	err := versionMsg.Encode(&bw, pver)
	if err != nil {
		return err
	}
	payload := bw.Bytes()

	return p.writeMessage(msg.CMD_VERSION, payload)
}

func (p *Peer) waitToFinishHandShake() error {

	for {
		hdr, _, err := p.readTrafficUnit()
		if err != nil {
			return err
		}

		if hdr.Cmd() != msg.CMD_VERACK {
			return fmt.Errorf("invalid message during handshake")
		}

		return nil
	}
}

func (p *Peer) handshakeIn(magic uint32) error {
	if err := p.readRemoteVersionMsg(magic); err != nil {
		return err
	}

	if err := p.writeLocalVersionMsg(magic); err != nil {
		return err
	}

	err := p.writeMessage(msg.CMD_VERACK, nil)
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

	err := p.writeMessage(msg.CMD_VERACK, nil)
	if err != nil {
		return err
	}

	return p.waitToFinishHandShake()
}

func (p *Peer) handshake() error {
	hsErr := make(chan error, 1)

	go func() {
		if p.inbound {
			hsErr <- p.handshakeIn(p.magic)
		} else {
			hsErr <- p.handshakeOut(p.magic)
		}
	}()

	shakeTimeout := time.After(time.Second * p.shake_timeout)

	select {
	case err := <-hsErr:
		if err != nil {
			p.Close()
			return err
		}
	case <-shakeTimeout:
		p.Close()
		return errors.New("protocol hand shake timeout")
	}

	return nil
}
