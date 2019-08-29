package ergonode

import (
	"time"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type ApplicationRestart = string

const (
	// Restart types:

	// ApplicationRestartPermanent child process is always restarted
	ApplicationRestartPermanent = "permanent"

	// ApplicationRestartTemporary child process is never restarted
	// (not even when the supervisor restart strategy is rest_for_one
	// or one_for_all and a sibling death causes the temporary process
	// to be terminated)
	ApplicationRestartTemporary = "temporary"

	// ApplicationRestartTransient child process is restarted only if
	// it terminates abnormally, that is, with an exit reason other
	// than normal, shutdown, or {shutdown,Term}.
	ApplicationRestartTransient = "transient"
)

// SupervisorBehavior interface
type ApplicationBehavior interface {
	Init(process *Process, args ...interface{}) ApplicationSpec
}

type ApplicationSpec struct {
	Name         string
	Description  string
	ID           string
	Version      string
	MaxTime      time.Duration
	Applications []Application
	Environment  map[string]interface{}
	// Depends		[]
	children []ApplicationChildSpec
	strategy ApplicationRestart
}

type ApplicationChildSpec struct {
	child ProcessBehaviour
}

// Supervisor is implementation of ProcessBehavior interface
type Application struct {
	process Process
}

func (sv *Application) loop(p *Process, object interface{}, args ...interface{}) {
	spec := object.(ApplicationBehavior).Init(p, args...)
	lib.Log("Application spec %#v\n", spec)
	p.ready <- true
	// var stop chan string
	// stop = make(chan string)
	for {
		// var message etf.Term
		var fromPid etf.Pid
		select {
		// case reason := <-stop:
		// 	object.(SupervisorBehavior).Terminate(reason, state)
		// 	return
		// case messageLocal := <-p.local:
		// 	message = messageLocal
		// case messageRemote := <-p.remote:
		// 	message = messageRemote[1]
		// 	fromPid = messageRemote[0].(etf.Pid)
		case <-p.context.Done():
			// object.(GenServerBehavior).Terminate("immediate", p.state)
			return
		}

		lib.Log("[%#v]. Message from %#v\n", p.self, fromPid)
		// switch m := message.(type) {
		// case etf.Tuple:
		// 	switch mtag := m[0].(type) {
		// 	case etf.Atom:
		// 		switch mtag {
		// 		case etf.Atom("$gen_call"):
		// 			fromTuple := m[1].(etf.Tuple)
		// 			code, reply, state1 := object.(GenServerBehavior).HandleCall(&fromTuple, &m[2], p.state)

		// 			p.state = state1
		// 			if code == "stop" {
		// 				stop <- code
		// 				continue
		// 			}

		// 			if reply != nil && code == "reply" {
		// 				// pid := fromTuple[0].(etf.Pid)
		// 				// ref := fromTuple[1]
		// 				// rep := etf.Term(etf.Tuple{ref, *reply})
		// 				// gs.Send(pid, &rep)
		// 			}

		// 		case etf.Atom("$gen_cast"):
		// 			code, state1 := object.(GenServerBehavior).HandleCast(&m[1], p.state)
		// 			p.state = state1
		// 			if code == "stop" {
		// 				stop <- code
		// 				continue
		// 			}
		// 		default:
		// 			code, state1 := object.(GenServerBehavior).HandleInfo(&message, p.state)
		// 			p.state = state1
		// 			if code == "stop" {
		// 				stop <- code
		// 				return
		// 			}
		// 		}
		// 	case etf.Ref:
		// 		lib.Log("got reply: %#v\n%#v", mtag, message)
		// 		// gs.chreply <- m
		// 	default:
		// 		lib.Log("mtag: %#v", mtag)
		// 		go func() {
		// 			code, state1 := object.(GenServerBehavior).HandleInfo(&message, p.state)
		// 			p.state = state1
		// 			if code == "stop" {
		// 				stop <- code
		// 				return
		// 			}
		// 		}()
		// 	}
		// default:
		// 	lib.Log("m: %#v", m)
		// 	go func() {
		// 		code, state1 := object.(GenServerBehavior).HandleInfo(&message, p.state)
		// 		p.state = state1
		// 		if code == "stop" {
		// 			stop <- code
		// 			return
		// 		}
		// 	}()
		// }
	}
}
