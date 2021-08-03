package ergo

import (
	"fmt"
	"sync"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
)

const (
	DefaultCallTimeout = 5
)

// GenServerBehavior interface
type GenServerBehavior interface {
	// Init(...) -> error
	Init(state *GenServerState, args ...interface{}) error

	// HandleCast -> "noreply" - noreply
	//				"stop" - stop with reason "normal"
	//		         "reason" - stop with given "reason"
	HandleCast(state *GenServerState, message etf.Term) string

	// HandleCall -> ("reply", message) - reply
	//				 ("noreply", _) - noreply
	//		         ("stop", _) - stop with reason "normal"
	//				 ("reason", _) - stop with given "reason"
	HandleCall(state *GenServerState, from GenServerFrom, message etf.Term) (string, etf.Term)

	// HandleDirect invoked on a direct request made via Process.Direct
	HandleDirect(state *GenServerState, message interface{}) (interface{}, error)

	// HandleInfo -> "noreply" - noreply
	//				"stop" - stop with reason "normal"
	//		         "reason" - stop with given "reason"
	HandleInfo(state *GenServerState, message etf.Term) string

	Terminate(state *GenServerState, reason string)
}

// GenServer is implementation of ProcessBehavior interface for GenServer objects
type GenServer struct{}

// GenServerFrom
type GenServerFrom struct {
	Pid           etf.Pid
	Ref           etf.Ref
	CalledByAlias bool
}

// GenServerState state of the GenServer process.
type GenServerState struct {
	Process *Process
	State   interface{}
}

func (gs *GenServer) Loop(p *Process, args ...interface{}) string {
	lockState := &sync.Mutex{}

	state := &GenServerState{
		Process: p,
	}
	err := p.object.(GenServerBehavior).Init(state, args...)
	if err != nil {
		return err.Error()
	}
	p.ready <- nil

	stop := make(chan string, 2)

	p.currentFunction = "GenServer:loop"

	for {
		var message etf.Term
		var fromPid etf.Pid

		select {
		case ex := <-p.gracefulExit:
			p.object.(GenServerBehavior).Terminate(state, ex.reason)
			return ex.reason

		case reason := <-stop:
			p.object.(GenServerBehavior).Terminate(state, reason)
			return reason

		case msg := <-p.mailBox:
			fromPid = msg.from
			message = msg.message

		case <-p.Context.Done():
			return "kill"

		case direct := <-p.direct:
			reply, err := p.object.(GenServerBehavior).HandleDirect(state, direct.message)
			if err != nil {
				direct.message = nil
				direct.err = err
				direct.reply <- direct
				continue
			}

			direct.message = reply
			direct.err = nil
			direct.reply <- direct
			continue
		}

		lib.Log("[%s]. %v got message from %#v\n", p.Node.FullName, p.self, fromPid)

		p.reductions++

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

						from := GenServerFrom{}

						from.Pid, ok = fromTuple.Element(1).(etf.Pid)
						if !ok {
							// wrong Pid value
							return
						}

						switch v := fromTuple.Element(2).(type) {
						case etf.Ref:
							from.Ref = v
						case etf.List:
							if vinc, ok := v.Element(1).(etf.List); ok {
								v = vinc
							}
							from.Ref, ok = v.Element(2).(etf.Ref)
							if !ok {
								// wrong value
								return
							}
							from.CalledByAlias = true

						default:
							// wrong tag value
							return
						}

						lockState.Lock()
						defer lockState.Unlock()

						cf := p.currentFunction
						p.currentFunction = "GenServer:HandleCall"
						code, reply := p.object.(GenServerBehavior).HandleCall(state, from, m.Element(3))
						p.currentFunction = cf
						switch code {
						case "reply":
							var fromTag etf.Term
							var to etf.Term
							if from.CalledByAlias {
								fromTag = etf.ListImproper{etf.Atom("alias"), from.Ref}
								to = from.Ref
							} else {
								fromTag = from.Ref
								to = from.Pid
							}

							if reply != nil {
								rep := etf.Tuple{fromTag, reply}
								p.Send(to, rep)
								return
							}
							rep := etf.Tuple{fromTag, etf.Atom("nil")}
							p.Send(to, rep)
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
						lockState.Lock()
						defer lockState.Unlock()

						cf := p.currentFunction
						p.currentFunction = "GenServer:HandleCast"
						code := p.object.(GenServerBehavior).HandleCast(state, m.Element(2))
						p.currentFunction = cf

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
						lockState.Lock()
						defer lockState.Unlock()

						cf := p.currentFunction
						p.currentFunction = "GenServer:HandleInfo"
						code := p.object.(GenServerBehavior).HandleInfo(state, message)
						p.currentFunction = cf
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

			case etf.Ref:
				lib.Log("got reply: %#v\n%#v", mtag, message)
				p.reply <- m

			default:
				lib.Log("mtag: %#v", mtag)
				go func() {
					lockState.Lock()
					defer lockState.Unlock()

					cf := p.currentFunction
					p.currentFunction = "GenServer:HandleInfo"
					code := p.object.(GenServerBehavior).HandleInfo(state, message)
					p.currentFunction = cf

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
				lockState.Lock()
				defer lockState.Unlock()

				cf := p.currentFunction
				p.currentFunction = "GenServer:HandleInfo"
				code := p.object.(GenServerBehavior).HandleInfo(state, message)
				p.currentFunction = cf

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
// default callbacks for GenServer interface
//
func (gs *GenServer) Init(state *GenServerState, args ...interface{}) error {
	return nil
}

func (gs *GenServer) HandleCast(state *GenServerState, message etf.Term) string {
	fmt.Printf("GenServer [%s] HandleCast: unhandled message %#v \n", state.Process.Name(), message)
	return "noreply"
}

func (gs *GenServer) HandleCall(state *GenServerState, from GenServerFrom, message etf.Term) (string, etf.Term) {
	fmt.Printf("GenServer [%s] HandleCall: unhandled message %#v from %#v \n", state.Process.Name(), message, from)
	return "reply", "ok"
}

func (gs *GenServer) HandleDirect(state *GenServerState, message interface{}) (interface{}, error) {
	return nil, ErrUnsupportedRequest
}

func (gs *GenServer) HandleInfo(state *GenServerState, message etf.Term) string {
	fmt.Printf("GenServer [%s] HandleInfo: unhandled message %#v \n", state.Process.Name(), message)
	return "noreply"
}

func (gs *GenServer) Terminate(state *GenServerState, reason string) {
	return
}
