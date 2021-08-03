package ergo

// TODO: https://github.com/erlang/otp/blob/master/lib/runtime_tools-1.13.1/src/appmon_info.erl

import (
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
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

	return appMonState{
		process: p,
		jobs:    make(map[etf.Atom][]jobDetails),
	}
}

func (am *appMon) HandleCast(message etf.Term, state interface{}) string {
	var appState appMonState = state.(appMonState)
	lib.Log("APP_MON: HandleCast: %#v", message)
	switch message {
	case "sendStat":

		for cmd, jobs := range state.(appMonState).jobs {
			switch cmd {
			case "app_ctrl":
				// From ! {delivery, self(), Cmd, Aux, Result}
				apps := appState.process.Node.WhichApplications()
				for i := range jobs {
					appList := make(etf.List, len(apps))
					for ai, a := range apps {
						appList[ai] = etf.Tuple{a.PID, etf.Atom(a.Name),
							etf.Tuple{etf.Atom(a.Name), a.Description, a.Version},
						}
					}
					delivery := etf.Tuple{etf.Atom("delivery"), appState.process.Self(), cmd, jobs[i].name, appList}
					appState.process.Send(jobs[i].sendTo, delivery)
				}

			case "app":
				for i := range jobs {
					appTree := am.makeAppTree(appState.process, jobs[i].name)
					if appTree == nil {
						continue
					}
					delivery := etf.Tuple{etf.Atom("delivery"), appState.process.Self(), cmd, jobs[i].name, appTree}
					appState.process.Send(jobs[i].sendTo, delivery)
				}

			}
		}

		appState.process.CastAfter(appState.process.Self(), "sendStat", 2*time.Second)
		return "noreply"

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
					if len(state.(appMonState).jobs) == 0 {
						appState.process.Cast(appState.process.Self(), "sendStat")
					}

					if jobList, ok := state.(appMonState).jobs[m.Element(2).(etf.Atom)]; ok {
						for i := range jobList {
							if jobList[i].name == job.name {
								return "noreply"
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

					if len(state.(appMonState).jobs) == 0 {
						return "stop"
					}

				}
				return "noreply"
			}
		}
	}

	return "stop"
}

func (am *appMon) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term) {
	lib.Log("APP_MON: HandleCall: %#v, From: %#v", message, from)
	// return "reply", reply, state
	return "stop", nil
}

func (am *appMon) HandleInfo(message etf.Term, state interface{}) string {
	lib.Log("APP_MON: HandleInfo: %#v", message)
	return "stop"
}

func (am *appMon) makeAppTree(p *Process, app etf.Atom) etf.Tuple {
	appInfo, err := p.Node.GetApplicationInfo(string(app))
	if err != nil {
		return nil
	}

	resolver := make(map[etf.Pid]interface{})

	tree := makeTree(p, resolver, appInfo.PID)
	children := etf.List{etf.Tuple{appInfo.PID, appInfo.PID.String()}}
	for p, n := range resolver {
		children = append(children, etf.Tuple{p, n})
	}

	appTree := etf.Tuple{
		appInfo.PID.String(), // pid or registered name
		children,
		tree,
		etf.List{}, // TODO: links
	}

	return appTree
}

func makeTree(p *Process, resolver map[etf.Pid]interface{}, pid etf.Pid) etf.List {

	pidProcess := p.Node.GetProcessByPid(pid)
	if pidProcess == nil {
		return etf.List{}
	}
	if pidProcess.name != "" {
		resolver[pid] = pidProcess.name
	} else {
		resolver[pid] = pid.String()
	}

	tree := etf.List{}

	for _, cp := range pidProcess.GetChildren() {
		children := makeTree(p, resolver, cp)
		child := etf.Tuple{resolver[pid], resolver[cp]}
		tree = append(tree, child)
		tree = append(tree, children...)
	}

	return tree
}
