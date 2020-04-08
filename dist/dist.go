package dist

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
)

var dTrace bool

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
	flag.BoolVar(&dTrace, "trace.dist", false, "trace erlang distribution protocol")
}

func dLog(f string, a ...interface{}) {
	if dTrace {
		log.Printf("d# "+f, a...)
	}
}

type flagId uint32
type nodeFlag flagId

const (
	// http://erlang.org/doc/apps/erts/erl_ext_dist.html#distribution_header
	protoDist           = 131
	protoDistCompressed = 80
	protoDistMessage    = 68
	protoDistFragment1  = 69
	protoDistFragmentN  = 70

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
)

func (nf nodeFlag) toUint32() uint32 {
	return uint32(nf)
}

func (nf nodeFlag) isSet(f flagId) bool {
	return (uint32(nf) & uint32(f)) != 0
}

func toNodeFlag(f ...flagId) nodeFlag {
	var flags uint32
	for _, v := range f {
		flags |= uint32(v)
	}
	return nodeFlag(flags)
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

	// atom cache for incomming messages
	cacheIn [2048]etf.Atom

	// atom cache for outgoing messages
	cacheOut *etf.AtomCache

	// fragmentation sequence ID
	sequenceID int64
}

func Handshake(conn net.Conn, name, cookie string, hidden bool) (*Link, error) {

	link := &Link{
		Name:   name,
		Cookie: cookie,
		Hidden: hidden,

		flags: toNodeFlag(PUBLISHED, UNICODE_IO, DIST_MONITOR, DIST_MONITOR_NAME,
			EXTENDED_PIDS_PORTS, EXTENDED_REFERENCES, ATOM_CACHE,
			DIST_HDR_ATOM_CACHE, HIDDEN_ATOM_CACHE, NEW_FUN_TAGS,
			SMALL_ATOM_TAGS, UTF8_ATOMS, MAP_TAG, BIG_CREATION,
			FRAGMENTS,
		),

		conn:       conn,
		sequenceID: time.Now().UnixNano(),
		version:    5,
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	link.composeName(b)
	if e := b.WriteDataTo(conn); e != nil {
		return nil, e
	}
	b.Reset()

	// define timeout for the handshaking
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	asyncReadChannel := make(chan error, 2)
	asyncRead := func() {
		//  If the buffer becomes too large, ReadDataFrom will panic with ErrTooLarge.
		defer func() {
			if r := recover(); r != nil {
				asyncReadChannel <- fmt.Errorf("malformed handshake (too large packet)")
			}
		}()
		_, e := b.ReadDataFrom(conn)
		asyncReadChannel <- e
	}
	for {
		go asyncRead()

		select {
		case <-timer.C:
			return nil, fmt.Errorf("timeout")

		case e := <-asyncReadChannel:
			if e != nil {
				return nil, e
			}
			switch b.B[0] {
			case 'n':
				// 'n' + 2 (version) + 4 (flags) + 4 (challenge) + name...
				if len(b.B) < 12 {
					return nil, fmt.Errorf("malformed handshake ('n')")
				}

				challenge := link.readChallenge(b.B)
				b.Reset()
				link.composeChallengeReply(challenge, b)
				if e := b.WriteDataTo(conn); e != nil {
					return nil, e
				}
				b.Reset()
				continue
			case 'a':
				// 'a' + 16 (digest)
				if len(b.B) != 17 {
					return nil, fmt.Errorf("malformed handshake ('a' length of digest)")
				}

				if !link.validateChallengeAck(b) {
					return nil, fmt.Errorf("malformed handshake ('a' digest)")
				}

				// handshaked
				return link, nil
			}

		}

	}

}

func HandshakeAccept(conn net.Conn, name, cookie string, hidden bool) (*Link, error) {
	link := &Link{
		Name:   name,
		Cookie: cookie,
		Hidden: hidden,

		flags: toNodeFlag(PUBLISHED, UNICODE_IO, DIST_MONITOR, DIST_MONITOR_NAME,
			EXTENDED_PIDS_PORTS, EXTENDED_REFERENCES, ATOM_CACHE,
			DIST_HDR_ATOM_CACHE, HIDDEN_ATOM_CACHE, NEW_FUN_TAGS,
			SMALL_ATOM_TAGS, UTF8_ATOMS, MAP_TAG, BIG_CREATION,
			FRAGMENTS,
		),

		conn:       conn,
		sequenceID: time.Now().UnixNano(),
		challenge:  rand.Uint32(),
		version:    5,
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	// define timeout for the handshaking
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	asyncReadChannel := make(chan error, 2)
	asyncRead := func() {
		//  If the buffer becomes too large, ReadDataFrom will panic with ErrTooLarge.
		defer func() {
			if r := recover(); r != nil {
				asyncReadChannel <- fmt.Errorf("malformed handshake (too large packet)")
			}
		}()
		_, e := b.ReadDataFrom(conn)
		asyncReadChannel <- e
	}
	for {
		go asyncRead()

		select {
		case <-timer.C:
			return nil, fmt.Errorf("timeout")
		case e := <-asyncReadChannel:
			if e != nil {
				return nil, e
			}

			switch b.B[2] {
			case 'n':
				if len(b.B) < 9 {
					return nil, fmt.Errorf("malformed handshake ('n' length)")
				}

				link.peer = link.readName(b.B[3:])
				b.Reset()
				link.composeStatus(b)
				if e := b.WriteDataTo(conn); e != nil {
					return nil, fmt.Errorf("malformed handshake ('n' accept name)")
				}

				b.Reset()
				link.composeChallenge(b)
				if e := b.WriteDataTo(conn); e != nil {
					return nil, e
				}
				b.Reset()
				continue
			case 'r':
				if len(b.B) < 21 {
					return nil, fmt.Errorf("malformed handshake ('r')")
				}

				if !link.validateChallengeReply(b.B[3:]) {
					return nil, fmt.Errorf("malformed handshake ('r1')")
				}
				b.Reset()

				link.composeChallengeAck(b)
				if e := b.WriteDataTo(conn); e != nil {
					return nil, e
				}
				b.Reset()
				// handshaked
				return link, nil

			default:
				return nil, fmt.Errorf("malformed handshake (unknown code %d)", b.B[2])
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
			n, e := b.ReadDataFrom(l.conn)
			if n == 0 {
				// link is closed
				return 0, nil
			}

			if e != nil && e != io.EOF {
				return 0, e
			}

		}

		packetLength := binary.BigEndian.Uint32(b.B[:4])
		if packetLength == 0 {
			// this is keepalive
			l.conn.Write(b.B[:4])
			b.Set(b.B[4:])

			// check the tail if it has enough data to go further
			if b.Len() < 4 {
				continue
			}
			packetLength = binary.BigEndian.Uint32(b.B[:4])
		}

		if b.Len() < int(packetLength)+4 {
			expectingBytes = int(packetLength) + 4
			continue
		}

		return int(packetLength) + 4, nil
	}

}

func (l *Link) ReadHandlePacket(ctx context.Context, recv <-chan *lib.Buffer,
	handler func(etf.Term, etf.Term)) {
	for {
		select {
		case b := <-recv:
			// read and decode recieved packet
			control, message, err := l.ReadPacket(b.B)
			if err != nil {
				fmt.Println("Malformed Dist proto at link with", l.PeerName(), err)
				l.Close()
				return
			}

			if control == nil {
				// fragment
				continue
			}

			// handle message
			handler(control, message)

			// we have to release this buffer
			lib.ReleaseBuffer(b)

		case <-ctx.Done():
			return

		}
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

		cache, packet = l.decodeDistHeaderAtomCache(packet[1:])
		if packet == nil {
			return nil, nil, fmt.Errorf("incorrect dist header atom cache")
		}
		control, packet, err = etf.Decode(packet, cache)
		if err != nil {
			return nil, nil, err
		}

		if len(packet) == 0 {
			return control, nil, nil
		}

		message, packet, err = etf.Decode(packet, cache)
		if err != nil {
			return nil, nil, err
		}

		if len(packet) != 0 {
			return nil, nil, fmt.Errorf("packet has extra %d byte(s)", len(packet))
		}

		return control, message, nil

	case protoDistFragment1:

	case protoDistFragmentN:

	}

	return nil, nil, fmt.Errorf("unknown packet type %d", packet[0])
}

func (l *Link) decodeDistHeaderAtomCache(packet []byte) ([]etf.Atom, []byte) {
	// all the details are here https://erlang.org/doc/apps/erts/erl_ext_dist.html#normal-distribution-header

	// number of atom references are present in package
	references := int(packet[0])
	if references == 0 {
		return nil, packet[1:]
	}

	cache := make([]etf.Atom, references)
	flagsLen := references/2 + 1
	if len(packet) < 1+flagsLen {
		// malformed
		return nil, nil
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
			return nil, nil
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
				return nil, nil
			}
			atom := string(packet[:atomLen])
			// store in temporary cache for encoding
			cache[i] = etf.Atom(atom)

			// store in link' cache
			l.cacheIn[idx] = etf.Atom(atom)
			packet = packet[atomLen:]
			continue
		}

		cache[i] = l.cacheIn[idx]
		packet = packet[1:]
	}

	return cache, packet
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

func (l *Link) Writer(ctx context.Context, send <-chan []etf.Term, fragmentationUnit int) {
	var terms []etf.Term

	var encodingAtomCache *etf.ListAtomCache
	var writerAtomCache map[etf.Atom]etf.CacheItem
	var linkAtomCache *etf.AtomCache
	var lastCacheID int16 = -1

	var lenControl, lenMessage, lenAtomCache, lenPacket, startDataPosition int
	var atomCacheBuffer, packetBuffer *lib.Buffer
	var err error

	cacheEnabled := l.peer.flags.isSet(DIST_HDR_ATOM_CACHE) && l.cacheOut != nil
	fragmentationEnabled := l.peer.flags.isSet(FRAGMENTS)

	// Header atom cache is encoded right after control/message encoding
	// but stored before.
	// Thats why we do reserve some space for it in order to get rid
	// of reallocation packetBuffer data
	reserveHeaderAtomCache := 8192

	if cacheEnabled {
		encodingAtomCache = etf.TakeListAtomCache()
		defer etf.ReleaseListAtomCache(encodingAtomCache)
		writerAtomCache = make(map[etf.Atom]etf.CacheItem)
		linkAtomCache = l.cacheOut
	}

	for {

		select {
		case terms = <-send:
		case <-ctx.Done():
			return
		}

		packetBuffer = lib.TakeBuffer()
		lenControl, lenMessage, lenAtomCache, lenPacket, startDataPosition = 0, 0, 0, 0, 0

		// do reserve for the header 8K, should be enough
		packetBuffer.Allocate(reserveHeaderAtomCache)

		// clear encoding cache
		if cacheEnabled {
			encodingAtomCache.Reset()
		}

		// encode Control
		err = etf.Encode(terms[0], packetBuffer, linkAtomCache, writerAtomCache, encodingAtomCache)
		if err != nil {
			fmt.Println(err)
			lib.ReleaseBuffer(packetBuffer)
			continue
		}
		lenControl = packetBuffer.Len() - reserveHeaderAtomCache

		// encode Message if present
		if len(terms) == 2 {
			err = etf.Encode(terms[1], packetBuffer, linkAtomCache, writerAtomCache, encodingAtomCache)
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

			// 4 (packet len) + 1 (dist header: 131) + 1 (dist header: protoDistX) + lenAtomCache
			// where protoDistX is protoDist[Message|Fragment1|FragmentN]
			startDataPosition = reserveHeaderAtomCache - 4 + 1 + 1 + lenAtomCache
			packetBuffer.B = packetBuffer.B[startDataPosition:]
			packetBuffer.B[4] = protoDist // 131
			copy(packetBuffer.B[6:], atomCacheBuffer.B)

			lib.ReleaseBuffer(atomCacheBuffer)
		} else {
			// 4 (packet len) + 1 (dist header: 131) + 1 (dist header: protoDistX) + 1 (byte(0) - empty cache)
			// where protoDistX is protoDist[Message|Fragment1|FragmentN]
			lenAtomCache = 1
			startDataPosition = reserveHeaderAtomCache - 7
			packetBuffer.B = packetBuffer.B[startDataPosition:]
			packetBuffer.B[4] = protoDist // 131
			packetBuffer.B[6] = byte(0)

		}

		// 1 (dist header: 131) + 1 (protoDistX) + ...
		lenPacket = 1 + 1 + lenAtomCache + lenControl + lenMessage

		for {

			if !fragmentationEnabled || lenPacket < fragmentationUnit {
				// send as a single packet
				binary.BigEndian.PutUint32(packetBuffer.B[:4], uint32(lenPacket))
				packetBuffer.B[5] = protoDistMessage // 68
				packetBuffer.WriteDataTo(l.conn)
				break

			}

			fmt.Println("FIXME Write Data1", packetBuffer.B)
			packetBuffer.WriteDataTo(l.conn)
			// https://erlang.org/doc/apps/erts/erl_ext_dist.html#distribution-header-for-fragmented-messages
			// "The entire atom cache and control message has to be part of the starting fragment"

			// fragment numbering should be like
			// sequenceID = atomic.AddInt64(&l.sequenceID, 1)
			//
			break
		}

		lib.ReleaseBuffer(packetBuffer)

		if !cacheEnabled {
			continue
		}

		// get updates from link AtomCache and update the local one (map writerAtomCache)
		id := linkAtomCache.GetLastID()
		if lastCacheID < id {
			for _, a := range linkAtomCache.ListSince(lastCacheID + 1) {
				writerAtomCache[a] = etf.CacheItem{ID: lastCacheID + 1, Name: a, Encoded: false}
				lastCacheID++
			}
		}

	}

}

func (l *Link) GetRemoteName() string {
	return l.peer.Name
}

func (l *Link) composeName(b *lib.Buffer) {
	dataLength := uint16(7 + len(l.Name)) // byte + uint16 + uint32 + len(l.Name)
	b.Allocate(9)
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.B[2] = 'n'
	binary.BigEndian.PutUint16(b.B[3:5], l.version)          // uint16
	binary.BigEndian.PutUint32(b.B[5:9], l.flags.toUint32()) // uint32
	b.Append([]byte(l.Name))
}

func (l *Link) readName(b []byte) *Link {
	peer := &Link{
		Name:    fmt.Sprintf("%s", b[6:]),
		version: binary.BigEndian.Uint16(b[0:2]),
		flags:   nodeFlag(binary.BigEndian.Uint32(b[2:6])),
	}
	return peer
}

func (l *Link) composeStatus(b *lib.Buffer) {
	//FIXME: there are few options for the status:
	// 	   ok, ok_simultaneous, nok, not_allowed, alive
	// More details here: https://erlang.org/doc/apps/erts/erl_dist_protocol.html#the-handshake-in-detail
	b.Allocate(2)
	dataLength := uint16(3) // 's' + "ok"
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.Append([]byte("sok"))
}

func (l *Link) composeChallenge(b *lib.Buffer) {
	b.Allocate(13)
	dataLength := uint16(11 + len(l.Name))
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.B[2] = 'n'
	binary.BigEndian.PutUint16(b.B[3:5], l.version)          // uint16
	binary.BigEndian.PutUint32(b.B[5:9], l.flags.toUint32()) // uint32
	binary.BigEndian.PutUint32(b.B[9:13], l.challenge)       // uint32
	b.Append([]byte(l.Name))
}

func (l *Link) readChallenge(msg []byte) (challenge uint32) {
	link := &Link{
		Name:    fmt.Sprintf("%s", msg[11:]),
		version: binary.BigEndian.Uint16(msg[1:3]),
		flags:   nodeFlag(binary.BigEndian.Uint32(msg[3:7])),
	}
	l.peer = link
	return binary.BigEndian.Uint32(msg[7:11])
}

func (l *Link) validateChallengeReply(b []byte) bool {
	l.peer.challenge = binary.BigEndian.Uint32(b[:4])
	digestB := b[4:]

	digestA := genDigest(l.challenge, l.Cookie)
	return bytes.Equal(digestA[:], digestB)
}

func (l *Link) composeChallengeAck(b *lib.Buffer) {

	b.Allocate(3)
	dataLength := uint16(17) // 'a' + 16 (digest)
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.B[2] = 'a'
	digest := genDigest(l.peer.challenge, l.Cookie)
	b.Append(digest[:])
}

func (l *Link) composeChallengeReply(challenge uint32, b *lib.Buffer) {
	digest := genDigest(challenge, l.Cookie)
	dataLength := uint16(21) // 1 (byte) + 4 (challenge) + 16 (digest)
	b.Allocate(7)
	binary.BigEndian.PutUint16(b.B[0:2], dataLength)
	b.B[2] = 'r'
	binary.BigEndian.PutUint32(b.B[3:7], l.challenge) // uint32
	b.Append(digest[:])
}

func (l *Link) validateChallengeAck(b *lib.Buffer) bool {
	//digest := msg[1:]
	//FIXME
	return true
}

func genDigest(challenge uint32, cookie string) [16]byte {
	s := fmt.Sprintf("%s%d", cookie, challenge)
	return md5.Sum([]byte(s))
}
