package ergonode

// https://github.com/erlang/otp/blob/master/lib/observer/src/observer_procinfo.erl

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type rex struct {
	GenServer
	process Process
}

// Init initializes process state using arbitrary arguments
// Init(...) -> state
func (r *rex) Init(p Process, args ...interface{}) (state interface{}) {
	lib.Log("REX: Init: %#v", args)
	r.process = p
	return nil
}

// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (r *rex) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("REX: HandleCast: %#v", message)
	return "noreply", state
}

// HandleCall serves incoming messages sending via gen_server:call
// HandleCall -> ("reply", message, state) - reply
//				 ("noreply", _, state) - noreply
//		         ("stop", reason, _) - normal stop
func (r *rex) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	lib.Log("REX: HandleCall: %#v, From: %#v", message, from)
	switch m := message.(type) {
	case etf.Tuple:
		//etf.Tuple{"call", "observer_backend", "sys_info",
		//           etf.List{}, etf.Pid{Node:"erl-examplenode@127.0.0.1", Id:0x46, Serial:0x0, Creation:0x2}}
		switch m.Element(1) {
		case etf.Atom("call"):
			process_name := string(m.Element(2).(etf.Atom))
			to := etf.Tuple{process_name, r.process.Node.FullName}

			reply, err := r.process.Call(to, m.Element(3))

			if err != nil {
				reply = etf.Term(etf.Tuple{etf.Atom("error"), err})
			}

			return "reply", reply, state
		}

	}

	reply := etf.Term(etf.Atom("unsupported"))
	return "reply", reply, state
}

// HandleInfo serves all another incoming messages (Pid ! message)
// HandleInfo -> ("noreply", state) - noreply
//		         ("stop", reason) - normal stop
func (r *rex) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("REX: HandleInfo: %#v", message)
	return "noreply", state
}

// Terminate called when process died
func (r *rex) Terminate(reason string, state interface{}) {
	lib.Log("REX: Terminate: %#v", reason)
}
