package cloud

import (
	"context"
	"fmt"
	"net"
)

type CloudHandshake struct {
}

func (ch *CloudHandshake) Start(ctx context.Context, conn net.Conn, options node.HandshakeOptions) (*node.Link, error) {
	return nil, nil
}

func (ch *CloudHandshake) Accept(ctx context.Context, conn net.Conn, options node.HandshakeOptions) (*node.Link, error) {
	return nil, fmt.Errorf("Cloud handshake can't be used within Ergo Node")
}
