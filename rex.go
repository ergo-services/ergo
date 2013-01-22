package node

import (
	"erlang/term"
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

func (nk *rexRPC) HandleCast(message *term.Term) {
	nLog("REX: HandleCast: %#v", *message)
}

func (nk *rexRPC) HandleCall(message *term.Term, from *term.Tuple) (reply *term.Term) {
	nLog("REX: HandleCall: %#v, From: %#v", *message, *from)
	switch req := (*message).(type) {
	case term.Tuple:
		if len(req) > 0 {
			switch act := req[0].(type) {
			case term.Atom:
				if string(act) == "call" {
					nLog("RPC CALL: Module: %#v, Function: %#v, Args: %#v, GroupLeader: %#v", req[1], req[2], req[3], req[4])
					replyTerm := term.Term(term.Tuple{req[1], req[2]})
					reply = &replyTerm
				}
			}
		}
	}
	if reply == nil {
		replyTerm := term.Term(term.Tuple{term.Atom("badrpc"), term.Atom("unknown")})
		reply = &replyTerm
	}
	return
}

func (nk *rexRPC) HandleInfo(message *term.Term) {
	nLog("REX: HandleInfo: %#v", *message)
}

func (nk *rexRPC) Terminate(reason interface{}) {
	nLog("REX: Terminate: %#v", reason.(int))
}
