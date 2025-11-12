package proto

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
)

const (
	latency time.Duration = 300 * time.Nanosecond

	// lib.Buffer has 4096 of capacity
	// 256 messages could have at least 1Mb of allocated memory
	limitMemRecvQueues int64 = 1024 * 1024 * 512
)

type connection struct {
	id                  string
	creation            int64 // for uptime
	core                gen.Core
	log                 gen.Log
	node_flags          gen.NetworkFlags
	node_maxmessagesize int

	peer                gen.Atom
	peer_creation       int64
	peer_flags          gen.NetworkFlags
	peer_version        gen.Version
	peer_maxmessagesize int

	handshakeVersion gen.Version
	protoVersion     gen.Version

	pool_dsn  []string
	pool_size int

	pool_mutex sync.RWMutex
	pool       []*pool_item

	recvQueues        []lib.QueueMPSC
	allocatedInQueues int64

	encodeOptions edf.Options
	decodeOptions edf.Options

	requestsMutex sync.RWMutex
	requests      map[gen.Ref]chan MessageResult

	messagesIn  uint64
	messagesOut uint64
	bytesIn     uint64
	bytesOut    uint64
	transitIn   uint64
	transitOut  uint64

	order      uint32
	terminated bool
	wg         sync.WaitGroup
}

type pool_item struct {
	connection net.Conn
	fl         io.Writer
	timer      *time.Timer
	handling   atomic.Bool
}

//
// gen.RemoteNode implementation
//

func (c *connection) Name() gen.Atom {
	return c.peer
}

func (c *connection) Proxy() gen.Atom {
	// TODO
	return ""
}

func (c *connection) Uptime() int64 {
	return time.Now().Unix() - c.peer_creation
}
func (c *connection) Version() gen.Version {
	return c.peer_version
}

func (c *connection) Info() gen.RemoteNodeInfo {
	info := gen.RemoteNodeInfo{
		Node:             c.peer,
		Uptime:           time.Now().Unix() - c.peer_creation,
		ConnectionUptime: time.Now().Unix() - c.creation,
		Version:          c.peer_version,

		HandshakeVersion: c.handshakeVersion,
		ProtoVersion:     c.protoVersion,

		NetworkFlags: c.peer_flags,

		PoolSize: c.pool_size,
		PoolDSN:  c.pool_dsn,

		MaxMessageSize: c.peer_maxmessagesize,
		MessagesIn:     atomic.LoadUint64(&c.messagesIn),
		MessagesOut:    atomic.LoadUint64(&c.messagesOut),

		BytesIn:  atomic.LoadUint64(&c.bytesIn),
		BytesOut: atomic.LoadUint64(&c.bytesOut),

		TransitBytesIn:  atomic.LoadUint64(&c.transitIn),
		TransitBytesOut: atomic.LoadUint64(&c.transitOut),
	}
	return info
}

func (c *connection) Spawn(name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		ParentPID:      c.core.PID(),
		ParentLeader:   c.core.PID(),
		ParentLogLevel: c.core.LogLevel(),
		Args:           args,
	}
	if c.core.Security().ExposeEnvRemoteSpawn {
		opts.ParentEnv = c.core.EnvList()
	}
	return c.RemoteSpawn(name, opts)
}

func (c *connection) SpawnRegister(
	register gen.Atom,
	name gen.Atom,
	options gen.ProcessOptions,
	args ...any,
) (gen.PID, error) {
	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		ParentPID:      c.core.PID(),
		ParentLeader:   c.core.PID(),
		ParentLogLevel: c.core.LogLevel(),
		Register:       register,
		Args:           args,
	}
	if c.core.Security().ExposeEnvRemoteSpawn {
		opts.ParentEnv = c.core.EnvList()
	}
	return c.RemoteSpawn(name, opts)
}

func (c *connection) ApplicationStart(name gen.Atom, options gen.ApplicationOptions) error {
	return c.applicationStart(name, 0, options)
}
func (c *connection) ApplicationStartTemporary(name gen.Atom, options gen.ApplicationOptions) error {
	return c.applicationStart(name, gen.ApplicationModeTemporary, options)
}
func (c *connection) ApplicationStartTransient(name gen.Atom, options gen.ApplicationOptions) error {
	return c.applicationStart(name, gen.ApplicationModeTransient, options)
}
func (c *connection) ApplicationStartPermanent(name gen.Atom, options gen.ApplicationOptions) error {
	return c.applicationStart(name, gen.ApplicationModePermanent, options)
}

func (c *connection) applicationStart(name gen.Atom, mode gen.ApplicationMode, options gen.ApplicationOptions) error {
	if c.peer_flags.Enable && c.peer_flags.EnableRemoteApplicationStart == false {
		return gen.ErrNotAllowed
	}

	ref := c.core.MakeRef()
	extra := gen.ApplicationOptionsExtra{
		ApplicationOptions: options,
		CorePID:            c.core.PID(),
		CoreLogLevel:       c.core.LogLevel(),
	}
	if c.core.Security().ExposeEnvRemoteApplicationStart {
		extra.CoreEnv = c.core.EnvList()
	}
	message := MessageApplicationStart{
		Name:    name,
		Mode:    mode,
		Options: extra,
		Ref:     ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, 0, 0, gen.Compression{}); err != nil {
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error
}

func (c *connection) Creation() int64 {
	return c.peer_creation
}

func (c *connection) Disconnect() {
	c.Terminate(gen.TerminateReasonNormal)
}

func (c *connection) ConnectionUptime() int64 {
	return time.Now().Unix() - c.creation
}

func (c *connection) updateCache() error {

	// TODO
	//implement cache updating and add API methods for that

	// create new entries for:
	//  - AtomCache
	//  - AtomMapping
	//  - RegCache
	//  - ErrCache

	ref := c.core.MakeRef()
	message := MessageUpdateCache{
		Ref: ref,
		// put them here
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, 0, 0, gen.Compression{}); err != nil {
		return err
	}
	result := c.waitResult(ref, ch)
	if result.Error != nil {
		return result.Error
	}

	// TODO
	// add new entries to the local encoding cache
	// - c.encodeOptions.AtomCache
	// - c.encodeOptions.AtomMapping
	// - c.encodeOptions.RegCache
	// - c.encodeOptions.ErrCache

	return nil
}

//
// gen.Connection implementation
//

func (c *connection) Node() gen.RemoteNode {
	return c
}

func (c *connection) SendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	if to.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}

	if options.ImportantDelivery {
		if c.peer_flags.EnableImportantDelivery == false {
			return gen.ErrUnsupported
		}
	}

	order := uint8(from.ID % 255)
	orderPeer := uint8(to.ID % 255)
	if options.KeepNetworkOrder == false {
		order = uint8(0)
		orderPeer = uint8(0)
	}

	buf := lib.TakeBuffer()
	// 8 (header) + 8 (process id from) + 1 priority +8 (message id) + 8 (process id to)
	buf.Allocate(8 + 8 + 1 + 8 + 8)

	if err := edf.Encode(message, buf, c.encodeOptions); err != nil {
		return err
	}

	if buf.Len() > math.MaxUint32 {
		return gen.ErrTooLarge
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = orderPeer
	buf.B[7] = protoMessagePID
	binary.BigEndian.PutUint64(buf.B[8:16], from.ID)

	buf.B[16] = byte(options.Priority) // usual value 0, 1, or 2, so just cast it
	if options.ImportantDelivery {
		if c.peer_flags.EnableImportantDelivery == false {
			lib.ReleaseBuffer(buf)
			return gen.ErrUnsupported
		}
		// set important flag
		buf.B[16] |= 128
		binary.BigEndian.PutUint64(buf.B[17:25], options.Ref.ID[0])
	}

	binary.BigEndian.PutUint64(buf.B[25:33], to.ID)

	return c.send(buf, order, options.Compression)
}

func (c *connection) SendProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	toName := to.Name
	toNameCached := uint16(0)
	if c.encodeOptions.AtomMapping != nil {
		if v, found := c.encodeOptions.AtomMapping.Load(toName); found {
			toName = v.(gen.Atom)
		}
	}

	bname := []byte(toName)
	if len(bname) > 255 {
		return fmt.Errorf("process name too long")
	}

	if c.encodeOptions.AtomCache != nil {
		if v, found := c.encodeOptions.AtomCache.Load(toName); found {
			toNameCached = v.(uint16)
		}
	}

	order := uint8(from.ID % 255)
	if options.KeepNetworkOrder == false {
		order = uint8(0)
	}

	buf := lib.TakeBuffer()
	if toNameCached > 0 {
		// 8 (header) + 8 (process id from) + 1 priority + 8 (message id) + 2 (cache id)
		buf.Allocate(8 + 8 + 1 + 8 + 2)
	} else {
		// 8 (header) + 8 (process id from) + 1 priority + 8 (message id) + 1 (size(bname) + bname
		buf.Allocate(8 + 8 + 1 + 8 + 1 + len(bname))
	}

	if err := edf.Encode(message, buf, c.encodeOptions); err != nil {
		return err
	}

	if buf.Len() > math.MaxUint32 {
		return gen.ErrTooLarge
	}

	if c.peer_maxmessagesize > 0 && buf.Len() > c.peer_maxmessagesize {
		return gen.ErrTooLarge
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = order // use the same order for the peer
	binary.BigEndian.PutUint64(buf.B[8:16], from.ID)

	buf.B[16] = byte(options.Priority) // usual value 0, 1, or 2, so just cast it
	if options.ImportantDelivery {
		if c.peer_flags.EnableImportantDelivery == false {
			lib.ReleaseBuffer(buf)
			return gen.ErrUnsupported
		}
		// set important flag
		buf.B[16] |= 128
		binary.BigEndian.PutUint64(buf.B[17:25], options.Ref.ID[0])
	}

	if toNameCached > 0 {
		buf.B[7] = protoMessageNameCache
		binary.BigEndian.PutUint16(buf.B[25:27], toNameCached)
	} else {
		buf.B[7] = protoMessageName
		buf.B[25] = byte(len(bname))
		copy(buf.B[26:], bname)
	}

	return c.send(buf, order, options.Compression)
}

func (c *connection) SendAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	if to.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}

	order := uint8(from.ID % 255)
	orderPeer := uint8(to.ID[1] % 255)
	if options.KeepNetworkOrder == false {
		order = uint8(0)
		orderPeer = uint8(0)
	}

	buf := lib.TakeBuffer()
	// 8 (header) + 8 (process id from) + 1 priority + 8 (message id) + 24 (alias id [3]uint64)
	buf.Allocate(8 + 8 + 1 + 8 + 24)

	if err := edf.Encode(message, buf, c.encodeOptions); err != nil {
		return err
	}

	if buf.Len() > math.MaxUint32 {
		return gen.ErrTooLarge
	}

	if c.peer_maxmessagesize > 0 && buf.Len() > c.peer_maxmessagesize {
		return gen.ErrTooLarge
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = orderPeer
	buf.B[7] = protoMessageAlias
	binary.BigEndian.PutUint64(buf.B[8:16], from.ID)

	buf.B[16] = byte(options.Priority) // usual value 0, 1, or 2, so just cast it
	if options.ImportantDelivery {
		if c.peer_flags.EnableImportantDelivery == false {
			lib.ReleaseBuffer(buf)
			return gen.ErrUnsupported
		}
		// set important flag
		buf.B[16] |= 128
		binary.BigEndian.PutUint64(buf.B[17:25], options.Ref.ID[0])
	}

	binary.BigEndian.PutUint64(buf.B[25:33], to.ID[0])
	binary.BigEndian.PutUint64(buf.B[33:41], to.ID[1])
	binary.BigEndian.PutUint64(buf.B[41:49], to.ID[2])

	return c.send(buf, order, options.Compression)
}

func (c *connection) SendEvent(from gen.PID, options gen.MessageOptions, message gen.MessageEvent) error {
	eventName := message.Event.Name
	eventNameCached := uint16(0)

	if c.encodeOptions.AtomMapping != nil {
		if v, found := c.encodeOptions.AtomMapping.Load(eventName); found {
			eventName = v.(gen.Atom)
		}
	}

	bname := []byte(eventName)
	if len(bname) > 255 {
		return fmt.Errorf("event name too long")
	}

	if c.encodeOptions.AtomCache != nil {
		if v, found := c.encodeOptions.AtomCache.Load(eventName); found {
			eventNameCached = v.(uint16)
		}
	}

	order := uint8(from.ID % 255)
	if options.KeepNetworkOrder == false {
		order = uint8(0)
	}

	buf := lib.TakeBuffer()
	if eventNameCached > 0 {
		// 8 (header) + 8 (process id from) + 1 priority + 8 timestamp + 2 (cache id)
		buf.Allocate(8 + 8 + 1 + 8 + 2)
	} else {
		// 8 (header) + 8 (process id from) + 1 priority + 8 timestamp + 1 size(bname) + bname
		buf.Allocate(8 + 8 + 1 + 8 + 1 + len(bname))
	}

	if err := edf.Encode(message.Message, buf, c.encodeOptions); err != nil {
		return err
	}

	if buf.Len() > math.MaxUint32 {
		return gen.ErrTooLarge
	}

	if c.peer_maxmessagesize > 0 && buf.Len() > c.peer_maxmessagesize {
		return gen.ErrTooLarge
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = order // use the same order for the peer
	binary.BigEndian.PutUint64(buf.B[8:16], from.ID)
	buf.B[16] = byte(options.Priority) // usual value 0, 1, or 2, so just cast it
	binary.BigEndian.PutUint64(buf.B[17:25], uint64(message.Timestamp))

	if eventNameCached > 0 {
		buf.B[7] = protoMessageEventCache
		binary.BigEndian.PutUint16(buf.B[25:27], eventNameCached)
	} else {
		buf.B[7] = protoMessageEvent
		buf.B[25] = byte(len(bname))
		copy(buf.B[26:], bname)
	}

	return c.send(buf, order, options.Compression)
}

func (c *connection) SendExit(from gen.PID, to gen.PID, reason error) error {
	if to.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}

	buf := lib.TakeBuffer()
	// 8 (header) + 8 (process id from) + 1 priority + 8 (process id to)
	buf.Allocate(8 + 8 + 1 + 8)

	if err := edf.Encode(reason, buf, c.encodeOptions); err != nil {
		return err
	}

	order := uint8(from.ID % 255)
	orderPeer := uint8(to.ID % 255)

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = orderPeer
	buf.B[7] = protoMessageExit
	binary.BigEndian.PutUint64(buf.B[8:16], from.ID)
	buf.B[16] = byte(gen.MessagePriorityMax)
	binary.BigEndian.PutUint64(buf.B[17:25], to.ID)

	return c.send(buf, order, gen.Compression{})
}

func (c *connection) SendResponse(from gen.PID, to gen.PID, options gen.MessageOptions, response any) error {
	if to.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}
	order := uint8(from.ID % 255)
	orderPeer := uint8(to.ID % 255)
	if options.KeepNetworkOrder == false {
		order = uint8(0)
		orderPeer = uint8(0)
	}

	buf := lib.TakeBuffer()
	// 8 (header) + 8 (process id from) + 1 priority + 8 (process id to) + 24 (ref [3]uint64)
	buf.Allocate(8 + 8 + 1 + 8 + 24)

	if err := edf.Encode(response, buf, c.encodeOptions); err != nil {
		return err
	}

	if buf.Len() > math.MaxUint32 {
		return gen.ErrTooLarge
	}

	if c.peer_maxmessagesize > 0 && buf.Len() > c.peer_maxmessagesize {
		return gen.ErrTooLarge
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = orderPeer
	buf.B[7] = protoMessageResponse
	binary.BigEndian.PutUint64(buf.B[8:16], from.ID)

	buf.B[16] = byte(options.Priority) // usual value 0, 1, or 2, so just cast it
	if options.ImportantDelivery {
		if c.peer_flags.EnableImportantDelivery == false {
			lib.ReleaseBuffer(buf)
			return gen.ErrUnsupported
		}
		// set important flag
		buf.B[16] |= 128
	}

	binary.BigEndian.PutUint64(buf.B[17:25], to.ID)
	binary.BigEndian.PutUint64(buf.B[25:33], options.Ref.ID[0])
	binary.BigEndian.PutUint64(buf.B[33:41], options.Ref.ID[1])
	binary.BigEndian.PutUint64(buf.B[41:49], options.Ref.ID[2])

	return c.send(buf, order, options.Compression)
}

func (c *connection) SendResponseError(from gen.PID, to gen.PID, options gen.MessageOptions, err error) error {
	if to.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}

	order := uint8(from.ID % 255)
	orderPeer := uint8(to.ID % 255)
	if options.KeepNetworkOrder == false {
		order = uint8(0)
		orderPeer = uint8(0)
	}

	buf := lib.TakeBuffer()
	// 8 (header) + 8 (process id from) + 1 priority + 8 (process id to) + 24 (ref [3]uint64) + 1 (err code)
	buf.Allocate(8 + 8 + 1 + 8 + 24 + 1)
	switch err {
	case nil:
		buf.B[49] = 0
	case gen.ErrProcessUnknown:
		buf.B[49] = 1
	case gen.ErrProcessMailboxFull:
		buf.B[49] = 2
	case gen.ErrProcessTerminated:
		buf.B[49] = 3
	default:
		buf.B[49] = 255
		if e := edf.Encode(err, buf, c.encodeOptions); e != nil {
			return e
		}
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = orderPeer
	buf.B[7] = protoMessageResponseError
	binary.BigEndian.PutUint64(buf.B[8:16], from.ID)

	buf.B[16] = byte(options.Priority) // usual value 0, 1, or 2, so just cast it
	if options.ImportantDelivery {
		if c.peer_flags.EnableImportantDelivery == false {
			lib.ReleaseBuffer(buf)
			return gen.ErrUnsupported
		}
		// set important flag
		buf.B[16] |= 128
	}

	binary.BigEndian.PutUint64(buf.B[17:25], to.ID)
	binary.BigEndian.PutUint64(buf.B[25:33], options.Ref.ID[0])
	binary.BigEndian.PutUint64(buf.B[33:41], options.Ref.ID[1])
	binary.BigEndian.PutUint64(buf.B[41:49], options.Ref.ID[2])

	return c.send(buf, order, options.Compression)
}

func (c *connection) SendTerminatePID(target gen.PID, reason error) error {
	if target.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}
	buf := lib.TakeBuffer()
	// 8 (header) + 1 priority + 8 (target process id)
	buf.Allocate(8 + 1 + 8)

	if err := edf.Encode(reason, buf, c.encodeOptions); err != nil {
		return err
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = 0
	buf.B[7] = protoMessageTerminatePID
	buf.B[8] = byte(gen.MessagePriorityHigh)
	binary.BigEndian.PutUint64(buf.B[9:17], target.ID)

	return c.send(buf, 0, gen.Compression{})
}

func (c *connection) SendTerminateProcessID(target gen.ProcessID, reason error) error {
	targetName := target.Name
	targetNameCached := uint16(0)
	if c.encodeOptions.AtomMapping != nil {
		if v, found := c.encodeOptions.AtomMapping.Load(targetName); found {
			targetName = v.(gen.Atom)
		}
	}

	bname := []byte(targetName)
	if len(bname) > 255 {
		return fmt.Errorf("target process name too long")
	}

	if c.encodeOptions.AtomCache != nil {
		if v, found := c.encodeOptions.AtomCache.Load(targetName); found {
			targetNameCached = v.(uint16)
		}
	}

	buf := lib.TakeBuffer()
	if targetNameCached > 0 {
		// 8 (header) + 1 priority + 2 (cache id)
		buf.Allocate(8 + 1 + 2)
	} else {
		// 8 (header) + 1 priority + 1 (len of bname) + bname
		buf.Allocate(8 + 1 + 1 + len(bname))
	}

	if err := edf.Encode(reason, buf, c.encodeOptions); err != nil {
		return err
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = 0 // order
	buf.B[8] = byte(gen.MessagePriorityHigh)
	if targetNameCached > 0 {
		buf.B[7] = protoMessageTerminateNameCache
		binary.BigEndian.PutUint16(buf.B[9:11], targetNameCached)
	} else {
		buf.B[7] = protoMessageTerminateName
		buf.B[9] = byte(len(bname))
		copy(buf.B[10:], bname)
	}

	return c.send(buf, 0, gen.Compression{})
}

func (c *connection) SendTerminateAlias(target gen.Alias, reason error) error {
	if target.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}
	buf := lib.TakeBuffer()
	// 8 (header) + 1 priority + 24 (target alias id [3]uint64)
	buf.Allocate(8 + 1 + 24)

	if err := edf.Encode(reason, buf, c.encodeOptions); err != nil {
		return err
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = 0
	buf.B[7] = protoMessageTerminateAlias
	buf.B[8] = byte(gen.MessagePriorityHigh)
	binary.BigEndian.PutUint64(buf.B[9:17], target.ID[0])
	binary.BigEndian.PutUint64(buf.B[17:25], target.ID[1])
	binary.BigEndian.PutUint64(buf.B[25:33], target.ID[2])

	return c.send(buf, 0, gen.Compression{})
}

func (c *connection) SendTerminateEvent(target gen.Event, reason error) error {
	eventName := target.Name
	eventNameCached := uint16(0)

	if c.encodeOptions.AtomMapping != nil {
		if v, found := c.encodeOptions.AtomMapping.Load(eventName); found {
			eventName = v.(gen.Atom)
		}
	}

	bname := []byte(eventName)
	if len(bname) > 255 {
		return fmt.Errorf("terminated event name too long")
	}

	if c.encodeOptions.AtomCache != nil {
		if v, found := c.encodeOptions.AtomCache.Load(eventName); found {
			eventNameCached = v.(uint16)
		}
	}

	buf := lib.TakeBuffer()
	if eventNameCached > 0 {
		// 8 (header) + 1 priority + 2 (cache id)
		buf.Allocate(8 + 1 + 2)
	} else {
		// 8 (header) + 1 priority + 1 size(bname) + bname
		buf.Allocate(8 + 1 + +1 + len(bname))
	}

	if err := edf.Encode(reason, buf, c.encodeOptions); err != nil {
		return err
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = 0 // order
	buf.B[8] = byte(gen.MessagePriorityHigh)

	if eventNameCached > 0 {
		buf.B[7] = protoMessageTerminateEventCache
		binary.BigEndian.PutUint16(buf.B[9:11], eventNameCached)
	} else {
		buf.B[7] = protoMessageTerminateEvent
		buf.B[9] = byte(len(bname))
		copy(buf.B[10:], bname)
	}

	return c.send(buf, 0, gen.Compression{})
}

func (c *connection) CallPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	if to.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}

	order := uint8(from.ID % 255)
	orderPeer := uint8(to.ID % 255)
	if options.KeepNetworkOrder == false {
		order = uint8(0)
		orderPeer = uint8(0)
	}

	buf := lib.TakeBuffer()
	// 8 (header) + 8 (process id from) + 1 priority + 24 (request ref) + 8 (process id to)
	buf.Allocate(8 + 8 + 1 + 24 + 8)

	if err := edf.Encode(message, buf, c.encodeOptions); err != nil {
		return err
	}
	if buf.Len() > math.MaxUint32 {
		return gen.ErrTooLarge
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = orderPeer
	buf.B[7] = protoRequestPID
	binary.BigEndian.PutUint64(buf.B[8:16], from.ID)

	buf.B[16] = byte(options.Priority)
	if options.ImportantDelivery {
		if c.peer_flags.EnableImportantDelivery == false {
			lib.ReleaseBuffer(buf)
			return gen.ErrUnsupported
		}
		// set important flag
		buf.B[16] |= 128
	}

	binary.BigEndian.PutUint64(buf.B[17:25], options.Ref.ID[0])
	binary.BigEndian.PutUint64(buf.B[25:33], options.Ref.ID[1])
	binary.BigEndian.PutUint64(buf.B[33:41], options.Ref.ID[2])
	binary.BigEndian.PutUint64(buf.B[41:49], to.ID)

	return c.send(buf, order, options.Compression)
}

func (c *connection) CallProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	toName := to.Name
	toNameCached := uint16(0)
	if c.encodeOptions.AtomMapping != nil {
		if v, found := c.encodeOptions.AtomMapping.Load(toName); found {
			toName = v.(gen.Atom)
		}
	}

	bname := []byte(toName)
	if len(bname) > 255 {
		return fmt.Errorf("process name too long")
	}

	if c.encodeOptions.AtomCache != nil {
		if v, found := c.encodeOptions.AtomCache.Load(toName); found {
			toNameCached = v.(uint16)
		}
	}

	order := uint8(from.ID % 255)
	if options.KeepNetworkOrder == false {
		order = uint8(0)
	}

	buf := lib.TakeBuffer()
	if toNameCached > 0 {
		// 8 (header) + 8 (process id from) + 1 priority + 24 (request ref)  + 2 (cache id)
		buf.Allocate(8 + 8 + 1 + 24 + 2)
	} else {
		// 8 (header) + 8 (process id from) + 1 priority + 24 (request ref) +1 (size(bname)) + bname
		buf.Allocate(8 + 8 + 1 + 24 + 1 + len(bname))
	}

	if err := edf.Encode(message, buf, c.encodeOptions); err != nil {
		return err
	}

	if buf.Len() > math.MaxUint32 {
		return gen.ErrTooLarge
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = order // use the same order for the peer
	binary.BigEndian.PutUint64(buf.B[8:16], from.ID)

	buf.B[16] = byte(options.Priority)
	if options.ImportantDelivery {
		if c.peer_flags.EnableImportantDelivery == false {
			lib.ReleaseBuffer(buf)
			return gen.ErrUnsupported
		}
		// set important flag
		buf.B[16] |= 128
	}

	binary.BigEndian.PutUint64(buf.B[17:25], options.Ref.ID[0])
	binary.BigEndian.PutUint64(buf.B[25:33], options.Ref.ID[1])
	binary.BigEndian.PutUint64(buf.B[33:41], options.Ref.ID[2])

	if toNameCached > 0 {
		buf.B[7] = protoRequestNameCache
		binary.BigEndian.PutUint16(buf.B[41:43], toNameCached)
	} else {
		buf.B[7] = protoRequestName
		buf.B[41] = byte(len(bname))
		copy(buf.B[42:], bname)
	}

	return c.send(buf, order, options.Compression)
}

func (c *connection) CallAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {

	if to.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}

	order := uint8(from.ID % 255)
	orderPeer := uint8(to.ID[1] % 255)
	if options.KeepNetworkOrder == false {
		order = uint8(0)
		orderPeer = uint8(0)
	}

	buf := lib.TakeBuffer()
	// 8 (header) + 8 (process id from) + 1 priority + 24 (request ref) + 24 (alias id to)
	buf.Allocate(8 + 8 + 1 + 24 + 24)

	if err := edf.Encode(message, buf, c.encodeOptions); err != nil {
		return err
	}

	if buf.Len() > math.MaxUint32 {
		return gen.ErrTooLarge
	}

	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = orderPeer
	buf.B[7] = protoRequestAlias
	binary.BigEndian.PutUint64(buf.B[8:16], from.ID)

	buf.B[16] = byte(options.Priority)
	if options.ImportantDelivery {
		if c.peer_flags.EnableImportantDelivery == false {
			lib.ReleaseBuffer(buf)
			return gen.ErrUnsupported
		}
		// set important flag
		buf.B[16] |= 128
	}

	binary.BigEndian.PutUint64(buf.B[17:25], options.Ref.ID[0])
	binary.BigEndian.PutUint64(buf.B[25:33], options.Ref.ID[1])
	binary.BigEndian.PutUint64(buf.B[33:41], options.Ref.ID[2])
	binary.BigEndian.PutUint64(buf.B[41:49], to.ID[0])
	binary.BigEndian.PutUint64(buf.B[49:57], to.ID[1])
	binary.BigEndian.PutUint64(buf.B[57:65], to.ID[2])

	return c.send(buf, order, options.Compression)
}

func (c *connection) LinkPID(pid gen.PID, target gen.PID) error {
	if target.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}
	order := uint8(pid.ID % 255)
	orderPeer := uint8(target.ID % 255)
	ref := c.core.MakeRef()
	message := MessageLinkPID{
		Source: pid,
		Target: target,
		Ref:    ref,
	}

	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, orderPeer, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error
}

func (c *connection) UnlinkPID(pid gen.PID, target gen.PID) error {
	if target.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}
	order := uint8(pid.ID % 255)
	orderPeer := uint8(target.ID % 255)
	ref := c.core.MakeRef()
	message := MessageUnlinkPID{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, orderPeer, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error
}

func (c *connection) LinkProcessID(pid gen.PID, target gen.ProcessID) error {
	order := uint8(pid.ID % 255)
	ref := c.core.MakeRef()
	message := MessageLinkProcessID{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, 0, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error

}

func (c *connection) UnlinkProcessID(pid gen.PID, target gen.ProcessID) error {
	order := uint8(pid.ID % 255)
	ref := c.core.MakeRef()
	message := MessageUnlinkProcessID{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, 0, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error

}

func (c *connection) LinkAlias(pid gen.PID, target gen.Alias) error {
	if target.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}
	order := uint8(pid.ID % 255)
	orderPeer := uint8(target.ID[1] % 255)
	ref := c.core.MakeRef()
	message := MessageLinkAlias{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, orderPeer, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error

}

func (c *connection) UnlinkAlias(pid gen.PID, target gen.Alias) error {
	if target.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}
	order := uint8(pid.ID % 255)
	orderPeer := uint8(target.ID[1] % 255)
	ref := c.core.MakeRef()
	message := MessageUnlinkAlias{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, orderPeer, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error
}

func (c *connection) LinkEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {
	order := uint8(pid.ID % 255)
	ref := c.core.MakeRef()
	message := MessageLinkEvent{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, 0, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return nil, err
	}
	result := c.waitResult(ref, ch)
	if result.Error != nil {
		return nil, result.Error
	}
	r, ok := result.Result.([]gen.MessageEvent)
	if ok == false {
		return nil, gen.ErrMalformed
	}
	return r, nil
}

func (c *connection) UnlinkEvent(pid gen.PID, target gen.Event) error {
	order := uint8(pid.ID % 255)
	ref := c.core.MakeRef()
	message := MessageUnlinkEvent{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, 0, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error
}

func (c *connection) MonitorPID(pid gen.PID, target gen.PID) error {
	if target.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}
	ref := c.core.MakeRef()
	order := uint8(pid.ID % 255)
	message := MessageMonitorPID{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, 0, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error
}

func (c *connection) DemonitorPID(pid gen.PID, target gen.PID) error {
	if target.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}
	ref := c.core.MakeRef()
	order := uint8(pid.ID % 255)
	message := MessageDemonitorPID{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, 0, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error
}

func (c *connection) MonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	order := uint8(pid.ID % 255)
	ref := c.core.MakeRef()
	message := MessageMonitorProcessID{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, 0, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error
}

func (c *connection) DemonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	order := uint8(pid.ID % 255)
	ref := c.core.MakeRef()
	message := MessageDemonitorProcessID{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, 0, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error
}

func (c *connection) MonitorAlias(pid gen.PID, target gen.Alias) error {
	if target.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}
	ref := c.core.MakeRef()
	order := uint8(pid.ID % 255)
	orderPeer := uint8(target.ID[1] % 255)
	message := MessageMonitorAlias{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, orderPeer, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error
}

func (c *connection) DemonitorAlias(pid gen.PID, target gen.Alias) error {
	if target.Creation != c.peer_creation {
		return gen.ErrProcessIncarnation
	}
	ref := c.core.MakeRef()
	order := uint8(pid.ID % 255)
	orderPeer := uint8(target.ID[1] % 255)
	message := MessageDemonitorAlias{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, orderPeer, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error
}

func (c *connection) MonitorEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {
	order := uint8(pid.ID % 255)
	ref := c.core.MakeRef()
	message := MessageMonitorEvent{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, 0, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return nil, err
	}
	result := c.waitResult(ref, ch)
	if result.Error != nil {
		return nil, result.Error
	}
	r, ok := result.Result.([]gen.MessageEvent)
	if ok == false {
		return nil, gen.ErrMalformed
	}
	return r, nil
}

func (c *connection) DemonitorEvent(pid gen.PID, target gen.Event) error {
	order := uint8(pid.ID % 255)
	ref := c.core.MakeRef()
	message := MessageDemonitorEvent{
		Source: pid,
		Target: target,
		Ref:    ref,
	}
	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()
	if err := c.sendAny(message, order, 0, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return err
	}
	result := c.waitResult(ref, ch)
	return result.Error
}

func (c *connection) RemoteSpawn(name gen.Atom, options gen.ProcessOptionsExtra) (gen.PID, error) {
	var pid gen.PID

	if c.peer_flags.Enable && c.peer_flags.EnableRemoteSpawn == false {
		return pid, gen.ErrNotAllowed
	}

	order := uint8(pid.ID % 255)
	ref := c.core.MakeRef()

	message := MessageSpawn{
		Name:    name,
		Options: options,
		Ref:     ref,
	}

	ch := make(chan MessageResult)
	c.requestsMutex.Lock()
	c.requests[ref] = ch
	c.requestsMutex.Unlock()

	if err := c.sendAny(message, order, 0, gen.Compression{}); err != nil {
		c.requestsMutex.Lock()
		delete(c.requests, ref)
		c.requestsMutex.Unlock()
		return pid, err
	}

	result := c.waitResult(ref, ch)
	if result.Error != nil {
		return pid, result.Error
	}
	pid, ok := result.Result.(gen.PID)
	if ok == false {
		return pid, gen.ErrMalformed
	}
	return pid, nil
}

func (c *connection) Join(conn net.Conn, id string, dial gen.NetworkDial, tail []byte) error {
	if id != c.id {
		return fmt.Errorf("connection id mismatch")
	}

	if c.terminated {
		return fmt.Errorf("connection terminated")
	}

	c.pool_mutex.Lock()
	if c.pool_size+1 < len(c.pool) {
		c.pool_mutex.Unlock()
		return fmt.Errorf("pool size limit")
	}
	pi := &pool_item{
		connection: conn,
		fl:         lib.NewFlusher(conn),
	}
	c.pool = append(c.pool, pi)
	c.pool_mutex.Unlock()

	c.wg.Add(1)
	go func() {
		if lib.Trace() {
			defer c.log.Trace("connection %s left the pool", conn.RemoteAddr().String())
		}

	re: // reconnected
		if lib.Trace() {
			c.log.Trace("joined new connection %s to the pool", conn.RemoteAddr().String())
		}

		c.serve(pi.connection, tail)

		if dial != nil {
			pool_dsn := []string{}
			pool_dsn = append(pool_dsn, c.pool_dsn...)
			rand.Shuffle(len(pool_dsn), func(i, j int) {
				pool_dsn[i], pool_dsn[j] = pool_dsn[j], pool_dsn[i]
			})
			for _, dsn := range pool_dsn {
				if c.terminated {
					c.wg.Done()
					return
				}
				c.log.Trace("re-dialing %s", dsn)
				nc, t, err := dial(dsn, id)
				if err != nil {
					continue
				}
				pi.connection = nc
				tail = t

				goto re
			}
		}
		if c.terminated {
			c.wg.Done()
			return
		}

		// remove it from the pool
		c.pool_mutex.Lock()
		for i, item := range c.pool {
			if item != pi {
				continue
			}
			c.pool[i] = c.pool[0]
			c.pool = c.pool[1:]
		}
		if len(c.pool) == 0 {
			c.pool_mutex.Unlock()
			c.wg.Done()
			return
		}
		c.pool_mutex.Unlock()

		c.wg.Done()
	}()

	return nil
}

func (c *connection) Terminate(reason error) {
	c.terminated = true

	c.pool_mutex.Lock()
	defer c.pool_mutex.Unlock()
	for _, pi := range c.pool {
		pi.connection.Close()
	}
}

func (c *connection) serve(conn net.Conn, tail []byte) {

	recvN := 0
	recvNQ := len(c.recvQueues)

	buf := lib.TakeBuffer()
	buf.Append(tail)

	// remove the deadline
	conn.SetReadDeadline(time.Time{})

	for {
		// read packet
		buftail, err := c.read(conn, buf)
		if err != nil || buftail == nil {
			if err != nil {
				c.log.Trace("link with %s closed: %s", conn.RemoteAddr(), err)
			}
			lib.ReleaseBuffer(buf)
			conn.Close()
			return
		}

		if buf.B[0] != protoMagic {
			c.log.Error("recevied malformed packet from %s (incorrect proto)", conn.RemoteAddr())
			lib.ReleaseBuffer(buf)
			conn.Close()
			return
		}

		if buf.B[1] != protoVersion {
			c.log.Error("recevied malformed packet from %s (incorrect proto version)", conn.RemoteAddr())
			lib.ReleaseBuffer(buf)
			conn.Close()
			return
		}

		recvN++

		atomic.AddUint64(&c.messagesIn, 1)
		atomic.AddUint64(&c.bytesIn, uint64(buf.Len()))
		// TODO
		// c.transitIn

		// send 'buf' to the decoding queue
		qN := recvN % recvNQ
		if order := int(buf.B[6]); order > 0 {
			qN = order % recvNQ
		}
		if lib.Trace() {
			c.log.Trace("received message. put it to pool[%d] of %s...", qN, conn.RemoteAddr())
		}
		queue := c.recvQueues[qN]
		atomic.AddInt64(&c.allocatedInQueues, int64(buf.Cap()))

		queue.Push(buf)
		if queue.Lock() {
			go c.handleRecvQueue(queue)
		}
		buf = buftail

	}
}

func (c *connection) handleRecvQueue(q lib.QueueMPSC) {
	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				c.log.Panic("panic on handling received message: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)
				c.Terminate(gen.TerminateReasonPanic)
			}
		}()
	}

	if lib.Trace() {
		c.log.Trace("start handling the message queue")
	}
	for {
		v, ok := q.Pop()
		if ok == false {
			// no more items in the queue, unlock it
			q.Unlock()

			// but check the queue before the exit this goroutine
			if i := q.Item(); i == nil {
				return
			}

			// there is something in the queue, try to lock it back
			if locked := q.Lock(); locked == false {
				// another goroutine is started
				return
			}
			// get back to work
			continue
		}

		buf := v.(*lib.Buffer)

		// to avoid getting the buffer pool too big, we check the total volume (capacity)
		// we took from there and don't put it back if the limit has been reached.
		releaseBuffer := true
		if atomic.AddInt64(&c.allocatedInQueues, int64(-buf.Cap())) > limitMemRecvQueues {
			releaseBuffer = false
		}

	re:
		switch buf.B[7] {
		case protoMessagePID: // process id
			if buf.Len() < 30 {
				c.log.Error("malformed message (too small MessagePID)")
				continue
			}
			idFrom := binary.BigEndian.Uint64(buf.B[8:16])
			priority := gen.MessagePriority(buf.B[16] & 3)
			important := (buf.B[16] & 128) > 0
			idTO := binary.BigEndian.Uint64(buf.B[25:33])

			msg, tail, err := edf.Decode(buf.B[33:], c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			from := gen.PID{
				Node:     c.peer,
				ID:       idFrom,
				Creation: c.peer_creation,
			}
			to := gen.PID{
				Node:     c.core.Name(),
				ID:       idTO,
				Creation: c.core.Creation(),
			}

			opts := gen.MessageOptions{
				Priority: priority,
			}

			err = c.core.RouteSendPID(from, to, opts, msg)
			if important == false {
				continue
			}
			if c.node_flags.EnableImportantDelivery == false {
				continue
			}

			opts.Ref.ID[0] = binary.BigEndian.Uint64(buf.B[17:25])
			c.SendResponseError(to, from, opts, err)

		case protoMessageName, protoMessageNameCache: // name, chached name
			var toName gen.Atom
			var data []byte

			if buf.Len() < 18 {
				c.log.Error("malformed message (too small MessageName*)")
				continue
			}

			if buf.B[7] == protoMessageName {
				l := int(buf.B[25])
				if buf.Len() < 26+l {
					c.log.Error("malformed message (too small MessageName)")
					continue
				}

				toName = gen.Atom(buf.B[26 : 26+l])
				data = buf.B[26+l:]

			} else {
				if buf.Len() < 28 {
					c.log.Error("malformed message (too small MessageNameCache)")
					continue
				}

				id := binary.BigEndian.Uint16(buf.B[25:27])
				if c.decodeOptions.AtomCache == nil {
					c.log.Error("received message with cached atom value %d, but cache is nil (message ignored). please, report this bug", id)
					lib.ReleaseBuffer(buf)
					continue
				}

				v, found := c.decodeOptions.AtomCache.Load(id)
				if found == false {
					c.log.Error("received message with unknown atom cache id %d (message ignored). please, report this bug", id)
					lib.ReleaseBuffer(buf)
					continue
				}

				toName = v.(gen.Atom)
				data = buf.B[27:]
			}
			idFrom := binary.BigEndian.Uint64(buf.B[8:16])
			priority := gen.MessagePriority(buf.B[16] & 3)
			important := (buf.B[16] & 128) > 0

			msg, tail, err := edf.Decode(data, c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			if c.decodeOptions.AtomMapping != nil {
				if v, found := c.decodeOptions.AtomMapping.Load(toName); found {
					toName = v.(gen.Atom)
				}
			}

			from := gen.PID{
				Node:     c.peer,
				ID:       idFrom,
				Creation: c.peer_creation,
			}
			to := gen.ProcessID{
				Node: c.core.Name(),
				Name: toName,
			}

			opts := gen.MessageOptions{
				Priority: priority,
			}

			err = c.core.RouteSendProcessID(from, to, opts, msg)
			if important == false {
				continue
			}
			if c.node_flags.EnableImportantDelivery == false {
				continue
			}

			opts.Ref.ID[0] = binary.BigEndian.Uint64(buf.B[17:25])
			c.SendResponseError(gen.PID{}, from, opts, err)

		case protoMessageAlias:
			if buf.Len() < 49 {
				c.log.Error("malformed message (too small MessageAlias)")
				continue
			}

			idFrom := binary.BigEndian.Uint64(buf.B[8:16])
			priority := gen.MessagePriority(buf.B[16] & 3)
			important := (buf.B[16] & 128) > 0
			idTo := [3]uint64{
				binary.BigEndian.Uint64(buf.B[25:33]),
				binary.BigEndian.Uint64(buf.B[33:41]),
				binary.BigEndian.Uint64(buf.B[41:49]),
			}

			msg, tail, err := edf.Decode(buf.B[49:], c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			from := gen.PID{
				Node:     c.peer,
				ID:       idFrom,
				Creation: c.peer_creation,
			}
			to := gen.Alias{
				Node:     c.core.Name(),
				ID:       idTo,
				Creation: c.core.Creation(),
			}

			opts := gen.MessageOptions{
				Priority: priority,
			}
			err = c.core.RouteSendAlias(from, to, opts, msg)

			if important == false {
				continue
			}
			if c.node_flags.EnableImportantDelivery == false {
				continue
			}

			opts.Ref.ID[0] = binary.BigEndian.Uint64(buf.B[17:25])
			c.SendResponseError(gen.PID{}, from, opts, err)

		case protoRequestPID:
			if buf.Len() < 50 {
				c.log.Error("malformed message (too small RequestPID)")
				continue
			}

			ref := gen.Ref{
				Node:     c.peer,
				Creation: c.peer_creation,
			}
			idFrom := binary.BigEndian.Uint64(buf.B[8:16])

			priority := gen.MessagePriority(buf.B[16] & 3)
			important := (buf.B[16] & 128) > 0

			ref.ID[0] = binary.BigEndian.Uint64(buf.B[17:25])
			ref.ID[1] = binary.BigEndian.Uint64(buf.B[25:33])
			ref.ID[2] = binary.BigEndian.Uint64(buf.B[33:41])
			idTO := binary.BigEndian.Uint64(buf.B[41:49])

			msg, tail, err := edf.Decode(buf.B[49:], c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			from := gen.PID{
				Node:     c.peer,
				ID:       idFrom,
				Creation: c.peer_creation,
			}
			to := gen.PID{
				Node:     c.core.Name(),
				ID:       idTO,
				Creation: c.core.Creation(),
			}

			opts := gen.MessageOptions{
				Ref:      ref,
				Priority: priority,
			}

			err = c.core.RouteCallPID(from, to, opts, msg)
			if err == nil {
				continue
			}

			if important == false {
				continue
			}

			if c.node_flags.EnableImportantDelivery == false {
				continue
			}

			c.SendResponseError(to, from, opts, err)

		case protoRequestName, protoRequestNameCache:
			if buf.Len() < 43 {
				c.log.Error("malformed message (too small RequestName*)")
				continue
			}
			from := gen.PID{
				Node:     c.peer,
				Creation: c.peer_creation,
				ID:       binary.BigEndian.Uint64(buf.B[8:16]),
			}
			priority := gen.MessagePriority(buf.B[16] & 3)
			important := (buf.B[16] & 128) > 0

			ref := gen.Ref{
				Node:     c.peer,
				Creation: c.peer_creation,
			}
			ref.ID[0] = binary.BigEndian.Uint64(buf.B[17:25])
			ref.ID[1] = binary.BigEndian.Uint64(buf.B[25:33])
			ref.ID[2] = binary.BigEndian.Uint64(buf.B[33:41])

			to := gen.ProcessID{
				Node: c.core.Name(),
			}

			var data []byte
			if buf.B[7] == protoRequestName {
				l := int(buf.B[41])
				if buf.Len() < 42+l {
					c.log.Error("malformed message (too small RequestName)")
					continue
				}

				to.Name = gen.Atom(buf.B[42 : 42+l])
				data = buf.B[42+l:]

			} else {
				id := binary.BigEndian.Uint16(buf.B[41:43])
				if c.decodeOptions.AtomCache == nil {
					c.log.Error("received message with cached atom value %d, but cache is nil (message ignored). please, report this bug", id)
					lib.ReleaseBuffer(buf)
					continue
				}

				v, found := c.decodeOptions.AtomCache.Load(id)
				if found == false {
					c.log.Error("received message with unknown atom cache id %d (message ignored). please, report this bug", id)
					lib.ReleaseBuffer(buf)
					continue
				}

				to.Name = v.(gen.Atom)
				data = buf.B[43:]
			}

			msg, tail, err := edf.Decode(data, c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			if c.decodeOptions.AtomMapping != nil {
				if v, found := c.decodeOptions.AtomMapping.Load(to.Name); found {
					to.Name = v.(gen.Atom)
				}
			}
			opts := gen.MessageOptions{
				Ref:      ref,
				Priority: priority,
			}

			err = c.core.RouteCallProcessID(from, to, opts, msg)
			if err == nil {
				continue
			}

			if important == false {
				continue
			}

			if c.node_flags.EnableImportantDelivery == false {
				continue
			}

			c.SendResponseError(gen.PID{}, from, opts, err)

		case protoRequestAlias:
			if buf.Len() < 66 {
				c.log.Error("malformed message (too small RequestAlias)")
				continue
			}
			ref := gen.Ref{
				Node:     c.peer,
				Creation: c.peer_creation,
			}
			to := gen.Alias{
				Node:     c.core.Name(),
				Creation: c.core.Creation(),
			}
			idFrom := binary.BigEndian.Uint64(buf.B[8:16])

			priority := gen.MessagePriority(buf.B[16] & 3)
			important := (buf.B[16] & 128) > 0

			ref.ID[0] = binary.BigEndian.Uint64(buf.B[17:25])
			ref.ID[1] = binary.BigEndian.Uint64(buf.B[25:33])
			ref.ID[2] = binary.BigEndian.Uint64(buf.B[33:41])
			to.ID[0] = binary.BigEndian.Uint64(buf.B[41:49])
			to.ID[1] = binary.BigEndian.Uint64(buf.B[49:57])
			to.ID[2] = binary.BigEndian.Uint64(buf.B[57:65])

			msg, tail, err := edf.Decode(buf.B[65:], c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			from := gen.PID{
				Node:     c.peer,
				ID:       idFrom,
				Creation: c.peer_creation,
			}

			opts := gen.MessageOptions{
				Ref:      ref,
				Priority: priority,
			}
			err = c.core.RouteCallAlias(from, to, opts, msg)
			if err == nil {
				continue
			}

			if important == false {
				continue
			}

			if c.node_flags.EnableImportantDelivery == false {
				continue
			}

			c.SendResponseError(gen.PID{}, from, opts, err)

		case protoMessageEvent, protoMessageEventCache:
			if buf.Len() < 28 {
				c.log.Error("malformed message (too small MessageEvent*)")
				continue
			}
			from := gen.PID{
				Node:     c.peer,
				Creation: c.peer_creation,
				ID:       binary.BigEndian.Uint64(buf.B[8:16]),
			}
			options := gen.MessageOptions{
				Priority: gen.MessagePriority(buf.B[16]),
			}
			message := gen.MessageEvent{
				Timestamp: int64(binary.BigEndian.Uint64(buf.B[17:25])),
			}
			message.Event.Node = c.peer

			var data []byte
			if buf.B[7] == protoMessageEvent {
				l := int(buf.B[25])
				if buf.Len() < 26+l {
					c.log.Error("malformed message (too small MessageEvent)")
					continue
				}
				message.Event.Name = gen.Atom(buf.B[26 : 26+l])
				data = buf.B[26+l:]

			} else {
				id := binary.BigEndian.Uint16(buf.B[25:27])
				if c.decodeOptions.AtomCache == nil {
					c.log.Error("received Event with cached atom value %d, but cache is nil (message ignored). please, report this bug", id)
					continue
				}

				v, found := c.decodeOptions.AtomCache.Load(id)
				if found == false {
					c.log.Error("received Event with unknown atom cache id %d (message ignored). please, report this bug", id)
					continue
				}

				message.Event.Name = v.(gen.Atom)
				data = buf.B[27:]
			}

			if c.decodeOptions.AtomMapping != nil {
				if v, found := c.decodeOptions.AtomMapping.Load(message.Event.Name); found {
					message.Event.Name = v.(gen.Atom)
				}
			}

			msg, tail, err := edf.Decode(data, c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			message.Message = msg
			c.core.RouteSendEvent(from, gen.Ref{}, options, message)

		case protoMessageExit:
			if buf.Len() < 26 {
				c.log.Error("malformed message (too small MessageExit)")
				continue
			}

			idFrom := binary.BigEndian.Uint64(buf.B[8:16])
			// priority := gen.MessagePriority(buf.B[16]) ignored
			idTO := binary.BigEndian.Uint64(buf.B[17:25])

			msg, tail, err := edf.Decode(buf.B[25:], c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			reason, ok := msg.(error)
			if ok == false {
				c.log.Error("received malformed Exit message: %v", msg)
				continue
			}

			from := gen.PID{
				Node:     c.peer,
				ID:       idFrom,
				Creation: c.peer_creation,
			}
			to := gen.PID{
				Node:     c.core.Name(),
				ID:       idTO,
				Creation: c.core.Creation(),
			}

			c.core.RouteSendExit(from, to, reason)

		case protoMessageResponse:
			if buf.Len() < 49 {
				c.log.Error("malformed message (too small MessageResponse)")
				continue
			}

			ref := gen.Ref{
				Node:     c.core.Name(),
				Creation: c.core.Creation(),
			}
			idFrom := binary.BigEndian.Uint64(buf.B[8:16])
			priority := gen.MessagePriority(buf.B[16] & 3)
			important := (buf.B[16] & 128) > 0
			idTO := binary.BigEndian.Uint64(buf.B[17:25])
			ref.ID[0] = binary.BigEndian.Uint64(buf.B[25:33])
			ref.ID[1] = binary.BigEndian.Uint64(buf.B[33:41])
			ref.ID[2] = binary.BigEndian.Uint64(buf.B[41:49])

			msg, tail, err := edf.Decode(buf.B[49:], c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			from := gen.PID{
				Node:     c.peer,
				ID:       idFrom,
				Creation: c.peer_creation,
			}
			to := gen.PID{
				Node:     c.core.Name(),
				ID:       idTO,
				Creation: c.core.Creation(),
			}

			opts := gen.MessageOptions{
				Ref:               ref,
				Priority:          priority,
				ImportantDelivery: important,
			}

			err = c.core.RouteSendResponse(from, to, opts, msg)

			if important == false {
				continue
			}
			if err != nil {
				opts.ImportantDelivery = false
				c.SendResponseError(to, from, opts, err)
			}

		case protoMessageResponseError:
			if buf.Len() < 50 {
				c.log.Error("malformed message (too small MessageResponseError)")
				continue
			}

			ref := gen.Ref{
				Node:     c.core.Name(),
				Creation: c.core.Creation(),
			}
			idFrom := binary.BigEndian.Uint64(buf.B[8:16])
			priority := gen.MessagePriority(buf.B[16] & 3)
			important := (buf.B[16] & 128) > 0
			idTO := binary.BigEndian.Uint64(buf.B[17:25])
			ref.ID[0] = binary.BigEndian.Uint64(buf.B[25:33])
			ref.ID[1] = binary.BigEndian.Uint64(buf.B[33:41])
			ref.ID[2] = binary.BigEndian.Uint64(buf.B[41:49])

			from := gen.PID{
				Node:     c.peer,
				ID:       idFrom,
				Creation: c.peer_creation,
			}
			to := gen.PID{
				Node:     c.core.Name(),
				ID:       idTO,
				Creation: c.core.Creation(),
			}
			opts := gen.MessageOptions{
				Ref:               ref,
				Priority:          priority,
				ImportantDelivery: important,
			}

			var r error // result
			switch buf.B[49] {
			case 0:
				break
			case 1:
				r = gen.ErrProcessUnknown
			case 2:
				r = gen.ErrProcessMailboxFull
			case 3:
				r = gen.ErrProcessTerminated
			case 255:
				var ok bool

				msg, tail, err := edf.Decode(buf.B[50:], c.decodeOptions)
				if releaseBuffer {
					lib.ReleaseBuffer(buf)
				}

				if err != nil {
					c.log.Error("unable to decode received message: %s", err)
					continue
				}

				if len(tail) > 0 {
					c.log.Warning("message has extra bytes: %#v", tail)
				}

				r, ok = msg.(error)
				if ok == false {
					c.log.Error("received incorrect response error")
					continue
				}

			default:
				c.log.Error("received incorrect response error id")
				continue
			}
			err := c.core.RouteSendResponseError(from, to, opts, r)
			if important == false {
				continue
			}
			if err != nil {
				opts.ImportantDelivery = false
				c.SendResponseError(to, from, opts, err)
			}
			continue

		case protoMessageTerminatePID:
			if buf.Len() < 18 {
				c.log.Error("malformed message (too small MessageTerminatePID)")
				continue
			}
			// priority := gen.MessagePriority(buf.B[8]) ignored
			idTarget := binary.BigEndian.Uint64(buf.B[9:17])
			msg, tail, err := edf.Decode(buf.B[17:], c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			reason, ok := msg.(error)
			if ok == false {
				c.log.Error("received malformed TerminatePID message: %v", msg)
				continue
			}

			target := gen.PID{
				Node:     c.peer,
				ID:       idTarget,
				Creation: c.peer_creation,
			}
			c.core.RouteTerminatePID(target, reason)

		case protoMessageTerminateName, protoMessageTerminateNameCache:
			var data []byte

			if buf.Len() < 12 {
				c.log.Error("malformed message (too small MessageTerminateName*)")
				continue
			}

			processid := gen.ProcessID{
				Node: c.peer,
			}
			// priority := gen.MessagePriority(buf.B[8]) ignored
			if buf.B[7] == protoMessageTerminateName {
				l := int(buf.B[9])
				if buf.Len() < 10+l {
					c.log.Error("malformed message (too small MessageTerminateName)")
					continue
				}
				processid.Name = gen.Atom(buf.B[10 : 10+l])
				data = buf.B[10+l:]
			} else {
				id := binary.BigEndian.Uint16(buf.B[9:11])
				if c.decodeOptions.AtomCache == nil {
					c.log.Error("received TerminateName with cached atom value %d, but cache is nil (message ignored). please, report this bug", id)
					lib.ReleaseBuffer(buf)
					continue
				}

				v, found := c.decodeOptions.AtomCache.Load(id)
				if found == false {
					c.log.Error("received TerminateName with unknown atom cache id %d (message ignored). please, report this bug", id)
					lib.ReleaseBuffer(buf)
					continue
				}

				processid.Name = v.(gen.Atom)
				data = buf.B[11:]
			}

			if c.decodeOptions.AtomMapping != nil {
				if v, found := c.decodeOptions.AtomMapping.Load(processid.Name); found {
					processid.Name = v.(gen.Atom)
				}
			}

			msg, tail, err := edf.Decode(data, c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			reason, ok := msg.(error)
			if ok == false {
				c.log.Error("received malformed TerminateName message: %v", msg)
				continue
			}
			c.core.RouteTerminateProcessID(processid, reason)

		case protoMessageTerminateEvent, protoMessageTerminateEventCache:
			var data []byte

			if buf.Len() < 12 {
				c.log.Error("malformed message (too small MessageTerminateEvent*)")
				continue
			}

			event := gen.Event{
				Node: c.peer,
			}
			// priority := gen.MessagePriority(buf.B[8]) ignored
			if buf.B[7] == protoMessageTerminateEvent {
				l := int(buf.B[9])
				if buf.Len() < 10+l {
					c.log.Error("malformed message (too small MessageTerminateEvent)")
					continue
				}
				event.Name = gen.Atom(buf.B[10 : 10+l])
				data = buf.B[10+l:]
			} else {
				id := binary.BigEndian.Uint16(buf.B[9:11])
				if c.decodeOptions.AtomCache == nil {
					c.log.Error("received TerminateEvent with cached atom value %d, but cache is nil (message ignored). please, report this bug", id)
					lib.ReleaseBuffer(buf)
					continue
				}

				v, found := c.decodeOptions.AtomCache.Load(id)
				if found == false {
					c.log.Error("received TerminateEvent with unknown atom cache id %d (message ignored). please, report this bug", id)
					lib.ReleaseBuffer(buf)
					continue
				}

				event.Name = v.(gen.Atom)
				data = buf.B[11:]
			}

			if c.decodeOptions.AtomMapping != nil {
				if v, found := c.decodeOptions.AtomMapping.Load(event.Name); found {
					event.Name = v.(gen.Atom)
				}
			}

			msg, tail, err := edf.Decode(data, c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			reason, ok := msg.(error)
			if ok == false {
				c.log.Error("received malformed TerminateEvent message: %v", msg)
				continue
			}
			c.core.RouteTerminateEvent(event, reason)

		case protoMessageTerminateAlias:
			if buf.Len() < 34 {
				c.log.Error("malformed message (too small MessageTerminateAlias)")
				continue
			}

			target := gen.Alias{
				Node:     c.peer,
				Creation: c.peer_creation,
			}
			// priority := gen.MessagePriority(buf.B[8]) // ignored
			target.ID[0] = binary.BigEndian.Uint64(buf.B[9:17])
			target.ID[1] = binary.BigEndian.Uint64(buf.B[17:25])
			target.ID[2] = binary.BigEndian.Uint64(buf.B[25:33])

			msg, tail, err := edf.Decode(buf.B[33:], c.decodeOptions)
			if releaseBuffer {
				lib.ReleaseBuffer(buf)
			}

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			reason, ok := msg.(error)
			if ok == false {
				c.log.Error("received malformed TerminateAlias message: %v", msg)
				continue
			}

			c.core.RouteTerminateAlias(target, reason)

		case protoMessageAny:
			if buf.Len() < 9 {
				c.log.Error("malformed message (too small MessageAny)")
				continue
			}

			msg, tail, err := edf.Decode(buf.B[8:], c.decodeOptions)
			lib.ReleaseBuffer(buf)

			if err != nil {
				c.log.Error("unable to decode received message: %s", err)
				continue
			}

			if len(tail) > 0 {
				c.log.Warning("message has extra bytes: %#v", tail)
			}

			c.routeMessage(msg)

		case protoMessageZ:
			if buf.Len() < 10 {
				c.log.Error("malformed message (too small MessageZ)")
				continue
			}
			skipBytes := 9 // proto header for compressed message
			switch buf.B[8] {
			case gen.CompressionTypeGZIP.ID():
				dbuf, err := lib.DecompressGZIP(buf, uint(skipBytes))
				if err != nil {
					c.log.Error("unable to decompress message (gzip), ignored: %s", err)
					continue
				}
				lib.ReleaseBuffer(buf)
				buf = dbuf
				goto re

			case gen.CompressionTypeLZW.ID():
				dbuf, err := lib.DecompressLZW(buf, uint(skipBytes))
				if err != nil {
					c.log.Error("unable to decompress message (lzw), ignored")
					continue
				}
				lib.ReleaseBuffer(buf)
				buf = dbuf
				goto re

			case gen.CompressionTypeZLIB.ID():
				dbuf, err := lib.DecompressZLIB(buf, uint(skipBytes))
				if err != nil {
					c.log.Error("unable to decompress message (zlib), ignored")
					continue
				}
				lib.ReleaseBuffer(buf)
				buf = dbuf
				goto re

			default:
				c.log.Error("message with unknown compression type %d, ignored", buf.B[7])
				continue
			}

		// case protoMessageF:
		// TODO fragmentation
		// TODO check the message size after assembling

		// case protoMessageP:
		// TODO proxy

		default:
			c.log.Error("unknown/unsupported message type %d, ignored", buf.B[6])
			lib.ReleaseBuffer(buf)
		}

		// TODO
		// check if connection has been terminated
		// if c.terminated {
		//     return
		// }

	}
}

func (c *connection) read(conn net.Conn, buf *lib.Buffer) (*lib.Buffer, error) {
	total := buf.Len()
	expect := 8 // 8 bytes as a header
	// 1 byte - protoMagic
	// 1 byte - protoVersion
	// 4 bytes - message length
	// 1 byte - order (0 - no order, N - use the specific queue)
	// 1 byte - message type
	for {
		if buf.Len() < expect {
			readLimit := expect
			if c.node_maxmessagesize > 0 && readLimit > c.node_maxmessagesize {
				readLimit = c.node_maxmessagesize
			}

			n, e := buf.ReadDataFrom(conn, readLimit)
			if e != nil {
				if e == io.EOF {
					// something went wrong
					return nil, nil
				}
				return nil, e
			}

			total += n
			// check if we should get more data
			continue
		}

		l := int(binary.BigEndian.Uint32(buf.B[2:6]))

		if c.node_maxmessagesize > 0 && l > c.node_maxmessagesize {
			return nil, fmt.Errorf("received too long message (len: %d, limit: %d)", l, c.node_maxmessagesize)
		}

		if lib.Trace() {
			c.log.Trace("...recv buf.Len: %d, packet %d (expect: %d)", buf.Len(), l, expect)
		}

		if buf.Len() < l {
			expect = l
			continue
		}

		tail := lib.TakeBuffer()
		tail.Append(buf.B[l:total])

		buf.B = buf.B[:l]

		return tail, nil
	}
}

func (c *connection) routeMessage(msg any) {
	switch m := msg.(type) {
	case MessageResult:
		c.requestsMutex.RLock()
		ch, found := c.requests[m.Ref]
		c.requestsMutex.RUnlock()
		if found == false {
			// no one is wating. seems request was handled to long
			return
		}

		select {
		case ch <- m:
		default:
		}

	case MessageUpdateCache:
		for k, v := range m.AtomCache {
			entry, exist := c.decodeOptions.AtomCache.LoadOrStore(k, v)
			if exist {
				c.log.Warning("updating atom cache ignored entry (already exist): %d => %v", k, entry)
			}
		}
		for k, v := range m.AtomMapping {
			entry, exist := c.decodeOptions.AtomMapping.LoadOrStore(k, v)
			if exist {
				c.log.Warning("updating atom mapping ignored entry (already exist): %s => %v", k, entry)
			}
		}
		for k, v := range m.RegCache {
			entry, exist := c.decodeOptions.RegCache.LoadOrStore(k, v)
			if exist {
				c.log.Warning("updating reg cache ignored entry (already exist): %d => %v", k, entry)
			}
		}
		for k, v := range m.ErrCache {

			// TODO
			// check if error v is registered as a error on this node
			// and replase it by the local one

			entry, exist := c.decodeOptions.ErrCache.LoadOrStore(k, v)
			if exist {
				c.log.Warning("updating err cache ignored entry (already exist): %d => %v", k, entry)
			}
		}
		result := MessageResult{
			Ref: m.Ref,
		}
		order := uint8(0)
		orderPeer := uint8(0)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageLinkPID:
		// TODO check the source/target node name
		err := c.core.RouteLinkPID(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(m.Target.ID % 255)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageUnlinkPID:
		err := c.core.RouteUnlinkPID(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(m.Target.ID % 255)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageLinkProcessID:
		err := c.core.RouteLinkProcessID(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(0)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageUnlinkProcessID:
		err := c.core.RouteUnlinkProcessID(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(0)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageLinkAlias:
		err := c.core.RouteLinkAlias(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(m.Target.ID[1] % 255)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageUnlinkAlias:
		err := c.core.RouteUnlinkAlias(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(m.Target.ID[1] % 255)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageLinkEvent:
		r, err := c.core.RouteLinkEvent(m.Source, m.Target)
		result := MessageResult{
			Result: r,
			Error:  err,
			Ref:    m.Ref,
		}
		order := uint8(0)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageUnlinkEvent:
		err := c.core.RouteUnlinkEvent(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(0)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageMonitorPID:
		err := c.core.RouteMonitorPID(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(m.Target.ID % 255)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageDemonitorPID:
		err := c.core.RouteDemonitorPID(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(m.Target.ID % 255)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageMonitorProcessID:
		err := c.core.RouteMonitorProcessID(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(0)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageDemonitorProcessID:
		err := c.core.RouteDemonitorProcessID(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(0)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageMonitorAlias:
		err := c.core.RouteMonitorAlias(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(m.Target.ID[1] % 255)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageDemonitorAlias:
		err := c.core.RouteDemonitorAlias(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(m.Target.ID[1] % 255)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageMonitorEvent:
		r, err := c.core.RouteMonitorEvent(m.Source, m.Target)
		result := MessageResult{
			Result: r,
			Error:  err,
			Ref:    m.Ref,
		}
		order := uint8(0)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageDemonitorEvent:
		err := c.core.RouteDemonitorEvent(m.Source, m.Target)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(0)
		orderPeer := uint8(m.Source.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageSpawn:
		if c.node_flags.Enable && c.node_flags.EnableRemoteSpawn == false {
			c.log.Warning("remote spawn is not allowed for %s", c.peer)
			return
		}
		pid, err := c.core.RouteSpawn(c.core.Name(), m.Name, m.Options, c.peer)
		result := MessageResult{
			Error:  err,
			Result: pid,
			Ref:    m.Ref,
		}
		order := uint8(0)
		orderPeer := uint8(m.Options.ParentPID.ID % 255)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	case MessageApplicationStart:
		if c.node_flags.Enable && c.node_flags.EnableRemoteApplicationStart == false {
			c.log.Warning("remote application start is not allowed for %s", c.peer)
			return
		}
		err := c.core.RouteApplicationStart(m.Name, m.Mode, m.Options, c.peer)
		result := MessageResult{
			Error: err,
			Ref:   m.Ref,
		}
		order := uint8(0)
		orderPeer := uint8(0)
		c.sendAny(result, order, orderPeer, gen.Compression{})

	default:
		c.log.Error("recevied unsupported type of message: %T", msg)
	}
}

func (c *connection) sendAny(msg any, order uint8, orderPeer uint8, compression gen.Compression) error {
	buf := lib.TakeBuffer()
	buf.Allocate(8) // for the header

	if err := edf.Encode(msg, buf, c.encodeOptions); err != nil {
		return err
	}
	if buf.Len() > math.MaxUint32 {
		return gen.ErrTooLarge
	}
	buf.B[0] = protoMagic
	buf.B[1] = protoVersion
	binary.BigEndian.PutUint32(buf.B[2:6], uint32(buf.Len()))
	buf.B[6] = orderPeer
	buf.B[7] = protoMessageAny

	return c.send(buf, order, compression)
}

func (c *connection) wait() {
	c.wg.Wait()
}

func (c *connection) send(buf *lib.Buffer, order uint8, compression gen.Compression) error {

	if compression.Enable && buf.Len() > compression.Threshold {
		var zbuf *lib.Buffer
		var err error

		// 1 - protoMagic
		// 1 - protoVersion
		// 4 - length
		// 1 - order
		// 1 - protoMessageZ
		// 1 - compression type
		preallocate := uint(9)

		switch compression.Type {
		case gen.CompressionTypeZLIB:
			zbuf, err = lib.CompressZLIB(buf, preallocate)
			if err != nil {
				return fmt.Errorf("unable to compress packet (zlib): %s", err)
			}
		case gen.CompressionTypeLZW:
			zbuf, err = lib.CompressLZW(buf, preallocate)
			if err != nil {
				return fmt.Errorf("unable to compress packet (lzw): %s", err)
			}
		default:
			compression.Type = gen.CompressionTypeGZIP
			zbuf, err = lib.CompressGZIP(buf, preallocate, int(compression.Level))
			if err != nil {
				return fmt.Errorf("unable to compress packet (gzip): %s", err)
			}

		}
		zbuf.B[0] = protoMagic
		zbuf.B[1] = protoVersion
		binary.BigEndian.PutUint32(zbuf.B[2:6], uint32(zbuf.Len()))
		zbuf.B[6] = buf.B[6] // keep order of the original message
		zbuf.B[7] = protoMessageZ
		zbuf.B[8] = compression.Type.ID()

		lib.ReleaseBuffer(buf)
		buf = zbuf
	}

	if c.peer_maxmessagesize > 0 && buf.Len() > c.peer_maxmessagesize {
		return gen.ErrTooLarge
	}

	var pi *pool_item
	c.pool_mutex.RLock()
	l := len(c.pool)
	if l == 0 {
		c.pool_mutex.RUnlock()
		return gen.ErrNoConnection
	}
	if order == 0 {
		neworder := atomic.AddUint32(&c.order, 1)
		n := int(neworder) % l
		pi = c.pool[n]
	} else {
		n := int(order) % l
		pi = c.pool[n]
	}
	c.pool_mutex.RUnlock()

	atomic.AddUint64(&c.messagesOut, 1)
	atomic.AddUint64(&c.bytesOut, uint64(buf.Len()))

	// TODO
	// add proxy, fragmentation support
	// c.transitOut++
	// if buf.Len() < protoFragmentSize {

	pi.fl.Write(buf.B)
	lib.ReleaseBuffer(buf)
	return nil

	// }

	// message must be fragmented
	// panic("TODO")
}

func (c *connection) waitResult(ref gen.Ref, ch chan MessageResult) (result MessageResult) {

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)
	timer.Reset(time.Second * time.Duration(gen.DefaultRequestTimeout))

	select {
	case <-timer.C:
		result.Error = gen.ErrTimeout
	case result = <-ch:
	}

	c.requestsMutex.Lock()
	delete(c.requests, ref)
	c.requestsMutex.Unlock()

	return
}
