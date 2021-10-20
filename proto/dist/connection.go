package dist

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

var (
	ErrMissingInCache = fmt.Errorf("Missing in cache")
	ErrMalformed      = fmt.Errorf("Malformed")
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

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
)

type fragmentedPacket struct {
	buffer           *lib.Buffer
	disordered       *lib.Buffer
	disorderedSlices map[uint64][]byte
	fragmentID       uint64
	lastUpdate       time.Time
}

type DistConnection struct {
	node.Connection

	options DistOptions

	// writer
	flusher *linkFlusher

	// atom cache for incoming messages
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

type DistOptions struct {
	SendQueueLength        int
	RecvQueueLength        int
	FragmentationUnit      int
	DisableHeaderAtomCache bool
}

func CreateDistConnection(ctx context.Context, name string, conn net.Conn, options DistOptions) *DistConnection {
	return &DistConnection{
		Name:    name,
		Conn:    conn,
		Context: ctx,
		options: options,
	}
}

//
// net.Connection interface implementation
//

func (dc *DistConnection) Serve(router node.Router) {
	// define the total number of reader/writer goroutines
	numHandlers := runtime.GOMAXPROCS(n.opts.ConnectionHandlers)

	// do not use shared channels within intencive code parts, impacts on a performance
	receivers := struct {
		recv []chan *lib.Buffer
		n    int
		i    int
	}{
		recv: make([]chan *lib.Buffer, n.opts.RecvQueueLength),
		n:    numHandlers,
	}

	// run readers for incoming messages
	for i := 0; i < numHandlers; i++ {
		// run packet reader/handler routines (decoder)
		recv := make(chan *lib.Buffer, dc.options.RecvQueueLength)
		receivers.recv[i] = recv
		go connection.ReadHandlePacket(ctx, recv, n.handleMessage)
	}

	cacheIsReady := make(chan bool)

	// run link reader routine
	go func() {
		var err error
		var packetLength int
		var recv chan *lib.Buffer

		linkctx, cancel := context.WithCancel(ctx)
		defer cancel()

		go func() {
			select {
			case <-linkctx.Done():
				// if node's context is done
				link.Close()
			}
		}()

		// initializing atom cache if its enabled
		if !n.opts.DisableHeaderAtomCache {
			link.SetAtomCache(etf.NewAtomCache(linkctx))
		}
		cacheIsReady <- true

		defer func() {
			link.Close()
			n.registrar.unregisterPeer(link.GetRemoteName())

			// close handlers channel
			p.mutex.Lock()
			for i := 0; i < numHandlers; i++ {
				if p.send[i] != nil {
					close(p.send[i])
				}
				if receivers.recv[i] != nil {
					close(receivers.recv[i])
				}
			}
			p.mutex.Unlock()
		}()

		b := lib.TakeBuffer()
		for {
			packetLength, err = link.Read(b)
			if err != nil || packetLength == 0 {
				// link was closed or got malformed data
				if err != nil {
					fmt.Println("link was closed", link.GetPeerName(), "error:", err)
				}
				lib.ReleaseBuffer(b)
				return
			}

			// take new buffer for the next reading and append the tail (part of the next packet)
			b1 := lib.TakeBuffer()
			b1.Set(b.B[packetLength:])
			// cut the tail and send it further for handling.
			// buffer b has to be released by the reader of
			// recv channel (link.ReadHandlePacket)
			b.B = b.B[:packetLength]
			recv = receivers.recv[receivers.i]

			recv <- b

			// set new buffer as a current for the next reading
			b = b1

			// round-robin switch to the next receiver
			receivers.i++
			if receivers.i < receivers.n {
				continue
			}
			receivers.i = 0

		}
	}()

	// we should make sure if the cache is ready before we start writers
	<-cacheIsReady

	// run readers/writers for incoming/outgoing messages
	for i := 0; i < numHandlers; i++ {
		// run writer routines (encoder)
		send := make(chan []etf.Term, n.opts.SendQueueLength)
		p.mutex.Lock()
		p.send[i] = send
		p.mutex.Unlock()
		go link.Writer(send, n.opts.FragmentationUnit)
	}

	return
}

func (dc *DistConnection) Send(from gen.Process, to etf.Pid, message etf.Term) error {
	return nil
}
func (dc *DistConnection) SendReg(from gen.Process, to gen.ProcessID, message etf.Term) error {
	return nil
}
func (dc *DistConnection) SendAlias(from gen.Process, to etf.Alias, message etf.Term) error {
	return nil
}

func (dc *DistConnection) Link(local gen.Process, remote etf.Pid) error {
	return nil
}
func (dc *DistConnection) Unlink(local gen.Process, remote etf.Pid) error {
	return nil
}
func (dc *DistConnection) SendExit(local etf.Pid, remote etf.Pid) error {
	return nil
}

func (dc *DistConnection) Monitor(local gen.Process, remote etf.Pid, ref etf.Ref) error {
	return nil
}
func (dc *DistConnection) MonitorReg(local gen.Process, remote gen.ProcessID, ref etf.Ref) error {
	return nil
}
func (dc *DistConnection) Demonitor(ref etf.Ref) error {
	return nil
}
func (dc *DistConnection) SendMonitorExitReg(process gen.Process, ref etf.Ref, reason string) error {
	return nil
}
func (dc *DistConnection) SendMonitorExit(process etf.Pid, ref etf.Ref, reason string) error {
	return nil
}

func (dc *DistConnection) SpawnRequest() error {
	return nil
}
func (dc *DistConnection) Proxy() error {
	return nil
}
func (dc *DistConnection) ProxyReg() error {
	return nil
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

type deferrMissing struct {
	b *lib.Buffer
	c int
}

func (l *Link) ReadHandlePacket(ctx context.Context, recv chan *lib.Buffer,
	handler func(string, etf.Term, etf.Term) error) {
	var b *lib.Buffer
	var missing deferrMissing
	var Timeout <-chan time.Time

	deferrChannel := make(chan deferrMissing, 100)
	defer close(deferrChannel)

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)

	dChannel := deferrChannel

	for {
		select {
		case missing = <-dChannel:
			b = missing.b
		default:
			if len(deferrChannel) > 0 {
				timer.Reset(150 * time.Millisecond)
				Timeout = timer.C
			} else {
				Timeout = nil
			}
			select {
			case b = <-recv:
				if b == nil {
					// channel was closed
					return
				}
			case <-Timeout:
				dChannel = deferrChannel
				continue
			}
		}

		// read and decode received packet
		control, message, err := l.ReadPacket(b.B)

		if err == ErrMissingInCache {
			if b == missing.b && missing.c > 100 {
				fmt.Println("Error: Disordered data at the link with", l.PeerName(), ". Close connection")
				l.Close()
				lib.ReleaseBuffer(b)
				return
			}

			if b == missing.b {
				missing.c++
			} else {
				missing.b = b
				missing.c = 0
			}

			select {
			case deferrChannel <- missing:
				// read recv channel
				dChannel = nil
				continue
			default:
				fmt.Println("Error: Mess at the link with", l.PeerName(), ". Close connection")
				l.Close()
				lib.ReleaseBuffer(b)
				return
			}
		}

		dChannel = deferrChannel

		if err != nil {
			fmt.Println("Malformed Dist proto at the link with", l.PeerName(), err)
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
			fmt.Printf("Malformed Control packet at the link with %s: %#v\n", l.PeerName(), control)
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
