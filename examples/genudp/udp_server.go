package main

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type udpServer struct {
	gen.UDP
}

func (us *udpServer) InitUDP(process *gen.UDPProcess, args ...etf.Term) (gen.UDPOptions, error) {
	return gen.UDPOptions{
		Host:    UDPListenHost,
		Port:    uint16(UDPListenPort),
		Handler: &udpHandler{}, // udp_handler.go
	}, nil
}
