package ergonode

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type globalNameServer struct {
	GenServer
}

func (gns *globalNameServer) Init(args ...interface{}) (state interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: Init: %#v", args)
	gns.Node.Register(etf.Atom("global_name_server"), gns.Self)
	return nil
}

func (gns *globalNameServer) HandleCast(message *etf.Term, state interface{}) (code int, stateout interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: HandleCast: %#v", *message)
	stateout = state
	code = 0
	return
}

func (gns *globalNameServer) HandleCall(from *etf.Tuple, message *etf.Term, state interface{}) (code int, reply *etf.Term, stateout interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: HandleCall: %#v, From: %#v", *message, *from)
	stateout = state
	code = 1
	replyTerm := etf.Term(etf.Atom("reply"))
	reply = &replyTerm
	return
}

func (gns *globalNameServer) HandleInfo(message *etf.Term, state interface{}) (code int, stateout interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: HandleInfo: %#v", *message)
	stateout = state
	code = 0
	return
}

func (gns *globalNameServer) Terminate(reason int, state interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: Terminate: %#v", reason)
}
