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
	"ergo.services/ergo/node/tm"
)

var (
	startID     = uint64(1000)
	startUniqID = uint64(time.Now().UnixNano())

	nodeCallPool = sync.Pool{
		New: func() any {
			return &nodeCall{
				done: make(chan struct{}, 1),
			}
		},
	}
)

type nodeCall struct {
	done      chan struct{} // semaphore to signal call is done
	response  any
	err       error
	from      gen.PID
	important bool
}

func takeNodeCall() *nodeCall {
	return nodeCallPool.Get().(*nodeCall)
}

func releaseNodeCall(r *nodeCall) {
	// Drain channel before returning to pool (safety)
	select {
	case <-r.done:
	default:
	}
	r.response, r.err = nil, nil
	r.important = false
	r.from = gen.PID{}
	nodeCallPool.Put(r)
}

type node struct {
	// core is the routing surface handed to collaborators (processes, connections,
	// node-level Send/Call). Defaults to the node itself; a decorator may replace it
	// (testing/stage) to observe routing and delivery.
	core gen.Core
	// wrapProcess optionally decorates the gen.Process handed to a behavior at
	// ProcessInit (testing/stage) to observe a process's egress actions.
	wrapProcess func(gen.Process) gen.Process

	name      gen.Atom
	version   gen.Version
	framework gen.Version

	creation int64

	env sync.Map // env name gen.Env -> any

	security    gen.SecurityOptions
	certmanager gen.CertManager

	corePID   gen.PID
	nextID    uint64
	uniqID    uint64
	traceID   uint64
	spanID    uint64
	nameCRC32 uint64

	processes sync.Map // process pid gen.PID -> *process
	names     sync.Map // process name gen.Atom -> *process
	aliases   sync.Map // process alias gen.Alias -> *process
	calls     sync.Map // node-level call responses gen.Ref -> *nodeCall

	applications sync.Map // application name -> *application

	// consumer lists (subscribers)
	targets gen.TargetManager

	network *network

	cron *cron

	loggers   map[gen.LogLevel]*sync.Map // level -> name -> gen.LoggerBehavior
	loggersMu sync.Mutex                 // serializes LoggerAdd/LoggerDelete registration
	log       *log

	tracingExporters sync.Map // name -> tracingExporterEntry
	tracing          gen.Tracing
	tracingSampler   atomic.Pointer[gen.TracingSampler]

	shutdownTimeout time.Duration
	waitprocesses   sync.WaitGroup
	wait            chan struct{}
	once            sync.Once
	stopping        atomic.Bool // CAS-guard against concurrent Stop/StopForce

	licenses sync.Map

	coreEventsToken gen.Ref

	enableCTRLC atomic.Bool
	ctrlc       chan os.Signal

	processesSpawned     uint64
	processesSpawnFailed uint64
	processesTerminated  uint64

	sendErrorsLocal  uint64
	sendErrorsRemote uint64
	callErrorsLocal  uint64
	callErrorsRemote uint64

	logMessages  [6]uint64                              // atomic: 0=trace, 1=debug, 2=info, 3=warning, 4=error, 5=panic
	tracingSpans [5]uint64                              // atomic: 0=send, 1=request, 2=response, 3=spawn, 4=terminate
	tracingAttrs atomic.Pointer[[]gen.TracingAttribute] // node-level permanent, COW; pointer always non-nil
}

type tracingExporterEntry struct {
	exporter   gen.TracingBehavior
	flags      gen.TracingFlags
	pid        gen.PID                          // non-zero for process-based exporters
	dispatcher *lib.Dispatcher[gen.TracingSpan] // decouples an object exporter from the routing goroutine
}

// tracingExporterQueue bounds the buffer of spans awaiting an object exporter's HandleSpan.
const tracingExporterQueue = 1024

type eventOwner struct {
	name      gen.Atom
	producer  gen.PID
	token     gen.Ref
	notify    bool
	consumers int32

	last lib.QueueMPSC
}

// NodeOptionsExtra carries options not exposed through the public ergo.StartNode
// entry point. WrapCore decorates the routing surface handed to collaborators;
// WrapProcess decorates the gen.Process handed to each behavior at ProcessInit.
type NodeOptionsExtra struct {
	gen.NodeOptions
	FrameworkVersion      gen.Version
	WrapCore              func(gen.Core) gen.Core
	WrapProcess           func(gen.Process) gen.Process
	WrapCoreTargetManager func(gen.CoreTargetManager) gen.CoreTargetManager
}

func Start(name gen.Atom, extra NodeOptionsExtra) (gen.Node, error) {
	options := extra.NodeOptions
	frameworkVersion := extra.FrameworkVersion
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

	if options.ShutdownTimeout <= 0 {
		options.ShutdownTimeout = gen.DefaultShutdownTimeout
	}

	node := &node{
		name:      name,
		version:   options.Version,
		framework: frameworkVersion,
		creation:  creation,

		corePID:   gen.PID{Node: name, ID: 1, Creation: creation},
		nextID:    startID,
		uniqID:    startUniqID,
		nameCRC32: uint64(name.CRC32Sum()),

		certmanager: options.CertManager,
		security:    options.Security,

		shutdownTimeout: options.ShutdownTimeout,

		loggers: make(map[gen.LogLevel]*sync.Map),

		wait: make(chan struct{}),
	}
	node.core = node
	if extra.WrapCore != nil {
		node.core = extra.WrapCore(node)
	}
	node.wrapProcess = extra.WrapProcess
	node.tracingAttrs.Store(new([]gen.TracingAttribute))

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

	for _, te := range options.Tracing.Exporters {
		if len(te.Name) == 0 {
			return nil, errors.New("tracing exporter name can not be empty")
		}
		if te.Exporter == nil {
			return nil, errors.New("tracing exporter can not be nil")
		}
		if err := node.TracingExporterAdd(te.Name, te.Exporter, te.Flags); err != nil {
			return nil, fmt.Errorf("tracing exporter %q: %w", te.Name, err)
		}
	}

	// create target manager (pub/sub subsystem) before network start
	// because registrar may call RegisterEvent during network initialization
	bridge := gen.CoreTargetManager(createTMBridge(node))
	if extra.WrapCoreTargetManager != nil {
		bridge = extra.WrapCoreTargetManager(bridge)
	}
	node.targets = tm.Create(bridge, tm.Options{})

	node.network = createNetwork(node)

	if err := node.NetworkStart(options.Network); err != nil {
		return nil, err
	}

	node.coreEventsToken, _ = node.RegisterEvent(gen.CoreEvent, gen.EventOptions{Buffer: 1000})

	// Pre-register user-declared node-level events before starting cron and
	// applications so processes can subscribe from Init() without racing the
	// producer registration.
	for _, spec := range options.Events {
		if _, err := node.RegisterEvent(spec.Name, gen.EventOptions{
			Buffer: spec.Buffer,
			Open:   true,
		}); err != nil {
			node.StopForce()
			return nil, fmt.Errorf("unable to pre-register event %q: %w", spec.Name, err)
		}
	}

	node.cron = createCron(node)
	for _, job := range options.Cron.Jobs {
		if err := node.cron.AddJob(job); err != nil {
			node.StopForce()
			return nil, err
		}
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

	node.log.Info("node %s built with %q successfully started", node.name, node.framework)

	// enable SIGTERM
	node.SetCTRLC(true)

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
	return n.env.Load(name.String())
}

func (n *node) EnvDefault(name gen.Env, def any) any {
	value, ok := n.env.Load(name.String())
	if ok == false {
		return def
	}
	return value
}

func (n *node) CertManager() gen.CertManager {
	return n.certmanager
}

func (n *node) Security() gen.SecurityOptions {
	return n.security
}

func (n *node) Spawn(
	factory gen.ProcessFactory,
	options gen.ProcessOptions,
	args ...any,
) (gen.PID, error) {
	if n.isRunning() == false {
		return gen.PID{}, gen.ErrNodeTerminated
	}

	// calculate deadline
	timeout := options.InitTimeout
	if timeout == 0 {
		timeout = gen.DefaultRequestTimeout
	}
	deadline := time.Now().Unix() + int64(timeout)
	ref, err := n.MakeRefWithDeadline(deadline)
	if err != nil {
		return gen.PID{}, err
	}

	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		Args:           args,
		ParentPID:      n.corePID,
		ParentLeader:   n.corePID,
		ParentLogLevel: n.log.Level(),
		ParentEnv:      n.EnvList(),
		Ref:            ref,
	}

	return n.spawn(factory, opts)
}

func (n *node) SpawnRegister(register gen.Atom, factory gen.ProcessFactory,
	options gen.ProcessOptions, args ...any) (gen.PID, error) {
	if n.isRunning() == false {
		return gen.PID{}, gen.ErrNodeTerminated
	}
	if len(register) > 255 {
		return gen.PID{}, gen.ErrAtomTooLong
	}

	// calculate deadline
	timeout := options.InitTimeout
	if timeout == 0 {
		timeout = gen.DefaultRequestTimeout
	}
	deadline := time.Now().Unix() + int64(timeout)
	ref, err := n.MakeRefWithDeadline(deadline)
	if err != nil {
		return gen.PID{}, err
	}

	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		Register:       register,
		Args:           args,
		ParentPID:      n.corePID,
		ParentLeader:   n.corePID,
		ParentLogLevel: n.log.Level(),
		ParentEnv:      n.EnvList(),
		Ref:            ref,
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
	info.Application = appName(p.application)
	info.Behavior = mp.sbehavior
	info.MailboxSize = mp.main.Size()
	info.MailboxQueues.Main = mp.main.Len()
	info.MailboxQueues.System = mp.system.Len()
	info.MessagesIn = atomic.LoadUint64(&mp.messagesIn)
	info.MessagesOut = atomic.LoadUint64(&mp.messagesOut)
	info.MessagePriority = gen.MessagePriority(mp.priority.Load())
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
	info.Application = appName(p.application)
	info.Behavior = p.sbehavior
	info.Kind = p.kind
	info.MailboxSize = p.mailbox.Main.Size()
	info.MailboxQueues.Main = p.mailbox.Main.Len()
	info.MailboxQueues.Urgent = p.mailbox.Urgent.Len()
	info.MailboxQueues.System = p.mailbox.System.Len()
	info.MailboxQueues.Log = p.mailbox.Log.Len()
	info.MailboxQueues.LatencyMain = p.mailbox.Main.Latency()
	info.MailboxQueues.LatencySystem = p.mailbox.System.Latency()
	info.MailboxQueues.LatencyUrgent = p.mailbox.Urgent.Latency()
	info.MailboxQueues.LatencyLog = p.mailbox.Log.Latency()
	info.MessagesIn = atomic.LoadUint64(&p.messagesIn)
	info.MessagesOut = atomic.LoadUint64(&p.messagesOut)
	info.RunningTime = atomic.LoadUint64(&p.runningTime)
	info.InitTime = atomic.LoadUint64(&p.initTime)
	info.Wakeups = atomic.LoadUint64(&p.wakeups)
	info.Compression = *p.compression.Load()
	info.MessagePriority = gen.MessagePriority(p.priority.Load())
	info.Uptime = p.Uptime()
	info.State = p.State()
	info.StateTime = time.Now().UnixNano() - atomic.LoadInt64(&p.stateEntered)
	info.Parent = p.parent
	info.Leader = p.leader
	info.Fallback = p.fallback
	info.Aliases = p.Aliases()
	info.Events = p.Events()
	info.LogLevel = p.log.Level()
	info.KeepNetworkOrder = p.keeporder.Load()
	info.ImportantDelivery = p.important.Load()
	info.Tracing = gen.TracingInfo{
		Sampler:    p.TracingSampler().String(),
		Attributes: *p.tracingAttrs.Load(),
	}

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

	// Get links and monitors from separate managers
	linkTargets := n.targets.LinksFor(pid)
	monitorTargets := n.targets.MonitorsFor(pid)

	for _, target := range linkTargets {
		switch m := target.(type) {
		case gen.PID:
			info.LinksPID = append(info.LinksPID, m)
		case gen.ProcessID:
			info.LinksProcessID = append(info.LinksProcessID, m)
		case gen.Alias:
			info.LinksAlias = append(info.LinksAlias, m)
		case gen.Event:
			info.LinksEvent = append(info.LinksEvent, m)
		case gen.Atom:
			info.LinksNode = append(info.LinksNode, m)
		}
	}

	for _, target := range monitorTargets {
		switch m := target.(type) {
		case gen.PID:
			info.MonitorsPID = append(info.MonitorsPID, m)
		case gen.ProcessID:
			info.MonitorsProcessID = append(info.MonitorsProcessID, m)
		case gen.Alias:
			info.MonitorsAlias = append(info.MonitorsAlias, m)
		case gen.Event:
			info.MonitorsEvent = append(info.MonitorsEvent, m)
		case gen.Atom:
			info.MonitorsNode = append(info.MonitorsNode, m)
		}
	}

	return info, nil
}

func (n *node) SetProcessLogLevel(pid gen.PID, level gen.LogLevel) error {
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

func (n *node) SetProcessSendPriority(pid gen.PID, priority gen.MessagePriority) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	switch priority {
	case gen.MessagePriorityNormal:
	case gen.MessagePriorityHigh:
	case gen.MessagePriorityMax:
	default:
		return gen.ErrIncorrect
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)
	p.priority.Store(int32(priority))
	return nil
}

func (n *node) SetProcessCompression(pid gen.PID, enabled bool) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)
	p.updateCompression(func(c *gen.Compression) { c.Enable = enabled })
	return nil
}

func (n *node) SetProcessCompressionType(pid gen.PID, ctype gen.CompressionType) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	switch ctype {
	case gen.CompressionTypeGZIP:
	case gen.CompressionTypeLZW:
	case gen.CompressionTypeZLIB:
	default:
		return gen.ErrIncorrect
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)
	p.updateCompression(func(c *gen.Compression) { c.Type = ctype })
	return nil
}

func (n *node) SetProcessCompressionLevel(pid gen.PID, level gen.CompressionLevel) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	switch level {
	case gen.CompressionBestSize:
	case gen.CompressionBestSpeed:
	case gen.CompressionDefault:
	default:
		return gen.ErrIncorrect
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)
	p.updateCompression(func(c *gen.Compression) { c.Level = level })
	return nil
}

func (n *node) SetProcessCompressionThreshold(pid gen.PID, threshold int) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	if threshold < gen.DefaultCompressionThreshold {
		return gen.ErrIncorrect
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)
	p.updateCompression(func(c *gen.Compression) { c.Threshold = threshold })
	return nil
}

func (n *node) SetProcessKeepNetworkOrder(pid gen.PID, order bool) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)
	p.keeporder.Store(order)
	return nil
}

func (n *node) SetProcessImportantDelivery(pid gen.PID, important bool) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)
	p.important.Store(important)
	return nil
}

func (n *node) SetMetaLogLevel(m gen.Alias, level gen.LogLevel) error {
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

func (n *node) SetMetaSendPriority(m gen.Alias, priority gen.MessagePriority) error {
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

	switch priority {
	case gen.MessagePriorityNormal:
	case gen.MessagePriorityHigh:
	case gen.MessagePriorityMax:
	default:
		return gen.ErrIncorrect
	}
	mp.priority.Store(int32(priority))
	return nil
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

	info.Tracing = gen.TracingInfo{
		Sampler:    n.TracingSampler().String(),
		Attributes: *n.tracingAttrs.Load(),
	}

	n.tracingExporters.Range(func(k, v any) bool {
		entry := v.(tracingExporterEntry)
		behavior := ""
		if entry.exporter != nil {
			behavior = strings.TrimPrefix(reflect.TypeOf(entry.exporter).String(), "*")
		}
		var dropped uint64
		if entry.dispatcher != nil {
			dropped = entry.dispatcher.Dropped()
		}
		info.TracingExporters = append(info.TracingExporters, gen.TracingExporterInfo{
			Name:         k.(string),
			Behavior:     behavior,
			Flags:        entry.flags,
			DroppedSpans: dropped,
		})
		return true
	})

	for i := 0; i < 6; i++ {
		info.LogMessages[i] = atomic.LoadUint64(&n.logMessages[i])
	}
	for i := 0; i < 5; i++ {
		info.TracingSpans[i] = atomic.LoadUint64(&n.tracingSpans[i])
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
			info.ProcessesWaitResponse++
		case gen.ProcessStateZombee:
			info.ProcessesZombee++
		}
		return true
	})

	info.ProcessesSpawned = atomic.LoadUint64(&n.processesSpawned)
	info.ProcessesSpawnFailed = atomic.LoadUint64(&n.processesSpawnFailed)
	info.ProcessesTerminated = atomic.LoadUint64(&n.processesTerminated)

	info.SendErrorsLocal = atomic.LoadUint64(&n.sendErrorsLocal)
	info.SendErrorsRemote = atomic.LoadUint64(&n.sendErrorsRemote)
	info.CallErrorsLocal = atomic.LoadUint64(&n.callErrorsLocal)
	info.CallErrorsRemote = atomic.LoadUint64(&n.callErrorsRemote)

	n.names.Range(func(_, _ any) bool {
		info.RegisteredNames++
		return true
	})
	n.aliases.Range(func(_, _ any) bool {
		info.RegisteredAliases++
		return true
	})

	tmInfo := n.targets.Info()
	info.RegisteredEvents = tmInfo.Events
	info.EventsPublished = tmInfo.EventsPublished
	info.EventsReceived = tmInfo.EventsReceived
	info.EventsLocalSent = tmInfo.EventsLocalSent
	info.EventsRemoteSent = tmInfo.EventsRemoteSent

	info.ApplicationsTotal = int64(len(n.Applications()))
	info.ApplicationsRunning = int64(len(n.ApplicationsRunning()))

	rm := lib.ReadRuntimeMetrics()
	info.MemoryUsed = rm.MemoryTotal
	info.MemoryAlloc = rm.MemoryObjects

	utime, stime := osdep.ResourceUsage()
	info.UserTime = utime
	info.SystemTime = stime

	info.ServerTime = time.Now()

	return info, nil
}

func (n *node) ShortInfo() (gen.NodeShortInfo, error) {
	var info gen.NodeShortInfo
	if n.isRunning() == false {
		return info, gen.ErrNodeTerminated
	}

	info.Name = n.name
	info.Creation = atomic.LoadInt64(&n.creation)
	info.Uptime = n.Uptime()
	info.Version = n.version
	info.Framework = n.framework
	info.LogLevel = n.log.Level()
	info.Mode = n.network.Mode()
	info.Peers = n.peers()

	n.processes.Range(func(_, v any) bool {
		info.ProcessesTotal++
		p := v.(*process)
		switch p.State() {
		case gen.ProcessStateRunning:
			info.ProcessesRunning++
		case gen.ProcessStateWaitResponse:
			info.ProcessesWaitResponse++
		case gen.ProcessStateZombee:
			info.ProcessesZombee++
		}
		return true
	})

	info.ProcessesSpawned = atomic.LoadUint64(&n.processesSpawned)
	info.ProcessesSpawnFailed = atomic.LoadUint64(&n.processesSpawnFailed)
	info.ProcessesTerminated = atomic.LoadUint64(&n.processesTerminated)

	// one pass for the counters and the names
	n.applications.Range(func(_, v any) bool {
		app := v.(*application)
		info.ApplicationsTotal++
		if app.isRunning() {
			info.ApplicationsRunning++
		}
		info.Applications = append(info.Applications, app.spec.Name)
		return true
	})

	info.SendErrorsLocal = atomic.LoadUint64(&n.sendErrorsLocal)
	info.SendErrorsRemote = atomic.LoadUint64(&n.sendErrorsRemote)
	info.CallErrorsLocal = atomic.LoadUint64(&n.callErrorsLocal)
	info.CallErrorsRemote = atomic.LoadUint64(&n.callErrorsRemote)

	for i := 0; i < 6; i++ {
		info.LogMessages[i] = atomic.LoadUint64(&n.logMessages[i])
	}

	rmShort := lib.ReadRuntimeMetrics()
	info.MemoryUsed = rmShort.MemoryTotal
	info.MemoryAlloc = rmShort.MemoryObjects
	info.MemoryLimit = rmShort.MemoryLimit
	info.HeapLive = rmShort.HeapLive
	info.HeapGoal = rmShort.HeapGoal
	info.Goroutines = rmShort.Goroutines
	info.GCCycles = rmShort.GCCycles
	info.GCCPUFraction = rmShort.GCCPUFraction

	utime, stime := osdep.ResourceUsage()
	info.UserTime = utime
	info.SystemTime = stime

	info.ServerTime = time.Now()

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

func (n *node) ProcessListShortInfo(start, limit int, filter ...func(gen.ProcessShortInfo) bool) ([]gen.ProcessShortInfo, error) {
	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	if limit < 0 || (start >= 0 && start < 1000) {
		return nil, gen.ErrIncorrect
	}
	if limit == 0 {
		limit = 100
	}

	nextID := atomic.LoadUint64(&n.nextID)
	from, to, step := int64(start), int64(nextID)+1, int64(1)
	if start < 0 {
		from, to, step = int64(nextID), 999, -1
	}

	psi := []gen.ProcessShortInfo{}
	pid := n.corePID

	for id := from; id != to && limit > 0; id += step {
		pid.ID = uint64(id)
		v, found := n.processes.Load(pid)
		if found == false {
			continue
		}
		process := v.(*process)
		info := process.shortInfo()
		if len(filter) > 0 && filter[0](info) == false {
			continue
		}
		psi = append(psi, info)
		limit--
	}

	return psi, nil

}

func (n *node) ProcessRangeShortInfo(fn func(gen.ProcessShortInfo) bool) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	n.processes.Range(func(_, v any) bool {
		p := v.(*process)
		return fn(p.shortInfo())
	})

	return nil
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
	return n.network
}

// Peers implements gen.NodeRegistrar.
func (n *node) Peers() []gen.Atom {
	return n.network.Nodes()
}

// peers describes the current connections for gen.NodeShortInfo.
func (n *node) peers() []gen.RemoteNodeShortInfo {
	nodes := n.network.Nodes()
	peers := make([]gen.RemoteNodeShortInfo, 0, len(nodes))

	for _, name := range nodes {
		remote, err := n.network.Node(name)
		if err != nil {
			// disconnected between listing and reading
			peers = append(peers, gen.RemoteNodeShortInfo{Node: name})
			continue
		}
		info := remote.Info()
		peers = append(peers, gen.RemoteNodeShortInfo{
			Node:             info.Node,
			ConnectionUptime: info.ConnectionUptime,
			MessagesIn:       info.MessagesIn,
			MessagesOut:      info.MessagesOut,
			BytesIn:          info.BytesIn,
			BytesOut:         info.BytesOut,
			Reconnections:    info.Reconnections,
			TLS:              info.TLS,
		})
	}
	return peers
}

func (n *node) Cron() gen.Cron {
	return n.cron
}

func (n *node) Stop() {
	n.stop(false, n.shutdownTimeout)
}

func (n *node) StopWithTimeout(timeout time.Duration) {
	if timeout <= 0 {
		timeout = n.shutdownTimeout
	}
	n.stop(false, timeout)
}

func (n *node) StopForce() {
	n.stop(true, 0)
}

func (n *node) stop(force bool, shutdownTimeout time.Duration) {
	if n.isRunning() == false {
		// already stopped
		return
	}
	if n.stopping.CompareAndSwap(false, true) == false {
		// another Stop/StopForce call already in flight
		return
	}

	if force == false {
		n.applications.Range(func(_, v any) bool {
			app := v.(*application)
			if app.spec.Name == system.Name {
				// skip system app
				return true
			}

			n.log.Trace("stopping application %s (waiting 5 seconds) ...", app.spec.Name)
			if err := app.stop(false, 5*time.Second); err == gen.ErrApplicationStopping {
				n.log.Trace("stopping application %s is still in progress", app.spec.Name)
				return true
			}

			n.log.Trace("stopped application: %s", app.spec.Name)
			return true
		})
	}

	n.processes.Range(func(_, v any) bool {
		p := v.(*process)

		if force {
			n.Kill(p.pid)
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
		n.waitProcessesWithEscalation(shutdownTimeout)
	}

	n.NetworkStop()
	atomic.StoreInt64(&n.creation, 0)

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

	n.once.Do(func() {
		close(n.wait)
	})
}

// waitProcessesWithEscalation blocks until all tracked processes have called
// waitprocesses.Done. Periodically logs stuck processes. On shutdownTimeout
// escalates to force-kill of all remaining processes, then waits a short
// settle window. As a last resort calls os.Exit(1) if processes refuse to die.
func (n *node) waitProcessesWithEscalation(shutdownTimeout time.Duration) {
	done := make(chan struct{})
	go func() {
		n.waitprocesses.Wait()
		close(done)
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timeout := time.After(shutdownTimeout)

	for {
		select {
		case <-done:
			return
		case <-timeout:
			lines, total := n.snapshotRunningProcesses()
			n.log.Error("shutdown timeout %s expired, killing %d remaining process(es):", shutdownTimeout, total)
			n.logSnapshot(lines, total)
			n.processes.Range(func(_, v any) bool {
				p := v.(*process)
				n.Kill(p.pid)
				return true
			})
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				lines, total := n.snapshotRunningProcesses()
				n.log.Error("processes still alive after force kill, hard exit (%d remaining):", total)
				n.logSnapshot(lines, total)
				os.Exit(1)
			}
			return
		case <-ticker.C:
			lines, total := n.snapshotRunningProcesses()
			if total > 0 {
				n.log.Warning("node %s is still waiting for process(es) to terminate:", n.name)
				n.logSnapshot(lines, total)
			}
		}
	}
}

// snapshotRunningProcesses gathers up to 10 still-running process descriptions
// plus the total count (which may exceed 10).
func (n *node) snapshotRunningProcesses() (lines []string, total int) {
	n.processes.Range(func(_, v any) bool {
		total++
		if total > 10 {
			return true
		}
		p := v.(*process)
		name := p.sbehavior
		if p.name != "" {
			name = fmt.Sprintf("%s, %s", p.name, p.sbehavior)
		}
		state := gen.ProcessState(atomic.LoadInt32(&p.state))
		qlen := p.mailbox.Len()
		lines = append(lines, fmt.Sprintf("  %s (%s) state: %s, queue: %d", p.pid, name, state, qlen))
		return true
	})
	return
}

func (n *node) logSnapshot(lines []string, total int) {
	for _, l := range lines {
		n.log.Warning(l)
	}
	if total > 10 {
		n.log.Warning("  ...and %d more", total-10)
	}
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

	var tracing gen.Tracing
	if s := n.tracingSampler.Load(); s != nil && (*s).Sample() {
		tracing = n.MakeTraceID()
		tracing.Behavior = "core"
	}
	options := gen.MessageOptions{
		Priority:          priority,
		Tracing:           tracing,
		TracingAttributes: *n.tracingAttrs.Load(),
	}

	switch t := to.(type) {
	case gen.Atom:
		return n.core.RouteSendProcessID(n.corePID, gen.ProcessID{Name: t, Node: n.name}, options, message)
	case gen.PID:
		return n.core.RouteSendPID(n.corePID, t, options, message)
	case gen.ProcessID:
		return n.core.RouteSendProcessID(n.corePID, t, options, message)
	case gen.Alias:
		return n.core.RouteSendAlias(n.corePID, t, options, message)
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

	return n.core.RouteSendEvent(n.corePID, token, options, em)
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

func (n *node) EventInfo(event gen.Event) (gen.EventInfo, error) {
	if n.isRunning() == false {
		return gen.EventInfo{}, gen.ErrNodeTerminated
	}
	return n.targets.EventInfo(event)
}

func (n *node) EventRangeInfo(fn func(gen.EventInfo) bool) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	return n.targets.EventRangeInfo(fn)
}

func (n *node) EventListInfo(timestamp int64, limit int, filter ...func(gen.EventInfo) bool) ([]gen.EventInfo, error) {
	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}
	return n.targets.EventListInfo(timestamp, limit, filter...)
}

func (n *node) SendExit(pid gen.PID, reason error) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	return n.core.RouteSendExit(n.corePID, pid, reason)
}

func (n *node) Call(to any, request any) (any, error) {
	options := gen.MessageOptions{
		Priority: gen.MessagePriorityNormal,
	}
	return n.callWithOptions(to, request, gen.DefaultRequestTimeout, options)
}

func (n *node) CallWithTimeout(to any, request any, timeout int) (any, error) {
	options := gen.MessageOptions{
		Priority: gen.MessagePriorityNormal,
	}
	return n.callWithOptions(to, request, timeout, options)
}

func (n *node) CallWithPriority(to any, request any, priority gen.MessagePriority) (any, error) {
	options := gen.MessageOptions{
		Priority: priority,
	}
	return n.callWithOptions(to, request, gen.DefaultRequestTimeout, options)
}

func (n *node) CallImportant(to any, request any) (any, error) {
	options := gen.MessageOptions{
		Priority:          gen.MessagePriorityNormal,
		ImportantDelivery: true,
	}
	return n.callWithOptions(to, request, gen.DefaultRequestTimeout, options)
}

func (n *node) callWithOptions(to any, request any, timeout int, options gen.MessageOptions) (any, error) {
	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}
	if s := n.tracingSampler.Load(); s != nil && (*s).Sample() {
		options.Tracing = n.MakeTraceID()
		options.Tracing.Behavior = "core"
	}
	if attrs := *n.tracingAttrs.Load(); options.Tracing.ID != [2]uint64{} && len(attrs) > 0 {
		options.TracingAttributes = attrs
	}

	switch t := to.(type) {
	case gen.Atom:
		return n.callProcessIDWithOptions(gen.ProcessID{Name: t, Node: n.name}, request, timeout, options)
	case gen.PID:
		return n.callPIDWithOptions(t, request, timeout, options)
	case gen.ProcessID:
		return n.callProcessIDWithOptions(t, request, timeout, options)
	case gen.Alias:
		return n.callAliasWithOptions(t, request, timeout, options)
	}

	return nil, gen.ErrUnsupported
}

func (n *node) CallPID(to gen.PID, request any, timeout int) (any, error) {
	options := gen.MessageOptions{
		Priority: gen.MessagePriorityNormal,
	}
	return n.callPIDWithOptions(to, request, timeout, options)
}

func (n *node) callPIDWithOptions(
	to gen.PID,
	request any,
	timeout int,
	options gen.MessageOptions,
) (any, error) {
	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	if timeout < 1 {
		timeout = gen.DefaultRequestTimeout
	}

	deadline := time.Now().Unix() + int64(timeout)
	ref, err := n.MakeRefWithDeadline(deadline)
	if err != nil {
		return nil, err
	}

	call := takeNodeCall()
	n.calls.Store(ref, call)
	defer n.calls.Delete(ref)

	options.Ref = ref

	if err = n.core.RouteCallPID(n.corePID, to, options, request); err != nil {
		releaseNodeCall(call)
		return nil, err
	}

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)
	timer.Reset(time.Duration(timeout) * time.Second)

	select {
	case <-call.done:
		goto handleResponse
	case <-timer.C:
		// Check if response arrived at the same moment as timeout
		select {
		case <-call.done:
			goto handleResponse
		default:
			// No response yet - don't return to pool as late response might arrive
			return nil, gen.ErrTimeout
		}
	}

handleResponse:
	response := call.response
	err = call.err
	important := call.important
	cfrom := call.from
	releaseNodeCall(call)

	if important {
		options := gen.MessageOptions{
			Ref: ref,
		}

		// send ack
		n.RouteSendResponseError(n.corePID, cfrom, options, nil)
	}

	if err != nil {
		return nil, err
	}
	return response, nil
}

func (n *node) CallProcessID(to gen.ProcessID, request any, timeout int) (any, error) {
	options := gen.MessageOptions{
		Priority: gen.MessagePriorityNormal,
	}
	return n.callProcessIDWithOptions(to, request, timeout, options)
}

func (n *node) callProcessIDWithOptions(
	to gen.ProcessID,
	request any,
	timeout int,
	options gen.MessageOptions,
) (any, error) {
	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	if timeout < 1 {
		timeout = gen.DefaultRequestTimeout
	}

	deadline := time.Now().Unix() + int64(timeout)
	ref, err := n.MakeRefWithDeadline(deadline)
	if err != nil {
		return nil, err
	}

	call := takeNodeCall()
	n.calls.Store(ref, call)
	defer n.calls.Delete(ref)

	options.Ref = ref

	if err = n.core.RouteCallProcessID(n.corePID, to, options, request); err != nil {
		releaseNodeCall(call)
		return nil, err
	}

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)
	timer.Reset(time.Duration(timeout) * time.Second)

	select {
	case <-call.done:
		goto handleResponse
	case <-timer.C:
		// Check if response arrived at the same moment as timeout
		select {
		case <-call.done:
			goto handleResponse
		default:
			// No response yet - don't return to pool as late response might arrive
			return nil, gen.ErrTimeout
		}
	}

handleResponse:
	response := call.response
	err = call.err
	important := call.important
	cfrom := call.from
	releaseNodeCall(call)

	if important {
		options := gen.MessageOptions{
			Ref: ref,
		}
		// send ack
		n.RouteSendResponseError(n.corePID, cfrom, options, nil)
	}

	if err != nil {
		return nil, err
	}
	return response, nil
}

func (n *node) CallAlias(to gen.Alias, request any, timeout int) (any, error) {
	options := gen.MessageOptions{
		Priority: gen.MessagePriorityNormal,
	}
	return n.callAliasWithOptions(to, request, timeout, options)
}

func (n *node) callAliasWithOptions(
	to gen.Alias,
	request any,
	timeout int,
	options gen.MessageOptions,
) (any, error) {
	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	if timeout < 1 {
		timeout = gen.DefaultRequestTimeout
	}

	deadline := time.Now().Unix() + int64(timeout)
	ref, err := n.MakeRefWithDeadline(deadline)
	if err != nil {
		return nil, err
	}

	call := takeNodeCall()
	n.calls.Store(ref, call)
	defer n.calls.Delete(ref)

	options.Ref = ref

	if err = n.core.RouteCallAlias(n.corePID, to, options, request); err != nil {
		releaseNodeCall(call)
		return nil, err
	}

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)
	timer.Reset(time.Duration(timeout) * time.Second)

	select {
	case <-call.done:
		goto handleResponse
	case <-timer.C:
		// Check if response arrived at the same moment as timeout
		select {
		case <-call.done:
			goto handleResponse
		default:
			// No response yet - don't return to pool as late response might arrive
			return nil, gen.ErrTimeout
		}
	}

handleResponse:
	response := call.response
	err = call.err
	important := call.important
	cfrom := call.from
	releaseNodeCall(call)

	if important {
		options := gen.MessageOptions{
			Ref: ref,
		}
		// send ack
		n.RouteSendResponseError(n.corePID, cfrom, options, nil)
	}

	if err != nil {
		return nil, err
	}
	return response, nil
}

func (n *node) Inspect(target gen.PID, item ...string) (map[string]string, error) {
	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	if target.Node != n.name {
		// inspecting remote process is not allowed
		return nil, gen.ErrNotAllowed
	}

	ref := n.MakeRef()

	value, found := n.processes.Load(target)
	if found == false {
		return nil, gen.ErrProcessUnknown
	}
	targetp := value.(*process)

	if alive := targetp.isAlive(); alive == false {
		return nil, gen.ErrProcessTerminated
	}

	call := takeNodeCall()
	n.calls.Store(ref, call)
	defer n.calls.Delete(ref)

	qm := gen.TakeMailboxMessage()
	qm.Ref = ref
	qm.From = n.corePID
	qm.Type = gen.MailboxMessageTypeInspect
	qm.Message = item

	if ok := targetp.mailbox.Urgent.Push(qm); ok == false {
		releaseNodeCall(call)
		return nil, gen.ErrProcessMailboxFull
	}

	targetp.run()

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)
	timer.Reset(time.Duration(gen.DefaultRequestTimeout) * time.Second)

	select {
	case <-call.done:
		response := call.response
		err := call.err
		releaseNodeCall(call)
		if err != nil {
			return nil, err
		}
		return response.(map[string]string), nil
	case <-timer.C:
		// Check if response arrived at the same moment as timeout
		select {
		case <-call.done:
			response := call.response
			err := call.err
			releaseNodeCall(call)
			if err != nil {
				return nil, err
			}
			return response.(map[string]string), nil
		default:
			// Don't release call - late response might arrive
			return nil, gen.ErrTimeout
		}
	}
}

func (n *node) InspectMeta(alias gen.Alias, item ...string) (map[string]string, error) {
	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	if alias.Node != n.name {
		// inspecting remote meta process is not allowed
		return nil, gen.ErrNotAllowed
	}

	value, found := n.aliases.Load(alias)
	if found == false {
		return nil, gen.ErrMetaUnknown
	}

	metap := value.(*process)
	if alive := metap.isAlive(); alive == false {
		return nil, gen.ErrProcessTerminated
	}

	value, found = metap.metas.Load(alias)
	if found == false {
		return nil, gen.ErrMetaUnknown
	}

	m := value.(*meta)
	ref := n.MakeRef()

	call := takeNodeCall()
	n.calls.Store(ref, call)
	defer n.calls.Delete(ref)

	qm := gen.TakeMailboxMessage()
	qm.Ref = ref
	qm.From = n.corePID
	qm.Type = gen.MailboxMessageTypeInspect
	qm.Message = item

	if ok := m.system.Push(qm); ok == false {
		releaseNodeCall(call)
		return nil, gen.ErrProcessMailboxFull
	}

	m.handle()

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)
	timer.Reset(time.Duration(gen.DefaultRequestTimeout) * time.Second)

	select {
	case <-call.done:
		response := call.response
		err := call.err
		releaseNodeCall(call)
		if err != nil {
			return nil, err
		}
		return response.(map[string]string), nil
	case <-timer.C:
		// Check if response arrived at the same moment as timeout
		select {
		case <-call.done:
			response := call.response
			err := call.err
			releaseNodeCall(call)
			if err != nil {
				return nil, err
			}
			return response.(map[string]string), nil
		default:
			// Don't release call - late response might arrive
			return nil, gen.ErrTimeout
		}
	}
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
	case int32(gen.ProcessStateInit),
		int32(gen.ProcessStateWaitResponse),
		int32(gen.ProcessStateRunning):
		atomic.StoreInt64(&p.stateEntered, time.Now().UnixNano())
		// do not unregister process until its goroutine stopped
		return nil
	case int32(gen.ProcessStateTerminated):
		atomic.StoreInt32(&p.state, int32(gen.ProcessStateTerminated))
		return nil
	case int32(gen.ProcessStateZombee):
		return nil
	}

	old := atomic.SwapInt32(&p.state, int32(gen.ProcessStateTerminated))
	if old == int32(gen.ProcessStateTerminated) {
		return nil
	}
	atomic.StoreInt64(&p.stateEntered, time.Now().UnixNano())
	// unregister process and stuff belonging to it. wrapPreserveMailbox so killing
	// a non-running (e.g. sleeping) process captures its mailbox just like the
	// run-loop kill path does, keeping the exit reason consistent and not losing a
	// message that raced into the mailbox before the kill.
	n.unregisterProcess(p, p.wrapPreserveMailbox(gen.TerminateReasonKill))

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

func (n *node) ProcessName(pid gen.PID) (gen.Atom, error) {
	if n.isRunning() == false {
		return "", gen.ErrNodeTerminated
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return "", gen.ErrProcessUnknown
	}
	p := value.(*process)
	return p.Name(), nil
}

func (n *node) ProcessPID(name gen.Atom) (gen.PID, error) {
	if n.isRunning() == false {
		return gen.PID{}, gen.ErrNodeTerminated
	}
	value, loaded := n.names.Load(name)
	if loaded == false {
		return gen.PID{}, gen.ErrProcessUnknown
	}
	p := value.(*process)
	return p.pid, nil
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

	a := &application{
		node:     n,
		behavior: app,
		state:    int32(gen.ApplicationStateLoaded),
	}
	a.log = createLog(n.log.Level(), n.dolog)

	spec, err := app.PreLoad(a, args...)
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

	if n.network != nil && n.network.Mode() != gen.NetworkModeDisabled {
		if len(spec.Network.RegisterTypes) > 0 {
			if err := n.network.RegisterTypes(spec.Network.RegisterTypes); err != nil {
				return name, fmt.Errorf("application %s: register types: %w", spec.Name, err)
			}
		}
		if len(spec.Network.RegisterErrors) > 0 {
			if err := n.network.RegisterErrors(spec.Network.RegisterErrors); err != nil {
				return name, fmt.Errorf("application %s: register errors: %w", spec.Name, err)
			}
		}
		if len(spec.Network.RegisterAtoms) > 0 {
			if err := n.network.RegisterAtoms(spec.Network.RegisterAtoms); err != nil {
				return name, fmt.Errorf("application %s: register atoms: %w", spec.Name, err)
			}
		}
	}

	a.spec = spec
	a.mode = spec.Mode
	a.tags = append([]gen.Atom(nil), spec.Tags...)
	a.weight = spec.Weight

	if spec.LogLevel != gen.LogLevelDefault {
		a.log.SetLevel(spec.LogLevel)
	}
	a.log.setSource(gen.MessageLogApplication{
		Node:     n.name,
		Name:     spec.Name,
		Mode:     spec.Mode,
		Behavior: strings.TrimPrefix(reflect.TypeOf(app).String(), "*"),
	})

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

func (n *node) ApplicationProcessList(name gen.Atom, limit int) ([]gen.PID, error) {
	if limit < 0 {
		return nil, gen.ErrIncorrect
	}

	if n.isRunning() == false {
		return nil, gen.ErrNodeTerminated
	}

	v, exist := n.applications.Load(name)
	if exist == false {
		return nil, gen.ErrApplicationUnknown
	}
	app := v.(*application)
	if app.isRunning() == false {
		return nil, fmt.Errorf("application %s is not running", name)
	}

	nextID := atomic.LoadUint64(&n.nextID)
	pids := []gen.PID{}
	pid := n.corePID
	for id := startID; id != nextID+1; id++ {
		pid.ID = id
		v, found := n.processes.Load(pid)
		if found == false {
			continue
		}
		p := v.(*process)
		if appName(p.application) != name {
			continue
		}
		pids = append(pids, p.pid)
		if limit > 0 && len(pids) >= limit {
			break
		}
	}

	return pids, nil
}

func (n *node) ApplicationProcessListShortInfo(name gen.Atom, limit int) ([]gen.ProcessShortInfo, int, error) {
	if limit < 0 {
		return nil, 0, gen.ErrIncorrect
	}

	if n.isRunning() == false {
		return nil, 0, gen.ErrNodeTerminated
	}

	v, exist := n.applications.Load(name)
	if exist == false {
		return nil, 0, gen.ErrApplicationUnknown
	}
	app := v.(*application)
	if app.isRunning() == false {
		return nil, 0, fmt.Errorf("application %s is not running", name)
	}

	if limit == 0 {
		limit = 100
	}

	nextID := atomic.LoadUint64(&n.nextID)
	psi := []gen.ProcessShortInfo{}
	omitted := 0
	pid := n.corePID
	for id := startID; id != nextID+1; id++ {
		pid.ID = id
		v, found := n.processes.Load(pid)
		if found == false {
			continue
		}
		p := v.(*process)
		if appName(p.application) != name {
			continue
		}
		if len(psi) >= limit {
			omitted++
			continue
		}

		info := p.shortInfo()
		psi = append(psi, info)
	}

	return psi, omitted, nil
}

// startDependencies starts every application this one depends on (each in its own
// ApplicationSpec.Mode) before it starts. Shared by all ApplicationStart* entry points.
func (n *node) startDependencies(name gen.Atom, deps []gen.Atom, options gen.ApplicationOptions, visited map[gen.Atom]bool) error {
	visited[name] = true
	defer delete(visited, name)
	for _, dep := range deps {
		if visited[dep] {
			n.log.Error("unable to start %s: circular application dependency on %s", name, dep)
			return gen.ErrApplicationDepends
		}
		v, exist := n.applications.Load(dep)
		if exist == false {
			n.log.Error("unable to start %s: unknown dependent application %s", name, dep)
			return gen.ErrApplicationDepends
		}
		app := v.(*application)
		if err := n.startDependencies(dep, app.spec.Depends.Applications, options, visited); err != nil {
			return err
		}
		opts := gen.ApplicationOptionsExtra{
			ApplicationOptions: options,
			CorePID:            n.corePID,
			CoreEnv:            n.EnvList(),
			CoreLogLevel:       n.log.Level(),
		}
		if err := app.start(app.spec.Mode, opts); err != nil {
			if err != gen.ErrApplicationRunning {
				n.log.Error(
					"unable to start %s: start dependent application %s failed: %s",
					name,
					dep,
					err,
				)
				return gen.ErrApplicationDepends
			}
		}
	}
	return nil
}

func (n *node) ApplicationStart(name gen.Atom, options gen.ApplicationOptions) error {
	v, exist := n.applications.Load(name)
	if exist == false {
		return gen.ErrApplicationUnknown
	}
	app := v.(*application)

	if err := n.startDependencies(name, app.spec.Depends.Applications, options, make(map[gen.Atom]bool)); err != nil {
		return err
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

	if err := n.startDependencies(name, app.spec.Depends.Applications, options, make(map[gen.Atom]bool)); err != nil {
		return err
	}

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

	if err := n.startDependencies(name, app.spec.Depends.Applications, options, make(map[gen.Atom]bool)); err != nil {
		return err
	}

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

	if err := n.startDependencies(name, app.spec.Depends.Applications, options, make(map[gen.Atom]bool)); err != nil {
		return err
	}

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
		p.loggerlevel = p.log.Level()
		p.log.SetLevel(gen.LogLevelDisabled)
	} else {
		return err
	}

	if lib.Verbose() {
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
	if name == "" {
		return gen.ErrIncorrect
	}

	if filter == nil {
		filter = gen.DefaultLogFilter
	}

	n.loggersMu.Lock()
	defer n.loggersMu.Unlock()

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

	if lib.Verbose() {
		n.log.Trace("node.LoggerAdd added new logger with name %q", name)
	}
	return nil
}

func (n *node) LoggerDeletePID(pid gen.PID) {
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return
	}

	p := value.(*process)
	if p.loggername != "" {
		name := p.loggername
		n.LoggerDelete(name)
		p.loggername = ""
		p.log.SetLevel(p.loggerlevel)
		n.log.Trace(
			"node.LoggerDeletePID removed process logger %s with name %q",
			pid,
			name,
		)
	}
}

func (n *node) LoggerDelete(name string) {
	var logger gen.LoggerBehavior

	n.loggersMu.Lock()
	for _, l := range n.loggers {
		if v, exist := l.LoadAndDelete(name); exist {
			logger = v.(gen.LoggerBehavior)
		}
	}
	n.loggersMu.Unlock()
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

// tracing

func (n *node) TracingExporterAddPID(pid gen.PID, name string, flags gen.TracingFlags) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)
	if p.tracingExporterName.Load() != nil {
		return gen.ErrNotAllowed
	}
	entry := tracingExporterEntry{
		flags: flags,
		pid:   pid,
	}
	if _, loaded := n.tracingExporters.LoadOrStore(name, entry); loaded {
		return gen.ErrTaken
	}
	p.tracingExporterName.Store(&name)
	return nil
}

func (n *node) TracingExporterAdd(name string, exporter gen.TracingBehavior, flags gen.TracingFlags) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	if exporter == nil {
		return gen.ErrIncorrect
	}
	// run HandleSpan on a dedicated worker so a slow or blocking exporter cannot stall the
	// routing goroutine; recover its panics so a buggy exporter cannot take the node down
	handle := func(span gen.TracingSpan) {
		defer func() {
			if rcv := recover(); rcv != nil {
				n.log.Error("tracing exporter %q panicked in HandleSpan: %#v", name, rcv)
			}
		}()
		exporter.HandleSpan(span)
	}
	entry := tracingExporterEntry{
		exporter:   exporter,
		flags:      flags,
		dispatcher: lib.NewDispatcher[gen.TracingSpan](tracingExporterQueue, handle),
	}
	if _, loaded := n.tracingExporters.LoadOrStore(name, entry); loaded {
		entry.dispatcher.Stop()
		return gen.ErrTaken
	}
	return nil
}

func (n *node) TracingExporterDeletePID(pid gen.PID) {
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return
	}
	p := value.(*process)
	if name := p.tracingExporterName.Load(); name != nil {
		n.TracingExporterDelete(*name)
		p.tracingExporterName.Store(nil)
	}
}

func (n *node) TracingExporterDelete(name string) {
	v, loaded := n.tracingExporters.LoadAndDelete(name)
	if loaded == false {
		return
	}
	entry := v.(tracingExporterEntry)
	if entry.exporter != nil {
		entry.dispatcher.Stop() // no HandleSpan runs after this returns
		entry.exporter.Terminate()
	}
}

func (n *node) TracingExporters() []string {
	var exporters []string
	n.tracingExporters.Range(func(k, _ any) bool {
		exporters = append(exporters, k.(string))
		return true
	})
	return exporters
}

func (n *node) TracingExporterFlags(name string) gen.TracingFlags {
	v, ok := n.tracingExporters.Load(name)
	if ok == false {
		return 0
	}
	return v.(tracingExporterEntry).flags
}

func (n *node) sendTracingSpan(span gen.TracingSpan) {
	if span.Kind >= 1 && span.Kind <= 5 {
		atomic.AddUint64(&n.tracingSpans[span.Kind-1], 1)
	}
	n.tracingExporters.Range(func(k, v any) bool {
		entry := v.(tracingExporterEntry)
		if matchTracingFlags(entry.flags, span) == false {
			return true
		}
		if entry.exporter != nil {
			entry.dispatcher.Push(span)
			return true
		}
		value, loaded := n.processes.Load(entry.pid)
		if loaded == false {
			return true
		}
		p := value.(*process)
		msg := gen.TakeMailboxMessage()
		msg.Type = gen.MailboxMessageTypeSpan
		msg.Message = span
		if p.mailbox.Main.Push(msg) == false {
			gen.ReleaseMailboxMessage(msg)
			return true
		}
		p.run()
		return true
	})
}

func matchTracingFlags(flags gen.TracingFlags, span gen.TracingSpan) bool {
	if span.Point == gen.TracingPointSpan {
		return flags&gen.TracingFlagReceive != 0
	}
	switch span.Kind {
	case gen.TracingKindSend, gen.TracingKindRequest, gen.TracingKindResponse:
		if span.Point == gen.TracingPointSent {
			return flags&gen.TracingFlagSend != 0
		}
		return flags&gen.TracingFlagReceive != 0
	case gen.TracingKindSpawn, gen.TracingKindTerminate:
		return flags&gen.TracingFlagProcs != 0
	}
	return false
}

func (n *node) SetTracingAttribute(key, value string) {
	if strings.HasPrefix(key, "ergo.") {
		return
	}
	cur := *n.tracingAttrs.Load()
	for i, a := range cur {
		if a.Key == key {
			attrs := make([]gen.TracingAttribute, len(cur))
			copy(attrs, cur)
			attrs[i] = gen.TracingAttribute{Key: key, Value: value}
			n.tracingAttrs.Store(&attrs)
			return
		}
	}
	attrs := make([]gen.TracingAttribute, len(cur)+1)
	copy(attrs, cur)
	attrs[len(attrs)-1] = gen.TracingAttribute{Key: key, Value: value}
	n.tracingAttrs.Store(&attrs)
}

func (n *node) RemoveTracingAttribute(key string) {
	cur := *n.tracingAttrs.Load()
	for i, a := range cur {
		if a.Key == key {
			attrs := make([]gen.TracingAttribute, len(cur)-1)
			copy(attrs, cur[:i])
			copy(attrs[i:], cur[i+1:])
			n.tracingAttrs.Store(&attrs)
			return
		}
	}
}

func (n *node) SetTracingSampler(sampler gen.TracingSampler) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	if sampler == gen.TracingSamplerDisable {
		n.tracingSampler.Store(nil)
		return nil
	}
	n.tracingSampler.Store(&sampler)
	return nil
}

func (n *node) TracingSampler() gen.TracingSampler {
	s := n.tracingSampler.Load()
	if s == nil {
		return gen.TracingSamplerDisable
	}
	return *s
}

func (n *node) SetProcessTracingSampler(pid gen.PID, sampler gen.TracingSampler) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}
	value, loaded := n.processes.Load(pid)
	if loaded == false {
		return gen.ErrProcessUnknown
	}
	p := value.(*process)
	p.tracingSampler.Store(&sampler)
	return nil
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

	switch message.Level {
	case gen.LogLevelTrace:
		atomic.AddUint64(&n.logMessages[0], 1)
	case gen.LogLevelDebug:
		atomic.AddUint64(&n.logMessages[1], 1)
	case gen.LogLevelInfo:
		atomic.AddUint64(&n.logMessages[2], 1)
	case gen.LogLevelWarning:
		atomic.AddUint64(&n.logMessages[3], 1)
	case gen.LogLevelError:
		atomic.AddUint64(&n.logMessages[4], 1)
	case gen.LogLevelPanic:
		atomic.AddUint64(&n.logMessages[5], 1)
	}

	if l := n.loggers[message.Level]; l != nil {
		if loggername != "" {
			if v, found := l.Load(loggername); found {
				v.(gen.LoggerBehavior).Log(message)
			}
			return
		}

		l.Range(func(k, v any) bool {
			logger := k.(string)
			if logger[0] != '.' {
				v.(gen.LoggerBehavior).Log(message)
			}
			return true
		})
	}
}

func (n *node) SetCTRLC(enable bool) {

	if swapped := n.enableCTRLC.CompareAndSwap(!enable, enable); swapped == false {
		n.Log().Info("handling SIGTERM is already set: %t", enable)
		return
	}

	go func() {
		if n.enableCTRLC.Load() == false {
			if n.ctrlc != nil {
				close(n.ctrlc)
				n.ctrlc = nil
			}
			return
		}

		n.ctrlc = make(chan os.Signal, 1)
		signal.Notify(n.ctrlc, os.Interrupt, syscall.SIGTERM)

		if sig := <-n.ctrlc; sig == nil {
			// closed channel. shutdown is in progress already
			return
		}

		signal.Reset()

		n.Log().Info("node %s is starting a graceful shutdown...", n.name)
		n.Stop()
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
		atomic.AddUint64(&n.processesSpawnFailed, 1)
		return empty, gen.ErrIncorrect
	}

	if options.ParentPID == empty || options.ParentLeader == empty {
		atomic.AddUint64(&n.processesSpawnFailed, 1)
		return empty, gen.ErrParentUnknown
	}

	now := time.Now()
	p := &process{
		node:         n,
		core:         n.core,
		response:     make(chan response, 10),
		creation:     now.Unix(),
		state:        int32(gen.ProcessStateInit),
		stateEntered: now.UnixNano(),
		parent:       options.ParentPID,
		leader:       options.ParentLeader,
	}
	p.keeporder.Store(true)
	p.important.Store(options.ImportantDelivery)
	p.tracingAttrs.Store(new([]gen.TracingAttribute))

	if options.Application != "" {
		if v, ok := n.applications.Load(options.Application); ok {
			p.application = v.(*application)
		}
	}

	// init mailbox before publishing the name: RouteSend* finds the process by
	// name and reads p.mailbox, so it must be ready before the LoadOrStore below
	if options.Mailbox != nil {
		// adopt the mailbox handed in by the supervisor (or test) instead
		// of allocating fresh queues
		p.mailbox = *options.Mailbox
		p.fallback = options.Fallback
	} else if options.MailboxSize > 0 {
		p.fallback = options.Fallback
		p.mailbox.Main = lib.NewQueueLimitMPSC(options.MailboxSize)
		p.mailbox.System = lib.NewQueueLimitMPSC(options.MailboxSize)
		p.mailbox.Urgent = lib.NewQueueLimitMPSC(options.MailboxSize)
		p.mailbox.Log = lib.NewQueueLimitMPSC(options.MailboxSize)
	} else {
		p.mailbox.Main = lib.NewQueueMPSC()
		p.mailbox.System = lib.NewQueueMPSC()
		p.mailbox.Urgent = lib.NewQueueMPSC()
		p.mailbox.Log = lib.NewQueueMPSC()
	}

	p.preserveMailbox = options.PreserveMailbox

	if options.Register != "" {
		if _, exist := n.names.LoadOrStore(options.Register, p); exist {
			atomic.AddUint64(&n.processesSpawnFailed, 1)
			return p.pid, gen.ErrTaken
		}
		p.name = options.Register
		p.registered.Store(true)
	}

	// create pid
	pid := gen.PID{
		Node:     n.name,
		ID:       atomic.AddUint64(&n.nextID, 1),
		Creation: atomic.LoadInt64(&n.creation),
	}
	p.pid = pid

	for k, v := range options.ParentEnv {
		p.SetEnv(k, v)
	}
	if lib.Verbose() {
		n.log.Trace(
			"...spawn new process %s (parent %s, %s) using %#v",
			p.pid,
			p.parent,
			p.name,
			factory,
		)
	}

	for k, v := range options.Env {
		p.SetEnv(k, v)
	}

	if options.Leader != empty {
		p.leader = options.Leader
	}

	compression := options.Compression
	if compression.Level == 0 {
		compression.Level = gen.DefaultCompressionLevel
	}
	if compression.Type == "" {
		compression.Type = gen.DefaultCompressionType
	}
	if compression.Threshold == 0 {
		compression.Threshold = gen.DefaultCompressionThreshold
	}
	p.compression.Store(&compression)

	switch options.SendPriority {
	case gen.MessagePriorityHigh:
		p.priority.Store(int32(gen.MessagePriorityHigh))
	case gen.MessagePriorityMax:
		p.priority.Store(int32(gen.MessagePriorityMax))
	default:
		p.priority.Store(int32(gen.MessagePriorityNormal))
	}

	// create a new process with provided behavior
	behavior := factory()
	if behavior == nil {
		n.names.Delete(p.name)
		atomic.AddUint64(&n.processesSpawnFailed, 1)
		return p.pid, errors.New("factory function must return non nil value")
	}
	p.behavior = behavior
	p.sbehavior = strings.TrimPrefix(reflect.TypeOf(behavior).String(), "*")
	p.kind = behavior.ProcessKind()

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

	// early registration - allows using Link/Monitor/RegisterEvent/RegisterName in Init
	n.processes.Store(p.pid, p)

	tracingActive := options.Tracing.ID != [2]uint64{}
	var spawnSpanID uint64
	var parentSpanID uint64
	if tracingActive {
		parentSpanID = options.Tracing.SpanID
		spawnSpanID = atomic.AddUint64(&n.spanID, 1)
		n.sendTracingSpan(gen.TracingSpan{
			TraceID: options.Tracing.ID, SpanID: spawnSpanID,
			ParentSpanID: parentSpanID,
			Point:        gen.TracingPointSent, Kind: gen.TracingKindSpawn,
			Timestamp: time.Now().UnixNano(),
			Node:      n.name, From: options.ParentPID, To: pid,
			Behavior: options.Tracing.Behavior,
		})
	}

	// Handle ProcessInit with timeout
	var initErr error
	deadline := options.Ref.ID[2]

	// the behavior gets the process directly, or a decorator (testing/stage)
	bp := gen.Process(p)
	if n.wrapProcess != nil {
		bp = n.wrapProcess(p)
	}

	if deadline > 0 {
		// check if already expired
		if options.Ref.IsAlive() == false {
			n.processes.Delete(p.pid)
			if p.registered.Load() {
				n.names.Delete(p.name)
			}
			atomic.AddUint64(&n.processesSpawnFailed, 1)
			return p.pid, gen.ErrTimeout
		}

		// calculate remaining time
		remaining := time.Duration(int64(deadline)-time.Now().Unix()) * time.Second

		var completed int32
		errCh := make(chan error, 1)

		go func() {
			initStart := time.Now()
			err := behavior.ProcessInit(bp, options.Args...)
			atomic.StoreUint64(&p.initTime, uint64(time.Since(initStart)))

			// try to claim "init completed"
			if atomic.CompareAndSwapInt32(&completed, 0, 1) {
				// we won - main will receive result
				errCh <- err
				return
			}

			// timeout won - main already called Kill, we do cleanup
			atomic.StoreInt32(&p.state, int32(gen.ProcessStateTerminated))
			atomic.StoreInt64(&p.stateEntered, time.Now().UnixNano())
			n.cleanupProcess(p, gen.TerminateReasonKill)
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

		timer := lib.TakeTimer()
		timer.Reset(remaining)

		select {
		case initErr = <-errCh:
			lib.ReleaseTimer(timer)
		case <-timer.C:
			lib.ReleaseTimer(timer)
			// try to claim "timeout"
			if atomic.CompareAndSwapInt32(&completed, 0, 2) {
				// we won - goroutine will do cleanup when ProcessInit completes
				n.Kill(p.pid)
				atomic.AddUint64(&n.processesSpawnFailed, 1)
				return p.pid, gen.ErrTimeout
			}
			// goroutine won - receive result
			initErr = <-errCh
		}
	} else {
		// no timeout - synchronous behavior
		initStart := time.Now()
		initErr = behavior.ProcessInit(bp, options.Args...)
		atomic.StoreUint64(&p.initTime, uint64(time.Since(initStart)))
	}

	if initErr != nil {
		if tracingActive {
			n.sendTracingSpan(gen.TracingSpan{
				TraceID: options.Tracing.ID, SpanID: spawnSpanID,
				ParentSpanID: parentSpanID,
				Point:        gen.TracingPointProcessed, Kind: gen.TracingKindSpawn,
				Timestamp: time.Now().UnixNano(),
				Node:      n.name, From: options.ParentPID, To: pid,
				Behavior: p.sbehavior, Error: initErr.Error(),
			})
		}
		n.cleanupProcess(p, initErr)
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
			p.behavior.ProcessTerminate(initErr)
		}()
		atomic.AddUint64(&n.processesSpawnFailed, 1)
		return p.pid, initErr
	}

	if tracingActive {
		n.sendTracingSpan(gen.TracingSpan{
			TraceID: options.Tracing.ID, SpanID: spawnSpanID,
			ParentSpanID: parentSpanID,
			Point:        gen.TracingPointProcessed, Kind: gen.TracingKindSpawn,
			Timestamp: time.Now().UnixNano(),
			Node:      n.name, From: options.ParentPID, To: pid,
			Behavior: p.sbehavior,
		})
	}

	if options.LinkParent {
		n.targets.LinkPID(p.pid, p.parent)
	}

	// switch to sleep state (process already registered above)
	atomic.StoreInt32(&p.state, int32(gen.ProcessStateSleep))
	atomic.StoreInt64(&p.stateEntered, time.Now().UnixNano())

	// do not count system app processes
	if appName(p.application) != system.Name {
		n.waitprocesses.Add(1)
	}

	// register a direct application group member before it can run (and terminate)
	if options.ApplicationGroupMember {
		if app, ok := p.application.(*application); ok {
			app.group.Store(p.pid, true)
		}
	}

	// process could send a message to itself during initialization
	// so we should run this process to make sure this message is handled
	p.run()

	atomic.AddUint64(&n.processesSpawned, 1)
	return p.pid, nil
}

// cleanupProcess performs core cleanup for a process.
// Does NOT call waitprocesses.Done() - caller must handle if needed.
func (n *node) cleanupProcess(p *process, reason error) {
	if p.tracing.ID != [2]uint64{} {
		var errString string
		if reason != nil {
			errString = reason.Error()
		}
		n.sendTracingSpan(gen.TracingSpan{
			TraceID: p.tracing.ID, SpanID: atomic.AddUint64(&n.spanID, 1),
			ParentSpanID: p.tracing.SpanID,
			Point:        gen.TracingPointProcessed, Kind: gen.TracingKindTerminate,
			Timestamp: time.Now().UnixNano(),
			Node:      n.name, From: p.pid, To: p.pid,
			Behavior: p.sbehavior, Error: errString,
		})
	}
	n.processes.Delete(p.pid)

	if name := p.tracingExporterName.Load(); name != nil {
		n.TracingExporterDelete(*name)
		p.tracingExporterName.Store(nil)
	}

	n.log.Trace("...cleanupProcess %s", p.pid)

	if p.registered.Load() {
		n.names.Delete(p.name)
		pname := gen.ProcessID{Name: p.name, Node: n.name}
		n.RouteTerminateProcessID(pname, reason)
	}

	for _, a := range p.aliases {
		n.aliases.Delete(a)
		n.RouteTerminateAlias(a, reason)
	}

	n.RouteTerminatePID(p.pid, reason) // calls TerminatedTargetPID internally
	n.targets.TerminatedProcess(p.pid, reason)

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
}

func (n *node) unregisterProcess(p *process, reason error) {
	n.cleanupProcess(p, reason)

	atomic.AddUint64(&n.processesTerminated, 1)

	if appName(p.application) != system.Name {
		n.waitprocesses.Done()
	}
	n.log.Trace("...unregisterProcess %s", p.pid)

	if p.loggername != "" {
		n.LoggerDelete(p.loggername)
		p.log.SetLevel(gen.LogLevelInfo)
	}

	if p.application != nil {
		if app, ok := p.application.(*application); ok {
			app.terminate(p.pid, reason)
		}
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

func (n *node) registerEvent(
	name gen.Atom,
	owner gen.PID,
	options gen.EventOptions,
) (gen.Ref, error) {
	if n.isRunning() == false {
		return gen.Ref{}, gen.ErrNodeTerminated
	}

	n.log.Trace("...registerEvent %s for %s", name, owner)

	return n.targets.RegisterEvent(owner, name, options)
}

func (n *node) unregisterEvent(name gen.Atom, pid gen.PID) error {
	if n.isRunning() == false {
		return gen.ErrNodeTerminated
	}

	n.log.Trace("...unregisterEvent %s for %s", name, pid)

	return n.targets.UnregisterEvent(pid, name)
}
