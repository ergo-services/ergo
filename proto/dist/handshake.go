package node

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/ergo-services/ergo/lib"
)

type DistHandshakeVersion int

const (
	DistHandshakeVersion5 DistHandshakeVersion = 5
	DistHandshakeVersion6 DistHandshakeVersion = 6

	defaultDistHandshakeVersion = DistHandshakeVersion5

	// distribution flags are defined here https://erlang.org/doc/apps/erts/erl_dist_protocol.html#distribution-flags
	flagPublished          flagId = 0x1
	flagAtomCache                 = 0x2
	flagExtendedReferences        = 0x4
	flagDistMonitor               = 0x8
	flagFunTags                   = 0x10
	flagDistMonitorName           = 0x20
	flagHiddenAtomCache           = 0x40
	flagNewFunTags                = 0x80
	flagExtendedPidsPorts         = 0x100
	flagExportPtrTag              = 0x200
	flagBitBinaries               = 0x400
	flagNewFloats                 = 0x800
	flagUnicodeIO                 = 0x1000
	flagDistHdrAtomCache          = 0x2000
	flagSmallAtomTags             = 0x4000
	flagUTF8Atoms                 = 0x10000
	flagMapTag                    = 0x20000
	flagBigCreation               = 0x40000
	flagSendSender                = 0x80000 // since OTP.21 enable replacement for SEND (distProtoSEND by distProtoSEND_SENDER)
	flagBigSeqTraceLabels         = 0x100000
	flagExitPayload               = 0x400000 // since OTP.22 enable replacement for EXIT, EXIT2, MONITOR_P_EXIT
	flagFragments                 = 0x800000
	flagHandshake23               = 0x1000000 // new connection setup handshake (version 6) introduced in OTP 23
	flagUnlinkID                  = 0x2000000
	// for 64bit flags
	flagSpawn  = 1 << 32
	flagNameMe = 1 << 33
	flagV4NC   = 1 << 34
	flagAlias  = 1 << 35
)

type nodeFlagId uint64
type nodeFlag nodeFlagId

func (nf nodeFlag) toUint32() uint32 {
	return uint32(nf)
}

func (nf nodeFlag) toUint64() uint64 {
	return uint64(nf)
}

func (nf nodeFlag) isSet(f nodeFlagId) bool {
	return (uint64(nf) & uint64(f)) != 0
}

func toNodeFlag(f ...nodeFlagId) nodeFlag {
	var flags uint64
	for _, v := range f {
		flags |= uint64(v)
	}
	return nodeFlag(flags)
}

// DistHandshake implements Erlang handshake
type DistHandshake struct {
	Handshake
	opts DistHandshakeOptions
}

type DistHandshakeOptions struct {
	Version    DistHandshakeVersion // 5 or 6
	Name       string
	Cookie     string
	EnabledTLS bool
	Hidden     bool
	Creation   uint32
}

type DistConnection struct {
	node.Connection
}

func CreateDistHandshake(options DistHandshakeOptions) *DistHandshake {
	// must be 5 or 6
	if options.Version != DistHandshakeVersion5 && options.Version != DistHandshakeVersion6 {
		options.Version = defaultDistHandshakeVersion
	}
	return &DistHandshake{
		opts: options,
	}
}

func (h *DistHandshake) Start(ctx context.Context, conn net.Conn) (*node.Connection, error) {

	link := &Link{
		Name:   options.Name,
		Cookie: options.Cookie,

		flags: toNodeFlag(
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
		),

		conn: conn,
		// sequenceID is using for the fragmentation messages
		sequenceID: time.Now().UnixNano(),

		version:  uint16(options.Version),
		creation: options.Creation,
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var await []byte

	if h.version == ProtoHandshake5 {
		h.composeName(b, options.TLS)
		// the next message must be send_status 's' or send_challenge 'n' (for
		// handshake version 5) or 'N' (for handshake version 6)
		await = []byte{'s', 'n', 'N'}
	} else {
		h.composeNameVersion6(b, options.TLS)
		await = []byte{'s', 'N'}
	}
	if e := b.WriteDataTo(conn); e != nil {
		return nil, e
	}

	// define timeout for the handshaking
	timer := time.NewTimer(5 * time.Second)
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
	if options.TLS {
		// TLS connection has 4 bytes packet length header
		expectingBytes = 4
	}

	for {
		go asyncRead()

		select {
		case <-timer.C:
			return nil, fmt.Errorf("handshake timeout")

		case e := <-asyncReadChannel:
			if e != nil {
				return nil, e
			}

		next:
			l := binary.BigEndian.Uint16(b.B[expectingBytes-2 : expectingBytes])
			buffer := b.B[expectingBytes:]

			if len(buffer) < int(l) {
				return nil, fmt.Errorf("malformed handshake (wrong packet length)")
			}

			// chech if we got correct message type regarding to 'await' value
			if bytes.Count(await, buffer[0:1]) == 0 {
				return nil, fmt.Errorf("malformed handshake (wrong response)")
			}

			switch buffer[0] {
			case 'n':
				// 'n' + 2 (version) + 4 (flags) + 4 (challenge) + name...
				if len(b.B) < 12 {
					return nil, fmt.Errorf("malformed handshake ('n')")
				}

				challenge := link.readChallenge(b.B[1:])
				if challenge == 0 {
					return nil, fmt.Errorf("malformed handshake (mismatch handshake version")
				}
				b.Reset()

				link.composeChallengeReply(b, challenge, options.TLS)

				if e := b.WriteDataTo(conn); e != nil {
					return nil, e
				}
				// add 's' status for the case if we got it after 'n' or 'N' message
				await = []byte{'s', 'a'}

			case 'N':
				// Peer support version 6.

				// The new challenge message format (version 6)
				// 8 (flags) + 4 (Creation) + 2 (NameLen) + Name
				if len(buffer) < 16 {
					return nil, fmt.Errorf("malformed handshake ('N' length)")
				}
				challenge := link.readChallengeVersion6(buffer[1:])
				b.Reset()

				if link.version == ProtoHandshake5 {
					// send complement message
					link.composeComplement(b, options.TLS)
					if e := b.WriteDataTo(conn); e != nil {
						return nil, e
					}
					link.version = ProtoHandshake6
				}

				link.composeChallengeReply(b, challenge, options.TLS)

				if e := b.WriteDataTo(conn); e != nil {
					return nil, e
				}

				// add 's' (send_status message) for the case if we got it after 'n' or 'N' message
				await = []byte{'s', 'a'}

			case 'a':
				// 'a' + 16 (digest)
				if len(buffer) != 17 {
					return nil, fmt.Errorf("malformed handshake ('a' length of digest)")
				}

				// 'a' + 16 (digest)
				digest := genDigest(link.peer.challenge, link.Cookie)
				if bytes.Compare(buffer[1:17], digest) != 0 {
					return nil, fmt.Errorf("malformed handshake ('a' digest)")
				}

				// handshaked
				link.flusher = newLinkFlusher(link.conn, defaultLatency)
				return link, nil

			case 's':
				if link.readStatus(buffer[1:]) == false {
					return nil, fmt.Errorf("handshake negotiation failed")
				}

				await = []byte{'n', 'N'}
				// "sok"
				if len(buffer) > 4 {
					b.B = b.B[expectingBytes+3:]
					goto next
				}
				b.Reset()

			default:
				return nil, fmt.Errorf("malformed handshake ('%c' digest)", buffer[0])
			}

		}

	}

}

func (h *DistHandshake) Accept(ctx context.Context, conn net.Conn, opts DistHandshakeOptions) (*node.Connection, error) {
	link := &Link{
		Name:   options.Name,
		Cookie: options.Cookie,
		Hidden: options.Hidden,

		flags: toNodeFlag(PUBLISHED, UNICODE_IO, DIST_MONITOR, DIST_MONITOR_NAME,
			EXTENDED_PIDS_PORTS, EXTENDED_REFERENCES, ATOM_CACHE,
			DIST_HDR_ATOM_CACHE, HIDDEN_ATOM_CACHE, NEW_FUN_TAGS,
			SMALL_ATOM_TAGS, UTF8_ATOMS, MAP_TAG,
			FRAGMENTS, HANDSHAKE23, BIG_CREATION, SPAWN, V4_NC, ALIAS,
		),

		conn:       conn,
		sequenceID: time.Now().UnixNano(),
		challenge:  rand.Uint32(),
		version:    ProtoHandshake6,
		creation:   options.Creation,
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var await []byte

	// define timeout for the handshaking
	timer := time.NewTimer(5 * time.Second)
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
	if options.TLS {
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
				return nil, e
			}

			if b.Len() < expectingBytes+1 {
				return nil, fmt.Errorf("malformed handshake (too short packet)")
			}

		next:
			l := binary.BigEndian.Uint16(b.B[expectingBytes-2 : expectingBytes])
			buffer := b.B[expectingBytes:]

			if len(buffer) < int(l) {
				return nil, fmt.Errorf("malformed handshake (wrong packet length)")
			}

			if bytes.Count(await, buffer[0:1]) == 0 {
				return nil, fmt.Errorf("malformed handshake (wrong response %d)", buffer[0])
			}

			switch buffer[0] {
			case 'n':
				if len(buffer) < 8 {
					return nil, fmt.Errorf("malformed handshake ('n' length)")
				}

				link.peer = link.readName(buffer[1:])
				b.Reset()
				link.composeStatus(b, options.TLS)
				if e := b.WriteDataTo(conn); e != nil {
					return nil, fmt.Errorf("malformed handshake ('n' accept name)")
				}

				b.Reset()
				if link.peer.flags.isSet(HANDSHAKE23) {
					link.composeChallengeVersion6(b, options.TLS)
					await = []byte{'s', 'r', 'c'}
				} else {
					link.version = ProtoHandshake5
					link.composeChallenge(b, options.TLS)
					await = []byte{'s', 'r'}
				}
				if e := b.WriteDataTo(conn); e != nil {
					return nil, e
				}

			case 'N':
				// The new challenge message format (version 6)
				// 8 (flags) + 4 (Creation) + 2 (NameLen) + Name
				if len(buffer) < 16 {
					return nil, fmt.Errorf("malformed handshake ('N' length)")
				}
				link.peer = link.readNameVersion6(buffer[1:])
				b.Reset()
				link.composeStatus(b, options.TLS)
				if e := b.WriteDataTo(conn); e != nil {
					return nil, fmt.Errorf("malformed handshake ('N' accept name)")
				}

				b.Reset()
				link.composeChallengeVersion6(b, options.TLS)
				if e := b.WriteDataTo(conn); e != nil {
					return nil, e
				}

				await = []byte{'s', 'r'}

			case 'c':
				if len(buffer) < 9 {
					return nil, fmt.Errorf("malformed handshake ('c' length)")
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
					return nil, fmt.Errorf("malformed handshake ('r' length)")
				}

				if !link.validateChallengeReply(buffer[1:]) {
					return nil, fmt.Errorf("malformed handshake ('r' invalid reply)")
				}
				b.Reset()

				link.composeChallengeAck(b, options.TLS)
				if e := b.WriteDataTo(conn); e != nil {
					return nil, e
				}

				// handshaked
				link.flusher = newLinkFlusher(link.conn, defaultLatency)

				return link, nil

			case 's':
				if link.readStatus(buffer[1:]) == false {
					return nil, fmt.Errorf("link status !ok")
				}

				await = []byte{'c', 'r'}
				if len(buffer) > 4 {
					b.B = b.B[expectingBytes+3:]
					goto next
				}
				b.Reset()

			default:
				return nil, fmt.Errorf("malformed handshake (unknown code %d)", b.B[0])
			}

		}

	}
}

// private functions

func (dh *DistHandshake) composeName(b *lib.Buffer, tls bool) {
	if tls {
		b.Allocate(11)
		dataLength := 7 + len(dh.Name) // byte + uint16 + uint32 + len(dh.Name)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'n'
		binary.BigEndian.PutUint16(b.B[5:7], dh.version)           // uint16
		binary.BigEndian.PutUint32(b.B[7:11], dh.flags.toUint32()) // uint32
		b.Append([]byte(dh.Name))
		return
	}

	b.Allocate(9)
	dataLength := 7 + len(dh.Name) // byte + uint16 + uint32 + len(dh.Name)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'n'
	binary.BigEndian.PutUint16(b.B[3:5], dh.version)          // uint16
	binary.BigEndian.PutUint32(b.B[5:9], dh.flags.toUint32()) // uint32
	b.Append([]byte(dh.Name))
}

func (dh *DistHandshake) composeNameVersion6(b *lib.Buffer, tls bool) {
	if tls {
		b.Allocate(19)
		dataLength := 15 + len(l.Name) // 1 + 8 (flags) + 4 (creation) + 2 (len l.Name)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'N'
		binary.BigEndian.PutUint64(b.B[5:13], l.flags.toUint64())   // uint64
		binary.BigEndian.PutUint32(b.B[13:17], l.creation)          //uint32
		binary.BigEndian.PutUint16(b.B[17:19], uint16(len(l.Name))) // uint16
		b.Append([]byte(l.Name))
		return
	}

	b.Allocate(17)
	dataLength := 15 + len(l.Name) // 1 + 8 (flags) + 4 (creation) + 2 (len l.Name)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'N'
	binary.BigEndian.PutUint64(b.B[3:11], l.flags.toUint64())   // uint64
	binary.BigEndian.PutUint32(b.B[11:15], l.creation)          // uint32
	binary.BigEndian.PutUint16(b.B[15:17], uint16(len(l.Name))) // uint16
	b.Append([]byte(l.Name))
}

func (dh *DistHandshake) readName(b []byte) *Link {
	peer := &Link{
		Name:    string(b[6:]),
		version: binary.BigEndian.Uint16(b[0:2]),
		flags:   nodeFlag(binary.BigEndian.Uint32(b[2:6])),
	}
	return peer
}

func (dh *DistHandshake) readNameVersion6(b []byte) *Link {
	nameLen := int(binary.BigEndian.Uint16(b[12:14]))
	peer := &Link{
		flags:    nodeFlag(binary.BigEndian.Uint64(b[0:8])),
		creation: binary.BigEndian.Uint32(b[8:12]),
		Name:     string(b[14 : 14+nameLen]),
		version:  ProtoHandshake6,
	}
	return peer
}

func (dh *DistHandshake) composeStatus(b *lib.Buffer, tls bool) {
	//FIXME: there are few options for the status:
	//	   ok, ok_simultaneous, nok, not_allowed, alive
	// More details here: https://erlang.org/doc/apps/erts/erl_dist_protocol.html#the-handshake-in-detail
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
	if tls {
		b.Allocate(15)
		dataLength := uint32(11 + len(l.Name))
		binary.BigEndian.PutUint32(b.B[0:4], dataLength)
		b.B[4] = 'n'
		binary.BigEndian.PutUint16(b.B[5:7], l.version)           // uint16
		binary.BigEndian.PutUint32(b.B[7:11], l.flags.toUint32()) // uint32
		binary.BigEndian.PutUint32(b.B[11:15], l.challenge)       // uint32
		b.Append([]byte(l.Name))
		return
	}

	b.Allocate(13)
	dataLength := 11 + len(l.Name)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'n'
	binary.BigEndian.PutUint16(b.B[3:5], l.version)          // uint16
	binary.BigEndian.PutUint32(b.B[5:9], l.flags.toUint32()) // uint32
	binary.BigEndian.PutUint32(b.B[9:13], l.challenge)       // uint32
	b.Append([]byte(l.Name))
}

func (dh *DistHandshake) composeChallengeVersion6(b *lib.Buffer, tls bool) {
	if tls {
		// 1 ('N') + 8 (flags) + 4 (chalange) + 4 (creation) + 2 (len(l.Name))
		b.Allocate(23)
		dataLength := 19 + len(l.Name)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'N'
		binary.BigEndian.PutUint64(b.B[5:13], uint64(l.flags))      // uint64
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
	binary.BigEndian.PutUint64(b.B[3:11], uint64(l.flags))      // uint64
	binary.BigEndian.PutUint32(b.B[11:15], l.challenge)         // uint32
	binary.BigEndian.PutUint32(b.B[15:19], l.creation)          // uint32
	binary.BigEndian.PutUint16(b.B[19:21], uint16(len(l.Name))) // uint16
	b.Append([]byte(l.Name))
}

func (dh *DistHandshake) readChallenge(msg []byte) (challenge uint32) {
	version := binary.BigEndian.Uint16(msg[0:2])
	if version != ProtoHandshake5 {
		return 0
	}

	link := &Link{
		Name:    string(msg[10:]),
		version: version,
		flags:   nodeFlag(binary.BigEndian.Uint32(msg[2:6])),
	}
	l.peer = link
	return binary.BigEndian.Uint32(msg[6:10])
}

func (dh *DistHandshake) readChallengeVersion6(msg []byte) (challenge uint32) {
	lenName := int(binary.BigEndian.Uint16(msg[16:18]))
	link := &Link{
		Name:     string(msg[18 : 18+lenName]),
		version:  ProtoHandshake6,
		flags:    nodeFlag(binary.BigEndian.Uint64(msg[0:8])),
		creation: binary.BigEndian.Uint32(msg[12:16]),
	}
	l.peer = link
	return binary.BigEndian.Uint32(msg[8:12])
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

func (dh *DistHandshake) composeChallengeAck(b *lib.Buffer, tls bool) {
	if tls {
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
		l.digest = genDigest(challenge, l.Cookie)
		b.Allocate(9)
		dataLength := 5 + len(l.digest) // 1 (byte) + 4 (challenge) + 16 (digest)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'r'
		binary.BigEndian.PutUint32(b.B[5:9], l.challenge) // uint32
		b.Append(l.digest[:])
		return
	}

	b.Allocate(7)
	l.digest = genDigest(challenge, l.Cookie)
	dataLength := 5 + len(l.digest) // 1 (byte) + 4 (challenge) + 16 (digest)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'r'
	binary.BigEndian.PutUint32(b.B[3:7], l.challenge) // uint32
	b.Append(l.digest)
}

func (dh *DistHandshake) composeComplement(b *lib.Buffer, tls bool) {
	flags := uint32(l.flags.toUint64() >> 32)
	if tls {
		b.Allocate(13)
		dataLength := 9 // 1 + 4 (flag high) + 4 (creation)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'c'
		binary.BigEndian.PutUint32(b.B[5:9], flags)
		binary.BigEndian.PutUint32(b.B[9:13], l.creation)
		return
	}

	dataLength := 9 // 1 + 4 (flag high) + 4 (creation)
	b.Allocate(11)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'c'
	binary.BigEndian.PutUint32(b.B[3:7], flags)
	binary.BigEndian.PutUint32(b.B[7:11], l.creation)
	return
}

func genDigest(challenge uint32, cookie string) []byte {
	s := fmt.Sprintf("%s%d", cookie, challenge)
	digest := md5.Sum([]byte(s))
	return digest[:]
}
