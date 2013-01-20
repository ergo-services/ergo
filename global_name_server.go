package node

import (
	"erlang/term"
)

type globalNameServer struct {
	node *Node
}

func (nk *globalNameServer) Behaviour() (behaviour Behaviour, options map[string]interface{}) {
	gsi := &GenServerImpl{}
	return gsi, gsi.Options()
}

func (gns *globalNameServer) Init(args ...interface{}) {
	nLog("GLOBAL_NAME_SERVER: Init: %#v", args)
	gns.node = args[0].(*Node)
}

func (gns *globalNameServer) HandleCast(message *term.Term) {
	nLog("GLOBAL_NAME_SERVER: HandleCast: %#v", *message)
}

func (gns *globalNameServer) HandleCall(message *term.Term, from *term.Tuple) {
	nLog("GLOBAL_NAME_SERVER: HandleCall: %#v, From: %#v", *message, *from)
}

func (gns *globalNameServer) HandleInfo(message *term.Term) {
	nLog("GLOBAL_NAME_SERVER: HandleInfo: %#v", *message)
}

func (gns *globalNameServer) Terminate(reason interface{}) {
	nLog("GLOBAL_NAME_SERVER: Terminate: %#v", reason.(int))
}
