package peer_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/coreservice-io/crypto_p2p/peer"
	"github.com/coreservice-io/crypto_p2p/wire/msg"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
)

const (
	// represents the simulation test network.
	SimNet uint32 = 0x12141c16
)

func startServer(listenAddr string) error {
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
		p := peer.NewInboundPeer()
		p.AttachConn(conn, nil)
	}()

	return nil
}

func startClient(addr string) (*peer.Peer, error) {
	p, err := peer.NewOutboundPeer(addr)
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
	p.AttachConn(conn, nil)

	return p, nil
}

// mockRemotePeer creates a basic inbound peer listening on the simnet port for
// use with Example_peerConnection.
// It does not return until the listner is active.
func mockRemotePeer() error {
	// Configure peer to act as a simnet node that offers no services.
	// peerCfg := &peer.Config{
	// 	NetMagic:       SimNet,
	// 	AllowSelfConns: true,
	// }

	// Accept connections on the simnet port.
	err := startServer("127.0.0.1:18555")
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

	// peerCfg := &peer.Config{
	// 	NetMagic:       SimNet,
	// 	AllowSelfConns: true,
	// }

	p, err := startClient("127.0.0.1:18555")
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

	listen := "127.0.0.1:18555"
	seeds := "127.0.0.1:18555"

	// seeds := "127.0.0.1:18532,127.0.0.1:9999"
	peer_mgr, _ := peer.InitPeerMgr(1, SimNet, listen, seeds)

	peer_mgr.Start()

	done := make(chan bool, 1)
	fmt.Printf("%v\n", peer_mgr.Seeds())

	fmt.Printf("%v\n", peer_mgr.IbPeers())

	select {
	case <-time.After(3 * time.Second):
		fmt.Printf("wait there is peer exists\n")
	}

	test_cmd := uint32(123)
	test_byte := []byte("Here is a string....")

	peer_mgr.RegHandler(test_cmd, testCmdMsg)
	peer_mgr.IbPeers().Range(func(key interface{}, value interface{}) bool {

		p := value.(*peer.Peer)

		fmt.Printf("send request: %v. %s\n", len(test_byte), p)
		resp, err := p.SendWithTimeout(test_cmd, test_byte, 10)
		if err != nil {
			fmt.Printf("can't send msg to %v\n", err)
		} else {
			myString := string(resp)

			fmt.Printf("receive msg %s\n", myString)
		}
		return true
	})

	test_byte2 := make([]byte, wirebase.TMU_MAX_PAYLOAD_SIZE*4)

	peer_mgr.IbPeers().Range(func(key interface{}, value interface{}) bool {

		p := value.(*peer.Peer)
		fmt.Printf("send request: %v. %s\n", len(test_byte2), p)
		resp, err := p.SendWithTimeout(test_cmd, test_byte2, 10)
		if err != nil {
			fmt.Printf("can't send msg to %v\n", err)
		} else {
			fmt.Printf("receive msg %d\n", len(resp))
		}
		return true
	})

	select {
	case <-done:
		fmt.Printf("done")
	}

	// Output:
	// outbound: received Seeds
}

func testCmdMsg(msg_payload []byte, ctx *msg.HandlerMsgCtx, p *peer.Peer) error {

	// fmt.Printf("echo request: %v. %s\n", msg_payload, p)
	fmt.Printf("echo request: %v. %s\n", len(msg_payload), p)

	ctx.Send(msg_payload)

	return nil
}
