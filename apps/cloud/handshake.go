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
	flagHeaderAtomCache clientFlagID = 0x1
	flagBigCreation     clientFlagID = 0x2
	flagBifPidRef       clientFlagID = 0x3
	flagFragmentation   clientFlagID = 0x4
	flagAlias           clientFlagID = 0x5
	flagRemoteSpawn     clientFlagID = 0x6
	flagCompression     clientFlagID = 0x7
)

type CloudHandshake struct {
	node.Handshake
	id     string
	cookie string
}

func CreateHandshake(cloud node.Cloud) node.HandshakeInterface {
	return &CloudHandshake{
		id:     cloud.ID,
		cookie: cloud.Cookie,
	}
}

func (ch *CloudHandshake) Init(nodename string, creation uint32, flags node.Flags) error {
	fmt.Println("INIT HANDSHAKE")
	if flags.EnableProxy == false {
		s := "Proxy feature must be enabled for the cloud connection"
		lib.Warning(s)
		return fmt.Errorf(s)
	}
	return nil
}

func (ch *CloudHandshake) Start(conn io.ReadWriter, tls bool, cookie string) (node.HandshakeDetails, error) {
	var details node.HandshakeDetails

	fmt.Println("START HANDSHAKE")
	return details, nil
}

func genDigest(nodename, challenge, cookie string) [16]byte {
	s := nodename + challenge + cookie
	digest := md5.Sum([]byte(s))
	return digest
}
