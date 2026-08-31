package mock

import (
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// Node is a standalone gen.Node mock. Every method has an On<Method> override;
// unset, egress methods record into the shared sink and return a synthetic success
// value, while query/accessor methods return safe zero values or no-op. The eager
// sub-mocks (Log/Network/Cron) share this node's recorder so all records collate
// into one ordered stream.
type Node struct {
	recorder
	pid     gen.PID
	next    atomic.Uint64
	log     *Log
	network *Network
	cron    *Cron
	ov      nodeOverrides
}

type nodeOverrides struct {
	name                            func() gen.Atom
	isAlive                         func() bool
	uptime                          func() int64
	version                         func() gen.Version
	frameworkVersion                func() gen.Version
	info                            func() (gen.NodeInfo, error)
	shortInfo                       func() (gen.NodeShortInfo, error)
	envList                         func() map[gen.Env]any
	setEnv                          func(name gen.Env, value any)
	env                             func(name gen.Env) (any, bool)
	envDefault                      func(name gen.Env, def any) any
	spawn                           func(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error)
	spawnRegister                   func(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error)
	registerName                    func(name gen.Atom, pid gen.PID) error
	unregisterName                  func(name gen.Atom) (gen.PID, error)
	metaInfo                        func(meta gen.Alias) (gen.MetaInfo, error)
	processInfo                     func(pid gen.PID) (gen.ProcessInfo, error)
	processList                     func() ([]gen.PID, error)
	processListShortInfo            func(start, limit int, filter ...func(gen.ProcessShortInfo) bool) ([]gen.ProcessShortInfo, error)
	processRangeShortInfo           func(fn func(gen.ProcessShortInfo) bool) error
	processName                     func(pid gen.PID) (gen.Atom, error)
	processPID                      func(name gen.Atom) (gen.PID, error)
	processState                    func(pid gen.PID) (gen.ProcessState, error)
	applicationLoad                 func(app gen.ApplicationBehavior, args ...any) (gen.Atom, error)
	applicationInfo                 func(name gen.Atom) (gen.ApplicationInfo, error)
	applicationProcessList          func(name gen.Atom, limit int) ([]gen.PID, error)
	applicationProcessListShortInfo func(name gen.Atom, limit int) ([]gen.ProcessShortInfo, int, error)
	applicationUnload               func(name gen.Atom) error
	applicationStart                func(name gen.Atom, options gen.ApplicationOptions) error
	applicationStartTemporary       func(name gen.Atom, options gen.ApplicationOptions) error
	applicationStartTransient       func(name gen.Atom, options gen.ApplicationOptions) error
	applicationStartPermanent       func(name gen.Atom, options gen.ApplicationOptions) error
	applicationStop                 func(name gen.Atom) error
	applicationStopForce            func(name gen.Atom) error
	applicationStopWithTimeout      func(name gen.Atom, timeout time.Duration) error
	applications                    func() []gen.Atom
	applicationsRunning             func() []gen.Atom
	networkStart                    func(options gen.NetworkOptions) error
	networkStop                     func() error
	networkFn                       func() gen.Network
	cronFn                          func() gen.Cron
	certManager                     func() gen.CertManager
	security                        func() gen.SecurityOptions
	stop                            func()
	stopWithTimeout                 func(timeout time.Duration)
	stopForce                       func()
	wait                            func()
	waitWithTimeout                 func(timeout time.Duration) error
	kill                            func(pid gen.PID) error
	send                            func(to any, message any) error
	sendWithPriority                func(to any, message any, priority gen.MessagePriority) error
	sendEvent                       func(name gen.Atom, token gen.Ref, options gen.MessageOptions, message any) error
	registerEvent                   func(name gen.Atom, options gen.EventOptions) (gen.Ref, error)
	unregisterEvent                 func(name gen.Atom) error
	eventInfo                       func(event gen.Event) (gen.EventInfo, error)
	eventRangeInfo                  func(fn func(gen.EventInfo) bool) error
	eventListInfo                   func(timestamp int64, limit int, filter ...func(gen.EventInfo) bool) ([]gen.EventInfo, error)
	sendExit                        func(pid gen.PID, reason error) error
	call                            func(to any, request any) (any, error)
	callWithTimeout                 func(to any, request any, timeout int) (any, error)
	callWithPriority                func(to any, request any, priority gen.MessagePriority) (any, error)
	callImportant                   func(to any, request any) (any, error)
	callPID                         func(to gen.PID, request any, timeout int) (any, error)
	callProcessID                   func(to gen.ProcessID, request any, timeout int) (any, error)
	callAlias                       func(to gen.Alias, request any, timeout int) (any, error)
	inspect                         func(target gen.PID, item ...string) (map[string]string, error)
	inspectMeta                     func(alias gen.Alias, item ...string) (map[string]string, error)
	logFn                           func() gen.Log
	setProcessLogLevel              func(pid gen.PID, level gen.LogLevel) error
	setProcessSendPriority          func(pid gen.PID, priority gen.MessagePriority) error
	setProcessCompression           func(pid gen.PID, enabled bool) error
	setProcessCompressionType       func(pid gen.PID, ctype gen.CompressionType) error
	setProcessCompressionLevel      func(pid gen.PID, level gen.CompressionLevel) error
	setProcessCompressionThreshold  func(pid gen.PID, threshold int) error
	setProcessKeepNetworkOrder      func(pid gen.PID, order bool) error
	setProcessImportantDelivery     func(pid gen.PID, important bool) error
	setMetaLogLevel                 func(meta gen.Alias, level gen.LogLevel) error
	setMetaSendPriority             func(meta gen.Alias, priority gen.MessagePriority) error
	loggers                         func() []string
	loggerAddPID                    func(pid gen.PID, name string, filter ...gen.LogLevel) error
	loggerAdd                       func(name string, logger gen.LoggerBehavior, filter ...gen.LogLevel) error
	loggerDeletePID                 func(pid gen.PID)
	loggerDelete                    func(name string)
	loggerLevels                    func(name string) []gen.LogLevel
	tracingExporterAddPID           func(pid gen.PID, name string, flags gen.TracingFlags) error
	tracingExporterAdd              func(name string, exporter gen.TracingBehavior, flags gen.TracingFlags) error
	tracingExporterDeletePID        func(pid gen.PID)
	tracingExporterDelete           func(name string)
	tracingExporters                func() []string
	tracingExporterFlags            func(name string) gen.TracingFlags
	setTracingSampler               func(sampler gen.TracingSampler) error
	setTracingAttribute             func(key, value string)
	removeTracingAttribute          func(key string)
	tracingSampler                  func() gen.TracingSampler
	setProcessTracingSampler        func(pid gen.PID, sampler gen.TracingSampler) error
	makeRef                         func() gen.Ref
	makeRefWithDeadline             func(deadline int64) (gen.Ref, error)
	commercial                      func() []gen.Version
	pidFn                           func() gen.PID
	creation                        func() int64
	peers                           func() []gen.Atom
	setCTRLC                        func(enable bool)
}

var (
	_ gen.Node          = (*Node)(nil)
	_ gen.NodeRegistrar = (*Node)(nil)
	_ gen.NodeHandshake = (*Node)(nil)
)

// NewNode returns a dumb gen.Node mock (no recording; use NewNodeT for Should*).
func NewNode() *Node { return newNode(recorder{}) }

// NewNodeT returns a gen.Node mock that records egress into a shared recorder and
// asserts through t. Its sub-mocks (Log/Network/Cron) share the same recorder.
func NewNodeT(t check.T) *Node { return newNode(newRecorder(t)) }

func newNode(r recorder) *Node {
	n := &Node{recorder: r, pid: synthPID(1)}
	n.next.Store(1)
	n.log = newLog(r)
	n.network = newNetwork(r)
	n.cron = newCron(r)
	return n
}

// On<Method> overrides

func (n *Node) OnName(fn func() gen.Atom)                        { n.ov.name = fn }
func (n *Node) OnIsAlive(fn func() bool)                         { n.ov.isAlive = fn }
func (n *Node) OnUptime(fn func() int64)                         { n.ov.uptime = fn }
func (n *Node) OnVersion(fn func() gen.Version)                  { n.ov.version = fn }
func (n *Node) OnFrameworkVersion(fn func() gen.Version)         { n.ov.frameworkVersion = fn }
func (n *Node) OnInfo(fn func() (gen.NodeInfo, error))           { n.ov.info = fn }
func (n *Node) OnShortInfo(fn func() (gen.NodeShortInfo, error)) { n.ov.shortInfo = fn }
func (n *Node) OnEnvList(fn func() map[gen.Env]any)              { n.ov.envList = fn }
func (n *Node) OnSetEnv(fn func(name gen.Env, value any))        { n.ov.setEnv = fn }
func (n *Node) OnEnv(fn func(name gen.Env) (any, bool))          { n.ov.env = fn }
func (n *Node) OnEnvDefault(fn func(name gen.Env, def any) any) {
	n.ov.envDefault = fn
}

func (n *Node) OnSpawn(fn func(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error)) {
	n.ov.spawn = fn
}

func (n *Node) OnSpawnRegister(fn func(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error)) {
	n.ov.spawnRegister = fn
}

func (n *Node) OnRegisterName(fn func(name gen.Atom, pid gen.PID) error) {
	n.ov.registerName = fn
}

func (n *Node) OnUnregisterName(fn func(name gen.Atom) (gen.PID, error)) {
	n.ov.unregisterName = fn
}

func (n *Node) OnMetaInfo(fn func(meta gen.Alias) (gen.MetaInfo, error)) {
	n.ov.metaInfo = fn
}

func (n *Node) OnProcessInfo(fn func(pid gen.PID) (gen.ProcessInfo, error)) {
	n.ov.processInfo = fn
}

func (n *Node) OnProcessList(fn func() ([]gen.PID, error)) { n.ov.processList = fn }

func (n *Node) OnProcessListShortInfo(fn func(start, limit int, filter ...func(gen.ProcessShortInfo) bool) ([]gen.ProcessShortInfo, error)) {
	n.ov.processListShortInfo = fn
}

func (n *Node) OnProcessRangeShortInfo(fn func(fn func(gen.ProcessShortInfo) bool) error) {
	n.ov.processRangeShortInfo = fn
}

func (n *Node) OnProcessName(fn func(pid gen.PID) (gen.Atom, error)) {
	n.ov.processName = fn
}

func (n *Node) OnProcessPID(fn func(name gen.Atom) (gen.PID, error)) {
	n.ov.processPID = fn
}

func (n *Node) OnProcessState(fn func(pid gen.PID) (gen.ProcessState, error)) {
	n.ov.processState = fn
}

func (n *Node) OnApplicationLoad(fn func(app gen.ApplicationBehavior, args ...any) (gen.Atom, error)) {
	n.ov.applicationLoad = fn
}

func (n *Node) OnApplicationInfo(fn func(name gen.Atom) (gen.ApplicationInfo, error)) {
	n.ov.applicationInfo = fn
}

func (n *Node) OnApplicationProcessList(fn func(name gen.Atom, limit int) ([]gen.PID, error)) {
	n.ov.applicationProcessList = fn
}

func (n *Node) OnApplicationProcessListShortInfo(fn func(name gen.Atom, limit int) ([]gen.ProcessShortInfo, int, error)) {
	n.ov.applicationProcessListShortInfo = fn
}

func (n *Node) OnApplicationUnload(fn func(name gen.Atom) error) {
	n.ov.applicationUnload = fn
}

func (n *Node) OnApplicationStart(fn func(name gen.Atom, options gen.ApplicationOptions) error) {
	n.ov.applicationStart = fn
}

func (n *Node) OnApplicationStartTemporary(fn func(name gen.Atom, options gen.ApplicationOptions) error) {
	n.ov.applicationStartTemporary = fn
}

func (n *Node) OnApplicationStartTransient(fn func(name gen.Atom, options gen.ApplicationOptions) error) {
	n.ov.applicationStartTransient = fn
}

func (n *Node) OnApplicationStartPermanent(fn func(name gen.Atom, options gen.ApplicationOptions) error) {
	n.ov.applicationStartPermanent = fn
}

func (n *Node) OnApplicationStop(fn func(name gen.Atom) error) {
	n.ov.applicationStop = fn
}

func (n *Node) OnApplicationStopForce(fn func(name gen.Atom) error) {
	n.ov.applicationStopForce = fn
}

func (n *Node) OnApplicationStopWithTimeout(fn func(name gen.Atom, timeout time.Duration) error) {
	n.ov.applicationStopWithTimeout = fn
}

func (n *Node) OnApplications(fn func() []gen.Atom)        { n.ov.applications = fn }
func (n *Node) OnApplicationsRunning(fn func() []gen.Atom) { n.ov.applicationsRunning = fn }

func (n *Node) OnNetworkStart(fn func(options gen.NetworkOptions) error) {
	n.ov.networkStart = fn
}

func (n *Node) OnNetworkStop(fn func() error)   { n.ov.networkStop = fn }
func (n *Node) OnNetwork(fn func() gen.Network) { n.ov.networkFn = fn }
func (n *Node) OnCron(fn func() gen.Cron)       { n.ov.cronFn = fn }
func (n *Node) OnCertManager(fn func() gen.CertManager) {
	n.ov.certManager = fn
}
func (n *Node) OnSecurity(fn func() gen.SecurityOptions) { n.ov.security = fn }
func (n *Node) OnStop(fn func())                         { n.ov.stop = fn }
func (n *Node) OnStopWithTimeout(fn func(timeout time.Duration)) {
	n.ov.stopWithTimeout = fn
}
func (n *Node) OnStopForce(fn func()) { n.ov.stopForce = fn }
func (n *Node) OnWait(fn func())      { n.ov.wait = fn }
func (n *Node) OnWaitWithTimeout(fn func(timeout time.Duration) error) {
	n.ov.waitWithTimeout = fn
}
func (n *Node) OnKill(fn func(pid gen.PID) error)         { n.ov.kill = fn }
func (n *Node) OnSend(fn func(to any, message any) error) { n.ov.send = fn }

func (n *Node) OnSendWithPriority(fn func(to any, message any, priority gen.MessagePriority) error) {
	n.ov.sendWithPriority = fn
}

func (n *Node) OnSendEvent(fn func(name gen.Atom, token gen.Ref, options gen.MessageOptions, message any) error) {
	n.ov.sendEvent = fn
}

func (n *Node) OnRegisterEvent(fn func(name gen.Atom, options gen.EventOptions) (gen.Ref, error)) {
	n.ov.registerEvent = fn
}

func (n *Node) OnUnregisterEvent(fn func(name gen.Atom) error) {
	n.ov.unregisterEvent = fn
}

func (n *Node) OnEventInfo(fn func(event gen.Event) (gen.EventInfo, error)) {
	n.ov.eventInfo = fn
}

func (n *Node) OnEventRangeInfo(fn func(fn func(gen.EventInfo) bool) error) {
	n.ov.eventRangeInfo = fn
}

func (n *Node) OnEventListInfo(fn func(timestamp int64, limit int, filter ...func(gen.EventInfo) bool) ([]gen.EventInfo, error)) {
	n.ov.eventListInfo = fn
}

func (n *Node) OnSendExit(fn func(pid gen.PID, reason error) error) {
	n.ov.sendExit = fn
}

func (n *Node) OnCall(fn func(to any, request any) (any, error)) { n.ov.call = fn }

func (n *Node) OnCallWithTimeout(fn func(to any, request any, timeout int) (any, error)) {
	n.ov.callWithTimeout = fn
}

func (n *Node) OnCallWithPriority(fn func(to any, request any, priority gen.MessagePriority) (any, error)) {
	n.ov.callWithPriority = fn
}

func (n *Node) OnCallImportant(fn func(to any, request any) (any, error)) {
	n.ov.callImportant = fn
}

func (n *Node) OnCallPID(fn func(to gen.PID, request any, timeout int) (any, error)) {
	n.ov.callPID = fn
}

func (n *Node) OnCallProcessID(fn func(to gen.ProcessID, request any, timeout int) (any, error)) {
	n.ov.callProcessID = fn
}

func (n *Node) OnCallAlias(fn func(to gen.Alias, request any, timeout int) (any, error)) {
	n.ov.callAlias = fn
}

func (n *Node) OnInspect(fn func(target gen.PID, item ...string) (map[string]string, error)) {
	n.ov.inspect = fn
}

func (n *Node) OnInspectMeta(fn func(alias gen.Alias, item ...string) (map[string]string, error)) {
	n.ov.inspectMeta = fn
}

func (n *Node) OnLog(fn func() gen.Log) { n.ov.logFn = fn }

func (n *Node) OnSetProcessLogLevel(fn func(pid gen.PID, level gen.LogLevel) error) {
	n.ov.setProcessLogLevel = fn
}

func (n *Node) OnSetProcessSendPriority(fn func(pid gen.PID, priority gen.MessagePriority) error) {
	n.ov.setProcessSendPriority = fn
}

func (n *Node) OnSetProcessCompression(fn func(pid gen.PID, enabled bool) error) {
	n.ov.setProcessCompression = fn
}

func (n *Node) OnSetProcessCompressionType(fn func(pid gen.PID, ctype gen.CompressionType) error) {
	n.ov.setProcessCompressionType = fn
}

func (n *Node) OnSetProcessCompressionLevel(fn func(pid gen.PID, level gen.CompressionLevel) error) {
	n.ov.setProcessCompressionLevel = fn
}

func (n *Node) OnSetProcessCompressionThreshold(fn func(pid gen.PID, threshold int) error) {
	n.ov.setProcessCompressionThreshold = fn
}

func (n *Node) OnSetProcessKeepNetworkOrder(fn func(pid gen.PID, order bool) error) {
	n.ov.setProcessKeepNetworkOrder = fn
}

func (n *Node) OnSetProcessImportantDelivery(fn func(pid gen.PID, important bool) error) {
	n.ov.setProcessImportantDelivery = fn
}

func (n *Node) OnSetMetaLogLevel(fn func(meta gen.Alias, level gen.LogLevel) error) {
	n.ov.setMetaLogLevel = fn
}

func (n *Node) OnSetMetaSendPriority(fn func(meta gen.Alias, priority gen.MessagePriority) error) {
	n.ov.setMetaSendPriority = fn
}

func (n *Node) OnLoggers(fn func() []string) { n.ov.loggers = fn }

func (n *Node) OnLoggerAddPID(fn func(pid gen.PID, name string, filter ...gen.LogLevel) error) {
	n.ov.loggerAddPID = fn
}

func (n *Node) OnLoggerAdd(fn func(name string, logger gen.LoggerBehavior, filter ...gen.LogLevel) error) {
	n.ov.loggerAdd = fn
}

func (n *Node) OnLoggerDeletePID(fn func(pid gen.PID)) { n.ov.loggerDeletePID = fn }
func (n *Node) OnLoggerDelete(fn func(name string))    { n.ov.loggerDelete = fn }

func (n *Node) OnLoggerLevels(fn func(name string) []gen.LogLevel) {
	n.ov.loggerLevels = fn
}

func (n *Node) OnTracingExporterAddPID(fn func(pid gen.PID, name string, flags gen.TracingFlags) error) {
	n.ov.tracingExporterAddPID = fn
}

func (n *Node) OnTracingExporterAdd(fn func(name string, exporter gen.TracingBehavior, flags gen.TracingFlags) error) {
	n.ov.tracingExporterAdd = fn
}

func (n *Node) OnTracingExporterDeletePID(fn func(pid gen.PID)) {
	n.ov.tracingExporterDeletePID = fn
}

func (n *Node) OnTracingExporterDelete(fn func(name string)) {
	n.ov.tracingExporterDelete = fn
}

func (n *Node) OnTracingExporters(fn func() []string) { n.ov.tracingExporters = fn }

func (n *Node) OnTracingExporterFlags(fn func(name string) gen.TracingFlags) {
	n.ov.tracingExporterFlags = fn
}

func (n *Node) OnSetTracingSampler(fn func(sampler gen.TracingSampler) error) {
	n.ov.setTracingSampler = fn
}

func (n *Node) OnSetTracingAttribute(fn func(key, value string)) {
	n.ov.setTracingAttribute = fn
}

func (n *Node) OnRemoveTracingAttribute(fn func(key string)) {
	n.ov.removeTracingAttribute = fn
}

func (n *Node) OnTracingSampler(fn func() gen.TracingSampler) {
	n.ov.tracingSampler = fn
}

func (n *Node) OnSetProcessTracingSampler(fn func(pid gen.PID, sampler gen.TracingSampler) error) {
	n.ov.setProcessTracingSampler = fn
}

func (n *Node) OnMakeRef(fn func() gen.Ref) { n.ov.makeRef = fn }

func (n *Node) OnMakeRefWithDeadline(fn func(deadline int64) (gen.Ref, error)) {
	n.ov.makeRefWithDeadline = fn
}

func (n *Node) OnCommercial(fn func() []gen.Version) { n.ov.commercial = fn }
func (n *Node) OnPID(fn func() gen.PID)              { n.ov.pidFn = fn }
func (n *Node) OnCreation(fn func() int64)           { n.ov.creation = fn }
func (n *Node) OnPeers(fn func() []gen.Atom)         { n.ov.peers = fn }
func (n *Node) OnSetCTRLC(fn func(enable bool))      { n.ov.setCTRLC = fn }

// gen.Node

func (n *Node) Name() gen.Atom {
	if n.ov.name != nil {
		return n.ov.name()
	}
	return mockNode
}

func (n *Node) IsAlive() bool {
	if n.ov.isAlive != nil {
		return n.ov.isAlive()
	}
	return true
}

func (n *Node) Uptime() int64 {
	if n.ov.uptime != nil {
		return n.ov.uptime()
	}
	return 0
}

func (n *Node) Version() gen.Version {
	if n.ov.version != nil {
		return n.ov.version()
	}
	return gen.Version{}
}

func (n *Node) FrameworkVersion() gen.Version {
	if n.ov.frameworkVersion != nil {
		return n.ov.frameworkVersion()
	}
	return gen.Version{}
}

func (n *Node) Info() (gen.NodeInfo, error) {
	if n.ov.info != nil {
		return n.ov.info()
	}
	return gen.NodeInfo{}, nil
}

func (n *Node) ShortInfo() (gen.NodeShortInfo, error) {
	if n.ov.shortInfo != nil {
		return n.ov.shortInfo()
	}
	return gen.NodeShortInfo{}, nil
}

func (n *Node) EnvList() map[gen.Env]any {
	if n.ov.envList != nil {
		return n.ov.envList()
	}
	return nil
}

func (n *Node) SetEnv(name gen.Env, value any) {
	if n.ov.setEnv != nil {
		n.ov.setEnv(name, value)
	}
}

func (n *Node) Env(name gen.Env) (any, bool) {
	if n.ov.env != nil {
		return n.ov.env(name)
	}
	return nil, false
}

func (n *Node) EnvDefault(name gen.Env, def any) any {
	if n.ov.envDefault != nil {
		return n.ov.envDefault(name, def)
	}
	return def
}

func (n *Node) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	child, err := synthPID(n.next.Add(1)), error(nil)
	if n.ov.spawn != nil {
		child, err = n.ov.spawn(factory, options, args...)
	}
	n.put(check.Spawn{Parent: n.pid, Child: child, Factory: factory, Options: options, Error: err})
	return child, err
}

func (n *Node) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	child, err := synthPID(n.next.Add(1)), error(nil)
	if n.ov.spawnRegister != nil {
		child, err = n.ov.spawnRegister(register, factory, options, args...)
	}
	n.put(check.Spawn{Parent: n.pid, Child: child, Register: register, Factory: factory, Options: options, Error: err})
	return child, err
}

func (n *Node) RegisterName(name gen.Atom, pid gen.PID) error {
	if n.ov.registerName != nil {
		return n.ov.registerName(name, pid)
	}
	return nil
}

func (n *Node) UnregisterName(name gen.Atom) (gen.PID, error) {
	if n.ov.unregisterName != nil {
		return n.ov.unregisterName(name)
	}
	return gen.PID{}, nil
}

func (n *Node) MetaInfo(meta gen.Alias) (gen.MetaInfo, error) {
	if n.ov.metaInfo != nil {
		return n.ov.metaInfo(meta)
	}
	return gen.MetaInfo{}, nil
}

func (n *Node) ProcessInfo(pid gen.PID) (gen.ProcessInfo, error) {
	if n.ov.processInfo != nil {
		return n.ov.processInfo(pid)
	}
	return gen.ProcessInfo{}, nil
}

func (n *Node) ProcessList() ([]gen.PID, error) {
	if n.ov.processList != nil {
		return n.ov.processList()
	}
	return nil, nil
}

func (n *Node) ProcessListShortInfo(start, limit int, filter ...func(gen.ProcessShortInfo) bool) ([]gen.ProcessShortInfo, error) {
	if n.ov.processListShortInfo != nil {
		return n.ov.processListShortInfo(start, limit, filter...)
	}
	return nil, nil
}

func (n *Node) ProcessRangeShortInfo(fn func(gen.ProcessShortInfo) bool) error {
	if n.ov.processRangeShortInfo != nil {
		return n.ov.processRangeShortInfo(fn)
	}
	return nil
}

func (n *Node) ProcessName(pid gen.PID) (gen.Atom, error) {
	if n.ov.processName != nil {
		return n.ov.processName(pid)
	}
	return "", nil
}

func (n *Node) ProcessPID(name gen.Atom) (gen.PID, error) {
	if n.ov.processPID != nil {
		return n.ov.processPID(name)
	}
	return gen.PID{}, nil
}

func (n *Node) ProcessState(pid gen.PID) (gen.ProcessState, error) {
	if n.ov.processState != nil {
		return n.ov.processState(pid)
	}
	return 0, nil
}

func (n *Node) ApplicationLoad(app gen.ApplicationBehavior, args ...any) (gen.Atom, error) {
	if n.ov.applicationLoad != nil {
		return n.ov.applicationLoad(app, args...)
	}
	return "", nil
}

func (n *Node) ApplicationInfo(name gen.Atom) (gen.ApplicationInfo, error) {
	if n.ov.applicationInfo != nil {
		return n.ov.applicationInfo(name)
	}
	return gen.ApplicationInfo{}, nil
}

func (n *Node) ApplicationProcessList(name gen.Atom, limit int) ([]gen.PID, error) {
	if n.ov.applicationProcessList != nil {
		return n.ov.applicationProcessList(name, limit)
	}
	return nil, nil
}

func (n *Node) ApplicationProcessListShortInfo(name gen.Atom, limit int) ([]gen.ProcessShortInfo, int, error) {
	if n.ov.applicationProcessListShortInfo != nil {
		return n.ov.applicationProcessListShortInfo(name, limit)
	}
	return nil, 0, nil
}

func (n *Node) ApplicationUnload(name gen.Atom) error {
	if n.ov.applicationUnload != nil {
		return n.ov.applicationUnload(name)
	}
	return nil
}

func (n *Node) ApplicationStart(name gen.Atom, options gen.ApplicationOptions) error {
	if n.ov.applicationStart != nil {
		return n.ov.applicationStart(name, options)
	}
	return nil
}

func (n *Node) ApplicationStartTemporary(name gen.Atom, options gen.ApplicationOptions) error {
	if n.ov.applicationStartTemporary != nil {
		return n.ov.applicationStartTemporary(name, options)
	}
	return nil
}

func (n *Node) ApplicationStartTransient(name gen.Atom, options gen.ApplicationOptions) error {
	if n.ov.applicationStartTransient != nil {
		return n.ov.applicationStartTransient(name, options)
	}
	return nil
}

func (n *Node) ApplicationStartPermanent(name gen.Atom, options gen.ApplicationOptions) error {
	if n.ov.applicationStartPermanent != nil {
		return n.ov.applicationStartPermanent(name, options)
	}
	return nil
}

func (n *Node) ApplicationStop(name gen.Atom) error {
	if n.ov.applicationStop != nil {
		return n.ov.applicationStop(name)
	}
	return nil
}

func (n *Node) ApplicationStopForce(name gen.Atom) error {
	if n.ov.applicationStopForce != nil {
		return n.ov.applicationStopForce(name)
	}
	return nil
}

func (n *Node) ApplicationStopWithTimeout(name gen.Atom, timeout time.Duration) error {
	if n.ov.applicationStopWithTimeout != nil {
		return n.ov.applicationStopWithTimeout(name, timeout)
	}
	return nil
}

func (n *Node) Applications() []gen.Atom {
	if n.ov.applications != nil {
		return n.ov.applications()
	}
	return nil
}

func (n *Node) ApplicationsRunning() []gen.Atom {
	if n.ov.applicationsRunning != nil {
		return n.ov.applicationsRunning()
	}
	return nil
}

func (n *Node) NetworkStart(options gen.NetworkOptions) error {
	if n.ov.networkStart != nil {
		return n.ov.networkStart(options)
	}
	return nil
}

func (n *Node) NetworkStop() error {
	if n.ov.networkStop != nil {
		return n.ov.networkStop()
	}
	return nil
}

func (n *Node) Network() gen.Network {
	if n.ov.networkFn != nil {
		return n.ov.networkFn()
	}
	return n.network
}

func (n *Node) Cron() gen.Cron {
	if n.ov.cronFn != nil {
		return n.ov.cronFn()
	}
	return n.cron
}

func (n *Node) CertManager() gen.CertManager {
	if n.ov.certManager != nil {
		return n.ov.certManager()
	}
	return nil
}

func (n *Node) Security() gen.SecurityOptions {
	if n.ov.security != nil {
		return n.ov.security()
	}
	return gen.SecurityOptions{}
}

func (n *Node) Stop() {
	if n.ov.stop != nil {
		n.ov.stop()
	}
}

func (n *Node) StopWithTimeout(timeout time.Duration) {
	if n.ov.stopWithTimeout != nil {
		n.ov.stopWithTimeout(timeout)
	}
}

func (n *Node) StopForce() {
	if n.ov.stopForce != nil {
		n.ov.stopForce()
	}
}

func (n *Node) Wait() {
	if n.ov.wait != nil {
		n.ov.wait()
	}
}

func (n *Node) WaitWithTimeout(timeout time.Duration) error {
	if n.ov.waitWithTimeout != nil {
		return n.ov.waitWithTimeout(timeout)
	}
	return nil
}

func (n *Node) Kill(pid gen.PID) error {
	if n.ov.kill != nil {
		return n.ov.kill(pid)
	}
	return nil
}

func (n *Node) Send(to any, message any) error {
	var err error
	if n.ov.send != nil {
		err = n.ov.send(to, message)
	}
	n.put(check.Send{From: n.pid, To: to, Message: message, Error: err})
	return err
}

func (n *Node) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	var err error
	if n.ov.sendWithPriority != nil {
		err = n.ov.sendWithPriority(to, message, priority)
	}
	n.put(check.Send{From: n.pid, To: to, Message: message, Options: gen.MessageOptions{Priority: priority}, Error: err})
	return err
}

func (n *Node) SendEvent(name gen.Atom, token gen.Ref, options gen.MessageOptions, message any) error {
	var err error
	if n.ov.sendEvent != nil {
		err = n.ov.sendEvent(name, token, options, message)
	}
	n.put(check.SendEvent{From: n.pid, Name: name, Token: token, Message: message, Options: options, Error: err})
	return err
}

func (n *Node) RegisterEvent(name gen.Atom, options gen.EventOptions) (gen.Ref, error) {
	ref, err := synthRef(n.next.Add(1)), error(nil)
	if n.ov.registerEvent != nil {
		ref, err = n.ov.registerEvent(name, options)
	}
	n.put(check.RegisterEvent{PID: n.pid, Name: name, Ref: ref, Options: options, Error: err})
	return ref, err
}

func (n *Node) UnregisterEvent(name gen.Atom) error {
	var err error
	if n.ov.unregisterEvent != nil {
		err = n.ov.unregisterEvent(name)
	}
	n.put(check.UnregisterEvent{PID: n.pid, Name: name, Error: err})
	return err
}

func (n *Node) EventInfo(event gen.Event) (gen.EventInfo, error) {
	if n.ov.eventInfo != nil {
		return n.ov.eventInfo(event)
	}
	return gen.EventInfo{}, nil
}

func (n *Node) EventRangeInfo(fn func(gen.EventInfo) bool) error {
	if n.ov.eventRangeInfo != nil {
		return n.ov.eventRangeInfo(fn)
	}
	return nil
}

func (n *Node) EventListInfo(timestamp int64, limit int, filter ...func(gen.EventInfo) bool) ([]gen.EventInfo, error) {
	if n.ov.eventListInfo != nil {
		return n.ov.eventListInfo(timestamp, limit, filter...)
	}
	return nil, nil
}

func (n *Node) SendExit(pid gen.PID, reason error) error {
	var err error
	if n.ov.sendExit != nil {
		err = n.ov.sendExit(pid, reason)
	}
	n.put(check.SendExit{From: n.pid, To: pid, Reason: reason, Error: err})
	return err
}

func (n *Node) Call(to any, request any) (any, error) {
	var resp any
	var err error
	if n.ov.call != nil {
		resp, err = n.ov.call(to, request)
	}
	n.put(check.Call{From: n.pid, To: to, Request: request, Response: resp, Error: err})
	return resp, err
}

func (n *Node) CallWithTimeout(to any, request any, timeout int) (any, error) {
	var resp any
	var err error
	if n.ov.callWithTimeout != nil {
		resp, err = n.ov.callWithTimeout(to, request, timeout)
	}
	n.put(check.Call{From: n.pid, To: to, Request: request, Response: resp, Error: err})
	return resp, err
}

func (n *Node) CallWithPriority(to any, request any, priority gen.MessagePriority) (any, error) {
	var resp any
	var err error
	if n.ov.callWithPriority != nil {
		resp, err = n.ov.callWithPriority(to, request, priority)
	}
	n.put(check.Call{From: n.pid, To: to, Request: request, Response: resp, Error: err})
	return resp, err
}

func (n *Node) CallImportant(to any, request any) (any, error) {
	var resp any
	var err error
	if n.ov.callImportant != nil {
		resp, err = n.ov.callImportant(to, request)
	}
	n.put(check.Call{From: n.pid, To: to, Request: request, Response: resp, Error: err})
	return resp, err
}

func (n *Node) CallPID(to gen.PID, request any, timeout int) (any, error) {
	var resp any
	var err error
	if n.ov.callPID != nil {
		resp, err = n.ov.callPID(to, request, timeout)
	}
	n.put(check.Call{From: n.pid, To: to, Request: request, Response: resp, Error: err})
	return resp, err
}

func (n *Node) CallProcessID(to gen.ProcessID, request any, timeout int) (any, error) {
	var resp any
	var err error
	if n.ov.callProcessID != nil {
		resp, err = n.ov.callProcessID(to, request, timeout)
	}
	n.put(check.Call{From: n.pid, To: to, Request: request, Response: resp, Error: err})
	return resp, err
}

func (n *Node) CallAlias(to gen.Alias, request any, timeout int) (any, error) {
	var resp any
	var err error
	if n.ov.callAlias != nil {
		resp, err = n.ov.callAlias(to, request, timeout)
	}
	n.put(check.Call{From: n.pid, To: to, Request: request, Response: resp, Error: err})
	return resp, err
}

func (n *Node) Inspect(target gen.PID, item ...string) (map[string]string, error) {
	if n.ov.inspect != nil {
		return n.ov.inspect(target, item...)
	}
	return nil, nil
}

func (n *Node) InspectMeta(alias gen.Alias, item ...string) (map[string]string, error) {
	if n.ov.inspectMeta != nil {
		return n.ov.inspectMeta(alias, item...)
	}
	return nil, nil
}

func (n *Node) Log() gen.Log {
	if n.ov.logFn != nil {
		return n.ov.logFn()
	}
	return n.log
}

func (n *Node) SetProcessLogLevel(pid gen.PID, level gen.LogLevel) error {
	if n.ov.setProcessLogLevel != nil {
		return n.ov.setProcessLogLevel(pid, level)
	}
	return nil
}

func (n *Node) SetProcessSendPriority(pid gen.PID, priority gen.MessagePriority) error {
	if n.ov.setProcessSendPriority != nil {
		return n.ov.setProcessSendPriority(pid, priority)
	}
	return nil
}

func (n *Node) SetProcessCompression(pid gen.PID, enabled bool) error {
	if n.ov.setProcessCompression != nil {
		return n.ov.setProcessCompression(pid, enabled)
	}
	return nil
}

func (n *Node) SetProcessCompressionType(pid gen.PID, ctype gen.CompressionType) error {
	if n.ov.setProcessCompressionType != nil {
		return n.ov.setProcessCompressionType(pid, ctype)
	}
	return nil
}

func (n *Node) SetProcessCompressionLevel(pid gen.PID, level gen.CompressionLevel) error {
	if n.ov.setProcessCompressionLevel != nil {
		return n.ov.setProcessCompressionLevel(pid, level)
	}
	return nil
}

func (n *Node) SetProcessCompressionThreshold(pid gen.PID, threshold int) error {
	if n.ov.setProcessCompressionThreshold != nil {
		return n.ov.setProcessCompressionThreshold(pid, threshold)
	}
	return nil
}

func (n *Node) SetProcessKeepNetworkOrder(pid gen.PID, order bool) error {
	if n.ov.setProcessKeepNetworkOrder != nil {
		return n.ov.setProcessKeepNetworkOrder(pid, order)
	}
	return nil
}

func (n *Node) SetProcessImportantDelivery(pid gen.PID, important bool) error {
	if n.ov.setProcessImportantDelivery != nil {
		return n.ov.setProcessImportantDelivery(pid, important)
	}
	return nil
}

func (n *Node) SetMetaLogLevel(meta gen.Alias, level gen.LogLevel) error {
	if n.ov.setMetaLogLevel != nil {
		return n.ov.setMetaLogLevel(meta, level)
	}
	return nil
}

func (n *Node) SetMetaSendPriority(meta gen.Alias, priority gen.MessagePriority) error {
	if n.ov.setMetaSendPriority != nil {
		return n.ov.setMetaSendPriority(meta, priority)
	}
	return nil
}

func (n *Node) Loggers() []string {
	if n.ov.loggers != nil {
		return n.ov.loggers()
	}
	return nil
}

func (n *Node) LoggerAddPID(pid gen.PID, name string, filter ...gen.LogLevel) error {
	if n.ov.loggerAddPID != nil {
		return n.ov.loggerAddPID(pid, name, filter...)
	}
	return nil
}

func (n *Node) LoggerAdd(name string, logger gen.LoggerBehavior, filter ...gen.LogLevel) error {
	if n.ov.loggerAdd != nil {
		return n.ov.loggerAdd(name, logger, filter...)
	}
	return nil
}

func (n *Node) LoggerDeletePID(pid gen.PID) {
	if n.ov.loggerDeletePID != nil {
		n.ov.loggerDeletePID(pid)
	}
}

func (n *Node) LoggerDelete(name string) {
	if n.ov.loggerDelete != nil {
		n.ov.loggerDelete(name)
	}
}

func (n *Node) LoggerLevels(name string) []gen.LogLevel {
	if n.ov.loggerLevels != nil {
		return n.ov.loggerLevels(name)
	}
	return nil
}

func (n *Node) TracingExporterAddPID(pid gen.PID, name string, flags gen.TracingFlags) error {
	if n.ov.tracingExporterAddPID != nil {
		return n.ov.tracingExporterAddPID(pid, name, flags)
	}
	return nil
}

func (n *Node) TracingExporterAdd(name string, exporter gen.TracingBehavior, flags gen.TracingFlags) error {
	if n.ov.tracingExporterAdd != nil {
		return n.ov.tracingExporterAdd(name, exporter, flags)
	}
	return nil
}

func (n *Node) TracingExporterDeletePID(pid gen.PID) {
	if n.ov.tracingExporterDeletePID != nil {
		n.ov.tracingExporterDeletePID(pid)
	}
}

func (n *Node) TracingExporterDelete(name string) {
	if n.ov.tracingExporterDelete != nil {
		n.ov.tracingExporterDelete(name)
	}
}

func (n *Node) TracingExporters() []string {
	if n.ov.tracingExporters != nil {
		return n.ov.tracingExporters()
	}
	return nil
}

func (n *Node) TracingExporterFlags(name string) gen.TracingFlags {
	if n.ov.tracingExporterFlags != nil {
		return n.ov.tracingExporterFlags(name)
	}
	return 0
}

func (n *Node) SetTracingSampler(sampler gen.TracingSampler) error {
	if n.ov.setTracingSampler != nil {
		return n.ov.setTracingSampler(sampler)
	}
	return nil
}

func (n *Node) SetTracingAttribute(key, value string) {
	if n.ov.setTracingAttribute != nil {
		n.ov.setTracingAttribute(key, value)
	}
}

func (n *Node) RemoveTracingAttribute(key string) {
	if n.ov.removeTracingAttribute != nil {
		n.ov.removeTracingAttribute(key)
	}
}

func (n *Node) TracingSampler() gen.TracingSampler {
	if n.ov.tracingSampler != nil {
		return n.ov.tracingSampler()
	}
	return nil
}

func (n *Node) SetProcessTracingSampler(pid gen.PID, sampler gen.TracingSampler) error {
	if n.ov.setProcessTracingSampler != nil {
		return n.ov.setProcessTracingSampler(pid, sampler)
	}
	return nil
}

func (n *Node) MakeRef() gen.Ref {
	if n.ov.makeRef != nil {
		return n.ov.makeRef()
	}
	return synthRef(n.next.Add(1))
}

func (n *Node) MakeRefWithDeadline(deadline int64) (gen.Ref, error) {
	if n.ov.makeRefWithDeadline != nil {
		return n.ov.makeRefWithDeadline(deadline)
	}
	return synthRef(n.next.Add(1)), nil
}

func (n *Node) Commercial() []gen.Version {
	if n.ov.commercial != nil {
		return n.ov.commercial()
	}
	return nil
}

func (n *Node) PID() gen.PID {
	if n.ov.pidFn != nil {
		return n.ov.pidFn()
	}
	return n.pid
}

func (n *Node) Creation() int64 {
	if n.ov.creation != nil {
		return n.ov.creation()
	}
	return 1
}

func (n *Node) Peers() []gen.Atom {
	if n.ov.peers != nil {
		return n.ov.peers()
	}
	return n.Network().Nodes()
}

func (n *Node) SetCTRLC(enable bool) {
	if n.ov.setCTRLC != nil {
		n.ov.setCTRLC(enable)
	}
}
