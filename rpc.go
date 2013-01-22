package node

import (
	erl "github.com/goerlang/etf/types"
)

type rpcRex struct {
	GenServerImpl
}

func (rpcs *rpcRex) Behaviour() (Behaviour, map[string]interface{}) {
	return rpcs, rpcs.Options()
}

func (rpcs *rpcRex) Init(args ...interface{}) {
	nLog("REX: Init: %#v", args)
	rpcs.Node.Register(erl.Atom("rex"), rpcs.Self)
}

func (rpcs *rpcRex) HandleCast(message *erl.Term) {
	nLog("REX: HandleCast: %#v", *message)
}

func (rpcs *rpcRex) HandleCall(message *erl.Term, from *erl.Tuple) (reply *erl.Term) {
	nLog("REX: HandleCall: %#v, From: %#v", *message, *from)
	switch req := (*message).(type) {
	case erl.Tuple:
		if len(req) > 0 {
			switch act := req[0].(type) {
			case erl.Atom:
				if string(act) == "call" {
					nLog("RPC CALL: Module: %#v, Function: %#v, Args: %#v, GroupLeader: %#v", req[1], req[2], req[3], req[4])
					replyTerm := erl.Term(erl.Tuple{req[1], req[2]})
					reply = &replyTerm
				}
			}
		}
	}
	if reply == nil {
		replyTerm := erl.Term(erl.Tuple{erl.Atom("badrpc"), erl.Atom("unknown")})
		reply = &replyTerm
	}
	return
}

func (rpcs *rpcRex) HandleInfo(message *erl.Term) {
	nLog("REX: HandleInfo: %#v", *message)
}

func (rpcs *rpcRex) Terminate(reason interface{}) {
	nLog("REX: Terminate: %#v", reason.(int))
}
