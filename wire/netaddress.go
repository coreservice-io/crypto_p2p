package wire

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type PeerAddr struct {
	host string
	port uint16
}

func (pa *PeerAddr) String() string {
	return fmt.Sprintf("%s:%d", pa.host, pa.port)
}

func ParsePeerAddr(addrs string) ([]PeerAddr, error) {

	addr_array := strings.Split(addrs, ",")

	res := make([]PeerAddr, len(addr_array))
	for _, addr := range addr_array {

		peerAddr, err := NewPeerAddr(addr)
		if err != nil {
			return nil, err
		}

		res = append(res, *peerAddr)
		return res, nil

	}

	return res, nil
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

// defines information about a peer on the network including the
// last time it was seen, the services it supports, its address, and port.
// This struct is used in the addrv message.
// Additionally, it can contain any NetAddress address.
type NetAddress struct {
	// Last time the address was seen. This is, unfortunately, encoded as a
	// uint32 on the wire and therefore is limited to 2106. This field is
	// not present in the version message (MsgVersion) nor was it
	// added until protocol version >= NetAddressTimeVersion.
	Timestamp time.Time

	// the network address of the peer.
	// This is a variable-length address.
	// Network() returns the networkID which is a uint8 encoded as a string.
	// String() returns the address as a string.
	Addr net.Addr

	// Port is the port of the address. This is 0 if the network doesn't
	// use ports.
	Port uint16
}

func (na *NetAddress) String() string {
	return fmt.Sprintf("%s", na.Addr.String())
}

func NewNetAddress(addr net.Addr) (*NetAddress, error) {
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		ip := tcpAddr.IP
		port := uint16(tcpAddr.Port)
		na := NewNetAddressIPPort(ip, port)
		return na, nil
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
	na := NewNetAddressIPPort(ip, uint16(port))

	return na, nil
}

func NewNetAddressIPPort(ip net.IP, port uint16) *NetAddress {

	timestamp := time.Now()
	// Limit the timestamp to one second precision
	// since the protocol doesn't support better.
	timestamp = time.Unix(timestamp.Unix(), 0)

	return NetAddressFromBytes(timestamp, ip, port)
}

// creates a NetAddress from a byte slice.
// It will also handle a torv2 address using the OnionCat encoding.
func NetAddressFromBytes(timestamp time.Time,
	addrBytes []byte, port uint16) *NetAddress {

	netAddr := IpPortFromBytes(addrBytes)

	return &NetAddress{
		Timestamp: timestamp,
		Addr:      *netAddr,
		Port:      port,
	}
}

func IpPortFromBytes(addrBytes []byte) *net.Addr {

	var netAddr net.Addr
	switch len(addrBytes) {
	case ipv4Size:
		addr := &ipv4Addr{}
		addr.netID = ipv4
		copy(addr.addr[:], addrBytes)
		netAddr = addr
	case ipv6Size:
		addr := &ipv6Addr{}
		addr.netID = ipv6
		copy(addr.addr[:], addrBytes)
		netAddr = addr
	}

	return &netAddr
}

// length 32 byte
func (na *NetAddress) ToBytes() [32]byte {

	if na.Addr == nil {
		return [32]byte{}
	}
	host, portStr, err := net.SplitHostPort(na.Addr.String())
	if err != nil {
		return [32]byte{}
	}
	ip := net.ParseIP(host)
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return [32]byte{}
	}

	var buf [32]byte

	copy(buf[:16], ip.To16())

	binary.LittleEndian.PutUint16(buf[30:32], uint16(port))

	return buf
}

// networkID represents the network that a given address is in.
// CJDNS and I2P addresses are not included.
type networkID uint8

const (
	// ipv4 means the following address is ipv4.
	ipv4 networkID = iota + 1

	// ipv6 means the following address is ipv6.
	ipv6
)

const (
	// ipv4Size is the size of an ipv4 address.
	ipv4Size = 4

	// ipv6Size is the size of an ipv6 address.
	ipv6Size = 16
)

type ipv4Addr struct {
	addr  [ipv4Size]byte
	netID networkID
}

// Part of the net.Addr interface.
func (a *ipv4Addr) String() string {
	return net.IP(a.addr[:]).String()
}

// Part of the net.Addr interface.
func (a *ipv4Addr) Network() string {
	return string(a.netID)
}

// Compile-time constraints to check that ipv4Addr meets the net.Addr
// interface.
var _ net.Addr = (*ipv4Addr)(nil)

type ipv6Addr struct {
	addr  [ipv6Size]byte
	netID networkID
}

// Part of the net.Addr interface.
func (a *ipv6Addr) String() string {
	return net.IP(a.addr[:]).String()
}

// Part of the net.Addr interface.
func (a *ipv6Addr) Network() string {
	return string(a.netID)
}

// Compile-time constraints to check that ipv6Addr meets the net.Addr
// interface.
var _ net.Addr = (*ipv4Addr)(nil)
