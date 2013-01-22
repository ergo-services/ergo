package node

import (
	"erlang/term"
)

type globalNameServer struct {
	node *Node
	self term.Pid
}

func (nk *globalNameServer) Behaviour() (behaviour Behaviour, options map[string]interface{}) {
	gsi := &GenServerImpl{}
	return gsi, gsi.Options()
}

func (gns *globalNameServer) setNode(node *Node) {
	gns.node = node
}

func (gns *globalNameServer) setPid(pid term.Pid) {
	gns.self = pid
}

func (gns *globalNameServer) Init(args ...interface{}) {
	nLog("GLOBAL_NAME_SERVER: Init: %#v", args)
}

func (gns *globalNameServer) HandleCast(message *term.Term) {
	nLog("GLOBAL_NAME_SERVER: HandleCast: %#v", *message)
}

func (gns *globalNameServer) HandleCall(message *term.Term, from *term.Tuple) (reply *term.Term) {
	nLog("GLOBAL_NAME_SERVER: HandleCall: %#v, From: %#v", *message, *from)
	replyTerm := term.Term(term.Atom("reply"))
	reply = &replyTerm
	return
}

func (gns *globalNameServer) HandleInfo(message *term.Term) {
	nLog("GLOBAL_NAME_SERVER: HandleInfo: %#v", *message)
}

func (gns *globalNameServer) Terminate(reason interface{}) {
	nLog("GLOBAL_NAME_SERVER: Terminate: %#v", reason.(int))
}
