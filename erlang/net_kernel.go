package erlang

// https://github.com/erlang/otp/blob/master/lib/kernel/src/net_kernel.erl

import (
	"context"
	"runtime"
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/lib"
)

type KernelApp struct {
	gen.Application
}

func (nka *KernelApp) Load(args ...etf.Term) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        "erlang",
		Description: "Erlang support app",
		Version:     "v.1.0",
		Children: []gen.ApplicationChildSpec{
			gen.ApplicationChildSpec{
				Child: &netKernelSup{},
				Name:  "net_kernel_sup",
			},
		},
	}, nil
}

func (nka *KernelApp) Start(p gen.Process, args ...etf.Term) {}

type netKernelSup struct {
	gen.Supervisor
}

func (nks *netKernelSup) Init(args ...etf.Term) (gen.SupervisorSpec, error) {
	return gen.SupervisorSpec{
		Children: []gen.SupervisorChildSpec{
			gen.SupervisorChildSpec{
				Name:  "net_kernel",
				Child: &netKernel{},
			},
			gen.SupervisorChildSpec{
				Name:  "global_name_server",
				Child: &globalNameServer{},
			},
			gen.SupervisorChildSpec{
				Name:  "rex",
				Child: &rex{},
			},
			//gen.SupervisorChildSpec{
			//	Name:    "observer_backend",
			//	Child:   &observerBackend{},
			//},
			gen.SupervisorChildSpec{
				Name:  "erlang",
				Child: &erlang{},
			},
		},
		Strategy: gen.SupervisorStrategy{
			Type:      gen.SupervisorStrategyOneForOne,
			Intensity: 10,
			Period:    5,
			Restart:   gen.SupervisorStrategyRestartPermanent,
		},
	}, nil
}

type netKernel struct {
	gen.Server
	routinesCtx map[etf.Pid]context.CancelFunc
}

func (nk *netKernel) Init(process *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("NET_KERNEL: Init: %#v", args)
	nk.routinesCtx = make(map[etf.Pid]context.CancelFunc)
	return nil
}

func (nk *netKernel) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (code string, reply etf.Term) {
	lib.Log("NET_KERNEL: HandleCall: %#v, From: %#v", message, from)
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
			switch t.Element(3) {
			case etf.Atom("procs_info"):
				// etf.Tuple{"spawn_link", "observer_backend", "procs_info", etf.List{etf.Pid{}}, etf.Pid{}}
				sendTo := t.Element(4).(etf.List).Element(1).(etf.Pid)
				go sendProcInfo(process, sendTo)
				reply = state.Process.Self()
			case etf.Atom("fetch_stats"):
				// etf.Tuple{"spawn_link", "observer_backend", "fetch_stats", etf.List{etf.Pid{}, 500}, etf.Pid{}}
				sendTo := t.Element(4).(etf.List).Element(1).(etf.Pid)
				period := t.Element(4).(etf.List).Element(2).(int)
				if _, ok := nk.routinesCtx[sendTo]; ok {
					reply = etf.Atom("error")
					return
				}

				process.MonitorProcess(sendTo)
				ctx, cancel := context.WithCancel(process.Context())
				nk.routinesCtx[sendTo] = cancel
				go sendStats(ctx, process, sendTo, period, cancel)
				reply = state.Process.Self()
			}
		}

	}
	return
}

func (nk *netKernel) HandleInfo(process *gen.ServerProcess, message etf.Term) string {
	lib.Log("NET_KERNEL: HandleInfo: %#v", message)
	// {"DOWN", etf.Ref{Node:"demo@127.0.0.1", Creation:0x1, Id:[]uint32{0x27715, 0x5762, 0x0}}, "process",
	// etf.Pid{Node:"erl-demo@127.0.0.1", Id:0x460, Serial:0x0, Creation:0x1}, "normal"}
	switch m := message.(type) {
	case etf.Tuple:
		if m.Element(1) == etf.Atom("DOWN") {
			pid := m.Element(4).(etf.Pid)
			if cancel, ok := nk.routinesCtx[pid]; ok {
				cancel()
				delete(nk.routinesCtx, pid)
			}
		}

	}
	return "noreply"
}

func sendProcInfo(p gen.Process, to etf.Pid) {
	list := p.ProcessList()
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
	// erlang's observer waits for the EXIT message since this function was executed via spawn
	p.Send(to, etf.Tuple{etf.Atom("EXIT"), p.Self(), etf.Atom("normal")})
}

func sendStats(ctx context.Context, p gen.Process, to etf.Pid, period int, cancel context.CancelFunc) {
	var utime, utimetotal, stime, stimetotal int64
	defer cancel()
	for {

		select {
		case <-time.After(time.Duration(period) * time.Millisecond):

			runtime.ReadMemStats(&m)

			total := etf.Tuple{etf.Atom("total"), m.TotalAlloc}
			system := etf.Tuple{etf.Atom("system"), m.HeapSys}
			processes := etf.Tuple{etf.Atom("processes"), m.Alloc}
			processesUsed := etf.Tuple{etf.Atom("processes_used"), m.HeapInuse}
			atom := etf.Tuple{etf.Atom("atom"), 0}
			atomUsed := etf.Tuple{etf.Atom("atom_used"), 0}
			binary := etf.Tuple{etf.Atom("binary"), 0}
			code := etf.Tuple{etf.Atom("code"), 0}
			ets := etf.Tuple{etf.Atom("ets"), 0}

			utime, stime = os_dep_getResourceUsage()
			utimetotal += utime
			stimetotal += stime
			stats := etf.Tuple{
				etf.Atom("stats"),
				1,
				etf.List{
					etf.Tuple{1, utime, utimetotal},
					etf.Tuple{2, stime, stimetotal},
				},
				etf.Tuple{
					etf.Tuple{etf.Atom("input"), 0},
					etf.Tuple{etf.Atom("output"), 0},
				},
				etf.List{
					total,
					system,
					processes,
					processesUsed,
					atom,
					atomUsed,
					binary,
					code,
					ets,
				},
			}
			p.Send(to, stats)
		case <-ctx.Done():
			return
		}
	}
}
