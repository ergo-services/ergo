package ergo

// https://github.com/erlang/otp/blob/master/lib/observer/src/observer_procinfo.erl

import (
	"runtime"
	"time"
	"unsafe"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
)

var m runtime.MemStats

type observerBackend struct {
	GenServer
	process *Process
}

// Init initializes process state using arbitrary arguments
// Init(...) -> state
func (o *observerBackend) Init(p *Process, args ...interface{}) (state interface{}) {
	lib.Log("OBSERVER: Init: %#v", args)
	o.process = p

	funProcLibInitialCall := func(a ...etf.Term) etf.Term {
		return etf.Tuple{etf.Atom("proc_lib"), etf.Atom("init_p"), 5}
	}
	p.Node.ProvideRPC("proc_lib", "translate_initial_call", funProcLibInitialCall)

	funAppmonInfo := func(a ...etf.Term) etf.Term {
		from := a[0] // pid
		am, e := p.Node.Spawn("", ProcessOptions{}, &appMon{}, from)
		if e != nil {
			return etf.Tuple{etf.Atom("error")}
		}
		return etf.Tuple{etf.Atom("ok"), am.Self()}
	}
	p.Node.ProvideRPC("appmon_info", "start_link2", funAppmonInfo)

	return nil
}

// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (o *observerBackend) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("OBSERVER: HandleCast: %#v", message)
	return "noreply", state
}

// HandleCall serves incoming messages sending via gen_server:call
// HandleCall -> ("reply", message, state) - reply
//				 ("noreply", _, state) - noreply
//		         ("stop", reason, _) - normal stop
func (o *observerBackend) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	lib.Log("OBSERVER: HandleCall: %v, From: %#v", message, from)
	function := message.(etf.Tuple).Element(1).(etf.Atom)
	// args := message.(etf.Tuple).Element(2).(etf.List)
	switch function {
	case etf.Atom("sys_info"):
		//etf.Tuple{"call", "observer_backend", "sys_info",
		//           etf.List{}, etf.Pid{Node:"erl-examplenode@127.0.0.1", Id:0x46, Serial:0x0, Creation:0x2}}
		reply := etf.Term(o.sysInfo())
		return "reply", reply, state
	case etf.Atom("get_table_list"):
		// TODO: add here implementation if we decide support ETS tables
		// args should be like:
		// etf.List{"ets", etf.List{etf.Tuple{"sys_hidden", "true"}, etf.Tuple{"unread_hidden", "true"}}}
		reply := etf.Term(etf.List{})
		return "reply", reply, state
	case etf.Atom("get_port_list"):
		reply := etf.Term(etf.List{})
		return "reply", reply, state
	}

	reply := etf.Term("ok")
	return "reply", reply, state
}

// HandleInfo serves all another incoming messages (Pid ! message)
// HandleInfo -> ("noreply", state) - noreply
//		         ("stop", reason) - normal stop
func (o *observerBackend) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("OBSERVER: HandleInfo: %#v", message)
	return "noreply", state
}

// Terminate called when process died
func (o *observerBackend) Terminate(reason string, state interface{}) {
	lib.Log("OBSERVER: Terminate: %#v", reason)
}

func (o *observerBackend) sysInfo() etf.List {
	// observer_backend:sys_info()
	processCount := etf.Tuple{etf.Atom("process_count"), len(o.process.Node.GetProcessList())}
	processLimit := etf.Tuple{etf.Atom("process_limit"), 262144}
	atomCount := etf.Tuple{etf.Atom("atom_count"), 0}
	atomLimit := etf.Tuple{etf.Atom("atom_limit"), 1}
	etsCount := etf.Tuple{etf.Atom("ets_count"), 0}
	etsLimit := etf.Tuple{etf.Atom("ets_limit"), 1}
	portCount := etf.Tuple{etf.Atom("port_count"), 0}
	portLimit := etf.Tuple{etf.Atom("port_limit"), 1}
	ut := time.Now().Unix() - o.process.Node.StartedAt.Unix()
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
	otpRelease := etf.Tuple{etf.Atom("otp_release"), o.process.Node.VersionOTP()}
	version := etf.Tuple{etf.Atom("version"), etf.Atom(o.process.Node.VersionERTS())}
	systemArchitecture := etf.Tuple{etf.Atom("system_architecture"), etf.Atom(runtime.GOARCH)}
	kernelPoll := etf.Tuple{etf.Atom("kernel_poll"), true}
	smpSupport := etf.Tuple{etf.Atom("smp_support"), true}
	threads := etf.Tuple{etf.Atom("threads"), true}
	threadsPoolSize := etf.Tuple{etf.Atom("threads_pool_size"), 1}
	i := int(1)
	wordsizeInternal := etf.Tuple{etf.Atom("wordsize_internal"), unsafe.Sizeof(i)}
	wordsizeExternal := etf.Tuple{etf.Atom("wordsize_external"), unsafe.Sizeof(i)}
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
