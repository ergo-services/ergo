package ergonode

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

// GenServerBehavior interface
type GenServerBehavior interface {
	// Init(...) -> state
	Init(process Process, args ...interface{}) (state interface{})
	// HandleCast -> ("noreply", state) - noreply
	//		         ("stop", state) - normal stop
	HandleCast(message *etf.Term, state interface{}) (string, interface{})
	// HandleCall -> ("reply", message, state) - reply
	//				 ("noreply", _, state) - noreply
	//		         ("stop", state) - normal stop
	HandleCall(from *etf.Tuple, message *etf.Term, state interface{}) (string, *etf.Term, interface{})
	// HandleInfo -> ("noreply", state) - noreply
	//		         ("stop", state) - normal stop
	HandleInfo(message *etf.Term, state interface{}) (string, interface{})
	Terminate(reason string, state interface{})
}

// GenServer is implementation of ProcessBehavior interface for GenServer objects
type GenServer struct {
	Process Process
}

func (gs *GenServer) loop(p Process, object interface{}, args ...interface{}) {
	state := object.(GenServerBehavior).Init(p, args...)
	p.ready <- true
	var stop chan string
	stop = make(chan string)
	for {
		var message etf.Term
		var fromPid etf.Pid
		select {
		case reason := <-stop:
			object.(GenServerBehavior).Terminate(reason, state)
			return
		case messageLocal := <-p.local:
			message = messageLocal
		case messageRemote := <-p.remote:
			message = messageRemote[1]
			fromPid = messageRemote[0].(etf.Pid)
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
					code, reply, state1 := object.(GenServerBehavior).HandleCall(&fromTuple, &m[2], p.state)

					p.state = state1
					if code == "stop" {
						stop <- code
						continue
					}

					if reply != nil && code == "reply" {
						// pid := fromTuple[0].(etf.Pid)
						// ref := fromTuple[1]
						// rep := etf.Term(etf.Tuple{ref, *reply})
						// gs.Send(pid, &rep)
					}

				case etf.Atom("$gen_cast"):
					code, state1 := object.(GenServerBehavior).HandleCast(&m[1], p.state)
					p.state = state1
					if code == "stop" {
						stop <- code
						continue
					}
				default:
					code, state1 := object.(GenServerBehavior).HandleInfo(&message, p.state)
					p.state = state1
					if code == "stop" {
						stop <- code
						return
					}
				}
			case etf.Ref:
				lib.Log("got reply: %#v\n%#v", mtag, message)
				// gs.chreply <- m
			default:
				lib.Log("mtag: %#v", mtag)
				go func() {
					code, state1 := object.(GenServerBehavior).HandleInfo(&message, p.state)
					p.state = state1
					if code == "stop" {
						stop <- code
						return
					}
				}()
			}
		default:
			lib.Log("m: %#v", m)
			go func() {
				code, state1 := object.(GenServerBehavior).HandleInfo(&message, p.state)
				p.state = state1
				if code == "stop" {
					stop <- code
					return
				}
			}()
		}
	}
}

func (gs *GenServer) Call(to interface{}, message *etf.Term, options ...interface{}) (reply *etf.Term, err error) {
	var (
	// option_timeout int = 5
	)

	// 	gs.chreply = make(chan etf.Tuple)
	// 	defer close(gs.chreply)

	// 	ref := gs.Process.Node.MakeRef()
	// 	from := etf.Tuple{gs.Process.self, ref}
	// 	msg := etf.Term(etf.Tuple{etf.Atom("$gen_call"), from, *message})
	// 	if err := gs.Process.Node.Send(gs.Process.self, to, &msg); err != nil {
	// 		return nil, err
	// 	}

	// 	switch len(options) {
	// 	case 1:
	// 		switch options[0].(type) {
	// 		case int:
	// 			if options[0].(int) > 0 {
	// 				option_timeout = options[0].(int)
	// 			}
	// 		}

	// 	}

	// 	for {
	// 		select {
	// 		case m := <-gs.chreply:
	// 			ref1 := m[0].(etf.Ref)
	// 			val := m[1].(etf.Term)

	// 			//check by id
	// 			if ref.Id[0] == ref1.Id[0] && ref.Id[1] == ref1.Id[1] && ref.Id[2] == ref1.Id[2] {
	// 				reply = &val
	// 				goto out
	// 			}
	// 		case <-time.After(time.Second * time.Duration(option_timeout)):
	// 			err = errors.New("timeout")
	// 			goto out
	// 		}
	// 	}
	// out:
	// 	gs.chreply = nil

	return
}

func (gs *GenServer) Cast(to interface{}, message *etf.Term) error {
	// msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), *message})
	// if err := gs.Process.Node.Send(gs.Process.self, to, &msg); err != nil {
	// 	return err
	// }

	return nil
}

func (gs *GenServer) Send(to etf.Pid, reply *etf.Term) {
	// gs.Process.Node.Send(nil, to, reply)
}

// func (gs *GenServer) Monitor(to etf.Pid) {
// 	gs.Node.Monitor(gs.self, to)
// }

// func (gs *GenServer) MonitorNode(to etf.Atom, flag bool) {
// 	gs.Node.MonitorNode(gs.self, to, flag)
// }
