package gen

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
)

// ServerBehavior interface
type ServerBehavior interface {
	ProcessBehavior

	// Init(...) -> error
	Init(state *ServerProcess, args ...etf.Term) error

	// HandleCast -> "noreply" - noreply
	//				"stop" - stop with reason "normal"
	//		         "reason" - stop with given "reason"
	HandleCast(state *ServerProcess, message etf.Term) string

	// HandleCall -> ("reply", message) - reply
	//				 ("noreply", _) - noreply
	//		         ("stop", _) - stop with reason "normal"
	//				 ("reason", _) - stop with given "reason"
	HandleCall(state *ServerProcess, from ServerFrom, message etf.Term) (string, etf.Term)

	// HandleDirect invoked on a direct request made via Process.Direct
	HandleDirect(state *ServerProcess, message interface{}) (interface{}, error)

	// HandleInfo -> "noreply" - noreply
	//				"stop" - stop with reason "normal"
	//		         "reason" - stop with given "reason"
	HandleInfo(state *ServerProcess, message etf.Term) string

	Terminate(state *ServerProcess, reason string)
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
}

func (gs *Server) ProcessInit(p Process, args ...etf.Term) (ProcessState, error) {
	behavior := p.Behavior().(ServerBehavior)
	//behavior, ok := p.Behavior().(ServerBehavior)
	//if !ok {
	//	return ProcessState{}, fmt.Errorf("ProcessInit: not a ServerBehavior")
	//}
	gsp := &ServerProcess{
		ProcessState: ProcessState{
			Process: p,
		},
	}
	err := behavior.Init(gsp, args...)
	if err != nil {
		return ProcessState{}, err
	}

	return gsp.ProcessState, nil
}

func (gs *Server) ProcessLoop(ps ProcessState, started chan<- bool) string {
	behavior, ok := ps.Behavior().(ServerBehavior)
	if !ok {
		return "ProcessLoop: not a ServerBehavior"
	}
	gsp := &ServerProcess{
		ProcessState: ps,
		behavior:     behavior,
	}

	lockState := &sync.Mutex{}
	stop := make(chan string, 2)

	gsp.currentFunction = "Server:loop"
	chs := gsp.ProcessChannels()

	started <- true
	for {
		var message etf.Term
		var fromPid etf.Pid

		select {
		case ex := <-chs.GracefulExit:
			if !gsp.TrapExit() {
				gsp.behavior.Terminate(gsp, ex.Reason)
				return ex.Reason
			}
			message = MessageExit{
				Pid:    ex.From,
				Reason: ex.Reason,
			}

		case reason := <-stop:
			gsp.behavior.Terminate(gsp, reason)
			return reason

		case msg := <-chs.Mailbox:
			fromPid = msg.From
			message = msg.Message

		case <-gsp.Context().Done():
			gsp.behavior.Terminate(gsp, "kill")
			return "kill"

		case direct := <-chs.Direct:
			reply, err := gsp.behavior.HandleDirect(gsp, direct.Message)
			if err != nil {
				direct.Message = nil
				direct.Err = err
				direct.Reply <- direct
				continue
			}

			direct.Message = reply
			direct.Err = nil
			direct.Reply <- direct
			continue
		}

		lib.Log("[%s] GEN_SERVER %s got message from %s", gsp.NodeName(), gsp.Self(), fromPid)

		gsp.reductions++

		panicHandler := func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				fmt.Printf("Warning: Server recovered (name: %s) %v %#v at %s[%s:%d]\n",
					gsp.Name(), gsp.Self(), r, runtime.FuncForPC(pc).Name(), fn, line)
				stop <- "panic"
			}
		}

		switch m := message.(type) {
		case etf.Tuple:
			switch mtag := m.Element(1).(type) {
			case etf.Atom:
				switch mtag {
				case etf.Atom("$gen_call"):
					// We need to wrap it out using goroutine in order to serve
					// sync-requests (like 'process.Call') within callback execution
					// since reply (etf.Ref) comes through the same mailBox channel
					go func() {
						defer panicHandler()

						var ok bool
						if len(m) != 3 {
							// wrong $gen_call message. ignore it
							return
						}

						fromTuple, ok := m.Element(2).(etf.Tuple)
						if !ok || len(fromTuple) != 2 {
							// not a tuple or has wrong value
							return
						}

						from := ServerFrom{}

						from.Pid, ok = fromTuple.Element(1).(etf.Pid)
						if !ok {
							// wrong Pid value
							return
						}

						switch v := fromTuple.Element(2).(type) {
						case etf.Ref:
							from.Ref = v
						case etf.List:
							var ok bool
							// was sent with "alias" [etf.Atom("alias"), etf.Ref]
							if len(v) != 2 {
								// wrong value
								return
							}
							if alias, ok := v.Element(1).(etf.Atom); !ok || alias != etf.Atom("alias") {
								// wrong value
								return
							}
							from.Ref, ok = v.Element(2).(etf.Ref)
							if !ok {
								// wrong value
								return
							}
							from.ReplyByAlias = true

						default:
							// wrong tag value
							return
						}

						lockState.Lock()
						defer lockState.Unlock()

						cf := gsp.currentFunction
						gsp.currentFunction = "Server:HandleCall"
						code, reply := gsp.behavior.HandleCall(gsp, from, m.Element(3))
						gsp.currentFunction = cf
						switch code {
						case "reply":
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
								gsp.Send(to, rep)
								return
							}
							rep := etf.Tuple{fromTag, etf.Atom("nil")}
							gsp.Send(to, rep)
						case "noreply":
							return
						case "stop":
							stop <- "normal"

						default:
							stop <- reply.(string)
						}
					}()

				case etf.Atom("$gen_cast"):
					go func() {
						defer panicHandler()

						lockState.Lock()
						defer lockState.Unlock()

						cf := gsp.currentFunction
						gsp.currentFunction = "Server:HandleCast"
						code := gsp.behavior.HandleCast(gsp, m.Element(2))
						gsp.currentFunction = cf

						switch code {
						case "noreply":
							return
						case "stop":
							stop <- "normal"
						default:
							stop <- code
						}
					}()

				default:
					go func() {
						defer panicHandler()

						lockState.Lock()
						defer lockState.Unlock()

						cf := gsp.currentFunction
						gsp.currentFunction = "Server:HandleInfo"
						code := gsp.behavior.HandleInfo(gsp, message)
						gsp.currentFunction = cf
						switch code {
						case "noreply":
							return
						case "stop":
							stop <- "normal"
						default:
							stop <- code
						}
					}()

				}

			default:
				if ref, ok := m.Element(1).(etf.Ref); ok && len(m) == 2 {
					lib.Log("[%s] GEN_SERVER %#v got reply: %#v", gsp.NodeName(), gsp.Self(), mtag)
					gsp.PutSyncReply(ref, m.Element(2))
					continue
				}

				lib.Log("[%s] GEN_SERVER %#v got simple message %#v", gsp.NodeName(), gsp.Self(), mtag)
				go func() {
					defer panicHandler()

					lockState.Lock()
					defer lockState.Unlock()

					cf := gsp.currentFunction
					gsp.currentFunction = "Server:HandleInfo"
					code := gsp.behavior.HandleInfo(gsp, message)
					gsp.currentFunction = cf

					switch code {
					case "noreply":
						return
					case "stop":
						stop <- "normal"
					default:
						stop <- code
					}
				}()
			}

		default:
			lib.Log("m: %#v", m)
			go func() {
				defer panicHandler()

				lockState.Lock()
				defer lockState.Unlock()

				cf := gsp.currentFunction
				gsp.currentFunction = "Server:HandleInfo"
				code := gsp.behavior.HandleInfo(gsp, message)
				gsp.currentFunction = cf

				switch code {
				case "noreply":
					return
				case "stop":
					stop <- "normal"
				default:
					stop <- code
				}
			}()
		}
	}
}

//
// default callbacks for Server interface
//
func (gs *Server) Init(process *ServerProcess, args ...etf.Term) error {
	return nil
}

func (gs *Server) HandleCast(process *ServerProcess, message etf.Term) string {
	fmt.Printf("Server [%s] HandleCast: unhandled message %#v \n", process.Name(), message)
	return "noreply"
}

func (gs *Server) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (string, etf.Term) {
	fmt.Printf("Server [%s] HandleCall: unhandled message %#v from %#v \n", process.Name(), message, from)
	return "reply", "ok"
}

func (gs *Server) HandleDirect(process *ServerProcess, message interface{}) (interface{}, error) {
	return nil, ErrUnsupportedRequest
}

func (gs *Server) HandleInfo(process *ServerProcess, message etf.Term) string {
	fmt.Printf("Server [%s] HandleInfo: unhandled message %#v \n", process.Name(), message)
	return "noreply"
}

func (gs *Server) Terminate(process *ServerProcess, reason string) {
	return
}
