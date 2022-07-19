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

//
// Server callbacks
//
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

	udpProcess.options = options
	if err := udpProcess.initHandlers(); err != nil {
		return err
	}

	if options.Port == 0 {
		return fmt.Errorf("UDP port must be defined")
	}
	if options.DeadlineTimeout < 1 {
		// we need to check the context if it was canceled to stop
		// reading and close the connection socket
		options.DeadlineTimeout = defaultUDPDeadlineTimeout
	}

	lc := net.ListenConfig{}
	ctx := process.Context()
	hostPort := net.JoinHostPort("", strconv.Itoa(int(options.Port)))
	pconn, err := lc.ListenPacket(process.Context(), "udp", hostPort)
	if err != nil {
		return err
	}

	udpProcess.packetConn = pconn
	udpProcess.State = udpProcess

	// start serving
	go udpProcess.serve()
	return nil
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
	defer func() {
		udpp.packetConn.Close()
	}()

	ctx := udpp.Context()
	deadlineTimeout := time.Second * time.Duration(udpp.options.DeadlineTimeout)
	buf := make([]byte, udpp.options.MaxPacketSize)
	stopserve := false

	l := uint64(udpp.options.NumHandlers)
	// make round robin using the counter value
	cnt := atomic.AddUint64(&udpp.counter, 1)
	// choose process as a handler for the packet received on this connection
	handlerProcessID = int(cnt % l)
	handlerProcess = *(*Process)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&udpp.pool[handlerProcessID]))))

	for {
		if ctx.Err() != nil {
			return nil
		}
		deadline := false
		if err := udpp.packetConn.SetReadDeadline(time.Now().Add(deadlineTimeout)); err == nil {
			deadline = true
		}
		//buf = buf[:0]
		n, a, err := udpp.packetConn.ReadFrom(buf)
		if n == 0 {
			if err, ok := e.(net.Error); deadline && ok && err.Timeout() {
				packet = messageUDPHandlerTimeout{}
				break
			}
			stopserve = true
		}
		if err != nil {
			fmt.Println("GOT ERR", a, err)
		}

		packet = messageUDPHandlerPacket{}
		break
	}

	for a := uint64(0); a < l; a++ {
		if ctx.Err() != nil {
			return nil
		}
	}
}
