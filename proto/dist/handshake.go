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
	DistHandshakeVersion5 node.HandshakeVersion = 5
	DistHandshakeVersion6 node.HandshakeVersion = 6

	DefaultDistHandshakeVersion = DistHandshakeVersion5

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
	creation  int64
	challenge uint32
	timeout   time.Duration
	options   DistHandshakeOptions
}

type DistHandshakeOptions struct {
	Version node.HandshakeVersion // 5 or 6
	Cookie  string
}

func CreateDistHandshake(timeout time.Duration, options DistHandshakeOptions) node.HandshakeInterface {
	// must be 5 or 6
	if options.Version != DistHandshakeVersion5 && options.Version != DistHandshakeVersion6 {
		options.Version = defaultDistHandshakeVersion
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
	return h.options.Version
}

func (dh *DistHandshake) Start(conn io.ReadWriter, tls bool) (node.ProtoOptions, error) {

	var peer_challenge uint32
	var peer_name string
	var peer_flags nodeFlags
	var protoOptions node.ProtoOptions

	flags := toNodeFlag(
		flagPublished,
		flagUnicodeIO,
		flagDistMonitor,
		flagDistMonitorName,
		flagExtendedPidsPorts,
		flagExtendedReferences,
		flagAtomCache,
		flagDistHdrAtomCache,
		flagHiddenAtomCache,
		flagNewFunTags,
		flagSmallAtomTags,
		flagUTF8Atoms,
		flagMapTag,
		flagFragments,
		flagHandshake23,
		flagBigCreation,
		flagSpawn,
		flagV4NC,
		flagAlias,
	)

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var await []byte

	if dh.options.Version == DistHandshakeVersion5 {
		dh.composeName(b, tls, flags)
		// the next message must be send_status 's' or send_challenge 'n' (for
		// handshake version 5) or 'N' (for handshake version 6)
		await = []byte{'s', 'n', 'N'}
	} else {
		dh.composeNameVersion6(b, tls, flags)
		await = []byte{'s', 'N'}
	}
	if e := b.WriteDataTo(conn); e != nil {
		return protoOptions, e
	}

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
	if dh.options.TLS {
		// TLS connection has 4 bytes packet length header
		expectingBytes = 4
	}

	for {
		go asyncRead()

		select {
		case <-timer.C:
			return protoOptions, fmt.Errorf("handshake timeout")

		case e := <-asyncReadChannel:
			if e != nil {
				return protoOptions, e
			}

		next:
			l := binary.BigEndian.Uint16(b.B[expectingBytes-2 : expectingBytes])
			buffer := b.B[expectingBytes:]

			if len(buffer) < int(l) {
				return protoOptions, fmt.Errorf("malformed handshake (wrong packet length)")
			}

			// chech if we got correct message type regarding to 'await' value
			if bytes.Count(await, buffer[0:1]) == 0 {
				return protoOptions, fmt.Errorf("malformed handshake (wrong response)")
			}

			switch buffer[0] {
			case 'n':
				// 'n' + 2 (version) + 4 (flags) + 4 (challenge) + name...
				if len(b.B) < 12 {
					return protoOptions, fmt.Errorf("malformed handshake ('n')")
				}

				peer_challenge, peer_name, peer_flags = dh.readChallenge(b.B[1:])
				if challenge == 0 {
					return protoOptions, fmt.Errorf("malformed handshake (mismatch handshake version")
				}
				b.Reset()

				dh.composeChallengeReply(b, peer_challenge, tls)

				if e := b.WriteDataTo(conn); e != nil {
					return protoOptions, e
				}
				// add 's' status for the case if we got it after 'n' or 'N' message
				// yes, sometime it happens
				await = []byte{'s', 'a'}

			case 'N':
				// Peer support version 6.

				// The new challenge message format (version 6)
				// 8 (flags) + 4 (Creation) + 2 (NameLen) + Name
				if len(buffer) < 16 {
					return protoOptions, fmt.Errorf("malformed handshake ('N' length)")
				}
				peer_challenge, peer_name, peer_flags = dh.readChallengeVersion6(buffer[1:])
				b.Reset()

				if dh.options.Version == DistHandshakeVersion5 {
					// upgrade handshake to version 6 by sending complement message
					dh.composeComplement(b, tls)
					if e := b.WriteDataTo(conn); e != nil {
						return protoOptions, e
					}
				}

				dh.composeChallengeReply(b, peer_challenge, tls)

				if e := b.WriteDataTo(conn); e != nil {
					return protoOptions, e
				}

				// add 's' (send_status message) for the case if we got it after 'n' or 'N' message
				await = []byte{'s', 'a'}

			case 'a':
				// 'a' + 16 (digest)
				if len(buffer) != 17 {
					return protoOptions, fmt.Errorf("malformed handshake ('a' length of digest)")
				}

				// 'a' + 16 (digest)
				digest := genDigest(dh.challenge, dh.options.Cookie)
				if bytes.Compare(buffer[1:17], digest) != 0 {
					return protoOptions, fmt.Errorf("malformed handshake ('a' digest)")
				}

				// handshaked
				//FIXME
				protoOptions = node.DefaultProtoOptions(0, opts.Comression, false)
				return protoOptions, nil

			case 's':
				if dh.readStatus(buffer[1:]) == false {
					return protoOptions, fmt.Errorf("handshake negotiation failed")
				}

				await = []byte{'n', 'N'}
				// "sok"
				if len(buffer) > 4 {
					b.B = b.B[expectingBytes+3:]
					goto next
				}
				b.Reset()

			default:
				return protoOptions, fmt.Errorf("malformed handshake ('%c' digest)", buffer[0])
			}

		}

	}

}

func (dh *DistHandshake) Accept(conn io.ReadWriter, tls bool) (string, node.ProtoOptions, error) {
	var peer_challenge uint32
	var peer_name string
	var peer_flags nodeFlags
	var protoOptions node.ProtoOptions

	flags := toNodeFlag(
		flagPublished,
		flagUnicodeIO,
		flagDistMonitor,
		flagDistMonitorName,
		flagExtendedPidsPorts,
		flagExtendedReferences,
		flagAtomCache,
		flagDistHdrAtomCache,
		flagHiddenAtomCache,
		flagNewFunTags,
		flagSmallAtomTags,
		flagUTF8Atoms,
		flagMapTag,
		flagFragments,
		flagHandshake23,
		flagBigCreation,
		flagSpawn,
		flagV4NC,
		flagAlias,
	)

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
			return nil, fmt.Errorf("handshake accept timeout")
		case e := <-asyncReadChannel:
			if e != nil {
				return peer_name, protoOptions, e
			}

			if b.Len() < expectingBytes+1 {
				return peer_name, protoOptions, fmt.Errorf("malformed handshake (too short packet)")
			}

		next:
			l := binary.BigEndian.Uint16(b.B[expectingBytes-2 : expectingBytes])
			buffer := b.B[expectingBytes:]

			if len(buffer) < int(l) {
				return peer_name, protoOptions, fmt.Errorf("malformed handshake (wrong packet length)")
			}

			if bytes.Count(await, buffer[0:1]) == 0 {
				return peer_name, protoOptions, fmt.Errorf("malformed handshake (wrong response %d)", buffer[0])
			}

			switch buffer[0] {
			case 'n':
				if len(buffer) < 8 {
					return peer_name, protoOptions, fmt.Errorf("malformed handshake ('n' length)")
				}

				link.peer = dh.readName(buffer[1:])
				b.Reset()
				dh.composeStatus(b, options.TLS)
				if e := b.WriteDataTo(conn); e != nil {
					return peer_name, protoOptions, fmt.Errorf("malformed handshake ('n' accept name)")
				}

				b.Reset()
				if link.peer.flags.isSet(HANDSHAKE23) {
					link.composeChallengeVersion6(b, tls, flags)
					await = []byte{'s', 'r', 'c'}
				} else {
					link.version = ProtoHandshake5
					link.composeChallenge(b, tls, flags)
					await = []byte{'s', 'r'}
				}
				if e := b.WriteDataTo(conn); e != nil {
					return peer_name, protoOptions, e
				}

			case 'N':
				// The new challenge message format (version 6)
				// 8 (flags) + 4 (Creation) + 2 (NameLen) + Name
				if len(buffer) < 16 {
					return peer_name, protoOptions, fmt.Errorf("malformed handshake ('N' length)")
				}
				link.peer = link.readNameVersion6(buffer[1:])
				b.Reset()
				link.composeStatus(b, options.TLS)
				if e := b.WriteDataTo(conn); e != nil {
					return peer_name, protoOptions, fmt.Errorf("malformed handshake ('N' accept name)")
				}

				b.Reset()
				link.composeChallengeVersion6(b, options.TLS)
				if e := b.WriteDataTo(conn); e != nil {
					return peer_name, protoOptions, e
				}

				await = []byte{'s', 'r'}

			case 'c':
				if len(buffer) < 9 {
					return peer_name, protoOptions, fmt.Errorf("malformed handshake ('c' length)")
				}
				link.readComplement(buffer[1:])

				await = []byte{'r'}

				if len(buffer) > 9 {
					b.B = b.B[expectingBytes+9:]
					goto next
				}
				b.Reset()

			case 'r':
				if len(buffer) < 19 {
					return peer_name, protoOptions, fmt.Errorf("malformed handshake ('r' length)")
				}

				if !link.validateChallengeReply(buffer[1:]) {
					return peer_name, protoOptions, fmt.Errorf("malformed handshake ('r' invalid reply)")
				}
				b.Reset()

				dh.composeChallengeAck(b, options.TLS)
				if e := b.WriteDataTo(conn); e != nil {
					return peer_name, protoOptions, e
				}

				// handshaked
				// FIXME
				protoOptions = node.DefaultProtoOptions(0, opts.Comression, false)

				return peer_name, protoOptions, nil

			case 's':
				if dh.readStatus(buffer[1:]) == false {
					return peer_name, protoOptions, fmt.Errorf("link status != ok")
				}

				await = []byte{'c', 'r'}
				if len(buffer) > 4 {
					b.B = b.B[expectingBytes+3:]
					goto next
				}
				b.Reset()

			default:
				return peer_name, protoOptions, fmt.Errorf("malformed handshake (unknown code %d)", b.B[0])
			}

		}

	}
}

// private functions

func (dh *DistHandshake) composeName(b *lib.Buffer, tls bool, flags nodeFlags) {
	version := uint16(dh.options.Version)
	if tls {
		b.Allocate(11)
		dataLength := 7 + len(dh.nodename) // byte + uint16 + uint32 + len(dh.Name)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'n'
		binary.BigEndian.PutUint16(b.B[5:7], version)              // uint16
		binary.BigEndian.PutUint32(b.B[7:11], dh.flags.toUint32()) // uint32
		b.Append([]byte(dh.nodename))
		return
	}

	b.Allocate(9)
	dataLength := 7 + len(dh.Name) // byte + uint16 + uint32 + len(dh.Name)
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
		dataLength := 15 + len(dh.nodename) // 1 + 8 (flags) + 4 (creation) + 2 (len l.Name)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'N'
		binary.BigEndian.PutUint64(b.B[5:13], flags.toUint64())         // uint64
		binary.BigEndian.PutUint32(b.B[13:17], creation)                //uint32
		binary.BigEndian.PutUint16(b.B[17:19], uint16(len(dh.nodeame))) // uint16
		b.Append([]byte(dh.nodename))
		return
	}

	b.Allocate(17)
	dataLength := 15 + len(dh.nodename) // 1 + 8 (flags) + 4 (creation) + 2 (len l.Name)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'N'
	binary.BigEndian.PutUint64(b.B[3:11], flags.toUint64())          // uint64
	binary.BigEndian.PutUint32(b.B[11:15], creation)                 // uint32
	binary.BigEndian.PutUint16(b.B[15:17], uint16(len(dh.nodename))) // uint16
	b.Append([]byte(dh.nodename))
}

func (dh *DistHandshake) readName(b []byte) (string, nodeFlags) {
	nodename := string(b[6:])
	flags := nodeFlag(binary.BigEndian.Uint32(b[2:6]))
	// don't care of it. its always == 5 according to the spec
	// version := binary.BigEndian.Uint16(b[0:2])
	return nodename, flags
}

func (dh *DistHandshake) readNameVersion6(b []byte) (string, nodeFlags) {
	nameLen := int(binary.BigEndian.Uint16(b[12:14]))
	nodename := string(b[14 : 14+nameLen])
	flags := nodeFlag(binary.BigEndian.Uint64(b[0:8]))
	// don't care of peer creation value
	//creation:= binary.BigEndian.Uint32(b[8:12]),
	return nodename, flags
}

func (dh *DistHandshake) composeStatus(b *lib.Buffer) {
	//FIXME: there are few options for the status:
	//	   ok, ok_simultaneous, nok, not_allowed, alive
	// More details here: https://erlang.org/doc/apps/erts/erl_dist_protocol.html#the-handshake-in-detail
	if dh.options.tls {
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
		dataLength := uint32(11 + len(l.Name))
		binary.BigEndian.PutUint32(b.B[0:4], dataLength)
		b.B[4] = 'n'
		binary.BigEndian.PutUint16(b.B[5:7], l.version)         // uint16
		binary.BigEndian.PutUint32(b.B[7:11], flags.toUint32()) // uint32
		binary.BigEndian.PutUint32(b.B[11:15], l.challenge)     // uint32
		b.Append([]byte(l.Name))
		return
	}

	b.Allocate(13)
	dataLength := 11 + len(l.Name)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'n'
	binary.BigEndian.PutUint16(b.B[3:5], l.version)        // uint16
	binary.BigEndian.PutUint32(b.B[5:9], flags.toUint32()) // uint32
	binary.BigEndian.PutUint32(b.B[9:13], l.challenge)     // uint32
	b.Append([]byte(l.Name))
}

func (dh *DistHandshake) composeChallengeVersion6(b *lib.Buffer, tls bool, flags nodeFlags) {
	if tls {
		// 1 ('N') + 8 (flags) + 4 (chalange) + 4 (creation) + 2 (len(l.Name))
		b.Allocate(23)
		dataLength := 19 + len(l.Name)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'N'
		binary.BigEndian.PutUint64(b.B[5:13], uint64(flags))        // uint64
		binary.BigEndian.PutUint32(b.B[13:17], l.challenge)         // uint32
		binary.BigEndian.PutUint32(b.B[17:21], l.creation)          // uint32
		binary.BigEndian.PutUint16(b.B[21:23], uint16(len(l.Name))) // uint16
		b.Append([]byte(l.Name))
		return
	}

	// 1 ('N') + 8 (flags) + 4 (chalange) + 4 (creation) + 2 (len(l.Name))
	b.Allocate(21)
	dataLength := 19 + len(l.Name)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'N'
	binary.BigEndian.PutUint64(b.B[3:11], uint64(flags))        // uint64
	binary.BigEndian.PutUint32(b.B[11:15], l.challenge)         // uint32
	binary.BigEndian.PutUint32(b.B[15:19], l.creation)          // uint32
	binary.BigEndian.PutUint16(b.B[19:21], uint16(len(l.Name))) // uint16
	b.Append([]byte(l.Name))
}

// returns challange, nodename, nodeFlags
func (dh *DistHandshake) readChallenge(msg []byte) (challenge uint32, nodename string, flags nodeFlags) {
	version := binary.BigEndian.Uint16(msg[0:2])
	if version != DistHandshakeVersion5 {
		return
	}
	challenge = binary.BigEndian.Uint32(msg[6:10])
	nodename = string(msg[10:])
	flags = nodeFlag(binary.BigEndian.Uint32(msg[2:6]))
}

func (dh *DistHandshake) readChallengeVersion6(msg []byte) (challenge uint32, nodename string, flags nodeFlags) {
	lenName := int(binary.BigEndian.Uint16(msg[16:18]))
	challenge = binary.BigEndian.Uint32(msg[8:12])
	nodename = string(msg[18 : 18+lenName])
	flags = nodeFlag(binary.BigEndian.Uint64(msg[0:8]))

	// don't care about 'creation'
	// creation := binary.BigEndian.Uint32(msg[12:16]),
}

func (dh *DistHandshake) readComplement(msg []byte) {
	flags := uint64(binary.BigEndian.Uint32(msg[0:4])) << 32
	l.peer.flags = nodeFlag(l.peer.flags.toUint64() | flags)
	l.peer.creation = binary.BigEndian.Uint32(msg[4:8])
	return
}

func (dh *DistHandshake) validateChallengeReply(b []byte) bool {
	l.peer.challenge = binary.BigEndian.Uint32(b[:4])
	digestB := b[4:]

	digestA := genDigest(l.challenge, l.Cookie)
	return bytes.Equal(digestA[:], digestB)
}

func (dh *DistHandshake) composeChallengeAck(b *lib.Buffer) {
	if dh.options.TLS {
		b.Allocate(5)
		dataLength := uint32(17) // 'a' + 16 (digest)
		binary.BigEndian.PutUint32(b.B[0:4], dataLength)
		b.B[4] = 'a'
		digest := genDigest(l.peer.challenge, l.Cookie)
		b.Append(digest)
		return
	}

	b.Allocate(3)
	dataLength := uint16(17) // 'a' + 16 (digest)
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.B[2] = 'a'
	digest := genDigest(l.peer.challenge, l.Cookie)
	b.Append(digest)
}

func (dh *DistHandshake) composeChallengeReply(b *lib.Buffer, challenge uint32, tls bool) {
	if tls {
		l.digest = genDigest(challenge, dh.options.Cookie)
		b.Allocate(9)
		dataLength := 5 + len(l.digest) // 1 (byte) + 4 (challenge) + 16 (digest)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'r'
		binary.BigEndian.PutUint32(b.B[5:9], dh.challenge) // uint32
		b.Append(l.digest[:])
		return
	}

	b.Allocate(7)
	l.digest = genDigest(challenge, dh.options.Cookie)
	dataLength := 5 + len(l.digest) // 1 (byte) + 4 (challenge) + 16 (digest)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'r'
	binary.BigEndian.PutUint32(b.B[3:7], dh.challenge) // uint32
	b.Append(l.digest)
}

func (dh *DistHandshake) composeComplement(b *lib.Buffer, tls bool) {
	// cast must cast creation to int32 in order to follow the
	// erlang's handshake. Ergo don't care of it.
	creation := int32(dh.creation)
	flags := uint32(l.flags.toUint64() >> 32)
	if tls {
		b.Allocate(13)
		dataLength := 9 // 1 + 4 (flag high) + 4 (creation)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'c'
		binary.BigEndian.PutUint32(b.B[5:9], flags)
		binary.BigEndian.PutUint32(b.B[9:13], creation)
		return
	}

	dataLength := 9 // 1 + 4 (flag high) + 4 (creation)
	b.Allocate(11)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'c'
	binary.BigEndian.PutUint32(b.B[3:7], flags)
	binary.BigEndian.PutUint32(b.B[7:11], creation)
}

func genDigest(challenge uint32, cookie string) []byte {
	s := fmt.Sprintf("%s%d", cookie, challenge)
	digest := md5.Sum([]byte(s))
	return digest[:]
}
