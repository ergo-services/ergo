package node

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"ergo.services/ergo/app/system"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/lib/osdep"
	"ergo.services/ergo/net/edf"
)

var (
	startID     = uint64(1000)
	startUniqID = uint64(time.Now().UnixNano())
)

type node struct {
	name      gen.Atom
	version   gen.Version
	framework gen.Version

	creation int64

	env sync.Map // env name gen.Env -> any

	security    gen.SecurityOptions
	certmanager gen.CertManager

	corePID gen.PID
	nextID  uint64
	uniqID  uint64

	processes sync.Map // process pid gen.PID -> *process
	names     sync.Map // process name gen.Atom -> *process
	aliases   sync.Map // process alias gen.Alias -> *process
	events    sync.Map // process event gen.Event -> *eventOwner

	applications sync.Map // application name -> *application

	// consumer lists (subcribers)
	monitors *target
	links    *target

	network *network

	cron *cron

	loggers map[gen.LogLevel]*sync.Map // level -> name -> gen.LoggerBehavior
	log     *log

	waitprocesses sync.WaitGroup
	wait          chan struct{}

	licenses sync.Map

	coreEventsToken gen.Ref

	ctrlc chan os.Signal
}

type eventOwner struct {
	name      gen.Atom
	producer  gen.PID
	token     gen.Ref
	notify    bool
	consumers int32

	last lib.QueueMPSC
}

func Start(name gen.Atom, options gen.NodeOptions, frameworkVersion gen.Version) (gen.Node, error) {
	if len(name) > 255 {
		return nil, gen.ErrAtomTooLong
	}

	if s := strings.Split(string(name), "@"); len(s) != 2 {
		return nil, fmt.Errorf("incorrect FQDN node name (example: node@localhost)")
	} else {
		if len(s[0]) < 1 {
			return nil, fmt.Errorf("too short node name")
		}
		if len(s[1]) < 1 {
			return nil, fmt.Errorf("too short host name")
		}
	}

	creation := time.Now().Unix()

	node := &node{
		name:      name,
		version:   options.Version,
		framework: frameworkVersion,
		creation:  creation,

		corePID: gen.PID{Node: name, ID: 1, Creation: creation},
		nextID:  startID,
		uniqID:  startUniqID,

		certmanager: options.CertManager,
		security:    options.Security,

		monitors: createTarget(),
		links:    createTarget(),

		loggers: make(map[gen.LogLevel]*sync.Map),

		wait: make(chan struct{}),
	}

	node.log = createLog(options.Log.Level, node.dolog)
	node.log.setSource(gen.MessageLogNode{Node: name, Creation: creation})

	if options.Log.Level == gen.LogLevelDefault {
		node.log.SetLevel(gen.LogLevelInfo)
	}

	node.loggers[gen.LogLevelSystem] = &sync.Map{}
	node.loggers[gen.LogLevelTrace] = &sync.Map{}
	node.loggers[gen.LogLevelDebug] = &sync.Map{}
	node.loggers[gen.LogLevelInfo] = &sync.Map{}
	node.loggers[gen.LogLevelWarning] = &sync.Map{}
	node.loggers[gen.LogLevelError] = &sync.Map{}
	node.loggers[gen.LogLevelPanic] = &sync.Map{}

	for k, v := range options.Env {
		node.SetEnv(k, v)
	}

	if options.Log.DefaultLogger.Disable == false {
		// add default logger
		logger := gen.CreateDefaultLogger(options.Log.DefaultLogger)
		node.LoggerAdd("default", logger, options.Log.DefaultLogger.Filter...)
	}

	for _, lo := range options.Log.Loggers {
		if len(lo.Name) == 0 {
			return nil, errors.New("logger name can not be empty")
		}
		if lo.Logger == nil {
			return nil, errors.New("logger can not be nil")
		}
		node.LoggerAdd(lo.Name, lo.Logger, lo.Filter...)
	}

	node.coreEventsToken, _ = node.RegisterEvent(gen.CoreEvent, gen.EventOptions{})

	node.validateLicenses(node.version)
	node.network = createNetwork(node)

	if err := node.NetworkStart(options.Network); err != nil {
		return nil, err
	}

	if len(options.Applications) > 0 {
		node.log.Trace("starting application(s)...")
		for _, app := range options.Applications {
			// load applications
			name, err := node.ApplicationLoad(app)
			if err != nil {
				node.log.Error("unable to load application %s: %s ", name, err)
				node.StopForce()
				return nil, err
			}
			// start applications
			if err := node.ApplicationStart(name, gen.ApplicationOptions{}); err != nil {
				node.log.Error("unable to start application %s:%s", name, err)
				node.StopForce()
				return nil, err
			}
		}
	}

	edf.RegisterAtom(name)
	node.log.Info("node %s built with %q successfully started", node.name, node.framework)
	node.cron = createCron(node)
	for _, job := range options.Cron.Jobs {
		if err := node.cron.AddJob(job); err != nil {
			node.StopForce()
			return nil, err
		}
	}
	return node, nil
}

//
// gen.Node interface implementation
//

func (n *node) Name() gen.Atom {
	return n.name
}

func (n *node) IsAlive() bool {
	return n.isRunning()
}

func (n *node) Uptime() int64 {
	if n.isRunning() == false {
		return 0
	}
	return time.Now().Unix() - atomic.LoadInt64(&n.creation)
}

func (n *node) Version() gen.Version {
	return n.version
}

func (n *node) FrameworkVersion() gen.Version {
	return n.framework
}

func (n *node) Commercial() []gen.Version {
	var commercial []gen.Version
	n.licenses.Range(func(k, _ any) bool {
		commercial = append(commercial, k.(gen.Version))
		return true
	})
	return commercial
}

func (n *node) EnvList() map[gen.Env]any {
	if n.isRunning() == false {
		return nil
	}
	env := make(map[gen.Env]any)
	n.env.Range(func(k, v any) bool {
		env[gen.Env(k.(string))] = v
		return true
	})
	return env
}

func (n *node) SetEnv(name gen.Env, value any) {
	if n.isRunning() == false {
		return
	}
	if value == nil {
		n.env.Delete(name.String())
		return
	}
	n.env.Store(name.String(), value)
}

func (n *node) Env(name gen.Env) (any, bool) {
	if n.isRunning() == false {
		return nil, false
	}

	return n.env.Load(name.String())
}

func (n *node) CertManager() gen.CertManager {
	return n.certmanager
}

func (n *node) Security() gen.SecurityOptions {
	return n.security
}

func (n *node) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	if n.isRunning() == false {
		return gen.PID{}, gen.ErrNodeTerminated
	}
	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		Args:           args,
		ParentPID:      n.corePID,
		ParentLeader:   n.corePID,
		ParentLogLevel: n.log.level,
		ParentEnv:      n.EnvList(),
	}

	return n.spawn(factory, opts)
}

func (n *node) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	if n.isRunning() == false {
		return gen.PID{}, gen.ErrNodeTerminated
	}
	if len(register) > 255 {
		return gen.PID{}, gen.ErrAtomTooLong
	}
	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		Register:       register,
		Args:           args,
		ParentPID:      n.corePID,
		ParentLeader:   n.corePID,
		ParentLogLevel: n.log.level,
		ParentEnv:      n.EnvList(),
	}
	return n.spawn(factory, opts)

}

func (n *node) RegisterName(name gen.Atom, pid gen.PID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	if len(name) > 255 {
		return gen.ErrAtomTooLong
	}

	n.log.Trace("RegisterName %s to %s", name, pid)

	value, ok := n.processes.Load(pid)
	if ok == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)

	if p.isAlive() == false {
		return gen.ErrProcessTerminated
	}

	if p.registered.CompareAndSwap(false, true) == false {
		return gen.ErrTaken
	}

	if _, exist := n.names.LoadOrStore(name, p); exist {
		p.registered.Store(false)
		return gen.ErrTaken
	}

	p.name = name

	return nil
}

func (n *node) UnregisterName(name gen.Atom) (gen.PID, error) {
	if n.isRunning() == false {
		return gen.PID{}, gen.ErrNodeTerminated
	}

	value, exist := n.names.LoadAndDelete(name)
	if exist == false {
		return gen.PID{}, gen.ErrNameUnknown
	}
	p := value.(*process)
	p.name = ""
	p.registered.Store(false)

	n.log.Trace("UnregisterName %s belonged to %s", name, p.pid)

	pname := gen.ProcessID{Name: name, Node: n.name}
	n.RouteTerminateProcessID(pname, gen.ErrUnregistered)
	return p.pid, nil
}

func (n *node) MetaInfo(m gen.Alias) (gen.MetaInfo, error) {
	var info gen.MetaInfo
	if n.isRunning() == false {
		return info, gen.ErrNodeTerminated
	}

	value, ok := n.aliases.Load(m)
	if ok == false {
		return info, gen.ErrProcessUnknown
	}
	p := value.(*process)

	value, ok = p.metas.Load(m)
	if ok == false {
		return info, gen.ErrMetaUnknown
	}
	mp := value.(*meta)

	info.ID = mp.id
	info.Parent = p.pid
	info.Application = p.application
	info.Behavior = mp.sbehavior
	info.MailboxSize = mp.main.Size()
	info.MailboxQueues.Main = mp.main.Len()
	info.MailboxQueues.System = mp.system.Len()
	info.MessagesIn = mp.messagesIn
	info.MessagesOut = mp.messagesOut
	info.MessagePriority = mp.priority
	info.Uptime = time.Now().Unix() - mp.creation
	info.LogLevel = mp.log.Level()
	info.State = gen.MetaState(mp.state)
	return info, nil
}

func (n *node) ProcessInfo(pid gen.PID) (gen.ProcessInfo, error) {
	var info gen.ProcessInfo

	if n.isRunning() == false {
		return info, gen.ErrNodeTerminated
	}

	value, ok := n.processes.Load(pid)
	if ok == false {
		return info, gen.ErrProcessUnknown
	}
	p := value.(*process)

	info.PID = p.pid
	info.Name = p.name
	info.Application = p.application
	info.Behavior = p.sbehavior
	info.MailboxSize = p.mailbox.Main.Size()
	info.MailboxQueues.Main = p.mailbox.Main.Len()
	info.MailboxQueues.Urgent = p.mailbox.Urgent.Len()
	info.MailboxQueues.System = p.mailbox.System.Len()
	info.MailboxQueues.Log = p.mailbox.Log.Len()
	info.MessagesIn = atomic.LoadUint64(&p.messagesIn)
	info.MessagesOut = atomic.LoadUint64(&p.messagesOut)
	info.RunningTime = atomic.LoadUint64(&p.runningTime)
	info.Compression = p.compression
	info.MessagePriority = p.priority
	info.Uptime = p.Uptime()
	info.State = p.State()
	info.Parent = p.parent
	info.Leader = p.leader
	info.Fallback = p.fallback
	info.Aliases = p.Aliases()
	info.Events = p.Events()
	info.LogLevel = p.log.Level()
	info.KeepNetworkOrder = p.keeporder
	info.ImportantDelivery = p.important

	if n.security.ExposeEnvInfo {
		info.Env = p.EnvList()
	} else {
		info.Env = make(map[gen.Env]any)
	}

	// initialized slices make json marshaler treat them as an empty list
	// (not a nil value)
	info.Metas = []gen.Alias{}
	info.LinksPID = []gen.PID{}
	info.MonitorsPID = []gen.PID{}
	info.LinksProcessID = []gen.ProcessID{}
	info.MonitorsProcessID = []gen.ProcessID{}
	info.LinksAlias = []gen.Alias{}
	info.MonitorsAlias = []gen.Alias{}
	info.LinksEvent = []gen.Event{}
	info.MonitorsEvent = []gen.Event{}
	info.LinksNode = []gen.Atom{}
	info.MonitorsNode = []gen.Atom{}

	p.metas.Range(func(k, _ any) bool {
		meta := k.(gen.Alias)
		info.Metas = append(info.Metas, meta)
		return true
	})

	p.targets.Range(func(k, v any) bool {
		is_link := v.(bool)
		switch m := k.(type) {
		case gen.PID:
			if is_link {
				info.LinksPID = append(info.LinksPID, m)
				break
			}
			info.MonitorsPID = append(info.MonitorsPID, m)
		case gen.ProcessID:
			if is_link {
				info.LinksProcessID = append(info.LinksProcessID, m)
				break
			}
			info.MonitorsProcessID = append(info.MonitorsProcessID, m)
		case gen.Alias:
			if is_link {
				info.LinksAlias = append(info.LinksAlias, m)
				break
			}
			info.MonitorsAlias = append(info.MonitorsAlias, m)
		case gen.Event:
			if is_link {
				info.LinksEvent = append(info.LinksEvent, m)
				break
			}
			info.MonitorsEvent = append(info.MonitorsEvent, m)
		case gen.Atom:
			if is_link {
				info.LinksNode = append(info.LinksNode, m)
				break
			}
			info.MonitorsNode = append(info.MonitorsNode, m)
		}
		return true
	})

	return info, nil
}

func (n *node) SetLogLevelProcess(pid gen.PID, level gen.LogLevel) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return gen.ErrProcessUnknown
	}

	p := value.(*process)
	return p.log.SetLevel(level)
}

func (n *node) LogLevelProcess(pid gen.PID) (gen.LogLevel, error) {
	var level gen.LogLevel
	if n.isRunning() == false {
		return level, gen.ErrNodeTerminated
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return level, gen.ErrProcessUnknown
	}

	p := value.(*process)
	level = p.log.Level()
	return level, nil
}

func (n *node) SetLogLevelMeta(m gen.Alias, level gen.LogLevel) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	value, loaded := n.aliases.Load(m)
	if loaded == false {
		return gen.ErrProcessUnknown
	}

	p := value.(*process)

	value, loaded = p.metas.Load(m)
	if loaded == false {
		return gen.ErrMetaUnknown
	}
	mp := value.(*meta)

	return mp.log.SetLevel(level)
}

func (n *node) LogLevelMeta(m gen.Alias) (gen.LogLevel, error) {
	var level gen.LogLevel
	if n.isRunning() == false {
		return level, gen.ErrNodeTerminated
	}
	value, loaded := n.aliases.Load(m)
	if loaded == false {
		return level, gen.ErrProcessUnknown
	}

	p := value.(*process)

	value, loaded = p.metas.Load(m)
	if loaded == false {
		return level, gen.ErrMetaUnknown
	}
	mp := value.(*meta)
	level = mp.log.Level()

	return level, nil
}

func (n *node) Info() (gen.NodeInfo, error) {
	var info gen.NodeInfo
	if n.isRunning() == false {
		return info, gen.ErrNodeTerminated
	}

	info.Name = n.name
	info.Uptime = n.Uptime()
	info.Version = n.version
	info.Framework = n.framework
	info.Commercial = n.Commercial()
	info.LogLevel = n.log.Level()
	info.Cron = n.cron.Info()

	mli := make(map[string]int)
	for _, level := range gen.DefaultLogLevels {
		loggers := n.loggers[level]
		loggers.Range(func(k, v any) bool {
			loggername := k.(string)
			n, found := mli[loggername]
			if found == false {
				loggerbehavior := strings.TrimPrefix(reflect.TypeOf(v).String(), "*")
				li := gen.LoggerInfo{
					Name:     loggername,
					Behavior: loggerbehavior,
				}
				info.Loggers = append(info.Loggers, li)
				n = len(info.Loggers) - 1
				mli[loggername] = n
			}
			info.Loggers[n].Levels = append(info.Loggers[n].Levels, level)
			return true
		})
	}

	if n.security.ExposeEnvInfo {
		info.Env = n.EnvList()
	} else {
		info.Env = make(map[gen.Env]any)
	}

	n.processes.Range(func(_, v any) bool {
		info.ProcessesTotal++
		p := v.(*process)
		switch p.State() {
		case gen.ProcessStateRunning:
			info.ProcessesRunning++
		case gen.ProcessStateWaitResponse:
			info.ProcessesRunning++
		case gen.ProcessStateZombee:
			info.ProcessesZombee++
		}
		return true
	})

	n.names.Range(func(_, _ any) bool {
		info.RegisteredNames++
		return true
	})
	n.aliases.Range(func(_, _ any) bool {
		info.RegisteredAliases++
		return true
	})
	n.events.Range(func(_, _ any) bool {
		info.RegisteredEvents++
		return true
	})

	info.ApplicationsTotal = int64(len(n.Applications()))
	info.ApplicationsRunning = int64(len(n.ApplicationsRunning()))

	var mstat runtime.MemStats
	runtime.ReadMemStats(&mstat)
	info.MemoryUsed = mstat.Sys
	info.MemoryAlloc = mstat.Alloc

	utime, stime := osdep.ResourceUsage()
	info.UserTime = utime
	info.SystemTime = stime

	return info, nil
}

func (n *node) ProcessList() ([]gen.PID, error) {
	var pl []gen.PID

	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	n.processes.Range(func(k, _ any) bool {
		pl = append(pl, k.(gen.PID))
		return true
	})

	return pl, nil
}

func (n *node) ProcessListShortInfo(start, limit int) ([]gen.ProcessShortInfo, error) {
	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	if start < 1000 || limit < 0 {
		return nil, gen.ErrIncorrect
	}
	if limit == 0 {
		limit = 100
	}
	ustart := uint64(start)
	psi := []gen.ProcessShortInfo{}
	pid := n.corePID

	for limit > 0 {

		if ustart > n.nextID {
			break
		}

		pid.ID = ustart
		ustart++
		v, found := n.processes.Load(pid)
		if found == false {
			continue
		}
		process := v.(*process)
		messagesMailbox := process.mailbox.Main.Len() +
			process.mailbox.System.Len() +
			process.mailbox.Urgent.Len() +
			process.mailbox.Log.Len()

		info := gen.ProcessShortInfo{
			PID:             process.pid,
			Name:            process.name,
			Application:     process.application,
			Behavior:        process.sbehavior,
			MessagesIn:      process.messagesIn,
			MessagesOut:     process.messagesOut,
			MessagesMailbox: uint64(messagesMailbox),
			RunningTime:     process.runningTime,
			Uptime:          process.Uptime(),
			State:           process.State(),
			Parent:          process.parent,
			Leader:          process.leader,
			LogLevel:        process.log.Level(),
		}
		psi = append(psi, info)
		limit--
	}

	return psi, nil

}

func (n *node) NetworkStart(options gen.NetworkOptions) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	return n.network.start(options)
}

func (n *node) NetworkStop() error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	return n.network.stop()
}

func (n *node) Network() gen.Network {
	if n.isRunning() == false {
		return nil
	}
	return n.network
}

func (n *node) Cron() gen.Cron {
	if n.isRunning() == false {
		return nil
	}
	return n.cron
}

func (n *node) Stop() {
	n.stop(false)
}

func (n *node) StopForce() {
	n.stop(true)
}

func (n *node) stop(force bool) {
	if n.isRunning() == false {
		// already stopped
		return
	}

	if force == false {
		n.applications.Range(func(_, v any) bool {
			app := v.(*application)
			if app.spec.Name == system.Name {
				// skip system app
				return true
			}
			app.stop(false, 5*time.Second)
			return true
		})
	}

	n.processes.Range(func(_, v any) bool {
		p := v.(*process)

		if force {
			n.Kill(p.pid)
			return true
		}

		if p.application != "" {
			// Do nothing if it belons to the app.
			// It has to be terminated via app.stop
			return true
		}

		// we should send an exit-signal using parent pid of the process,
		// so it wont be trapped
		n.RouteSendExit(p.parent, p.pid, gen.TerminateReasonShutdown)
		return true
	})

	if n.cron != nil {
		n.cron.terminate()
	}

	if force == false {
		n.waitprocesses.Wait()
	}

	n.NetworkStop()
	atomic.StoreInt64(&n.creation, 0)
	n.log.Info("node %s stopped", n.name)

	// call terminate loggers
	loggers := make(map[string]gen.LoggerBehavior)
	for _, l := range n.loggers {
		l.Range(func(k, v any) bool {
			name := k.(string)
			logger := v.(gen.LoggerBehavior)
			loggers[name] = logger
			return true
		})
	}
	for _, logger := range loggers {
		logger.Terminate()
	}

	close(n.wait)
}

func (n *node) Wait() {
	// if the node is terminated this channel is already closed so it returns immediately
	<-n.wait
}

func (n *node) WaitWithTimeout(timeout time.Duration) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		return gen.ErrTimeout
	case <-n.wait:
		return nil
	}
}

func (n *node) Send(to any, message any) error {
	return n.SendWithPriority(to, message, gen.MessagePriorityNormal)
}

func (n *node) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	options := gen.MessageOptions{
		Priority: priority,
	}

	switch t := to.(type) {
	case gen.Atom:
		return n.RouteSendProcessID(n.corePID, gen.ProcessID{Name: t, Node: n.name}, options, message)
	case gen.PID:
		return n.RouteSendPID(n.corePID, t, options, message)
	case gen.ProcessID:
		return n.RouteSendProcessID(n.corePID, t, options, message)
	case gen.Alias:
		return n.RouteSendAlias(n.corePID, t, options, message)
	}

	return gen.ErrUnsupported
}

func (n *node) SendEvent(name gen.Atom, token gen.Ref, options gen.MessageOptions, message any) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	n.log.Trace("node.SendEvent %s with token %s", name, token)

	em := gen.MessageEvent{
		Event:     gen.Event{Name: name, Node: n.name},
		Timestamp: time.Now().UnixNano(),
		Message:   message,
	}

	return n.RouteSendEvent(n.corePID, token, options, em)
}

func (n *node) RegisterEvent(name gen.Atom, options gen.EventOptions) (gen.Ref, error) {
	var empty gen.Ref
	if n.isRunning() == false {
		return empty, gen.ErrNodeTerminated
	}

	n.log.Trace("node.RegisterEvent %s", name)

	return n.registerEvent(name, n.corePID, options)
}

func (n *node) UnregisterEvent(name gen.Atom) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	n.log.Trace("node.UnregisterEvent %s", name)
	return n.unregisterEvent(name, n.corePID)
}

func (n *node) SendExit(pid gen.PID, reason error) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	return n.RouteSendExit(n.corePID, pid, reason)
}

func (n *node) Kill(pid gen.PID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return gen.ErrProcessUnknown
	}

	p := value.(*process)
	state := atomic.SwapInt32(&p.state, int32(gen.ProcessStateZombee))
	switch state {
	case int32(gen.ProcessStateWaitResponse), int32(gen.ProcessStateRunning):
		// do not unregister process until its goroutine stopped
		return nil
	case int32(gen.ProcessStateTerminated):
		atomic.StoreInt32(&p.state, int32(gen.ProcessStateTerminated))
		return nil
	}

	old := atomic.SwapInt32(&p.state, int32(gen.ProcessStateTerminated))
	if old == int32(gen.ProcessStateTerminated) {
		return nil
	}
	// unregister process and stuff belonging to it
	n.unregisterProcess(p, gen.TerminateReasonKill)

	go func() {
		if lib.Recover() {
			defer func() {
				if rcv := recover(); rcv != nil {
					pc, fn, line, _ := runtime.Caller(2)
					p.log.Panic("panic in ProcessTerminate - %s[%s] %#v at %s[%s:%d]",
						p.pid, p.name, rcv, runtime.FuncForPC(pc).Name(), fn, line)
				}
			}()
		}
		p.behavior.ProcessTerminate(gen.TerminateReasonKill)
	}()

	return nil
}

func (n *node) ProcessState(pid gen.PID) (gen.ProcessState, error) {
	if n.isRunning() == false {
		return 0, gen.ErrNodeTerminated
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return 0, gen.ErrProcessUnknown
	}
	p := value.(*process)
	return p.State(), nil
}

func (n *node) ApplicationLoad(app gen.ApplicationBehavior, args ...any) (name gen.Atom, r error) {
	if lib.Recover() {
		defer func() {
			if rcv := recover(); rcv != nil {
				pc, fn, line, _ := runtime.Caller(2)
				n.log.Panic("panic in ApplicationLoad - %#v at %s[%s:%d]",
					rcv, runtime.FuncForPC(pc).Name(), fn, line)
				r = gen.ErrApplicationLoadPanic
			}
		}()
	}

	spec, err := app.Load(n, args...)
	if err != nil {
		return name, err
	}

	if len(spec.Group) == 0 {
		return name, gen.ErrApplicationEmpty
	}

	if len(spec.Name) == 0 {
		return name, gen.ErrApplicationName
	}

	if spec.Depends.Network {
		// TODO make it right
		if n.network == nil {
			return name, gen.ErrApplicationDepends
		}
	}

	if spec.Mode == 0 {
		spec.Mode = gen.ApplicationModeTemporary
	}

	if spec.Depends.Applications == nil {
		spec.Depends.Applications = []gen.Atom{}
	}

	env := n.EnvList()
	for k, v := range spec.Env {
		env[k] = v
	}
	spec.Env = env

	if spec.LogLevel == gen.LogLevelDefault {
		spec.LogLevel = n.log.Level()
	}

	a := &application{
		spec:     spec,
		node:     n,
		behavior: app,
		state:    int32(gen.ApplicationStateLoaded),
		mode:     spec.Mode,
	}
	if _, exist := n.applications.LoadOrStore(spec.Name, a); exist {
		return spec.Name, gen.ErrTaken
	}

	a.registerAppRoute()

	return spec.Name, nil
}

func (n *node) ApplicationInfo(name gen.Atom) (gen.ApplicationInfo, error) {
	var info gen.ApplicationInfo
	v, exist := n.applications.Load(name)
	if exist == false {
		return info, gen.ErrApplicationUnknown
	}
	app := v.(*application)
	info = app.info()
	return info, nil
}

func (n *node) ApplicationUnload(name gen.Atom) error {
	v, exist := n.applications.Load(name)
	if exist == false {
		return gen.ErrApplicationUnknown
	}

	app := v.(*application)
	if unloaded := app.tryUnload(); unloaded == false {
		return gen.ErrApplicationRunning
	}
	n.applications.Delete(name)
	app.unregisterAppRoute()
	return nil
}

func (n *node) ApplicationStart(name gen.Atom, options gen.ApplicationOptions) error {
	v, exist := n.applications.Load(name)
	if exist == false {
		return gen.ErrApplicationUnknown
	}
	app := v.(*application)

	// check dependency on the other applications
	for _, dep := range app.spec.Depends.Applications {
		if err := n.ApplicationStart(dep, options); err != nil {
			if err == gen.ErrApplicationUnknown {
				n.log.Error("unable to start %s: unknown dependent application %s", name, dep)
				return gen.ErrApplicationDepends
			}

			if err != gen.ErrApplicationRunning {
				n.log.Error("unable to start %s: start dependent application %s failed: %s", dep, err)
				return gen.ErrApplicationDepends
			}
		}
	}

	opts := gen.ApplicationOptionsExtra{
		ApplicationOptions: options,
		CorePID:            n.corePID,
		CoreEnv:            n.EnvList(),
		CoreLogLevel:       n.log.Level(),
	}
	return app.start(app.spec.Mode, opts)
}

func (n *node) ApplicationStartPermanent(name gen.Atom, options gen.ApplicationOptions) error {
	v, exist := n.applications.Load(name)
	if exist == false {
		return gen.ErrApplicationUnknown
	}
	app := v.(*application)
	opts := gen.ApplicationOptionsExtra{
		ApplicationOptions: options,
		CorePID:            n.corePID,
		CoreEnv:            n.EnvList(),
		CoreLogLevel:       n.log.Level(),
	}
	return app.start(gen.ApplicationModePermanent, opts)
}

func (n *node) ApplicationStartTransient(name gen.Atom, options gen.ApplicationOptions) error {
	v, exist := n.applications.Load(name)
	if exist == false {
		return gen.ErrApplicationUnknown
	}
	app := v.(*application)
	opts := gen.ApplicationOptionsExtra{
		ApplicationOptions: options,
		CorePID:            n.corePID,
		CoreEnv:            n.EnvList(),
		CoreLogLevel:       n.log.Level(),
	}
	return app.start(gen.ApplicationModeTransient, opts)
}

func (n *node) ApplicationStartTemporary(name gen.Atom, options gen.ApplicationOptions) error {
	v, exist := n.applications.Load(name)
	if exist == false {
		return gen.ErrApplicationUnknown
	}
	app := v.(*application)
	opts := gen.ApplicationOptionsExtra{
		ApplicationOptions: options,
		CorePID:            n.corePID,
		CoreEnv:            n.EnvList(),
		CoreLogLevel:       n.log.Level(),
	}
	return app.start(gen.ApplicationModeTemporary, opts)
}

func (n *node) ApplicationStop(name gen.Atom) error {
	v, exist := n.applications.Load(name)
	if exist == false {
		return gen.ErrApplicationUnknown
	}

	// system app can not be stopped
	if name == system.Name {
		return gen.ErrNotAllowed
	}

	app := v.(*application)
	return app.stop(false, 5*time.Second)
}

func (n *node) ApplicationStopForce(name gen.Atom) error {
	v, exist := n.applications.Load(name)
	if exist == false {
		return gen.ErrApplicationUnknown
	}

	// system app can not be stopped
	if name == system.Name {
		return gen.ErrNotAllowed
	}

	app := v.(*application)
	return app.stop(true, 0)
}

func (n *node) ApplicationStopWithTimeout(name gen.Atom, timeout time.Duration) error {
	v, exist := n.applications.Load(name)
	if exist == false {
		return gen.ErrApplicationUnknown
	}
	app := v.(*application)
	return app.stop(false, timeout)
}

func (n *node) Applications() []gen.Atom {
	apps := []gen.Atom{}
	n.applications.Range(func(_, v any) bool {
		app := v.(*application)
		apps = append(apps, app.spec.Name)
		return true
	})
	return apps
}

func (n *node) ApplicationsRunning() []gen.Atom {
	apps := []gen.Atom{}
	n.applications.Range(func(_, v any) bool {
		app := v.(*application)
		if app.isRunning() {
			apps = append(apps, app.spec.Name)
		}
		return true
	})
	return apps
}

func (n *node) Log() gen.Log {
	return n.log
}

func (n *node) LoggerAddPID(pid gen.PID, name string, filter ...gen.LogLevel) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return gen.ErrProcessUnknown
	}

	if name == "" {
		return gen.ErrIncorrect
	}

	p := value.(*process)

	if p.loggername != "" {
		// already registered as a logger
		return gen.ErrNotAllowed
	}

	logger := createProcessLogger(p.mailbox.Log, p.run)
	if err := n.LoggerAdd(name, logger, filter...); err == nil {
		p.loggername = name
		p.log.SetLevel(gen.LogLevelDisabled)
	} else {
		return err
	}

	if lib.Trace() {
		n.log.Trace("node.LoggerAddPID added new process logger %s with name %q", pid, name)
	}
	return nil
}

func (n *node) LoggerAdd(name string, logger gen.LoggerBehavior, filter ...gen.LogLevel) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	if logger == nil {
		return gen.ErrIncorrect
	}

	if filter == nil {
		filter = gen.DefaultLogFilter
	}

	for _, l := range n.loggers {
		if _, exist := l.Load(name); exist {
			return gen.ErrTaken
		}
	}

	for _, level := range filter {
		if l := n.loggers[level]; l != nil {
			l.Store(name, logger)
		}
	}

	if lib.Trace() {
		n.log.Trace("node.LoggerAdd added new logger with name %q", name)
	}
	return nil
}

func (n *node) LoggerDeletePID(pid gen.PID) {
	if n.isRunning() == false {
		return
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return
	}

	p := value.(*process)
	if p.loggername != "" {
		n.LoggerDelete(p.loggername)
		p.loggername = ""
		// TODO we should restore previous log level
		p.log.SetLevel(gen.LogLevelInfo)
		n.log.Trace("node.LoggerDeletePID removed process logger %s with name %q", pid, p.loggername)
	}
	return
}

func (n *node) LoggerDelete(name string) {
	var logger gen.LoggerBehavior

	if n.isRunning() == false {
		return
	}

	for _, l := range n.loggers {
		if v, exist := l.LoadAndDelete(name); exist {
			logger = v.(gen.LoggerBehavior)
		}
	}
	// call terminate
	if logger != nil {
		logger.Terminate()
	}
	n.log.Trace("node.LoggerDelete removed logger with name %q", name)
}

func (n *node) LoggerLevels(name string) []gen.LogLevel {
	var levels []gen.LogLevel
	for level, l := range n.loggers {
		if _, exist := l.Load(name); exist {
			levels = append(levels, level)
		}
	}
	return levels
}

func (n *node) Loggers() []string {
	m := make(map[string]bool)
	for _, l := range n.loggers {
		l.Range(func(k, _ any) bool {
			name := k.(string)
			m[name] = true
			return true
		})
	}
	loggers := []string{}
	for k := range m {
		loggers = append(loggers, k)
	}
	return loggers
}

func (n *node) dolog(message gen.MessageLog, loggername string) {
	if n.isRunning() == false {
		return
	}
	if l := n.loggers[message.Level]; l != nil {
		l.Range(func(k, v any) bool {
			if loggername == "" {
				logger := v.(gen.LoggerBehavior)
				logger.Log(message)
				return true
			}
			if loggername == k.(string) {
				logger := v.(gen.LoggerBehavior)
				logger.Log(message)
			}
			return true
		})
	}
}

func (n *node) SetCTRLC(enable bool) {
	if enable == true && n.ctrlc != nil {
		// already set up
		return
	}

	if enable == false && n.ctrlc != nil {
		close(n.ctrlc)
		n.ctrlc = nil
		n.Log().Info("(CRTL+C) disabled for %s", n.name)
		return
	}

	go func() {
		n.ctrlc = make(chan os.Signal)
		signal.Notify(n.ctrlc, os.Interrupt, syscall.SIGTERM)
		n.Log().Info("(CRTL+C) enabled for %s", n.name)

		n.Log().Info("         press Ctrl+C to enable/disable debug logging level for %s", n.name)
		n.Log().Info("         press Ctrl+C twice to stop %s gracefully", n.name)
		ctrlcTime := time.Now().Unix()
		level := n.Log().Level()
		debug := false
		for {
			sig := <-n.ctrlc
			if sig == nil {
				// closed channel. disable ctrlc
				signal.Reset()

				return
			}

			now := time.Now().Unix()
			if now-ctrlcTime == 0 {
				signal.Reset()
				n.Log().Info("(CRTL+C) stopping %s (graceful shutdown)...", n.name)
				n.Stop()
				return
			}

			ctrlcTime = now

			if debug {
				n.Log().Info("(CRTL+C) disabling debug level for %s", n.name)
				n.Log().SetLevel(level)
				debug = false
				continue
			}

			n.Log().Info("(CRTL+C) enabling debug level for %s", n.name)
			level = n.Log().Level()
			n.Log().SetLevel(gen.LogLevelDebug)
			debug = true
		}
	}()
}

//
// private
//

func (n *node) spawn(factory gen.ProcessFactory, options gen.ProcessOptionsExtra) (gen.PID, error) {
	var empty gen.PID

	if n.isRunning() == false {
		return empty, gen.ErrNodeTerminated
	}

	if factory == nil {
		return empty, gen.ErrIncorrect
	}

	if options.ParentPID == empty || options.ParentLeader == empty {
		return empty, gen.ErrParentUnknown
	}

	p := &process{
		node:        n,
		response:    make(chan response, 10),
		creation:    time.Now().Unix(),
		keeporder:   true,
		state:       int32(gen.ProcessStateInit),
		parent:      options.ParentPID,
		leader:      options.ParentLeader,
		application: options.Application,
		important:   options.ImportantDelivery,
	}

	if options.Register != "" {
		if _, exist := n.names.LoadOrStore(options.Register, p); exist {
			return p.pid, gen.ErrTaken
		}
		p.name = options.Register
		p.registered.Store(true)
	}

	// init mailbox
	if options.MailboxSize > 0 {
		p.fallback = options.Fallback
		p.mailbox.Main = lib.NewQueueLimitMPSC(options.MailboxSize, false)
		p.mailbox.System = lib.NewQueueLimitMPSC(options.MailboxSize, false)
		p.mailbox.Urgent = lib.NewQueueLimitMPSC(options.MailboxSize, false)
		p.mailbox.Log = lib.NewQueueLimitMPSC(options.MailboxSize, false)
	} else {
		p.mailbox.Main = lib.NewQueueMPSC()
		p.mailbox.System = lib.NewQueueMPSC()
		p.mailbox.Urgent = lib.NewQueueMPSC()
		p.mailbox.Log = lib.NewQueueMPSC()
	}

	// create pid
	pid := gen.PID{
		Node:     n.name,
		ID:       atomic.AddUint64(&n.nextID, 1),
		Creation: n.creation,
	}
	p.pid = pid

	for k, v := range options.ParentEnv {
		p.SetEnv(k, v)
	}
	if lib.Trace() {
		n.log.Trace("...spawn new process %s (parent %s, %s) using %#v", p.pid, p.parent, p.name, factory)
	}

	for k, v := range options.Env {
		p.SetEnv(k, v)
	}

	if options.Leader != empty {
		p.leader = options.Leader
	}

	p.compression = options.Compression
	if p.compression.Level == 0 {
		p.compression.Level = gen.DefaultCompressionLevel
	}
	if p.compression.Type == "" {
		p.compression.Type = gen.DefaultCompressionType
	}
	if p.compression.Threshold == 0 {
		p.compression.Threshold = gen.DefaultCompressionThreshold
	}

	switch options.SendPriority {
	case gen.MessagePriorityHigh:
		p.priority = gen.MessagePriorityHigh
	case gen.MessagePriorityMax:
		p.priority = gen.MessagePriorityMax
	default:
		p.priority = gen.MessagePriorityNormal
	}

	// create a new process with provided behavior
	behavior := factory()
	if behavior == nil {
		n.names.Delete(p.name)
		return p.pid, errors.New("factory function must return non nil value")
	}
	p.behavior = behavior
	p.sbehavior = strings.TrimPrefix(reflect.TypeOf(behavior).String(), "*")

	if options.LogLevel == gen.LogLevelDefault {
		// parent's log level
		options.LogLevel = options.ParentLogLevel
	}
	p.log = createLog(options.LogLevel, n.dolog)

	logSource := gen.MessageLogProcess{
		Node:     p.pid.Node,
		PID:      p.pid,
		Name:     p.name,
		Behavior: p.sbehavior,
	}
	p.log.setSource(logSource)

	if err := behavior.ProcessInit(p, options.Args...); err != nil {
		n.names.Delete(p.name)
		// make sure to notify children that might have been spawned
		// (during ProcessInit callback) with the enabled LinkParent option
		messageExit := gen.MessageExitPID{
			PID:    p.pid,
			Reason: err,
		}
		for _, pid := range n.links.unregister(p.pid) {
			n.sendExitMessage(p.pid, pid, messageExit)
		}

		// clean up links to the processes that might have been spawned
		// (during ProcessInit callback) with the enabled LinkChild option
		p.targets.Range(func(k, v any) bool {
			if v.(bool) {
				n.links.unregisterConsumer(k, p.pid)
			}
			return true
		})

		// terminate meta process that spawned during initialization

		p.metas.Range(func(_, v any) bool {
			m := v.(*meta)

			qm := gen.TakeMailboxMessage()
			qm.From = p.pid
			qm.Type = gen.MailboxMessageTypeExit
			qm.Message = err

			if ok := m.system.Push(qm); ok == false {
				p.log.Error("unable to stop meta process %s. mailbox is full", m.id)
			}
			p.node.aliases.Delete(m.id)
			go m.handle()
			return true
		})

		return p.pid, err
	}

	if options.LinkParent {
		n.links.registerConsumer(p.parent, p.pid)
		p.targets.Store(p.parent, true)
	}

	// register process and switch it to the sleep state
	p.state = int32(gen.ProcessStateSleep)
	n.processes.Store(p.pid, p)

	// do not count system app processes
	if p.application != system.Name {
		n.waitprocesses.Add(1)
	}

	// process could send a message to itself during initialization
	// so we should run this process to make sure this message is handled
	p.run()

	return p.pid, nil
}

func (n *node) unregisterProcess(p *process, reason error) {
	n.processes.Delete(p.pid)
	n.RouteTerminatePID(p.pid, reason)

	if p.application != system.Name {
		// do not count system app processes
		n.waitprocesses.Done()
	}
	n.log.Trace("...unregisterProcess %s", p.pid)

	if p.registered.Load() {
		n.names.Delete(p.name)
		pname := gen.ProcessID{Name: p.name, Node: n.name}
		n.RouteTerminateProcessID(pname, reason)
	}

	for _, a := range p.aliases {
		n.aliases.Delete(a)
		n.RouteTerminateAlias(a, reason)
	}

	p.events.Range(func(k, _ any) bool {
		ev := gen.Event{Name: k.(gen.Atom), Node: p.node.name}
		n.events.Delete(ev)
		n.RouteTerminateEvent(ev, reason)
		return true
	})

	p.targets.Range(func(target, v any) bool {
		if v.(bool) {
			n.links.unregisterConsumer(target, p.pid)
		} else {
			n.monitors.unregisterConsumer(target, p.pid)
		}
		return true
	})

	// send exit signal to the meta processes
	p.metas.Range(func(_, v any) bool {
		m := v.(*meta)

		qm := gen.TakeMailboxMessage()
		qm.From = p.pid
		qm.Type = gen.MailboxMessageTypeExit
		qm.Message = reason

		p.node.aliases.Delete(m.id)
		if ok := m.system.Push(qm); ok == false {
			p.log.Error("unable to stop meta process %s. mailbox is full", m.id)
		}
		m.handle()
		return true
	})

	if p.loggername != "" { // acted as a logger
		n.LoggerDelete(p.loggername)
		// enable logging. it might be used
		// in the termination callback (like a act.ActorBehavior.Terminate)
		p.log.SetLevel(gen.LogLevelInfo)
	}

	if p.application == "" {
		return
	}

	if v, exist := n.applications.Load(p.application); exist {
		// this process was a member of the application
		app := v.(*application)
		app.terminate(p.pid, reason)
	}
}

func (n *node) isRunning() bool {
	return atomic.LoadInt64(&n.creation) > 0
}

func (n *node) registerAlias(alias gen.Alias, p *process) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	n.log.Trace("...registerAlias %s for %s", alias, p.pid)
	if _, exist := n.aliases.LoadOrStore(alias, p); exist {
		return gen.ErrTaken
	}
	return nil
}

func (n *node) unregisterAlias(alias gen.Alias, p *process) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	value, found := n.aliases.Load(alias)
	if found == false {
		return gen.ErrAliasUnknown
	}
	owner := value.(*process)
	if p != owner {
		return gen.ErrAliasOwner
	}
	n.log.Trace("...unregisterAlias %s for %s", alias, p.pid)

	n.aliases.Delete(alias)
	return nil
}

func (n *node) registerEvent(name gen.Atom, owner gen.PID, options gen.EventOptions) (gen.Ref, error) {
	token := gen.Ref{}
	if n.isRunning() == false {
		return token, gen.ErrNodeTerminated
	}

	n.log.Trace("...registerEvent %s for %s", name, owner)
	ev := gen.Event{Name: name, Node: n.name}
	event := &eventOwner{
		name:     name,
		producer: owner,
		notify:   options.Notify,
	}

	if options.Buffer > 0 {
		event.last = lib.NewQueueLimitMPSC(int64(options.Buffer), true)
	}

	if _, exist := n.events.LoadOrStore(ev, event); exist {
		return token, gen.ErrTaken
	}
	event.token = n.MakeRef()
	return event.token, nil
}

func (n *node) unregisterEvent(name gen.Atom, pid gen.PID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	n.log.Trace("...unregisterEvent %s for %s", name, pid)
	ev := gen.Event{Name: name, Node: n.name}
	value, exist := n.events.Load(ev)
	if exist == false {
		return gen.ErrEventUnknown
	}

	event := value.(*eventOwner)
	if event.producer != pid {
		return gen.ErrEventOwner
	}

	n.events.Delete(ev)
	n.RouteTerminateEvent(ev, gen.ErrUnregistered)
	return nil
}

func (n *node) validateLicenses(versions ...gen.Version) {
	for _, version := range versions {
		switch version.License {
		case gen.LicenseMIT:
			continue

		case "":
			if lib.Trace() {
				n.Log().Trace("undefined license for %s", version)
			}
			continue

		case gen.LicenseBSL1:
			var valid bool

			if _, exist := n.licenses.LoadOrStore(version, valid); exist {
				continue
			}

			// TODO validate license
			//if valid {
			//	continue
			//}

			n.Log().Warning("%s is distributed under %q and can not be used "+
				"without a license for production/commercial purposes",
				version, version.License)

		default:
			if lib.Trace() {
				n.Log().Trace("unhandled license %q for %s", version.License, version)
			}
		}
	}
}
