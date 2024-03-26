package handshake

import (
	"crypto/sha256"
	"fmt"
	"net"
	"time"

	"ergo.services/ergo/gen"
)

func (h *handshake) Join(node gen.NodeHandshake, conn net.Conn, id string, options gen.HandshakeOptions) ([]byte, error) {
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s:%s", id, options.Cookie)))
	digest := fmt.Sprintf("%x", hash.Sum(nil))
	message := MessageJoin{
		Node:         node.Name(),
		ConnectionID: id,
		Digest:       digest,
	}

	if err := h.writeMessage(conn, message); err != nil {
		conn.Close()
		return nil, err
	}

	v, tail, err := h.readMessage(conn, time.Second, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if _, ok := v.(MessageAccept); ok == false {
		return nil, fmt.Errorf("malformed handshake Accept message")
	}

	return tail, nil
}
