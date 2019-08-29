package ergonode

import (
	"errors"
	"time"

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
type GenServer struct {
	Process Process
	reply   chan etf.Tuple
}

func (gs *GenServer) loop(p Process, object interface{}, args ...interface{}) {
	state := object.(GenServerBehavior).Init(p, args...)
	p.ready <- true

	gs.reply = make(chan etf.Tuple)
	stop := make(chan string)

	for {
		var message etf.Term
		var fromPid etf.Pid
		select {
		case reason := <-stop:
			object.(GenServerBehavior).Terminate(reason, state)
			return
		case msg := <-p.mailBox:
			fromPid = msg[0].(etf.Pid)
			message = msg[1]
		case <-p.context.Done():
			object.(GenServerBehavior).Terminate("immediate", p.state)
			return
		}

		lib.Log("[%#v]. Message from %#v\n", p.self, fromPid)
		switch m := message.(type) {
		case etf.Tuple:
			switch mtag := m[0].(type) {
			case etf.Atom:
				switch mtag {
				case etf.Atom("$gen_call"):
					fromTuple := m[1].(etf.Tuple)
					code, reply, result := object.(GenServerBehavior).HandleCall(fromTuple, &m[2], p.state)

					p.state = result
					if code == "stop" {
						stop <- result.(string)
					}

					if reply != nil && code == "reply" {
						// pid := fromTuple[0].(etf.Pid)
						// ref := fromTuple[1]
						// rep := etf.Term(etf.Tuple{ref, *reply})
						// gs.Send(pid, &rep)
					}

				case etf.Atom("$gen_cast"):
					code, result := object.(GenServerBehavior).HandleCast(m[1], p.state)
					p.state = result
					if code == "stop" {
						stop <- result.(string)
					}
				default:
					code, result := object.(GenServerBehavior).HandleInfo(message, p.state)
					p.state = result
					if code == "stop" {
						stop <- result.(string)
					}
				}
			case etf.Ref:
				lib.Log("got reply: %#v\n%#v", mtag, message)
				gs.reply <- m
			default:
				lib.Log("mtag: %#v", mtag)
				code, result := object.(GenServerBehavior).HandleInfo(message, p.state)
				p.state = result
				if code == "stop" {
					stop <- result.(string)
				}
			}
		default:
			lib.Log("m: %#v", m)
			code, result := object.(GenServerBehavior).HandleInfo(message, p.state)
			p.state = result
			if code == "stop" {
				stop <- result.(string)
			}
		}
	}
}
func (gs *GenServer) Call(to interface{}, message etf.Term) (etf.Term, error) {
	return gs.CallWithTimeout(to, message, DefaultCallTimeout)
}

func (gs *GenServer) CallWithTimeout(to interface{}, message etf.Term, timeout int) (etf.Term, error) {
	ref := gs.Process.Node.MakeRef()
	from := etf.Tuple{gs.Process.self, ref}
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_call"), from, message})
	gs.Process.Send(to, msg)
	for {
		select {
		case m := <-gs.reply:
			ref1 := m[0].(etf.Ref)
			val := m[1].(etf.Term)
			// check message Ref
			if len(ref.Id) == 3 && ref.Id[0] == ref1.Id[0] && ref.Id[1] == ref1.Id[1] && ref.Id[2] == ref1.Id[2] {
				return val, nil
			}
			// ignore this message. waiting for the next one
		case <-time.After(time.Second * time.Duration(timeout)):
			return nil, errors.New("timeout")
		case <-gs.Process.context.Done():
			return nil, errors.New("stopped")
		}
	}
}

func (gs *GenServer) Cast(to interface{}, message etf.Term) {
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), message})
	gs.Process.Send(to, msg)
}

func (gs *GenServer) Send(to etf.Pid, reply etf.Term) {
	gs.Process.Send(to, reply)
}
