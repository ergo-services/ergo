package ergonode

import (
	"sync"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

const (
	DefaultCallTimeout = 5
)

// GenServerBehavior interface
type GenServerBehavior interface {
	// Init(...) -> state
	Init(process *Process, args ...interface{}) (state interface{})
	// HandleCast -> ("noreply", state) - noreply
	//		         ("stop", reason) - stop with reason
	HandleCast(message etf.Term, state interface{}) (string, interface{})
	// HandleCall -> ("reply", message, state) - reply
	//				 ("noreply", _, state) - noreply
	//		         ("stop", reason, _) - normal stop
	HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{})
	// HandleInfo -> ("noreply", state) - noreply
	//		         ("stop", reason) - normal stop
	HandleInfo(message etf.Term, state interface{}) (string, interface{})
	Terminate(reason string, state interface{})
}

// GenServer is implementation of ProcessBehavior interface for GenServer objects
type GenServer struct{}

func (gs *GenServer) loop(p *Process, object interface{}, args ...interface{}) string {
	p.state = object.(GenServerBehavior).Init(p, args...)
	p.ready <- true

	stop := make(chan string, 2)

	p.currentFunction = "GenServer:loop"

	for {
		var message etf.Term
		var fromPid etf.Pid
		var lockState = &sync.Mutex{}

		select {
		case ex := <-p.gracefulExit:
			if p.trapExit {
				message = etf.Tuple{
					etf.Atom("EXIT"),
					ex.from,
					etf.Atom(ex.reason),
				}
			} else {
				object.(GenServerBehavior).Terminate(ex.reason, p.state)
				return ex.reason
			}
		case reason := <-stop:
			object.(GenServerBehavior).Terminate(reason, p.state)
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

						cf := p.currentFunction
						p.currentFunction = "GenServer:HandleCall"
						code, reply, state := object.(GenServerBehavior).HandleCall(fromTuple, m.Element(3), p.state)
						p.currentFunction = cf

						if code == "stop" {
							stop <- reply.(string)
							// do not unlock, coz we have to keep this state unchanged for Terminate handler
							return
						}

						p.state = state
						lockState.Unlock()

						if reply != nil && code == "reply" {
							pid := fromTuple.Element(1).(etf.Pid)
							ref := fromTuple.Element(2)
							rep := etf.Term(etf.Tuple{ref, reply})
							p.Send(pid, rep)
						}
					}()

				case etf.Atom("$gen_cast"):
					go func() {
						lockState.Lock()

						cf := p.currentFunction
						p.currentFunction = "GenServer:HandleCast"
						code, state := object.(GenServerBehavior).HandleCast(m.Element(2), p.state)
						p.currentFunction = cf

						if code == "stop" {
							stop <- state.(string)
							return
						}
						p.state = state
						lockState.Unlock()
					}()

				case etf.Atom("EXIT"):
					// handling linked process termination
					// {'EXIT', <pid>, reason}
					ex := gracefulExitRequest{
						from:   m.Element(2).(etf.Pid),
						reason: m.Element(3).(string),
					}
					go func() { p.gracefulExit <- ex }()

				default:
					go func() {
						lockState.Lock()

						cf := p.currentFunction
						p.currentFunction = "GenServer:HandleInfo"
						code, state := object.(GenServerBehavior).HandleInfo(message, p.state)
						p.currentFunction = cf

						if code == "stop" {
							stop <- state.(string)
							return
						}
						p.state = state
						lockState.Unlock()
					}()

				}

			case etf.Ref:
				lib.Log("got reply: %#v\n%#v", mtag, message)
				p.reply <- m

			default:
				lib.Log("mtag: %#v", mtag)
				go func() {
					lockState.Lock()

					cf := p.currentFunction
					p.currentFunction = "GenServer:HandleInfo"
					code, state := object.(GenServerBehavior).HandleInfo(message, p.state)
					p.currentFunction = cf

					if code == "stop" {
						stop <- state.(string)
					}
					p.state = state
					lockState.Unlock()
				}()
			}

		default:
			lib.Log("m: %#v", m)
			go func() {
				lockState.Lock()

				cf := p.currentFunction
				p.currentFunction = "GenServer:HandleInfo"
				code, state := object.(GenServerBehavior).HandleInfo(message, p.state)
				p.currentFunction = cf

				if code == "stop" {
					stop <- state.(string)
					return
				}
				p.state = state
				lockState.Unlock()
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
