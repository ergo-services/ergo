package dist

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

var (
	errMissingInCache = fmt.Errorf("missing in cache")
	errMalformed      = fmt.Errorf("malformed")
	gzipReaders       = &sync.Pool{
		New: func() interface{} {
			return nil
		},
	}
	gzipWriters  = [10]*sync.Pool{}
	sendMessages = &sync.Pool{
		New: func() interface{} {
			return &sendMessage{}
		},
	}
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

	// ergo proxy message
	protoProxy = 141
	// ergo proxy encrypted message
	protoProxyX = 142

	// http://erlang.org/doc/apps/erts/erl_ext_dist.html#distribution_header
	protoDist          = 131
	protoDistMessage   = 68
	protoDistFragment1 = 69
	protoDistFragmentN = 70

	// ergo gzipped messages
	protoDistMessageZ   = 200
	protoDistFragment1Z = 201
	protoDistFragmentNZ = 202
)

type fragmentedPacket struct {
	buffer           *lib.Buffer
	disordered       *lib.Buffer
	disorderedSlices map[uint64][]byte
	fragmentID       uint64
	lastUpdate       time.Time
}

type proxySession struct {
	session     node.ProxySession
	cache       etf.AtomCache
	senderCache []map[etf.Atom]etf.CacheItem
}

type distConnection struct {
	node.Connection

	nodename string
	peername string
	ctx      context.Context

	// peer flags
	flags node.Flags

	creation uint32

	// socket
	conn          lib.NetReadWriter
	cancelContext context.CancelFunc

	// proxy session (endpoints)
	proxySessionsByID       map[string]proxySession
	proxySessionsByPeerName map[string]proxySession
	proxySessionsMutex      sync.RWMutex

	// route incoming messages
	router node.CoreRouter

	// writer
	flusher *linkFlusher

	// buffer
	buffer *lib.Buffer

	// senders list of channels for the sending goroutines
	senders senders
	// receivers list of channels for the receiving goroutines
	receivers receivers

	// atom cache for outgoing messages
	cache etf.AtomCache

	mapping etf.AtomMapping

	// fragmentation sequence ID
	sequenceID     int64
	fragments      map[uint64]*fragmentedPacket
	fragmentsMutex sync.Mutex

	// check and clean lost fragments
	checkCleanPending  bool
	checkCleanTimer    *time.Timer
	checkCleanTimeout  time.Duration // default is 5 seconds
	checkCleanDeadline time.Duration // how long we wait for the next fragment of the certain sequenceID. Default is 30 seconds

	// stats
	stats node.NetworkStats
}

type distProto struct {
	node.Proto
	nodename string
	options  node.ProtoOptions
}

func CreateProto(options node.ProtoOptions) node.ProtoInterface {
	return &distProto{
		options: options,
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
	packet               *lib.Buffer
	control              etf.Term
	payload              etf.Term
	compression          bool
	compressionLevel     int
	compressionThreshold int
	proxy                *proxySession
}

type receivers struct {
	recv []chan *lib.Buffer
	n    int32
	i    int32
}

func (dp *distProto) Init(ctx context.Context, conn lib.NetReadWriter, nodename string, details node.HandshakeDetails) (node.ConnectionInterface, error) {
	connection := &distConnection{
		nodename:                nodename,
		peername:                details.Name,
		flags:                   details.Flags,
		creation:                details.Creation,
		buffer:                  details.Buffer,
		conn:                    conn,
		cache:                   etf.NewAtomCache(),
		mapping:                 details.AtomMapping,
		proxySessionsByID:       make(map[string]proxySession),
		proxySessionsByPeerName: make(map[string]proxySession),
		fragments:               make(map[uint64]*fragmentedPacket),
		checkCleanTimeout:       defaultCleanTimeout,
		checkCleanDeadline:      defaultCleanDeadline,
	}
	connection.ctx, connection.cancelContext = context.WithCancel(ctx)

	connection.stats.NodeName = details.Name

	// create connection buffering
	connection.flusher = newLinkFlusher(conn, defaultLatency)

	numHandlers := dp.options.NumHandlers
	if details.NumHandlers > 0 {
		numHandlers = details.NumHandlers
	}

	// do not use shared channels within intencive code parts, impacts on a performance
	connection.receivers = receivers{
		recv: make([]chan *lib.Buffer, numHandlers),
		n:    int32(numHandlers),
	}

	// run readers for incoming messages
	for i := 0; i < numHandlers; i++ {
		// run packet reader routines (decoder)
		recv := make(chan *lib.Buffer, dp.options.RecvQueueLength)
		connection.receivers.recv[i] = recv
		go connection.receiver(recv)
	}

	connection.senders = senders{
		sender: make([]*senderChannel, numHandlers),
		n:      int32(numHandlers),
	}

	// run readers/writers for incoming/outgoing messages
	for i := 0; i < numHandlers; i++ {
		// run writer routines (encoder)
		send := make(chan *sendMessage, dp.options.SendQueueLength)
		connection.senders.sender[i] = &senderChannel{
			sendChannel: send,
		}
		go connection.sender(i, send, dp.options, connection.flags)
	}

	return connection, nil
}

func (dp *distProto) Serve(ci node.ConnectionInterface, router node.CoreRouter) {
	connection, ok := ci.(*distConnection)
	if !ok {
		lib.Warning("conn is not a *distConnection type")
		return
	}

	connection.router = router

	// run read loop
	var err error
	var packetLength int

	b := connection.buffer // not nil if we got extra data withing the handshake process
	if b == nil {
		b = lib.TakeBuffer()
	}

	for {
		packetLength, err = connection.read(b, dp.options.MaxMessageSize)

		// validation
		if err != nil || packetLength == 0 {
			// link was closed or got malformed data
			if err != nil {
				lib.Warning("link was closed", connection.peername, "error:", err)
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
		lib.Warning("conn is not a *distConnection type")
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
	connection.flusher.Stop()
	connection.cancelContext()
}

// node.Connection interface implementation

func (dc *distConnection) Send(from gen.Process, to etf.Pid, message etf.Term) error {
	msg := sendMessages.Get().(*sendMessage)

	msg.control = etf.Tuple{distProtoSEND, etf.Atom(""), to}
	msg.payload = message
	msg.compression = from.Compression()
	msg.compressionLevel = from.CompressionLevel()
	msg.compressionThreshold = from.CompressionThreshold()

	return dc.send(string(to.Node), to.Creation, msg)
}
func (dc *distConnection) SendReg(from gen.Process, to gen.ProcessID, message etf.Term) error {
	msg := sendMessages.Get().(*sendMessage)

	msg.control = etf.Tuple{distProtoREG_SEND, from.Self(), etf.Atom(""), etf.Atom(to.Name)}
	msg.payload = message
	msg.compression = from.Compression()
	msg.compressionLevel = from.CompressionLevel()
	msg.compressionThreshold = from.CompressionThreshold()
	return dc.send(to.Node, 0, msg)
}
func (dc *distConnection) SendAlias(from gen.Process, to etf.Alias, message etf.Term) error {
	if dc.flags.EnableAlias == false {
		return lib.ErrUnsupported
	}

	msg := sendMessages.Get().(*sendMessage)

	msg.control = etf.Tuple{distProtoALIAS_SEND, from.Self(), to}
	msg.payload = message
	msg.compression = from.Compression()
	msg.compressionLevel = from.CompressionLevel()
	msg.compressionThreshold = from.CompressionThreshold()

	return dc.send(string(to.Node), to.Creation, msg)
}

func (dc *distConnection) Link(local etf.Pid, remote etf.Pid) error {
	dc.proxySessionsMutex.RLock()
	ps, isProxy := dc.proxySessionsByPeerName[string(remote.Node)]
	dc.proxySessionsMutex.RUnlock()
	if isProxy && ps.session.PeerFlags.EnableLink == false {
		return lib.ErrPeerUnsupported
	}
	msg := &sendMessage{
		control: etf.Tuple{distProtoLINK, local, remote},
	}
	return dc.send(string(remote.Node), remote.Creation, msg)
}
func (dc *distConnection) Unlink(local etf.Pid, remote etf.Pid) error {
	dc.proxySessionsMutex.RLock()
	ps, isProxy := dc.proxySessionsByPeerName[string(remote.Node)]
	dc.proxySessionsMutex.RUnlock()
	if isProxy && ps.session.PeerFlags.EnableLink == false {
		return lib.ErrPeerUnsupported
	}
	msg := &sendMessage{
		control: etf.Tuple{distProtoUNLINK, local, remote},
	}
	return dc.send(string(remote.Node), remote.Creation, msg)
}
func (dc *distConnection) LinkExit(to etf.Pid, terminated etf.Pid, reason string) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoEXIT, terminated, to, etf.Atom(reason)},
	}
	return dc.send(string(to.Node), 0, msg)
}

func (dc *distConnection) Monitor(local etf.Pid, remote etf.Pid, ref etf.Ref) error {
	dc.proxySessionsMutex.RLock()
	ps, isProxy := dc.proxySessionsByPeerName[string(remote.Node)]
	dc.proxySessionsMutex.RUnlock()
	if isProxy && ps.session.PeerFlags.EnableMonitor == false {
		return lib.ErrPeerUnsupported
	}
	msg := &sendMessage{
		control: etf.Tuple{distProtoMONITOR, local, remote, ref},
	}
	return dc.send(string(remote.Node), remote.Creation, msg)
}
func (dc *distConnection) MonitorReg(local etf.Pid, remote gen.ProcessID, ref etf.Ref) error {
	dc.proxySessionsMutex.RLock()
	ps, isProxy := dc.proxySessionsByPeerName[remote.Node]
	dc.proxySessionsMutex.RUnlock()
	if isProxy && ps.session.PeerFlags.EnableMonitor == false {
		return lib.ErrPeerUnsupported
	}
	msg := &sendMessage{
		control: etf.Tuple{distProtoMONITOR, local, etf.Atom(remote.Name), ref},
	}
	return dc.send(remote.Node, 0, msg)
}
func (dc *distConnection) Demonitor(local etf.Pid, remote etf.Pid, ref etf.Ref) error {
	dc.proxySessionsMutex.RLock()
	ps, isProxy := dc.proxySessionsByPeerName[string(remote.Node)]
	dc.proxySessionsMutex.RUnlock()
	if isProxy && ps.session.PeerFlags.EnableMonitor == false {
		return lib.ErrPeerUnsupported
	}
	msg := &sendMessage{
		control: etf.Tuple{distProtoDEMONITOR, local, remote, ref},
	}
	return dc.send(string(remote.Node), remote.Creation, msg)
}
func (dc *distConnection) DemonitorReg(local etf.Pid, remote gen.ProcessID, ref etf.Ref) error {
	dc.proxySessionsMutex.RLock()
	ps, isProxy := dc.proxySessionsByPeerName[remote.Node]
	dc.proxySessionsMutex.RUnlock()
	if isProxy && ps.session.PeerFlags.EnableMonitor == false {
		return lib.ErrPeerUnsupported
	}
	msg := &sendMessage{
		control: etf.Tuple{distProtoDEMONITOR, local, etf.Atom(remote.Name), ref},
	}
	return dc.send(remote.Node, 0, msg)
}
func (dc *distConnection) MonitorExitReg(to etf.Pid, terminated gen.ProcessID, reason string, ref etf.Ref) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoMONITOR_EXIT, etf.Atom(terminated.Name), to, ref, etf.Atom(reason)},
	}
	return dc.send(string(to.Node), to.Creation, msg)
}
func (dc *distConnection) MonitorExit(to etf.Pid, terminated etf.Pid, reason string, ref etf.Ref) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoMONITOR_EXIT, terminated, to, ref, etf.Atom(reason)},
	}
	return dc.send(string(to.Node), to.Creation, msg)
}

func (dc *distConnection) SpawnRequest(nodeName string, behaviorName string, request gen.RemoteSpawnRequest, args ...etf.Term) error {
	dc.proxySessionsMutex.RLock()
	ps, isProxy := dc.proxySessionsByPeerName[nodeName]
	dc.proxySessionsMutex.RUnlock()
	if isProxy {
		if ps.session.PeerFlags.EnableRemoteSpawn == false {
			return lib.ErrPeerUnsupported
		}
	} else {
		if dc.flags.EnableRemoteSpawn == false {
			return lib.ErrPeerUnsupported
		}
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
	return dc.send(nodeName, 0, msg)
}

func (dc *distConnection) SpawnReply(to etf.Pid, ref etf.Ref, pid etf.Pid) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoSPAWN_REPLY, ref, to, 0, pid},
	}
	return dc.send(string(to.Node), to.Creation, msg)
}

func (dc *distConnection) SpawnReplyError(to etf.Pid, ref etf.Ref, err error) error {
	msg := &sendMessage{
		control: etf.Tuple{distProtoSPAWN_REPLY, ref, to, 0, etf.Atom(err.Error())},
	}
	return dc.send(string(to.Node), to.Creation, msg)
}

func (dc *distConnection) ProxyConnectRequest(request node.ProxyConnectRequest) error {
	if dc.flags.EnableProxy == false {
		return lib.ErrPeerUnsupported
	}

	path := []etf.Atom{}
	for i := range request.Path {
		path = append(path, etf.Atom(request.Path[i]))
	}

	msg := &sendMessage{
		control: etf.Tuple{distProtoPROXY_CONNECT_REQUEST,
			request.ID,           // etf.Ref
			etf.Atom(request.To), // to node
			request.Digest,       //
			request.PublicKey,    // public key for the sending symmetric key
			proxyFlagsToUint64(request.Flags),
			request.Creation,
			request.Hop,
			path,
		},
	}
	return dc.send(dc.peername, 0, msg)
}

func (dc *distConnection) ProxyConnectReply(reply node.ProxyConnectReply) error {
	if dc.flags.EnableProxy == false {
		return lib.ErrPeerUnsupported
	}

	path := etf.List{}
	for i := range reply.Path {
		path = append(path, etf.Atom(reply.Path[i]))
	}

	msg := &sendMessage{
		control: etf.Tuple{distProtoPROXY_CONNECT_REPLY,
			reply.ID,           // etf.Ref
			etf.Atom(reply.To), // to node
			reply.Digest,       //
			reply.Cipher,       //
			proxyFlagsToUint64(reply.Flags),
			reply.Creation,
			reply.SessionID,
			path,
		},
	}

	return dc.send(dc.peername, 0, msg)
}

func (dc *distConnection) ProxyConnectCancel(err node.ProxyConnectCancel) error {
	if dc.flags.EnableProxy == false {
		return lib.ErrPeerUnsupported
	}

	path := etf.List{}
	for i := range err.Path {
		path = append(path, etf.Atom(err.Path[i]))
	}

	msg := &sendMessage{
		control: etf.Tuple{distProtoPROXY_CONNECT_CANCEL,
			err.ID,             // etf.Ref
			etf.Atom(err.From), // from node
			err.Reason,
			path,
		},
	}

	return dc.send(dc.peername, 0, msg)
}

func (dc *distConnection) ProxyDisconnect(disconnect node.ProxyDisconnect) error {
	if dc.flags.EnableProxy == false {
		return lib.ErrPeerUnsupported
	}

	msg := &sendMessage{
		control: etf.Tuple{distProtoPROXY_DISCONNECT,
			etf.Atom(disconnect.Node),
			etf.Atom(disconnect.Proxy),
			disconnect.SessionID,
			disconnect.Reason,
		},
	}

	return dc.send(dc.peername, 0, msg)
}

func (dc *distConnection) ProxyRegisterSession(session node.ProxySession) error {
	dc.proxySessionsMutex.Lock()
	defer dc.proxySessionsMutex.Unlock()
	_, exist := dc.proxySessionsByPeerName[session.PeerName]
	if exist {
		return lib.ErrProxySessionDuplicate
	}
	_, exist = dc.proxySessionsByID[session.ID]
	if exist {
		return lib.ErrProxySessionDuplicate
	}
	ps := proxySession{
		session: session,
		cache:   etf.NewAtomCache(),
		// every sender should have its own senderAtomCache in the proxy session
		senderCache: make([]map[etf.Atom]etf.CacheItem, len(dc.senders.sender)),
	}
	dc.proxySessionsByPeerName[session.PeerName] = ps
	dc.proxySessionsByID[session.ID] = ps
	return nil
}

func (dc *distConnection) ProxyUnregisterSession(id string) error {
	dc.proxySessionsMutex.Lock()
	defer dc.proxySessionsMutex.Unlock()
	ps, exist := dc.proxySessionsByID[id]
	if exist == false {
		return lib.ErrProxySessionUnknown
	}
	delete(dc.proxySessionsByPeerName, ps.session.PeerName)
	delete(dc.proxySessionsByID, ps.session.ID)
	return nil
}

func (dc *distConnection) ProxyPacket(packet *lib.Buffer) error {
	if dc.flags.EnableProxy == false {
		return lib.ErrPeerUnsupported
	}
	msg := &sendMessage{
		packet: packet,
	}
	return dc.send(dc.peername, 0, msg)
}

//
// internal
//

func (dc *distConnection) read(b *lib.Buffer, max int) (int, error) {
	// http://erlang.org/doc/apps/erts/erl_dist_protocol.html#protocol-between-connected-nodes
	expectingBytes := 4
	for {
		if b.Len() < expectingBytes {
			// if no data is received during the 4 * keepAlivePeriod the remote node
			// seems to be stuck.
			deadline := true
			if err := dc.conn.SetReadDeadline(time.Now().Add(4 * keepAlivePeriod)); err != nil {
				deadline = false
			}

			n, e := b.ReadDataFrom(dc.conn, max)
			if n == 0 {
				if err, ok := e.(net.Error); deadline && ok && err.Timeout() {
					lib.Warning("Node %q not responding. Drop connection", dc.peername)
				}
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
			if len(b.B) == 4 {
				b.Reset()
				continue
			}
			b.B = b.B[4:]
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

type distMessage struct {
	control etf.Term
	payload etf.Term
	proxy   *proxySession
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
		message, err := dc.decodePacket(b)

		if err == errMissingInCache {
			if b == missing.b && missing.c > 100 {
				lib.Warning("Disordered data at the link with %q. Close connection", dc.peername)
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
				lib.Warning("Mess at the link with %q. Close connection", dc.peername)
				dc.cancelContext()
				lib.ReleaseBuffer(b)
				return
			}
		}

		dChannel = deferrChannel

		if err != nil {
			lib.Warning("[%s] Malformed Dist proto at the link with %s: %s", dc.nodename, dc.peername, err)
			dc.cancelContext()
			lib.ReleaseBuffer(b)
			return
		}

		if message == nil {
			// fragment or proxy message
			continue
		}

		// handle message
		if err := dc.handleMessage(message); err != nil {
			if message.proxy == nil {
				lib.Warning("[%s] Malformed Control packet at the link with %s: %#v", dc.nodename, dc.peername, message.control)
				dc.cancelContext()
				lib.ReleaseBuffer(b)
				return
			}
			// drop proxy session
			lib.Warning("[%s] Malformed Control packet at the proxy link with %s: %#v", dc.nodename, message.proxy.session.PeerName, message.control)
			disconnect := node.ProxyDisconnect{
				Node:      dc.nodename,
				Proxy:     dc.nodename,
				SessionID: message.proxy.session.ID,
				Reason:    err.Error(),
			}
			// route it locally to unregister this session
			dc.router.RouteProxyDisconnect(dc, disconnect)
			// send it to the peer
			dc.ProxyDisconnect(disconnect)
		}

		atomic.AddUint64(&dc.stats.MessagesIn, 1)

		// we have to release this buffer
		lib.ReleaseBuffer(b)

	}
}

func (dc *distConnection) decodePacket(b *lib.Buffer) (*distMessage, error) {
	packet := b.B
	if len(packet) < 5 {
		return nil, fmt.Errorf("malformed packet")
	}

	// [:3] length
	switch packet[4] {
	case protoDist:
		// do not check the length. it was checked on the receiving this packet.
		control, payload, err := dc.decodeDist(packet[5:], nil)
		if control == nil {
			return nil, err
		}
		atomic.AddUint64(&dc.stats.BytesIn, uint64(b.Len()))

		message := &distMessage{control: control, payload: payload}
		return message, err

	case protoProxy:
		sessionID := string(packet[5:37])
		dc.proxySessionsMutex.RLock()
		ps, exist := dc.proxySessionsByID[sessionID]
		dc.proxySessionsMutex.RUnlock()
		if exist == false {
			// must be send further
			if err := dc.router.RouteProxy(dc, sessionID, b); err != nil {
				// drop proxy session
				disconnect := node.ProxyDisconnect{
					Node:      dc.nodename,
					Proxy:     dc.nodename,
					SessionID: sessionID,
					Reason:    err.Error(),
				}
				dc.ProxyDisconnect(disconnect)
				return nil, nil
			}
			atomic.AddUint64(&dc.stats.TransitBytesIn, uint64(b.Len()))
			return nil, nil
		}

		// this node is endpoint of this session
		packet = b.B[37:]
		control, payload, err := dc.decodeDist(packet, &ps)
		if err != nil {
			if err == errMissingInCache {
				// will be deferred.
				// 37 - 5
				// where:
				// 37 = packet len (4) + protoProxy (1) + session id (32)
				// reserving 5 bytes for: packet len(4) + protoDist (1)
				// we don't update packet len value. it was already validated
				// and will be ignored on the next dc.decodeDist call
				b.B = b.B[32:]
				b.B[4] = protoDist
				return nil, err
			}
			// drop this proxy session. send back ProxyDisconnect
			disconnect := node.ProxyDisconnect{
				Node:      dc.nodename,
				Proxy:     dc.nodename,
				SessionID: sessionID,
				Reason:    err.Error(),
			}
			dc.router.RouteProxyDisconnect(dc, disconnect)
			dc.ProxyDisconnect(disconnect)
			return nil, nil
		}

		atomic.AddUint64(&dc.stats.BytesIn, uint64(b.Len()))

		if control == nil {
			return nil, nil
		}
		message := &distMessage{control: control, payload: payload, proxy: &ps}
		return message, nil

	case protoProxyX:
		atomic.AddUint64(&dc.stats.BytesIn, uint64(b.Len()))
		sessionID := string(packet[5:37])
		dc.proxySessionsMutex.RLock()
		ps, exist := dc.proxySessionsByID[sessionID]
		dc.proxySessionsMutex.RUnlock()
		if exist == false {
			// must be send further
			if err := dc.router.RouteProxy(dc, sessionID, b); err != nil {
				// drop proxy session
				disconnect := node.ProxyDisconnect{
					Node:      dc.nodename,
					Proxy:     dc.nodename,
					SessionID: sessionID,
					Reason:    err.Error(),
				}
				dc.ProxyDisconnect(disconnect)
				return nil, nil
			}
			atomic.AddUint64(&dc.stats.TransitBytesIn, uint64(b.Len()))
			return nil, nil
		}

		packet = b.B[37:]
		if (len(packet) % aes.BlockSize) != 0 {
			// drop this proxy session.
			disconnect := node.ProxyDisconnect{
				Node:      dc.nodename,
				Proxy:     dc.nodename,
				SessionID: sessionID,
				Reason:    "wrong blocksize of the encrypted message",
			}
			dc.router.RouteProxyDisconnect(dc, disconnect)
			dc.ProxyDisconnect(disconnect)
			return nil, nil
		}
		atomic.AddUint64(&dc.stats.BytesIn, uint64(b.Len()))

		iv := packet[:aes.BlockSize]
		msg := packet[aes.BlockSize:]
		cfb := cipher.NewCFBDecrypter(ps.session.Block, iv)
		cfb.XORKeyStream(msg, msg)

		// check padding
		length := len(msg)
		unpadding := int(msg[length-1])
		if unpadding > length {
			// drop this proxy session.
			disconnect := node.ProxyDisconnect{
				Node:      dc.nodename,
				Proxy:     dc.nodename,
				SessionID: sessionID,
				Reason:    "wrong padding of the encrypted message",
			}
			dc.router.RouteProxyDisconnect(dc, disconnect)
			dc.ProxyDisconnect(disconnect)
			return nil, nil
		}
		packet = msg[:(length - unpadding)]
		control, payload, err := dc.decodeDist(packet, &ps)
		if err != nil {
			if err == errMissingInCache {
				// will be deferred
				b.B = b.B[32+aes.BlockSize:]
				b.B[4] = protoDist
				return nil, err
			}
			// drop this proxy session.
			disconnect := node.ProxyDisconnect{
				Node:      dc.nodename,
				Proxy:     dc.nodename,
				SessionID: sessionID,
				Reason:    err.Error(),
			}
			dc.router.RouteProxyDisconnect(dc, disconnect)
			dc.ProxyDisconnect(disconnect)
			return nil, nil
		}
		atomic.AddUint64(&dc.stats.BytesIn, uint64(b.Len()))
		if control == nil {
			return nil, nil
		}
		message := &distMessage{control: control, payload: payload, proxy: &ps}
		return message, nil

	default:
		// unknown proto
		return nil, fmt.Errorf("unknown/unsupported proto")
	}

}

func (dc *distConnection) decodeDist(packet []byte, proxy *proxySession) (etf.Term, etf.Term, error) {
	switch packet[0] {
	case protoDistMessage:
		var control, payload etf.Term
		var err error
		var cache []etf.Atom

		cache, packet, err = dc.decodeDistHeaderAtomCache(packet[1:], proxy)
		if err != nil {
			return nil, nil, err
		}

		decodeOptions := etf.DecodeOptions{
			AtomMapping:   dc.mapping,
			FlagBigPidRef: dc.flags.EnableBigPidRef,
		}
		if proxy != nil {
			decodeOptions.FlagBigPidRef = true
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
		payload, packet, err = etf.Decode(packet, cache, decodeOptions)
		if err != nil {
			return nil, nil, err
		}

		if len(packet) != 0 {
			return nil, nil, fmt.Errorf("packet has extra %d byte(s)", len(packet))
		}

		return control, payload, nil

	case protoDistMessageZ:
		var control, payload etf.Term
		var err error
		var cache []etf.Atom
		var zReader *gzip.Reader
		var total int
		// compressed protoDistMessage

		cache, packet, err = dc.decodeDistHeaderAtomCache(packet[1:], proxy)
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
		if proxy != nil {
			decodeOptions.FlagBigPidRef = true
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
		payload, packet, err = etf.Decode(packet, cache, decodeOptions)
		if err != nil {
			return nil, nil, err
		}

		if len(packet) != 0 {
			return nil, nil, fmt.Errorf("packet has extra %d byte(s)", len(packet))
		}

		return control, payload, nil

	case protoDistFragment1, protoDistFragmentN, protoDistFragment1Z, protoDistFragmentNZ:
		if len(packet) < 18 {
			return nil, nil, fmt.Errorf("malformed fragment (too small)")
		}

		if assembled, err := dc.decodeFragment(packet, proxy); assembled != nil {
			if err != nil {
				return nil, nil, err
			}
			control, payload, err := dc.decodeDist(assembled.B, nil)
			lib.ReleaseBuffer(assembled)
			return control, payload, err
		} else {
			if err != nil {
				return nil, nil, err
			}
		}
		return nil, nil, nil
	}

	return nil, nil, fmt.Errorf("unknown packet type %d", packet[0])
}

func (dc *distConnection) handleMessage(message *distMessage) (err error) {
	defer func() {
		if lib.CatchPanic() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%s", r)
			}
		}
	}()

	switch t := message.control.(type) {
	case etf.Tuple:
		switch act := t.Element(1).(type) {
		case int:
			switch act {
			case distProtoREG_SEND:
				// {6, FromPid, Unused, ToName}
				lib.Log("[%s] CONTROL REG_SEND [from %s]: %#v", dc.nodename, dc.peername, message.control)
				to := gen.ProcessID{
					Node: dc.nodename,
					Name: string(t.Element(4).(etf.Atom)),
				}
				dc.router.RouteSendReg(t.Element(2).(etf.Pid), to, message.payload)
				return nil

			case distProtoSEND:
				// {2, Unused, ToPid}
				// SEND has no sender pid
				lib.Log("[%s] CONTROL SEND [from %s]: %#v", dc.nodename, dc.peername, message.control)
				dc.router.RouteSend(etf.Pid{}, t.Element(3).(etf.Pid), message.payload)
				return nil

			case distProtoLINK:
				// {1, FromPid, ToPid}
				lib.Log("[%s] CONTROL LINK [from %s]: %#v", dc.nodename, dc.peername, message.control)
				if message.proxy != nil && message.proxy.session.NodeFlags.EnableLink == false {
					// we didn't allow this feature. proxy session will be closed due to
					// this violation of the contract
					return lib.ErrPeerUnsupported
				}
				dc.router.RouteLink(t.Element(2).(etf.Pid), t.Element(3).(etf.Pid))
				return nil

			case distProtoUNLINK:
				// {4, FromPid, ToPid}
				lib.Log("[%s] CONTROL UNLINK [from %s]: %#v", dc.nodename, dc.peername, message.control)
				if message.proxy != nil && message.proxy.session.NodeFlags.EnableLink == false {
					// we didn't allow this feature. proxy session will be closed due to
					// this violation of the contract
					return lib.ErrPeerUnsupported
				}
				dc.router.RouteUnlink(t.Element(2).(etf.Pid), t.Element(3).(etf.Pid))
				return nil

			case distProtoNODE_LINK:
				lib.Log("[%s] CONTROL NODE_LINK [from %s]: %#v", dc.nodename, dc.peername, message.control)
				return nil

			case distProtoEXIT:
				// {3, FromPid, ToPid, Reason}
				lib.Log("[%s] CONTROL EXIT [from %s]: %#v", dc.nodename, dc.peername, message.control)
				terminated := t.Element(2).(etf.Pid)
				to := t.Element(3).(etf.Pid)
				reason := fmt.Sprint(t.Element(4))
				dc.router.RouteExit(to, terminated, string(reason))
				return nil

			case distProtoEXIT2:
				lib.Log("[%s] CONTROL EXIT2 [from %s]: %#v", dc.nodename, dc.peername, message.control)
				return nil

			case distProtoMONITOR:
				// {19, FromPid, ToProc, Ref}, where FromPid = monitoring process
				// and ToProc = monitored process pid or name (atom)
				lib.Log("[%s] CONTROL MONITOR [from %s]: %#v", dc.nodename, dc.peername, message.control)
				if message.proxy != nil && message.proxy.session.NodeFlags.EnableMonitor == false {
					// we didn't allow this feature. proxy session will be closed due to
					// this violation of the contract
					return lib.ErrPeerUnsupported
				}

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
				lib.Log("[%s] CONTROL DEMONITOR [from %s]: %#v", dc.nodename, dc.peername, message.control)
				if message.proxy != nil && message.proxy.session.NodeFlags.EnableMonitor == false {
					// we didn't allow this feature. proxy session will be closed due to
					// this violation of the contract
					return lib.ErrPeerUnsupported
				}
				ref := t.Element(4).(etf.Ref)
				fromPid := t.Element(2).(etf.Pid)
				dc.router.RouteDemonitor(fromPid, ref)
				return nil

			case distProtoMONITOR_EXIT:
				// {21, FromProc, ToPid, Ref, Reason}, where FromProc = monitored process
				// pid or name (atom), ToPid = monitoring process, and Reason = exit reason for the monitored process
				lib.Log("[%s] CONTROL MONITOR_EXIT [from %s]: %#v", dc.nodename, dc.peername, message.control)
				reason := fmt.Sprint(t.Element(5))
				ref := t.Element(4).(etf.Ref)
				switch terminated := t.Element(2).(type) {
				case etf.Pid:
					dc.router.RouteMonitorExit(terminated, reason, ref)
					return nil
				case etf.Atom:
					processID := gen.ProcessID{Name: string(terminated), Node: dc.peername}
					if message.proxy != nil {
						processID.Node = message.proxy.session.PeerName
					}
					dc.router.RouteMonitorExitReg(processID, reason, ref)
					return nil
				}
				return fmt.Errorf("malformed monitor exit message")

			// Not implemented yet, just stubs. TODO.
			case distProtoSEND_SENDER:
				lib.Log("[%s] CONTROL SEND_SENDER unsupported [from %s]: %#v", dc.nodename, dc.peername, message.control)
				return nil
			case distProtoPAYLOAD_EXIT:
				lib.Log("[%s] CONTROL PAYLOAD_EXIT unsupported [from %s]: %#v", dc.nodename, dc.peername, message.control)
				return nil
			case distProtoPAYLOAD_EXIT2:
				lib.Log("[%s] CONTROL PAYLOAD_EXIT2 unsupported [from %s]: %#v", dc.nodename, dc.peername, message.control)
				return nil
			case distProtoPAYLOAD_MONITOR_P_EXIT:
				lib.Log("[%s] CONTROL PAYLOAD_MONITOR_P_EXIT unsupported [from %s]: %#v", dc.nodename, dc.peername, message.control)
				return nil

			// alias support
			case distProtoALIAS_SEND:
				// {33, FromPid, Alias}
				lib.Log("[%s] CONTROL ALIAS_SEND [from %s]: %#v", dc.nodename, dc.peername, message.control)
				alias := etf.Alias(t.Element(3).(etf.Ref))
				dc.router.RouteSendAlias(t.Element(2).(etf.Pid), alias, message.payload)
				return nil

			case distProtoSPAWN_REQUEST:
				// {29, ReqId, From, GroupLeader, {Module, Function, Arity}, OptList}
				lib.Log("[%s] CONTROL SPAWN_REQUEST [from %s]: %#v", dc.nodename, dc.peername, message.control)
				if message.proxy != nil && message.proxy.session.NodeFlags.EnableRemoteSpawn == false {
					// we didn't allow this feature. proxy session will be closed due to
					// this violation of the contract
					return lib.ErrPeerUnsupported
				}
				registerName := ""
				for _, option := range t.Element(6).(etf.List) {
					name, ok := option.(etf.Tuple)
					if ok || len(name) == 2 {
						switch name.Element(1) {
						case etf.Atom("name"):
							registerName = string(name.Element(2).(etf.Atom))
						}
					}
				}

				from := t.Element(3).(etf.Pid)
				ref := t.Element(2).(etf.Ref)

				mfa := t.Element(5).(etf.Tuple)
				module := mfa.Element(1).(etf.Atom)
				function := mfa.Element(2).(etf.Atom)
				var args etf.List
				if str, ok := message.payload.(string); !ok {
					args, _ = message.payload.(etf.List)
				} else {
					// stupid Erlang's strings :). [1,2,3,4,5] sends as a string.
					// args can't be anything but etf.List.
					for i := range []byte(str) {
						args = append(args, str[i])
					}
				}

				if registerName == "" {
					if module == etf.Atom("erpc") {
						registerName = "erpc"
					} else {
						return fmt.Errorf("malformed spawn request")
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
				lib.Log("[%s] CONTROL SPAWN_REPLY [from %s]: %#v", dc.nodename, dc.peername, message.control)
				ref := t.Element(2).(etf.Ref)
				to := t.Element(3).(etf.Pid)
				dc.router.RouteSpawnReply(to, ref, t.Element(5))
				return nil

			case distProtoPROXY_CONNECT_REQUEST:
				// {101, ID, To, Digest, PublicKey, Flags, Hop, Path}
				lib.Log("[%s] PROXY CONNECT REQUEST [from %s]: %#v", dc.nodename, dc.peername, message.control)
				request := node.ProxyConnectRequest{
					ID:        t.Element(2).(etf.Ref),
					To:        string(t.Element(3).(etf.Atom)),
					Digest:    t.Element(4).([]byte),
					PublicKey: t.Element(5).([]byte),
					// FIXME it will be int64 after using more than 32 flags
					Flags:    proxyFlagsFromUint64(uint64(t.Element(6).(int))),
					Creation: uint32(t.Element(7).(int64)),
					Hop:      t.Element(8).(int),
				}
				for _, p := range t.Element(9).(etf.List) {
					request.Path = append(request.Path, string(p.(etf.Atom)))
				}
				if err := dc.router.RouteProxyConnectRequest(dc, request); err != nil {
					errReply := node.ProxyConnectCancel{
						ID:     request.ID,
						From:   dc.nodename,
						Reason: err.Error(),
						Path:   request.Path[1:],
					}
					dc.ProxyConnectCancel(errReply)
				}
				return nil

			case distProtoPROXY_CONNECT_REPLY:
				// {102, ID, To, Digest, Cipher, Flags, SessionID, Path}
				lib.Log("[%s] PROXY CONNECT REPLY [from %s]: %#v", dc.nodename, dc.peername, message.control)
				connectReply := node.ProxyConnectReply{
					ID:     t.Element(2).(etf.Ref),
					To:     string(t.Element(3).(etf.Atom)),
					Digest: t.Element(4).([]byte),
					Cipher: t.Element(5).([]byte),
					// FIXME it will be int64 after using more than 32 flags
					Flags:     proxyFlagsFromUint64(uint64(t.Element(6).(int))),
					Creation:  uint32(t.Element(7).(int64)),
					SessionID: t.Element(8).(string),
				}
				for _, p := range t.Element(9).(etf.List) {
					connectReply.Path = append(connectReply.Path, string(p.(etf.Atom)))
				}
				if err := dc.router.RouteProxyConnectReply(dc, connectReply); err != nil {
					lib.Log("[%s] PROXY CONNECT REPLY error %s (message: %#v)", dc.nodename, err, connectReply)
					// send disconnect to clean up this session all the way to the
					// destination node
					disconnect := node.ProxyDisconnect{
						Node:      dc.nodename,
						Proxy:     dc.nodename,
						SessionID: connectReply.SessionID,
						Reason:    err.Error(),
					}
					dc.ProxyDisconnect(disconnect)
					if err == lib.ErrNoRoute {
						return nil
					}

					// send cancel message to the source node
					cancel := node.ProxyConnectCancel{
						ID:     connectReply.ID,
						From:   dc.nodename,
						Reason: err.Error(),
						Path:   connectReply.Path,
					}
					dc.router.RouteProxyConnectCancel(dc, cancel)
				}

				return nil

			case distProtoPROXY_CONNECT_CANCEL:
				lib.Log("[%s] PROXY CONNECT CANCEL [from %s]: %#v", dc.nodename, dc.peername, message.control)
				connectError := node.ProxyConnectCancel{
					ID:     t.Element(2).(etf.Ref),
					From:   string(t.Element(3).(etf.Atom)),
					Reason: t.Element(4).(string),
				}
				for _, p := range t.Element(5).(etf.List) {
					connectError.Path = append(connectError.Path, string(p.(etf.Atom)))
				}
				dc.router.RouteProxyConnectCancel(dc, connectError)
				return nil

			case distProtoPROXY_DISCONNECT:
				// {104, Node, Proxy, SessionID, Reason}
				lib.Log("[%s] PROXY DISCONNECT [from %s]: %#v", dc.nodename, dc.peername, message.control)
				proxyDisconnect := node.ProxyDisconnect{
					Node:      string(t.Element(2).(etf.Atom)),
					Proxy:     string(t.Element(3).(etf.Atom)),
					SessionID: t.Element(4).(string),
					Reason:    t.Element(5).(string),
				}
				dc.router.RouteProxyDisconnect(dc, proxyDisconnect)
				return nil

			default:
				lib.Log("[%s] CONTROL unknown command [from %s]: %#v", dc.nodename, dc.peername, message.control)
				return fmt.Errorf("unknown control command %#v", message.control)
			}
		}
	}

	return fmt.Errorf("unsupported control message %#v", message.control)
}

func (dc *distConnection) decodeFragment(packet []byte, proxy *proxySession) (*lib.Buffer, error) {
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
		_, _, err = dc.decodeDistHeaderAtomCache(packet[17:], proxy)
		if err != nil {
			return nil, err
		}
		first = true
	case protoDistFragment1Z:
		_, _, err = dc.decodeDistHeaderAtomCache(packet[17:], proxy)
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

func (dc *distConnection) decodeDistHeaderAtomCache(packet []byte, proxy *proxySession) ([]etf.Atom, []byte, error) {
	var err error
	// all the details are here https://erlang.org/doc/apps/erts/erl_ext_dist.html#normal-distribution-header

	// number of atom references are present in package
	references := int(packet[0])
	if references == 0 {
		return nil, packet[1:], nil
	}

	cache := dc.cache.In
	if proxy != nil {
		cache = proxy.cache.In
	}
	cached := make([]etf.Atom, references)
	flagsLen := references/2 + 1
	if len(packet) < 1+flagsLen {
		// malformed
		return nil, nil, errMalformed
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
			return nil, nil, errMalformed
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
				return nil, nil, errMalformed
			}
			atom := etf.Atom(packet[:atomLen])
			// store in temporary cache for decoding
			cached[i] = atom

			// store in link' cache
			cache.Atoms[idx] = &atom
			packet = packet[atomLen:]
			continue
		}

		c := cache.Atoms[idx]
		if c == nil {
			packet = packet[1:]
			// decode the rest of this cache but set return err = errMissingInCache
			err = errMissingInCache
			continue
		}
		cached[i] = *c
		packet = packet[1:]
	}

	return cached, packet, err
}

func (dc *distConnection) encodeDistHeaderAtomCache(b *lib.Buffer,
	senderAtomCache map[etf.Atom]etf.CacheItem,
	encodingAtomCache *etf.EncodingAtomCache) {

	n := encodingAtomCache.Len()
	b.AppendByte(byte(n)) // write NumberOfAtomCache
	if n == 0 {
		return
	}

	startPosition := len(b.B)
	lenFlags := n/2 + 1
	flags := b.Extend(lenFlags)
	flags[lenFlags-1] = 0 // clear last byte to make sure we have valid LongAtom flag

	for i := 0; i < len(encodingAtomCache.L); i++ {
		// clean internal name cache
		encodingAtomCache.Delete(encodingAtomCache.L[i].Name)

		shift := uint((i & 0x01) * 4)
		idxReference := byte(encodingAtomCache.L[i].ID >> 8) // SegmentIndex
		idxInternal := byte(encodingAtomCache.L[i].ID & 255) // InternalSegmentIndex

		cachedItem := senderAtomCache[encodingAtomCache.L[i].Name]
		if !cachedItem.Encoded {
			idxReference |= 8 // set NewCacheEntryFlag
		}

		// the 'flags' slice could be changed if b.B was reallocated during the encoding atoms
		flags = b.B[startPosition : startPosition+lenFlags]
		// clean it up before reuse
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
		senderAtomCache[encodingAtomCache.L[i].Name] = cachedItem
	}

	if encodingAtomCache.HasLongAtom {
		shift := uint((n & 0x01) * 4)
		flags = b.B[startPosition : startPosition+lenFlags]
		flags[lenFlags-1] |= 1 << shift // set LongAtom = 1
	}
}

func (dc *distConnection) sender(sender_id int, send <-chan *sendMessage, options node.ProtoOptions, peerFlags node.Flags) {
	var lenMessage, lenAtomCache, lenPacket, startDataPosition int
	var atomCacheBuffer, packetBuffer *lib.Buffer
	var err error
	var compressed bool
	var cacheEnabled, fragmentationEnabled, compressionEnabled, encryptionEnabled bool

	// cancel connection context if something went wrong
	// it will cause closing connection with stopping all
	// goroutines around this connection
	defer dc.cancelContext()

	// Header atom cache is encoded right after the control/message encoding process
	// but should be stored as a first item in the packet.
	// Thats why we do reserve some space for it in order to get rid
	// of reallocation packetBuffer data
	reserveHeaderAtomCache := 8192

	// atom cache of this sender
	senderAtomCache := make(map[etf.Atom]etf.CacheItem)
	// atom cache of this encoding
	encodingAtomCache := etf.TakeEncodingAtomCache()
	defer etf.ReleaseEncodingAtomCache(encodingAtomCache)

	encrypt := func(data []byte, sessionID string, block cipher.Block) *lib.Buffer {
		l := len(data)
		padding := aes.BlockSize - l%aes.BlockSize
		padtext := bytes.Repeat([]byte{byte(padding)}, padding)
		data = append(data, padtext...)
		l = len(data)

		// take another buffer for encrypted message
		xBuffer := lib.TakeBuffer()
		// 4 (packet len) + 1 (protoProxyX) + 32 (sessionID) + aes.BlockSize + l
		xBuffer.Allocate(4 + 1 + 32 + aes.BlockSize + l)

		binary.BigEndian.PutUint32(xBuffer.B, uint32(xBuffer.Len()-4))
		xBuffer.B[4] = protoProxyX
		copy(xBuffer.B[5:], sessionID)
		iv := xBuffer.B[4+1+32 : 4+1+32+aes.BlockSize]
		if _, err := io.ReadFull(crand.Reader, iv); err != nil {
			lib.ReleaseBuffer(xBuffer)
			return nil
		}
		cfb := cipher.NewCFBEncrypter(block, iv)
		cfb.XORKeyStream(xBuffer.B[4+1+32+aes.BlockSize:], data)
		return xBuffer
	}

	message := &sendMessage{}
	encodingOptions := etf.EncodeOptions{
		EncodingAtomCache: encodingAtomCache,
		AtomMapping:       dc.mapping,
		NodeName:          dc.nodename,
		PeerName:          dc.peername,
	}

	for {
		// clean up and get back message struct to the pool
		message.packet = nil
		message.control = nil
		message.payload = nil
		message.compression = false
		message.proxy = nil
		sendMessages.Put(message)

		// waiting for the next message
		message = <-send

		if message == nil {
			// channel was closed
			return
		}

		if message.packet != nil {
			// transit proxy message
			bytesOut, err := dc.flusher.Write(message.packet.B)
			if err != nil {
				return
			}
			atomic.AddUint64(&dc.stats.TransitBytesOut, uint64(bytesOut))
			lib.ReleaseBuffer(message.packet)
			continue
		}

		atomic.AddUint64(&dc.stats.MessagesOut, 1)

		packetBuffer = lib.TakeBuffer()
		lenMessage, lenAtomCache, lenPacket = 0, 0, 0
		startDataPosition = reserveHeaderAtomCache

		// do reserve for the header 8K, should be enough
		packetBuffer.Allocate(reserveHeaderAtomCache)

		// compression feature is always available for the proxy connection
		// check whether compress is enabled for the peer and for this message
		compressed = false
		compressionEnabled = false
		if message.compression {
			if message.proxy != nil || peerFlags.EnableCompression {
				compressionEnabled = true
			}
		}

		cacheEnabled = false
		// atom cache feature is always available for the proxy connection
		if message.proxy != nil || peerFlags.EnableHeaderAtomCache {
			cacheEnabled = true
			encodingAtomCache.Reset()
		}

		// fragmentation feature is always available for the proxy connection
		fragmentationEnabled = false
		if options.FragmentationUnit > 0 {
			if message.proxy != nil || peerFlags.EnableFragmentation {
				fragmentationEnabled = true
			}
		}

		// encryption feature is only available for the proxy connection
		encryptionEnabled = false
		if message.proxy != nil && message.proxy.session.PeerFlags.EnableEncryption {
			encryptionEnabled = true
		}

		if message.proxy == nil {
			// use connection atom cache
			encodingOptions.AtomCache = dc.cache.Out
			encodingOptions.SenderAtomCache = senderAtomCache
			// use connection flags
			encodingOptions.FlagBigCreation = peerFlags.EnableBigCreation
			encodingOptions.FlagBigPidRef = peerFlags.EnableBigPidRef

		} else {
			// use proxy connection atom cache
			encodingOptions.AtomCache = message.proxy.cache.Out
			if message.proxy.senderCache[sender_id] == nil {
				message.proxy.senderCache[sender_id] = make(map[etf.Atom]etf.CacheItem)
			}
			encodingOptions.SenderAtomCache = message.proxy.senderCache[sender_id]
			// these flags are always enabled for the proxy connection
			encodingOptions.FlagBigCreation = true
			encodingOptions.FlagBigPidRef = true
		}

		// We could use gzip writer for the encoder, but we don't know
		// the actual size of the control/payload. For small data, gzipping
		// is getting extremely inefficient. That's why it is cheaper to
		// encode control/payload first and then decide whether to compress it
		// according to a threshold value.

		// encode Control
		err = etf.Encode(message.control, packetBuffer, encodingOptions)
		if err != nil {
			lib.Warning("can not encode control message: %s", err)
			lib.ReleaseBuffer(packetBuffer)
			continue
		}

		// encode Message if present
		if message.payload != nil {
			err = etf.Encode(message.payload, packetBuffer, encodingOptions)
			if err != nil {
				lib.Warning("can not encode payload message: %s", err)
				lib.ReleaseBuffer(packetBuffer)
				continue
			}

		}
		lenMessage = packetBuffer.Len() - reserveHeaderAtomCache

		if compressionEnabled && packetBuffer.Len() > (reserveHeaderAtomCache+message.compressionThreshold) {
			var zWriter *gzip.Writer

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
			atomCacheBuffer.Allocate(1024)
			dc.encodeDistHeaderAtomCache(atomCacheBuffer, encodingOptions.SenderAtomCache, encodingAtomCache)

			lenAtomCache = atomCacheBuffer.Len() - 1024
			if lenAtomCache > reserveHeaderAtomCache-1024 {
				// we got huge atom cache
				atomCacheBuffer.Append(packetBuffer.B[startDataPosition:])
				startDataPosition = 1024
				lib.ReleaseBuffer(packetBuffer)
				packetBuffer = atomCacheBuffer
			} else {
				startDataPosition -= lenAtomCache
				copy(packetBuffer.B[startDataPosition:], atomCacheBuffer.B[1024:])
				lib.ReleaseBuffer(atomCacheBuffer)
			}

		} else {
			lenAtomCache = 1
			startDataPosition -= lenAtomCache
			packetBuffer.B[startDataPosition] = byte(0)
		}

		for {
			// 4 (packet len) + 1 (dist header: 131) + 1 (dist header: protoDistMessage[Z]) + lenAtomCache
			lenPacket = 1 + 1 + lenAtomCache + lenMessage
			if !fragmentationEnabled || lenMessage < options.FragmentationUnit {
				// send as a single packet
				startDataPosition -= 1
				if compressed {
					packetBuffer.B[startDataPosition] = protoDistMessageZ // 200
				} else {
					packetBuffer.B[startDataPosition] = protoDistMessage // 68
				}

				if message.proxy == nil {
					// 4 (packet len) + 1 (protoDist)
					startDataPosition -= 4 + 1

					binary.BigEndian.PutUint32(packetBuffer.B[startDataPosition:], uint32(lenPacket))
					packetBuffer.B[startDataPosition+4] = protoDist // 131

					bytesOut, err := dc.flusher.Write(packetBuffer.B[startDataPosition:])
					if err != nil {
						return
					}
					atomic.AddUint64(&dc.stats.BytesOut, uint64(bytesOut))
					break
				}

				// proxy message.
				if encryptionEnabled == false {
					// no encryption
					// 4 (packet len) + protoProxy + sessionID
					startDataPosition -= 1 + 4 + 32
					l := len(packetBuffer.B[startDataPosition:]) - 4
					binary.BigEndian.PutUint32(packetBuffer.B[startDataPosition:], uint32(l))
					packetBuffer.B[startDataPosition+4] = protoProxy
					copy(packetBuffer.B[startDataPosition+5:], message.proxy.session.ID)
					bytesOut, err := dc.flusher.Write(packetBuffer.B[startDataPosition:])
					if err != nil {
						return
					}
					atomic.AddUint64(&dc.stats.BytesOut, uint64(bytesOut))
					break
				}

				// send encrypted proxy message
				xBuffer := encrypt(packetBuffer.B[startDataPosition:],
					message.proxy.session.ID, message.proxy.session.Block)
				if xBuffer == nil {
					// can't encrypt message
					return
				}
				bytesOut, err := dc.flusher.Write(xBuffer.B)
				if err != nil {
					return
				}
				atomic.AddUint64(&dc.stats.BytesOut, uint64(bytesOut))
				lib.ReleaseBuffer(xBuffer)
				break
			}

			// Message should be fragmented

			// https://erlang.org/doc/apps/erts/erl_ext_dist.html#distribution-header-for-fragmented-messages
			// "The entire atom cache and control message has to be part of the starting fragment"

			sequenceID := uint64(atomic.AddInt64(&dc.sequenceID, 1))
			numFragments := lenMessage/options.FragmentationUnit + 1

			// 1 (dist header: 131) + 1 (dist header: protoDistFragment) + 8 (sequenceID) + 8 (fragmentID) + ...
			lenPacket = 1 + 1 + 8 + 8 + lenAtomCache + options.FragmentationUnit

			// 4 (packet len) + 1 (dist header: 131) + 1 (dist header: protoDistFragment[Z]) + 8 (sequenceID) + 8 (fragmentID)
			startDataPosition -= 22

			if compressed {
				packetBuffer.B[startDataPosition+5] = protoDistFragment1Z // 201
			} else {
				packetBuffer.B[startDataPosition+5] = protoDistFragment1 // 69
			}

			binary.BigEndian.PutUint64(packetBuffer.B[startDataPosition+6:], uint64(sequenceID))
			binary.BigEndian.PutUint64(packetBuffer.B[startDataPosition+14:], uint64(numFragments))

			if message.proxy == nil {
				binary.BigEndian.PutUint32(packetBuffer.B[startDataPosition:], uint32(lenPacket))
				packetBuffer.B[startDataPosition+4] = protoDist // 131
				bytesOut, err := dc.flusher.Write(packetBuffer.B[startDataPosition : startDataPosition+4+lenPacket])
				if err != nil {
					return
				}
				atomic.AddUint64(&dc.stats.BytesOut, uint64(bytesOut))
			} else {
				// proxy message
				if encryptionEnabled == false {
					// send proxy message
					// shift left on 32 bytes for the session id
					binary.BigEndian.PutUint32(packetBuffer.B[startDataPosition-32:], uint32(lenPacket+32))
					packetBuffer.B[startDataPosition-32+4] = protoProxy // 141
					copy(packetBuffer.B[startDataPosition-32+5:], message.proxy.session.ID)
					bytesOut, err := dc.flusher.Write(packetBuffer.B[startDataPosition-32 : startDataPosition+4+lenPacket])
					if err != nil {
						return
					}
					atomic.AddUint64(&dc.stats.BytesOut, uint64(bytesOut))

				} else {
					// send encrypted proxy message
					// encryption makes padding (up to aes.BlockSize = 16 bytes) so we should keep the data
					tail16 := [16]byte{}
					n := copy(tail16[:], packetBuffer.B[startDataPosition+4+lenPacket:])
					xBuffer := encrypt(packetBuffer.B[startDataPosition+5:startDataPosition+4+lenPacket],
						message.proxy.session.ID, message.proxy.session.Block)
					if xBuffer == nil {
						// can't encrypt message
						return
					}
					bytesOut, err := dc.flusher.Write(xBuffer.B)
					if err != nil {
						return
					}
					atomic.AddUint64(&dc.stats.BytesOut, uint64(bytesOut))

					// resore tail
					copy(packetBuffer.B[startDataPosition+4+lenPacket:], tail16[:n])
					lib.ReleaseBuffer(xBuffer)
				}
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

			if compressed {
				packetBuffer.B[startDataPosition+5] = protoDistFragmentNZ // 202
			} else {
				packetBuffer.B[startDataPosition+5] = protoDistFragmentN // 70
			}

			binary.BigEndian.PutUint64(packetBuffer.B[startDataPosition+6:], uint64(sequenceID))
			binary.BigEndian.PutUint64(packetBuffer.B[startDataPosition+14:], uint64(numFragments))
			if message.proxy == nil {
				// send fragment
				binary.BigEndian.PutUint32(packetBuffer.B[startDataPosition:], uint32(lenPacket))
				packetBuffer.B[startDataPosition+4] = protoDist // 131
				bytesOut, err := dc.flusher.Write(packetBuffer.B[startDataPosition : startDataPosition+4+lenPacket])
				if err != nil {
					return
				}
				atomic.AddUint64(&dc.stats.BytesOut, uint64(bytesOut))
			} else {
				// wrap it as a proxy message
				if encryptionEnabled == false {
					binary.BigEndian.PutUint32(packetBuffer.B[startDataPosition-32:], uint32(lenPacket+32))
					packetBuffer.B[startDataPosition-32+4] = protoProxy // 141
					copy(packetBuffer.B[startDataPosition-32+5:], message.proxy.session.ID)
					bytesOut, err := dc.flusher.Write(packetBuffer.B[startDataPosition-32 : startDataPosition+4+lenPacket])
					if err != nil {
						return
					}
					atomic.AddUint64(&dc.stats.BytesOut, uint64(bytesOut))
				} else {
					// send encrypted proxy message
					tail16 := [16]byte{}
					n := copy(tail16[:], packetBuffer.B[startDataPosition+4+lenPacket:])
					xBuffer := encrypt(packetBuffer.B[startDataPosition+5:startDataPosition+4+lenPacket],
						message.proxy.session.ID, message.proxy.session.Block)
					if xBuffer == nil {
						// can't encrypt message
						return
					}
					bytesOut, err := dc.flusher.Write(xBuffer.B)
					if err != nil {
						return
					}
					atomic.AddUint64(&dc.stats.BytesOut, uint64(bytesOut))
					// resore tail
					copy(packetBuffer.B[startDataPosition+4+lenPacket:], tail16[:n])
					lib.ReleaseBuffer(xBuffer)
				}
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

		// get updates from the connection AtomCache and update the sender's cache (senderAtomCache)
		lastAddedAtom, lastAddedID := encodingOptions.AtomCache.LastAdded()
		if lastAddedID < 0 {
			continue
		}
		if _, exist := encodingOptions.SenderAtomCache[lastAddedAtom]; exist {
			continue
		}

		encodingOptions.AtomCache.RLock()
		for _, a := range encodingOptions.AtomCache.ListSince(lastAddedID) {
			encodingOptions.SenderAtomCache[a] = etf.CacheItem{ID: lastAddedID, Name: a, Encoded: false}
			lastAddedID++
		}
		encodingOptions.AtomCache.RUnlock()

	}

}

func (dc *distConnection) send(to string, creation uint32, msg *sendMessage) error {
	i := atomic.AddInt32(&dc.senders.i, 1)
	n := i % dc.senders.n
	s := dc.senders.sender[n]
	if s == nil {
		// connection was closed
		return lib.ErrNoRoute
	}
	dc.proxySessionsMutex.RLock()
	ps, isProxy := dc.proxySessionsByPeerName[to]
	dc.proxySessionsMutex.RUnlock()
	peer_creation := dc.creation
	if isProxy {
		msg.proxy = &ps
		peer_creation = ps.session.Creation
	} else {
		// its direct sending, so have to make sure if this peer does support compression
		if dc.flags.EnableCompression == false {
			msg.compression = false
		}
	}

	// if this peer is Erlang OTP 22 (and earlier), peer_creation is always 0, so we
	// must skip this checking.
	if creation > 0 && peer_creation > 0 && peer_creation != creation {
		return lib.ErrProcessIncarnation
	}

	// TODO to decide whether to return error if channel is full
	//select {
	//case s.sendChannel <- msg:
	//	return nil
	//default:
	//	return ErrOverloadConnection
	//}

	s.Lock()
	defer s.Unlock()

	s.sendChannel <- msg
	return nil
}

func (dc *distConnection) Stats() node.NetworkStats {
	return dc.stats
}

func proxyFlagsToUint64(pf node.ProxyFlags) uint64 {
	var flags uint64
	if pf.EnableLink {
		flags |= 1
	}
	if pf.EnableMonitor {
		flags |= 1 << 1
	}
	if pf.EnableRemoteSpawn {
		flags |= 1 << 2
	}
	if pf.EnableEncryption {
		flags |= 1 << 3
	}
	return flags
}

func proxyFlagsFromUint64(f uint64) node.ProxyFlags {
	var flags node.ProxyFlags
	flags.EnableLink = f&1 > 0
	flags.EnableMonitor = f&(1<<1) > 0
	flags.EnableRemoteSpawn = f&(1<<2) > 0
	flags.EnableEncryption = f&(1<<3) > 0
	return flags
}
