package ergonode

// https://github.com/erlang/otp/blob/master/lib/observer/src/observer_procinfo.erl

import (
	"runtime"
	"time"
	"unsafe"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
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

// sys_info() ->
//     MemInfo = try erlang:memory() of
//                   Mem -> Mem
//               catch _:_ -> []
//               end,

//     SchedulersOnline = erlang:system_info(schedulers_online),
//     SchedulersAvailable = case erlang:system_info(multi_scheduling) of
//                               enabled -> SchedulersOnline;
//                               _ -> 1
//                           end,

//     {{_,Input},{_,Output}} = erlang:statistics(io),
//     [{process_count, erlang:system_info(process_count)},
//      {process_limit, erlang:system_info(process_limit)},
//      {uptime, element(1, erlang:statistics(wall_clock))},
//      {run_queue, erlang:statistics(run_queue)},
//      {io_input, Input},
//      {io_output,  Output},

//      {logical_processors, erlang:system_info(logical_processors)},
//      {logical_processors_online, erlang:system_info(logical_processors_online)},
//      {logical_processors_available, erlang:system_info(logical_processors_available)},
//      {schedulers, erlang:system_info(schedulers)},
//      {schedulers_online, SchedulersOnline},
//      {schedulers_available, SchedulersAvailable},

//      {otp_release, erlang:system_info(otp_release)},
//      {version, erlang:system_info(version)},
//      {system_architecture, erlang:system_info(system_architecture)},
//      {kernel_poll, erlang:system_info(kernel_poll)},
//      {smp_support, erlang:system_info(smp_support)},
//      {threads, erlang:system_info(threads)},
//      {thread_pool_size, erlang:system_info(thread_pool_size)},
//      {wordsize_internal, erlang:system_info({wordsize, internal})},
//      {wordsize_external, erlang:system_info({wordsize, external})},
//      {alloc_info, alloc_info()}
//      | MemInfo].

func (o *observerBackend) sysInfo() etf.List {

	processCount := etf.Tuple{etf.Atom("process_count"), 123}
	processLimit := etf.Tuple{etf.Atom("process_limit"), 123}
	ut := int(time.Since(o.process.Node.StartedAt).Seconds())
	uptime := etf.Tuple{etf.Atom("uptime"), ut}
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
	allocInfo := etf.Tuple{etf.Atom("alloc_info"), etf.List{}}

	// meminfo
	// > erlang:memory().
	// [{total,23254256},
	//  {processes,5792512},
	//  {processes_used,5791328},
	//  {system,17461744},
	//  {atom,380433},
	//  {atom_used,349728},
	//  {binary,239800},
	//  {code,7768539},
	//  {ets,846496}]

	runtime.ReadMemStats(&m)

	total := etf.Tuple{etf.Atom("total"), m.TotalAlloc}
	system := etf.Tuple{etf.Atom("system"), m.HeapSys}
	processes := etf.Tuple{etf.Atom("processes"), m.Alloc}
	processesUsed := etf.Tuple{etf.Atom("processes_used"), m.HeapInuse}

	info := etf.List{
		processCount,
		processLimit,
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

		total,
		system,
		processes,
		processesUsed,
	}
	return info
}
