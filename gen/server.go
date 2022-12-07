package gen

import (
	"fmt"
	"runtime"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

const (
	DefaultCallTimeout = 5
)

// ServerBehavior interface
type ServerBehavior interface {
	ProcessBehavior

	// methods below are optional

	// Init invoked on a start Server
	Init(process *ServerProcess, args ...etf.Term) error

	// HandleCast invoked if Server received message sent with ServerProcess.Cast.
	// Return ServerStatusStop to stop server with "normal" reason. Use ServerStatus(error)
	// for the custom reason
	HandleCast(process *ServerProcess, message etf.Term) ServerStatus

	// HandleCall invoked if Server got sync request using ServerProcess.Call
	HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)

	// HandleDirect invoked on a direct request made with Process.Direct
	HandleDirect(process *ServerProcess, ref etf.Ref, message interface{}) (interface{}, DirectStatus)

	// HandleInfo invoked if Server received message sent with Process.Send.
	HandleInfo(process *ServerProcess, message etf.Term) ServerStatus

	// Terminate invoked on a termination process. ServerProcess.State is not locked during
	// this callback.
	Terminate(process *ServerProcess, reason string)
}

// ServerStatus
type ServerStatus error
type DirectStatus error

var (
	ServerStatusOK     ServerStatus = nil
	ServerStatusStop   ServerStatus = fmt.Errorf("stop")
	ServerStatusIgnore ServerStatus = fmt.Errorf("ignore")

	DirectStatusOK     DirectStatus = nil
	DirectStatusIgnore DirectStatus = fmt.Errorf("ignore")
)

// ServerStatusStopWithReason
func ServerStatusStopWithReason(s string) ServerStatus {
	return ServerStatus(fmt.Errorf(s))
}

// Server is implementation of ProcessBehavior interface for Server objects
type Server struct {
	ServerBehavior
}

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
	counter         uint64 // total number of processed messages from mailBox
	currentFunction string

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

// CastAfter a simple wrapper for Process.SendAfter to send a message in fashion of 'gen_server:cast'
func (sp *ServerProcess) CastAfter(to interface{}, message etf.Term, after time.Duration) CancelFunc {
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), message})
	return sp.SendAfter(to, msg, after)
}

// Cast sends a message in fashion of 'gen_server:cast'. 'to' can be a Pid, registered local name
// or gen.ProcessID{RegisteredName, NodeName}
func (sp *ServerProcess) Cast(to interface{}, message etf.Term) error {
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), message})
	return sp.Send(to, msg)
}

// Call makes outgoing sync request in fashion of 'gen_server:call'.
// 'to' can be Pid, registered local name or gen.ProcessID{RegisteredName, NodeName}.
func (sp *ServerProcess) Call(to interface{}, message etf.Term) (etf.Term, error) {
	return sp.CallWithTimeout(to, message, DefaultCallTimeout)
}

// CallWithTimeout makes outgoing sync request in fashiod of 'gen_server:call' with given timeout.
func (sp *ServerProcess) CallWithTimeout(to interface{}, message etf.Term, timeout int) (etf.Term, error) {
	ref := sp.MakeRef()
	from := etf.Tuple{sp.Self(), ref}
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_call"), from, message})

	sp.PutSyncRequest(ref)
	if err := sp.Send(to, msg); err != nil {
		sp.CancelSyncRequest(ref)
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
		etf.Atom("user"),
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
		etf.Atom("user"),
	}
	to := ProcessID{"rex", node}
	return sp.Cast(to, message)
}

// SendReply sends a reply message to the sender made ServerProcess.Call request.
// Useful for the case with dispatcher and pool of workers: Dispatcher process
// forwards Call requests (asynchronously) within a HandleCall callback to the worker(s)
// using ServerProcess.Cast or ServerProcess.Send but returns ServerStatusIgnore
// instead of ServerStatusOK; Worker process sends result using ServerProcess.SendReply
// method with 'from' value received from the Dispatcher.
func (sp *ServerProcess) SendReply(from ServerFrom, reply etf.Term) error {
	var fromTag etf.Term
	var to etf.Term
	if from.ReplyByAlias {
		// Erlang gen_server:call uses improper list for the reply ['alias'|Ref]
		fromTag = etf.ListImproper{etf.Atom("alias"), from.Ref}
		to = etf.Alias(from.Ref)
	} else {
		fromTag = from.Ref
		to = from.Pid
	}

	if reply != nil {
		rep := etf.Tuple{fromTag, reply}
		return sp.Send(to, rep)
	}
	rep := etf.Tuple{fromTag, etf.Atom("nil")}
	return sp.Send(to, rep)
}

// Reply the handling process.Direct(...) calls can be done asynchronously
// using gen.DirectStatusIgnore as a returning status in the HandleDirect callback.
// In this case, you must reply manualy using gen.ServerProcess.Reply method in any other
// callback. If a caller has canceled this request due to timeout it returns lib.ErrReferenceUnknown
func (sp *ServerProcess) Reply(ref etf.Ref, reply etf.Term, err error) error {
	return sp.PutSyncReply(ref, reply, err)
}

// MessageCounter returns the total number of messages handled by Server callbacks: HandleCall,
// HandleCast, HandleInfo, HandleDirect
func (sp *ServerProcess) MessageCounter() uint64 {
	return sp.counter
}

// ProcessInit
func (gs *Server) ProcessInit(p Process, args ...etf.Term) (ProcessState, error) {
	behavior, ok := p.Behavior().(ServerBehavior)
	if !ok {
		return ProcessState{}, fmt.Errorf("ProcessInit: not a ServerBehavior")
	}
	ps := ProcessState{
		Process: p,
	}

	sp := &ServerProcess{
		ProcessState: ps,
		behavior:     behavior,

		// callbackWaitReply must be defined here, otherwise making a Call request
		// will not be able in the inherited object (locks on trying to send
		// a message to the nil channel)
		callbackWaitReply: make(chan *etf.Ref),
	}

	err := behavior.Init(sp, args...)
	if err != nil {
		return ProcessState{}, err
	}
	ps.State = sp
	return ps, nil
}

// ProcessLoop
func (gs *Server) ProcessLoop(ps ProcessState, started chan<- bool) string {
	sp, ok := ps.State.(*ServerProcess)
	if !ok {
		return "ProcessLoop: not a ServerBehavior"
	}

	channels := ps.ProcessChannels()
	sp.mailbox = channels.Mailbox
	sp.original = channels.Mailbox
	sp.deferred = make(chan ProcessMailboxMessage, cap(channels.Mailbox))
	sp.currentFunction = "Server:loop"
	sp.stop = make(chan string, 2)

	defer func() {
		if sp.waitReply == nil {
			return
		}
		// there is running callback goroutine that waiting for a reply. to get rid
		// of infinity lock (of this callback goroutine) we must provide a reader
		// for the callbackWaitReply channel (it writes a nil value to this channel
		// on exit)
		go sp.waitCallbackOrDeferr(nil)
	}()

	started <- true
	for {
		var message etf.Term
		var fromPid etf.Pid

		select {
		case ex := <-channels.GracefulExit:
			if sp.TrapExit() == false {
				sp.behavior.Terminate(sp, ex.Reason)
				return ex.Reason
			}
			// Enabled trap exit message. Transform exit signal
			// into MessageExit and send it to itself as a regular message
			// keeping the processing order right.
			// We should process this message after the others we got earlier
			// from the died process.
			message = MessageExit{
				Pid:    ex.From,
				Reason: ex.Reason,
			}
			// We can't write this message to the mailbox directly so use
			// the common way to send it to itself
			ps.Send(ps.Self(), message)
			continue

		case reason := <-sp.stop:
			sp.behavior.Terminate(sp, reason)
			return reason

		case msg := <-sp.mailbox:
			sp.mailbox = sp.original
			fromPid = msg.From
			message = msg.Message

		case <-sp.Context().Done():
			sp.behavior.Terminate(sp, "kill")
			return "kill"

		case direct := <-channels.Direct:
			sp.waitCallbackOrDeferr(direct)
			continue
		case sp.waitReply = <-sp.callbackWaitReply:
			continue
		}

		lib.Log("[%s] GEN_SERVER %s got message from %s", sp.NodeName(), sp.Self(), fromPid)

		switch m := message.(type) {
		case etf.Tuple:

			switch mtag := m.Element(1).(type) {
			case etf.Ref:
				// check if we waiting for reply
				if len(m) != 2 {
					break
				}
				sp.PutSyncReply(mtag, m.Element(2), nil)
				if sp.waitReply != nil && *sp.waitReply == mtag {
					sp.waitReply = nil
					// continue read sp.callbackWaitReply channel
					// to wait for the exit from the callback call
					sp.waitCallbackOrDeferr(nil)
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
					sp.waitCallbackOrDeferr(callMessage)
					continue

				case etf.Atom("$gen_cast"):
					if len(m) != 2 {
						// wrong $gen_cast message. ignore it
						break
					}
					castMessage := handleCastMessage{
						message: m.Element(2),
					}
					sp.waitCallbackOrDeferr(castMessage)
					continue
				}
			}

			lib.Log("[%s] GEN_SERVER %#v got simple message %#v", sp.NodeName(), sp.Self(), message)
			infoMessage := handleInfoMessage{
				message: message,
			}
			sp.waitCallbackOrDeferr(infoMessage)

		case handleCallMessage:
			sp.waitCallbackOrDeferr(message)
		case handleCastMessage:
			sp.waitCallbackOrDeferr(message)
		case handleInfoMessage:
			sp.waitCallbackOrDeferr(message)
		case ProcessDirectMessage:
			sp.waitCallbackOrDeferr(message)

		default:
			lib.Log("m: %#v", m)
			infoMessage := handleInfoMessage{
				message: m,
			}
			sp.waitCallbackOrDeferr(infoMessage)
		}
	}
}

// ServerProcess handlers

func (sp *ServerProcess) waitCallbackOrDeferr(message interface{}) {
	if sp.waitReply != nil {
		// already waiting for reply. deferr this message
		deferred := ProcessMailboxMessage{
			Message: message,
		}
		select {
		case sp.deferred <- deferred:
			// do nothing
		default:
			lib.Warning("deferred mailbox of %s[%q] is full. dropped message %v",
				sp.Self(), sp.Name(), message)
		}

		return
	}

	switch m := message.(type) {
	case handleCallMessage:
		go func() {
			sp.counter++
			sp.handleCall(m)
			sp.callbackWaitReply <- nil
		}()
	case handleCastMessage:
		go func() {
			sp.counter++
			sp.handleCast(m)
			sp.callbackWaitReply <- nil
		}()
	case handleInfoMessage:
		go func() {
			sp.counter++
			sp.handleInfo(m)
			sp.callbackWaitReply <- nil
		}()
	case ProcessDirectMessage:
		go func() {
			sp.counter++
			sp.handleDirect(m)
			sp.callbackWaitReply <- nil
		}()
	case nil:
		// it was called just to read the channel sp.callbackWaitReply

	default:
		lib.Warning("unknown message type in waitCallbackOrDeferr: %#v", message)
		return
	}

	select {

	//case <-sp.Context().Done():
	// do not read the context state. otherwise the goroutine with running callback
	// might lock forever on exit (or on making a Call request) as nobody read
	// the callbackWaitReply channel.

	case sp.waitReply = <-sp.callbackWaitReply:
		// not nil value means callback made a Call request and waiting for reply
		if sp.waitReply == nil && len(sp.deferred) > 0 {
			sp.mailbox = sp.deferred
		}
		return
	}
}

func (sp *ServerProcess) panicHandler() {
	if r := recover(); r != nil {
		pc, fn, line, _ := runtime.Caller(2)
		lib.Warning("Server terminated %s[%q]. Panic reason: %#v at %s[%s:%d]",
			sp.Self(), sp.Name(), r, runtime.FuncForPC(pc).Name(), fn, line)
		sp.stop <- "panic"
	}
}

func (sp *ServerProcess) handleDirect(direct ProcessDirectMessage) {
	if lib.CatchPanic() {
		defer sp.panicHandler()
	}

	cf := sp.currentFunction
	sp.currentFunction = "Server:HandleDirect"
	reply, status := sp.behavior.HandleDirect(sp, direct.Ref, direct.Message)
	sp.currentFunction = cf
	switch status {
	case DirectStatusIgnore:
		return
	default:
		sp.PutSyncReply(direct.Ref, reply, status)
	}
}

func (sp *ServerProcess) handleCall(m handleCallMessage) {
	if lib.CatchPanic() {
		defer sp.panicHandler()
	}

	cf := sp.currentFunction
	sp.currentFunction = "Server:HandleCall"
	reply, status := sp.behavior.HandleCall(sp, m.from, m.message)
	sp.currentFunction = cf
	switch status {
	case ServerStatusOK:
		sp.SendReply(m.from, reply)
	case ServerStatusIgnore:
		return
	case ServerStatusStop:
		sp.stop <- "normal"

	default:
		sp.stop <- status.Error()
	}
}

func (sp *ServerProcess) handleCast(m handleCastMessage) {
	if lib.CatchPanic() {
		defer sp.panicHandler()
	}

	cf := sp.currentFunction
	sp.currentFunction = "Server:HandleCast"
	status := sp.behavior.HandleCast(sp, m.message)
	sp.currentFunction = cf

	switch status {
	case ServerStatusOK, ServerStatusIgnore:
		return
	case ServerStatusStop:
		sp.stop <- "normal"
	default:
		sp.stop <- status.Error()
	}
}

func (sp *ServerProcess) handleInfo(m handleInfoMessage) {
	if lib.CatchPanic() {
		defer sp.panicHandler()
	}

	cf := sp.currentFunction
	sp.currentFunction = "Server:HandleInfo"
	status := sp.behavior.HandleInfo(sp, m.message)
	sp.currentFunction = cf
	switch status {
	case ServerStatusOK, ServerStatusIgnore:
		return
	case ServerStatusStop:
		sp.stop <- "normal"
	default:
		sp.stop <- status.Error()
	}
}

//
// default callbacks for Server interface
//

// Init
func (gs *Server) Init(process *ServerProcess, args ...etf.Term) error {
	return nil
}

// HanldeCast
func (gs *Server) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	lib.Warning("Server [%s] HandleCast: unhandled message %#v", process.Name(), message)
	return ServerStatusOK
}

// HandleInfo
func (gs *Server) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	lib.Warning("Server [%s] HandleCall: unhandled message %#v from %#v", process.Name(), message, from)
	return "ok", ServerStatusOK
}

// HandleDirect
func (gs *Server) HandleDirect(process *ServerProcess, ref etf.Ref, message interface{}) (interface{}, DirectStatus) {
	return nil, lib.ErrUnsupportedRequest
}

// HandleInfo
func (gs *Server) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	lib.Warning("Server [%s] HandleInfo: unhandled message %#v", process.Name(), message)
	return ServerStatusOK
}

// Terminate
func (gs *Server) Terminate(process *ServerProcess, reason string) {
	return
}
