package mock

import (
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/testing/check"
)

// Process is a standalone gen.Process mock. Every method has an On<Method> override.
// Unset, egress methods record a check.Record (only when built via NewProcessT) and
// return a success/synthetic value; queries and setters return safe defaults.
type Process struct {
	recorder
	pid     gen.PID
	name    gen.Atom
	next    atomic.Uint64
	mailbox gen.ProcessMailbox
	state   gen.ProcessState
	node    *Node
	log     *Log
	ov      processOverrides
}

type processOverrides struct {
	node                       func() gen.Node
	name                       func() gen.Atom
	pid                        func() gen.PID
	leader                     func() gen.PID
	parent                     func() gen.PID
	application                func() gen.Application
	uptime                     func() int64
	spawn                      func(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error)
	spawnRegister              func(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error)
	spawnMeta                  func(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error)
	remoteSpawn                func(node gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error)
	remoteSpawnRegister        func(node gen.Atom, name gen.Atom, register gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error)
	state                      func() gen.ProcessState
	registerName               func(name gen.Atom) error
	unregisterName             func() error
	envList                    func() map[gen.Env]any
	setEnv                     func(name gen.Env, value any)
	env                        func(name gen.Env) (any, bool)
	envDefault                 func(name gen.Env, def any) any
	compression                func() bool
	setCompression             func(enabled bool) error
	compressionType            func() gen.CompressionType
	setCompressionType         func(ctype gen.CompressionType) error
	compressionLevel           func() gen.CompressionLevel
	setCompressionLevel        func(level gen.CompressionLevel) error
	compressionThreshold       func() int
	setCompressionThreshold    func(threshold int) error
	sendPriority               func() gen.MessagePriority
	setSendPriority            func(priority gen.MessagePriority) error
	setProcessKind             func(kind gen.ProcessKind) error
	setKeepNetworkOrder        func(order bool) error
	keepNetworkOrder           func() bool
	setImportantDelivery       func(important bool) error
	importantDelivery          func() bool
	setTracingSampler          func(sampler gen.TracingSampler) error
	tracingSampler             func() gen.TracingSampler
	createAlias                func() (gen.Alias, error)
	deleteAlias                func(alias gen.Alias) error
	aliases                    func() []gen.Alias
	events                     func() []gen.Atom
	send                       func(to any, message any) error
	sendPID                    func(to gen.PID, message any) error
	sendProcessID              func(to gen.ProcessID, message any) error
	sendAlias                  func(to gen.Alias, message any) error
	sendWithPriority           func(to any, message any, priority gen.MessagePriority) error
	sendImportant              func(to any, message any) error
	sendAfter                  func(to any, message any, after time.Duration) (gen.CancelFunc, error)
	sendWithPriorityAfter      func(to any, message any, priority gen.MessagePriority, after time.Duration) (gen.CancelFunc, error)
	sendEvent                  func(name gen.Atom, token gen.Ref, message any) error
	sendExit                   func(to gen.PID, reason error) error
	sendExitAfter              func(to gen.PID, reason error, after time.Duration) (gen.CancelFunc, error)
	sendExitMeta               func(meta gen.Alias, reason error) error
	sendExitMetaAfter          func(meta gen.Alias, reason error, after time.Duration) (gen.CancelFunc, error)
	sendResponse               func(to gen.PID, ref gen.Ref, message any) error
	sendResponseImportant      func(to gen.PID, ref gen.Ref, message any) error
	sendResponseError          func(to gen.PID, ref gen.Ref, err error) error
	sendResponseErrorImportant func(to gen.PID, ref gen.Ref, err error) error
	call                       func(to any, message any) (any, error)
	callWithTimeout            func(to any, message any, timeout int) (any, error)
	callWithPriority           func(to any, message any, priority gen.MessagePriority) (any, error)
	callImportant              func(to any, message any) (any, error)
	callPID                    func(to gen.PID, message any, timeout int) (any, error)
	callProcessID              func(to gen.ProcessID, message any, timeout int) (any, error)
	callAlias                  func(to gen.Alias, message any, timeout int) (any, error)
	inspect                    func(target gen.PID, item ...string) (map[string]string, error)
	inspectMeta                func(meta gen.Alias, item ...string) (map[string]string, error)
	registerEvent              func(name gen.Atom, options gen.EventOptions) (gen.Ref, error)
	unregisterEvent            func(name gen.Atom) error
	link                       func(target any) error
	unlink                     func(target any) error
	linkPID                    func(target gen.PID) error
	unlinkPID                  func(target gen.PID) error
	linkProcessID              func(target gen.ProcessID) error
	unlinkProcessID            func(target gen.ProcessID) error
	linkAlias                  func(target gen.Alias) error
	unlinkAlias                func(target gen.Alias) error
	linkEvent                  func(target gen.Event) ([]gen.MessageEvent, error)
	unlinkEvent                func(target gen.Event) error
	linkNode                   func(target gen.Atom) error
	unlinkNode                 func(target gen.Atom) error
	monitor                    func(target any) error
	demonitor                  func(target any) error
	monitorPID                 func(pid gen.PID) error
	demonitorPID               func(pid gen.PID) error
	monitorProcessID           func(process gen.ProcessID) error
	demonitorProcessID         func(process gen.ProcessID) error
	monitorAlias               func(alias gen.Alias) error
	demonitorAlias             func(alias gen.Alias) error
	monitorEvent               func(event gen.Event) ([]gen.MessageEvent, error)
	demonitorEvent             func(event gen.Event) error
	monitorNode                func(node gen.Atom) error
	demonitorNode              func(node gen.Atom) error
	log                        func() gen.Log
	info                       func() (gen.ProcessInfo, error)
	metaInfo                   func(meta gen.Alias) (gen.MetaInfo, error)
	mailbox                    func() gen.ProcessMailbox
	behavior                   func() gen.ProcessBehavior
	behaviorName               func() string
	propagatingTrace           func() gen.Tracing
	setPropagatingTrace        func(t gen.Tracing)
	setTracingAttribute        func(key, value string)
	removeTracingAttribute     func(key string)
	setTracingSpanAttribute    func(key, value string)
	tracingAttributes          func() []gen.TracingAttribute
	clearTracingSpanAttributes func()
	sendTracingSpan            func(span gen.TracingSpan)
	forward                    func(to gen.PID, message *gen.MailboxMessage, priority gen.MessagePriority) error
}

var _ gen.Process = (*Process)(nil)

// NewProcess returns a dumb gen.Process mock (no recording; use NewProcessT for Should*).
func NewProcess() *Process { return newProcess(recorder{}) }

// NewProcessT returns a gen.Process mock that records egress and asserts through t.
func NewProcessT(t check.T) *Process { return newProcess(newRecorder(t)) }

func newProcess(r recorder) *Process {
	p := &Process{
		recorder: r,
		pid:      synthPID(1000),
		state:    gen.ProcessStateRunning,
		mailbox: gen.ProcessMailbox{
			Main:   lib.NewQueueMPSC(),
			System: lib.NewQueueMPSC(),
			Urgent: lib.NewQueueMPSC(),
			Log:    lib.NewQueueMPSC(),
		},
	}
	p.node = newNode(r)
	p.log = newLog(r)
	return p
}

// On<Method> overrides

func (p *Process) OnNode(fn func() gen.Node)               { p.ov.node = fn }
func (p *Process) OnName(fn func() gen.Atom)               { p.ov.name = fn }
func (p *Process) OnPID(fn func() gen.PID)                 { p.ov.pid = fn }
func (p *Process) OnLeader(fn func() gen.PID)              { p.ov.leader = fn }
func (p *Process) OnParent(fn func() gen.PID)              { p.ov.parent = fn }
func (p *Process) OnApplication(fn func() gen.Application) { p.ov.application = fn }
func (p *Process) OnUptime(fn func() int64)                { p.ov.uptime = fn }

func (p *Process) OnSpawn(fn func(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error)) {
	p.ov.spawn = fn
}

func (p *Process) OnSpawnRegister(fn func(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error)) {
	p.ov.spawnRegister = fn
}

func (p *Process) OnSpawnMeta(fn func(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error)) {
	p.ov.spawnMeta = fn
}

func (p *Process) OnRemoteSpawn(fn func(node gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error)) {
	p.ov.remoteSpawn = fn
}

func (p *Process) OnRemoteSpawnRegister(fn func(node gen.Atom, name gen.Atom, register gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error)) {
	p.ov.remoteSpawnRegister = fn
}

func (p *Process) OnState(fn func() gen.ProcessState)          { p.ov.state = fn }
func (p *Process) OnRegisterName(fn func(name gen.Atom) error) { p.ov.registerName = fn }
func (p *Process) OnUnregisterName(fn func() error)            { p.ov.unregisterName = fn }
func (p *Process) OnEnvList(fn func() map[gen.Env]any)         { p.ov.envList = fn }
func (p *Process) OnSetEnv(fn func(name gen.Env, value any))   { p.ov.setEnv = fn }
func (p *Process) OnEnv(fn func(name gen.Env) (any, bool))     { p.ov.env = fn }
func (p *Process) OnEnvDefault(fn func(name gen.Env, def any) any) {
	p.ov.envDefault = fn
}
func (p *Process) OnCompression(fn func() bool)                    { p.ov.compression = fn }
func (p *Process) OnSetCompression(fn func(enabled bool) error)    { p.ov.setCompression = fn }
func (p *Process) OnCompressionType(fn func() gen.CompressionType) { p.ov.compressionType = fn }
func (p *Process) OnSetCompressionType(fn func(ctype gen.CompressionType) error) {
	p.ov.setCompressionType = fn
}
func (p *Process) OnCompressionLevel(fn func() gen.CompressionLevel) { p.ov.compressionLevel = fn }
func (p *Process) OnSetCompressionLevel(fn func(level gen.CompressionLevel) error) {
	p.ov.setCompressionLevel = fn
}
func (p *Process) OnCompressionThreshold(fn func() int) { p.ov.compressionThreshold = fn }
func (p *Process) OnSetCompressionThreshold(fn func(threshold int) error) {
	p.ov.setCompressionThreshold = fn
}
func (p *Process) OnSendPriority(fn func() gen.MessagePriority) { p.ov.sendPriority = fn }
func (p *Process) OnSetSendPriority(fn func(priority gen.MessagePriority) error) {
	p.ov.setSendPriority = fn
}
func (p *Process) OnSetProcessKind(fn func(kind gen.ProcessKind) error) { p.ov.setProcessKind = fn }
func (p *Process) OnSetKeepNetworkOrder(fn func(order bool) error)      { p.ov.setKeepNetworkOrder = fn }
func (p *Process) OnKeepNetworkOrder(fn func() bool)                    { p.ov.keepNetworkOrder = fn }
func (p *Process) OnSetImportantDelivery(fn func(important bool) error) {
	p.ov.setImportantDelivery = fn
}
func (p *Process) OnImportantDelivery(fn func() bool) { p.ov.importantDelivery = fn }
func (p *Process) OnSetTracingSampler(fn func(sampler gen.TracingSampler) error) {
	p.ov.setTracingSampler = fn
}
func (p *Process) OnTracingSampler(fn func() gen.TracingSampler) { p.ov.tracingSampler = fn }
func (p *Process) OnCreateAlias(fn func() (gen.Alias, error))    { p.ov.createAlias = fn }
func (p *Process) OnDeleteAlias(fn func(alias gen.Alias) error)  { p.ov.deleteAlias = fn }
func (p *Process) OnAliases(fn func() []gen.Alias)               { p.ov.aliases = fn }
func (p *Process) OnEvents(fn func() []gen.Atom)                 { p.ov.events = fn }
func (p *Process) OnSend(fn func(to any, message any) error)     { p.ov.send = fn }
func (p *Process) OnSendPID(fn func(to gen.PID, message any) error) {
	p.ov.sendPID = fn
}
func (p *Process) OnSendProcessID(fn func(to gen.ProcessID, message any) error) {
	p.ov.sendProcessID = fn
}
func (p *Process) OnSendAlias(fn func(to gen.Alias, message any) error) {
	p.ov.sendAlias = fn
}
func (p *Process) OnSendWithPriority(fn func(to any, message any, priority gen.MessagePriority) error) {
	p.ov.sendWithPriority = fn
}
func (p *Process) OnSendImportant(fn func(to any, message any) error) { p.ov.sendImportant = fn }
func (p *Process) OnSendAfter(fn func(to any, message any, after time.Duration) (gen.CancelFunc, error)) {
	p.ov.sendAfter = fn
}
func (p *Process) OnSendWithPriorityAfter(fn func(to any, message any, priority gen.MessagePriority, after time.Duration) (gen.CancelFunc, error)) {
	p.ov.sendWithPriorityAfter = fn
}
func (p *Process) OnSendEvent(fn func(name gen.Atom, token gen.Ref, message any) error) {
	p.ov.sendEvent = fn
}
func (p *Process) OnSendExit(fn func(to gen.PID, reason error) error) { p.ov.sendExit = fn }
func (p *Process) OnSendExitAfter(fn func(to gen.PID, reason error, after time.Duration) (gen.CancelFunc, error)) {
	p.ov.sendExitAfter = fn
}
func (p *Process) OnSendExitMeta(fn func(meta gen.Alias, reason error) error) {
	p.ov.sendExitMeta = fn
}
func (p *Process) OnSendExitMetaAfter(fn func(meta gen.Alias, reason error, after time.Duration) (gen.CancelFunc, error)) {
	p.ov.sendExitMetaAfter = fn
}
func (p *Process) OnSendResponse(fn func(to gen.PID, ref gen.Ref, message any) error) {
	p.ov.sendResponse = fn
}
func (p *Process) OnSendResponseImportant(fn func(to gen.PID, ref gen.Ref, message any) error) {
	p.ov.sendResponseImportant = fn
}
func (p *Process) OnSendResponseError(fn func(to gen.PID, ref gen.Ref, err error) error) {
	p.ov.sendResponseError = fn
}
func (p *Process) OnSendResponseErrorImportant(fn func(to gen.PID, ref gen.Ref, err error) error) {
	p.ov.sendResponseErrorImportant = fn
}
func (p *Process) OnCall(fn func(to any, message any) (any, error)) { p.ov.call = fn }
func (p *Process) OnCallWithTimeout(fn func(to any, message any, timeout int) (any, error)) {
	p.ov.callWithTimeout = fn
}
func (p *Process) OnCallWithPriority(fn func(to any, message any, priority gen.MessagePriority) (any, error)) {
	p.ov.callWithPriority = fn
}
func (p *Process) OnCallImportant(fn func(to any, message any) (any, error)) { p.ov.callImportant = fn }
func (p *Process) OnCallPID(fn func(to gen.PID, message any, timeout int) (any, error)) {
	p.ov.callPID = fn
}
func (p *Process) OnCallProcessID(fn func(to gen.ProcessID, message any, timeout int) (any, error)) {
	p.ov.callProcessID = fn
}
func (p *Process) OnCallAlias(fn func(to gen.Alias, message any, timeout int) (any, error)) {
	p.ov.callAlias = fn
}
func (p *Process) OnInspect(fn func(target gen.PID, item ...string) (map[string]string, error)) {
	p.ov.inspect = fn
}
func (p *Process) OnInspectMeta(fn func(meta gen.Alias, item ...string) (map[string]string, error)) {
	p.ov.inspectMeta = fn
}
func (p *Process) OnRegisterEvent(fn func(name gen.Atom, options gen.EventOptions) (gen.Ref, error)) {
	p.ov.registerEvent = fn
}
func (p *Process) OnUnregisterEvent(fn func(name gen.Atom) error)        { p.ov.unregisterEvent = fn }
func (p *Process) OnLink(fn func(target any) error)                      { p.ov.link = fn }
func (p *Process) OnUnlink(fn func(target any) error)                    { p.ov.unlink = fn }
func (p *Process) OnLinkPID(fn func(target gen.PID) error)               { p.ov.linkPID = fn }
func (p *Process) OnUnlinkPID(fn func(target gen.PID) error)             { p.ov.unlinkPID = fn }
func (p *Process) OnLinkProcessID(fn func(target gen.ProcessID) error)   { p.ov.linkProcessID = fn }
func (p *Process) OnUnlinkProcessID(fn func(target gen.ProcessID) error) { p.ov.unlinkProcessID = fn }
func (p *Process) OnLinkAlias(fn func(target gen.Alias) error)           { p.ov.linkAlias = fn }
func (p *Process) OnUnlinkAlias(fn func(target gen.Alias) error)         { p.ov.unlinkAlias = fn }
func (p *Process) OnLinkEvent(fn func(target gen.Event) ([]gen.MessageEvent, error)) {
	p.ov.linkEvent = fn
}
func (p *Process) OnUnlinkEvent(fn func(target gen.Event) error) { p.ov.unlinkEvent = fn }
func (p *Process) OnLinkNode(fn func(target gen.Atom) error)     { p.ov.linkNode = fn }
func (p *Process) OnUnlinkNode(fn func(target gen.Atom) error)   { p.ov.unlinkNode = fn }
func (p *Process) OnMonitor(fn func(target any) error)           { p.ov.monitor = fn }
func (p *Process) OnDemonitor(fn func(target any) error)         { p.ov.demonitor = fn }
func (p *Process) OnMonitorPID(fn func(pid gen.PID) error)       { p.ov.monitorPID = fn }
func (p *Process) OnDemonitorPID(fn func(pid gen.PID) error)     { p.ov.demonitorPID = fn }
func (p *Process) OnMonitorProcessID(fn func(process gen.ProcessID) error) {
	p.ov.monitorProcessID = fn
}
func (p *Process) OnDemonitorProcessID(fn func(process gen.ProcessID) error) {
	p.ov.demonitorProcessID = fn
}
func (p *Process) OnMonitorAlias(fn func(alias gen.Alias) error)   { p.ov.monitorAlias = fn }
func (p *Process) OnDemonitorAlias(fn func(alias gen.Alias) error) { p.ov.demonitorAlias = fn }
func (p *Process) OnMonitorEvent(fn func(event gen.Event) ([]gen.MessageEvent, error)) {
	p.ov.monitorEvent = fn
}
func (p *Process) OnDemonitorEvent(fn func(event gen.Event) error) { p.ov.demonitorEvent = fn }
func (p *Process) OnMonitorNode(fn func(node gen.Atom) error)      { p.ov.monitorNode = fn }
func (p *Process) OnDemonitorNode(fn func(node gen.Atom) error)    { p.ov.demonitorNode = fn }
func (p *Process) OnLog(fn func() gen.Log)                         { p.ov.log = fn }
func (p *Process) OnInfo(fn func() (gen.ProcessInfo, error))       { p.ov.info = fn }
func (p *Process) OnMetaInfo(fn func(meta gen.Alias) (gen.MetaInfo, error)) {
	p.ov.metaInfo = fn
}
func (p *Process) OnMailbox(fn func() gen.ProcessMailbox)       { p.ov.mailbox = fn }
func (p *Process) OnBehavior(fn func() gen.ProcessBehavior)     { p.ov.behavior = fn }
func (p *Process) OnBehaviorName(fn func() string)              { p.ov.behaviorName = fn }
func (p *Process) OnPropagatingTrace(fn func() gen.Tracing)     { p.ov.propagatingTrace = fn }
func (p *Process) OnSetPropagatingTrace(fn func(t gen.Tracing)) { p.ov.setPropagatingTrace = fn }
func (p *Process) OnSetTracingAttribute(fn func(key, value string)) {
	p.ov.setTracingAttribute = fn
}
func (p *Process) OnRemoveTracingAttribute(fn func(key string)) { p.ov.removeTracingAttribute = fn }
func (p *Process) OnSetTracingSpanAttribute(fn func(key, value string)) {
	p.ov.setTracingSpanAttribute = fn
}
func (p *Process) OnTracingAttributes(fn func() []gen.TracingAttribute) {
	p.ov.tracingAttributes = fn
}
func (p *Process) OnClearTracingSpanAttributes(fn func()) { p.ov.clearTracingSpanAttributes = fn }
func (p *Process) OnSendTracingSpan(fn func(span gen.TracingSpan)) {
	p.ov.sendTracingSpan = fn
}
func (p *Process) OnForward(fn func(to gen.PID, message *gen.MailboxMessage, priority gen.MessagePriority) error) {
	p.ov.forward = fn
}

// gen.Process

func (p *Process) Node() gen.Node {
	if p.ov.node != nil {
		return p.ov.node()
	}
	return p.node
}

func (p *Process) Name() gen.Atom {
	if p.ov.name != nil {
		return p.ov.name()
	}
	return p.name
}

func (p *Process) PID() gen.PID {
	if p.ov.pid != nil {
		return p.ov.pid()
	}
	return p.pid
}

func (p *Process) Leader() gen.PID {
	if p.ov.leader != nil {
		return p.ov.leader()
	}
	return gen.PID{}
}

func (p *Process) Parent() gen.PID {
	if p.ov.parent != nil {
		return p.ov.parent()
	}
	return gen.PID{}
}

func (p *Process) Application() gen.Application {
	if p.ov.application != nil {
		return p.ov.application()
	}
	return nil
}

func (p *Process) Uptime() int64 {
	if p.ov.uptime != nil {
		return p.ov.uptime()
	}
	return 0
}

func (p *Process) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	child, err := synthPID(p.next.Add(1)), error(nil)
	if p.ov.spawn != nil {
		child, err = p.ov.spawn(factory, options, args...)
	}
	p.put(check.Spawn{Parent: p.pid, Child: child, Factory: factory, Options: options, Error: err})
	return child, err
}

func (p *Process) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	child, err := synthPID(p.next.Add(1)), error(nil)
	if p.ov.spawnRegister != nil {
		child, err = p.ov.spawnRegister(register, factory, options, args...)
	}
	p.put(check.Spawn{Parent: p.pid, Child: child, Register: register, Factory: factory, Options: options, Error: err})
	return child, err
}

func (p *Process) SpawnMeta(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	alias, err := synthAlias(p.next.Add(1)), error(nil)
	if p.ov.spawnMeta != nil {
		alias, err = p.ov.spawnMeta(behavior, options)
	}
	p.put(check.SpawnMeta{Parent: p.pid, Alias: alias, Error: err})
	return alias, err
}

func (p *Process) RemoteSpawn(node gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	child, err := synthPID(p.next.Add(1)), error(nil)
	if p.ov.remoteSpawn != nil {
		child, err = p.ov.remoteSpawn(node, name, options, args...)
	}
	p.put(check.RemoteSpawn{Parent: p.pid, Node: node, Name: name, Child: child, Options: options, Error: err})
	return child, err
}

func (p *Process) RemoteSpawnRegister(node gen.Atom, name gen.Atom, register gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	child, err := synthPID(p.next.Add(1)), error(nil)
	if p.ov.remoteSpawnRegister != nil {
		child, err = p.ov.remoteSpawnRegister(node, name, register, options, args...)
	}
	p.put(check.RemoteSpawn{Parent: p.pid, Node: node, Name: name, Register: register, Child: child, Options: options, Error: err})
	return child, err
}

func (p *Process) State() gen.ProcessState {
	if p.ov.state != nil {
		return p.ov.state()
	}
	return p.state
}

func (p *Process) RegisterName(name gen.Atom) error {
	if p.ov.registerName != nil {
		return p.ov.registerName(name)
	}
	return nil
}

func (p *Process) UnregisterName() error {
	if p.ov.unregisterName != nil {
		return p.ov.unregisterName()
	}
	return nil
}

func (p *Process) EnvList() map[gen.Env]any {
	if p.ov.envList != nil {
		return p.ov.envList()
	}
	return nil
}

func (p *Process) SetEnv(name gen.Env, value any) {
	if p.ov.setEnv != nil {
		p.ov.setEnv(name, value)
	}
}

func (p *Process) Env(name gen.Env) (any, bool) {
	if p.ov.env != nil {
		return p.ov.env(name)
	}
	return nil, false
}

func (p *Process) EnvDefault(name gen.Env, def any) any {
	if p.ov.envDefault != nil {
		return p.ov.envDefault(name, def)
	}
	return def
}

func (p *Process) Compression() bool {
	if p.ov.compression != nil {
		return p.ov.compression()
	}
	return false
}

func (p *Process) SetCompression(enabled bool) error {
	if p.ov.setCompression != nil {
		return p.ov.setCompression(enabled)
	}
	return nil
}

func (p *Process) CompressionType() gen.CompressionType {
	if p.ov.compressionType != nil {
		return p.ov.compressionType()
	}
	return gen.CompressionType("")
}

func (p *Process) SetCompressionType(ctype gen.CompressionType) error {
	if p.ov.setCompressionType != nil {
		return p.ov.setCompressionType(ctype)
	}
	return nil
}

func (p *Process) CompressionLevel() gen.CompressionLevel {
	if p.ov.compressionLevel != nil {
		return p.ov.compressionLevel()
	}
	return gen.CompressionLevel(0)
}

func (p *Process) SetCompressionLevel(level gen.CompressionLevel) error {
	if p.ov.setCompressionLevel != nil {
		return p.ov.setCompressionLevel(level)
	}
	return nil
}

func (p *Process) CompressionThreshold() int {
	if p.ov.compressionThreshold != nil {
		return p.ov.compressionThreshold()
	}
	return 0
}

func (p *Process) SetCompressionThreshold(threshold int) error {
	if p.ov.setCompressionThreshold != nil {
		return p.ov.setCompressionThreshold(threshold)
	}
	return nil
}

func (p *Process) SendPriority() gen.MessagePriority {
	if p.ov.sendPriority != nil {
		return p.ov.sendPriority()
	}
	return gen.MessagePriorityNormal
}

func (p *Process) SetSendPriority(priority gen.MessagePriority) error {
	if p.ov.setSendPriority != nil {
		return p.ov.setSendPriority(priority)
	}
	return nil
}

func (p *Process) SetProcessKind(kind gen.ProcessKind) error {
	if p.ov.setProcessKind != nil {
		return p.ov.setProcessKind(kind)
	}
	return nil
}

func (p *Process) SetKeepNetworkOrder(order bool) error {
	if p.ov.setKeepNetworkOrder != nil {
		return p.ov.setKeepNetworkOrder(order)
	}
	return nil
}

func (p *Process) KeepNetworkOrder() bool {
	if p.ov.keepNetworkOrder != nil {
		return p.ov.keepNetworkOrder()
	}
	return true
}

func (p *Process) SetImportantDelivery(important bool) error {
	if p.ov.setImportantDelivery != nil {
		return p.ov.setImportantDelivery(important)
	}
	return nil
}

func (p *Process) ImportantDelivery() bool {
	if p.ov.importantDelivery != nil {
		return p.ov.importantDelivery()
	}
	return false
}

func (p *Process) SetTracingSampler(sampler gen.TracingSampler) error {
	if p.ov.setTracingSampler != nil {
		return p.ov.setTracingSampler(sampler)
	}
	return nil
}

func (p *Process) TracingSampler() gen.TracingSampler {
	if p.ov.tracingSampler != nil {
		return p.ov.tracingSampler()
	}
	return nil
}

func (p *Process) CreateAlias() (gen.Alias, error) {
	alias, err := synthAlias(p.next.Add(1)), error(nil)
	if p.ov.createAlias != nil {
		alias, err = p.ov.createAlias()
	}
	p.put(check.CreateAlias{PID: p.pid, Alias: alias, Error: err})
	return alias, err
}

func (p *Process) DeleteAlias(alias gen.Alias) error {
	var err error
	if p.ov.deleteAlias != nil {
		err = p.ov.deleteAlias(alias)
	}
	p.put(check.DeleteAlias{PID: p.pid, Alias: alias, Error: err})
	return err
}

func (p *Process) Aliases() []gen.Alias {
	if p.ov.aliases != nil {
		return p.ov.aliases()
	}
	return nil
}

func (p *Process) Events() []gen.Atom {
	if p.ov.events != nil {
		return p.ov.events()
	}
	return nil
}

func (p *Process) Send(to any, message any) error {
	var err error
	if p.ov.send != nil {
		err = p.ov.send(to, message)
	}
	p.put(check.Send{From: p.pid, To: to, Message: message, Error: err})
	return err
}

func (p *Process) SendPID(to gen.PID, message any) error {
	var err error
	if p.ov.sendPID != nil {
		err = p.ov.sendPID(to, message)
	}
	p.put(check.Send{From: p.pid, To: to, Message: message, Error: err})
	return err
}

func (p *Process) SendProcessID(to gen.ProcessID, message any) error {
	var err error
	if p.ov.sendProcessID != nil {
		err = p.ov.sendProcessID(to, message)
	}
	p.put(check.Send{From: p.pid, To: to, Message: message, Error: err})
	return err
}

func (p *Process) SendAlias(to gen.Alias, message any) error {
	var err error
	if p.ov.sendAlias != nil {
		err = p.ov.sendAlias(to, message)
	}
	p.put(check.Send{From: p.pid, To: to, Message: message, Error: err})
	return err
}

func (p *Process) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	var err error
	if p.ov.sendWithPriority != nil {
		err = p.ov.sendWithPriority(to, message, priority)
	}
	p.put(check.Send{From: p.pid, To: to, Message: message, Options: gen.MessageOptions{Priority: priority}, Error: err})
	return err
}

func (p *Process) SendImportant(to any, message any) error {
	var err error
	if p.ov.sendImportant != nil {
		err = p.ov.sendImportant(to, message)
	}
	p.put(check.Send{From: p.pid, To: to, Message: message, Options: gen.MessageOptions{ImportantDelivery: true}, Error: err})
	return err
}

func (p *Process) SendAfter(to any, message any, after time.Duration) (gen.CancelFunc, error) {
	cancel, err := gen.CancelFunc(func() bool { return true }), error(nil)
	if p.ov.sendAfter != nil {
		cancel, err = p.ov.sendAfter(to, message, after)
	}
	p.put(check.SendAfter{From: p.pid, To: to, Message: message, After: after, Error: err})
	return cancel, err
}

func (p *Process) SendWithPriorityAfter(to any, message any, priority gen.MessagePriority, after time.Duration) (gen.CancelFunc, error) {
	cancel, err := gen.CancelFunc(func() bool { return true }), error(nil)
	if p.ov.sendWithPriorityAfter != nil {
		cancel, err = p.ov.sendWithPriorityAfter(to, message, priority, after)
	}
	p.put(check.SendAfter{From: p.pid, To: to, Message: message, After: after, Options: gen.MessageOptions{Priority: priority}, Error: err})
	return cancel, err
}

func (p *Process) SendEvent(name gen.Atom, token gen.Ref, message any) error {
	var err error
	if p.ov.sendEvent != nil {
		err = p.ov.sendEvent(name, token, message)
	}
	p.put(check.SendEvent{From: p.pid, Name: name, Token: token, Message: message, Error: err})
	return err
}

func (p *Process) SendExit(to gen.PID, reason error) error {
	var err error
	if p.ov.sendExit != nil {
		err = p.ov.sendExit(to, reason)
	}
	p.put(check.SendExit{From: p.pid, To: to, Reason: reason, Error: err})
	return err
}

func (p *Process) SendExitAfter(to gen.PID, reason error, after time.Duration) (gen.CancelFunc, error) {
	cancel, err := gen.CancelFunc(func() bool { return true }), error(nil)
	if p.ov.sendExitAfter != nil {
		cancel, err = p.ov.sendExitAfter(to, reason, after)
	}
	p.put(check.SendExit{From: p.pid, To: to, Reason: reason, Error: err})
	return cancel, err
}

func (p *Process) SendExitMeta(meta gen.Alias, reason error) error {
	var err error
	if p.ov.sendExitMeta != nil {
		err = p.ov.sendExitMeta(meta, reason)
	}
	p.put(check.SendExitMeta{From: p.pid, Meta: meta, Reason: reason, Error: err})
	return err
}

func (p *Process) SendExitMetaAfter(meta gen.Alias, reason error, after time.Duration) (gen.CancelFunc, error) {
	cancel, err := gen.CancelFunc(func() bool { return true }), error(nil)
	if p.ov.sendExitMetaAfter != nil {
		cancel, err = p.ov.sendExitMetaAfter(meta, reason, after)
	}
	p.put(check.SendExitMeta{From: p.pid, Meta: meta, Reason: reason, Error: err})
	return cancel, err
}

func (p *Process) SendResponse(to gen.PID, ref gen.Ref, message any) error {
	var err error
	if p.ov.sendResponse != nil {
		err = p.ov.sendResponse(to, ref, message)
	}
	p.put(check.SendResponse{From: p.pid, To: to, Ref: ref, Message: message, Error: err})
	return err
}

func (p *Process) SendResponseImportant(to gen.PID, ref gen.Ref, message any) error {
	var err error
	if p.ov.sendResponseImportant != nil {
		err = p.ov.sendResponseImportant(to, ref, message)
	}
	p.put(check.SendResponse{From: p.pid, To: to, Ref: ref, Message: message, Options: gen.MessageOptions{ImportantDelivery: true}, Error: err})
	return err
}

func (p *Process) SendResponseError(to gen.PID, ref gen.Ref, err error) error {
	var rerr error
	if p.ov.sendResponseError != nil {
		rerr = p.ov.sendResponseError(to, ref, err)
	}
	p.put(check.SendResponse{From: p.pid, To: to, Ref: ref, Message: err, Error: rerr})
	return rerr
}

func (p *Process) SendResponseErrorImportant(to gen.PID, ref gen.Ref, err error) error {
	var rerr error
	if p.ov.sendResponseErrorImportant != nil {
		rerr = p.ov.sendResponseErrorImportant(to, ref, err)
	}
	p.put(check.SendResponse{From: p.pid, To: to, Ref: ref, Message: err, Options: gen.MessageOptions{ImportantDelivery: true}, Error: rerr})
	return rerr
}

func (p *Process) Call(to any, message any) (any, error) {
	var resp any
	var err error
	if p.ov.call != nil {
		resp, err = p.ov.call(to, message)
	}
	p.put(check.Call{From: p.pid, To: to, Request: message, Response: resp, Error: err})
	return resp, err
}

func (p *Process) CallWithTimeout(to any, message any, timeout int) (any, error) {
	var resp any
	var err error
	if p.ov.callWithTimeout != nil {
		resp, err = p.ov.callWithTimeout(to, message, timeout)
	}
	p.put(check.Call{From: p.pid, To: to, Request: message, Response: resp, Error: err})
	return resp, err
}

func (p *Process) CallWithPriority(to any, message any, priority gen.MessagePriority) (any, error) {
	var resp any
	var err error
	if p.ov.callWithPriority != nil {
		resp, err = p.ov.callWithPriority(to, message, priority)
	}
	p.put(check.Call{From: p.pid, To: to, Request: message, Response: resp, Error: err})
	return resp, err
}

func (p *Process) CallImportant(to any, message any) (any, error) {
	var resp any
	var err error
	if p.ov.callImportant != nil {
		resp, err = p.ov.callImportant(to, message)
	}
	p.put(check.Call{From: p.pid, To: to, Request: message, Response: resp, Error: err})
	return resp, err
}

func (p *Process) CallPID(to gen.PID, message any, timeout int) (any, error) {
	var resp any
	var err error
	if p.ov.callPID != nil {
		resp, err = p.ov.callPID(to, message, timeout)
	}
	p.put(check.Call{From: p.pid, To: to, Request: message, Response: resp, Error: err})
	return resp, err
}

func (p *Process) CallProcessID(to gen.ProcessID, message any, timeout int) (any, error) {
	var resp any
	var err error
	if p.ov.callProcessID != nil {
		resp, err = p.ov.callProcessID(to, message, timeout)
	}
	p.put(check.Call{From: p.pid, To: to, Request: message, Response: resp, Error: err})
	return resp, err
}

func (p *Process) CallAlias(to gen.Alias, message any, timeout int) (any, error) {
	var resp any
	var err error
	if p.ov.callAlias != nil {
		resp, err = p.ov.callAlias(to, message, timeout)
	}
	p.put(check.Call{From: p.pid, To: to, Request: message, Response: resp, Error: err})
	return resp, err
}

func (p *Process) Inspect(target gen.PID, item ...string) (map[string]string, error) {
	if p.ov.inspect != nil {
		return p.ov.inspect(target, item...)
	}
	return nil, nil
}

func (p *Process) InspectMeta(meta gen.Alias, item ...string) (map[string]string, error) {
	if p.ov.inspectMeta != nil {
		return p.ov.inspectMeta(meta, item...)
	}
	return nil, nil
}

func (p *Process) RegisterEvent(name gen.Atom, options gen.EventOptions) (gen.Ref, error) {
	ref, err := synthRef(p.next.Add(1)), error(nil)
	if p.ov.registerEvent != nil {
		ref, err = p.ov.registerEvent(name, options)
	}
	p.put(check.RegisterEvent{PID: p.pid, Name: name, Ref: ref, Error: err})
	return ref, err
}

func (p *Process) UnregisterEvent(name gen.Atom) error {
	var err error
	if p.ov.unregisterEvent != nil {
		err = p.ov.unregisterEvent(name)
	}
	p.put(check.UnregisterEvent{PID: p.pid, Name: name, Error: err})
	return err
}

func (p *Process) Link(target any) error {
	var err error
	if p.ov.link != nil {
		err = p.ov.link(target)
	}
	p.put(check.Link{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) Unlink(target any) error {
	var err error
	if p.ov.unlink != nil {
		err = p.ov.unlink(target)
	}
	p.put(check.Unlink{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) LinkPID(target gen.PID) error {
	var err error
	if p.ov.linkPID != nil {
		err = p.ov.linkPID(target)
	}
	p.put(check.Link{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) UnlinkPID(target gen.PID) error {
	var err error
	if p.ov.unlinkPID != nil {
		err = p.ov.unlinkPID(target)
	}
	p.put(check.Unlink{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) LinkProcessID(target gen.ProcessID) error {
	var err error
	if p.ov.linkProcessID != nil {
		err = p.ov.linkProcessID(target)
	}
	p.put(check.Link{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) UnlinkProcessID(target gen.ProcessID) error {
	var err error
	if p.ov.unlinkProcessID != nil {
		err = p.ov.unlinkProcessID(target)
	}
	p.put(check.Unlink{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) LinkAlias(target gen.Alias) error {
	var err error
	if p.ov.linkAlias != nil {
		err = p.ov.linkAlias(target)
	}
	p.put(check.Link{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) UnlinkAlias(target gen.Alias) error {
	var err error
	if p.ov.unlinkAlias != nil {
		err = p.ov.unlinkAlias(target)
	}
	p.put(check.Unlink{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) LinkEvent(target gen.Event) ([]gen.MessageEvent, error) {
	var events []gen.MessageEvent
	var err error
	if p.ov.linkEvent != nil {
		events, err = p.ov.linkEvent(target)
	}
	p.put(check.Link{From: p.pid, Target: target, Error: err})
	return events, err
}

func (p *Process) UnlinkEvent(target gen.Event) error {
	var err error
	if p.ov.unlinkEvent != nil {
		err = p.ov.unlinkEvent(target)
	}
	p.put(check.Unlink{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) LinkNode(target gen.Atom) error {
	var err error
	if p.ov.linkNode != nil {
		err = p.ov.linkNode(target)
	}
	p.put(check.Link{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) UnlinkNode(target gen.Atom) error {
	var err error
	if p.ov.unlinkNode != nil {
		err = p.ov.unlinkNode(target)
	}
	p.put(check.Unlink{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) Monitor(target any) error {
	var err error
	if p.ov.monitor != nil {
		err = p.ov.monitor(target)
	}
	p.put(check.Monitor{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) Demonitor(target any) error {
	var err error
	if p.ov.demonitor != nil {
		err = p.ov.demonitor(target)
	}
	p.put(check.Demonitor{From: p.pid, Target: target, Error: err})
	return err
}

func (p *Process) MonitorPID(pid gen.PID) error {
	var err error
	if p.ov.monitorPID != nil {
		err = p.ov.monitorPID(pid)
	}
	p.put(check.Monitor{From: p.pid, Target: pid, Error: err})
	return err
}

func (p *Process) DemonitorPID(pid gen.PID) error {
	var err error
	if p.ov.demonitorPID != nil {
		err = p.ov.demonitorPID(pid)
	}
	p.put(check.Demonitor{From: p.pid, Target: pid, Error: err})
	return err
}

func (p *Process) MonitorProcessID(process gen.ProcessID) error {
	var err error
	if p.ov.monitorProcessID != nil {
		err = p.ov.monitorProcessID(process)
	}
	p.put(check.Monitor{From: p.pid, Target: process, Error: err})
	return err
}

func (p *Process) DemonitorProcessID(process gen.ProcessID) error {
	var err error
	if p.ov.demonitorProcessID != nil {
		err = p.ov.demonitorProcessID(process)
	}
	p.put(check.Demonitor{From: p.pid, Target: process, Error: err})
	return err
}

func (p *Process) MonitorAlias(alias gen.Alias) error {
	var err error
	if p.ov.monitorAlias != nil {
		err = p.ov.monitorAlias(alias)
	}
	p.put(check.Monitor{From: p.pid, Target: alias, Error: err})
	return err
}

func (p *Process) DemonitorAlias(alias gen.Alias) error {
	var err error
	if p.ov.demonitorAlias != nil {
		err = p.ov.demonitorAlias(alias)
	}
	p.put(check.Demonitor{From: p.pid, Target: alias, Error: err})
	return err
}

func (p *Process) MonitorEvent(event gen.Event) ([]gen.MessageEvent, error) {
	var events []gen.MessageEvent
	var err error
	if p.ov.monitorEvent != nil {
		events, err = p.ov.monitorEvent(event)
	}
	p.put(check.Monitor{From: p.pid, Target: event, Error: err})
	return events, err
}

func (p *Process) DemonitorEvent(event gen.Event) error {
	var err error
	if p.ov.demonitorEvent != nil {
		err = p.ov.demonitorEvent(event)
	}
	p.put(check.Demonitor{From: p.pid, Target: event, Error: err})
	return err
}

func (p *Process) MonitorNode(node gen.Atom) error {
	var err error
	if p.ov.monitorNode != nil {
		err = p.ov.monitorNode(node)
	}
	p.put(check.Monitor{From: p.pid, Target: node, Error: err})
	return err
}

func (p *Process) DemonitorNode(node gen.Atom) error {
	var err error
	if p.ov.demonitorNode != nil {
		err = p.ov.demonitorNode(node)
	}
	p.put(check.Demonitor{From: p.pid, Target: node, Error: err})
	return err
}

func (p *Process) Log() gen.Log {
	if p.ov.log != nil {
		return p.ov.log()
	}
	return p.log
}

func (p *Process) Info() (gen.ProcessInfo, error) {
	if p.ov.info != nil {
		return p.ov.info()
	}
	return gen.ProcessInfo{}, nil
}

func (p *Process) MetaInfo(meta gen.Alias) (gen.MetaInfo, error) {
	if p.ov.metaInfo != nil {
		return p.ov.metaInfo(meta)
	}
	return gen.MetaInfo{}, nil
}

func (p *Process) Mailbox() gen.ProcessMailbox {
	if p.ov.mailbox != nil {
		return p.ov.mailbox()
	}
	return p.mailbox
}

func (p *Process) Behavior() gen.ProcessBehavior {
	if p.ov.behavior != nil {
		return p.ov.behavior()
	}
	return nil
}

func (p *Process) BehaviorName() string {
	if p.ov.behaviorName != nil {
		return p.ov.behaviorName()
	}
	return ""
}

func (p *Process) PropagatingTrace() gen.Tracing {
	if p.ov.propagatingTrace != nil {
		return p.ov.propagatingTrace()
	}
	return gen.Tracing{}
}

func (p *Process) SetPropagatingTrace(t gen.Tracing) {
	if p.ov.setPropagatingTrace != nil {
		p.ov.setPropagatingTrace(t)
	}
}

func (p *Process) SetTracingAttribute(key, value string) {
	if p.ov.setTracingAttribute != nil {
		p.ov.setTracingAttribute(key, value)
	}
}

func (p *Process) RemoveTracingAttribute(key string) {
	if p.ov.removeTracingAttribute != nil {
		p.ov.removeTracingAttribute(key)
	}
}

func (p *Process) SetTracingSpanAttribute(key, value string) {
	if p.ov.setTracingSpanAttribute != nil {
		p.ov.setTracingSpanAttribute(key, value)
	}
}

func (p *Process) TracingAttributes() []gen.TracingAttribute {
	if p.ov.tracingAttributes != nil {
		return p.ov.tracingAttributes()
	}
	return nil
}

func (p *Process) ClearTracingSpanAttributes() {
	if p.ov.clearTracingSpanAttributes != nil {
		p.ov.clearTracingSpanAttributes()
	}
}

func (p *Process) SendTracingSpan(span gen.TracingSpan) {
	if p.ov.sendTracingSpan != nil {
		p.ov.sendTracingSpan(span)
	}
}

func (p *Process) StartTracingSpan(name string) gen.TracingSpanScope {
	return gen.TracingSpanScopeNoop
}

func (p *Process) CloseTracingSpans() {}

func (p *Process) Forward(to gen.PID, message *gen.MailboxMessage, priority gen.MessagePriority) error {
	var err error
	if p.ov.forward != nil {
		err = p.ov.forward(to, message, priority)
	}
	var from gen.PID
	var inner any
	if message != nil {
		from = message.From
		inner = message.Message
	}
	p.put(check.Forward{By: p.pid, To: to, From: from, Message: inner, Error: err})
	return err
}
