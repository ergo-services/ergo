package unit

import (
	"time"

	"ergo.services/ergo/gen"
)

// nodeOverrides holds an optional handler per non-egress gen.Node method. When a
// field is set, the corresponding mockNode method returns the handler's result
// instead of the default. Egress methods (Send/Call/Spawn/...) are controlled via
// the typed stub sugar on the Subject instead.
//
// The On<Method> setters live on *mockNode, so they promote onto MockNode: a test
// configures them via sub.Node().On<Method>(...) (or before spawn on the node from
// unit.Node(...)).
type nodeOverrides struct {
	name             func() gen.Atom
	isAlive          func() bool
	uptime           func() int64
	version          func() gen.Version
	frameworkVersion func() gen.Version
	pid              func() gen.PID
	creation         func() int64
	log              func() gen.Log
	commercial       func() []gen.Version

	envList    func() map[gen.Env]any
	setEnv     func(name gen.Env, value any)
	env        func(name gen.Env) (any, bool)
	envDefault func(name gen.Env, def any) any

	makeRef             func() gen.Ref
	makeRefWithDeadline func(deadline int64) (gen.Ref, error)

	certManager  func() gen.CertManager
	security     func() gen.SecurityOptions
	networkStart func(options gen.NetworkOptions) error
	networkStop  func() error

	kill func(pid gen.PID) error

	registerName   func(name gen.Atom, pid gen.PID) error
	unregisterName func(name gen.Atom) (gen.PID, error)

	unregisterEvent func(name gen.Atom) error
	eventInfo       func(event gen.Event) (gen.EventInfo, error)
	eventRangeInfo  func(fn func(gen.EventInfo) bool) error
	eventListInfo   func(timestamp int64, limit int, filter ...func(gen.EventInfo) bool) ([]gen.EventInfo, error)

	info                  func() (gen.NodeInfo, error)
	metaInfo              func(meta gen.Alias) (gen.MetaInfo, error)
	processInfo           func(pid gen.PID) (gen.ProcessInfo, error)
	processList           func() ([]gen.PID, error)
	processListShortInfo  func(start, limit int, filter ...func(gen.ProcessShortInfo) bool) ([]gen.ProcessShortInfo, error)
	processRangeShortInfo func(fn func(gen.ProcessShortInfo) bool) error
	processName           func(pid gen.PID) (gen.Atom, error)
	processPID            func(name gen.Atom) (gen.PID, error)
	processState          func(pid gen.PID) (gen.ProcessState, error)

	applicationLoad                 func(app gen.ApplicationBehavior, args ...any) (gen.Atom, error)
	applicationInfo                 func(name gen.Atom) (gen.ApplicationInfo, error)
	applicationProcessList          func(name gen.Atom, limit int) ([]gen.PID, error)
	applicationProcessListShortInfo func(name gen.Atom, limit int) ([]gen.ProcessShortInfo, error)
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

	inspect     func(target gen.PID, item ...string) (map[string]string, error)
	inspectMeta func(alias gen.Alias, item ...string) (map[string]string, error)

	setProcessLogLevel             func(pid gen.PID, level gen.LogLevel) error
	setProcessSendPriority         func(pid gen.PID, priority gen.MessagePriority) error
	setProcessCompression          func(pid gen.PID, enabled bool) error
	setProcessCompressionType      func(pid gen.PID, ctype gen.CompressionType) error
	setProcessCompressionLevel     func(pid gen.PID, level gen.CompressionLevel) error
	setProcessCompressionThreshold func(pid gen.PID, threshold int) error
	setProcessKeepNetworkOrder     func(pid gen.PID, order bool) error
	setProcessImportantDelivery    func(pid gen.PID, important bool) error
	setMetaLogLevel                func(meta gen.Alias, level gen.LogLevel) error
	setMetaSendPriority            func(meta gen.Alias, priority gen.MessagePriority) error

	loggers         func() []string
	loggerAddPID    func(pid gen.PID, name string, filter ...gen.LogLevel) error
	loggerAdd       func(name string, logger gen.LoggerBehavior, filter ...gen.LogLevel) error
	loggerDeletePID func(pid gen.PID)
	loggerDelete    func(name string)
	loggerLevels    func(name string) []gen.LogLevel

	tracingExporterAddPID    func(pid gen.PID, name string, flags gen.TracingFlags) error
	tracingExporterAdd       func(name string, exporter gen.TracingBehavior, flags gen.TracingFlags) error
	tracingExporterDeletePID func(pid gen.PID)
	tracingExporterDelete    func(name string)
	tracingExporters         func() []string
	tracingExporterFlags     func(name string) gen.TracingFlags
	setTracingSampler        func(sampler gen.TracingSampler) error
	setTracingAttribute      func(key, value string)
	removeTracingAttribute   func(key string)
	tracingSampler           func() gen.TracingSampler
	setProcessTracingSampler func(pid gen.PID, sampler gen.TracingSampler) error

	stop            func()
	stopWithTimeout func(timeout time.Duration)
	stopForce       func()
	wait            func()
	waitWithTimeout func(timeout time.Duration) error
	setCTRLC        func(enable bool)
}

// On<Method> setters. Defined on *mockNode so they promote onto MockNode; call them
// as sub.Node().On<Method>(...) or, before spawning, on the node from unit.Node(...).

func (n *mockNode) OnName(fn func() gen.Atom)                { n.ov.name = fn }
func (n *mockNode) OnIsAlive(fn func() bool)                 { n.ov.isAlive = fn }
func (n *mockNode) OnUptime(fn func() int64)                 { n.ov.uptime = fn }
func (n *mockNode) OnVersion(fn func() gen.Version)          { n.ov.version = fn }
func (n *mockNode) OnFrameworkVersion(fn func() gen.Version) { n.ov.frameworkVersion = fn }
func (n *mockNode) OnPID(fn func() gen.PID)                  { n.ov.pid = fn }
func (n *mockNode) OnCreation(fn func() int64)               { n.ov.creation = fn }
func (n *mockNode) OnLog(fn func() gen.Log)                  { n.ov.log = fn }
func (n *mockNode) OnCommercial(fn func() []gen.Version)     { n.ov.commercial = fn }

func (n *mockNode) OnEnvList(fn func() map[gen.Env]any)             { n.ov.envList = fn }
func (n *mockNode) OnSetEnv(fn func(name gen.Env, value any))       { n.ov.setEnv = fn }
func (n *mockNode) OnEnv(fn func(name gen.Env) (any, bool))         { n.ov.env = fn }
func (n *mockNode) OnEnvDefault(fn func(name gen.Env, def any) any) { n.ov.envDefault = fn }

func (n *mockNode) OnMakeRef(fn func() gen.Ref) { n.ov.makeRef = fn }
func (n *mockNode) OnMakeRefWithDeadline(fn func(deadline int64) (gen.Ref, error)) {
	n.ov.makeRefWithDeadline = fn
}

func (n *mockNode) OnCertManager(fn func() gen.CertManager)                  { n.ov.certManager = fn }
func (n *mockNode) OnSecurity(fn func() gen.SecurityOptions)                 { n.ov.security = fn }
func (n *mockNode) OnNetworkStart(fn func(options gen.NetworkOptions) error) { n.ov.networkStart = fn }
func (n *mockNode) OnNetworkStop(fn func() error)                            { n.ov.networkStop = fn }

func (n *mockNode) OnKill(fn func(pid gen.PID) error) { n.ov.kill = fn }

func (n *mockNode) OnRegisterName(fn func(name gen.Atom, pid gen.PID) error) { n.ov.registerName = fn }
func (n *mockNode) OnUnregisterName(fn func(name gen.Atom) (gen.PID, error)) {
	n.ov.unregisterName = fn
}

func (n *mockNode) OnUnregisterEvent(fn func(name gen.Atom) error)              { n.ov.unregisterEvent = fn }
func (n *mockNode) OnEventInfo(fn func(event gen.Event) (gen.EventInfo, error)) { n.ov.eventInfo = fn }
func (n *mockNode) OnEventRangeInfo(fn func(fn func(gen.EventInfo) bool) error) {
	n.ov.eventRangeInfo = fn
}
func (n *mockNode) OnEventListInfo(fn func(timestamp int64, limit int, filter ...func(gen.EventInfo) bool) ([]gen.EventInfo, error)) {
	n.ov.eventListInfo = fn
}

func (n *mockNode) OnInfo(fn func() (gen.NodeInfo, error))                   { n.ov.info = fn }
func (n *mockNode) OnMetaInfo(fn func(meta gen.Alias) (gen.MetaInfo, error)) { n.ov.metaInfo = fn }
func (n *mockNode) OnProcessInfo(fn func(pid gen.PID) (gen.ProcessInfo, error)) {
	n.ov.processInfo = fn
}
func (n *mockNode) OnProcessList(fn func() ([]gen.PID, error)) { n.ov.processList = fn }
func (n *mockNode) OnProcessListShortInfo(fn func(start, limit int, filter ...func(gen.ProcessShortInfo) bool) ([]gen.ProcessShortInfo, error)) {
	n.ov.processListShortInfo = fn
}
func (n *mockNode) OnProcessRangeShortInfo(fn func(fn func(gen.ProcessShortInfo) bool) error) {
	n.ov.processRangeShortInfo = fn
}
func (n *mockNode) OnProcessName(fn func(pid gen.PID) (gen.Atom, error)) { n.ov.processName = fn }
func (n *mockNode) OnProcessPID(fn func(name gen.Atom) (gen.PID, error)) { n.ov.processPID = fn }
func (n *mockNode) OnProcessState(fn func(pid gen.PID) (gen.ProcessState, error)) {
	n.ov.processState = fn
}

func (n *mockNode) OnApplicationLoad(fn func(app gen.ApplicationBehavior, args ...any) (gen.Atom, error)) {
	n.ov.applicationLoad = fn
}
func (n *mockNode) OnApplicationInfo(fn func(name gen.Atom) (gen.ApplicationInfo, error)) {
	n.ov.applicationInfo = fn
}
func (n *mockNode) OnApplicationProcessList(fn func(name gen.Atom, limit int) ([]gen.PID, error)) {
	n.ov.applicationProcessList = fn
}
func (n *mockNode) OnApplicationProcessListShortInfo(fn func(name gen.Atom, limit int) ([]gen.ProcessShortInfo, error)) {
	n.ov.applicationProcessListShortInfo = fn
}
func (n *mockNode) OnApplicationUnload(fn func(name gen.Atom) error) { n.ov.applicationUnload = fn }
func (n *mockNode) OnApplicationStart(fn func(name gen.Atom, options gen.ApplicationOptions) error) {
	n.ov.applicationStart = fn
}
func (n *mockNode) OnApplicationStartTemporary(fn func(name gen.Atom, options gen.ApplicationOptions) error) {
	n.ov.applicationStartTemporary = fn
}
func (n *mockNode) OnApplicationStartTransient(fn func(name gen.Atom, options gen.ApplicationOptions) error) {
	n.ov.applicationStartTransient = fn
}
func (n *mockNode) OnApplicationStartPermanent(fn func(name gen.Atom, options gen.ApplicationOptions) error) {
	n.ov.applicationStartPermanent = fn
}
func (n *mockNode) OnApplicationStop(fn func(name gen.Atom) error) { n.ov.applicationStop = fn }
func (n *mockNode) OnApplicationStopForce(fn func(name gen.Atom) error) {
	n.ov.applicationStopForce = fn
}
func (n *mockNode) OnApplicationStopWithTimeout(fn func(name gen.Atom, timeout time.Duration) error) {
	n.ov.applicationStopWithTimeout = fn
}
func (n *mockNode) OnApplications(fn func() []gen.Atom)        { n.ov.applications = fn }
func (n *mockNode) OnApplicationsRunning(fn func() []gen.Atom) { n.ov.applicationsRunning = fn }

func (n *mockNode) OnInspect(fn func(target gen.PID, item ...string) (map[string]string, error)) {
	n.ov.inspect = fn
}
func (n *mockNode) OnInspectMeta(fn func(alias gen.Alias, item ...string) (map[string]string, error)) {
	n.ov.inspectMeta = fn
}

func (n *mockNode) OnSetProcessLogLevel(fn func(pid gen.PID, level gen.LogLevel) error) {
	n.ov.setProcessLogLevel = fn
}
func (n *mockNode) OnSetProcessSendPriority(fn func(pid gen.PID, priority gen.MessagePriority) error) {
	n.ov.setProcessSendPriority = fn
}
func (n *mockNode) OnSetProcessCompression(fn func(pid gen.PID, enabled bool) error) {
	n.ov.setProcessCompression = fn
}
func (n *mockNode) OnSetProcessCompressionType(fn func(pid gen.PID, ctype gen.CompressionType) error) {
	n.ov.setProcessCompressionType = fn
}
func (n *mockNode) OnSetProcessCompressionLevel(fn func(pid gen.PID, level gen.CompressionLevel) error) {
	n.ov.setProcessCompressionLevel = fn
}
func (n *mockNode) OnSetProcessCompressionThreshold(fn func(pid gen.PID, threshold int) error) {
	n.ov.setProcessCompressionThreshold = fn
}
func (n *mockNode) OnSetProcessKeepNetworkOrder(fn func(pid gen.PID, order bool) error) {
	n.ov.setProcessKeepNetworkOrder = fn
}
func (n *mockNode) OnSetProcessImportantDelivery(fn func(pid gen.PID, important bool) error) {
	n.ov.setProcessImportantDelivery = fn
}
func (n *mockNode) OnSetMetaLogLevel(fn func(meta gen.Alias, level gen.LogLevel) error) {
	n.ov.setMetaLogLevel = fn
}
func (n *mockNode) OnSetMetaSendPriority(fn func(meta gen.Alias, priority gen.MessagePriority) error) {
	n.ov.setMetaSendPriority = fn
}

func (n *mockNode) OnLoggers(fn func() []string) { n.ov.loggers = fn }
func (n *mockNode) OnLoggerAddPID(fn func(pid gen.PID, name string, filter ...gen.LogLevel) error) {
	n.ov.loggerAddPID = fn
}
func (n *mockNode) OnLoggerAdd(fn func(name string, logger gen.LoggerBehavior, filter ...gen.LogLevel) error) {
	n.ov.loggerAdd = fn
}
func (n *mockNode) OnLoggerDeletePID(fn func(pid gen.PID))             { n.ov.loggerDeletePID = fn }
func (n *mockNode) OnLoggerDelete(fn func(name string))                { n.ov.loggerDelete = fn }
func (n *mockNode) OnLoggerLevels(fn func(name string) []gen.LogLevel) { n.ov.loggerLevels = fn }

func (n *mockNode) OnTracingExporterAddPID(fn func(pid gen.PID, name string, flags gen.TracingFlags) error) {
	n.ov.tracingExporterAddPID = fn
}
func (n *mockNode) OnTracingExporterAdd(fn func(name string, exporter gen.TracingBehavior, flags gen.TracingFlags) error) {
	n.ov.tracingExporterAdd = fn
}
func (n *mockNode) OnTracingExporterDeletePID(fn func(pid gen.PID)) {
	n.ov.tracingExporterDeletePID = fn
}
func (n *mockNode) OnTracingExporterDelete(fn func(name string)) { n.ov.tracingExporterDelete = fn }
func (n *mockNode) OnTracingExporters(fn func() []string)        { n.ov.tracingExporters = fn }
func (n *mockNode) OnTracingExporterFlags(fn func(name string) gen.TracingFlags) {
	n.ov.tracingExporterFlags = fn
}
func (n *mockNode) OnSetTracingSampler(fn func(sampler gen.TracingSampler) error) {
	n.ov.setTracingSampler = fn
}
func (n *mockNode) OnSetTracingAttribute(fn func(key, value string)) { n.ov.setTracingAttribute = fn }
func (n *mockNode) OnRemoveTracingAttribute(fn func(key string))     { n.ov.removeTracingAttribute = fn }
func (n *mockNode) OnTracingSampler(fn func() gen.TracingSampler)    { n.ov.tracingSampler = fn }
func (n *mockNode) OnSetProcessTracingSampler(fn func(pid gen.PID, sampler gen.TracingSampler) error) {
	n.ov.setProcessTracingSampler = fn
}

func (n *mockNode) OnStop(fn func())                                       { n.ov.stop = fn }
func (n *mockNode) OnStopWithTimeout(fn func(timeout time.Duration))       { n.ov.stopWithTimeout = fn }
func (n *mockNode) OnStopForce(fn func())                                  { n.ov.stopForce = fn }
func (n *mockNode) OnWait(fn func())                                       { n.ov.wait = fn }
func (n *mockNode) OnWaitWithTimeout(fn func(timeout time.Duration) error) { n.ov.waitWithTimeout = fn }
func (n *mockNode) OnSetCTRLC(fn func(enable bool))                        { n.ov.setCTRLC = fn }
