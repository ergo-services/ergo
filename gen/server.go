package gen

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
)

const (
	DefaultCallTimeout = 5
)

// ServerBehavior interface
type ServerBehavior interface {
	ProcessBehavior

	// Init invoked on a start Server
	Init(process *ServerProcess, args ...etf.Term) error

	// HandleCast invoked if Server received message sent with ServerProcess.Cast.
	// Return ServerStatusStop to stop server with "normal" reason. Use ServerStatus(error)
	// for the custom reason
	HandleCast(process *ServerProcess, message etf.Term) ServerStatus

	// HandleCall invoked if Server got sync request using ServerProcess.Call
	HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)

	// HandleDirect invoked on a direct request made with Process.Direct
	HandleDirect(process *ServerProcess, message interface{}) (interface{}, error)

	// HandleInfo invoked if Server received message sent with Process.Send.
	HandleInfo(process *ServerProcess, message etf.Term) ServerStatus

	// Terminate invoked on a termination process. ServerProcess.State is not locked during
	// this callback.
	Terminate(process *ServerProcess, reason string)
}

type ServerStatus error

var (
	ServerStatusOK     ServerStatus = nil
	ServerStatusStop   ServerStatus = fmt.Errorf("stop")
	ServerStatusIgnore ServerStatus = fmt.Errorf("ignore")
)

func ServerStatusStopWithReason(s string) ServerStatus {
	return ServerStatus(fmt.Errorf(s))
}

// Server is implementation of ProcessBehavior interface for Server objects
type Server struct{}

// ServerFrom
type ServerFrom struct {
	Pid          etf.Pid
	Ref          etf.Ref
	ReplyByAlias bool
}

// ServerState state of the Server process.
type ServerProcess struct {
	ProcessState

	behavior        ServerBehavior
	reductions      uint64 // we use this term to count total number of processed messages from mailBox
	currentFunction string
	trapExit        bool

	mailbox  <-chan ProcessMailboxMessage
	original <-chan ProcessMailboxMessage
	deferred chan ProcessMailboxMessage

	waitReply         *etf.Ref
	callbackWaitReply chan *etf.Ref
	stop              chan string
}

type handleCallMessage struct {
	from    ServerFrom
	message etf.Term
}

type handleCastMessage struct {
	message etf.Term
}

type handleInfoMessage struct {
	message etf.Term
}

// CastAfter simple wrapper for SendAfter to send '$gen_cast' message
func (sp *ServerProcess) CastAfter(to interface{}, message etf.Term, after time.Duration) context.CancelFunc {
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), message})
	return sp.SendAfter(to, msg, after)
}

// Cast sends a message in fashion of 'gen_cast'. 'to' can be a Pid, registered local name
// or gen.ProcessID{RegisteredName, NodeName}
func (sp *ServerProcess) Cast(to interface{}, message etf.Term) error {
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), message})
	return sp.Send(to, msg)
}

// Call makes outgoing sync request in fashion of 'gen_call'.
// 'to' can be Pid, registered local name or gen.ProcessID{RegisteredName, NodeName}.
// This method shouldn't be used outside of the actor. Use Direct method instead.
func (sp *ServerProcess) Call(to interface{}, message etf.Term) (etf.Term, error) {
	return sp.CallWithTimeout(to, message, DefaultCallTimeout)
}

// CallWithTimeout makes outgoing sync request in fashiod of 'gen_call' with given timeout.
// This method shouldn't be used outside of the actor. Use DirectWithTimeout method instead.
func (sp *ServerProcess) CallWithTimeout(to interface{}, message etf.Term, timeout int) (etf.Term, error) {
	ref := sp.MakeRef()
	from := etf.Tuple{sp.Self(), ref}
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_call"), from, message})
	if err := sp.SendSyncRequest(ref, to, msg); err != nil {
		return nil, err
	}
	sp.callbackWaitReply <- &ref
	value, err := sp.WaitSyncReply(ref, timeout)
	return value, err

}

// CallRPC evaluate rpc call with given node/MFA
func (sp *ServerProcess) CallRPC(node, module, function string, args ...etf.Term) (etf.Term, error) {
	return sp.CallRPCWithTimeout(DefaultCallTimeout, node, module, function, args...)
}

// CallRPCWithTimeout evaluate rpc call with given node/MFA and timeout
func (sp *ServerProcess) CallRPCWithTimeout(timeout int, node, module, function string, args ...etf.Term) (etf.Term, error) {
	lib.Log("[%s] RPC calling: %s:%s:%s", sp.NodeName(), node, module, function)

	message := etf.Tuple{
		etf.Atom("call"),
		etf.Atom(module),
		etf.Atom(function),
		etf.List(args),
		sp.Self(),
	}
	to := ProcessID{"rex", node}
	return sp.CallWithTimeout(to, message, timeout)
}

// CastRPC evaluate rpc cast with given node/MFA
func (sp *ServerProcess) CastRPC(node, module, function string, args ...etf.Term) error {
	lib.Log("[%s] RPC casting: %s:%s:%s", sp.NodeName(), node, module, function)
	message := etf.Tuple{
		etf.Atom("cast"),
		etf.Atom(module),
		etf.Atom(function),
		etf.List(args),
	}
	to := ProcessID{"rex", node}
	return sp.Cast(to, message)
}

func (gs *Server) ProcessInit(p Process, args ...etf.Term) (ProcessState, error) {
	behavior, ok := p.Behavior().(ServerBehavior)
	if !ok {
		return ProcessState{}, fmt.Errorf("ProcessInit: not a ServerBehavior")
	}
	ps := ProcessState{
		Process: p,
	}

	gsp := &ServerProcess{
		ProcessState:      ps,
		behavior:          behavior,
		callbackWaitReply: make(chan *etf.Ref),
		stop:              make(chan string, 2),
	}

	err := behavior.Init(gsp, args...)
	if err != nil {
		return ProcessState{}, err
	}
	ps.State = gsp
	return ps, nil
}

func (gs *Server) ProcessLoop(ps ProcessState, started chan<- bool) string {
	gsp, ok := ps.State.(*ServerProcess)
	if !ok {
		return "ProcessLoop: not a ServerBehavior"
	}

	channels := ps.ProcessChannels()
	gsp.mailbox = channels.Mailbox
	gsp.original = channels.Mailbox
	gsp.deferred = make(chan ProcessMailboxMessage, cap(channels.Mailbox))
	gsp.currentFunction = "Server:loop"

	started <- true
	for {
		var message etf.Term
		var fromPid etf.Pid

		select {
		case ex := <-channels.GracefulExit:
			if !gsp.TrapExit() {
				gsp.behavior.Terminate(gsp, ex.Reason)
				return ex.Reason
			}
			message = MessageExit{
				Pid:    ex.From,
				Reason: ex.Reason,
			}

		case reason := <-gsp.stop:
			gsp.behavior.Terminate(gsp, reason)
			return reason

		case msg := <-gsp.mailbox:
			fromPid = msg.From
			message = msg.Message

		case <-gsp.Context().Done():
			gsp.behavior.Terminate(gsp, "kill")
			return "kill"

		case direct := <-channels.Direct:
			gsp.waitCallbackOrDeferr(direct)
			continue
		}

		lib.Log("[%s] GEN_SERVER %s got message from %s", gsp.NodeName(), gsp.Self(), fromPid)

		gsp.reductions++

		if gsp.waitReply == nil && len(gsp.deferred) == 0 {
			gsp.mailbox = gsp.original
		}

		switch m := message.(type) {
		case etf.Tuple:

			switch mtag := m.Element(1).(type) {
			case etf.Ref:
				// check if we waiting for reply
				if len(m) != 2 {
					break
				}
				gsp.PutSyncReply(mtag, m.Element(2))
				if gsp.waitReply != nil && *gsp.waitReply == mtag {
					gsp.waitReply = nil
					if len(gsp.deferred) > 0 {
						gsp.mailbox = gsp.deferred
					}
					// continue read gsp.callbackWaitReply channel
					// to wait for the exit from the callback call
					gsp.waitCallbackOrDeferr(nil)
					continue
				}

			case etf.Atom:
				switch mtag {
				case etf.Atom("$gen_call"):

					var from ServerFrom
					var ok bool
					if len(m) != 3 {
						// wrong $gen_call message. ignore it
						break
					}

					fromTuple, ok := m.Element(2).(etf.Tuple)
					if !ok || len(fromTuple) != 2 {
						// not a tuple or has wrong value
						break
					}

					from.Pid, ok = fromTuple.Element(1).(etf.Pid)
					if !ok {
						// wrong Pid value
						break
					}

					correct := false
					switch v := fromTuple.Element(2).(type) {
					case etf.Ref:
						from.Ref = v
						correct = true
					case etf.List:
						var ok bool
						// was sent with "alias" [etf.Atom("alias"), etf.Ref]
						if len(v) != 2 {
							// wrong value
							break
						}
						if alias, ok := v.Element(1).(etf.Atom); !ok || alias != etf.Atom("alias") {
							// wrong value
							break
						}
						from.Ref, ok = v.Element(2).(etf.Ref)
						if !ok {
							// wrong value
							break
						}
						from.ReplyByAlias = true
						correct = true
					}

					if correct == false {
						break
					}

					callMessage := handleCallMessage{
						from:    from,
						message: m.Element(3),
					}
					gsp.waitCallbackOrDeferr(callMessage)
					continue

				case etf.Atom("$gen_cast"):
					if len(m) != 2 {
						// wrong $gen_cast message. ignore it
						break
					}
					castMessage := handleCastMessage{
						message: m.Element(2),
					}
					gsp.waitCallbackOrDeferr(castMessage)
					continue
				}
			}

			lib.Log("[%s] GEN_SERVER %#v got simple message %#v", gsp.NodeName(), gsp.Self(), message)
			infoMessage := handleInfoMessage{
				message: message,
			}
			gsp.waitCallbackOrDeferr(infoMessage)

		case handleCallMessage:
			gsp.waitCallbackOrDeferr(message)
		case handleCastMessage:
			gsp.waitCallbackOrDeferr(message)
		case handleInfoMessage:
			gsp.waitCallbackOrDeferr(message)
		case ProcessDirectMessage:
			gsp.waitCallbackOrDeferr(message)

		default:
			lib.Log("m: %#v", m)
			infoMessage := handleInfoMessage{
				message: m,
			}
			gsp.waitCallbackOrDeferr(infoMessage)
		}
	}
}

// ServerProcess handlers

func (gsp *ServerProcess) waitCallbackOrDeferr(message interface{}) {
	if gsp.waitReply != nil {
		// already waiting for reply. deferr this message
		deferred := ProcessMailboxMessage{
			Message: message,
		}
		select {
		case gsp.deferred <- deferred:
			// do nothing
		default:
			// deferred mailbox is full
		}
	} else {
		switch m := message.(type) {
		case handleCallMessage:
			go func() {
				gsp.handleCall(m)
				gsp.callbackWaitReply <- nil
			}()
		case handleCastMessage:
			go func() {
				gsp.handleCast(m)
				gsp.callbackWaitReply <- nil
			}()
		case handleInfoMessage:
			go func() {
				gsp.handleInfo(m)
				gsp.callbackWaitReply <- nil
			}()
		case ProcessDirectMessage:
			go func() {
				gsp.handleDirect(m)
				gsp.callbackWaitReply <- nil
			}()

		}
	}
	select {
	case <-gsp.Context().Done():
		// got request to kill this process
		return
	case gsp.waitReply = <-gsp.callbackWaitReply:
		// not nil value means callback made a Call request and waiting for reply
		return
	}
}

func (gsp *ServerProcess) panicHandler() {
	if r := recover(); r != nil {
		pc, fn, line, _ := runtime.Caller(2)
		fmt.Printf("Warning: Server terminated (name: %q) %v %#v at %s[%s:%d]\n",
			gsp.Name(), gsp.Self(), r, runtime.FuncForPC(pc).Name(), fn, line)
		gsp.stop <- "panic"
	}
}

func (gsp *ServerProcess) handleDirect(direct ProcessDirectMessage) {
	defer gsp.panicHandler()

	reply, err := gsp.behavior.HandleDirect(gsp, direct.Message)
	if err != nil {
		direct.Message = nil
		direct.Err = err
		direct.Reply <- direct
		return
	}

	direct.Message = reply
	direct.Err = nil
	direct.Reply <- direct
	return
}

func (gsp *ServerProcess) handleCall(m handleCallMessage) {
	defer gsp.panicHandler()

	cf := gsp.currentFunction
	gsp.currentFunction = "Server:HandleCall"
	reply, status := gsp.behavior.HandleCall(gsp, m.from, m.message)
	gsp.currentFunction = cf
	switch status {
	case ServerStatusOK:
		var fromTag etf.Term
		var to etf.Term
		if m.from.ReplyByAlias {
			// Erlang gen_server:call uses improper list for the reply ['alias'|Ref]
			fromTag = etf.ListImproper{etf.Atom("alias"), m.from.Ref}
			to = etf.Alias(m.from.Ref)
		} else {
			fromTag = m.from.Ref
			to = m.from.Pid
		}

		if reply != nil {
			rep := etf.Tuple{fromTag, reply}
			gsp.Send(to, rep)
			return
		}
		rep := etf.Tuple{fromTag, etf.Atom("nil")}
		gsp.Send(to, rep)
	case ServerStatusIgnore:
		return
	case ServerStatusStop:
		gsp.stop <- "normal"

	default:
		gsp.stop <- status.Error()
	}
}

func (gsp *ServerProcess) handleCast(m handleCastMessage) {
	defer gsp.panicHandler()

	cf := gsp.currentFunction
	gsp.currentFunction = "Server:HandleCast"
	status := gsp.behavior.HandleCast(gsp, m.message)
	gsp.currentFunction = cf

	switch status {
	case ServerStatusOK, ServerStatusIgnore:
		return
	case ServerStatusStop:
		gsp.stop <- "normal"
	default:
		gsp.stop <- status.Error()
	}
}

func (gsp *ServerProcess) handleInfo(m handleInfoMessage) {
	defer gsp.panicHandler()

	cf := gsp.currentFunction
	gsp.currentFunction = "Server:HandleInfo"
	status := gsp.behavior.HandleInfo(gsp, m.message)
	gsp.currentFunction = cf
	switch status {
	case ServerStatusOK, ServerStatusIgnore:
		return
	case ServerStatusStop:
		gsp.stop <- "normal"
	default:
		gsp.stop <- status.Error()
	}
}

//
// default callbacks for Server interface
//
func (gs *Server) Init(process *ServerProcess, args ...etf.Term) error {
	return nil
}

func (gs *Server) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	fmt.Printf("Server [%s] HandleCast: unhandled message %#v \n", process.Name(), message)
	return ServerStatusOK
}

func (gs *Server) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	fmt.Printf("Server [%s] HandleCall: unhandled message %#v from %#v \n", process.Name(), message, from)
	return "ok", ServerStatusOK
}

func (gs *Server) HandleDirect(process *ServerProcess, message interface{}) (interface{}, error) {
	return nil, ErrUnsupportedRequest
}

func (gs *Server) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	fmt.Printf("Server [%s] HandleInfo: unhandled message %#v \n", process.Name(), message)
	return ServerStatusOK
}

func (gs *Server) Terminate(process *ServerProcess, reason string) {
	return
}
