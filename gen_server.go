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
	// Init(...) -> state
	Init(process *Process, args ...interface{}) (interface{}, error)

	// HandleCast -> "noreply" - noreply
	//				"stop" - stop with reason 'normal'
	//		         "reason" - stop with given reason
	HandleCast(message etf.Term, state GenServerState) string

	// HandleCall -> ("reply", message) - reply
	//				 ("noreply", _) - noreply
	//		         ("stop", _) - stop with reason 'normal'
	//				 ("reason", _) - stop with given reason
	HandleCall(from etf.Tuple, message etf.Term, state GenServerState) (string, etf.Term)

	// HandleInfo -> "noreply" - noreply
	//				"stop" - stop with reason 'normal'
	//		         "reason" - stop with given reason
	HandleInfo(message etf.Term, state GenServerState) string

	Terminate(reason string, state GenServerState)
}

// GenServer is implementation of ProcessBehavior interface for GenServer objects
type GenServer struct{}

// GenServerState state of the GenServer process. State keeps the value returned from the Init callback.
type GenServerState struct {
	Process *Process
	State   interface{}
}

func (gs *GenServer) Loop(p *Process, args ...interface{}) string {
	lockState := &sync.Mutex{}

	gsState, err := p.object.(GenServerBehavior).Init(p, args...)
	if err != nil {
		return err.Error()
	}
	state := GenServerState{
		Process: p,
		State:   gsState,
	}
	p.ready <- nil

	stop := make(chan string, 2)

	p.currentFunction = "GenServer:loop"

	for {
		var message etf.Term
		var fromPid etf.Pid

		select {
		case ex := <-p.gracefulExit:
			p.object.(GenServerBehavior).Terminate(ex.reason, state)
			return ex.reason

		case reason := <-stop:
			p.object.(GenServerBehavior).Terminate(reason, state)
			return reason

		case msg := <-p.mailBox:
			fromPid = msg.Element(1).(etf.Pid)
			message = msg.Element(2)

		case <-p.Context.Done():
			return "kill"

		case direct := <-p.direct:
			gs.handleDirect(direct)
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
						fromTuple := m.Element(2).(etf.Tuple)
						lockState.Lock()
						defer lockState.Unlock()

						cf := p.currentFunction
						p.currentFunction = "GenServer:HandleCall"
						code, reply := p.object.(GenServerBehavior).HandleCall(fromTuple, m.Element(3), state)
						p.currentFunction = cf
						switch code {
						case "reply":
							pid := fromTuple.Element(1).(etf.Pid)
							ref := fromTuple.Element(2)
							if reply != nil {
								rep := etf.Term(etf.Tuple{ref, reply})
								p.Send(pid, rep)
								return
							}
							rep := etf.Term(etf.Tuple{ref, etf.Atom("nil")})
							p.Send(pid, rep)
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
						code := p.object.(GenServerBehavior).HandleCast(m.Element(2), state)
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
						code := p.object.(GenServerBehavior).HandleInfo(message, state)
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
					code := p.object.(GenServerBehavior).HandleInfo(message, state)
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
				code := p.object.(GenServerBehavior).HandleInfo(message, state)
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

func (gs *GenServer) handleDirect(m directMessage) {

	if m.reply != nil {
		m.err = ErrUnsupportedRequest
		m.reply <- m
	}
}

//
// default callbacks for GenServer interface
//
func (gs *GenServer) Init(process *Process, args ...interface{}) (interface{}, error) {
	return nil, nil
}

func (gs *GenServer) HandleCast(message etf.Term, state GenServerState) string {
	fmt.Printf("GenServer [%s] HandleCast: unhandled message %#v \n", state.Process.Name(), message)
	return "noreply"
}

func (gs *GenServer) HandleCall(from etf.Tuple, message etf.Term, state GenServerState) (string, etf.Term) {
	fmt.Printf("GenServer [%s] HandleCall: unhandled message %#v from %#v \n", state.Process.Name(), message, from)
	return "reply", "ok"
}

func (gs *GenServer) HandleInfo(message etf.Term, state GenServerState) string {
	fmt.Printf("GenServer [%s] HandleInfo: unhandled message %#v \n", state.Process.Name(), message)
	return "noreply"
}

func (gs *GenServer) Terminate(reason string, state GenServerState) {
	return
}
