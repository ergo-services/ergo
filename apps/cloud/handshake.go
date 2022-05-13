package cloud

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

type cloudFlagID uint64
type cloudFlags cloudFlagID

func (f cloudFlags) toUint64() uint64 {
	return uint64(f)
}

func toCloudFlags(f ...cloudFlagID) cloudFlags {
	var flags uint64
	for _, v := range f {
		flags |= uint64(v)
	}
	return cloudFlags(flags)
}

const (
	clientVersion int = 1

	flagIntrospection cloudFlagID = 0x1
	flagMetrics       cloudFlagID = 0x2
	flagRemoteSpawn   cloudFlagID = 0x3

	defaultHandshakeTimeout = 5 * time.Second

	messageTypeM1 = byte(1)
	messageTypeM2 = byte(2)
	messageTypeM3 = byte(3)
	messageTypeM4 = byte(4)
	messageTypeMX = byte(100)
)

type cloudHandshake struct {
	node.Handshake
	nodename  string
	creation  uint32
	options   node.Cloud
	flags     node.Flags
	challenge string
}

type messageM2 struct {
	flags     uint64
	creation  uint32
	challenge string
	peername  string
}
type messageM4 struct {
	digest     string
	updatename string // use this name to present itself instead of the original one within this connection
	warnings   int    // number warning message must be received after this message
}

// message
type messageMX struct {
	// error messages
	// id = 0 - malformed message from the cloud
	//      1 - incorrect cluster name or cookie value
	//      2 - taken (this node is already presented in the cloud cluster)
	//      3 - suspended
	//      4 - blocked
	// informing messages have id above 100
	id          int
	description string
}

func createHandshake(options node.Cloud) node.HandshakeInterface {
	if options.Timeout == 0 {
		options.Timeout = defaultHandshakeTimeout
	}
	return &cloudHandshake{
		options:   options,
		challenge: lib.RandomString(32),
	}
}

func (ch *cloudHandshake) Init(nodename string, creation uint32, flags node.Flags) error {
	if flags.EnableProxy == false {
		s := "Proxy feature must be enabled for the cloud connection"
		lib.Warning(s)
		return fmt.Errorf(s)
	}
	if ch.options.ClusterID == "" {
		s := "Options Cloud.Cluster can not be empty"
		lib.Warning(s)
		return fmt.Errorf(s)
	}
	if len(ch.options.ClusterID) != 32 {
		s := "Options Cloud.Cluster has wrong value"
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

func (ch *cloudHandshake) Start(remote net.Addr, conn io.ReadWriter, tls bool, cookie string) (node.HandshakeDetails, error) {
	var details node.HandshakeDetails

	fmt.Println("START HANDSHAKE with", remote)

	details.Flags = ch.flags

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	ch.composeMessageM1(b)
	if e := b.WriteDataTo(conn); e != nil {
		return details, e
	}
	b.Reset()

	// define timeout for the handshaking
	timer := time.NewTimer(ch.options.Timeout)
	defer timer.Stop()

	asyncReadChannel := make(chan error, 2)
	asyncRead := func() {
		_, err := b.ReadDataFrom(conn, 512)
		asyncReadChannel <- err
	}

	expectingBytes := 2
	await := []byte{messageTypeM2, messageTypeMX}

	for {
		go asyncRead()
		select {
		case <-timer.C:
			return details, fmt.Errorf("timeout")
		case err := <-asyncReadChannel:
			if err != nil {
				return details, err
			}

			if b.Len() < expectingBytes {
				continue
			}

			l := binary.BigEndian.Uint16(b.B[expectingBytes-2 : expectingBytes])
			buffer := b.B[expectingBytes:]

			if len(buffer) < int(l) {
				return details, fmt.Errorf("malformed handshake (wrong packet length)")
			}

			// check if we got correct message type regarding to 'await' value
			if bytes.Count(await, buffer[1:2]) == 0 {
				return details, fmt.Errorf("malformed handshake (wrong response)")
			}

			switch buffer[1] {
			case messageTypeM2:
			case messageTypeM4:
			case messageTypeMX:
				//message, _ := ch.readMX(buffer[2:])
				return details, fmt.Errorf("malformed handshake (message type %d)", buffer[0])
			default:
				return details, fmt.Errorf("malformed handshake (message type %d)", buffer[0])
			}
		}
	}

	return details, nil
}

func genDigest(cluster, challenge, cookie string) [16]byte {
	s := cluster + challenge + cookie
	digest := md5.Sum([]byte(s))
	return digest
}

func (ch *cloudHandshake) composeMessageM1(b *lib.Buffer) {
	// header: 2 (packet len) + 1 (message type)
	// body: 1 (client version) + 8 (flags) + 4 (creation) + 32 (cluster id) + nodename
	b.Allocate(2 + 1 + 1 + 8 + 4)
	b.B[2] = messageTypeM1
	b.B[3] = byte(clientVersion)
	flags := ch.composeFlags()
	binary.BigEndian.PutUint64(b.B[4:12], flags.toUint64())
	b.Append([]byte(ch.options.ClusterID))
	b.Append([]byte(ch.nodename))
	l := b.Len() - 2
	binary.BigEndian.PutUint16(b.B[0:2], uint16(l)) // uint16
}

func (ch *cloudHandshake) composeMessageM3(b *lib.Buffer, digest []byte) {
	// header: 2 (packet len) + 1 (message type)
	// body: 32 (challengeB) + 16 (digestA)
	b.Allocate(2 + 1)
	binary.BigEndian.PutUint16(b.B[0:2], 1+32+16) // uint16
	b.B[2] = messageTypeM3
	b.Append([]byte(ch.challenge))
	b.Append(digest)
}

func (ch *cloudHandshake) readM2() (messageM2, error) {
	// body: 8 (flags) + 4 (creation) + 32 (challengeA) + nodename
	var m2 messageM2

	return m2, nil
}

func (ch *cloudHandshake) readM4() (messageM4, error) {
	// body: 16 (digestB) + updatename
	var m4 messageM4

	return m4, nil
}

func (ch *cloudHandshake) readMX(buffer []byte) (messageMX, error) {
	var mx messageMX

	return mx, nil
}

func (ch *cloudHandshake) composeFlags() cloudFlags {
	flags := ch.options.Flags
	enabledFlags := []cloudFlagID{}

	if flags.EnableIntrospection {
		enabledFlags = append(enabledFlags, flagIntrospection)
	}
	if flags.EnableMetrics {
		enabledFlags = append(enabledFlags, flagMetrics)
	}
	if flags.EnableRemoteSpawn {
		enabledFlags = append(enabledFlags, flagRemoteSpawn)
	}
	return toCloudFlags(enabledFlags...)
}
