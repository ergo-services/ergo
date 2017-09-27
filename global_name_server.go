package ergonode

import (
	"github.com/halturin/ergonode/etf"
)

type globalNameServer struct {
	GenServerImpl
}

func (gns *globalNameServer) Init(args ...interface{}) (state interface{}) {
	nLog("GLOBAL_NAME_SERVER: Init: %#v", args)
	gns.Node.Register(etf.Atom("global_name_server"), gns.Self)
	return nil
}

func (gns *globalNameServer) HandleCast(message *etf.Term, state interface{}) (code int, stateout interface{}) {
	nLog("GLOBAL_NAME_SERVER: HandleCast: %#v", *message)
	stateout = state
	code = 0
	return
}

func (gns *globalNameServer) HandleCall(from *etf.Tuple, message *etf.Term, state interface{}) (code int, reply *etf.Term, stateout interface{}) {
	nLog("GLOBAL_NAME_SERVER: HandleCall: %#v, From: %#v", *message, *from)
	stateout = state
	code = 1
	replyTerm := etf.Term(etf.Atom("reply"))
	reply = &replyTerm
	return
}

func (gns *globalNameServer) HandleInfo(message *etf.Term, state interface{}) (code int, stateout interface{}) {
	nLog("GLOBAL_NAME_SERVER: HandleInfo: %#v", *message)
	stateout = state
	code = 0
	return
}

func (gns *globalNameServer) Terminate(reason int, state interface{}) {
	nLog("GLOBAL_NAME_SERVER: Terminate: %#v", reason)
}
