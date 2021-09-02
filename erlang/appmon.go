package erlang

// TODO: https://github.com/erlang/otp/blob/master/lib/runtime_tools-1.13.1/src/appmon_info.erl

import (
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/lib"
	"github.com/halturin/ergo/node"
)

type appMon struct {
	gen.Server
}

type appMonState struct {
	jobs map[etf.Atom][]jobDetails
}

type jobDetails struct {
	name   etf.Atom
	args   etf.List
	sendTo etf.Pid
}

// Init initializes process state using arbitrary arguments
// Init -> state
func (am *appMon) Init(process *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("APP_MON: Init %#v", args)
	from := args[0]
	process.Link(from.(etf.Pid))
	process.State = &appMonState{
		jobs: make(map[etf.Atom][]jobDetails),
	}
	return nil
}

func (am *appMon) HandleCast(process *gen.ServerProcess, message etf.Term) string {
	var appState *appMonState = process.State.(*appMonState)
	lib.Log("APP_MON: HandleCast: %#v", message)
	node := process.Env("node").(node.Node)
	switch message {
	case "sendStat":

		for cmd, jobs := range appState.jobs {
			switch cmd {
			case "app_ctrl":
				// From ! {delivery, self(), Cmd, Aux, Result}
				apps := node.WhichApplications()
				for i := range jobs {
					appList := make(etf.List, len(apps))
					for ai, a := range apps {
						appList[ai] = etf.Tuple{a.PID, etf.Atom(a.Name),
							etf.Tuple{etf.Atom(a.Name), a.Description, a.Version},
						}
					}
					delivery := etf.Tuple{etf.Atom("delivery"), process.Self(), cmd, jobs[i].name, appList}
					process.Send(jobs[i].sendTo, delivery)
				}

			case "app":
				for i := range jobs {
					appTree := am.makeAppTree(process, jobs[i].name)
					if appTree == nil {
						continue
					}
					delivery := etf.Tuple{etf.Atom("delivery"), process.Self(), cmd, jobs[i].name, appTree}
					process.Send(jobs[i].sendTo, delivery)
				}

			}
		}

		process.CastAfter(process.Self(), "sendStat", 2*time.Second)
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
					if len(appState.jobs) == 0 {
						process.Cast(process.Self(), "sendStat")
					}

					if jobList, ok := appState.jobs[m.Element(2).(etf.Atom)]; ok {
						for i := range jobList {
							if jobList[i].name == job.name {
								return "noreply"
							}
						}
						jobList = append(jobList, job)
						appState.jobs[m.Element(2).(etf.Atom)] = jobList
					} else {
						appState.jobs[m.Element(2).(etf.Atom)] = []jobDetails{job}
					}

				} else {
					// remove a job
					if jobList, ok := appState.jobs[m.Element(2).(etf.Atom)]; ok {
						for i := range jobList {
							if jobList[i].name == job.name {
								jobList[i] = jobList[0]
								jobList = jobList[1:]

								if len(jobList) > 0 {
									appState.jobs[m.Element(2).(etf.Atom)] = jobList
								} else {
									delete(appState.jobs, m.Element(2).(etf.Atom))
								}
								break
							}
						}
					}

					if len(appState.jobs) == 0 {
						return "stop"
					}

				}
				return "noreply"
			}
		}
	}

	return "stop"
}

func (am *appMon) makeAppTree(process gen.Process, app etf.Atom) etf.Tuple {
	node := process.Env("node").(node.Node)
	appInfo, err := node.ApplicationInfo(string(app))
	if err != nil {
		return nil
	}

	resolver := make(map[etf.Pid]interface{})

	tree := makeTree(process, resolver, appInfo.PID)
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

func makeTree(process gen.Process, resolver map[etf.Pid]interface{}, pid etf.Pid) etf.List {

	pidProcess := process.ProcessByPid(pid)
	if pidProcess == nil {
		return etf.List{}
	}
	if name := pidProcess.Name(); name != "" {
		resolver[pid] = name
	} else {
		resolver[pid] = pid.String()
	}

	tree := etf.List{}

	for _, cp := range pidProcess.Children() {
		children := makeTree(process, resolver, cp)
		child := etf.Tuple{resolver[pid], resolver[cp]}
		tree = append(tree, child)
		tree = append(tree, children...)
	}

	return tree
}
