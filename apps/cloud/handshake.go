package cloud

import (
	"crypto/md5"
	"fmt"
	"io"

	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

type clientFlagID uint64
type clientFlags clientFlagID

func (f clientFlags) toUint64() uint64 {
	return uint64(f)
}

const (
	clientVersion int = 1

	flagIntrospection clientFlagID = 0x1
	flagMetrics       clientFlagID = 0x2
)

type cloudHandshake struct {
	node.Handshake
	nodename string
	creation uint32
	options  node.Cloud
	flags    node.Flags
}

func createHandshake(options node.Cloud) node.HandshakeInterface {
	return &cloudHandshake{
		options: options,
	}
}

func (ch *cloudHandshake) Init(nodename string, creation uint32, flags node.Flags) error {
	if flags.EnableProxy == false {
		s := "Proxy feature must be enabled for the cloud connection"
		lib.Warning(s)
		return fmt.Errorf(s)
	}
	ch.nodename = nodename
	ch.creation = creation
	ch.flags = flags
	if ch.options.Flags.Enable == false {
		return nil
	}

	ch.flags.EnableRemoteSpawn = ch.options.Flags.EnableRemoteSpawn
	return nil
}

func (ch *cloudHandshake) Start(conn io.ReadWriter, tls bool, cookie string) (node.HandshakeDetails, error) {
	var details node.HandshakeDetails

	details.Flags = node.DefaultFlags()

	//message := ch.composeMessageM1()

	fmt.Println("START HANDSHAKE")
	return details, nil
}

func genDigest(nodename, challenge, cookie string) [16]byte {
	s := nodename + challenge + cookie
	digest := md5.Sum([]byte(s))
	return digest
}

type messageM2 struct{}
type messageM4 struct{}
type messageM2XX struct{}

func (ch *cloudHandshake) composeMessageM1() []byte {
	return nil
}

func (ch *cloudHandshake) composeMessageM3() []byte {
	return nil
}

func (ch *cloudHandshake) composeMessageM2XX() messageM2XX {
	var m2XX messageM2XX

	return m2XX
}

func (ch *cloudHandshake) readM2() (messageM2, error) {
	var m2 messageM2

	return m2, nil
}

func (ch *cloudHandshake) readM4() (messageM4, error) {
	var m4 messageM4

	return m4, nil
}

func (ch *cloudHandshake) readM2XX() (messageM2XX, error) {
	var m2XX messageM2XX

	return m2XX, nil
}
