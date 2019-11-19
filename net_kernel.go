package ergonode

// https://github.com/erlang/otp/blob/master/lib/kernel/src/net_kernel.erl

import (
	"fmt"
	"syscall"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type netKernelSup struct {
	Supervisor
}

func (nks *netKernelSup) Init(args ...interface{}) SupervisorSpec {
	return SupervisorSpec{
		Children: []SupervisorChildSpec{
			SupervisorChildSpec{
				Name:    "net_kernel",
				Child:   &netKernel{},
				Restart: SupervisorChildRestartPermanent,
			},
			SupervisorChildSpec{
				Name:    "global_name_server",
				Child:   &globalNameServer{},
				Restart: SupervisorChildRestartPermanent,
			},
			SupervisorChildSpec{
				Name:    "rex",
				Child:   &rex{},
				Restart: SupervisorChildRestartPermanent,
			},
			SupervisorChildSpec{
				Name:    "observer_backend",
				Child:   &observerBackend{},
				Restart: SupervisorChildRestartPermanent,
			},
			SupervisorChildSpec{
				Name:    "erlang",
				Child:   &erlang{},
				Restart: SupervisorChildRestartPermanent,
			},
		},
		Strategy: SupervisorStrategy{
			Type:      SupervisorStrategyOneForOne,
			Intensity: 10,
			Period:    5,
		},
	}
}

type netKernel struct {
	GenServer
	process *Process
}

// Init initializes process state using arbitrary arguments
// Init(...) -> state
func (nk *netKernel) Init(p *Process, args ...interface{}) (state interface{}) {
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
					reply = etf.Atom("yes")
				}
			}
		}
		if len(t) == 5 {
			// etf.Tuple{"spawn_link", "observer_backend", "procs_info", etf.List{etf.Pid{}}, etf.Pid{}}
			sendTo := t.Element(4).(etf.List).Element(1).(etf.Pid)
			go sendProcInfo(nk.process, sendTo)
			reply = nk.process.Self()
		}

		usg := syscall.Rusage{}
		if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usg); err != nil {
			fmt.Println("CANT GET RUSAGE for", syscall.Getpid(), err)
		} else {
			fmt.Printf("RES USAGE: %#v\n", usg)
		}

		// etf.Tuple{"spawn_link", "observer_backend", "fetch_stats", etf.List{etf.Pid{}, 500}, etf.Pid{}}
		// https://godoc.org/golang.org/x/sys/unix#Getrusage - for unix

		// {stats,1,
		// 	[{8,1235478,2009146206},
		// 	 {4,1642253,2008966955},
		// 	 {6,1612731,2009166131},
		// 	 {7,1534032,2009125109},
		// 	 {1,1906006,2009181936},
		// 	 {5,1371164,2009112859},
		// 	 {3,1109390,2009100239},
		// 	 {2,7498692,2009133735},
		// 	 {9,242304,14978295546},
		// 	 {10,246047,14978296186},
		// 	 {11,69425,14978296753},
		// 	 {12,120835,14978297087},
		// 	 {13,68511,14978297487},
		// 	 {14,51743,14978298867},
		// 	 {15,131861,14978299200},
		// 	 {16,69327,14978299556}],
		// 	{{input,9207},{output,10528}},
		// 	[{total,17544800},
		// 	 {processes,4835608},
		// 	 {processes_used,4835608},
		// 	 {system,12709192},
		// 	 {atom,256337},
		// 	 {atom_used,232702},
		// 	 {binary,149336},
		// 	 {code,4955622},
		// 	 {ets,356568}]}

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

func sendProcInfo(p *Process, to etf.Pid) {
	list := p.Node.GetProcessList()
	procsInfoList := etf.List{}
	for i := range list {
		info := list[i].Info()
		// {procs_info, self(), etop_collect(Pids, [])}
		procsInfoList = append(procsInfoList,
			etf.Tuple{
				etf.Atom("etop_proc_info"), // record name #etop_proc_info
				list[i].Self(),             // pid
				0,                          // mem
				info.Reductions,            // reds
				etf.Atom(list[i].Name()),   // etf.Tuple{etf.Atom("ergo"), etf.Atom(list[i].Name()), 0}, // name
				0,                          // runtime
				info.CurrentFunction,       // etf.Tuple{etf.Atom("ergo"), etf.Atom(info.CurrentFunction), 0}, // cf
				info.MessageQueueLen,       // mq
			},
		)

	}

	procsInfo := etf.Tuple{
		etf.Atom("procs_info"),
		p.Self(),
		procsInfoList,
	}
	p.Send(to, procsInfo)
	// observer waits for the EXIT message since this function was executed via spawn
	p.Send(to, etf.Tuple{etf.Atom("EXIT"), p.Self(), etf.Atom("normal")})
}
