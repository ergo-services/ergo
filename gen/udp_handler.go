package gen

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

type UDPHandlerBehavior interface {
	ServerBehavior

	// Mandatory callback
	HandlePacket(process *UDPHandlerProcess, data []byte, packet *UDPPacket)

	// Optional callbacks
	HandleTimeout(process *UDPHandlerProcess)

	HandleUDPHandlerCall(process *UDPHandlerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	HandleUDPHandlerCast(process *UDPHandlerProcess, message etf.Term) ServerStatus
	HandleUDPHandlerInfo(process *UDPHandlerProcess, message etf.Term) ServerStatus
	HandleUDPHandlerTerminate(process *UDPHandlerProcess, reason string)
}

type UDPHandler struct {
	Server
}

type UDPHandlerProcess struct {
	ServerProcess
	behavior UDPHandlerBehavior

	lastPacket  int64
	idleTimeout int
	id          int
}

type optsUDPHandler struct {
	id          int
	idleTimeout int
}
type messageUDPHandlerIdleCheck struct{}
type messageUDPHandlerPacket struct {
	data   *lib.Buffer
	packet UDPPacket
}
type messageUDPHandlerTimeout struct{}

func (udph *UDPHandler) Init(process *ServerProcess, args ...etf.Term) error {
	behavior, ok := process.Behavior().(UDPHandlerBehavior)
	if !ok {
		return fmt.Errorf("UDP: not a UDPHandlerBehavior")
	}
	handlerProcess := &UDPHandlerProcess{
		ServerProcess: *process,
		behavior:      behavior,
	}
	if len(args) == 0 {
		return fmt.Errorf("UDP: can not start with no args")
	}

	if a, ok := args[0].(optsUDPHandler); ok {
		handlerProcess.idleTimeout = a.idleTimeout
		handlerProcess.id = a.id
	} else {
		return fmt.Errorf("UDP: wrong args for the UDPHandler")
	}

	// do not inherit parent State
	handlerProcess.State = nil
	process.State = handlerProcess

	if handlerProcess.idleTimeout > 0 {
		process.CastAfter(process.Self(), messageUDPHandlerIdleCheck{}, 5*time.Second)
	}

	return nil
}

func (udph *UDPHandler) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	udpp := process.State.(*UDPHandlerProcess)
	return udpp.behavior.HandleUDPHandlerCall(udpp, from, message)
}

func (udph *UDPHandler) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	udpp := process.State.(*UDPHandlerProcess)
	switch m := message.(type) {
	case messageUDPHandlerIdleCheck:
		if time.Now().Unix()-udpp.lastPacket > int64(udpp.idleTimeout) {
			return ServerStatusStop
		}
		process.CastAfter(process.Self(), messageUDPHandlerIdleCheck{}, 5*time.Second)
	case messageUDPHandlerPacket:
		udpp.lastPacket = time.Now().Unix()
		tcpp.behavior.HandlePacket(tcpp, m.data.B, m.packet)
		lib.ReleaseBuffer(m.data)

	case messageUDPHandlerTimeout:
		fmt.Println("UDP SOCKET TIMEOUT")
		tcpp.behavior.HandleTimeout(tcpp)

	default:
		return udpp.behavior.HandleUDPHandlerCast(udpp, message)
	}
	return ServerStatusOK
}

func (udph *UDPHandler) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	udpp := process.State.(*UDPHandlerProcess)
	return udpp.behavior.HandleUDPHandlerInfo(udpp, message)
}

func (udph *UDPHandler) Terminate(process *ServerProcess, reason string) {
	udpp := process.State.(*UDPHandlerProcess)
	udpp.behavior.HandleUDPHandlerTerminate(udpp, reason)
}

//
// default callbacks
//

func (udph *UDPHandler) HandleTimeout(process *UDPHandlerProcess, conn *UDPConnection) {
	return
}

// HandleUDPHandlerCall
func (udph *UDPHandler) HandleUDPHandlerCall(process *UDPHandlerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	lib.Warning("HandleUDPHandlerCall: unhandled message (from %#v) %#v", from, message)
	return etf.Atom("ok"), ServerStatusOK
}

// HandleUDPHandlerCast
func (udph *UDPHandler) HandleUDPHandlerCast(process *UDPHandlerProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleUDPHandlerCast: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleUDPHandlerInfo
func (udph *UDPHandler) HandleUDPHandlerInfo(process *UDPHandlerProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleUDPHandlerInfo: unhandled message %#v", message)
	return ServerStatusOK
}
func (udph *UDPHandler) HandleUDPHandlerTerminate(process *UDPHandlerProcess, reason string) {
	return
}

// we should disable SetTrapExit for the UDPHandlerProcess by overriding it.
func (udpp *UDPHandlerProcess) SetTrapExit(trap bool) {
	lib.Warning("[%s] method 'SetTrapExit' is disabled for UDPHandlerProcess", udpp.Self())
}
