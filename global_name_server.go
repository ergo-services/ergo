package node

import (
	"erlang/term"
)

type globalNameServer struct {
	gsi *GenServerImpl
}

func (gns *globalNameServer) Behaviour() (behaviour Behaviour, options map[string]interface{}) {
	gns.gsi = &GenServerImpl{}
	return gns.gsi, gns.gsi.Options()
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
