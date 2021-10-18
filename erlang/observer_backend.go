package erlang

// https://github.com/erlang/otp/blob/master/lib/observer/src/observer_procinfo.erl

import (
	"runtime"
	"unsafe"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

var m runtime.MemStats

type observerBackend struct {
	gen.Server
}

// Init initializes process state using arbitrary arguments
func (o *observerBackend) Init(process *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("OBSERVER: Init: %#v", args)

	funProcLibInitialCall := func(a ...etf.Term) etf.Term {
		return etf.Tuple{etf.Atom("proc_lib"), etf.Atom("init_p"), 5}
	}
	node := process.Env("ergo:Node").(node.Node)
	node.ProvideRPC("proc_lib", "translate_initial_call", funProcLibInitialCall)

	funAppmonInfo := func(a ...etf.Term) etf.Term {
		from := a[0] // pid
		am, e := process.Spawn("", gen.ProcessOptions{}, &appMon{}, from)
		if e != nil {
			return etf.Tuple{etf.Atom("error")}
		}
		return etf.Tuple{etf.Atom("ok"), am.Self()}
	}
	node.ProvideRPC("appmon_info", "start_link2", funAppmonInfo)

	return nil
}

// HandleCall
func (o *observerBackend) HandleCall(state *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	lib.Log("OBSERVER: HandleCall: %v, From: %#v", message, from)
	function := message.(etf.Tuple).Element(1).(etf.Atom)
	// args := message.(etf.Tuple).Element(2).(etf.List)
	switch function {
	case etf.Atom("sys_info"):
		//etf.Tuple{"call", "observer_backend", "sys_info",
		//           etf.List{}, etf.Pid{Node:"erl-examplenode@127.0.0.1", Id:0x46, Serial:0x0, Creation:0x2}}
		reply := etf.Term(o.sysInfo(state.Process))
		return reply, gen.ServerStatusOK
	case etf.Atom("get_table_list"):
		// TODO: add here implementation if we decide to support ETS tables
		// args should be like:
		// etf.List{"ets", etf.List{etf.Tuple{"sys_hidden", "true"}, etf.Tuple{"unread_hidden", "true"}}}
		reply := etf.Term(etf.List{})
		return reply, gen.ServerStatusOK
	case etf.Atom("get_port_list"):
		reply := etf.Term(etf.List{})
		return reply, gen.ServerStatusOK
	}

	return "ok", gen.ServerStatusOK
}

func (o *observerBackend) sysInfo(p gen.Process) etf.List {
	// observer_backend:sys_info()
	node := p.Env("ergo:Node").(node.Node)
	processCount := etf.Tuple{etf.Atom("process_count"), len(p.ProcessList())}
	processLimit := etf.Tuple{etf.Atom("process_limit"), 262144}
	atomCount := etf.Tuple{etf.Atom("atom_count"), 0}
	atomLimit := etf.Tuple{etf.Atom("atom_limit"), 1}
	etsCount := etf.Tuple{etf.Atom("ets_count"), 0}
	etsLimit := etf.Tuple{etf.Atom("ets_limit"), 1}
	portCount := etf.Tuple{etf.Atom("port_count"), 0}
	portLimit := etf.Tuple{etf.Atom("port_limit"), 1}
	ut := node.Uptime()
	uptime := etf.Tuple{etf.Atom("uptime"), ut * 1000}
	runQueue := etf.Tuple{etf.Atom("run_queue"), 0}
	ioInput := etf.Tuple{etf.Atom("io_input"), 0}
	ioOutput := etf.Tuple{etf.Atom("io_output"), 0}
	logicalProcessors := etf.Tuple{etf.Atom("logical_processors"), runtime.NumCPU()}
	logicalProcessorsOnline := etf.Tuple{etf.Atom("logical_processors_online"), runtime.NumCPU()}
	logicalProcessorsAvailable := etf.Tuple{etf.Atom("logical_processors_available"), runtime.NumCPU()}
	schedulers := etf.Tuple{etf.Atom("schedulers"), 1}
	schedulersOnline := etf.Tuple{etf.Atom("schedulers_online"), 1}
	schedulersAvailable := etf.Tuple{etf.Atom("schedulers_available"), 1}
	v := node.Version()
	otpRelease := etf.Tuple{etf.Atom("otp_release"), v.OTP}
	version := etf.Tuple{etf.Atom("version"), etf.Atom(v.Release)}
	systemArchitecture := etf.Tuple{etf.Atom("system_architecture"), etf.Atom(runtime.GOARCH)}
	kernelPoll := etf.Tuple{etf.Atom("kernel_poll"), true}
	smpSupport := etf.Tuple{etf.Atom("smp_support"), true}
	threads := etf.Tuple{etf.Atom("threads"), true}
	threadsPoolSize := etf.Tuple{etf.Atom("threads_pool_size"), 1}
	i := int(1)
	wordsizeInternal := etf.Tuple{etf.Atom("wordsize_internal"), int(unsafe.Sizeof(i))}
	wordsizeExternal := etf.Tuple{etf.Atom("wordsize_external"), int(unsafe.Sizeof(i))}
	tmp := etf.Tuple{etf.Atom("instance"), 0,
		etf.List{
			etf.Tuple{etf.Atom("mbcs"), etf.List{
				etf.Tuple{etf.Atom("blocks_size"), 1, 1, 1},
				etf.Tuple{etf.Atom("carriers_size"), 1, 1, 1},
			}},
			etf.Tuple{etf.Atom("sbcs"), etf.List{
				etf.Tuple{etf.Atom("blocks_size"), 0, 0, 0},
				etf.Tuple{etf.Atom("carriers_size"), 0, 0, 0},
			}},
		}}

	allocInfo := etf.Tuple{etf.Atom("alloc_info"), etf.List{
		etf.Tuple{etf.Atom("temp_alloc"), etf.List{tmp}},
		etf.Tuple{etf.Atom("sl_alloc"), etf.List{tmp}},
		etf.Tuple{etf.Atom("std_alloc"), etf.List{tmp}},
		etf.Tuple{etf.Atom("ll_alloc"), etf.List{tmp}},
		etf.Tuple{etf.Atom("eheap_alloc"), etf.List{tmp}},
		etf.Tuple{etf.Atom("ets_alloc"), etf.List{tmp}},
		etf.Tuple{etf.Atom("fix_alloc"), etf.List{tmp}},
		etf.Tuple{etf.Atom("literal_alloc"), etf.List{tmp}},
		etf.Tuple{etf.Atom("binary_alloc"), etf.List{tmp}},
		etf.Tuple{etf.Atom("driver_alloc"), etf.List{tmp}},
	}}

	// Meminfo = erlang:memory().
	runtime.ReadMemStats(&m)

	total := etf.Tuple{etf.Atom("total"), m.HeapAlloc}
	system := etf.Tuple{etf.Atom("system"), m.HeapAlloc}
	processes := etf.Tuple{etf.Atom("processes"), m.HeapAlloc}
	processesUsed := etf.Tuple{etf.Atom("processes_used"), m.HeapAlloc}
	atom := etf.Tuple{etf.Atom("atom"), 0}
	atomUsed := etf.Tuple{etf.Atom("atom_used"), 0}
	binary := etf.Tuple{etf.Atom("binary"), 0}
	code := etf.Tuple{etf.Atom("code"), 0}
	ets := etf.Tuple{etf.Atom("ets"), 0}

	info := etf.List{
		processCount,
		processLimit,
		atomCount,
		atomLimit,
		etsCount,
		etsLimit,
		portCount,
		portLimit,
		uptime,
		runQueue,
		ioInput,
		ioOutput,
		logicalProcessors,
		logicalProcessorsOnline,
		logicalProcessorsAvailable,
		schedulers,
		schedulersOnline,
		schedulersAvailable,
		otpRelease,
		version,
		systemArchitecture,
		kernelPoll,
		smpSupport,
		threads,
		threadsPoolSize,
		wordsizeInternal,
		wordsizeExternal,
		allocInfo,
		// Meminfo
		total,
		system,
		processes,
		processesUsed,
		atom,
		atomUsed,
		binary,
		code,
		ets,
	}
	return info
}
