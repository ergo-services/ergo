package unit

import (
	"strings"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/testing/check"
)

// mockProcess is the mocked gen.Process handed to the behavior under test. Its
// outbound operations delegate to the node (record + stub) with its own pid; its
// accessors return the configured identity; Node() returns the mock node.
//
// Every non-egress method first consults its override (see processOverrides and the
// Subject.On<Method> setters in process_overrides.go); when unset it falls back to
// the default. Egress methods (Send/Call/Spawn/Link/...) keep the typed stub sugar.
type mockProcess struct {
	node     *mockNode
	pid      gen.PID
	parent   gen.PID
	leader   gen.PID
	name     gen.Atom
	state    gen.ProcessState
	behavior gen.ProcessBehavior
	mailbox  gen.ProcessMailbox
	kind     gen.ProcessKind
	log      *mockLog
	env      map[gen.Env]any
	aliases  []gen.Alias
	events   []gen.Atom
	stubs    *stubs // the process's own egress stubs (shared with its Subject)
	ov       processOverrides

	// message-attribute state, managed by the Set*/Send* methods exactly like the
	// real process; every Send builds its gen.MessageOptions from here. Seeded from
	// gen.ProcessOptions at spawn (mirrors node spawn).
	priority    gen.MessagePriority
	keeporder   bool
	important   bool
	compression gen.Compression
}

var _ gen.Process = (*mockProcess)(nil)

func newMockProcess(node *mockNode, register gen.Atom, o gen.ProcessOptions) *mockProcess {
	pid := gen.PID{Node: node.nodeName, ID: 1000, Creation: node.creation}
	parent := node.nodePID() // node spawns the process under test
	leader := o.Leader
	if leader == (gen.PID{}) {
		leader = parent
	}
	level := o.LogLevel
	if level == gen.LogLevelDefault {
		level = node.logLevel
	}
	// process env = node env overlaid by process-specific env (highest priority)
	env := make(map[gen.Env]any, len(node.env)+len(o.Env))
	for k, v := range node.env {
		env[k] = v
	}
	for k, v := range o.Env {
		env[k] = v
	}
	mailbox := o.Mailbox
	if mailbox == nil {
		mailbox = &gen.ProcessMailbox{
			Main:   lib.NewQueueMPSC(),
			System: lib.NewQueueMPSC(),
			Urgent: lib.NewQueueMPSC(),
			Log:    lib.NewQueueMPSC(),
		}
	}
	// compression: adopt options, fill the same defaults the node spawn fills
	compression := o.Compression
	if compression.Level == 0 {
		compression.Level = gen.DefaultCompressionLevel
	}
	if compression.Type == "" {
		compression.Type = gen.DefaultCompressionType
	}
	if compression.Threshold == 0 {
		compression.Threshold = gen.DefaultCompressionThreshold
	}
	return &mockProcess{
		node:        node,
		pid:         pid,
		parent:      parent,
		leader:      leader,
		name:        register,
		state:       gen.ProcessStateInit, // Init during ProcessInit; driver advances it
		log:         newMockLog(node, pid, level),
		env:         env,
		mailbox:     *mailbox,
		priority:    o.SendPriority,
		keeporder:   true,
		important:   o.ImportantDelivery,
		compression: compression,
	}
}

// stateIR reports Init or Running, the gate the real process applies to Set*,
// Spawn, Call, Link, Monitor, and the response/event/delayed sends (ErrNotAllowed
// otherwise).
func (p *mockProcess) stateIR() bool {
	return p.state == gen.ProcessStateInit || p.state == gen.ProcessStateRunning
}

// stateIRT reports Init, Running, or Terminated, the gate the real process applies
// to the immediate Send family and SendExit, since a terminating process may still
// send (node/process.go SendPID/SendExit).
func (p *mockProcess) stateIRT() bool {
	return p.state == gen.ProcessStateInit ||
		p.state == gen.ProcessStateRunning ||
		p.state == gen.ProcessStateTerminated
}

// msgOptions builds the effective gen.MessageOptions from the current state, exactly
// as the real process does for each Send.
func (p *mockProcess) msgOptions() gen.MessageOptions {
	return gen.MessageOptions{
		Priority:          p.priority,
		Compression:       p.compression,
		KeepNetworkOrder:  p.keeporder,
		ImportantDelivery: p.important,
	}
}

// accessors

func (p *mockProcess) Node() gen.Node {
	if p.ov.node != nil {
		return p.ov.node()
	}
	return p.node
}
func (p *mockProcess) Name() gen.Atom {
	if p.ov.name != nil {
		return p.ov.name()
	}
	return p.name
}
func (p *mockProcess) PID() gen.PID {
	if p.ov.pid != nil {
		return p.ov.pid()
	}
	return p.pid
}
func (p *mockProcess) Leader() gen.PID {
	if p.ov.leader != nil {
		return p.ov.leader()
	}
	return p.leader
}
func (p *mockProcess) Parent() gen.PID {
	if p.ov.parent != nil {
		return p.ov.parent()
	}
	return p.parent
}
func (p *mockProcess) Application() gen.Application {
	if p.ov.application != nil {
		return p.ov.application()
	}
	return nil
}
func (p *mockProcess) Uptime() int64 {
	if p.ov.uptime != nil {
		return p.ov.uptime()
	}
	return 0
}
func (p *mockProcess) State() gen.ProcessState {
	if p.ov.state != nil {
		return p.ov.state()
	}
	return p.state
}
func (p *mockProcess) Behavior() gen.ProcessBehavior {
	if p.ov.behavior != nil {
		return p.ov.behavior()
	}
	return p.behavior
}
func (p *mockProcess) BehaviorName() string {
	if p.ov.behaviorName != nil {
		return p.ov.behaviorName()
	}
	return ""
}
func (p *mockProcess) Mailbox() gen.ProcessMailbox {
	if p.ov.mailbox != nil {
		return p.ov.mailbox()
	}
	return p.mailbox
}
func (p *mockProcess) Log() gen.Log {
	if p.ov.log != nil {
		return p.ov.log()
	}
	return p.log
}
func (p *mockProcess) Aliases() []gen.Alias {
	if p.ov.aliases != nil {
		return p.ov.aliases()
	}
	return p.aliases
}
func (p *mockProcess) Events() []gen.Atom {
	if p.ov.events != nil {
		return p.ov.events()
	}
	return p.events
}

func (p *mockProcess) EnvList() map[gen.Env]any {
	if p.ov.envList != nil {
		return p.ov.envList()
	}
	return p.env
}
func (p *mockProcess) SetEnv(name gen.Env, value any) {
	if p.ov.setEnv != nil {
		p.ov.setEnv(name, value)
		return
	}
	if value == nil {
		delete(p.env, name)
		return
	}
	p.env[name] = value
}
func (p *mockProcess) Env(name gen.Env) (any, bool) {
	if p.ov.env != nil {
		return p.ov.env(name)
	}
	v, ok := p.env[name]
	return v, ok
}
func (p *mockProcess) EnvDefault(name gen.Env, def any) any {
	if p.ov.envDefault != nil {
		return p.ov.envDefault(name, def)
	}
	if v, ok := p.env[name]; ok {
		return v
	}
	return def
}

// settings

func (p *mockProcess) Compression() bool {
	if p.ov.compression != nil {
		return p.ov.compression()
	}
	return p.compression.Enable
}
func (p *mockProcess) SetCompression(enabled bool) error {
	if p.ov.setCompression != nil {
		return p.ov.setCompression(enabled)
	}
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	p.compression.Enable = enabled
	return nil
}
func (p *mockProcess) CompressionType() gen.CompressionType {
	if p.ov.compressionType != nil {
		return p.ov.compressionType()
	}
	return p.compression.Type
}
func (p *mockProcess) SetCompressionType(ctype gen.CompressionType) error {
	if p.ov.setCompressionType != nil {
		return p.ov.setCompressionType(ctype)
	}
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	switch ctype {
	case gen.CompressionTypeGZIP, gen.CompressionTypeLZW, gen.CompressionTypeZLIB:
	default:
		return gen.ErrIncorrect
	}
	p.compression.Type = ctype
	return nil
}
func (p *mockProcess) CompressionLevel() gen.CompressionLevel {
	if p.ov.compressionLevel != nil {
		return p.ov.compressionLevel()
	}
	return p.compression.Level
}
func (p *mockProcess) SetCompressionLevel(level gen.CompressionLevel) error {
	if p.ov.setCompressionLevel != nil {
		return p.ov.setCompressionLevel(level)
	}
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	switch level {
	case gen.CompressionBestSize, gen.CompressionBestSpeed, gen.CompressionDefault:
	default:
		return gen.ErrIncorrect
	}
	p.compression.Level = level
	return nil
}
func (p *mockProcess) CompressionThreshold() int {
	if p.ov.compressionThreshold != nil {
		return p.ov.compressionThreshold()
	}
	return p.compression.Threshold
}
func (p *mockProcess) SetCompressionThreshold(threshold int) error {
	if p.ov.setCompressionThreshold != nil {
		return p.ov.setCompressionThreshold(threshold)
	}
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	if threshold < gen.DefaultCompressionThreshold {
		return gen.ErrIncorrect
	}
	p.compression.Threshold = threshold
	return nil
}
func (p *mockProcess) SendPriority() gen.MessagePriority {
	if p.ov.sendPriority != nil {
		return p.ov.sendPriority()
	}
	return p.priority
}
func (p *mockProcess) SetSendPriority(priority gen.MessagePriority) error {
	if p.ov.setSendPriority != nil {
		return p.ov.setSendPriority(priority)
	}
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	switch priority {
	case gen.MessagePriorityNormal, gen.MessagePriorityHigh, gen.MessagePriorityMax:
	default:
		return gen.ErrIncorrect
	}
	p.priority = priority
	return nil
}
func (p *mockProcess) SetProcessKind(kind gen.ProcessKind) error {
	if p.ov.setProcessKind != nil {
		return p.ov.setProcessKind(kind)
	}
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	p.kind = kind
	return nil
}
func (p *mockProcess) SetKeepNetworkOrder(order bool) error {
	if p.ov.setKeepNetworkOrder != nil {
		return p.ov.setKeepNetworkOrder(order)
	}
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	p.keeporder = order
	return nil
}
func (p *mockProcess) KeepNetworkOrder() bool {
	if p.ov.keepNetworkOrder != nil {
		return p.ov.keepNetworkOrder()
	}
	return p.keeporder
}
func (p *mockProcess) SetImportantDelivery(important bool) error {
	if p.ov.setImportantDelivery != nil {
		return p.ov.setImportantDelivery(important)
	}
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	p.important = important
	return nil
}
func (p *mockProcess) ImportantDelivery() bool {
	if p.ov.importantDelivery != nil {
		return p.ov.importantDelivery()
	}
	return p.important
}
func (p *mockProcess) SetTracingSampler(sampler gen.TracingSampler) error {
	if p.ov.setTracingSampler != nil {
		return p.ov.setTracingSampler(sampler)
	}
	return nil
}
func (p *mockProcess) TracingSampler() gen.TracingSampler {
	if p.ov.tracingSampler != nil {
		return p.ov.tracingSampler()
	}
	return nil
}
func (p *mockProcess) RegisterName(name gen.Atom) error {
	if p.ov.registerName != nil {
		return p.ov.registerName(name)
	}
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	if err := p.node.RegisterName(name, p.pid); err != nil {
		return err
	}
	p.name = name
	return nil
}
func (p *mockProcess) UnregisterName() error {
	if p.ov.unregisterName != nil {
		return p.ov.unregisterName()
	}
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	if _, err := p.node.UnregisterName(p.name); err != nil {
		return err
	}
	p.name = ""
	return nil
}

// aliases / events (safe-synthetic / stub)

func (p *mockProcess) CreateAlias() (gen.Alias, error) {
	if p.stateIR() == false {
		return gen.Alias{}, gen.ErrNotAllowed
	}
	a, err := p.node.routeCreateAlias(p.stubs, p.pid)
	if err == nil {
		p.aliases = append(p.aliases, a)
	}
	return a, err
}
func (p *mockProcess) DeleteAlias(alias gen.Alias) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	var err error
	if p.ov.deleteAlias != nil {
		err = p.ov.deleteAlias(alias)
	}
	return p.node.routeDeleteAlias(p.pid, alias, err)
}
func (p *mockProcess) RegisterEvent(name gen.Atom, opts gen.EventOptions) (gen.Ref, error) {
	if p.stateIR() == false {
		return gen.Ref{}, gen.ErrNotAllowed
	}
	ref, err := p.node.routeRegisterEvent(p.stubs, p.pid, name)
	if err == nil {
		p.events = append(p.events, name)
	}
	return ref, err
}
func (p *mockProcess) UnregisterEvent(name gen.Atom) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	var err error
	if p.ov.unregisterEvent != nil {
		err = p.ov.unregisterEvent(name)
	}
	return p.node.routeUnregisterEvent(p.pid, name, err)
}

// send (egress + fail-stub)

func (p *mockProcess) Send(to any, message any) error {
	if p.stateIRT() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeSend(p.stubs, p.pid, to, message, p.msgOptions())
}
func (p *mockProcess) SendPID(to gen.PID, message any) error {
	if p.stateIRT() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeSend(p.stubs, p.pid, to, message, p.msgOptions())
}
func (p *mockProcess) SendProcessID(to gen.ProcessID, message any) error {
	if p.stateIRT() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeSend(p.stubs, p.pid, to, message, p.msgOptions())
}
func (p *mockProcess) SendAlias(to gen.Alias, message any) error {
	if p.stateIRT() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeSend(p.stubs, p.pid, to, message, p.msgOptions())
}

// SendWithPriority overrides the priority for this one send via local options,
// without mutating process state (see audit.md: the save/restore idiom races).
// The real process routes this through Send, so it shares Send's IRT gate.
func (p *mockProcess) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	if p.stateIRT() == false {
		return gen.ErrNotAllowed
	}
	opts := p.msgOptions()
	opts.Priority = priority
	return p.node.routeSend(p.stubs, p.pid, to, message, opts)
}

// SendImportant forces the important-delivery flag for this one send via local
// options, without mutating process state. Important delivery needs a round-trip
// confirmation, so the real process gates it on Init|Running only (not Terminated).
func (p *mockProcess) SendImportant(to any, message any) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	opts := p.msgOptions()
	opts.ImportantDelivery = true
	return p.node.routeSend(p.stubs, p.pid, to, message, opts)
}
func (p *mockProcess) SendEvent(name gen.Atom, token gen.Ref, message any) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeSendEvent(p.pid, name, token, message, p.msgOptions())
}

// delayed sends (timers)

func (p *mockProcess) SendAfter(to any, message any, after time.Duration) (gen.CancelFunc, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return p.node.schedule(p.pid, to, message, after, p.msgOptions()), nil
}
func (p *mockProcess) SendWithPriorityAfter(to any, message any, priority gen.MessagePriority, after time.Duration) (gen.CancelFunc, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	opts := p.msgOptions()
	opts.Priority = priority
	return p.node.schedule(p.pid, to, message, after, opts), nil
}

func (p *mockProcess) SendEvery(to any, message any, period time.Duration) (gen.CancelFunc, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return p.node.schedule(p.pid, to, message, period, p.msgOptions()), nil
}

func (p *mockProcess) SendWithPriorityEvery(to any, message any, priority gen.MessagePriority, period time.Duration) (gen.CancelFunc, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	opts := p.msgOptions()
	opts.Priority = priority
	return p.node.schedule(p.pid, to, message, period, opts), nil
}
func (p *mockProcess) SendExitAfter(to gen.PID, reason error, after time.Duration) (gen.CancelFunc, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return p.node.schedule(p.pid, to, gen.MessageExitPID{PID: p.pid, Reason: reason}, after, p.msgOptions()), nil
}
func (p *mockProcess) SendExitMetaAfter(meta gen.Alias, reason error, after time.Duration) (gen.CancelFunc, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return p.node.schedule(p.pid, meta, gen.MessageExitAlias{Alias: meta, Reason: reason}, after, p.msgOptions()), nil
}

// exit

func (p *mockProcess) SendExit(to gen.PID, reason error) error {
	if p.stateIRT() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeSendExit(p.stubs, p.pid, to, reason)
}
func (p *mockProcess) SendExitMeta(meta gen.Alias, reason error) error {
	if p.stateIRT() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeSendExitMeta(p.stubs, p.pid, meta, reason)
}

// responses

func (p *mockProcess) SendResponse(to gen.PID, ref gen.Ref, message any) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeSendResponse(p.pid, to, ref, message, p.msgOptions())
}
func (p *mockProcess) SendResponseImportant(to gen.PID, ref gen.Ref, message any) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	opts := p.msgOptions()
	opts.ImportantDelivery = true
	return p.node.routeSendResponse(p.pid, to, ref, message, opts)
}
func (p *mockProcess) SendResponseError(to gen.PID, ref gen.Ref, err error) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeSendResponse(p.pid, to, ref, err, p.msgOptions())
}
func (p *mockProcess) SendResponseErrorImportant(to gen.PID, ref gen.Ref, err error) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	opts := p.msgOptions()
	opts.ImportantDelivery = true
	return p.node.routeSendResponse(p.pid, to, ref, err, opts)
}

// calls (tier 3: strict stub)

func (p *mockProcess) Call(to any, message any) (any, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return p.node.routeCall(p.stubs, p.pid, to, message)
}
func (p *mockProcess) CallWithTimeout(to any, message any, timeout int) (any, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return p.node.routeCall(p.stubs, p.pid, to, message)
}
func (p *mockProcess) CallWithPriority(to any, message any, priority gen.MessagePriority) (any, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return p.node.routeCall(p.stubs, p.pid, to, message)
}
func (p *mockProcess) CallImportant(to any, message any) (any, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return p.node.routeCall(p.stubs, p.pid, to, message)
}
func (p *mockProcess) CallPID(to gen.PID, message any, timeout int) (any, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return p.node.routeCall(p.stubs, p.pid, to, message)
}
func (p *mockProcess) CallProcessID(to gen.ProcessID, message any, timeout int) (any, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return p.node.routeCall(p.stubs, p.pid, to, message)
}
func (p *mockProcess) CallAlias(to gen.Alias, message any, timeout int) (any, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return p.node.routeCall(p.stubs, p.pid, to, message)
}

// spawn (safe-synthetic / stub)

func (p *mockProcess) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	if p.stateIR() == false {
		return gen.PID{}, gen.ErrNotAllowed
	}
	return p.node.routeSpawn(p.stubs, p.pid, "", factory, options)
}
func (p *mockProcess) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	if p.stateIR() == false {
		return gen.PID{}, gen.ErrNotAllowed
	}
	return p.node.routeSpawn(p.stubs, p.pid, register, factory, options)
}
func (p *mockProcess) SpawnMeta(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	if p.stateIR() == false {
		return gen.Alias{}, gen.ErrNotAllowed
	}
	return p.node.routeSpawnMeta(p.stubs, p.pid, behavior)
}
func (p *mockProcess) RemoteSpawn(node gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	if p.stateIR() == false {
		return gen.PID{}, gen.ErrNotAllowed
	}
	return p.node.routeRemoteSpawn(p.stubs, p.pid, node, name, "", options)
}
func (p *mockProcess) RemoteSpawnRegister(node gen.Atom, name gen.Atom, register gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	if p.stateIR() == false {
		return gen.PID{}, gen.ErrNotAllowed
	}
	return p.node.routeRemoteSpawn(p.stubs, p.pid, node, name, register, options)
}

// links / monitors (egress + fail-stub)

func (p *mockProcess) Link(target any) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeLink(p.stubs, p.pid, target)
}
func (p *mockProcess) Unlink(target any) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeUnlink(p.stubs, p.pid, target)
}
func (p *mockProcess) LinkPID(target gen.PID) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	if target == p.pid {
		return gen.ErrNotAllowed // mirrors the real runtime: no self-link
	}
	return p.node.routeLink(p.stubs, p.pid, target)
}
func (p *mockProcess) UnlinkPID(target gen.PID) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeUnlink(p.stubs, p.pid, target)
}
func (p *mockProcess) LinkProcessID(target gen.ProcessID) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeLink(p.stubs, p.pid, target)
}
func (p *mockProcess) UnlinkProcessID(target gen.ProcessID) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeUnlink(p.stubs, p.pid, target)
}
func (p *mockProcess) LinkAlias(target gen.Alias) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeLink(p.stubs, p.pid, target)
}
func (p *mockProcess) UnlinkAlias(target gen.Alias) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeUnlink(p.stubs, p.pid, target)
}
func (p *mockProcess) LinkEvent(target gen.Event) ([]gen.MessageEvent, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return nil, p.node.routeLink(p.stubs, p.pid, target)
}
func (p *mockProcess) UnlinkEvent(target gen.Event) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeUnlink(p.stubs, p.pid, target)
}
func (p *mockProcess) LinkNode(target gen.Atom) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeLink(p.stubs, p.pid, target)
}
func (p *mockProcess) UnlinkNode(target gen.Atom) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeUnlink(p.stubs, p.pid, target)
}

func (p *mockProcess) Monitor(target any) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeMonitor(p.stubs, p.pid, target)
}
func (p *mockProcess) Demonitor(target any) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeDemonitor(p.stubs, p.pid, target)
}
func (p *mockProcess) MonitorPID(pid gen.PID) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeMonitor(p.stubs, p.pid, pid)
}
func (p *mockProcess) DemonitorPID(pid gen.PID) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeDemonitor(p.stubs, p.pid, pid)
}
func (p *mockProcess) MonitorProcessID(process gen.ProcessID) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeMonitor(p.stubs, p.pid, process)
}
func (p *mockProcess) DemonitorProcessID(process gen.ProcessID) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeDemonitor(p.stubs, p.pid, process)
}
func (p *mockProcess) MonitorAlias(alias gen.Alias) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeMonitor(p.stubs, p.pid, alias)
}
func (p *mockProcess) DemonitorAlias(alias gen.Alias) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeDemonitor(p.stubs, p.pid, alias)
}
func (p *mockProcess) MonitorEvent(event gen.Event) ([]gen.MessageEvent, error) {
	if p.stateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	return nil, p.node.routeMonitor(p.stubs, p.pid, event)
}
func (p *mockProcess) DemonitorEvent(event gen.Event) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeDemonitor(p.stubs, p.pid, event)
}
func (p *mockProcess) MonitorNode(node gen.Atom) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeMonitor(p.stubs, p.pid, node)
}
func (p *mockProcess) DemonitorNode(node gen.Atom) error {
	if p.stateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.node.routeDemonitor(p.stubs, p.pid, node)
}

// inspect / info

func (p *mockProcess) Inspect(target gen.PID, item ...string) (map[string]string, error) {
	if p.ov.inspect != nil {
		return p.ov.inspect(target, item...)
	}
	p.node.unsupported("Inspect")
	return nil, nil
}
func (p *mockProcess) InspectMeta(meta gen.Alias, item ...string) (map[string]string, error) {
	if p.ov.inspectMeta != nil {
		return p.ov.inspectMeta(meta, item...)
	}
	p.node.unsupported("InspectMeta")
	return nil, nil
}
func (p *mockProcess) Info() (gen.ProcessInfo, error) {
	if p.ov.info != nil {
		return p.ov.info()
	}
	if p.stateIR() == false {
		return gen.ProcessInfo{}, gen.ErrNotAllowed
	}
	return gen.ProcessInfo{PID: p.pid, Name: p.name, Parent: p.parent, Leader: p.leader, State: p.state, Env: p.env}, nil
}
func (p *mockProcess) MetaInfo(meta gen.Alias) (gen.MetaInfo, error) {
	if p.ov.metaInfo != nil {
		return p.ov.metaInfo(meta)
	}
	return gen.MetaInfo{}, nil
}

// tracing

func (p *mockProcess) PropagatingTrace() gen.Tracing {
	if p.ov.propagatingTrace != nil {
		return p.ov.propagatingTrace()
	}
	return gen.Tracing{}
}
func (p *mockProcess) SetPropagatingTrace(t gen.Tracing) {
	if p.ov.setPropagatingTrace != nil {
		p.ov.setPropagatingTrace(t)
		return
	}
}
func (p *mockProcess) SetTracingAttribute(key, value string) {
	if p.ov.setTracingAttribute != nil {
		p.ov.setTracingAttribute(key, value)
		return
	}
}
func (p *mockProcess) RemoveTracingAttribute(key string) {
	if p.ov.removeTracingAttribute != nil {
		p.ov.removeTracingAttribute(key)
		return
	}
}
func (p *mockProcess) SetTracingSpanAttribute(key, value string) {
	if p.ov.setTracingSpanAttribute != nil {
		p.ov.setTracingSpanAttribute(key, value)
		return
	}
}
func (p *mockProcess) TracingAttributes() []gen.TracingAttribute {
	if p.ov.tracingAttributes != nil {
		return p.ov.tracingAttributes()
	}
	return nil
}
func (p *mockProcess) ClearTracingSpanAttributes() {
	if p.ov.clearTracingSpanAttributes != nil {
		p.ov.clearTracingSpanAttributes()
		return
	}
}
func (p *mockProcess) SendTracingSpan(span gen.TracingSpan) {
	if p.ov.sendTracingSpan != nil {
		p.ov.sendTracingSpan(span)
		return
	}
}

func (p *mockProcess) StartTracingSpan(name string) gen.TracingSpanScope {
	return &unitSpanScope{p: p, name: name}
}

func (p *mockProcess) CloseTracingSpans() {}

// unitSpanScope records a business span as a check.Span when closed, so a test can
// assert the actor's tracing instrumentation via subject.ShouldSpan().Named(...).
// Unlike the live runtime it does not depend on a sampler: the actor's intent to
// open and close a span is always observed, like every other egress. The span is
// recorded on explicit End/EndError (the unit harness has no handler loop to
// auto-close a forgotten span; that path is covered by the live harness).
type unitSpanScope struct {
	p     *mockProcess
	name  string
	attrs []gen.TracingAttribute
	ended bool
}

func (s *unitSpanScope) SetAttribute(key, value string) {
	if strings.HasPrefix(key, "ergo.") {
		return
	}
	for i := range s.attrs {
		if s.attrs[i].Key == key {
			s.attrs[i].Value = value
			return
		}
	}
	s.attrs = append(s.attrs, gen.TracingAttribute{Key: key, Value: value})
}

func (s *unitSpanScope) End() { s.record("") }

func (s *unitSpanScope) EndError(err error) {
	if err == nil {
		s.record("")
		return
	}
	s.record(err.Error())
}

func (s *unitSpanScope) record(errStr string) {
	if s.ended {
		return
	}
	s.ended = true
	s.p.node.rec.Put(check.Span{
		From:       s.p.pid,
		Name:       s.name,
		Point:      gen.TracingPointSpan,
		Attributes: s.attrs,
		Error:      errStr,
	})
}

// forward

func (p *mockProcess) Forward(to gen.PID, message *gen.MailboxMessage, priority gen.MessagePriority) error {
	return p.node.routeForward(p.stubs, p.pid, to, message.From, message.Message)
}
