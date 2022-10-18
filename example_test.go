package peer_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/coreservice-io/crypto_p2p/peer"
)

const (
	// represents the simulation test network.
	SimNet uint32 = 0x12141c16
)

func startServer(listenAddr string, cfg *peer.Config) error {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Accept: error %v\n", err)
			return
		}

		// Create and start the inbound peer.
		p := peer.NewInboundPeer(cfg)
		p.AttachConn(conn)
	}()

	return nil
}

func startClient(addr string, cfg *peer.Config) (*peer.Peer, error) {
	p, err := peer.NewOutboundPeer(cfg, addr)
	if err != nil {
		fmt.Printf("NewOutboundPeer: error %v\n", err)
		return nil, err
	}

	// Establish the connection to the peer address.
	conn, err := net.Dial("tcp", p.Addr())
	if err != nil {
		fmt.Printf("net.Dial: error %v\n", err)
		return nil, err
	}
	p.AttachConn(conn)

	return p, nil
}

// mockRemotePeer creates a basic inbound peer listening on the simnet port for
// use with Example_peerConnection.
// It does not return until the listner is active.
func mockRemotePeer() error {
	// Configure peer to act as a simnet node that offers no services.
	peerCfg := &peer.Config{
		NetMagic:       SimNet,
		AllowSelfConns: true,
	}

	// Accept connections on the simnet port.
	err := startServer("127.0.0.1:18555", peerCfg)
	if err != nil {
		return err
	}

	return nil
}

func Test_NewOutboundPeer(t *testing.T) {

	// Ordinarily this will not be needed since the outbound peer will be
	// connecting to a remote peer, however, since this example is executed
	// and tested, a mock remote peer is needed to listen for the outbound
	// peer.
	if err := mockRemotePeer(); err != nil {
		fmt.Printf("mockRemotePeer: unexpected error %v\n", err)
		return
	}

	// Create an outbound peer that is configured to act as a simnet node
	// that offers no services and has listeners for the version and verack
	// messages.
	// The verack listener is used here to signal the code below
	// when the handshake has been finished by signalling a channel.
	verack := make(chan uint32)

	peerCfg := &peer.Config{
		NetMagic:       SimNet,
		AllowSelfConns: true,
	}

	p, err := startClient("127.0.0.1:18555", peerCfg)
	if err != nil {
		fmt.Printf("startClient: error %v\n", err)
		return
	}

	// Wait for the verack message or timeout in case of failure.
	select {
	case <-verack:
	case <-time.After(time.Second * 1):
		fmt.Printf("Example_peerConnection: verack timeout")
	}

	p.Close()
	p.WaitForClose()

	// Output:
	// outbound: received version
}

func Test_PeerMgr(t *testing.T) {

	// llog, err := logrus_log.New("./logs", 1, 20, 30)
	// if err != nil {
	// 	panic(err.Error())
	// }
	listen := "127.0.0.1:18555"
	seeds := "127.0.0.1:18555,127.0.0.1:9999"
	peer_mgr, _ := peer.InitPeerMgr(1, SimNet, listen, seeds)

	peer_mgr.Start()

	fmt.Printf("%v\n", peer_mgr.Seeds())

	// inbound_peer.RegRequestHandler(cmd.CMD_HANDSHAKE, func(req []byte) []byte {
	// 	cmd_req := &cmd.CmdHandShake_REQ{}
	// 	err := cmd_req.Decode(req)
	// 	if err == nil || cmd_req.Net_magic == peer_mgr.net_magic {
	// 		inbound_peer.boosted <- struct{}{}
	// 	}

	// 	return (&cmd.CmdHandShake_RESP{Net_magic: peer_mgr.net_magic}).Encode()
	// })

	// Output:
	// outbound: received Seeds
}
