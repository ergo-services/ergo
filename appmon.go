package ergonode

// TODO: https://github.com/erlang/otp/blob/master/lib/kernel/src/global.erl

import (
	"fmt"
	"time"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type appMon struct {
	GenServer
}

type appMonState struct {
	process *Process
	running bool
	sendTo  etf.Pid
	aux     etf.Atom
	cmd     etf.Atom
}

// Init initializes process state using arbitrary arguments
// Init -> state
func (am *appMon) Init(p *Process, args ...interface{}) interface{} {
	lib.Log("APP_MON: Init: %#v", args)
	from := args[0]
	p.Link(from.(etf.Pid))

	go func() {
		time.Sleep(3 * time.Second)
		p.Cast(p.Self(), "check")
	}()

	return appMonState{process: p}
}

// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (am *appMon) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	var newState appMonState = state.(appMonState)
	lib.Log("APP_MON: HandleCast: %#v", message)
	switch message {
	case "check":
		if state.(appMonState).running {
			return "noreply", state
		}
		return "stor", "normal" //
	case "sendStat":
		// From ! {delivery, self(), Cmd, Aux, Result}
		appList := etf.List{etf.Tuple{etf.Atom("kernel"), "my app", "version 01.01"}}
		delivery := etf.Tuple{etf.Atom("delivery"), newState.process.Self(), newState.cmd, newState.aux, appList}
		newState.process.Send(newState.sendTo, delivery)
		go func() {
			time.Sleep(2 * time.Second)
			newState.process.Cast(newState.process.Self(), "sendStat")
		}()
		return "noreply", state

	default:
		switch m := message.(type) {
		case etf.Tuple:
			if len(m) == 5 {
				// etf.Tuple{etf.Pid{Node:"erl-demo@127.0.0.1", Id:0x7c, Serial:0x0, Creation:0x1}, "app_ctrl", "demo@127.0.0.1", "true", etf.List{}}
				if m.Element(4) == etf.Atom("true") {
					newState.sendTo = m.Element(1).(etf.Pid)
					newState.running = true
					newState.aux = m.Element(3).(etf.Atom)
					newState.cmd = m.Element(2).(etf.Atom)

					go func() {
						time.Sleep(2 * time.Second)
						newState.process.Cast(newState.process.Self(), "sendStat")
					}()

					return "noreply", newState
				}
				fmt.Println("YYYY2")
			}

			// etf.Tuple{etf.Atom("EXIT"), Pid, reason}

		}
	}

	return "stop", "normal"
}

// HandleCall serves incoming messages sending via gen_server:call
// HandleCall -> ("reply", message, state) - reply
//				 ("noreply", _, state) - noreply
//		         ("stop", reason, _) - normal stop
func (am *appMon) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	lib.Log("APP_MON: HandleCall: %#v, From: %#v", message, from)
	reply := etf.Term(etf.Atom("reply"))
	return "reply", reply, state
}

// HandleInfo serves all another incoming messages (Pid ! message)
// HandleInfo -> ("noreply", state) - noreply
//		         ("stop", reason) - normal stop
func (am *appMon) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("APP_MON: HandleInfo: %#v", message)
	return "stop", "normal"
}

// Terminate called when process died
func (am *appMon) Terminate(reason string, state interface{}) {
	lib.Log("APP_MON: Terminate: %#v", reason)
}
