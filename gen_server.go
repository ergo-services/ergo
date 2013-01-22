package node

import (
	erl "github.com/goerlang/etf/types"
	"log"
)

type GenServer interface {
	Init(args ...interface{})
	HandleCast(message *erl.Term)
	HandleCall(message *erl.Term, from *erl.Tuple) (reply *erl.Term)
	HandleInfo(message *erl.Term)
	Terminate(reason interface{})
}

type GenServerImpl struct {
	node *Node
	self erl.Pid
}

func (gs *GenServerImpl) Options() map[string]interface{} {
	return map[string]interface{}{
		"chan-size":     100,
		"ctl-chan-size": 100,
	}
}

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

func (gs *GenServerImpl) Reply(fromTuple *erl.Tuple, reply *erl.Term) {
	gs.node.Send((*fromTuple)[0].(erl.Pid), erl.Tuple{(*fromTuple)[1], *reply})
}

func (gs *GenServerImpl) setNode(node *Node) {
	gs.node = node
}

func (gs *GenServerImpl) setPid(pid erl.Pid) {
	gs.self = pid
}
