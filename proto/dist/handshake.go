package dist

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
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
	flagAtomCache                     = 0x2
	flagExtendedReferences            = 0x4
	flagDistMonitor                   = 0x8
	flagFunTags                       = 0x10
	flagDistMonitorName               = 0x20
	flagHiddenAtomCache               = 0x40
	flagNewFunTags                    = 0x80
	flagExtendedPidsPorts             = 0x100
	flagExportPtrTag                  = 0x200
	flagBitBinaries                   = 0x400
	flagNewFloats                     = 0x800
	flagUnicodeIO                     = 0x1000
	flagDistHdrAtomCache              = 0x2000
	flagSmallAtomTags                 = 0x4000
	flagUTF8Atoms                     = 0x10000
	flagMapTag                        = 0x20000
	flagBigCreation                   = 0x40000
	flagSendSender                    = 0x80000 // since OTP.21 enable replacement for SEND (distProtoSEND by distProtoSEND_SENDER)
	flagBigSeqTraceLabels             = 0x100000
	flagExitPayload                   = 0x400000 // since OTP.22 enable replacement for EXIT, EXIT2, MONITOR_P_EXIT
	flagFragments                     = 0x800000
	flagHandshake23                   = 0x1000000 // new connection setup handshake (version 6) introduced in OTP 23
	flagUnlinkID                      = 0x2000000
	// for 64bit flags
	flagSpawn  = 1 << 32
	flagNameMe = 1 << 33
	flagV4NC   = 1 << 34
	flagAlias  = 1 << 35
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
	creation  uint32
	challenge uint32
	timeout   time.Duration
	options   HandshakeOptions
}

type HandshakeOptions struct {
	Timeout time.Duration
	Version node.HandshakeVersion // 5 or 6
	Cookie  string
	// Flags defines enabled options for the running node
	Flags node.ProtoFlags
}

func CreateHandshake(options HandshakeOptions) node.HandshakeInterface {
	// must be 5 or 6
	if options.Version != HandshakeVersion5 && options.Version != HandshakeVersion6 {
		options.Version = DefaultHandshakeVersion
	}

	emptyFlags := node.ProtoFlags{}
	if options.Flags == emptyFlags {
		options.Flags = node.DefaultProtoFlags()
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
func (dh *DistHandshake) Init(nodename string, creation uint32) error {
	dh.nodename = nodename
	dh.creation = creation
	return nil
}

func (dh *DistHandshake) Version() node.HandshakeVersion {
	return dh.options.Version
}

func (dh *DistHandshake) Start(conn io.ReadWriter, tls bool) (node.ProtoFlags, error) {

	var peer_challenge uint32
	var peer_flags nodeFlags
	var protoFlags node.ProtoFlags

	flags := composeFlags(dh.options.Flags)

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var await []byte

	if dh.options.Version == HandshakeVersion5 {
		dh.composeName(b, tls, flags)
		// the next message must be send_status 's' or send_challenge 'n' (for
		// handshake version 5) or 'N' (for handshake version 6)
		await = []byte{'s', 'n', 'N'}
	} else {
		dh.composeNameVersion6(b, tls, flags)
		await = []byte{'s', 'N'}
	}
	if e := b.WriteDataTo(conn); e != nil {
		return protoFlags, e
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
			return protoFlags, fmt.Errorf("handshake timeout")

		case e := <-asyncReadChannel:
			if e != nil {
				return protoFlags, e
			}

		next:
			l := binary.BigEndian.Uint16(b.B[expectingBytes-2 : expectingBytes])
			buffer := b.B[expectingBytes:]

			if len(buffer) < int(l) {
				return protoFlags, fmt.Errorf("malformed handshake (wrong packet length)")
			}

			// chech if we got correct message type regarding to 'await' value
			if bytes.Count(await, buffer[0:1]) == 0 {
				return protoFlags, fmt.Errorf("malformed handshake (wrong response)")
			}

			switch buffer[0] {
			case 'n':
				// 'n' + 2 (version) + 4 (flags) + 4 (challenge) + name...
				if len(b.B) < 12 {
					return protoFlags, fmt.Errorf("malformed handshake ('n')")
				}

				// ignore peer_name value if we initiate the connection
				peer_challenge, _, peer_flags = dh.readChallenge(b.B[1:])
				if peer_challenge == 0 {
					return protoFlags, fmt.Errorf("malformed handshake (mismatch handshake version")
				}
				b.Reset()

				dh.composeChallengeReply(b, peer_challenge, tls)

				if e := b.WriteDataTo(conn); e != nil {
					return protoFlags, e
				}
				// add 's' status for the case if we got it after 'n' or 'N' message
				// yes, sometime it happens
				await = []byte{'s', 'a'}

			case 'N':
				// Peer support version 6.

				// The new challenge message format (version 6)
				// 8 (flags) + 4 (Creation) + 2 (NameLen) + Name
				if len(buffer) < 16 {
					return protoFlags, fmt.Errorf("malformed handshake ('N' length)")
				}

				// ignore peer_name value if we initiate the connection
				peer_challenge, _, peer_flags = dh.readChallengeVersion6(buffer[1:])
				b.Reset()

				if dh.options.Version == HandshakeVersion5 {
					// upgrade handshake to version 6 by sending complement message
					dh.composeComplement(b, flags, tls)
					if e := b.WriteDataTo(conn); e != nil {
						return protoFlags, e
					}
				}

				dh.composeChallengeReply(b, peer_challenge, tls)

				if e := b.WriteDataTo(conn); e != nil {
					return protoFlags, e
				}

				// add 's' (send_status message) for the case if we got it after 'n' or 'N' message
				await = []byte{'s', 'a'}

			case 'a':
				// 'a' + 16 (digest)
				if len(buffer) != 17 {
					return protoFlags, fmt.Errorf("malformed handshake ('a' length of digest)")
				}

				// 'a' + 16 (digest)
				digest := genDigest(dh.challenge, dh.options.Cookie)
				if bytes.Compare(buffer[1:17], digest) != 0 {
					return protoFlags, fmt.Errorf("malformed handshake ('a' digest)")
				}

				// handshaked
				protoFlags = node.DefaultProtoFlags()
				protoFlags.EnableFragmentation = peer_flags.isSet(flagFragments)
				protoFlags.EnableBigCreation = peer_flags.isSet(flagBigCreation)
				protoFlags.EnableHeaderAtomCache = peer_flags.isSet(flagDistHdrAtomCache)
				protoFlags.EnableAlias = peer_flags.isSet(flagAlias)
				protoFlags.EnableRemoteSpawn = peer_flags.isSet(flagSpawn)
				protoFlags.EnableBigPidRef = peer_flags.isSet(flagV4NC)
				return protoFlags, nil

			case 's':
				if dh.readStatus(buffer[1:]) == false {
					return protoFlags, fmt.Errorf("handshake negotiation failed")
				}

				await = []byte{'n', 'N'}
				// "sok"
				if len(buffer) > 4 {
					b.B = b.B[expectingBytes+3:]
					goto next
				}
				b.Reset()

			default:
				return protoFlags, fmt.Errorf("malformed handshake ('%c' digest)", buffer[0])
			}

		}

	}

}

func (dh *DistHandshake) Accept(conn io.ReadWriter, tls bool) (string, node.ProtoFlags, error) {
	var peer_challenge uint32
	var peer_name string
	var peer_flags nodeFlags
	var protoFlags node.ProtoFlags
	var err error

	flags := composeFlags(dh.options.Flags)

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var await []byte

	// define timeout for the handshaking
	timer := time.NewTimer(dh.timeout)
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
			return peer_name, protoFlags, fmt.Errorf("handshake accept timeout")
		case e := <-asyncReadChannel:
			if e != nil {
				return peer_name, protoFlags, e
			}

			if b.Len() < expectingBytes+1 {
				return peer_name, protoFlags, fmt.Errorf("malformed handshake (too short packet)")
			}

		next:
			l := binary.BigEndian.Uint16(b.B[expectingBytes-2 : expectingBytes])
			buffer := b.B[expectingBytes:]

			if len(buffer) < int(l) {
				return peer_name, protoFlags, fmt.Errorf("malformed handshake (wrong packet length)")
			}

			if bytes.Count(await, buffer[0:1]) == 0 {
				return peer_name, protoFlags, fmt.Errorf("malformed handshake (wrong response %d)", buffer[0])
			}

			switch buffer[0] {
			case 'n':
				if len(buffer) < 8 {
					return peer_name, protoFlags, fmt.Errorf("malformed handshake ('n' length)")
				}

				peer_name, peer_flags, err = dh.readName(buffer[1:])
				if err != nil {
					return peer_name, protoFlags, err
				}
				b.Reset()
				dh.composeStatus(b, tls)
				if e := b.WriteDataTo(conn); e != nil {
					return peer_name, protoFlags, fmt.Errorf("malformed handshake ('n' accept name)")
				}

				b.Reset()
				if peer_flags.isSet(flagHandshake23) {
					dh.composeChallengeVersion6(b, tls, flags)
					await = []byte{'s', 'r', 'c'}
				} else {
					dh.composeChallenge(b, tls, flags)
					await = []byte{'s', 'r'}
				}
				if e := b.WriteDataTo(conn); e != nil {
					return peer_name, protoFlags, e
				}

			case 'N':
				// The new challenge message format (version 6)
				// 8 (flags) + 4 (Creation) + 2 (NameLen) + Name
				if len(buffer) < 16 {
					return peer_name, protoFlags, fmt.Errorf("malformed handshake ('N' length)")
				}
				peer_name, peer_flags, err = dh.readNameVersion6(buffer[1:])
				if err != nil {
					return peer_name, protoFlags, err
				}
				b.Reset()
				dh.composeStatus(b, tls)
				if e := b.WriteDataTo(conn); e != nil {
					return peer_name, protoFlags, fmt.Errorf("malformed handshake ('N' accept name)")
				}

				b.Reset()
				dh.composeChallengeVersion6(b, tls, flags)
				if e := b.WriteDataTo(conn); e != nil {
					return peer_name, protoFlags, e
				}

				await = []byte{'s', 'r'}

			case 'c':
				if len(buffer) < 9 {
					return peer_name, protoFlags, fmt.Errorf("malformed handshake ('c' length)")
				}
				peer_flags = dh.readComplement(buffer[1:], peer_flags)

				await = []byte{'r'}

				if len(buffer) > 9 {
					b.B = b.B[expectingBytes+9:]
					goto next
				}
				b.Reset()

			case 'r':
				var valid bool
				if len(buffer) < 19 {
					return peer_name, protoFlags, fmt.Errorf("malformed handshake ('r' length)")
				}

				peer_challenge, valid = dh.validateChallengeReply(buffer[1:])
				if valid == false {
					return peer_name, protoFlags, fmt.Errorf("malformed handshake ('r' invalid reply)")
				}
				b.Reset()

				dh.composeChallengeAck(b, peer_challenge, tls)
				if e := b.WriteDataTo(conn); e != nil {
					return peer_name, protoFlags, e
				}

				// handshaked
				protoFlags = node.DefaultProtoFlags()
				protoFlags.EnableFragmentation = peer_flags.isSet(flagFragments)
				protoFlags.EnableBigCreation = peer_flags.isSet(flagBigCreation)
				protoFlags.EnableHeaderAtomCache = peer_flags.isSet(flagDistHdrAtomCache)
				protoFlags.EnableAlias = peer_flags.isSet(flagAlias)
				protoFlags.EnableRemoteSpawn = peer_flags.isSet(flagSpawn)
				protoFlags.EnableBigPidRef = peer_flags.isSet(flagV4NC)

				return peer_name, protoFlags, nil

			case 's':
				if dh.readStatus(buffer[1:]) == false {
					return peer_name, protoFlags, fmt.Errorf("link status != ok")
				}

				await = []byte{'c', 'r'}
				if len(buffer) > 4 {
					b.B = b.B[expectingBytes+3:]
					goto next
				}
				b.Reset()

			default:
				return peer_name, protoFlags, fmt.Errorf("malformed handshake (unknown code %d)", b.B[0])
			}

		}

	}
}

// private functions

func (dh *DistHandshake) composeName(b *lib.Buffer, tls bool, flags nodeFlags) {
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

func (dh *DistHandshake) composeNameVersion6(b *lib.Buffer, tls bool, flags nodeFlags) {
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

func (dh *DistHandshake) readName(b []byte) (string, nodeFlags, error) {
	if len(b[6:]) > 250 {
		return "", 0, fmt.Errorf("Malformed node name")
	}
	nodename := string(b[6:])
	flags := nodeFlags(binary.BigEndian.Uint32(b[2:6]))

	// don't care of version value. its always == 5 according to the spec
	// version := binary.BigEndian.Uint16(b[0:2])

	return nodename, flags, nil
}

func (dh *DistHandshake) readNameVersion6(b []byte) (string, nodeFlags, error) {
	nameLen := int(binary.BigEndian.Uint16(b[12:14]))
	if nameLen > 250 {
		return "", 0, fmt.Errorf("Malformed node name")
	}
	nodename := string(b[14 : 14+nameLen])
	flags := nodeFlags(binary.BigEndian.Uint64(b[0:8]))

	// don't care of peer creation value
	// creation:= binary.BigEndian.Uint32(b[8:12]),

	return nodename, flags, nil
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

func (dh *DistHandshake) composeChallenge(b *lib.Buffer, tls bool, flags nodeFlags) {
	if tls {
		b.Allocate(15)
		dataLength := uint32(11 + len(dh.nodename))
		binary.BigEndian.PutUint32(b.B[0:4], dataLength)
		b.B[4] = 'n'
		binary.BigEndian.PutUint16(b.B[5:7], uint16(dh.options.Version)) // uint16
		binary.BigEndian.PutUint32(b.B[7:11], flags.toUint32())          // uint32
		binary.BigEndian.PutUint32(b.B[11:15], dh.challenge)             // uint32
		b.Append([]byte(dh.nodename))
		return
	}

	b.Allocate(13)
	dataLength := 11 + len(dh.nodename)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'n'
	binary.BigEndian.PutUint16(b.B[3:5], uint16(dh.options.Version)) // uint16
	binary.BigEndian.PutUint32(b.B[5:9], flags.toUint32())           // uint32
	binary.BigEndian.PutUint32(b.B[9:13], dh.challenge)              // uint32
	b.Append([]byte(dh.nodename))
}

func (dh *DistHandshake) composeChallengeVersion6(b *lib.Buffer, tls bool, flags nodeFlags) {
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

// returns challange, nodename, nodeFlags
func (dh *DistHandshake) readChallenge(msg []byte) (challenge uint32, nodename string, flags nodeFlags) {
	version := binary.BigEndian.Uint16(msg[0:2])
	if version != uint16(HandshakeVersion5) {
		return
	}
	challenge = binary.BigEndian.Uint32(msg[6:10])
	nodename = string(msg[10:])
	flags = nodeFlags(binary.BigEndian.Uint32(msg[2:6]))
	return
}

func (dh *DistHandshake) readChallengeVersion6(msg []byte) (challenge uint32, nodename string, flags nodeFlags) {
	lenName := int(binary.BigEndian.Uint16(msg[16:18]))
	challenge = binary.BigEndian.Uint32(msg[8:12])
	nodename = string(msg[18 : 18+lenName])
	flags = nodeFlags(binary.BigEndian.Uint64(msg[0:8]))

	// don't care about 'creation'
	// creation := binary.BigEndian.Uint32(msg[12:16]),
	return
}

func (dh *DistHandshake) readComplement(msg []byte, peer_flags nodeFlags) nodeFlags {
	flags := uint64(binary.BigEndian.Uint32(msg[0:4])) << 32
	peer_flags = nodeFlags(peer_flags.toUint64() | flags)
	// creation = binary.BigEndian.Uint32(msg[4:8])
	return peer_flags
}

func (dh *DistHandshake) validateChallengeReply(b []byte) (uint32, bool) {
	challenge := binary.BigEndian.Uint32(b[:4])
	digestB := b[4:]

	digestA := genDigest(dh.challenge, dh.options.Cookie)
	return challenge, bytes.Equal(digestA[:], digestB)
}

func (dh *DistHandshake) composeChallengeAck(b *lib.Buffer, peer_challenge uint32, tls bool) {
	if tls {
		b.Allocate(5)
		dataLength := uint32(17) // 'a' + 16 (digest)
		binary.BigEndian.PutUint32(b.B[0:4], dataLength)
		b.B[4] = 'a'
		digest := genDigest(peer_challenge, dh.options.Cookie)
		b.Append(digest)
		return
	}

	b.Allocate(3)
	dataLength := uint16(17) // 'a' + 16 (digest)
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.B[2] = 'a'
	digest := genDigest(peer_challenge, dh.options.Cookie)
	b.Append(digest)
}

func (dh *DistHandshake) composeChallengeReply(b *lib.Buffer, challenge uint32, tls bool) {
	if tls {
		digest := genDigest(challenge, dh.options.Cookie)
		b.Allocate(9)
		dataLength := 5 + len(digest) // 1 (byte) + 4 (challenge) + 16 (digest)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'r'
		binary.BigEndian.PutUint32(b.B[5:9], dh.challenge) // uint32
		b.Append(digest)
		return
	}

	b.Allocate(7)
	digest := genDigest(challenge, dh.options.Cookie)
	dataLength := 5 + len(digest) // 1 (byte) + 4 (challenge) + 16 (digest)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'r'
	binary.BigEndian.PutUint32(b.B[3:7], dh.challenge) // uint32
	b.Append(digest)
}

func (dh *DistHandshake) composeComplement(b *lib.Buffer, flags nodeFlags, tls bool) {
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

func composeFlags(flags node.ProtoFlags) nodeFlags {

	// default flags
	enabledFlags := []nodeFlagId{
		flagPublished,
		flagUnicodeIO,
		flagDistMonitor,
		flagDistMonitorName,
		flagExtendedPidsPorts,
		flagExtendedReferences,
		flagAtomCache,
		flagHiddenAtomCache,
		flagNewFunTags,
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
	return toNodeFlags(enabledFlags...)
}
