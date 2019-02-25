package ergonode

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

// GenServerInt interface
type GenServerInt interface {
	// Init(...) -> state
	Init(args ...interface{}) (state interface{})
	// HandleCast -> (0, state) - noreply
	//		         (-1, state) - normal stop
	HandleCast(message *etf.Term, state interface{}) (int, interface{})
	// HandleCall -> (1, reply, state) - reply
	//				 (0, _, state) - noreply
	//		         (-1, state) - normal stop
	HandleCall(from *etf.Tuple, message *etf.Term, state interface{}) (int, *etf.Term, interface{})
	// HandleInfo -> (0, state) - noreply
	//		         (-1, state) - normal stop (-2, -3 .... custom reasons to stop)
	HandleInfo(message *etf.Term, state interface{}) (int, interface{})
	Terminate(reason int, state interface{})

	// Making outgoing request
	Call(to interface{}, message *etf.Term, options ...interface{}) (reply *etf.Term, err error)
	Cast(to interface{}, message *etf.Term) (err error)

	// Monitors
	Monitor(to etf.Pid)
	MonitorNode(to etf.Atom, flag bool)
}

// GenServer is implementation of GenServerInt interface
type GenServer struct {
	Node    *Node   // current node of process
	Self    etf.Pid // Pid of process
	state   interface{}
	lock    sync.Mutex
	chreply chan *etf.Tuple
}

// Options returns map of default process-related options
func (gs *GenServer) Options() map[string]interface{} {
	return map[string]interface{}{
		"chan-size": 100, // size of channel for regular messages
	}
}

// ProcessLoop executes during whole time of process life.
// It receives incoming messages from channels and handle it using methods of behaviour implementation
func (gs *GenServer) ProcessLoop(pcs procChannels, pd Process, args ...interface{}) {
	state := pd.(GenServerInt).Init(args...)
	gs.state = state
	pcs.init <- true
	var chstop chan int
	chstop = make(chan int)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("GenServerInt recovered: %#v", r)
		}
	}()
	for {
		var message etf.Term
		var fromPid etf.Pid
		select {
		case reason := <-chstop:
			pd.(GenServerInt).Terminate(reason, gs.state)
		case msg := <-pcs.in:
			message = msg
		case msgFrom := <-pcs.inFrom:
			message = msgFrom[1]
			fromPid = msgFrom[0].(etf.Pid)

		}
		lib.Log("[%#v]. Message from %#v\n", gs.Self, fromPid)
		switch m := message.(type) {
		case etf.Tuple:
			switch mtag := m[0].(type) {
			case etf.Atom:
				gs.lock.Lock()
				switch mtag {
				case etf.Atom("$gen_call"):

					go func() {
						fromTuple := m[1].(etf.Tuple)
						code, reply, state1 := pd.(GenServerInt).HandleCall(&fromTuple, &m[2], gs.state)

						gs.state = state1
						gs.lock.Unlock()
						if code < 0 {
							chstop <- code
							return
						}
						if reply != nil && code == 1 {
							pid := fromTuple[0].(etf.Pid)
							ref := fromTuple[1]
							rep := etf.Term(etf.Tuple{ref, *reply})
							gs.Send(pid, &rep)
						}
					}()
				case etf.Atom("$gen_cast"):
					go func() {
						code, state1 := pd.(GenServerInt).HandleCast(&m[1], gs.state)
						gs.state = state1
						gs.lock.Unlock()
						if code < 0 {
							chstop <- code
							return
						}
					}()
				default:
					go func() {
						code, state1 := pd.(GenServerInt).HandleInfo(&message, gs.state)
						gs.state = state1
						gs.lock.Unlock()
						if code < 0 {
							chstop <- code
							return
						}
					}()
				}
			case etf.Ref:
				lib.Log("got reply: %#v\n%#v", mtag, message)
				gs.chreply <- &m
			default:
				lib.Log("mtag: %#v", mtag)
				gs.lock.Lock()
				go func() {
					code, state1 := pd.(GenServerInt).HandleInfo(&message, gs.state)
					gs.state = state1
					gs.lock.Unlock()
					if code < 0 {
						chstop <- code
						return
					}
				}()
			}
		default:
			lib.Log("m: %#v", m)
			gs.lock.Lock()
			go func() {
				code, state1 := pd.(GenServerInt).HandleInfo(&message, gs.state)
				gs.state = state1
				gs.lock.Unlock()
				if code < 0 {
					chstop <- code
					return
				}
			}()
		}
	}
}

func (gs *GenServer) setNode(node *Node) {
	gs.Node = node
}

func (gs *GenServer) setPid(pid etf.Pid) {
	gs.Self = pid
}

func (gs *GenServer) Call(to interface{}, message *etf.Term, options ...interface{}) (reply *etf.Term, err error) {
	var (
		option_timeout int = 5
	)

	gs.chreply = make(chan *etf.Tuple)
	defer close(gs.chreply)

	ref := gs.Node.MakeRef()
	from := etf.Tuple{gs.Self, ref}
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_call"), from, *message})
	if err := gs.Node.Send(gs.Self, to, &msg); err != nil {
		return nil, err
	}

	switch len(options) {
	case 1:
		switch options[0].(type) {
		case int:
			if options[0].(int) > 0 {
				option_timeout = options[0].(int)
			}
		}

	}

	for {
		select {
		case m := <-gs.chreply:
			retmsg := *m
			ref1 := retmsg[0].(etf.Ref)
			val := retmsg[1].(etf.Term)

			//check by id
			if ref.Id[0] == ref1.Id[0] && ref.Id[1] == ref1.Id[1] && ref.Id[2] == ref1.Id[2] {
				reply = &val
				goto out
			}
		case <-time.After(time.Second * time.Duration(option_timeout)):
			err = errors.New("timeout")
			goto out
		}
	}
out:
	gs.chreply = nil

	return
}

func (gs *GenServer) Cast(to interface{}, message *etf.Term) error {
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), *message})
	if err := gs.Node.Send(gs.Self, to, &msg); err != nil {
		return err
	}

	return nil
}

func (gs *GenServer) Send(to etf.Pid, reply *etf.Term) {
	gs.Node.Send(nil, to, reply)
}

func (gs *GenServer) Monitor(to etf.Pid) {
	gs.Node.Monitor(gs.Self, to)
}

func (gs *GenServer) MonitorNode(to etf.Atom, flag bool) {
	gs.Node.MonitorNode(gs.Self, to, flag)
}
