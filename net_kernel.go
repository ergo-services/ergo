package node

import (
	"erlang/term"
)

type netKernel struct {
	node *Node
}

func (nk *netKernel) Behaviour() (Behaviour, map[string]interface{}) {
	gsi := &GenServerImpl{}
	return gsi, gsi.Options()
}

func (nk *netKernel) Init(args ...interface{}) {
	nLog("NET_KERNEL: Init: %#v", args)
	nk.node = args[0].(*Node)
}

func (nk *netKernel) HandleCast(message *term.Term) {
	nLog("NET_KERNEL: HandleCast: %#v", *message)
}

func (nk *netKernel) HandleCall(message *term.Term, from *term.Tuple) (reply *term.Term) {
	nLog("NET_KERNEL: HandleCall: %#v, From: %#v", *message, *from)
	replyTerm := term.Term(term.Atom("yes"))
	reply = &replyTerm
	return
}

func (nk *netKernel) HandleInfo(message *term.Term) {
	nLog("NET_KERNEL: HandleInfo: %#v", *message)
}

func (nk *netKernel) Terminate(reason interface{}) {
	nLog("NET_KERNEL: Terminate: %#v", reason.(int))
}
