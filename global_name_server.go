package ergonode

// TODO: https://github.com/erlang/otp/blob/master/lib/kernel/src/global.erl

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type globalNameServer struct {
	GenServer
	process *Process
}

type state struct {
}

// Init initializes process state using arbitrary arguments
// Init -> state
func (ns *globalNameServer) Init(p *Process, args ...interface{}) interface{} {
	lib.Log("GLOBAL_NAME_SERVER: Init: %#v", args)
	ns.process = p

	return state{}
}

// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (ns *globalNameServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: HandleCast: %#v", message)
	return "noreply", state
}

// HandleCall serves incoming messages sending via gen_server:call
// HandleCall -> ("reply", message, state) - reply
//				 ("noreply", _, state) - noreply
//		         ("stop", reason, _) - normal stop
func (ns *globalNameServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: HandleCall: %#v, From: %#v", message, from)
	reply := etf.Term(etf.Atom("reply"))
	return "reply", reply, state
}

// HandleInfo serves all another incoming messages (Pid ! message)
// HandleInfo -> ("noreply", state) - noreply
//		         ("stop", reason) - normal stop
func (ns *globalNameServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: HandleInfo: %#v", message)
	return "noreply", state
}

// Terminate called when process died
func (ns *globalNameServer) Terminate(reason string, state interface{}) {
	lib.Log("GLOBAL_NAME_SERVER: Terminate: %#v", reason)
}
