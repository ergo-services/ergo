package ergonode

// https://github.com/erlang/otp/blob/master/lib/observer/src/observer_procinfo.erl

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type observer struct {
	GenServer
}

// Init initializes process state using arbitrary arguments
// Init(...) -> state
func (o *observer) Init(p Process, args ...interface{}) (state interface{}) {
	lib.Log("OBSERVER: Init: %#v", args)
	o.Process = p
	return nil
}

// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (o *observer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("OBSERVER: HandleCast: %#v", message)
	return "noreply", state
}

// HandleCall serves incoming messages sending via gen_server:call
// HandleCall -> ("reply", message, state) - reply
//				 ("noreply", _, state) - noreply
//		         ("stop", reason, _) - normal stop
func (o *observer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	lib.Log("OBSERVER: HandleCall: %#v, From: %#v", message, from)
	reply := etf.Term("reply")
	return "reply", reply, state
}

// HandleInfo serves all another incoming messages (Pid ! message)
// HandleInfo -> ("noreply", state) - noreply
//		         ("stop", reason) - normal stop
func (o *observer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("OBSERVER: HandleInfo: %#v", message)
	return "noreply", state
}

// Terminate called when process died
func (o *observer) Terminate(reason string, state interface{}) {
	lib.Log("OBSERVER: Terminate: %#v", reason)
}
