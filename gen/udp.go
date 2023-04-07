package gen

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

type UDPBehavior interface {
	ServerBehavior

	InitUDP(process *UDPProcess, args ...etf.Term) (UDPOptions, error)

	HandleUDPCall(process *UDPProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	HandleUDPCast(process *UDPProcess, message etf.Term) ServerStatus
	HandleUDPInfo(process *UDPProcess, message etf.Term) ServerStatus

	HandleUDPTerminate(process *UDPProcess, reason string)
}

type UDPStatus error

var (
	UDPStatusOK   UDPStatus
	UDPStatusStop UDPStatus = fmt.Errorf("stop")

	defaultUDPDeadlineTimeout int = 3
	defaultUDPQueueLength     int = 10
	defaultUDPMaxPacketSize       = int(65000)
)

type UDP struct {
	Server
}

type UDPOptions struct {
	Host string
	Port uint16

	Handler         UDPHandlerBehavior
	NumHandlers     int
	IdleTimeout     int
	DeadlineTimeout int
	QueueLength     int
	MaxPacketSize   int
	ExtraHandlers   bool
}

type UDPProcess struct {
	ServerProcess
	options  UDPOptions
	behavior UDPBehavior

	pool       []*Process
	counter    uint64
	packetConn net.PacketConn
}

type UDPPacket struct {
	Addr   net.Addr
	Socket io.Writer
}

// Server callbacks
func (udp *UDP) Init(process *ServerProcess, args ...etf.Term) error {

	behavior := process.Behavior().(UDPBehavior)
	behavior, ok := process.Behavior().(UDPBehavior)
	if !ok {
		return fmt.Errorf("not a UDPBehavior")
	}

	udpProcess := &UDPProcess{
		ServerProcess: *process,
		behavior:      behavior,
	}
	// do not inherit parent State
	udpProcess.State = nil

	options, err := behavior.InitUDP(udpProcess, args...)
	if err != nil {
		return err
	}
	if options.Handler == nil {
		return fmt.Errorf("handler must be defined")
	}

	if options.QueueLength == 0 {
		options.QueueLength = defaultUDPQueueLength
	}

	if options.DeadlineTimeout < 1 {
		// we need to check the context if it was canceled to stop
		// reading and close the connection socket
		options.DeadlineTimeout = defaultUDPDeadlineTimeout
	}

	if options.MaxPacketSize == 0 {
		options.MaxPacketSize = defaultUDPMaxPacketSize
	}

	udpProcess.options = options
	if err := udpProcess.initHandlers(); err != nil {
		return err
	}

	if options.Port == 0 {
		return fmt.Errorf("UDP port must be defined")
	}

	lc := net.ListenConfig{}
	hostPort := net.JoinHostPort("", strconv.Itoa(int(options.Port)))
	pconn, err := lc.ListenPacket(process.Context(), "udp", hostPort)
	if err != nil {
		return err
	}

	udpProcess.packetConn = pconn
	process.State = udpProcess

	// start serving
	go udpProcess.serve()
	return nil
}

func (udp *UDP) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	udpp := process.State.(*UDPProcess)
	return udpp.behavior.HandleUDPCall(udpp, from, message)
}

func (udp *UDP) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	udpp := process.State.(*UDPProcess)
	return udpp.behavior.HandleUDPCast(udpp, message)
}

func (udp *UDP) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	udpp := process.State.(*UDPProcess)
	return udpp.behavior.HandleUDPInfo(udpp, message)
}

func (udp *UDP) Terminate(process *ServerProcess, reason string) {
	p := process.State.(*UDPProcess)
	p.packetConn.Close()
	p.behavior.HandleUDPTerminate(p, reason)
}

//
// default UDP callbacks
//

// HandleUDPCall
func (udp *UDP) HandleUDPCall(process *UDPProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	lib.Warning("[gen.UDP] HandleUDPCall: unhandled message (from %#v) %#v", from, message)
	return etf.Atom("ok"), ServerStatusOK
}

// HandleUDPCast
func (udp *UDP) HandleUDPCast(process *UDPProcess, message etf.Term) ServerStatus {
	lib.Warning("[gen.UDP] HandleUDPCast: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleUDPInfo
func (udp *UDP) HandleUDPInfo(process *UDPProcess, message etf.Term) ServerStatus {
	lib.Warning("[gen.UDP] HandleUDPInfo: unhandled message %#v", message)
	return ServerStatusOK
}
func (udp *UDP) HandleUDPTerminate(process *UDPProcess, reason string) {
	return
}

// internals

func (udpp *UDPProcess) initHandlers() error {
	if udpp.options.NumHandlers < 1 {
		udpp.options.NumHandlers = 1
	}
	if udpp.options.IdleTimeout < 0 {
		udpp.options.IdleTimeout = 0
	}

	c := atomic.AddUint64(&udpp.counter, 1)
	if c > 1 {
		return fmt.Errorf("you can not use the same object more than once")
	}

	for i := 0; i < udpp.options.NumHandlers; i++ {
		p := udpp.startHandler(i, udpp.options.IdleTimeout)
		if p == nil {
			return fmt.Errorf("can not initialize handlers")
		}
		udpp.pool = append(udpp.pool, &p)
	}
	return nil
}

func (udpp *UDPProcess) startHandler(id int, idleTimeout int) Process {
	opts := ProcessOptions{
		Context:     udpp.Context(),
		MailboxSize: uint16(udpp.options.QueueLength),
	}

	optsHandler := optsUDPHandler{id: id, idleTimeout: idleTimeout}
	p, err := udpp.Spawn("", opts, udpp.options.Handler, optsHandler)
	if err != nil {
		lib.Warning("[gen.UDP] can not start UDPHandler: %s", err)
		return nil
	}
	return p
}

func (udpp *UDPProcess) serve() {
	var handlerProcess Process
	var handlerProcessID int
	var packet interface{}
	defer udpp.packetConn.Close()

	writer := &writer{
		pconn: udpp.packetConn,
	}

	ctx := udpp.Context()
	deadlineTimeout := time.Second * time.Duration(udpp.options.DeadlineTimeout)

	l := uint64(udpp.options.NumHandlers)
	// make round robin using the counter value
	cnt := atomic.AddUint64(&udpp.counter, 1)
	// choose process as a handler for the packet received on this connection
	handlerProcessID = int(cnt % l)
	handlerProcess = *(*Process)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&udpp.pool[handlerProcessID]))))

nextPacket:
	for {
		if ctx.Err() != nil {
			return
		}
		deadline := false
		if err := udpp.packetConn.SetReadDeadline(time.Now().Add(deadlineTimeout)); err == nil {
			deadline = true
		}
		buf := lib.TakeBuffer()
		buf.Allocate(udpp.options.MaxPacketSize)
		n, a, err := udpp.packetConn.ReadFrom(buf.B)
		if n == 0 {
			if err, ok := err.(net.Error); deadline && ok && err.Timeout() {
				packet = messageUDPHandlerTimeout{}
				break
			}
			// stop serving and close this socket
			return
		}
		if err != nil {
			lib.Warning("[gen.UDP] got error on receiving packet from %q: %s", a, err)
		}

		writer.addr = a
		packet = messageUDPHandlerPacket{
			data: buf,
			packet: UDPPacket{
				Addr:   a,
				Socket: writer,
			},
			n: n,
		}
		break
	}

retry:
	for a := uint64(0); a < l; a++ {
		if ctx.Err() != nil {
			return
		}

		err := udpp.Cast(handlerProcess.Self(), packet)
		switch err {
		case nil:
			break
		case lib.ErrProcessUnknown:
			if handlerProcessID == -1 {
				// it was an extra handler do not restart. try to use the existing one
				cnt = atomic.AddUint64(&udpp.counter, 1)
				handlerProcessID = int(cnt % l)
				handlerProcess = *(*Process)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&udpp.pool[handlerProcessID]))))
				goto retry
			}

			// respawn terminated process
			handlerProcess = udpp.startHandler(handlerProcessID, udpp.options.IdleTimeout)
			atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&udpp.pool[handlerProcessID])), unsafe.Pointer(&handlerProcess))
			continue

		case lib.ErrProcessBusy:
			handlerProcessID = int((a + cnt) % l)
			handlerProcess = *(*Process)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&udpp.pool[handlerProcessID]))))
			continue
		default:
			lib.Warning("[gen.UDP] error on handling packet %#v: %s", packet, err)
		}
		goto nextPacket
	}
}

type writer struct {
	pconn net.PacketConn
	addr  net.Addr
}

func (w *writer) Write(data []byte) (int, error) {
	return w.pconn.WriteTo(data, w.addr)
}
