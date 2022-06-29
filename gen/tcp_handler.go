package gen

import (
	"fmt"
	"io"
	"net"
	"reflect"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

type TCPHandlerStatus error

var (
	TCPHandlerStatusOK    TCPHandlerStatus = nil
	TCPHandlerStatusMore  TCPHandlerStatus = fmt.Errorf("more")
	TCPHandlerStatusLeft  TCPHandlerStatus = fmt.Errorf("left")
	TCPHandlerStatusClose TCPHandlerStatus = fmt.Errorf("close")

	defaultQueueLength = 10
)

type TCPHandlerBehavior interface {
	ServerBehavior

	// Mandatory callback
	HandlePacket(process *TCPHandlerProcess, packet []byte, conn TCPConnection) (int, TCPHandlerStatus)

	// Optional callbacks
	HandleConnect(process *TCPHandlerProcess, conn TCPConnection) TCPHandlerStatus
	HandleDisconnect(process *TCPHandlerProcess, conn TCPConnection)
	HandleTimeout(process *TCPHandlerProcess, conn TCPConnection) TCPHandlerStatus

	HandleTCPHandlerCall(process *TCPHandlerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	HandleTCPHandlerCast(process *TCPHandlerProcess, message etf.Term) ServerStatus
	HandleTCPHandlerInfo(process *TCPHandlerProcess, message etf.Term) ServerStatus
	HandleTCPHandlerTerminate(process *TCPHandlerProcess, reason string)

	initHandler(parent Process, handler TCPHandlerBehavior, options TCPHandlerOptions)
}

type TCPHandler struct {
	Server

	parent   Process
	behavior TCPHandlerBehavior
	options  TCPHandlerOptions
	pool     []*Process
	counter  uint64
}

type TCPHandlerProcess struct {
	ServerProcess
	behavior TCPHandlerBehavior

	lastPacket  int64
	counter     int64
	idleTimeout int
	id          int
}

type TCPHandlerOptions struct {
	// QueueLength defines how many parallel requests can be directed to this process. Default value is 10.
	QueueLength int
	// NumHandlers defines how many handlers will be started. Default 1
	NumHandlers int
	// IdleTimeout defines how long (in seconds) keeps the started handler alive with no packets. Zero value makes the handler non-stop.
	IdleTimeout int
}
type optsTCPHandler struct {
	id          int
	idleTimeout int
}

type TCPConnection struct {
	Addr   net.Addr
	Socket io.Writer
}

type messageTCPHandlerIdleCheck struct{}
type messageTCPHandlerPacket struct {
	packet     []byte
	connection TCPConnection
}
type messageTCPHandlerConnect struct {
	connection TCPConnection
}
type messageTCPHandlerDisconnect struct {
	connection TCPConnection
}

type messageTCPHandlerTimeout struct {
	connection TCPConnection
}

func (tcph *TCPHandler) initHandler(parent Process, handler TCPHandlerBehavior, options TCPHandlerOptions) error {
	if options.NumHandlers < 1 {
		options.NumHandlers = 1
	}
	if options.IdleTimeout < 0 {
		options.IdleTimeout = 0
	}

	if options.QueueLength < 1 {
		options.QueueLength = defaultQueueLength
	}

	tcph.parent = parent
	tcph.behavior = handler
	tcph.options = options
	c := atomic.AddUint64(&tcph.counter, 1)
	if c > 1 {
		return fmt.Errorf("you can not use the same object more than once")
	}

	for i := 0; i < options.NumHandlers; i++ {
		p := tcph.startHandler(i, options.IdleTimeout)
		if p == nil {
			return fmt.Errorf("can not initialize handlers")
		}
		tcph.pool = append(tcph.pool, &p)
	}
	return nil
}

func (tcph *TCPHandler) servePacket() {
	var p Process

	l := uint64(tcph.options.NumHandlers)
	// make round robin using the counter value
	c := atomic.AddUint64(&tcph.counter, 1)

	// attempts
	for a := uint64(0); a < l; a++ {
		i := (c + a) % l

		p = *(*Process)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&tcph.pool[i]))))
		fmt.Println("PPP", p)

		//respawned:
		//	if r.Context().Err() != nil {
		//		// canceled by the client
		//		return
		//	}
		//	_, err := p.DirectWithTimeout(mr, timeout)
		//	switch err {
		//	case nil:
		//		webMessageRequestPool.Put(mr)
		//		return

		//	case lib.ErrProcessTerminated:
		//		mr.Lock()
		//		if mr.requestState > 0 {
		//			mr.Unlock()
		//			return
		//		}
		//		mr.Unlock()
		//		p = wh.startHandler(int(i), wh.options.IdleTimeout)
		//		atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&wh.pool[i])), unsafe.Pointer(&p))
		//		goto respawned

		//	case lib.ErrProcessBusy:
		//		continue

		//	case lib.ErrTimeout:
		//		mr.Lock()
		//		if mr.requestState == 2 {
		//			// timeout happened during the handling request
		//			mr.Unlock()
		//			webMessageRequestPool.Put(mr)
		//			return
		//		}
		//		mr.requestState = 1 // canceled
		//		mr.Unlock()
		//		w.WriteHeader(http.StatusGatewayTimeout)
		//		return

		//	default:
		//		lib.Warning("WebHandler %s return error: %s", p.Self(), err)
		//		mr.Lock()
		//		if mr.requestState > 0 {
		//			mr.Unlock()
		//			return
		//		}
		//		mr.Unlock()

		//		w.WriteHeader(http.StatusInternalServerError) // 500
		//		return
		//	}
	}

	// all handlers are busy
	name := reflect.ValueOf(tcph.behavior).Elem().Type().Name()
	lib.Warning("too many packets for %s", name)
	//w.WriteHeader(http.StatusServiceUnavailable) // 503
	//webMessageRequestPool.Put(mr)
}

func (tcph *TCPHandler) startHandler(id int, idleTimeout int) Process {
	opts := ProcessOptions{
		Context:       tcph.parent.Context(),
		DirectboxSize: uint16(tcph.options.QueueLength),
	}

	optsHandler := optsTCPHandler{id: id, idleTimeout: idleTimeout}
	p, err := tcph.parent.Spawn("", opts, tcph.behavior, optsHandler)
	if err != nil {
		lib.Warning("can not start TCPHandler: %s", err)
		return nil
	}
	return p
}

func (tcph *TCPHandler) Init(process *ServerProcess, args ...etf.Term) error {
	behavior, ok := process.Behavior().(TCPHandlerBehavior)
	if !ok {
		return fmt.Errorf("TCP: not a TCPHandlerBehavior")
	}
	handlerProcess := &TCPHandlerProcess{
		ServerProcess: *process,
		behavior:      behavior,
	}
	if len(args) == 0 {
		return fmt.Errorf("TCP: can not start with no args")
	}

	if a, ok := args[0].(optsTCPHandler); ok {
		handlerProcess.idleTimeout = a.idleTimeout
		handlerProcess.id = a.id
	} else {
		return fmt.Errorf("TCP: wrong args for the TCPHandler")
	}

	// do not inherit parent State
	handlerProcess.State = nil
	process.State = handlerProcess

	if handlerProcess.idleTimeout > 0 {
		process.CastAfter(process.Self(), messageTCPHandlerIdleCheck{}, 5*time.Second)
	}

	return nil
}

func (tcph *TCPHandler) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	tcpp := process.State.(*TCPHandlerProcess)
	return tcpp.behavior.HandleTCPHandlerCall(tcpp, from, message)
}

func (tcph *TCPHandler) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	tcpp := process.State.(*TCPHandlerProcess)
	switch message.(type) {
	case messageTCPHandlerIdleCheck:
		if time.Now().Unix()-tcpp.lastPacket > int64(tcpp.idleTimeout) {
			return ServerStatusStop
		}
		process.CastAfter(process.Self(), messageTCPHandlerIdleCheck{}, 5*time.Second)

	default:
		return tcpp.behavior.HandleTCPHandlerCast(tcpp, message)
	}
	return ServerStatusOK
}

func (tcph *TCPHandler) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	tcpp := process.State.(*TCPHandlerProcess)
	return tcpp.behavior.HandleTCPHandlerInfo(tcpp, message)
}

func (tcph *TCPHandler) HandleDirect(process *ServerProcess, ref etf.Ref, message interface{}) (interface{}, DirectStatus) {
	tcpp := process.State.(*TCPHandlerProcess)
	switch m := message.(type) {
	case *messageTCPHandlerPacket:
		tcpp.lastPacket = time.Now().Unix()
		tcpp.counter++
		return tcpp.behavior.HandlePacket(tcpp, m.packet, m.connection)
	case *messageTCPHandlerConnect:
		return nil, tcpp.behavior.HandleConnect(tcpp, m.connection)
	case *messageTCPHandlerDisconnect:
		tcpp.behavior.HandleDisconnect(tcpp, m.connection)
		return nil, DirectStatusOK
	case *messageTCPHandlerTimeout:
		return nil, tcpp.behavior.HandleTimeout(tcpp, m.connection)
	default:
		return nil, DirectStatusOK
	}
}

// HandleTCPHandlerCall
func (tcph *TCPHandler) HandleTCPHandlerCall(process *WebHandlerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	lib.Warning("HandleTCPHandlerCall: unhandled message (from %#v) %#v", from, message)
	return etf.Atom("ok"), ServerStatusOK
}

// HandleTCPHandlerCast
func (tcph *TCPHandler) HandleTCPHandlerCast(process *WebHandlerProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleTCPHandlerCast: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleTCPHandlerInfo
func (tcph *TCPHandler) HandleTCPHandlerInfo(process *WebHandlerProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleTCPHandlerInfo: unhandled message %#v", message)
	return ServerStatusOK
}
func (tcph *TCPHandler) HandleTCPHandlerTerminate(process *WebHandlerProcess, reason string, count int64) {
	return
}

// we should disable SetTrapExit for the TCPHandlerProcess by overriding it.
func (tcpp *TCPHandlerProcess) SetTrapExit(trap bool) {
	lib.Warning("[%s] method 'SetTrapExit' is disabled for TCPHandlerProcess", tcpp.Self())
}
