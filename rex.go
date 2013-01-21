package node

import (
	"erlang/term"
)

type rexRPC struct {
	node *Node
}

func (nk *rexRPC) Behaviour() (Behaviour, map[string]interface{}) {
	gsi := &GenServerImpl{}
	return gsi, gsi.Options()
}

func (nk *rexRPC) Init(args ...interface{}) {
	nLog("REX: Init: %#v", args)
	nk.node = args[0].(*Node)
}

func (nk *rexRPC) HandleCast(message *term.Term) {
	nLog("REX: HandleCast: %#v", *message)
}

func (nk *rexRPC) HandleCall(message *term.Term, from *term.Tuple) (reply *term.Term) {
	nLog("REX: HandleCall: %#v, From: %#v", *message, *from)
	replyTerm := term.Term(term.Atom("yes"))
	reply = &replyTerm
	return
}

func (nk *rexRPC) HandleInfo(message *term.Term) {
	nLog("REX: HandleInfo: %#v", *message)
}

func (nk *rexRPC) Terminate(reason interface{}) {
	nLog("REX: Terminate: %#v", reason.(int))
}
