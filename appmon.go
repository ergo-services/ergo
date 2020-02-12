package ergonode

// TODO: https://github.com/erlang/otp/blob/master/lib/runtime_tools-1.13.1/src/appmon_info.erl

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
	jobs    map[etf.Atom][]jobDetails
}

type jobDetails struct {
	name   etf.Atom
	args   etf.List
	sendTo etf.Pid
}

// Init initializes process state using arbitrary arguments
// Init -> state
func (am *appMon) Init(p *Process, args ...interface{}) interface{} {
	lib.Log("APP_MON: Init %#v", args)
	from := args[0]
	p.Link(from.(etf.Pid))

	p.CastAfter(p.Self(), "sendStat", 2*time.Second)

	return appMonState{
		process: p,
		jobs:    make(map[etf.Atom][]jobDetails),
	}
}

// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (am *appMon) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	var newState appMonState = state.(appMonState)
	lib.Log("APP_MON: HandleCast: %#v", message)
	switch message {
	case "sendStat":
		if len(state.(appMonState).jobs) == 0 {
			return "stop", "normal"
		}

		for cmd, jobs := range state.(appMonState).jobs {
			switch cmd {
			case "app_ctrl":
				// From ! {delivery, self(), Cmd, Aux, Result}
				apps := newState.process.Node.WhichApplications()
				fmt.Println("APPS: ", apps)
				for i := range jobs {
					// appInfo := etf.Tuple{etf.Atom("firstApp"), "my app", "version 01.01"}
					appList := make(etf.List, len(apps))
					for ai, a := range apps {
						appList[ai] = etf.Tuple{jobs[i].sendTo, etf.Atom(a.Name),
							etf.Tuple{etf.Atom(a.Name), a.Description, a.Version},
						}
					}
					fmt.Printf("AppLIST: %#v\n", appList)
					// appList := etf.List{etf.Tuple{jobs[i].sendTo, appInfo.Element(1), appInfo}}
					delivery := etf.Tuple{etf.Atom("delivery"), newState.process.Self(), cmd, jobs[i].name, appList}
					newState.process.Send(jobs[i].sendTo, delivery)
				}

			case "app":
				for i := range jobs {
					fmt.Println("DO JOB for ", jobs[i])
					appTree := etf.Tuple{
						jobs[i].name, // pid
						etf.List{etf.Tuple{111, "a3"}, etf.Tuple{222, "a2"}},     // children
						etf.List{etf.Tuple{newState.process.Self(), "appppppp"}}, // links
						etf.List{}, // remote links
					}
					delivery := etf.Tuple{etf.Atom("delivery"), newState.process.Self(), cmd, jobs[i].name, appTree}
					newState.process.Send(jobs[i].sendTo, delivery)
				}

			}
		}

		newState.process.CastAfter(newState.process.Self(), "sendStat", 2*time.Second)
		return "noreply", state

	default:
		switch m := message.(type) {
		case etf.Tuple:
			if len(m) == 5 {
				// etf.Tuple{etf.Pid{Node:"erl-demo@127.0.0.1", Id:0x7c, Serial:0x0, Creation:0x1}, "app_ctrl", "demo@127.0.0.1", "true", etf.List{}}
				job := jobDetails{
					name:   m.Element(3).(etf.Atom),
					args:   m.Element(5).(etf.List),
					sendTo: m.Element(1).(etf.Pid),
				}

				if m.Element(4) == etf.Atom("true") {
					// add new job

					if jobList, ok := state.(appMonState).jobs[m.Element(2).(etf.Atom)]; ok {
						for i := range jobList {
							if jobList[i].name == job.name {
								return "noreply", newState
							}
						}
						jobList = append(jobList, job)
						state.(appMonState).jobs[m.Element(2).(etf.Atom)] = jobList
					} else {
						state.(appMonState).jobs[m.Element(2).(etf.Atom)] = []jobDetails{job}
					}

				} else {
					// remove a job
					if jobList, ok := state.(appMonState).jobs[m.Element(2).(etf.Atom)]; ok {
						for i := range jobList {
							if jobList[i].name == job.name {
								jobList[i] = jobList[0]
								jobList = jobList[1:]

								if len(jobList) > 0 {
									state.(appMonState).jobs[m.Element(2).(etf.Atom)] = jobList
								} else {
									delete(state.(appMonState).jobs, m.Element(2).(etf.Atom))
								}
								break
							}
						}
					}
				}
				return "noreply", newState
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
	// return "reply", reply, state
	return "stop", "normal", state
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
