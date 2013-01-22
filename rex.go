package node

import (
	erl "github.com/goerlang/etf/types"
)

type rexRPC struct {
	gsi *GenServerImpl
}

func (nk *rexRPC) Behaviour() (Behaviour, map[string]interface{}) {
	nk.gsi = &GenServerImpl{}
	return nk.gsi, nk.gsi.Options()
}

func (nk *rexRPC) Init(args ...interface{}) {
	nLog("REX: Init: %#v", args)
}

func (nk *rexRPC) HandleCast(message *erl.Term) {
	nLog("REX: HandleCast: %#v", *message)
}

func (nk *rexRPC) HandleCall(message *erl.Term, from *erl.Tuple) (reply *erl.Term) {
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

func (nk *rexRPC) HandleInfo(message *erl.Term) {
	nLog("REX: HandleInfo: %#v", *message)
}

func (nk *rexRPC) Terminate(reason interface{}) {
	nLog("REX: Terminate: %#v", reason.(int))
}
