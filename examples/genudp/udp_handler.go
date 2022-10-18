package main

import (
	"fmt"

	"github.com/ergo-services/ergo/gen"
)

type udpHandler struct {
	gen.UDPHandler
}

func (uh *udpHandler) HandlePacket(process *gen.UDPHandlerProcess, data []byte, packet gen.UDPPacket) {
	fmt.Printf("[UDP handler] got message from %q: %q\n", packet.Addr.String(), string(data))

	// If you want to send a reply message, use packet.Socket.Write(reply) for that.
}
