package main

import (
	"fmt"

	"github.com/ergo-services/ergo/gen"
)

type tcpHandler struct {
	gen.TCPHandler
}

func (th *tcpHandler) HandleConnect(process *gen.TCPHandlerProcess, conn *gen.TCPConnection) gen.TCPHandlerStatus {
	fmt.Printf("[TCP handler] got new connection from %q\n", conn.Addr.String())
	return gen.TCPHandlerStatusOK
}
func (th *tcpHandler) HandleDisconnect(process *gen.TCPHandlerProcess, conn *gen.TCPConnection) {
	fmt.Printf("[TCP handler] connection with %q terminated\n", conn.Addr.String())
}

func (th *tcpHandler) HandlePacket(process *gen.TCPHandlerProcess, packet []byte, conn *gen.TCPConnection) (int, int, gen.TCPHandlerStatus) {
	fmt.Printf("[TCP handler] got message from %q: %q\n", conn.Addr.String(), string(packet))

	// If you want to send a reply message, use conn.Socket.Write(reply) for that.

	// You may keep any data related to this connection in conn.State

	// return values: left, await, status
	//        left   - how many bytes are left in the packet buffer (you might have
	//                 received a part of the next logical data).
	//        await  - what exact number of bytes you expect in the next packet
	//                 or leave it 0 if you are unsure.
	//        status - return gen.TCPHandlerStatusClose to close this connection

	// example:
	//   expected data of 5 bytes, but have received packet = []byte{1,2,3,4,5,6,7,8}
	//   you must return 3, 5, gen.TCPHandlerStatusOK
	//   So the following invocation will happen after receiving two more bytes
	//   The next packet will have []byte{6,7,8,9,0}
	return 0, 0, gen.TCPHandlerStatusOK
}
