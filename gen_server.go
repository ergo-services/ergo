package ergonode

import (
	"github.com/halturin/ergonode/etf"
	"log"
	"sync"
	"time"
)

// GenServer interface
type GenServer interface {
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
}

// GenServerImpl is implementation of GenServer interface
type GenServerImpl struct {
	Node  *Node   // current node of process
	Self  etf.Pid // Pid of process
	state interface{}
	lock  sync.Mutex
}

// Options returns map of default process-related options
func (gs *GenServerImpl) Options() map[string]interface{} {
	return map[string]interface{}{
		"chan-size": 100, // size of channel for regular messages
	}
}

// ProcessLoop executes during whole time of process life.
// It receives incoming messages from channels and handle it using methods of behaviour implementation
func (gs *GenServerImpl) ProcessLoop(pcs procChannels, pd Process, args ...interface{}) {
	pd.(GenServer).Init(args...)
	pcs.init <- true
	var chstop chan int
	chstop = make(chan int)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("GenServer recovered: %#v", r)
		}
	}()
	for {
		var message etf.Term
		var fromPid etf.Pid

		select {
		case reason := <-chstop:
			pd.(GenServer).Terminate(reason, gs.state)
		case msg := <-pcs.in:
			message = msg
		case msgFrom := <-pcs.inFrom:
			message = msgFrom[1]
			fromPid = msgFrom[0].(etf.Pid)

		}

		nLog("Message from %#v", fromPid)
		switch m := message.(type) {
		case etf.Tuple:
			switch mtag := m[0].(type) {
			case etf.Atom:
				gs.lock.Lock()
				switch mtag {
				case etf.Atom("$gen_call"):

					go func() {
						fromTuple := m[1].(etf.Tuple)
						code, reply, state1 := pd.(GenServer).HandleCall(&fromTuple, &m[2], gs.state)

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
						code, state1 := pd.(GenServer).HandleCast(&m[1], gs.state)
						gs.state = state1
						gs.lock.Unlock()
						if code < 0 {
							chstop <- code
							return
						}
					}()
				default:
					go func() {
						code, state1 := pd.(GenServer).HandleInfo(&message, gs.state)
						gs.state = state1
						gs.lock.Unlock()
						if code < 0 {
							chstop <- code
							return
						}
					}()
				}
			case etf.Ref:
				nLog("got reply: %#v\n%#v", mtag, message)
			default:
				nLog("mtag: %#v", mtag)
				gs.lock.Lock()
				go func() {
					code, state1 := pd.(GenServer).HandleInfo(&message, gs.state)
					gs.state = state1
					gs.lock.Unlock()
					if code < 0 {
						chstop <- code
						return
					}
				}()
			}
		default:
			nLog("m: %#v", m)
			gs.lock.Lock()
			go func() {
				code, state1 := pd.(GenServer).HandleInfo(&message, gs.state)
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

func (gs *GenServerImpl) setNode(node *Node) {
	gs.Node = node
}

func (gs *GenServerImpl) setPid(pid etf.Pid) {
	gs.Self = pid
}

func (gs *GenServerImpl) Call(to interface{}, message *etf.Term) (reply *etf.Term) {

	from := etf.Tuple{gs.Self, gs.MakeRef()}
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_call"), from, *message})
	if err := gs.Node.Send(gs.Self, to, &msg); err != nil {
		panic(err.Error())
	}

	// FIXME. just stub

	replyTerm := etf.Term(etf.Atom("ok"))
	reply = &replyTerm

	return
}

func (gs *GenServerImpl) Cast(to interface{}, message *etf.Term) error {
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), *message})
	if err := gs.Node.Send(gs.Self, to, &msg); err != nil {
		panic(err.Error())
	}

	return nil
}

func (gs *GenServerImpl) Send(to etf.Pid, reply *etf.Term) {
	gs.Node.Send(nil, to, reply)
}

func (gs *GenServerImpl) MakeRef() (ref etf.Ref) {
	ref.Node = etf.Atom(gs.Node.FullName)
	ref.Creation = 1

	nt := time.Now().UnixNano()
	id1 := uint32(uint64(nt) & ((2 << 17) - 1))
	id2 := uint32(uint64(nt) >> 46)
	ref.Id = []uint32{id1, id2, 0}

	return
}
