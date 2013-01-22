package node

import (
	erl "github.com/goerlang/etf/types"
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

func (nk *netKernel) HandleCast(message *erl.Term) {
	nLog("NET_KERNEL: HandleCast: %#v", *message)
}

func (nk *netKernel) HandleCall(message *erl.Term, from *erl.Tuple) (reply *erl.Term) {
	nLog("NET_KERNEL: HandleCall: %#v, From: %#v", *message, *from)
	replyTerm := erl.Term(erl.Atom("yes"))
	reply = &replyTerm
	return
}

func (nk *netKernel) HandleInfo(message *erl.Term) {
	nLog("NET_KERNEL: HandleInfo: %#v", *message)
}

func (nk *netKernel) Terminate(reason interface{}) {
	nLog("NET_KERNEL: Terminate: %#v", reason.(int))
}
