package ergonode

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

const (
	DefaultCallTimeout = 5
)

// GenServerBehavior interface
type GenServerBehavior interface {
	// Init(...) -> state
	Init(process Process, args ...interface{}) (state interface{})
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

func (gs *GenServer) loop(p Process, object interface{}, args ...interface{}) string {
	p.state = object.(GenServerBehavior).Init(p, args...)
	p.ready <- true

	stop := make(chan string, 2)

	for {
		var message etf.Term
		var fromPid etf.Pid
		select {
		case reason := <-stop:
			object.(GenServerBehavior).Terminate(reason, p.state)
			return reason
		case msg := <-p.mailBox:
			fromPid = msg.Element(1).(etf.Pid)
			message = msg.Element(2)
		case <-p.Context.Done():
			object.(GenServerBehavior).Terminate("immediate", p.state)
			return "immediate"
		}

		lib.Log("[%#v]. Message from %#v\n", p.self, fromPid)
		switch m := message.(type) {
		case etf.Tuple:
			switch mtag := m.Element(1).(type) {
			case etf.Atom:
				switch mtag {
				case etf.Atom("$gen_call"):
					fromTuple := m.Element(2).(etf.Tuple)
					code, reply, state := object.(GenServerBehavior).HandleCall(fromTuple, m.Element(3), p.state)

					if code == "stop" {
						stop <- reply.(string)
						continue
					}
					p.state = state

					if reply != nil && code == "reply" {
						pid := fromTuple.Element(1).(etf.Pid)
						ref := fromTuple.Element(2)
						rep := etf.Term(etf.Tuple{ref, reply})
						p.Send(pid, rep)
					}

				case etf.Atom("$gen_cast"):
					code, state := object.(GenServerBehavior).HandleCast(m.Element(2), p.state)
					if code == "stop" {
						stop <- state.(string)
						continue
					}
					p.state = state

				default:
					code, state := object.(GenServerBehavior).HandleInfo(message, p.state)
					if code == "stop" {
						stop <- state.(string)
						continue
					}
					p.state = state

				}
			case etf.Ref:
				lib.Log("got reply: %#v\n%#v", mtag, message)
				p.reply <- m
			default:
				lib.Log("mtag: %#v", mtag)
				code, state := object.(GenServerBehavior).HandleInfo(message, p.state)
				if code == "stop" {
					stop <- state.(string)
					continue
				}
				p.state = state
			}
		default:
			lib.Log("m: %#v", m)
			code, state := object.(GenServerBehavior).HandleInfo(message, p.state)
			if code == "stop" {
				stop <- state.(string)
				continue
			}
			p.state = state
		}
	}
}
