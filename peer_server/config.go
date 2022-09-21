package main

import (
	"net"
	"time"
)

const (
	defaultMaxPeers         = 125
	defaultTargetOutbound   = 8
	connectionRetryInterval = time.Second * 5
	defaultConnectTimeout   = time.Second * 30
)

type config struct {
	Listeners []string
	DataPath  string
	lookup    func(string) ([]net.IP, error)
	dial      func(string, string, time.Duration) (net.Conn, error)

	MaxPeers     int
	ConnectPeers []string
	AddPeers     []string
}

func loadConfig() (*config, []string, error) {
	return &config{}, nil, nil
}

func msndLookup(host string) ([]net.IP, error) {
	return cfg.lookup(host)
}

func msndDial(addr net.Addr) (net.Conn, error) {
	return cfg.dial(addr.Network(), addr.String(), defaultConnectTimeout)
}
