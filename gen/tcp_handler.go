package gen

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

type TCPHandlerStatus error

var (
	TCPHandlerStatusOK    TCPHandlerStatus = nil
	TCPHandlerStatusNext  TCPHandlerStatus = fmt.Errorf("next")
	TCPHandlerStatusClose TCPHandlerStatus = fmt.Errorf("close")

	defaultQueueLength = 10
)

type TCPHandlerBehavior interface {
	ServerBehavior

	// Mandatory callback
	HandlePacket(process *TCPHandlerProcess, packet []byte, conn TCPConnection) (int, int, TCPHandlerStatus)

	// Optional callbacks
	HandleConnect(process *TCPHandlerProcess, conn TCPConnection) TCPHandlerStatus
	HandleDisconnect(process *TCPHandlerProcess, conn TCPConnection)
	HandleTimeout(process *TCPHandlerProcess, conn TCPConnection) TCPHandlerStatus

	HandleTCPHandlerCall(process *TCPHandlerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	HandleTCPHandlerCast(process *TCPHandlerProcess, message etf.Term) ServerStatus
	HandleTCPHandlerInfo(process *TCPHandlerProcess, message etf.Term) ServerStatus
	HandleTCPHandlerTerminate(process *TCPHandlerProcess, reason string)
}

type TCPHandler struct {
	Server

	behavior TCPHandlerBehavior
}

type TCPHandlerProcess struct {
	ServerProcess
	behavior TCPHandlerBehavior

	lastPacket  int64
	idleTimeout int
	id          int
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
type messageTCPHandlerPacketResult struct {
	left  int
	await int
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
		left, await, err := tcpp.behavior.HandlePacket(tcpp, m.packet, m.connection)
		res := messageTCPHandlerPacketResult{
			left:  left,
			await: await,
		}
		return res, err
	case messageTCPHandlerConnect:
		return nil, tcpp.behavior.HandleConnect(tcpp, m.connection)
	case messageTCPHandlerDisconnect:
		tcpp.behavior.HandleDisconnect(tcpp, m.connection)
		return nil, DirectStatusOK
	case messageTCPHandlerTimeout:
		return nil, tcpp.behavior.HandleTimeout(tcpp, m.connection)
	default:
		return nil, DirectStatusOK
	}
}

func (tcph *TCPHandler) Terminate(process *ServerProcess, reason string) {
	fmt.Println("TERMINATED", process.Self())
	tcpp := process.State.(*TCPHandlerProcess)
	tcpp.behavior.HandleTCPHandlerTerminate(tcpp, reason)
}

//
// default callbacks
//

func (tcph *TCPHandler) HandleConnect(process *TCPHandlerProcess, conn TCPConnection) TCPHandlerStatus {
	return TCPHandlerStatusOK
}
func (tcph *TCPHandler) HandleDisconnect(process *TCPHandlerProcess, conn TCPConnection) {
	return
}
func (tcph *TCPHandler) HandleTimeout(process *TCPHandlerProcess, conn TCPConnection) TCPHandlerStatus {
	return TCPHandlerStatusOK
}

// HandleTCPHandlerCall
func (tcph *TCPHandler) HandleTCPHandlerCall(process *TCPHandlerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	lib.Warning("HandleTCPHandlerCall: unhandled message (from %#v) %#v", from, message)
	return etf.Atom("ok"), ServerStatusOK
}

// HandleTCPHandlerCast
func (tcph *TCPHandler) HandleTCPHandlerCast(process *TCPHandlerProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleTCPHandlerCast: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleTCPHandlerInfo
func (tcph *TCPHandler) HandleTCPHandlerInfo(process *TCPHandlerProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleTCPHandlerInfo: unhandled message %#v", message)
	return ServerStatusOK
}
func (tcph *TCPHandler) HandleTCPHandlerTerminate(process *TCPHandlerProcess, reason string) {
	return
}

// we should disable SetTrapExit for the TCPHandlerProcess by overriding it.
func (tcpp *TCPHandlerProcess) SetTrapExit(trap bool) {
	lib.Warning("[%s] method 'SetTrapExit' is disabled for TCPHandlerProcess", tcpp.Self())
}
