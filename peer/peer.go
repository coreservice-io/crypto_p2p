package peer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net"

	"github.com/coreservice-io/crypto_p2p/wire/msg"
)

const HANDSHAKE_TIMEOUT_SECS = 15

type PeerAddr struct {
	ipv4      string
	ipv4_port int
}

type Peer struct {
	id      int
	Version uint32

	inbound                bool
	addr                   PeerAddr
	conn                   net.Conn
	handshake_timeout_secs int
}

func NewPeer() *Peer {
	return &Peer{id: rand.Int(), handshake_timeout_secs: HANDSHAKE_TIMEOUT_SECS}
}

func (peer *Peer) SetAddr(p_addr *PeerAddr) {
	peer.addr = *p_addr
}

// recycle
func (peer *Peer) Close() {

}

func (peer *Peer) startOutBound() error {

	//handshake send message
	//handshake read result

	//start heatbeat

	//return nil
}

func (peer *Peer) startInBound() error {

	//handshake send message
	//handshake read result

	//startMsgLoop()

	//start heatbeat

}

// start processing incoming messages
func (peer *Peer) startMsgLoop() error {

	defer func() {
		//peer_conn.Conn.Close()
		//fmt.Println("peer conn exit")
	}()

	reader := bufio.NewReader(peer.conn)

	header_buf := make([]byte, msg.MSG_HEADER_SIZE)
	chunk_buf := make([]byte, msg.MSG_CHUNK_MAX_LEN)

	for {

		_, err := io.ReadFull(reader, header_buf[:])
		if err != nil {
			if err == io.EOF {
				return nil
			}
			GetPeerMgr().log.Errorln(err)
			return err
		}

		msg_header := msg.Decode_msg_header(header_buf[:])

		//check version ,payload size ,...etc..
		if msg_header.Payload_size > msg.MSG_PAYLOAD_SIZE_LIMIT {
			fmt.Println("handle_request msg_header Payload_size oversize:", msg_header.Payload_size)
			return
		}

		//read payload
		if msg_header.Payload_size > 0 {
			_, err := io.ReadFull(reader, payload_buf[0:msg_header.Payload_size])
			if err != nil {
				fmt.Println("handle_request payload conn io read err:", err)
				return
			}
		}

		/////////////////////////////////////////
		if msg_header.Cmd == msg.CMD_ERR || msg_header.Cmd == msg.CMD_0 {

			if peer_msg_i, exist := peer_conn.Messages.Load(msg_header.Id); exist {

				if msg_header.Payload_size > 0 {
					peer_msg_i.(*Peer_msg).Msg_buffer.Write(payload_buf[0:msg_header.Payload_size])
				}
				peer_msg_i.(*Peer_msg).Msg_chunks <- int32(msg_header.Payload_size)

				if msg_header.EOF == 1 {
					peer_msg_i.(*Peer_msg).Msg_chunks <- -1
				}

				if msg_header.Cmd == msg.CMD_ERR {
					peer_msg_i.(*Peer_msg).Msg_err = true
				}

			} else {
				fmt.Println("handle_request msg_id not found, id:", msg_header.Id)
			}

		} else {
			if _, exist := peer_conn.Messages.Load(msg_header.Id); exist {
				fmt.Println("handle_request msg_id overlap, id:", msg_header.Id)
				return //must return not continue as consequent message will bring chaos if continue
			}

			if GetPeerManager().Get_msg_handler(msg_header.Cmd) == nil {
				fmt.Println("msg_handler not found, msg_cmd:", msg_header.Cmd)
				return //must return not continue as consequent message will bring chaos if continue
			}

			p_msg := &Peer_msg{
				Id:         msg_header.Id,
				Msg_cmd:    msg_header.Cmd,
				Msg_chunks: make(chan int32, msg.MSG_CHUNKS_LIMIT+1), //+1 for eof
				Msg_buffer: bytes.NewBuffer([]byte{}),
			}

			if msg_header.Payload_size > 0 {
				p_msg.Msg_buffer.Write(payload_buf[0:msg_header.Payload_size])
			}

			p_msg.Msg_chunks <- int32(msg_header.Payload_size)

			if msg_header.EOF == 1 {
				p_msg.Msg_chunks <- -1
			}

			peer_conn.Messages.Store(msg_header.Id, p_msg)

			//start receive process
			go func(peer_msg *Peer_msg, nc *Peer_conn) {

				defer peer_conn.Messages.Delete(peer_msg.Id)

				calldata, err := peer_msg.Receive()

				if err != nil {
					nc.send_msg(peer_msg.Id, msg.CMD_ERR, []byte(err.Error()))
					fmt.Println(err)
					return
				}

				//send call result
				result := GetPeerManager().Get_msg_handler(msg_header.Cmd)(calldata)
				nc.send_msg(peer_msg.Id, msg.CMD_0, result)

			}(p_msg, peer_conn)

		}

	}

}
