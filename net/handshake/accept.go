package handshake

import (
	"fmt"
	"net"

	"ergo.services/ergo/gen"
)

// Accept is the acceptor step 2: it sends the prepared accept (msg4) and
// introduce (msg5), then reads the peer's final accept (msg6) and fills Tail.
// The node calls it after registering the connection by ConnectionID, so
// pool-join TCPs are guaranteed to arrive after registration.
func (h *handshake) Accept(node gen.NodeHandshake, conn net.Conn, options gen.HandshakeOptions, state gen.HandshakeResult) (gen.HandshakeResult, error) {
	opts, ok := state.Custom.(ConnectionOptions)
	if ok == false || opts.awaitingAccept == false {
		// short Join path, or a stack without the negotiate/accept split: nothing to do
		return state, nil
	}

	if err := h.writeMessage(conn, opts.pendingAccept); err != nil {
		return state, err
	}
	if err := h.writeMessage(conn, opts.pendingIntroduce); err != nil {
		return state, err
	}

	v, tail, err := h.readMessage(conn, opts.pendingTail)
	if err != nil {
		return state, err
	}
	if _, ok := v.(MessageAccept); ok == false {
		return state, fmt.Errorf("malformed handshake Accept message")
	}
	state.Tail = tail

	opts.pendingAccept = MessageAccept{}
	opts.pendingIntroduce = MessageIntroduce{}
	opts.pendingTail = nil
	opts.awaitingAccept = false
	state.Custom = opts
	return state, nil
}
