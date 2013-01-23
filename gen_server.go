package node

import (
	erl "github.com/goerlang/etf/types"
	"log"
)

// GenServer interface
type GenServer interface {
	Init(args ...interface{})
	HandleCast(message *erl.Term)
	HandleCall(message *erl.Term, from *erl.Tuple) (reply *erl.Term)
	HandleInfo(message *erl.Term)
	Terminate(reason interface{})
}

// GenServerImpl is implementation of GenServer interface
type GenServerImpl struct {
	Node *Node					// current node of process
	Self erl.Pid				// Pid of process
}

// Options returns map of default process-related options
func (gs *GenServerImpl) Options() map[string]interface{} {
	return map[string]interface{}{
		"chan-size":     100,	// size of channel for regular messages
		"ctl-chan-size": 100,	// size of channel for control messages
	}
}

// ProcessLoop executes during whole time of process life.
// It receives incoming messages from channels and handle it using methods of behaviour implementation
func (gs *GenServerImpl) ProcessLoop(node *Node, pid erl.Pid, pcs procChannels, pd Process, args ...interface{}) {
	gs.setNode(node)
	gs.setPid(pid)
	pd.(GenServer).Init(args...)
	//pcs.ctl <- erl.Tuple{erl.Atom("$go_ctl"), erl.Tuple{erl.Atom("control-message"), erl.Atom("example")}}
	defer func() {
		if r := recover(); r != nil {
			// TODO: send message to parent process
			log.Printf("GenServer recovered: %#v", r)
		}
	}()
	for {
		var message erl.Term
		var fromPid erl.Pid
		select {
		case msg := <-pcs.in:
			message = msg
		case msgFrom := <-pcs.inFrom:
			message = msgFrom[1]
			fromPid = msgFrom[0].(erl.Pid)
		case ctlMsg := <-pcs.ctl:
			switch m := ctlMsg.(type) {
			case erl.Tuple:
				switch mtag := m[0].(type) {
				case erl.Atom:
					switch mtag {
					case erl.Atom("$go_ctl"):
						nLog("Control message: %#v", m)
					default:
						nLog("Unknown message: %#v", m)
					}
				default:
					nLog("Unknown message: %#v", m)
				}
			default:
				nLog("Unknown message: %#v", m)
			}
			continue
		}
		nLog("Message from %#v", fromPid)
		switch m := message.(type) {
		case erl.Tuple:
			switch mtag := m[0].(type) {
			case erl.Atom:
				switch mtag {
				case erl.Atom("$go_ctl"):
					nLog("Control message: %#v", message)
				case erl.Atom("$gen_call"):
					fromTuple := m[1].(erl.Tuple)
					reply := pd.(GenServer).HandleCall(&m[2], &fromTuple)
					if reply != nil {
						gs.Reply(&fromTuple, reply)
					}
				case erl.Atom("$gen_cast"):
					pd.(GenServer).HandleCast(&m[1])
				default:
					pd.(GenServer).HandleInfo(&message)
				}
			default:
				nLog("mtag: %#v", mtag)
				pd.(GenServer).HandleInfo(&message)
			}
		default:
			nLog("m: %#v", m)
			pd.(GenServer).HandleInfo(&message)
		}
	}
}

// Reply sends delayed reply at incoming `gen_server:call/2`
func (gs *GenServerImpl) Reply(fromTuple *erl.Tuple, reply *erl.Term) {
	gs.Node.Send((*fromTuple)[0].(erl.Pid), erl.Tuple{(*fromTuple)[1], *reply})
}

func (gs *GenServerImpl) setNode(node *Node) {
	gs.Node = node
}

func (gs *GenServerImpl) setPid(pid erl.Pid) {
	gs.Self = pid
}
