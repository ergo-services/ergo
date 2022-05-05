package cloud

import (
	"fmt"
	"io"

	"github.com/ergo-services/ergo/node"
)

type CloudHandshake struct {
	node.Handshake
}

func CreateHandshake(cloud node.Cloud) node.HandshakeInterface {
	return &CloudHandshake{}
}

func (ch *CloudHandshake) Init(nodename string, creation uint32, flags node.Flags) error {
	fmt.Println("INIT HANDSHAKE")
	return nil
}

func (ch *CloudHandshake) Start(conn io.ReadWriter, tls bool, cookie string) (node.HandshakeDetails, error) {
	var details node.HandshakeDetails

	fmt.Println("START HANDSHAKE")
	return details, nil
}
