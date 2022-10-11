package peer

import (
	"errors"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/coreservice-io/crypto_p2p/cmd"
	"github.com/coreservice-io/crypto_p2p/msg"
)

const DEFAULT_REQUEST_TIMEOUT_SECS = 60
const PEER_BOOST_TIMEOUT_SECS = 15

// ///////////////////////

type PeerResp struct {
	Request_id uint32
	Done       chan struct{}
	buffer     *msg.MsgBuffer
}

type PeerReq struct {
	Request_id uint32
	buffer     *msg.MsgBuffer
}

// ///////////////////////
type PeerAddr struct {
	ipv4      string
	ipv4_port int
}

func ParsePeerAddr(from string) (*PeerAddr, error) {
	//todo more complex parsing
	result := strings.Split(from, ":")

	ipv4_port, err := strconv.Atoi(result[1])
	if err != nil {
		return nil, err
	}

	return &PeerAddr{
		ipv4:      result[0],
		ipv4_port: ipv4_port,
	}, nil

}

type Peer struct {
	Version uint32
	inbound bool

	addr    *PeerAddr
	conn    net.Conn
	boosted chan struct{}

	request_map  map[uint32]*PeerReq
	response_map map[uint32]*PeerResp

	request_handlers map[uint32]func(resp []byte) ([]byte, error) //error happens only if caller put bad strange param , e.g error happens peer will break
}

func NewPeer() *Peer {
	return &Peer{
		request_map:      make(map[uint32]*PeerReq),
		response_map:     make(map[uint32]*PeerResp),
		request_handlers: make(map[uint32]func(resp []byte) ([]byte, error)),
	}
}

func (peer *Peer) SetAddr(p_addr *PeerAddr) {
	peer.addr = p_addr
}

func (peer *Peer) SetConn(conn net.Conn) {
	peer.conn = conn
}

// recycle
func (peer *Peer) Close() {
	peer.conn.Close()
}

func (peer *Peer) RegRequestHandler(cmd uint32, callback func(req []byte) ([]byte, error)) error {
	if _, ok := peer.request_handlers[cmd]; ok {
		return errors.New("cmd handler overlap")
	} else {
		peer.request_handlers[cmd] = callback
		return nil
	}
}

// ///////////////
func (peer *Peer) SendRequest(cmd uint32) ([]byte, error) {
	return peer.SendRequestTimeout(cmd, DEFAULT_REQUEST_TIMEOUT_SECS)
}

func (peer *Peer) SendRequestTimeout(cmd uint32, timeout_secs int) ([]byte, error) {

	p_resp := &PeerResp{
		Request_id: rand.Uint32(),
		Done:       make(chan struct{}),
	}

	peer.response_map[p_resp.Request_id] = p_resp

	//todo write msg to chunks to tcp-conn

	select {
	case <-p_resp.Done:
		callback(p_resp.buffer.Body.Bytes(), nil)

	case <-time.After(time.Duration(timeout_secs) * time.Second):
		callback(nil, errors.New("PEER_MSG_READ_TIMEOUT_SECS"))
	}

	delete(peer.response_map, p_resp.Request_id)

}

// ///////////////
// func (peer *Peer) SendResponseTimeout() {

// }

// start the msg loop
func (peer *Peer) Start() error {

	for {
		chunk_msg, err := msg.ReadMsgChunk(peer.conn)
		if err != nil {
			return err
		}

		defer chunk_msg.Recycle()

		if chunk_msg.Header.Cmd == cmd.CMD_RESP {
			//handle response messages
			if resp, ok := peer.response_map[chunk_msg.Header.Req_id]; ok {

				finished, err := resp.buffer.ConnectChunk(chunk_msg)
				if err != nil {
					//todo add credit system call as an error happend
					//todo add credit penalty system call as this ip-peer is bad
					return err
				}

				if finished {
					delete(peer.response_map, chunk_msg.Header.Req_id)
					resp.Done <- struct{}{}
				}

			} else {
				return errors.New("response not match")
			}

		} else {
			//handle request messages

			//check	cmd handler exist
			if handler, ok := peer.request_handlers[chunk_msg.Header.Cmd]; !ok {
				//todo add credit penalty system call as this ip-peer is bad
				return errors.New("no request handler exist")
			} else {

				//todo add concurrent request limit check
				//todo requests/second check ,e.g too many Req_id exist for a remote peer is not allowed
				req, ok := peer.request_map[chunk_msg.Header.Req_id]
				if !ok {
					req = &PeerReq{Request_id: rand.Uint32()}
					peer.request_map[req.Request_id] = req
				}

				finished, err := req.buffer.ConnectChunk(chunk_msg)
				if err != nil {
					//todo add credit penalty system call as this ip-peer is bad
					return err
				}

				if finished {
					result, err := handler(req.buffer.Body.Bytes())
					delete(peer.request_map, chunk_msg.Header.Req_id)

					if err != nil {
						return err
					}

				}

			}

		}

	}

}
