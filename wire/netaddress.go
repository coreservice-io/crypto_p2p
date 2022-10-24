package wire

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type PeerAddr struct {
	host string
	port uint16
}

func (pa *PeerAddr) Host() string {
	return pa.host
}
func (pa *PeerAddr) Port() uint16 {
	return pa.port
}
func (pa *PeerAddr) String() string {
	return fmt.Sprintf("%s:%d", pa.host, pa.port)
}

func ParsePeerAddr(addrs string) ([]PeerAddr, error) {

	addr_array := strings.Split(addrs, ",")

	res := make([]PeerAddr, 0)

	for _, addr := range addr_array {

		peerAddr, err := NewPeerAddr(addr)
		if err != nil {
			return nil, err
		}

		res = append(res, *peerAddr)
	}

	return res, nil
}

func NewPeerAddrStr(host string, port uint16) (*PeerAddr, error) {

	return &PeerAddr{
		host: host,
		port: port,
	}, nil
}

func NewPeerAddr(addr string) (*PeerAddr, error) {

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	return &PeerAddr{
		host: host,
		port: uint16(port),
	}, nil
}

func NewPeerAddress(addr net.Addr) (*PeerAddr, error) {
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		ip := tcpAddr.IP
		port := uint16(tcpAddr.Port)

		return &PeerAddr{
			host: ip.String(),
			port: port,
		}, nil
	}

	host, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	return &PeerAddr{
		host: ip.String(),
		port: uint16(port),
	}, nil
}
