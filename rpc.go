package node

import (
	"github.com/goerlang/etf"
)

type rpcRex struct {
	GenServerImpl
}

func (rpcs *rpcRex) Init(args ...interface{}) {
	nLog("REX: Init: %#v", args)
	rpcs.Node.Register(etf.Atom("rex"), rpcs.Self)
}

func (rpcs *rpcRex) HandleCast(message *etf.Term) {
	nLog("REX: HandleCast: %#v", *message)
}

func (rpcs *rpcRex) HandleCall(message *etf.Term, from *etf.Tuple) (reply *etf.Term) {
	nLog("REX: HandleCall: %#v, From: %#v", *message, *from)
	switch req := (*message).(type) {
	case etf.Tuple:
		if len(req) > 0 {
			switch act := req[0].(type) {
			case etf.Atom:
				if string(act) == "call" {
					nLog("RPC CALL: Module: %#v, Function: %#v, Args: %#v, GroupLeader: %#v", req[1], req[2], req[3], req[4])
					replyTerm := etf.Term(etf.Tuple{req[1], req[2]})
					reply = &replyTerm
				}
			}
		}
	}
	if reply == nil {
		replyTerm := etf.Term(etf.Tuple{etf.Atom("badrpc"), etf.Atom("unknown")})
		reply = &replyTerm
	}
	return
}

func (rpcs *rpcRex) HandleInfo(message *etf.Term) {
	nLog("REX: HandleInfo: %#v", *message)
}

func (rpcs *rpcRex) Terminate(reason interface{}) {
	nLog("REX: Terminate: %#v", reason.(int))
}
