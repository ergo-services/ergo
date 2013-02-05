package node

import (
	"github.com/goerlang/etf"
)

type globalNameServer struct {
	GenServerImpl
}

func (gns *globalNameServer) Init(args ...interface{}) {
	nLog("GLOBAL_NAME_SERVER: Init: %#v", args)
	gns.Node.Register(etf.Atom("global_name_server"), gns.Self)
}

func (gns *globalNameServer) HandleCast(message *etf.Term) {
	nLog("GLOBAL_NAME_SERVER: HandleCast: %#v", *message)
}

func (gns *globalNameServer) HandleCall(message *etf.Term, from *etf.Tuple) (reply *etf.Term) {
	nLog("GLOBAL_NAME_SERVER: HandleCall: %#v, From: %#v", *message, *from)
	replyTerm := etf.Term(etf.Atom("reply"))
	reply = &replyTerm
	return
}

func (gns *globalNameServer) HandleInfo(message *etf.Term) {
	nLog("GLOBAL_NAME_SERVER: HandleInfo: %#v", *message)
}

func (gns *globalNameServer) Terminate(reason interface{}) {
	nLog("GLOBAL_NAME_SERVER: Terminate: %#v", reason.(int))
}
