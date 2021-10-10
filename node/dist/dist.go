package dist

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

var (
	ErrMissingInCache = fmt.Errorf("Missing in cache")
	ErrMalformed      = fmt.Errorf("Malformed")
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

type flagId uint64
type nodeFlag flagId

const (
	defaultLatency = 200 * time.Nanosecond // for linkFlusher

	defaultCleanTimeout  = 5 * time.Second  // for checkClean
	defaultCleanDeadline = 30 * time.Second // for checkClean

	// http://erlang.org/doc/apps/erts/erl_ext_dist.html#distribution_header
	protoDist           = 131
	protoDistCompressed = 80
	protoDistMessage    = 68
	protoDistFragment1  = 69
	protoDistFragmentN  = 70

	ProtoHandshake5 = 5
	ProtoHandshake6 = 6

	// distribution flags are defined here https://erlang.org/doc/apps/erts/erl_dist_protocol.html#distribution-flags
	PUBLISHED           flagId = 0x1
	ATOM_CACHE                 = 0x2
	EXTENDED_REFERENCES        = 0x4
	DIST_MONITOR               = 0x8
	FUN_TAGS                   = 0x10
	DIST_MONITOR_NAME          = 0x20
	HIDDEN_ATOM_CACHE          = 0x40
	NEW_FUN_TAGS               = 0x80
	EXTENDED_PIDS_PORTS        = 0x100
	EXPORT_PTR_TAG             = 0x200
	BIT_BINARIES               = 0x400
	NEW_FLOATS                 = 0x800
	UNICODE_IO                 = 0x1000
	DIST_HDR_ATOM_CACHE        = 0x2000
	SMALL_ATOM_TAGS            = 0x4000
	UTF8_ATOMS                 = 0x10000
	MAP_TAG                    = 0x20000
	BIG_CREATION               = 0x40000
	SEND_SENDER                = 0x80000 // since OTP.21 enable replacement for SEND (distProtoSEND by distProtoSEND_SENDER)
	BIG_SEQTRACE_LABELS        = 0x100000
	EXIT_PAYLOAD               = 0x400000 // since OTP.22 enable replacement for EXIT, EXIT2, MONITOR_P_EXIT
	FRAGMENTS                  = 0x800000
	HANDSHAKE23                = 0x1000000 // new connection setup handshake (version 6) introduced in OTP 23
	UNLINK_ID                  = 0x2000000
	// for 64bit flags
	SPAWN   = 1 << 32
	NAME_ME = 1 << 33
	V4_NC   = 1 << 34
	ALIAS   = 1 << 35
)

type HandshakeOptions struct {
	Version  int // 5 or 6
	Name     string
	Cookie   string
	TLS      bool
	Hidden   bool
	Creation uint32
}

func (nf nodeFlag) toUint32() uint32 {
	return uint32(nf)
}

func (nf nodeFlag) toUint64() uint64 {
	return uint64(nf)
}

func (nf nodeFlag) isSet(f flagId) bool {
	return (uint64(nf) & uint64(f)) != 0
}

func toNodeFlag(f ...flagId) nodeFlag {
	var flags uint64
	for _, v := range f {
		flags |= uint64(v)
	}
	return nodeFlag(flags)
}

type fragmentedPacket struct {
	buffer           *lib.Buffer
	disordered       *lib.Buffer
	disorderedSlices map[uint64][]byte
	fragmentID       uint64
	lastUpdate       time.Time
}

type Link struct {
	Name      string
	Cookie    string
	Hidden    bool
	peer      *Link
	conn      net.Conn
	challenge uint32
	flags     nodeFlag
	version   uint16
	creation  uint32
	digest    []byte

	// writer
	flusher *linkFlusher

	// atom cache for incomming messages
	cacheIn      [2048]*etf.Atom
	cacheInMutex sync.Mutex

	// atom cache for outgoing messages
	cacheOut *etf.AtomCache

	// fragmentation sequence ID
	sequenceID     int64
	fragments      map[uint64]*fragmentedPacket
	fragmentsMutex sync.Mutex

	// check and clean lost fragments
	checkCleanPending  bool
	checkCleanTimer    *time.Timer
	checkCleanTimeout  time.Duration // default is 5 seconds
	checkCleanDeadline time.Duration // how long we wait for the next fragment of the certain sequenceID. Default is 30 seconds
}

func (l *Link) GetPeerName() string {
	if l.peer == nil {
		return ""
	}

	return l.peer.Name
}

func newLinkFlusher(w io.Writer, latency time.Duration) *linkFlusher {
	return &linkFlusher{
		latency: latency,
		writer:  bufio.NewWriter(w),
		w:       w, // in case if we skip buffering
	}
}

type linkFlusher struct {
	mutex   sync.Mutex
	latency time.Duration
	writer  *bufio.Writer
	w       io.Writer

	timer   *time.Timer
	pending bool
}

func (lf *linkFlusher) Write(b []byte) (int, error) {
	lf.mutex.Lock()
	defer lf.mutex.Unlock()

	l := len(b)
	lenB := l

	// long data write directly to the socket.
	if l > 64000 {
		for {
			n, e := lf.w.Write(b[lenB-l:])
			if e != nil {
				return n, e
			}
			// check if something left
			l -= n
			if l > 0 {
				continue
			}
			return lenB, nil
		}
	}

	// write data to the buffer
	for {
		n, e := lf.writer.Write(b)
		if e != nil {
			return n, e
		}
		// check if something left
		l -= n
		if l > 0 {
			continue
		}
		break
	}

	if lf.pending {
		return lenB, nil
	}

	lf.pending = true

	if lf.timer != nil {
		lf.timer.Reset(lf.latency)
		return lenB, nil
	}

	lf.timer = time.AfterFunc(lf.latency, func() {
		// KeepAlive packet is just 4 bytes with zero value
		var keepAlivePacket = []byte{0, 0, 0, 0}

		lf.mutex.Lock()
		defer lf.mutex.Unlock()

		// if we have no pending data to send we should
		// send a KeepAlive packet
		if !lf.pending {
			lf.w.Write(keepAlivePacket)
			return
		}

		lf.writer.Flush()
		lf.pending = false
	})

	return lenB, nil

}

func Handshake(conn net.Conn, options HandshakeOptions) (*Link, error) {

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
		version:    uint16(options.Version),
		creation:   options.Creation,
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var await []byte

	if options.Version == ProtoHandshake5 {
		link.composeName(b, options.TLS)
		// the next message must be send_status 's' or send_challenge 'n' (for
		// handshake version 5) or 'N' (for handshake version 6)
		await = []byte{'s', 'n', 'N'}
	} else {
		link.composeNameVersion6(b, options.TLS)
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

func HandshakeAccept(conn net.Conn, options HandshakeOptions) (*Link, error) {
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

func (l *Link) Close() {
	if l.conn != nil {
		l.conn.Close()
	}
}

func (l *Link) PeerName() string {
	if l.peer != nil {
		return l.peer.Name
	}
	return ""
}

func (l *Link) Read(b *lib.Buffer) (int, error) {
	// http://erlang.org/doc/apps/erts/erl_dist_protocol.html#protocol-between-connected-nodes
	expectingBytes := 4

	for {
		if b.Len() < expectingBytes {
			n, e := b.ReadDataFrom(l.conn, 0)
			if n == 0 {
				// link was closed
				return 0, nil
			}

			if e != nil && e != io.EOF {
				// something went wrong
				return 0, e
			}

			// check onemore time if we should read more data
			continue
		}

		packetLength := binary.BigEndian.Uint32(b.B[:4])
		if packetLength == 0 {
			// keepalive
			l.conn.Write(b.B[:4])
			b.Set(b.B[4:])

			expectingBytes = 4
			continue
		}

		if b.Len() < int(packetLength)+4 {
			expectingBytes = int(packetLength) + 4
			continue
		}

		return int(packetLength) + 4, nil
	}

}

func (l *Link) ReadHandlePacket(ctx context.Context, recv chan *lib.Buffer,
	handler func(string, etf.Term, etf.Term) error) {
	var b *lib.Buffer
	var retry bool

	for {
		retry = false
		b = nil
		b = <-recv
		if b == nil {
			// channel was closed
			return
		}

	retryReadPacket:
		// read and decode received packet
		control, message, err := l.ReadPacket(b.B)

		//////////////////////////////////////////////////////////////////////
		// The main idea of sleeping here is that we have N goroutines
		// for the processing packets.
		// There is a case when we got two packets like MONITOR, REG_SEND
		// (which is pretty usual for the regular gen_server:call from the Erlang).
		// The first packet (MONITOR) has new atom cache entries, which
		// should use in the next packet (REG_SEND). But sometimes,
		// doing handle REG_SEND packet, the atom cache still
		// has no entries due to processing of the MONITOR packet
		// is doing on another goroutine and hasn't finished yet.
		//
		// Besides that, if we have the only packet in the 'recv' channle add some
		// delay before we take another attempt to handle this packet.
		// If this delay is not enought it seems we got disordered data. Drop
		// this connection.
		if err == ErrMissingInCache && retry == false {
			retry = true
			time.Sleep(150 * time.Millisecond)
			goto retryReadPacket
		}
		if err == ErrMissingInCache {
			fmt.Println("Disordered data at link with", l.PeerName())
			l.Close()
			lib.ReleaseBuffer(b)
			return
		}
		//////////////////////////////////////////////////////////////////////

		if err != nil {
			fmt.Println("Malformed Dist proto at link with", l.PeerName(), err)
			l.Close()
			lib.ReleaseBuffer(b)
			return
		}

		if control == nil {
			// fragment
			continue
		}

		// handle message
		if err := handler(l.peer.Name, control, message); err != nil {
			fmt.Printf("Malformed Control packet at link with %s: %#v\n", l.PeerName(), control)
			l.Close()
			lib.ReleaseBuffer(b)
			return
		}

		// we have to release this buffer
		lib.ReleaseBuffer(b)

	}
}

func (l *Link) ReadPacket(packet []byte) (etf.Term, etf.Term, error) {
	if len(packet) < 5 {
		return nil, nil, fmt.Errorf("malformed packet")
	}

	// [:3] length
	switch packet[4] {
	case protoDist:
		return l.ReadDist(packet[5:])
	default:
		// unknown proto
		return nil, nil, fmt.Errorf("unknown/unsupported proto")
	}

}

func (l *Link) ReadDist(packet []byte) (etf.Term, etf.Term, error) {
	switch packet[0] {
	case protoDistCompressed:
		// do we need it?
		// zip.NewReader(...)
		// ...unzipping to the new buffer b (lib.TakeBuffer)
		// just in case: if b[0] == protoDistCompressed return error
		// otherwise it will cause recursive call and im not sure if its ok
		// return l.ReadDist(b)

	case protoDistMessage:
		var control, message etf.Term
		var cache []etf.Atom
		var err error

		cache, packet, err = l.decodeDistHeaderAtomCache(packet[1:])

		if err != nil {
			return nil, nil, err
		}

		decodeOptions := etf.DecodeOptions{
			FlagV4NC:        l.peer.flags.isSet(V4_NC),
			FlagBigCreation: l.peer.flags.isSet(BIG_CREATION),
		}

		control, packet, err = etf.Decode(packet, cache, decodeOptions)
		if err != nil {
			return nil, nil, err
		}

		if len(packet) == 0 {
			return control, nil, nil
		}

		message, packet, err = etf.Decode(packet, cache, decodeOptions)
		if err != nil {
			return nil, nil, err
		}

		if len(packet) != 0 {
			return nil, nil, fmt.Errorf("packet has extra %d byte(s)", len(packet))
		}

		return control, message, nil

	case protoDistFragment1, protoDistFragmentN:
		first := packet[0] == protoDistFragment1
		if len(packet) < 18 {
			return nil, nil, fmt.Errorf("malformed fragment")
		}

		// We should decode first fragment in order to process Atom Cache Header
		// to get rid the case when we get the first fragment of the packet
		// and the next packet is not the part of the fragmented packet, but with
		// the ids were encoded in the first fragment
		if first {
			l.decodeDistHeaderAtomCache(packet[1:])
		}

		if assembled, err := l.decodeFragment(packet[1:], first); assembled != nil {
			if err != nil {
				return nil, nil, err
			}
			defer lib.ReleaseBuffer(assembled)
			return l.ReadDist(assembled.B)
		} else {
			if err != nil {
				return nil, nil, err
			}
		}

		return nil, nil, nil
	}

	return nil, nil, fmt.Errorf("unknown packet type %d", packet[0])
}

func (l *Link) decodeFragment(packet []byte, first bool) (*lib.Buffer, error) {
	l.fragmentsMutex.Lock()
	defer l.fragmentsMutex.Unlock()

	if l.fragments == nil {
		l.fragments = make(map[uint64]*fragmentedPacket)
	}

	sequenceID := binary.BigEndian.Uint64(packet)
	fragmentID := binary.BigEndian.Uint64(packet[8:])
	if fragmentID == 0 {
		return nil, fmt.Errorf("fragmentID can't be 0")
	}

	fragmented, ok := l.fragments[sequenceID]
	if !ok {
		fragmented = &fragmentedPacket{
			buffer:           lib.TakeBuffer(),
			disordered:       lib.TakeBuffer(),
			disorderedSlices: make(map[uint64][]byte),
			lastUpdate:       time.Now(),
		}
		fragmented.buffer.AppendByte(protoDistMessage)
		l.fragments[sequenceID] = fragmented
	}

	// until we get the first item everything will be treated as disordered
	if first {
		fragmented.fragmentID = fragmentID + 1
	}

	if fragmented.fragmentID-fragmentID != 1 {
		// got the next fragment. disordered
		slice := fragmented.disordered.Extend(len(packet) - 16)
		copy(slice, packet[16:])
		fragmented.disorderedSlices[fragmentID] = slice
	} else {
		// order is correct. just append
		fragmented.buffer.Append(packet[16:])
		fragmented.fragmentID = fragmentID
	}

	// check whether we have disordered slices and try
	// to append them if it does fit
	if fragmented.fragmentID > 0 && len(fragmented.disorderedSlices) > 0 {
		for i := fragmented.fragmentID - 1; i > 0; i-- {
			if slice, ok := fragmented.disorderedSlices[i]; ok {
				fragmented.buffer.Append(slice)
				delete(fragmented.disorderedSlices, i)
				fragmented.fragmentID = i
				continue
			}
			break
		}
	}

	fragmented.lastUpdate = time.Now()

	if fragmented.fragmentID == 1 && len(fragmented.disorderedSlices) == 0 {
		// it was the last fragment
		delete(l.fragments, sequenceID)
		lib.ReleaseBuffer(fragmented.disordered)
		return fragmented.buffer, nil
	}

	if l.checkCleanPending {
		return nil, nil
	}

	if l.checkCleanTimer != nil {
		l.checkCleanTimer.Reset(l.checkCleanTimeout)
		return nil, nil
	}

	l.checkCleanTimer = time.AfterFunc(l.checkCleanTimeout, func() {
		l.fragmentsMutex.Lock()
		defer l.fragmentsMutex.Unlock()

		if l.checkCleanTimeout == 0 {
			l.checkCleanTimeout = defaultCleanTimeout
		}
		if l.checkCleanDeadline == 0 {
			l.checkCleanDeadline = defaultCleanDeadline
		}

		valid := time.Now().Add(-l.checkCleanDeadline)
		for sequenceID, fragmented := range l.fragments {
			if fragmented.lastUpdate.Before(valid) {
				// dropping  due to excided deadline
				delete(l.fragments, sequenceID)
			}
		}
		if len(l.fragments) == 0 {
			l.checkCleanPending = false
			return
		}

		l.checkCleanPending = true
		l.checkCleanTimer.Reset(l.checkCleanTimeout)
	})

	return nil, nil
}

func (l *Link) decodeDistHeaderAtomCache(packet []byte) ([]etf.Atom, []byte, error) {
	// all the details are here https://erlang.org/doc/apps/erts/erl_ext_dist.html#normal-distribution-header

	// number of atom references are present in package
	references := int(packet[0])
	if references == 0 {
		return nil, packet[1:], nil
	}

	cache := make([]etf.Atom, references)
	flagsLen := references/2 + 1
	if len(packet) < 1+flagsLen {
		// malformed
		return nil, nil, ErrMalformed
	}
	flags := packet[1 : flagsLen+1]

	// The least significant bit in a half byte is flag LongAtoms.
	// If it is set, 2 bytes are used for atom lengths instead of 1 byte
	// in the distribution header.
	headerAtomLength := 1 // if 'LongAtom' is not set

	// extract this bit. just increase headereAtomLength if this flag is set
	lastByte := flags[len(flags)-1]
	shift := uint((references & 0x01) * 4)
	headerAtomLength += int((lastByte >> shift) & 0x01)

	// 1 (number of references) + references/2+1 (length of flags)
	packet = packet[1+flagsLen:]

	for i := 0; i < references; i++ {
		if len(packet) < 1+headerAtomLength {
			// malformed
			return nil, nil, ErrMalformed
		}
		shift = uint((i & 0x01) * 4)
		flag := (flags[i/2] >> shift) & 0x0F
		isNewReference := flag&0x08 == 0x08
		idxReference := uint16(flag & 0x07)
		idxInternal := uint16(packet[0])
		idx := (idxReference << 8) | idxInternal

		if isNewReference {
			atomLen := uint16(packet[1])
			if headerAtomLength == 2 {
				atomLen = binary.BigEndian.Uint16(packet[1:3])
			}
			// extract atom
			packet = packet[1+headerAtomLength:]
			if len(packet) < int(atomLen) {
				// malformed
				return nil, nil, ErrMalformed
			}
			atom := etf.Atom(packet[:atomLen])
			// store in temporary cache for decoding
			cache[i] = atom

			// store in link' cache
			l.cacheInMutex.Lock()
			l.cacheIn[idx] = &atom
			l.cacheInMutex.Unlock()
			packet = packet[atomLen:]
			continue
		}

		l.cacheInMutex.Lock()
		c := l.cacheIn[idx]
		l.cacheInMutex.Unlock()
		if c == nil {
			return cache, packet, ErrMissingInCache
		}
		cache[i] = *c
		packet = packet[1:]
	}

	return cache, packet, nil
}

func (l *Link) SetAtomCache(cache *etf.AtomCache) {
	l.cacheOut = cache
}

func (l *Link) encodeDistHeaderAtomCache(b *lib.Buffer,
	writerAtomCache map[etf.Atom]etf.CacheItem,
	encodingAtomCache *etf.ListAtomCache) {

	n := encodingAtomCache.Len()
	if n == 0 {
		b.AppendByte(0)
		return
	}

	b.AppendByte(byte(n)) // write NumberOfAtomCache

	lenFlags := n/2 + 1
	b.Extend(lenFlags)

	flags := b.B[1 : lenFlags+1]
	flags[lenFlags-1] = 0 // clear last byte to make sure we have valid LongAtom flag

	for i := 0; i < len(encodingAtomCache.L); i++ {
		shift := uint((i & 0x01) * 4)
		idxReference := byte(encodingAtomCache.L[i].ID >> 8) // SegmentIndex
		idxInternal := byte(encodingAtomCache.L[i].ID & 255) // InternalSegmentIndex

		cachedItem := writerAtomCache[encodingAtomCache.L[i].Name]
		if !cachedItem.Encoded {
			idxReference |= 8 // set NewCacheEntryFlag
		}

		// we have to clear before reuse
		if shift == 0 {
			flags[i/2] = 0
		}
		flags[i/2] |= idxReference << shift

		if cachedItem.Encoded {
			b.AppendByte(idxInternal)
			continue
		}

		if encodingAtomCache.HasLongAtom {
			// 1 (InternalSegmentIndex) + 2 (length) + name
			allocLen := 1 + 2 + len(encodingAtomCache.L[i].Name)
			buf := b.Extend(allocLen)
			buf[0] = idxInternal
			binary.BigEndian.PutUint16(buf[1:3], uint16(len(encodingAtomCache.L[i].Name)))
			copy(buf[3:], encodingAtomCache.L[i].Name)
		} else {

			// 1 (InternalSegmentIndex) + 1 (length) + name
			allocLen := 1 + 1 + len(encodingAtomCache.L[i].Name)
			buf := b.Extend(allocLen)
			buf[0] = idxInternal
			buf[1] = byte(len(encodingAtomCache.L[i].Name))
			copy(buf[2:], encodingAtomCache.L[i].Name)
		}

		cachedItem.Encoded = true
		writerAtomCache[encodingAtomCache.L[i].Name] = cachedItem
	}

	if encodingAtomCache.HasLongAtom {
		shift := uint((n & 0x01) * 4)
		flags[lenFlags-1] |= 1 << shift // set LongAtom = 1
	}
}

func (l *Link) Writer(send <-chan []etf.Term, fragmentationUnit int) {
	var terms []etf.Term

	var encodingAtomCache *etf.ListAtomCache
	var writerAtomCache map[etf.Atom]etf.CacheItem
	var linkAtomCache *etf.AtomCache
	var lastCacheID int16 = -1

	var lenControl, lenMessage, lenAtomCache, lenPacket, startDataPosition int
	var atomCacheBuffer, packetBuffer *lib.Buffer
	var err error

	cacheEnabled := l.peer.flags.isSet(DIST_HDR_ATOM_CACHE) && l.cacheOut != nil
	fragmentationEnabled := l.peer.flags.isSet(FRAGMENTS) && fragmentationUnit > 0

	// Header atom cache is encoded right after the control/message encoding process
	// but should be stored as a first item in the packet.
	// Thats why we do reserve some space for it in order to get rid
	// of reallocation packetBuffer data
	reserveHeaderAtomCache := 8192

	if cacheEnabled {
		encodingAtomCache = etf.TakeListAtomCache()
		defer etf.ReleaseListAtomCache(encodingAtomCache)
		writerAtomCache = make(map[etf.Atom]etf.CacheItem)
		linkAtomCache = l.cacheOut
	}

	encodeOptions := etf.EncodeOptions{
		LinkAtomCache:     linkAtomCache,
		WriterAtomCache:   writerAtomCache,
		EncodingAtomCache: encodingAtomCache,
		FlagBigCreation:   l.peer.flags.isSet(BIG_CREATION),
		FlagV4NC:          l.peer.flags.isSet(V4_NC),
	}

	for {
		terms = nil
		terms = <-send

		if terms == nil {
			// channel was closed
			return
		}

		packetBuffer = lib.TakeBuffer()
		lenControl, lenMessage, lenAtomCache, lenPacket, startDataPosition = 0, 0, 0, 0, reserveHeaderAtomCache

		// do reserve for the header 8K, should be enough
		packetBuffer.Allocate(reserveHeaderAtomCache)

		// clear encoding cache
		if cacheEnabled {
			encodingAtomCache.Reset()
		}

		// encode Control
		err = etf.Encode(terms[0], packetBuffer, encodeOptions)
		if err != nil {
			fmt.Println(err)
			lib.ReleaseBuffer(packetBuffer)
			continue
		}
		lenControl = packetBuffer.Len() - reserveHeaderAtomCache

		// encode Message if present
		if len(terms) == 2 {
			err = etf.Encode(terms[1], packetBuffer, encodeOptions)
			if err != nil {
				fmt.Println(err)
				lib.ReleaseBuffer(packetBuffer)
				continue
			}

		}
		lenMessage = packetBuffer.Len() - reserveHeaderAtomCache - lenControl

		// encode Header Atom Cache if its enabled
		if cacheEnabled && encodingAtomCache.Len() > 0 {
			atomCacheBuffer = lib.TakeBuffer()
			l.encodeDistHeaderAtomCache(atomCacheBuffer, writerAtomCache, encodingAtomCache)
			lenAtomCache = atomCacheBuffer.Len()

			if lenAtomCache > reserveHeaderAtomCache-22 {
				// are you serious? ))) what da hell you just sent?
				// FIXME i'm gonna fix it if someone report about this issue :)
				panic("exceed atom header cache size limit. please report about this issue")
			}

			startDataPosition -= lenAtomCache
			copy(packetBuffer.B[startDataPosition:], atomCacheBuffer.B)
			lib.ReleaseBuffer(atomCacheBuffer)

		} else {
			lenAtomCache = 1
			startDataPosition -= lenAtomCache
			packetBuffer.B[startDataPosition] = byte(0)
		}

		for {

			// 4 (packet len) + 1 (dist header: 131) + 1 (dist header: protoDistMessage) + lenAtomCache
			lenPacket = 1 + 1 + lenAtomCache + lenControl + lenMessage

			if !fragmentationEnabled || lenPacket < fragmentationUnit {
				// send as a single packet
				startDataPosition -= 6

				binary.BigEndian.PutUint32(packetBuffer.B[startDataPosition:], uint32(lenPacket))
				packetBuffer.B[startDataPosition+4] = protoDist        // 131
				packetBuffer.B[startDataPosition+5] = protoDistMessage // 68
				if _, err := l.flusher.Write(packetBuffer.B[startDataPosition:]); err != nil {
					return
				}
				break
			}

			// Message should be fragmented

			// https://erlang.org/doc/apps/erts/erl_ext_dist.html#distribution-header-for-fragmented-messages
			// "The entire atom cache and control message has to be part of the starting fragment"

			sequenceID := uint64(atomic.AddInt64(&l.sequenceID, 1))
			numFragments := lenMessage/fragmentationUnit + 1

			// 1 (dist header: 131) + 1 (dist header: protoDistFragment) + 8 (sequenceID) + 8 (fragmentID) + ...
			lenPacket = 1 + 1 + 8 + 8 + lenAtomCache + lenControl + fragmentationUnit

			// 4 (packet len) + 1 (dist header: 131) + 1 (dist header: protoDistFragment) + 8 (sequenceID) + 8 (fragmentID)
			startDataPosition -= 22

			binary.BigEndian.PutUint32(packetBuffer.B[startDataPosition:], uint32(lenPacket))
			packetBuffer.B[startDataPosition+4] = protoDist          // 131
			packetBuffer.B[startDataPosition+5] = protoDistFragment1 // 69

			binary.BigEndian.PutUint64(packetBuffer.B[startDataPosition+6:], uint64(sequenceID))
			binary.BigEndian.PutUint64(packetBuffer.B[startDataPosition+14:], uint64(numFragments))
			if _, err := l.flusher.Write(packetBuffer.B[startDataPosition : startDataPosition+4+lenPacket]); err != nil {
				return
			}

			startDataPosition += 4 + lenPacket
			numFragments--

		nextFragment:

			if len(packetBuffer.B[startDataPosition:]) > fragmentationUnit {
				lenPacket = 1 + 1 + 8 + 8 + fragmentationUnit
				// reuse the previous 22 bytes for the next frame header
				startDataPosition -= 22

			} else {
				// the last one
				lenPacket = 1 + 1 + 8 + 8 + len(packetBuffer.B[startDataPosition:])
				startDataPosition -= 22
			}

			binary.BigEndian.PutUint32(packetBuffer.B[startDataPosition:], uint32(lenPacket))
			packetBuffer.B[startDataPosition+4] = protoDist          // 131
			packetBuffer.B[startDataPosition+5] = protoDistFragmentN // 70

			binary.BigEndian.PutUint64(packetBuffer.B[startDataPosition+6:], uint64(sequenceID))
			binary.BigEndian.PutUint64(packetBuffer.B[startDataPosition+14:], uint64(numFragments))

			if _, err := l.flusher.Write(packetBuffer.B[startDataPosition : startDataPosition+4+lenPacket]); err != nil {
				return
			}

			startDataPosition += 4 + lenPacket
			numFragments--
			if numFragments > 0 {
				goto nextFragment
			}

			// done
			break
		}

		lib.ReleaseBuffer(packetBuffer)

		if !cacheEnabled {
			continue
		}

		// get updates from link AtomCache and update the local one (map writerAtomCache)
		id := linkAtomCache.GetLastID()
		if lastCacheID < id {
			linkAtomCache.Lock()
			for _, a := range linkAtomCache.ListSince(lastCacheID + 1) {
				writerAtomCache[a] = etf.CacheItem{ID: lastCacheID + 1, Name: a, Encoded: false}
				lastCacheID++
			}
			linkAtomCache.Unlock()
		}

	}

}

func (l *Link) GetRemoteName() string {
	return l.peer.Name
}

func (l *Link) composeName(b *lib.Buffer, tls bool) {
	if tls {
		b.Allocate(11)
		dataLength := 7 + len(l.Name) // byte + uint16 + uint32 + len(l.Name)
		binary.BigEndian.PutUint32(b.B[0:4], uint32(dataLength))
		b.B[4] = 'n'
		binary.BigEndian.PutUint16(b.B[5:7], l.version)           // uint16
		binary.BigEndian.PutUint32(b.B[7:11], l.flags.toUint32()) // uint32
		b.Append([]byte(l.Name))
		return
	}

	b.Allocate(9)
	dataLength := 7 + len(l.Name) // byte + uint16 + uint32 + len(l.Name)
	binary.BigEndian.PutUint16(b.B[0:2], uint16(dataLength))
	b.B[2] = 'n'
	binary.BigEndian.PutUint16(b.B[3:5], l.version)          // uint16
	binary.BigEndian.PutUint32(b.B[5:9], l.flags.toUint32()) // uint32
	b.Append([]byte(l.Name))
}

func (l *Link) composeNameVersion6(b *lib.Buffer, tls bool) {
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

func (l *Link) readName(b []byte) *Link {
	peer := &Link{
		Name:    string(b[6:]),
		version: binary.BigEndian.Uint16(b[0:2]),
		flags:   nodeFlag(binary.BigEndian.Uint32(b[2:6])),
	}
	return peer
}

func (l *Link) readNameVersion6(b []byte) *Link {
	nameLen := int(binary.BigEndian.Uint16(b[12:14]))
	peer := &Link{
		flags:    nodeFlag(binary.BigEndian.Uint64(b[0:8])),
		creation: binary.BigEndian.Uint32(b[8:12]),
		Name:     string(b[14 : 14+nameLen]),
		version:  ProtoHandshake6,
	}
	return peer
}

func (l *Link) composeStatus(b *lib.Buffer, tls bool) {
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

func (l *Link) readStatus(msg []byte) bool {
	if string(msg[:2]) == "ok" {
		return true
	}

	return false
}

func (l *Link) composeChallenge(b *lib.Buffer, tls bool) {
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

func (l *Link) composeChallengeVersion6(b *lib.Buffer, tls bool) {
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

func (l *Link) readChallenge(msg []byte) (challenge uint32) {
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

func (l *Link) readChallengeVersion6(msg []byte) (challenge uint32) {
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

func (l *Link) readComplement(msg []byte) {
	flags := uint64(binary.BigEndian.Uint32(msg[0:4])) << 32
	l.peer.flags = nodeFlag(l.peer.flags.toUint64() | flags)
	l.peer.creation = binary.BigEndian.Uint32(msg[4:8])
	return
}

func (l *Link) validateChallengeReply(b []byte) bool {
	l.peer.challenge = binary.BigEndian.Uint32(b[:4])
	digestB := b[4:]

	digestA := genDigest(l.challenge, l.Cookie)
	return bytes.Equal(digestA[:], digestB)
}

func (l *Link) composeChallengeAck(b *lib.Buffer, tls bool) {
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

func (l *Link) composeChallengeReply(b *lib.Buffer, challenge uint32, tls bool) {
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

func (l *Link) composeComplement(b *lib.Buffer, tls bool) {
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
