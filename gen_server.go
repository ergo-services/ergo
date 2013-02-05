package node

import (
	"github.com/goerlang/etf"
	"log"
)

// GenServer interface
type GenServer interface {
	Init(args ...interface{})
	HandleCast(message *etf.Term)
	HandleCall(message *etf.Term, from *etf.Tuple) (reply *etf.Term)
	HandleInfo(message *etf.Term)
	Terminate(reason interface{})
}

// GenServerImpl is implementation of GenServer interface
type GenServerImpl struct {
	Node *Node   // current node of process
	Self etf.Pid // Pid of process
}

// Options returns map of default process-related options
func (gs *GenServerImpl) Options() map[string]interface{} {
	return map[string]interface{}{
		"chan-size":     100, // size of channel for regular messages
		"ctl-chan-size": 100, // size of channel for control messages
	}
}

// ProcessLoop executes during whole time of process life.
// It receives incoming messages from channels and handle it using methods of behaviour implementation
func (gs *GenServerImpl) ProcessLoop(pcs procChannels, pd Process, args ...interface{}) {
	pd.(GenServer).Init(args...)
	//pcs.ctl <- etf.Tuple{etf.Atom("$go_ctl"), etf.Tuple{etf.Atom("control-message"), etf.Atom("example")}}
	defer func() {
		if r := recover(); r != nil {
			// TODO: send message to parent process
			log.Printf("GenServer recovered: %#v", r)
		}
	}()
	for {
		var message etf.Term
		var fromPid etf.Pid
		select {
		case msg := <-pcs.in:
			message = msg
		case msgFrom := <-pcs.inFrom:
			message = msgFrom[1]
			fromPid = msgFrom[0].(etf.Pid)
		case ctlMsg := <-pcs.ctl:
			switch m := ctlMsg.(type) {
			case etf.Tuple:
				switch mtag := m[0].(type) {
				case etf.Atom:
					switch mtag {
					case etf.Atom("$go_ctl"):
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
		case etf.Tuple:
			switch mtag := m[0].(type) {
			case etf.Atom:
				switch mtag {
				case etf.Atom("$go_ctl"):
					nLog("Control message: %#v", message)
				case etf.Atom("$gen_call"):
					fromTuple := m[1].(etf.Tuple)
					reply := pd.(GenServer).HandleCall(&m[2], &fromTuple)
					if reply != nil {
						gs.Reply(&fromTuple, reply)
					}
				case etf.Atom("$gen_cast"):
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
func (gs *GenServerImpl) Reply(fromTuple *etf.Tuple, reply *etf.Term) {
	gs.Node.Send((*fromTuple)[0].(etf.Pid), etf.Tuple{(*fromTuple)[1], *reply})
}

func (gs *GenServerImpl) setNode(node *Node) {
	gs.Node = node
}

func (gs *GenServerImpl) setPid(pid etf.Pid) {
	gs.Self = pid
}
