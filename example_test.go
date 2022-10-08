package peer_test

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/coreservice-io/crypto_p2p/peer"
	"github.com/coreservice-io/crypto_p2p/wire/wirebase"
	"github.com/coreservice-io/crypto_p2p/wire/wmsg"

	"github.com/btcsuite/btclog"
)

const (
	// represents the simulation test network.
	SimNet wirebase.NetMagic = 0x12141c16
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
		p.AssociateConnection(conn)
	}()

	return nil
}

func startClient(addr string, cfg *peer.Config) (*peer.Peer, error) {
	p, err := peer.NewOutboundPeer(cfg, addr)
	if err != nil {
		fmt.Printf("NewOutboundPeer: error %v\n", err)
		return nil, err
	}

	// Establish the connection to the peer address and mark it connected.
	conn, err := net.Dial("tcp", p.Addr())
	if err != nil {
		fmt.Printf("net.Dial: error %v\n", err)
		return nil, err
	}
	p.AssociateConnection(conn)

	return p, nil
}

// mockRemotePeer creates a basic inbound peer listening on the simnet port for
// use with Example_peerConnection.
// It does not return until the listner is active.
func mockRemotePeer() error {
	// Configure peer to act as a simnet node that offers no services.
	peerCfg := &peer.Config{
		UserAgentName:    "test peer", // User agent name to advertise.
		UserAgentVersion: "1.0.0",     // User agent version to advertise.
		NetMagic:         SimNet,
		OnMessageHook: peer.MessageWatcher{
			OnRead: func(p *peer.Peer, bytesRead int, msg wirebase.Message, err error) {

				fmt.Println("Remote OnRead: Occure")

				if err != nil {
					str := "Remote OnRead Meet Error at length: %d, detail: %v"
					err := fmt.Errorf(str, bytesRead, err)
					fmt.Println("", err)
					return
				}

				if msg == nil {
					fmt.Println("OnRead: msg nil")
					return
				}
			},
		},
		AllowSelfConns: true,
	}

	// Accept connections on the simnet port.
	err := startServer("127.0.0.1:18555", peerCfg)
	if err != nil {
		return err
	}

	return nil
}

// This example demonstrates the basic process for initializing and creating an
// outbound peer.
// Peers negotiate by exchanging version and verack messages.
// For demonstration, a simple handler for version message is attached to the
// peer.
func Example_newOutboundPeer() {

	backendLogger := btclog.NewBackend(os.Stdout)
	defer os.Stdout.Sync()
	peerLog := backendLogger.Logger("PEER")
	peerLog.SetLevel(btclog.LevelDebug)
	peer.UseLogger(peerLog)

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
	verack := make(chan string)

	peerCfg := &peer.Config{
		UserAgentName:    "peer",  // User agent name to advertise.
		UserAgentVersion: "1.0.0", // User agent version to advertise.
		NetMagic:         SimNet,
		OnMessageHook: peer.MessageWatcher{
			OnRead: func(p *peer.Peer, bytesRead int, msg wirebase.Message, err error) {

				fmt.Println("Local OnRead: Occure")

				if err != nil {
					str := "Local OnRead Meet Error at length: %d, detail: %v"
					err := fmt.Errorf(str, bytesRead, err)
					fmt.Println("", err)
					return
				}

				if msg == nil {
					fmt.Println("OnRead: msg nil")
					return
				}

				fmt.Println("outbound: ", msg.Command())

				if msg.Command() == wmsg.CmdVerAck {
					verack <- msg.Command()
				}
			},
		},
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

	// Disconnect the peer.
	p.Disconnect()
	p.WaitForDisconnect()

	// Output:
	// outbound: received version
}
