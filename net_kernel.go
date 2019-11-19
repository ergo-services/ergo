package ergonode

// https://github.com/erlang/otp/blob/master/lib/kernel/src/net_kernel.erl

import (
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
