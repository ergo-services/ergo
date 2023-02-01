package dist

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

const (
	HandshakeVersion5 node.HandshakeVersion = 5
	HandshakeVersion6 node.HandshakeVersion = 6

	DefaultHandshakeVersion = HandshakeVersion5
	DefaultHandshakeTimeout = 5 * time.Second

	// distribution flags are defined here https://erlang.org/doc/apps/erts/erl_dist_protocol.html#distribution-flags
	flagPublished          nodeFlagId = 0x1
	flagAtomCache          nodeFlagId = 0x2
	flagExtendedReferences nodeFlagId = 0x4
	flagDistMonitor        nodeFlagId = 0x8
	flagFunTags            nodeFlagId = 0x10
	flagDistMonitorName    nodeFlagId = 0x20
	flagHiddenAtomCache    nodeFlagId = 0x40
	flagNewFunTags         nodeFlagId = 0x80
	flagExtendedPidsPorts  nodeFlagId = 0x100
	flagExportPtrTag       nodeFlagId = 0x200
	flagBitBinaries        nodeFlagId = 0x400
	flagNewFloats          nodeFlagId = 0x800
	flagUnicodeIO          nodeFlagId = 0x1000
	flagDistHdrAtomCache   nodeFlagId = 0x2000
	flagSmallAtomTags      nodeFlagId = 0x4000
	//	flagCompressed                   = 0x8000 // erlang uses this flag for the internal purposes
	flagUTF8Atoms         nodeFlagId = 0x10000
	flagMapTag            nodeFlagId = 0x20000
	flagBigCreation       nodeFlagId = 0x40000
	flagSendSender        nodeFlagId = 0x80000 // since OTP.21 enable replacement for SEND (distProtoSEND by distProtoSEND_SENDER)
	flagBigSeqTraceLabels            = 0x100000
	flagExitPayload       nodeFlagId = 0x400000 // since OTP.22 enable replacement for EXIT, EXIT2, MONITOR_P_EXIT
	flagFragments         nodeFlagId = 0x800000
	flagHandshake23       nodeFlagId = 0x1000000 // new connection setup handshake (version 6) introduced in OTP 23
	flagUnlinkID          nodeFlagId = 0x2000000
	// for 64bit flags
	flagSpawn  nodeFlagId = 1 << 32
	flagNameMe nodeFlagId = 1 << 33
	flagV4NC   nodeFlagId = 1 << 34
	flagAlias  nodeFlagId = 1 << 35

	// ergo flags
	flagCompression = 1 << 63
	flagProxy       = 1 << 62
)

type nodeFlagId uint64
type nodeFlags nodeFlagId

func (nf nodeFlags) toUint32() uint32 {
	return uint32(nf)
}

func (nf nodeFlags) toUint64() uint64 {
	return uint64(nf)
}

func (nf nodeFlags) isSet(f nodeFlagId) bool {
	return (uint64(nf) & uint64(f)) != 0
}

func toNodeFlags(f ...nodeFlagId) nodeFlags {
	var flags uint64
	for _, v := range f {
		flags |= uint64(v)
	}
	return nodeFlags(flags)
}

// DistHandshake implements Erlang handshake
type DistHandshake struct {
	node.Handshake
	nodename  string
	flags     node.Flags
	creation  uint32
	challenge uint32
	options   HandshakeOptions
}

type HandshakeOptions struct {
	Timeout time.Duration
	Version node.HandshakeVersion // 5 or 6
}

func CreateHandshake(options HandshakeOptions) node.HandshakeInterface {
	// must be 5 or 6
	if options.Version != HandshakeVersion5 && options.Version != HandshakeVersion6 {
		options.Version = DefaultHandshakeVersion
	}

	if options.Timeout == 0 {
		options.Timeout = DefaultHandshakeTimeout
	}
	return &DistHandshake{
		options:   options,
		challenge: rand.Uint32(),
	}
}

// Init implements Handshake interface mothod
func (dh *DistHandshake) Init(nodename string, creation uint32, flags node.Flags) error {
	dh.nodename = nodename
	dh.creation = creation
	dh.flags = flags
	return nil
}

func (dh *DistHandshake) Version() node.HandshakeVersion {
	return dh.options.Version
}

func (dh *DistHandshake) Start(remote net.Addr, conn lib.NetReadWriter, tls bool, cookie string) (node.HandshakeDetails, error) {

	var details node.HandshakeDetails

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var await []byte

	if dh.options.Version == HandshakeVersion5 {
		dh.composeName(b, tls)
		// the next message must be send_status 's' or send_challenge 'n' (for
		// handshake version 5) or 'N' (for handshake version 6)
		await = []byte{'s', 'n', 'N'}
	} else {
		dh.composeNameVersion6(b, tls)
		await = []byte{'s', 'N'}
	}
	if e := b.WriteDataTo(conn); e != nil {
		return details, e
	}

	// define timeout for the handshaking
	timer := time.NewTimer(dh.options.Timeout)
	defer timer.Stop()

	asyncReadChannel := make(chan error, 2)
	asyncRead := func() {
		_, e := b.ReadDataFrom(conn, 512)
		asyncReadChannel <- e
	}

	// http://erlang.org/doc/apps/erts/erl_dist_protocol.html#distribution-handshake
	// Every message in the handshake starts with a 16-bit big-endian integer,
	// which contains the message length (not counting the two initial bytes).
	// In Erlang this corresponds to option {packet, 2} in gen_tcp(3). Notice
	// that after the handshake, the distribution switches to 4 byte packet headers.
	expectingBytes := 2
	if tls {
		// TLS connection has 4 bytes packet length header
		expectingBytes = 4
	}

	for {
		go asyncRead()

		select {
		case <-timer.C:
			return details, fmt.Errorf("handshake timeout")

		case e := <-asyncReadChannel:
			if e != nil {
				return details, e
			}

		next:
			l := binary.BigEndian.Uint16(b.B[expectingBytes-2 : expectingBytes])
			buffer := b.B[expectingBytes:]

			if len(buffer) < int(l) {
				return details, fmt.Errorf("malformed handshake (wrong packet length)")
			}

			// check if we got correct message type regarding to 'await' value
			if bytes.Count(await, buffer[0:1]) == 0 {
				return details, fmt.Errorf("malformed handshake (wrong response)")
			}

			switch buffer[0] {
			case 'n':
				// 'n' + 2 (version) + 4 (flags) + 4 (challenge) + name...
				if len(b.B) < 12 {
					return details, fmt.Errorf("malformed handshake ('n')")
				}

				challenge, err := dh.readChallenge(buffer[1:], &details)
				if err != nil {
					return details, err
				}
				b.Reset()

				dh.composeChallengeReply(b, challenge, tls, cookie)

				if e := b.WriteDataTo(conn); e != nil {
					return details, e
				}
				// add 's' status for the case if we got it after 'n' or 'N' message
				// yes, sometime it happens
				await = []byte{'s', 'a'}

			case 'N':
				// Peer support version 6.

				// The new challenge message format (version 6)
				// 8 (flags) + 4 (Creation) + 2 (NameLen) + Name
				if len(buffer) < 16 {
					return details, fmt.Errorf("malformed handshake ('N' length)")
				}

				challenge, err := dh.readChallengeVersion6(buffer[1:], &details)
				if err != nil {
					return details, err
				}
				b.Reset()

				if dh.options.Version == HandshakeVersion5 {
					// upgrade handshake to version 6 by sending complement message
					dh.composeComplement(b, tls)
					if e := b.WriteDataTo(conn); e != nil {
						return details, e
					}
				}

				dh.composeChallengeReply(b, challenge, tls, cookie)

				if e := b.WriteDataTo(conn); e != nil {
					return details, e
				}

				// add 's' (send_status message) for the case if we got it after 'n' or 'N' message
				await = []byte{'s', 'a'}

			case 'a':
				// 'a' + 16 (digest)
				if len(buffer) < 17 {
					return details, fmt.Errorf("malformed handshake ('a' length of digest)")
				}

				// 'a' + 16 (digest)
				digest := genDigest(dh.challenge, cookie)
				if bytes.Compare(buffer[1:17], digest) != 0 {
					return details, fmt.Errorf("malformed handshake ('a' digest)")
				}

				// check if we got DIST packet with the final handshake data.
				if len(buffer) > 17 {
					details.Buffer = lib.TakeBuffer()
					details.Buffer.Set(buffer[17:])
				}

				// handshaked
				return details, nil

			case 's':
				if dh.readStatus(buffer[1:]) == false {
					return details, fmt.Errorf("handshake negotiation failed")
				}

				await = []byte{'n', 'N'}
				// "sok"
				if len(buffer) > 4 {
					b.B = b.B[expectingBytes+3:]
					goto next
				}
				b.Reset()

			default:
				return details, fmt.Errorf("malformed handshake ('%c' digest)", buffer[0])
			}

		}

	}

}

func (dh *DistHandshake) Accept(remote net.Addr, conn lib.NetReadWriter, tls bool, cookie string) (node.HandshakeDetails, error) {
	var details node.HandshakeDetails

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var await []byte

	// define timeout for the handshaking
	timer := time.NewTimer(dh.options.Timeout)
	defer timer.Stop()

	asyncReadChannel := make(chan error, 2)
	asyncRead := func() {
		_, e := b.ReadDataFrom(conn, 512)
		asyncReadChannel <- e
	}

	// http://erlang.org/doc/apps/erts/erl_dist_protocol.html#distribution-handshake
	// Every message in the handshake starts with a 16-bit big-endian integer,
	// which contains the message length (not counting the two initial bytes).
	// In Erlang this corresponds to option {packet, 2} in gen_tcp(3). Notice
	// that after the handshake, the distribution switches to 4 byte packet headers.
	expectingBytes := 2
	if tls {
		// TLS connection has 4 bytes packet length header
		expectingBytes = 4
	}

	// the comming message must be 'receive_name' as an answer for the
	// 'send_name' message request we just sent
	await = []byte{'n', 'N'}

	for {
		go asyncRead()

		select {
		case <-timer.C:
			return details, fmt.Errorf("handshake accept timeout")
		case e := <-asyncReadChannel:
			if e != nil {
				return details, e
			}

			if b.Len() < expectingBytes+1 {
				return details, fmt.Errorf("malformed handshake (too short packet)")
			}

		next:
			l := binary.BigEndian.Uint16(b.B[expectingBytes-2 : expectingBytes])
			buffer := b.B[expectingBytes:]

			if len(buffer) < int(l) {
				return details, fmt.Errorf("malformed handshake (wrong packet length)")
			}

			if bytes.Count(await, buffer[0:1]) == 0 {
				return details, fmt.Errorf("malformed handshake (wrong response %d)", buffer[0])
			}

			switch buffer[0] {
			case 'n':
				if len(buffer) < 8 {
					return details, fmt.Errorf("malformed handshake ('n' length)")
				}

				if err := dh.readName(buffer[1:], &details); err != nil {
					return details, err
				}
				b.Reset()
				dh.composeStatus(b, tls)
				if e := b.WriteDataTo(conn); e != nil {
					return details, fmt.Errorf("malformed handshake ('n' accept name)")
				}

				b.Reset()
				if details.Version == 6 {
					dh.composeChallengeVersion6(b, tls)
					await = []byte{'s', 'r', 'c'}
				} else {
					dh.composeChallenge(b, tls)
					await = []byte{'s', 'r'}
				}
				if e := b.WriteDataTo(conn); e != nil {
					return details, e
				}

			case 'N':
				// The new challenge message format (version 6)
				// 8 (flags) + 4 (Creation) + 2 (NameLen) + Name
				if len(buffer) < 16 {
					return details, fmt.Errorf("malformed handshake ('N' length)")
				}
				if err := dh.readNameVersion6(buffer[1:], &details); err != nil {
					return details, err
				}
				b.Reset()
				dh.composeStatus(b, tls)
				if e := b.WriteDataTo(conn); e != nil {
					return details, fmt.Errorf("malformed handshake ('N' accept name)")
				}

				b.Reset()
				dh.composeChallengeVersion6(b, tls)
				if e := b.WriteDataTo(conn); e != nil {
					return details, e
				}

				await = []byte{'s', 'r'}

			case 'c':
				if len(buffer) < 9 {
					return details, fmt.Errorf("malformed handshake ('c' length)")
				}
				dh.readComplement(buffer[1:], &details)

				await = []byte{'r'}

				if len(buffer) > 9 {
					b.B = b.B[expectingBytes+9:]
					goto next
				}
				b.Reset()

			case 'r':
				if len(buffer) < 19 {
					return details, fmt.Errorf("malformed handshake ('r' length)")
				}

				challenge, valid := dh.validateChallengeReply(buffer[1:], cookie)
				if valid == false {
					return details, fmt.Errorf("malformed handshake ('r' invalid reply)")
				}
				b.Reset()

				dh.composeChallengeAck(b, challenge, tls, cookie)
				if e := b.WriteDataTo(conn); e != nil {
					return details, e
				}

				// handshaked

				return details, nil

			case 's':
				if dh.readStatus(buffer[1:]) == false {
					return details, fmt.Errorf("link status != ok")
				}

				await = []byte{'c', 'r'}
				if len(buffer) > 4 {
					b.B = b.B[expectingBytes+3:]
					goto next
				}
				b.Reset()

			default:
				return details, fmt.Errorf("malformed handshake (unknown code %d)", b.B[0])
			}

		}

	}
}

// private functions

func (dh *DistHandshake) composeName(b *lib.Buffer, tls bool) {
	flags := composeFlags(dh.flags)
	version := uint16(dh.options.Version)
	if tls {
		b.Allocate(11)
		dataLength := 7 + len(dh.nodename) // byte + uint16 + uint32 + len(dh.nodename)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'n'
		binary.BigEndian.PutUint16(b.B[5:7], version)           // uint16
		binary.BigEndian.PutUint32(b.B[7:11], flags.toUint32()) // uint32
		b.Append([]byte(dh.nodename))
		return
	}

	b.Allocate(9)
	dataLength := 7 + len(dh.nodename) // byte + uint16 + uint32 + len(dh.nodename)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'n'
	binary.BigEndian.PutUint16(b.B[3:5], version)          // uint16
	binary.BigEndian.PutUint32(b.B[5:9], flags.toUint32()) // uint32
	b.Append([]byte(dh.nodename))
}

func (dh *DistHandshake) composeNameVersion6(b *lib.Buffer, tls bool) {
	flags := composeFlags(dh.flags)
	creation := uint32(dh.creation)
	if tls {
		b.Allocate(19)
		dataLength := 15 + len(dh.nodename) // 1 + 8 (flags) + 4 (creation) + 2 (len dh.nodename)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'N'
		binary.BigEndian.PutUint64(b.B[5:13], flags.toUint64())          // uint64
		binary.BigEndian.PutUint32(b.B[13:17], creation)                 //uint32
		binary.BigEndian.PutUint16(b.B[17:19], uint16(len(dh.nodename))) // uint16
		b.Append([]byte(dh.nodename))
		return
	}

	b.Allocate(17)
	dataLength := 15 + len(dh.nodename) // 1 + 8 (flags) + 4 (creation) + 2 (len dh.nodename)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'N'
	binary.BigEndian.PutUint64(b.B[3:11], flags.toUint64())          // uint64
	binary.BigEndian.PutUint32(b.B[11:15], creation)                 // uint32
	binary.BigEndian.PutUint16(b.B[15:17], uint16(len(dh.nodename))) // uint16
	b.Append([]byte(dh.nodename))
}

func (dh *DistHandshake) readName(b []byte, details *node.HandshakeDetails) error {
	flags := nodeFlags(binary.BigEndian.Uint32(b[2:6]))
	details.Flags = node.DefaultFlags()
	details.Flags.EnableFragmentation = flags.isSet(flagFragments)
	details.Flags.EnableBigCreation = flags.isSet(flagBigCreation)
	details.Flags.EnableHeaderAtomCache = flags.isSet(flagDistHdrAtomCache)
	details.Flags.EnableAlias = flags.isSet(flagAlias)
	details.Flags.EnableRemoteSpawn = flags.isSet(flagSpawn)
	details.Flags.EnableBigPidRef = flags.isSet(flagV4NC)
	version := int(binary.BigEndian.Uint16(b[0:2]))
	if version != 5 {
		return fmt.Errorf("Malformed version for handshake 5")
	}

	details.Version = 5
	if flags.isSet(flagHandshake23) {
		details.Version = 6
	}

	// Erlang node limits the node name length to 256 characters (not bytes).
	// I don't think anyone wants to use such a ridiculous name with a length > 250 bytes.
	// Report an issue you really want to have a name longer that 255 bytes.
	if len(b[6:]) > 255 {
		return fmt.Errorf("Malformed node name")
	}
	details.Name = string(b[6:])

	return nil
}

func (dh *DistHandshake) readNameVersion6(b []byte, details *node.HandshakeDetails) error {
	details.Creation = binary.BigEndian.Uint32(b[8:12])

	flags := nodeFlags(binary.BigEndian.Uint64(b[0:8]))
	details.Flags = node.DefaultFlags()
	details.Flags.EnableFragmentation = flags.isSet(flagFragments)
	details.Flags.EnableBigCreation = flags.isSet(flagBigCreation)
	details.Flags.EnableHeaderAtomCache = flags.isSet(flagDistHdrAtomCache)
	details.Flags.EnableAlias = flags.isSet(flagAlias)
	details.Flags.EnableRemoteSpawn = flags.isSet(flagSpawn)
	details.Flags.EnableBigPidRef = flags.isSet(flagV4NC)
	details.Flags.EnableCompression = flags.isSet(flagCompression)
	details.Flags.EnableProxy = flags.isSet(flagProxy)

	// see my prev comment about name len
	nameLen := int(binary.BigEndian.Uint16(b[12:14]))
	if nameLen > 255 {
		return fmt.Errorf("Malformed node name")
	}
	nodename := string(b[14 : 14+nameLen])
	details.Name = nodename

	return nil
}

func (dh *DistHandshake) composeStatus(b *lib.Buffer, tls bool) {
	// there are few options for the status: ok, ok_simultaneous, nok, not_allowed, alive
	// More details here: https://erlang.org/doc/apps/erts/erl_dist_protocol.html#the-handshake-in-detail
	// support "ok" only, in any other cases link will be just closed

	if tls {
		b.Allocate(4)
		dataLength := 3 // 's' + "ok"
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.Append([]byte("sok"))
		return
	}

	b.Allocate(2)
	dataLength := 3 // 's' + "ok"
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.Append([]byte("sok"))

}

func (dh *DistHandshake) readStatus(msg []byte) bool {
	if string(msg[:2]) == "ok" {
		return true
	}

	return false
}

func (dh *DistHandshake) composeChallenge(b *lib.Buffer, tls bool) {
	flags := composeFlags(dh.flags)
	if tls {
		b.Allocate(15)
		dataLength := uint32(11 + len(dh.nodename))
		binary.BigEndian.PutUint32(b.B[0:4], dataLength)
		b.B[4] = 'n'

		//https://www.erlang.org/doc/apps/erts/erl_dist_protocol.html#distribution-handshake
		// The Version is a 16-bit big endian integer and must always have the value 5
		binary.BigEndian.PutUint16(b.B[5:7], 5) // uint16

		binary.BigEndian.PutUint32(b.B[7:11], flags.toUint32()) // uint32
		binary.BigEndian.PutUint32(b.B[11:15], dh.challenge)    // uint32
		b.Append([]byte(dh.nodename))
		return
	}

	b.Allocate(13)
	dataLength := 11 + len(dh.nodename)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'n'
	//https://www.erlang.org/doc/apps/erts/erl_dist_protocol.html#distribution-handshake
	// The Version is a 16-bit big endian integer and must always have the value 5
	binary.BigEndian.PutUint16(b.B[3:5], 5)                // uint16
	binary.BigEndian.PutUint32(b.B[5:9], flags.toUint32()) // uint32
	binary.BigEndian.PutUint32(b.B[9:13], dh.challenge)    // uint32
	b.Append([]byte(dh.nodename))
}

func (dh *DistHandshake) composeChallengeVersion6(b *lib.Buffer, tls bool) {

	flags := composeFlags(dh.flags)
	if tls {
		// 1 ('N') + 8 (flags) + 4 (chalange) + 4 (creation) + 2 (len(dh.nodename))
		b.Allocate(23)
		dataLength := 19 + len(dh.nodename)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'N'
		binary.BigEndian.PutUint64(b.B[5:13], uint64(flags))             // uint64
		binary.BigEndian.PutUint32(b.B[13:17], dh.challenge)             // uint32
		binary.BigEndian.PutUint32(b.B[17:21], dh.creation)              // uint32
		binary.BigEndian.PutUint16(b.B[21:23], uint16(len(dh.nodename))) // uint16
		b.Append([]byte(dh.nodename))
		return
	}

	// 1 ('N') + 8 (flags) + 4 (chalange) + 4 (creation) + 2 (len(dh.nodename))
	b.Allocate(21)
	dataLength := 19 + len(dh.nodename)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'N'
	binary.BigEndian.PutUint64(b.B[3:11], uint64(flags))             // uint64
	binary.BigEndian.PutUint32(b.B[11:15], dh.challenge)             // uint32
	binary.BigEndian.PutUint32(b.B[15:19], dh.creation)              // uint32
	binary.BigEndian.PutUint16(b.B[19:21], uint16(len(dh.nodename))) // uint16
	b.Append([]byte(dh.nodename))
}

func (dh *DistHandshake) readChallenge(msg []byte, details *node.HandshakeDetails) (uint32, error) {
	var challenge uint32
	if len(msg) < 15 {
		return challenge, fmt.Errorf("malformed handshake challenge")
	}
	flags := nodeFlags(binary.BigEndian.Uint32(msg[2:6]))
	details.Flags = node.DefaultFlags()
	details.Flags.EnableFragmentation = flags.isSet(flagFragments)
	details.Flags.EnableBigCreation = flags.isSet(flagBigCreation)
	details.Flags.EnableHeaderAtomCache = flags.isSet(flagDistHdrAtomCache)
	details.Flags.EnableAlias = flags.isSet(flagAlias)
	details.Flags.EnableRemoteSpawn = flags.isSet(flagSpawn)
	details.Flags.EnableBigPidRef = flags.isSet(flagV4NC)

	version := binary.BigEndian.Uint16(msg[0:2])
	if version != uint16(HandshakeVersion5) {
		return challenge, fmt.Errorf("malformed handshake version %d", version)
	}
	details.Version = int(version)

	if flags.isSet(flagHandshake23) {
		// remote peer does support version 6
		details.Version = 6
	}

	details.Name = string(msg[10:])
	challenge = binary.BigEndian.Uint32(msg[6:10])
	return challenge, nil
}

func (dh *DistHandshake) readChallengeVersion6(msg []byte, details *node.HandshakeDetails) (uint32, error) {
	var challenge uint32
	flags := nodeFlags(binary.BigEndian.Uint64(msg[0:8]))
	details.Flags = node.DefaultFlags()
	details.Flags.EnableFragmentation = flags.isSet(flagFragments)
	details.Flags.EnableBigCreation = flags.isSet(flagBigCreation)
	details.Flags.EnableHeaderAtomCache = flags.isSet(flagDistHdrAtomCache)
	details.Flags.EnableAlias = flags.isSet(flagAlias)
	details.Flags.EnableRemoteSpawn = flags.isSet(flagSpawn)
	details.Flags.EnableBigPidRef = flags.isSet(flagV4NC)
	details.Flags.EnableCompression = flags.isSet(flagCompression)
	details.Flags.EnableProxy = flags.isSet(flagProxy)

	details.Creation = binary.BigEndian.Uint32(msg[12:16])
	details.Version = 6

	challenge = binary.BigEndian.Uint32(msg[8:12])

	lenName := int(binary.BigEndian.Uint16(msg[16:18]))
	details.Name = string(msg[18 : 18+lenName])

	return challenge, nil
}

func (dh *DistHandshake) readComplement(msg []byte, details *node.HandshakeDetails) {
	flags := nodeFlags(uint64(binary.BigEndian.Uint32(msg[0:4])) << 32)

	details.Flags.EnableCompression = flags.isSet(flagCompression)
	details.Flags.EnableProxy = flags.isSet(flagProxy)
	details.Creation = binary.BigEndian.Uint32(msg[4:8])
}

func (dh *DistHandshake) validateChallengeReply(b []byte, cookie string) (uint32, bool) {
	challenge := binary.BigEndian.Uint32(b[:4])
	digestB := b[4:]

	digestA := genDigest(dh.challenge, cookie)
	return challenge, bytes.Equal(digestA[:], digestB)
}

func (dh *DistHandshake) composeChallengeAck(b *lib.Buffer, challenge uint32, tls bool, cookie string) {
	if tls {
		b.Allocate(5)
		dataLength := uint32(17) // 'a' + 16 (digest)
		binary.BigEndian.PutUint32(b.B[0:4], dataLength)
		b.B[4] = 'a'
		digest := genDigest(challenge, cookie)
		b.Append(digest)
		return
	}

	b.Allocate(3)
	dataLength := uint16(17) // 'a' + 16 (digest)
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.B[2] = 'a'
	digest := genDigest(challenge, cookie)
	b.Append(digest)
}

func (dh *DistHandshake) composeChallengeReply(b *lib.Buffer, challenge uint32, tls bool, cookie string) {
	if tls {
		digest := genDigest(challenge, cookie)
		b.Allocate(9)
		dataLength := 5 + len(digest) // 1 (byte) + 4 (challenge) + 16 (digest)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'r'
		binary.BigEndian.PutUint32(b.B[5:9], dh.challenge) // uint32
		b.Append(digest)
		return
	}

	b.Allocate(7)
	digest := genDigest(challenge, cookie)
	dataLength := 5 + len(digest) // 1 (byte) + 4 (challenge) + 16 (digest)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'r'
	binary.BigEndian.PutUint32(b.B[3:7], dh.challenge) // uint32
	b.Append(digest)
}

func (dh *DistHandshake) composeComplement(b *lib.Buffer, tls bool) {
	flags := composeFlags(dh.flags)
	// cast must cast creation to int32 in order to follow the
	// erlang's handshake. Ergo don't care of it.
	node_flags := uint32(flags.toUint64() >> 32)
	if tls {
		b.Allocate(13)
		dataLength := 9 // 1 + 4 (flag high) + 4 (creation)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'c'
		binary.BigEndian.PutUint32(b.B[5:9], node_flags)
		binary.BigEndian.PutUint32(b.B[9:13], dh.creation)
		return
	}

	dataLength := 9 // 1 + 4 (flag high) + 4 (creation)
	b.Allocate(11)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'c'
	binary.BigEndian.PutUint32(b.B[3:7], node_flags)
	binary.BigEndian.PutUint32(b.B[7:11], dh.creation)
}

func genDigest(challenge uint32, cookie string) []byte {
	s := fmt.Sprintf("%s%d", cookie, challenge)
	digest := md5.Sum([]byte(s))
	return digest[:]
}

func composeFlags(flags node.Flags) nodeFlags {

	// default flags
	enabledFlags := []nodeFlagId{
		flagPublished,
		flagUnicodeIO,
		flagDistMonitor,
		flagNewFloats,
		flagBitBinaries,
		flagDistMonitorName,
		flagExtendedPidsPorts,
		flagExtendedReferences,
		flagAtomCache,
		flagHiddenAtomCache,
		flagFunTags,
		flagNewFunTags,
		flagExportPtrTag,
		flagSmallAtomTags,
		flagUTF8Atoms,
		flagMapTag,
		flagHandshake23,
	}

	// optional flags
	if flags.EnableHeaderAtomCache {
		enabledFlags = append(enabledFlags, flagDistHdrAtomCache)
	}
	if flags.EnableFragmentation {
		enabledFlags = append(enabledFlags, flagFragments)
	}
	if flags.EnableBigCreation {
		enabledFlags = append(enabledFlags, flagBigCreation)
	}
	if flags.EnableAlias {
		enabledFlags = append(enabledFlags, flagAlias)
	}
	if flags.EnableBigPidRef {
		enabledFlags = append(enabledFlags, flagV4NC)
	}
	if flags.EnableRemoteSpawn {
		enabledFlags = append(enabledFlags, flagSpawn)
	}
	if flags.EnableCompression {
		enabledFlags = append(enabledFlags, flagCompression)
	}
	if flags.EnableProxy {
		enabledFlags = append(enabledFlags, flagProxy)
	}
	return toNodeFlags(enabledFlags...)
}
