package ergonode

// https://github.com/erlang/otp/blob/master/lib/kernel/src/net_kernel.erl

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type netKernel struct {
	GenServer
	process Process
}

// Init initializes process state using arbitrary arguments
// Init(...) -> state
func (nk *netKernel) Init(p Process, args ...interface{}) (state interface{}) {
	lib.Log("NET_KERNEL: Init: %#v", args)
	nk.process = p

	return nil
}

// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (nk *netKernel) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("NET_KERNEL: HandleCast: %#v", message)
	return "noreply", state
}

// HandleCall serves incoming messages sending via gen_server:call
// HandleCall -> ("reply", message, state) - reply
//				 ("noreply", _, state) - noreply
//		         ("stop", reason, _) - normal stop
func (nk *netKernel) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (code string, reply etf.Term, stateout interface{}) {
	lib.Log("NET_KERNEL: HandleCall: %#v, From: %#v", message, from)
	stateout = state
	code = "reply"
	switch t := (message).(type) {
	case etf.Tuple:
		if len(t) == 2 {
			switch tag := t[0].(type) {
			case etf.Atom:
				if string(tag) == "is_auth" {
					lib.Log("NET_KERNEL: is_auth: %#v", t[1])
					reply = etf.Term(etf.Atom("yes"))
				}
			}
		}
	}
	return
}

// HandleInfo serves all another incoming messages (Pid ! message)
// HandleInfo -> ("noreply", state) - noreply
//		         ("stop", reason) - normal stop
func (nk *netKernel) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("NET_KERNEL: HandleInfo: %#v", message)
	return "noreply", state
}

// Terminate called when process died
func (nk *netKernel) Terminate(reason string, state interface{}) {
	lib.Log("NET_KERNEL: Terminate: %#v", reason)
}
