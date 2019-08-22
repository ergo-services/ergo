package ergonode

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type globalNameServer struct {
	GenServer
}

type state struct {
}

func (ns *globalNameServer) Init(p Process, args ...interface{}) interface{} {
	lib.Log("GLOBAL_NAME_SERVER: Init: %#v", args)
	ns.Process = p

	return state{}
}

func (ns *globalNameServer) HandleCast(message *etf.Term, state interface{}) (string, interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: HandleCast: %#v", *message)
	return "noreply", state
}

func (ns *globalNameServer) HandleCall(from *etf.Tuple, message *etf.Term, state interface{}) (string, *etf.Term, interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: HandleCall: %#v, From: %#v", *message, *from)
	replyTerm := etf.Term(etf.Atom("reply"))
	message = &replyTerm
	return "reply", message, state
}

func (ns *globalNameServer) HandleInfo(message *etf.Term, state interface{}) (string, interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: HandleInfo: %#v", *message)
	return "noreply", state
}

func (ns *globalNameServer) Terminate(reason string, state interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: Terminate: %#v", reason)
}
