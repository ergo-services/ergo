package ergonode

// TODO: https://github.com/erlang/otp/blob/master/lib/runtime_tools-1.13.1/src/erlang_info.erl

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type erlang struct {
	GenServer
}

type erlangState struct {
	process *Process
}

// Init initializes process state using arbitrary arguments
// Init -> state
func (e *erlang) Init(p *Process, args ...interface{}) interface{} {
	lib.Log("ERLANG: Init: %#v", args)
	return erlangState{
		process: p,
	}
}

// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (e *erlang) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	var newState erlangState = state.(erlangState)
	lib.Log("ERLANG: HandleCast: %#v", message)
	return "noreply", newState
}

// HandleCall serves incoming messages sending via gen_server:call
// HandleCall -> ("reply", message, state) - reply
//				 ("noreply", _, state) - noreply
//		         ("stop", reason, _) - normal stop
func (e *erlang) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	lib.Log("ERLANG: HandleCall: %#v, From: %#v", message, from)
	var newState erlangState = state.(erlangState)

	switch m := message.(type) {
	case etf.Tuple:
		switch m.Element(1) {
		case etf.Atom("process_info"):
			args := m.Element(2).(etf.List)
			reply := process_info(newState.process, args[0].(etf.Pid), args[1])
			return "reply", reply, state
		case etf.Atom("system_info"):
			args := m.Element(2).(etf.List)
			reply := system_info(newState.process, args[0].(etf.Atom))
			return "reply", reply, state

		case etf.Atom("function_exported"):
			return "reply", true, state
		}

	}
	return "reply", etf.Atom("ok"), state
}

// HandleInfo serves all another incoming messages (Pid ! message)
// HandleInfo -> ("noreply", state) - noreply
//		         ("stop", reason) - normal stop
func (e *erlang) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("ERLANG: HandleInfo: %#v", message)
	return "noreply", "normal"
}

// Terminate called when process died
func (e *erlang) Terminate(reason string, state interface{}) {
	lib.Log("ERLANG: Terminate: %#v", reason)
}

func process_info(p *Process, pid etf.Pid, property etf.Term) etf.Term {
	process := p.Node.GetProcessByPid(pid)
	if process == nil {
		return etf.Atom("undefined")
	}

	switch property {
	case etf.Atom("registered_name"):
		name := process.Name()
		if name == "" {
			return etf.List{}
		}

		return etf.Tuple{property, etf.Atom(name)}
	case etf.Atom("messages"):
		return etf.Tuple{property, etf.List{}}
	case etf.Atom("dictionary"):
		return etf.Tuple{property, etf.List{}}
	case etf.Atom("current_stacktrace"):
		return etf.Tuple{property, etf.List{}}
	}

	switch p := property.(type) {
	case etf.List:
		values := etf.List{}
		info := process.Info()
		for i := range p {
			switch p[i] {
			case etf.Atom("binary"):
				values = append(values, etf.Tuple{p[i], etf.List{}})
			case etf.Atom("catchlevel"):
				// values = append(values, etf.Tuple{p[i], 0})
			case etf.Atom("current_function"):
				values = append(values, etf.Tuple{p[i], info.CurrentFunction})
			case etf.Atom("error_handler"):
				// values = append(values, etf.Tuple{p[i], })
			case etf.Atom("garbage_collection"):
				values = append(values, etf.Tuple{p[i], etf.List{}})
			case etf.Atom("group_leader"):
				values = append(values, etf.Tuple{p[i], info.GroupLeader})
			case etf.Atom("heap_size"):
				// values = append(values, etf.Tuple{p[i], etf.Tuple{etf.Atom("words"), 0}})
			case etf.Atom("initial_call"):
				values = append(values, etf.Tuple{p[i], "object:loop"})
			case etf.Atom("last_calls"):
				// values = append(values, etf.Tuple{p[i], })
			case etf.Atom("links"):
				values = append(values, etf.Tuple{p[i], etf.List{}})
			case etf.Atom("memory"):
				values = append(values, etf.Tuple{p[i], 0})
			case etf.Atom("message_queue_len"):
				values = append(values, etf.Tuple{p[i], info.MessageQueueLen})
			case etf.Atom("monitored_by"):
				values = append(values, etf.Tuple{p[i], etf.List{}})
			case etf.Atom("monitors"):
				values = append(values, etf.Tuple{p[i], etf.List{}})
			case etf.Atom("priority"):
				// values = append(values, etf.Tuple{p[i], 0})
			case etf.Atom("reductions"):
				values = append(values, etf.Tuple{p[i], info.Reductions})
			case etf.Atom("registered_name"):
				values = append(values, etf.Tuple{p[i], process.Name()})
			case etf.Atom("sequential_trace_token"):
				// values = append(values, etf.Tuple{p[i], })
			case etf.Atom("stack_size"):
				// values = append(values, etf.Tuple{p[i], etf.Tuple{etf.Atom("words"), 0}})
			case etf.Atom("status"):
				values = append(values, etf.Tuple{p[i], info.Status})
			case etf.Atom("suspending"):
				// values = append(values, etf.Tuple{p[i], })
			case etf.Atom("total_heap_size"):
				// values = append(values, etf.Tuple{p[i], etf.Tuple{etf.Atom("words"), 0}})
			case etf.Atom("trace"):
				// values = append(values, etf.Tuple{p[i], 0})
			case etf.Atom("trap_exit"):
				values = append(values, etf.Tuple{p[i], info.TrapExit})

			}

		}
		return values
	}
	return nil
}

func system_info(p *Process, name etf.Atom) etf.Term {
	switch name {
	case etf.Atom("dirty_cpu_schedulers"):
		return 1
	}
	return etf.Atom("unknown")
}
