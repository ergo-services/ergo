package dist

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

var (
	ErrMissingInCache     = fmt.Errorf("missing in cache")
	ErrMalformed          = fmt.Errorf("malformed")
	ErrOverloadConnection = fmt.Errorf("connection buffer is overloaded")
	gzipReaders           = &sync.Pool{
		New: func() interface{} {
			return nil
		},
	}
	gzipWriters = [10]*sync.Pool{}
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
	for i := range gzipWriters {
		gzipWriters[i] = &sync.Pool{
			New: func() interface{} {
				return nil
			},
		}
	}
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

	// ergo gzipped messages
	protoDistMessageZ   = 200
	protoDistFragment1Z = 201
	protoDistFragmentNZ = 202

	// ergo proxy message
	protoDistProxy = 210
)

type fragmentedPacket struct {
	buffer           *lib.Buffer
	disordered       *lib.Buffer
	disorderedSlices map[uint64][]byte
	fragmentID       uint64
	lastUpdate       time.Time
}

type distConnection struct {
	node.Connection

	nodename string
	peername string
	ctx      context.Context

	// peer flags
	flags node.Flags

	// socket
	conn          io.ReadWriter
	cancelContext context.CancelFunc

	// route incoming messages
	router node.CoreRouter

	// writer
	flusher *linkFlusher

	// senders list of channels for the sending goroutines
	senders senders
	// receivers list of channels for the receiving goroutines
	receivers receivers

	// atom cache for incoming messages
	cacheIn      [2048]*etf.Atom
	cacheInMutex sync.RWMutex

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

type distProto struct {
	node.Proto
	nodename string
	options  node.ProtoOptions
}

func CreateProto(nodename string, options node.ProtoOptions) node.ProtoInterface {
	return &distProto{
		nodename: nodename,
		options:  options,
	}
}

//
// node.Proto interface implementation
//

type senders struct {
	sender []*senderChannel
	n      int32
	i      int32
}

type senderChannel struct {
	sync.Mutex
	sendChannel chan *sendMessage
}

type sendMessage struct {
	control              etf.Term
	payload              etf.Term
	compression          bool
	compressionLevel     int
	compressionThreshold int
}

type receivers struct {
	recv []chan *lib.Buffer
	n    int32
	i    int32
}

func (dp *distProto) Init(ctx context.Context, conn io.ReadWriter, peername string, flags node.Flags) (node.ConnectionInterface, error) {
	connection := &distConnection{
		nodename:           dp.nodename,
		peername:           peername,
		flags:              flags,
		conn:               conn,
		fragments:          make(map[uint64]*fragmentedPacket),
		checkCleanTimeout:  defaultCleanTimeout,
		checkCleanDeadline: defaultCleanDeadline,
	}
	connection.ctx, connection.cancelContext = context.WithCancel(ctx)

	// initializing atom cache if its enabled
	if flags.EnableHeaderAtomCache {
		connection.cacheOut = etf.StartAtomCache()
	}

	// create connection buffering
	connection.flusher = newLinkFlusher(conn, defaultLatency, flags.EnableSoftwareKeepAlive)

	// do not use shared channels within intencive code parts, impacts on a performance
	connection.receivers = receivers{
		recv: make([]chan *lib.Buffer, dp.options.NumHandlers),
		n:    int32(dp.options.NumHandlers),
	}

	// run readers for incoming messages
	for i := 0; i < dp.options.NumHandlers; i++ {
		// run packet reader routines (decoder)
		recv := make(chan *lib.Buffer, dp.options.RecvQueueLength)
		connection.receivers.recv[i] = recv
		go connection.receiver(recv)
	}

	connection.senders = senders{
		sender: make([]*senderChannel, dp.options.NumHandlers),
		n:      int32(dp.options.NumHandlers),
	}

	// run readers/writers for incoming/outgoing messages
	for i := 0; i < dp.options.NumHandlers; i++ {
		// run writer routines (encoder)
		send := make(chan *sendMessage, dp.options.SendQueueLength)
		connection.senders.sender[i] = &senderChannel{
			sendChannel: send,
		}
		go connection.sender(send, dp.options, connection.flags)
	}

	return connection, nil
}

func (dp *distProto) Serve(ci node.ConnectionInterface, router node.CoreRouter) {
	connection, ok := ci.(*distConnection)
	if !ok {
		fmt.Println("conn is not a *distConnection type")
		return
	}

	connection.router = router

	// run read loop
	var err error
	var packetLength int

	b := lib.TakeBuffer()
	for {
		packetLength, err = connection.read(b, dp.options.MaxMessageSize)

		// validation
		if err != nil || packetLength == 0 {
			// link was closed or got malformed data
			if err != nil {
				fmt.Println("link was closed", connection.peername, "error:", err)
			}
			lib.ReleaseBuffer(b)
			return
		}

		// check the context if it was cancelled
		if connection.ctx.Err() != nil {
			// canceled
			lib.ReleaseBuffer(b)
			return
		}

		// take the new buffer for the next reading and append the tail
		// (which is part of the next packet)
		b1 := lib.TakeBuffer()
		b1.Set(b.B[packetLength:])

		// cut the tail and send it further for handling.
		// buffer b has to be released by the reader of
		// recv channel (link.ReadHandlePacket)
		b.B = b.B[:packetLength]
		connection.receivers.recv[connection.receivers.i] <- b

		// set new buffer as a current for the next reading
		b = b1

		// round-robin switch to the next receiver
		connection.receivers.i++
		if connection.receivers.i < connection.receivers.n {
			continue
		}
		connection.receivers.i = 0
	}

}

func (dp *distProto) Terminate(ci node.ConnectionInterface) {
	connection, ok := ci.(*distConnection)
	if !ok {
		fmt.Println("conn is not a *distConnection type")
		return
	}

	for i := 0; i < dp.options.NumHandlers; i++ {
		sender := connection.senders.sender[i]
		if sender != nil {
			sender.Lock()
			close(sender.sendChannel)
			sender.sendChannel = nil
			sender.Unlock()
			connection.senders.sender[i] = nil
		}
		if connection.receivers.recv[i] != nil {
			close(connection.receivers.recv[i])
		}
	}
	if connection.cacheOut != nil {
		connection.cacheOut.Stop()
	}
	connection.flusher.Stop()
	connection.cancelContext()
}

// node.Connection interface implementation

func (dc *distConnection) Send(from gen.Process, to etf.Pid, message etf.Term) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoSEND, etf.Atom(""), to},
		payload: message,
	}
	if dc.flags.EnableCompression {
		msg.compression = from.Compression()
		msg.compressionLevel = from.CompressionLevel()
		msg.compressionThreshold = from.CompressionThreshold()
	}

	return dc.send(msg)
}
func (dc *distConnection) SendReg(from gen.Process, to gen.ProcessID, message etf.Term) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoREG_SEND, from.Self(), etf.Atom(""), etf.Atom(to.Name)},
		payload: message,
	}

	if dc.flags.EnableCompression {
		msg.compression = from.Compression()
		msg.compressionLevel = from.CompressionLevel()
		msg.compressionThreshold = from.CompressionThreshold()
	}
	return dc.send(msg)
}
func (dc *distConnection) SendAlias(from gen.Process, to etf.Alias, message etf.Term) error {
	if dc.flags.EnableAlias == false {
		return node.ErrUnsupported
	}

	msg := &sendMessage{
		control: etf.Tuple{distProtoALIAS_SEND, from.Self(), to},
		payload: message,
	}

	if dc.flags.EnableCompression {
		msg.compression = from.Compression()
		msg.compressionLevel = from.CompressionLevel()
		msg.compressionThreshold = from.CompressionThreshold()
	}
	return dc.send(msg)
}

func (dc *distConnection) Link(local etf.Pid, remote etf.Pid) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoLINK, local, remote},
	}
	return dc.send(msg)
}
func (dc *distConnection) Unlink(local etf.Pid, remote etf.Pid) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoUNLINK, local, remote},
	}
	return dc.send(msg)
}
func (dc *distConnection) LinkExit(to etf.Pid, terminated etf.Pid, reason string) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoEXIT, terminated, to, etf.Atom(reason)},
	}
	return dc.send(msg)
}

func (dc *distConnection) Monitor(local etf.Pid, remote etf.Pid, ref etf.Ref) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoMONITOR, local, remote, ref},
	}
	return dc.send(msg)
}
func (dc *distConnection) MonitorReg(local etf.Pid, remote gen.ProcessID, ref etf.Ref) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoMONITOR, local, etf.Atom(remote.Name), ref},
	}
	return dc.send(msg)
}
func (dc *distConnection) Demonitor(local etf.Pid, remote etf.Pid, ref etf.Ref) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoDEMONITOR, local, remote, ref},
	}
	return dc.send(msg)
}
func (dc *distConnection) DemonitorReg(local etf.Pid, remote gen.ProcessID, ref etf.Ref) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoDEMONITOR, local, etf.Atom(remote.Name), ref},
	}
	return dc.send(msg)
}
func (dc *distConnection) MonitorExitReg(to etf.Pid, terminated gen.ProcessID, reason string, ref etf.Ref) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoMONITOR_EXIT, etf.Atom(terminated.Name), to, ref, etf.Atom(reason)},
	}
	return dc.send(msg)
}
func (dc *distConnection) MonitorExit(to etf.Pid, terminated etf.Pid, reason string, ref etf.Ref) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoMONITOR_EXIT, terminated, to, ref, etf.Atom(reason)},
	}
	return dc.send(msg)
}

func (dc *distConnection) SpawnRequest(behaviorName string, request gen.RemoteSpawnRequest, args ...etf.Term) error {
	if dc.flags.EnableRemoteSpawn == false {
		return node.ErrUnsupported
	}

	optlist := etf.List{}
	if request.Options.Name != "" {
		optlist = append(optlist, etf.Tuple{etf.Atom("name"), etf.Atom(request.Options.Name)})

	}
	msg := &sendMessage{
		control: etf.Tuple{distProtoSPAWN_REQUEST, request.Ref, request.From, request.From,
			// {M,F,A}
			etf.Tuple{etf.Atom(behaviorName), etf.Atom(request.Options.Function), len(args)},
			optlist,
		},
		payload: args,
	}
	return dc.send(msg)
}

func (dc *distConnection) SpawnReply(to etf.Pid, ref etf.Ref, pid etf.Pid) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoSPAWN_REPLY, ref, to, 0, pid},
	}
	return dc.send(msg)
}

func (dc *distConnection) SpawnReplyError(to etf.Pid, ref etf.Ref, err error) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoSPAWN_REPLY, ref, to, 0, etf.Atom(err.Error())},
	}
	return dc.send(msg)
}

func (dc *distConnection) Proxy() error {
	return nil
}

//
// internal
//

func (dc *distConnection) read(b *lib.Buffer, max int) (int, error) {
	// http://erlang.org/doc/apps/erts/erl_dist_protocol.html#protocol-between-connected-nodes
	expectingBytes := 4

	for {
		if b.Len() < expectingBytes {
			n, e := b.ReadDataFrom(dc.conn, max)
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
			// it was "software" keepalive
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

func (dc *distConnection) receiver(recv <-chan *lib.Buffer) {
	var b *lib.Buffer
	var missing deferrMissing
	var Timeout <-chan time.Time

	// cancel connection context if something went wrong
	// it will cause closing connection with stopping all
	// goroutines around this connection
	defer dc.cancelContext()

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
		control, message, err := dc.decodePacket(b.B)

		if err == ErrMissingInCache {
			if b == missing.b && missing.c > 100 {
				fmt.Println("Error: Disordered data at the link with", dc.peername, ". Close connection")
				dc.cancelContext()
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
				fmt.Println("Error: Mess at the link with", dc.peername, ". Close connection")
				dc.cancelContext()
				lib.ReleaseBuffer(b)
				return
			}
		}

		dChannel = deferrChannel

		if err != nil {
			fmt.Println("Malformed Dist proto at the link with", dc.peername, err)
			dc.cancelContext()
			lib.ReleaseBuffer(b)
			return
		}

		if control == nil {
			// fragment
			continue
		}

		// handle message
		if err := dc.handleMessage(control, message); err != nil {
			fmt.Printf("Malformed Control packet at the link with %s: %#v\n", dc.peername, control)
			dc.cancelContext()
			lib.ReleaseBuffer(b)
			return
		}

		// we have to release this buffer
		lib.ReleaseBuffer(b)

	}
}

func (dc *distConnection) decodePacket(packet []byte) (etf.Term, etf.Term, error) {
	if len(packet) < 5 {
		return nil, nil, fmt.Errorf("malformed packet")
	}

	// [:3] length
	switch packet[4] {
	case protoDist:
		return dc.decodeDist(packet[5:])
	default:
		// unknown proto
		return nil, nil, fmt.Errorf("unknown/unsupported proto")
	}

}

func (dc *distConnection) decodeDist(packet []byte) (etf.Term, etf.Term, error) {
	switch packet[0] {
	case protoDistMessage:
		var control, message etf.Term
		var err error
		var cache []etf.Atom

		cache, packet, err = dc.decodeDistHeaderAtomCache(packet[1:])
		if err != nil {
			return nil, nil, err
		}

		decodeOptions := etf.DecodeOptions{
			FlagBigPidRef: dc.flags.EnableBigPidRef,
		}

		// decode control message
		control, packet, err = etf.Decode(packet, cache, decodeOptions)
		if err != nil {
			return nil, nil, err
		}

		if len(packet) == 0 {
			return control, nil, nil
		}

		// decode payload message
		message, packet, err = etf.Decode(packet, cache, decodeOptions)
		if err != nil {
			return nil, nil, err
		}

		if len(packet) != 0 {
			return nil, nil, fmt.Errorf("packet has extra %d byte(s)", len(packet))
		}

		return control, message, nil

	case protoDistMessageZ:
		var control, message etf.Term
		var err error
		var cache []etf.Atom
		var zReader *gzip.Reader
		var total int
		// compressed protoDistMessage

		cache, packet, err = dc.decodeDistHeaderAtomCache(packet[1:])
		if err != nil {
			return nil, nil, err
		}

		// read the length of unpacked data
		lenUnpacked := int(binary.BigEndian.Uint32(packet[:4]))

		// take the gzip reader from the pool
		if r, ok := gzipReaders.Get().(*gzip.Reader); ok {
			zReader = r
			zReader.Reset(bytes.NewBuffer(packet[4:]))
		} else {
			zReader, _ = gzip.NewReader(bytes.NewBuffer(packet[4:]))
		}
		defer gzipReaders.Put(zReader)

		// take new buffer and allocate space for the unpacked data
		zBuffer := lib.TakeBuffer()
		zBuffer.Allocate(lenUnpacked)
		defer lib.ReleaseBuffer(zBuffer)

		// unzipping and decoding the data
		for {
			n, e := zReader.Read(zBuffer.B[total:])
			if n == 0 {
				return nil, nil, fmt.Errorf("zbuffer too small")
			}
			total += n
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, nil, e
			}
		}

		packet = zBuffer.B
		decodeOptions := etf.DecodeOptions{
			FlagBigPidRef: dc.flags.EnableBigPidRef,
		}

		// decode control message
		control, packet, err = etf.Decode(packet, cache, decodeOptions)
		if err != nil {
			fmt.Println("CTLR")
			return nil, nil, err
		}
		if len(packet) == 0 {
			return control, nil, nil
		}

		// decode payload message
		message, packet, err = etf.Decode(packet, cache, decodeOptions)
		if err != nil {
			return nil, nil, err
		}

		if len(packet) != 0 {
			return nil, nil, fmt.Errorf("packet has extra %d byte(s)", len(packet))
		}

		return control, message, nil

	case protoDistFragment1, protoDistFragmentN, protoDistFragment1Z, protoDistFragmentNZ:
		if len(packet) < 18 {
			return nil, nil, fmt.Errorf("malformed fragment")
		}

		if assembled, err := dc.decodeFragment(packet); assembled != nil {
			if err != nil {
				return nil, nil, err
			}
			defer lib.ReleaseBuffer(assembled)
			return dc.decodeDist(assembled.B)
		} else {
			if err != nil {
				return nil, nil, err
			}
		}
		return nil, nil, nil
	}

	return nil, nil, fmt.Errorf("unknown packet type %d", packet[0])
}

func (dc *distConnection) handleMessage(control, message etf.Term) (err error) {
	defer func() {
		if lib.CatchPanic() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%s", r)
			}
		}
	}()

	switch t := control.(type) {
	case etf.Tuple:
		switch act := t.Element(1).(type) {
		case int:
			switch act {
			case distProtoREG_SEND:
				// {6, FromPid, Unused, ToName}
				lib.Log("[%s] CONTROL REG_SEND [from %s]: %#v", dc.nodename, dc.peername, control)
				to := gen.ProcessID{
					Node: dc.nodename,
					Name: string(t.Element(4).(etf.Atom)),
				}
				dc.router.RouteSendReg(t.Element(2).(etf.Pid), to, message)
				return nil

			case distProtoSEND:
				// {2, Unused, ToPid}
				// SEND has no sender pid
				lib.Log("[%s] CONTROL SEND [from %s]: %#v", dc.nodename, dc.peername, control)
				dc.router.RouteSend(etf.Pid{}, t.Element(3).(etf.Pid), message)
				return nil

			case distProtoLINK:
				// {1, FromPid, ToPid}
				lib.Log("[%s] CONTROL LINK [from %s]: %#v", dc.nodename, dc.peername, control)
				dc.router.RouteLink(t.Element(2).(etf.Pid), t.Element(3).(etf.Pid))
				return nil

			case distProtoUNLINK:
				// {4, FromPid, ToPid}
				lib.Log("[%s] CONTROL UNLINK [from %s]: %#v", dc.nodename, dc.peername, control)
				dc.router.RouteUnlink(t.Element(2).(etf.Pid), t.Element(3).(etf.Pid))
				return nil

			case distProtoNODE_LINK:
				lib.Log("[%s] CONTROL NODE_LINK [from %s]: %#v", dc.nodename, dc.peername, control)
				return nil

			case distProtoEXIT:
				// {3, FromPid, ToPid, Reason}
				lib.Log("[%s] CONTROL EXIT [from %s]: %#v", dc.nodename, dc.peername, control)
				terminated := t.Element(2).(etf.Pid)
				to := t.Element(3).(etf.Pid)
				reason := fmt.Sprint(t.Element(4))
				dc.router.RouteExit(to, terminated, string(reason))
				return nil

			case distProtoEXIT2:
				lib.Log("[%s] CONTROL EXIT2 [from %s]: %#v", dc.nodename, dc.peername, control)
				return nil

			case distProtoMONITOR:
				// {19, FromPid, ToProc, Ref}, where FromPid = monitoring process
				// and ToProc = monitored process pid or name (atom)
				lib.Log("[%s] CONTROL MONITOR [from %s]: %#v", dc.nodename, dc.peername, control)

				fromPid := t.Element(2).(etf.Pid)
				ref := t.Element(4).(etf.Ref)
				// if monitoring by pid
				if to, ok := t.Element(3).(etf.Pid); ok {
					dc.router.RouteMonitor(fromPid, to, ref)
					return nil
				}

				// if monitoring by process name
				if to, ok := t.Element(3).(etf.Atom); ok {
					processID := gen.ProcessID{
						Node: dc.nodename,
						Name: string(to),
					}
					dc.router.RouteMonitorReg(fromPid, processID, ref)
					return nil
				}

				return fmt.Errorf("malformed monitor message")

			case distProtoDEMONITOR:
				// {20, FromPid, ToProc, Ref}, where FromPid = monitoring process
				// and ToProc = monitored process pid or name (atom)
				lib.Log("[%s] CONTROL DEMONITOR [from %s]: %#v", dc.nodename, dc.peername, control)
				ref := t.Element(4).(etf.Ref)
				fromPid := t.Element(2).(etf.Pid)
				dc.router.RouteDemonitor(fromPid, ref)
				return nil

			case distProtoMONITOR_EXIT:
				// {21, FromProc, ToPid, Ref, Reason}, where FromProc = monitored process
				// pid or name (atom), ToPid = monitoring process, and Reason = exit reason for the monitored process
				lib.Log("[%s] CONTROL MONITOR_EXIT [from %s]: %#v", dc.nodename, dc.peername, control)
				reason := fmt.Sprint(t.Element(5))
				ref := t.Element(4).(etf.Ref)
				switch terminated := t.Element(2).(type) {
				case etf.Pid:
					dc.router.RouteMonitorExit(terminated, reason, ref)
					return nil
				case etf.Atom:
					processID := gen.ProcessID{Name: string(terminated), Node: dc.peername}
					dc.router.RouteMonitorExitReg(processID, reason, ref)
					return nil
				}
				return fmt.Errorf("malformed monitor exit message")

			// Not implemented yet, just stubs. TODO.
			case distProtoSEND_SENDER:
				lib.Log("[%s] CONTROL SEND_SENDER unsupported [from %s]: %#v", dc.nodename, dc.peername, control)
				return nil
			case distProtoPAYLOAD_EXIT:
				lib.Log("[%s] CONTROL PAYLOAD_EXIT unsupported [from %s]: %#v", dc.nodename, dc.peername, control)
				return nil
			case distProtoPAYLOAD_EXIT2:
				lib.Log("[%s] CONTROL PAYLOAD_EXIT2 unsupported [from %s]: %#v", dc.nodename, dc.peername, control)
				return nil
			case distProtoPAYLOAD_MONITOR_P_EXIT:
				lib.Log("[%s] CONTROL PAYLOAD_MONITOR_P_EXIT unsupported [from %s]: %#v", dc.nodename, dc.peername, control)
				return nil

			// alias support
			case distProtoALIAS_SEND:
				// {33, FromPid, Alias}
				lib.Log("[%s] CONTROL ALIAS_SEND [from %s]: %#v", dc.nodename, dc.peername, control)
				alias := etf.Alias(t.Element(3).(etf.Ref))
				dc.router.RouteSendAlias(t.Element(2).(etf.Pid), alias, message)
				return nil

			case distProtoSPAWN_REQUEST:
				// {29, ReqId, From, GroupLeader, {Module, Function, Arity}, OptList}
				lib.Log("[%s] CONTROL SPAWN_REQUEST [from %s]: %#v", dc.nodename, dc.peername, control)
				registerName := ""
				for _, option := range t.Element(6).(etf.List) {
					name, ok := option.(etf.Tuple)
					if !ok || len(name) != 2 {
						return fmt.Errorf("malformed spawn request")
					}
					switch name.Element(1) {
					case etf.Atom("name"):
						registerName = string(name.Element(2).(etf.Atom))
					}
				}

				from := t.Element(3).(etf.Pid)
				ref := t.Element(2).(etf.Ref)

				mfa := t.Element(5).(etf.Tuple)
				module := mfa.Element(1).(etf.Atom)
				function := mfa.Element(2).(etf.Atom)
				var args etf.List
				if str, ok := message.(string); !ok {
					args, _ = message.(etf.List)
				} else {
					// stupid Erlang's strings :). [1,2,3,4,5] sends as a string.
					// args can't be anything but etf.List.
					for i := range []byte(str) {
						args = append(args, str[i])
					}
				}

				spawnRequestOptions := gen.RemoteSpawnOptions{
					Name:     registerName,
					Function: string(function),
				}
				spawnRequest := gen.RemoteSpawnRequest{
					From:    from,
					Ref:     ref,
					Options: spawnRequestOptions,
				}
				dc.router.RouteSpawnRequest(dc.nodename, string(module), spawnRequest, args...)
				return nil

			case distProtoSPAWN_REPLY:
				// {31, ReqId, To, Flags, Result}
				lib.Log("[%s] CONTROL SPAWN_REPLY [from %s]: %#v", dc.nodename, dc.peername, control)
				to := t.Element(3).(etf.Pid)
				ref := t.Element(2).(etf.Ref)
				dc.router.RouteSpawnReply(to, ref, t.Element(5))
				return nil

			default:
				lib.Log("[%s] CONTROL unknown command [from %s]: %#v", dc.nodename, dc.peername, control)
				return fmt.Errorf("unknown control command %#v", control)
			}
		}
	}

	return fmt.Errorf("unsupported control message %#v", control)
}

func (dc *distConnection) decodeFragment(packet []byte) (*lib.Buffer, error) {
	var first, compressed bool
	var err error

	sequenceID := binary.BigEndian.Uint64(packet[1:9])
	fragmentID := binary.BigEndian.Uint64(packet[9:17])
	if fragmentID == 0 {
		return nil, fmt.Errorf("fragmentID can't be 0")
	}

	switch packet[0] {
	case protoDistFragment1:
		// We should decode atom cache from the first fragment in order
		// to get rid the case when we get the first fragment of the packet with
		// cached atoms and the next packet is not the part of the fragmented packet,
		// but with the ids were cached in the first fragment
		_, _, err = dc.decodeDistHeaderAtomCache(packet[17:])
		if err != nil {
			return nil, err
		}
		first = true
	case protoDistFragment1Z:
		_, _, err = dc.decodeDistHeaderAtomCache(packet[17:])
		if err != nil {
			return nil, err
		}
		first = true
		compressed = true
	case protoDistFragmentNZ:
		compressed = true
	}
	packet = packet[17:]

	dc.fragmentsMutex.Lock()
	defer dc.fragmentsMutex.Unlock()

	fragmented, ok := dc.fragments[sequenceID]
	if !ok {
		fragmented = &fragmentedPacket{
			buffer:           lib.TakeBuffer(),
			disordered:       lib.TakeBuffer(),
			disorderedSlices: make(map[uint64][]byte),
			lastUpdate:       time.Now(),
		}

		// append new packet type
		if compressed {
			fragmented.buffer.AppendByte(protoDistMessageZ)
		} else {
			fragmented.buffer.AppendByte(protoDistMessage)
		}
		dc.fragments[sequenceID] = fragmented
	}

	// until we get the first item everything will be treated as disordered
	if first {
		fragmented.fragmentID = fragmentID + 1
	}

	if fragmented.fragmentID-fragmentID != 1 {
		// got the next fragment. disordered
		slice := fragmented.disordered.Extend(len(packet))
		copy(slice, packet)
		fragmented.disorderedSlices[fragmentID] = slice
	} else {
		// order is correct. just append
		fragmented.buffer.Append(packet)
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
		delete(dc.fragments, sequenceID)
		lib.ReleaseBuffer(fragmented.disordered)
		return fragmented.buffer, nil
	}

	if dc.checkCleanPending {
		return nil, nil
	}

	if dc.checkCleanTimer != nil {
		dc.checkCleanTimer.Reset(dc.checkCleanTimeout)
		return nil, nil
	}

	dc.checkCleanTimer = time.AfterFunc(dc.checkCleanTimeout, func() {
		dc.fragmentsMutex.Lock()
		defer dc.fragmentsMutex.Unlock()

		if len(dc.fragments) == 0 {
			dc.checkCleanPending = false
			return
		}

		valid := time.Now().Add(-dc.checkCleanDeadline)
		for sequenceID, fragmented := range dc.fragments {
			if fragmented.lastUpdate.Before(valid) {
				// dropping  due to exceeded deadline
				delete(dc.fragments, sequenceID)
			}
		}
		if len(dc.fragments) == 0 {
			dc.checkCleanPending = false
			return
		}

		dc.checkCleanPending = true
		dc.checkCleanTimer.Reset(dc.checkCleanTimeout)
	})

	return nil, nil
}

func (dc *distConnection) decodeDistHeaderAtomCache(packet []byte) ([]etf.Atom, []byte, error) {
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
			dc.cacheInMutex.Lock()
			dc.cacheIn[idx] = &atom
			dc.cacheInMutex.Unlock()
			packet = packet[atomLen:]
			continue
		}

		dc.cacheInMutex.RLock()
		c := dc.cacheIn[idx]
		dc.cacheInMutex.RUnlock()
		if c == nil {
			return cache, packet, ErrMissingInCache
		}
		cache[i] = *c
		packet = packet[1:]
	}

	return cache, packet, nil
}

func (dc *distConnection) encodeDistHeaderAtomCache(b *lib.Buffer,
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

		// we have to clear it before reuse
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

func (dc *distConnection) sender(send <-chan *sendMessage, options node.ProtoOptions, peerFlags node.Flags) {
	var encodingAtomCache *etf.ListAtomCache
	var writerAtomCache map[etf.Atom]etf.CacheItem
	var linkAtomCache *etf.AtomCache
	var lastCacheID int16 = -1

	var lenMessage, lenAtomCache, lenPacket, startDataPosition int
	var atomCacheBuffer, packetBuffer *lib.Buffer
	var message *sendMessage
	var err error
	var compress, compressed bool
	var zWriter *gzip.Writer

	// cancel connection context if something went wrong
	// it will cause closing connection with stopping all
	// goroutines around this connection
	defer dc.cancelContext()

	cacheEnabled := peerFlags.EnableHeaderAtomCache && dc.cacheOut != nil
	fragmentationEnabled := peerFlags.EnableFragmentation && options.FragmentationUnit > 0
	compressionEnabled := peerFlags.EnableCompression

	// Header atom cache is encoded right after the control/message encoding process
	// but should be stored as a first item in the packet.
	// Thats why we do reserve some space for it in order to get rid
	// of reallocation packetBuffer data
	reserveHeaderAtomCache := 8192

	if cacheEnabled {
		encodingAtomCache = etf.TakeListAtomCache()
		defer etf.ReleaseListAtomCache(encodingAtomCache)
		writerAtomCache = make(map[etf.Atom]etf.CacheItem)
		linkAtomCache = dc.cacheOut
	}

	encodeOptions := etf.EncodeOptions{
		LinkAtomCache:     linkAtomCache,
		WriterAtomCache:   writerAtomCache,
		EncodingAtomCache: encodingAtomCache,
		FlagBigCreation:   peerFlags.EnableBigCreation,
		FlagBigPidRef:     peerFlags.EnableBigPidRef,
	}

	for {
		if message != nil {
			message.control = nil
			message.payload = nil
		}

		message = <-send

		if message == nil {
			// channel was closed
			return
		}

		packetBuffer = lib.TakeBuffer()
		lenMessage, lenAtomCache, lenPacket = 0, 0, 0
		startDataPosition = reserveHeaderAtomCache

		// do reserve for the header 8K, should be enough
		packetBuffer.Allocate(reserveHeaderAtomCache)

		// check whether compress is enabled for the peer and for this message
		compress = compressionEnabled && message.compression
		compressed = false

		// clear encoding cache
		if cacheEnabled {
			encodingAtomCache.Reset()
		}

		// We could use gzip writer for the encoder, but we don't know
		// the actual size of the control/payload. For small data, gzipping
		// is getting extremely inefficient. That's why it is cheaper to
		// encode control/payload first and then decide whether to compress it
		// according to a threshold value.

		// encode Control
		err = etf.Encode(message.control, packetBuffer, encodeOptions)
		if err != nil {
			fmt.Println(err)
			lib.ReleaseBuffer(packetBuffer)
			continue
		}

		// encode Message if present
		if message.payload != nil {
			err = etf.Encode(message.payload, packetBuffer, encodeOptions)
			if err != nil {
				fmt.Println(err)
				lib.ReleaseBuffer(packetBuffer)
				continue
			}

		}
		lenMessage = packetBuffer.Len() - reserveHeaderAtomCache

		if compress && packetBuffer.Len() > (reserveHeaderAtomCache+message.compressionThreshold) {
			//// take another buffer
			zBuffer := lib.TakeBuffer()
			// allocate extra 4 bytes for the lenMessage (length of unpacked data)
			zBuffer.Allocate(reserveHeaderAtomCache + 4)
			level := message.compressionLevel
			if level == -1 {
				level = 0
			}
			if w, ok := gzipWriters[level].Get().(*gzip.Writer); ok {
				zWriter = w
				zWriter.Reset(zBuffer)
			} else {
				zWriter, _ = gzip.NewWriterLevel(zBuffer, message.compressionLevel)
			}
			zWriter.Write(packetBuffer.B[reserveHeaderAtomCache:])
			zWriter.Close()
			gzipWriters[level].Put(zWriter)

			// swap buffers only if gzipped data less than the original ones
			if zBuffer.Len() < packetBuffer.Len() {
				binary.BigEndian.PutUint32(zBuffer.B[reserveHeaderAtomCache:], uint32(lenMessage))
				lenMessage = zBuffer.Len() - reserveHeaderAtomCache
				packetBuffer, zBuffer = zBuffer, packetBuffer
				compressed = true
			}
			lib.ReleaseBuffer(zBuffer)
		}

		// encode Header Atom Cache if its enabled
		if cacheEnabled && encodingAtomCache.Len() > 0 {
			atomCacheBuffer = lib.TakeBuffer()
			dc.encodeDistHeaderAtomCache(atomCacheBuffer, writerAtomCache, encodingAtomCache)
			lenAtomCache = atomCacheBuffer.Len()

			if lenAtomCache > reserveHeaderAtomCache-22 {
				// are you serious? ))) what da hell you just sent?
				// FIXME i'm gonna fix it if someone report about this issue :)
				fmt.Println("WARNING: exceed atom header cache size limit. please report about this issue")
				return
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
			// 4 (packet len) + 1 (dist header: 131) + 1 (dist header: protoDistMessage[Z]) + lenAtomCache
			lenPacket = 1 + 1 + lenAtomCache + lenMessage
			if !fragmentationEnabled || lenPacket < options.FragmentationUnit {
				// send as a single packet
				startDataPosition -= 6

				binary.BigEndian.PutUint32(packetBuffer.B[startDataPosition:], uint32(lenPacket))
				packetBuffer.B[startDataPosition+4] = protoDist // 131
				if compressed {
					packetBuffer.B[startDataPosition+5] = protoDistMessageZ // 200
				} else {
					packetBuffer.B[startDataPosition+5] = protoDistMessage // 68
				}
				if _, err := dc.flusher.Write(packetBuffer.B[startDataPosition:]); err != nil {
					return
				}
				break
			}

			// Message should be fragmented

			// https://erlang.org/doc/apps/erts/erl_ext_dist.html#distribution-header-for-fragmented-messages
			// "The entire atom cache and control message has to be part of the starting fragment"

			sequenceID := uint64(atomic.AddInt64(&dc.sequenceID, 1))
			numFragments := lenPacket/options.FragmentationUnit + 1

			// 1 (dist header: 131) + 1 (dist header: protoDistFragment) + 8 (sequenceID) + 8 (fragmentID) + ...
			lenPacket = 1 + 1 + 8 + 8 + lenAtomCache + options.FragmentationUnit

			// 4 (packet len) + 1 (dist header: 131) + 1 (dist header: protoDistFragment[Z]) + 8 (sequenceID) + 8 (fragmentID)
			startDataPosition -= 22

			binary.BigEndian.PutUint32(packetBuffer.B[startDataPosition:], uint32(lenPacket))
			packetBuffer.B[startDataPosition+4] = protoDist // 131
			if compressed {
				packetBuffer.B[startDataPosition+5] = protoDistFragment1Z // 201
			} else {
				packetBuffer.B[startDataPosition+5] = protoDistFragment1 // 69
			}

			binary.BigEndian.PutUint64(packetBuffer.B[startDataPosition+6:], uint64(sequenceID))
			binary.BigEndian.PutUint64(packetBuffer.B[startDataPosition+14:], uint64(numFragments))
			if _, err := dc.flusher.Write(packetBuffer.B[startDataPosition : startDataPosition+4+lenPacket]); err != nil {
				return
			}

			startDataPosition += 4 + lenPacket
			numFragments--

		nextFragment:

			if len(packetBuffer.B[startDataPosition:]) > options.FragmentationUnit {
				lenPacket = 1 + 1 + 8 + 8 + options.FragmentationUnit
				// reuse the previous 22 bytes for the next frame header
				startDataPosition -= 22

			} else {
				// the last one
				lenPacket = 1 + 1 + 8 + 8 + len(packetBuffer.B[startDataPosition:])
				startDataPosition -= 22
			}

			binary.BigEndian.PutUint32(packetBuffer.B[startDataPosition:], uint32(lenPacket))
			packetBuffer.B[startDataPosition+4] = protoDist // 131
			if compressed {
				packetBuffer.B[startDataPosition+5] = protoDistFragmentNZ // 202
			} else {
				packetBuffer.B[startDataPosition+5] = protoDistFragmentN // 70
			}

			binary.BigEndian.PutUint64(packetBuffer.B[startDataPosition+6:], uint64(sequenceID))
			binary.BigEndian.PutUint64(packetBuffer.B[startDataPosition+14:], uint64(numFragments))
			if _, err := dc.flusher.Write(packetBuffer.B[startDataPosition : startDataPosition+4+lenPacket]); err != nil {
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

		if cacheEnabled == false {
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

func (dc *distConnection) send(msg *sendMessage) error {
	i := atomic.AddInt32(&dc.senders.i, 1)
	n := i % dc.senders.n
	s := dc.senders.sender[n]
	if s == nil {
		// connection was closed
		return node.ErrNoRoute
	}
	s.Lock()
	defer s.Unlock()

	s.sendChannel <- msg
	return nil

	// TODO to decide whether to return error if channel is full
	//select {
	//case s.sendChannel <- msg:
	//	return nil
	//default:
	//	return ErrOverloadConnection
	//}
}
