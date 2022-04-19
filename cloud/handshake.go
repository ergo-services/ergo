package cloud

import (
	"net"

	"github.com/ergo-services/ergo/node"
)

type CloudHandshake struct {
	node.Handshake
}

func (ch *CloudHandshake) Init(nodename string, creation uint32, enabledTLS bool) error {
	return nil
}
func (ch *CloudHandshake) Start(c net.Conn) (node.ProtoOptions, error) {
	return node.ProtoOptions{}, nil
}
