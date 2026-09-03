package node

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type process struct {
	node *node
	// core is the routing surface for egress; equals node.core (the node itself,
	// or a decorator in testing/stage). Internals (spawn, registry, logging) still
	// go through node directly.
	core gen.Core
	pid  gen.PID

	name        gen.Atom
	registered  atomic.Bool
	application gen.Application

	// used for the process Uptime method only. PID value uses node creation value.
	creation int64

	// registered aliases
	aliases []gen.Alias

	behavior  gen.ProcessBehavior
	sbehavior string
	kind      gen.ProcessKind

	state        int32
	stateEntered int64 // Unix nanoseconds when current state was entered

	parent   gen.PID
	leader   gen.PID
	fallback gen.ProcessFallback

	mailbox         gen.ProcessMailbox
	priority        atomic.Int32 // gen.MessagePriority; mutable from node-level setters
	keeporder       atomic.Bool
	important       atomic.Bool
	preserveMailbox bool

	messagesIn  uint64
	messagesOut uint64
	runningTime uint64
	initTime    uint64
	wakeups     uint64

	compression      atomic.Pointer[gen.Compression]
	tracing          gen.Tracing
	tracingSampler   atomic.Pointer[gen.TracingSampler]
	tracingAttrs     atomic.Pointer[[]gen.TracingAttribute] // permanent, COW; pointer always non-nil
	tracingSpanAttrs []gen.TracingAttribute                 // one-shot, nil after handler
	tracingSpanStack []*tracingSpanScope                    // open business spans (handler goroutine only)

	env sync.Map

	// channel for the sync requests made this process
	response chan response

	// meta processes
	metas sync.Map // metas[Alias] -> *meta

	// gen.Log interface
	log *log

	// if act as a logger
	loggername  string
	loggerlevel gen.LogLevel

	// if act as a tracing exporter
	tracingExporterName atomic.Pointer[string] // mutable from node-level setters
}

type response struct {
	message   any
	err       error
	ref       gen.Ref
	from      gen.PID
	important bool
}

// gen.Process implementation

func (p *process) Node() gen.Node {
	return p.node
}

func (p *process) Name() gen.Atom {
	return p.name
}

func (p *process) PID() gen.PID {
	return p.pid
}

func (p *process) Leader() gen.PID {
	return p.leader
}

func (p *process) Application() gen.Application {
	return p.application
}

// appName returns the application name or empty atom if app is nil.
func appName(a gen.Application) gen.Atom {
	if a == nil {
		return ""
	}
	return a.Name()
}

func (p *process) Parent() gen.PID {
	return p.parent
}

// shortInfo builds the essential information about this process.
func (p *process) shortInfo() gen.ProcessShortInfo {
	messagesMailbox := p.mailbox.Main.Len() +
		p.mailbox.System.Len() +
		p.mailbox.Urgent.Len() +
		p.mailbox.Log.Len()

	return gen.ProcessShortInfo{
		PID:             p.pid,
		Name:            p.name,
		Application:     appName(p.application),
		Behavior:        p.sbehavior,
		Kind:            p.kind,
		MessagesIn:      atomic.LoadUint64(&p.messagesIn),
		MessagesOut:     atomic.LoadUint64(&p.messagesOut),
		MessagesMailbox: uint64(messagesMailbox),
		MailboxLatency:  p.mailbox.Latency(),
		RunningTime:     atomic.LoadUint64(&p.runningTime),
		InitTime:        atomic.LoadUint64(&p.initTime),
		Wakeups:         atomic.LoadUint64(&p.wakeups),
		Uptime:          p.Uptime(),
		State:           p.State(),
		StateTime:       time.Now().UnixNano() - atomic.LoadInt64(&p.stateEntered),
		Parent:          p.parent,
		Leader:          p.leader,
		LogLevel:        p.log.Level(),
	}
}

func (p *process) ShortInfo() (gen.ProcessShortInfo, error) {
	if p.isStateIRT() == false {
		return gen.ProcessShortInfo{}, gen.ErrNotAllowed
	}
	return p.shortInfo(), nil
}

func (p *process) Uptime() int64 {
	return time.Now().Unix() - p.creation
}

func (p *process) Spawn(
	factory gen.ProcessFactory,
	options gen.ProcessOptions,
	args ...any,
) (gen.PID, error) {
	if p.isStateIR() == false {
		return gen.PID{}, gen.ErrNotAllowed
	}

	// calculate deadline
	timeout := options.InitTimeout
	if timeout == 0 {
		timeout = gen.DefaultRequestTimeout
	}
	deadline := time.Now().Unix() + int64(timeout)
	ref, err := p.node.MakeRefWithDeadline(deadline)
	if err != nil {
		return gen.PID{}, err
	}

	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		ParentPID:      p.pid,
		ParentLeader:   p.leader,
		ParentEnv:      p.EnvList(),
		ParentLogLevel: p.log.Level(),
		Application:    appName(p.application),
		Args:           args,
		Ref:            ref,
	}

	opts.Tracing = p.tracing

	pid, err := p.node.spawn(factory, opts)
	if err != nil {
		return pid, err
	}

	if options.LinkChild {
		// method LinkPID is not allowed to be used in the initialization state,
		// so we use linking manually.
		p.node.targets.LinkPID(p.pid, pid)
	}
	return pid, err
}

func (p *process) SpawnRegister(
	register gen.Atom,
	factory gen.ProcessFactory,
	options gen.ProcessOptions,
	args ...any,
) (gen.PID, error) {

	if p.isStateIR() == false {
		return gen.PID{}, gen.ErrNotAllowed
	}

	// calculate deadline
	timeout := options.InitTimeout
	if timeout == 0 {
		timeout = gen.DefaultRequestTimeout
	}
	deadline := time.Now().Unix() + int64(timeout)
	ref, err := p.node.MakeRefWithDeadline(deadline)
	if err != nil {
		return gen.PID{}, err
	}

	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		ParentPID:      p.pid,
		ParentLeader:   p.leader,
		ParentEnv:      p.EnvList(),
		Register:       register,
		ParentLogLevel: p.log.Level(),
		Application:    appName(p.application),
		Args:           args,
		Ref:            ref,
	}

	opts.Tracing = p.tracing

	pid, err := p.node.spawn(factory, opts)
	if err != nil {
		return pid, err
	}

	if options.LinkChild {
		// method LinkPID is not allowed to be used in the initialization state,
		// so we use linking manually.
		p.node.targets.LinkPID(p.pid, pid)
	}
	return pid, err
}

func (p *process) SpawnMeta(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	var alias gen.Alias
	// use isStateIR - meta can spawn in Init or Running
	if p.isStateIR() == false {
		return alias, gen.ErrNotAllowed
	}
	return p.spawnMeta(behavior, options)
}

func (p *process) spawnMeta(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	var alias gen.Alias

	if behavior == nil {
		return alias, errors.New("behavior is nil")
	}

	m := &meta{
		p:        p,
		behavior: behavior,
	}
	m.compression.Store(options.Compression)
	switch options.SendPriority {
	case gen.MessagePriorityHigh:
		m.priority.Store(int32(gen.MessagePriorityHigh))
	case gen.MessagePriorityMax:
		m.priority.Store(int32(gen.MessagePriorityMax))
	default:
		m.priority.Store(int32(gen.MessagePriorityNormal))
	}
	if options.MailboxSize > 0 {
		m.main = lib.NewQueueLimitMPSC(options.MailboxSize)
		m.system = lib.NewQueueLimitMPSC(options.MailboxSize)
	} else {
		m.main = lib.NewQueueMPSC()
		m.system = lib.NewQueueMPSC()
	}

	m.id = gen.Alias(p.node.MakeRef())
	m.sbehavior = strings.TrimPrefix(reflect.TypeOf(behavior).String(), "*")

	if options.LogLevel == gen.LogLevelDefault {
		options.LogLevel = p.log.Level()
	}
	m.log = createLog(options.LogLevel, p.node.dolog)
	logSource := gen.MessageLogMeta{
		Node:     p.node.name,
		Parent:   p.pid,
		Meta:     m.id,
		Behavior: m.sbehavior,
	}
	m.log.setSource(logSource)

	// register to be able routing messages to this meta process
	if _, exist := p.metas.LoadOrStore(m.id, m); exist {
		return alias, gen.ErrTaken
	}
	if err := p.node.registerAlias(m.id, p); err != nil {
		p.metas.Delete(m.id)
		return alias, err
	}

	if err := m.init(); err != nil {
		p.node.unregisterAlias(m.id, p)
		p.metas.Delete(m.id)
		return alias, err
	}

	go m.start()

	return m.id, nil
}

func (p *process) RemoteSpawn(
	node gen.Atom,
	name gen.Atom,
	options gen.ProcessOptions,
	args ...any,
) (gen.PID, error) {

	if p.isStateIR() == false {
		return gen.PID{}, gen.ErrNotAllowed
	}

	if p.node.Name() == node {
		return gen.PID{}, gen.ErrNotAllowed
	}

	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		ParentPID:      p.pid,
		ParentLeader:   p.leader,
		ParentLogLevel: p.log.Level(),
		Application:    appName(p.application),
		Args:           args,
	}
	if p.node.Security().ExposeEnvRemoteSpawn {
		opts.ParentEnv = p.EnvList()
	}
	pid, err := p.core.RouteSpawn(node, name, opts, p.Node().Name())
	if err != nil {
		return gen.PID{}, err
	}

	if opts.LinkChild {
		// method LinkPID is not allowed to be used in the initialization state,
		// so we use linking manually.
		p.node.targets.LinkPID(p.pid, pid)
	}

	return pid, err
}

func (p *process) RemoteSpawnRegister(
	node gen.Atom,
	name gen.Atom,
	register gen.Atom,
	options gen.ProcessOptions,
	args ...any,
) (gen.PID, error) {

	if p.isStateIR() == false {
		return gen.PID{}, gen.ErrNotAllowed
	}

	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		ParentPID:      p.pid,
		ParentLeader:   p.leader,
		Register:       register,
		ParentLogLevel: p.log.Level(),
		Application:    appName(p.application),
		Args:           args,
	}
	if p.node.Security().ExposeEnvRemoteSpawn {
		opts.ParentEnv = p.EnvList()
	}
	pid, err := p.core.RouteSpawn(node, name, opts, p.Node().Name())
	if err != nil {
		return gen.PID{}, err
	}

	if opts.LinkChild {
		// method LinkPID is not allowed to be used in the initialization state,
		// so we use linking manually.
		p.node.targets.LinkPID(p.pid, pid)
	}

	return pid, err
}

func (p *process) State() gen.ProcessState {
	return gen.ProcessState(atomic.LoadInt32(&p.state))
}

func (p *process) RegisterName(name gen.Atom) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}
	if err := p.node.RegisterName(name, p.pid); err != nil {
		return err
	}

	p.log.setSource(gen.MessageLogProcess{Node: p.node.name, PID: p.pid, Name: p.name, Behavior: p.sbehavior})
	return nil
}

func (p *process) UnregisterName() error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}
	_, err := p.node.UnregisterName(p.name)
	return err
}

func (p *process) EnvList() map[gen.Env]any {
	env := make(map[gen.Env]any)
	p.env.Range(func(k, v any) bool {
		env[gen.Env(k.(string))] = v
		return true
	})
	return env
}

func (p *process) SetEnv(name gen.Env, value any) {
	if p.isStateIR() == false {
		return
	}
	if value == nil {
		p.env.Delete(name.String())
		return
	}
	p.env.Store(name.String(), value)
}

func (p *process) Env(name gen.Env) (any, bool) {
	x, y := p.env.Load(name.String())
	return x, y
}

func (p *process) EnvDefault(name gen.Env, defaultValue any) any {
	if value, ok := p.Env(name); ok {
		return value
	}
	return defaultValue
}

// updateCompression applies f to a copy of the current compression settings and
// publishes it atomically, retrying if a concurrent setter raced in.
func (p *process) updateCompression(f func(c *gen.Compression)) {
	for {
		old := p.compression.Load()
		nc := *old
		f(&nc)
		if p.compression.CompareAndSwap(old, &nc) {
			return
		}
	}
}

func (p *process) Compression() bool {
	return p.compression.Load().Enable
}

func (p *process) SetCompression(enable bool) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}
	p.updateCompression(func(c *gen.Compression) { c.Enable = enable })
	return nil
}

func (p *process) CompressionType() gen.CompressionType {
	return p.compression.Load().Type
}

func (p *process) SetCompressionType(ctype gen.CompressionType) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	switch ctype {
	case gen.CompressionTypeGZIP:
	case gen.CompressionTypeLZW:
	case gen.CompressionTypeZLIB:
	default:
		return gen.ErrIncorrect
	}

	p.updateCompression(func(c *gen.Compression) { c.Type = ctype })
	return nil
}

func (p *process) CompressionLevel() gen.CompressionLevel {
	return p.compression.Load().Level
}

func (p *process) SetCompressionLevel(level gen.CompressionLevel) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	switch level {
	case gen.CompressionBestSize:
	case gen.CompressionBestSpeed:
	case gen.CompressionDefault:
	default:
		return gen.ErrIncorrect
	}

	p.updateCompression(func(c *gen.Compression) { c.Level = level })
	return nil
}

func (p *process) CompressionThreshold() int {
	return p.compression.Load().Threshold
}

func (p *process) SetCompressionThreshold(threshold int) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}
	if threshold < gen.DefaultCompressionThreshold {
		return gen.ErrIncorrect
	}
	p.updateCompression(func(c *gen.Compression) { c.Threshold = threshold })
	return nil
}

func (p *process) SendPriority() gen.MessagePriority {
	return gen.MessagePriority(p.priority.Load())
}

func (p *process) SetSendPriority(priority gen.MessagePriority) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	switch priority {
	case gen.MessagePriorityNormal:
	case gen.MessagePriorityHigh:
	case gen.MessagePriorityMax:
	default:
		return gen.ErrIncorrect
	}
	p.priority.Store(int32(priority))
	return nil
}

func (p *process) SetProcessKind(kind gen.ProcessKind) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}
	p.kind = kind
	return nil
}

func (p *process) SetKeepNetworkOrder(order bool) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	p.keeporder.Store(order)
	return nil
}

func (p *process) KeepNetworkOrder() bool {
	return p.keeporder.Load()
}

func (p *process) SetImportantDelivery(important bool) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	p.important.Store(important)
	return nil
}

func (p *process) ImportantDelivery() bool {
	return p.important.Load()
}

func (p *process) SetTracingSampler(sampler gen.TracingSampler) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}
	if sampler == gen.TracingSamplerDisable {
		p.tracingSampler.Store(nil)
		return nil
	}
	p.tracingSampler.Store(&sampler)
	return nil
}

func (p *process) TracingSampler() gen.TracingSampler {
	s := p.tracingSampler.Load()
	if s == nil {
		return gen.TracingSamplerDisable
	}
	return *s
}

func (p *process) PropagatingTrace() gen.Tracing {
	return p.tracing
}

func (p *process) SetPropagatingTrace(t gen.Tracing) {
	p.tracing = t
}

func (p *process) propagatingTrace() gen.Tracing {
	if p.tracing.ID != [2]uint64{} {
		t := p.tracing
		t.Behavior = p.sbehavior
		return t
	}
	// do not start new traces during Init phase
	if atomic.LoadInt32(&p.state) == int32(gen.ProcessStateInit) {
		return gen.Tracing{}
	}
	if s := p.tracingSampler.Load(); s != nil && (*s).Sample() {
		t := p.node.MakeTraceID()
		t.Behavior = p.sbehavior
		return t
	}
	return gen.Tracing{}
}

// messageOptions builds the base send options from process state; the caller sets Ref after.
func (p *process) messageOptions(priority gen.MessagePriority, important, tracing bool) gen.MessageOptions {
	options := gen.MessageOptions{
		Priority:          priority,
		Compression:       *p.compression.Load(),
		KeepNetworkOrder:  p.keeporder.Load(),
		ImportantDelivery: important,
	}
	if tracing {
		options.Tracing = p.propagatingTrace()
		if options.Tracing.ID != [2]uint64{} {
			p.applyTracingAttrs(&options)
		}
	}
	return options
}

func (p *process) CreateAlias() (gen.Alias, error) {
	if p.isStateIR() == false {
		return gen.Alias{}, gen.ErrNotAllowed
	}
	alias := gen.Alias(p.node.MakeRef())
	if err := p.node.registerAlias(alias, p); err != nil {
		return gen.Alias{}, err
	}

	p.aliases = append(p.aliases, alias)
	return alias, nil
}

func (p *process) DeleteAlias(alias gen.Alias) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if err := p.node.unregisterAlias(alias, p); err != nil {
		return err
	}

	p.core.RouteTerminateAlias(alias, gen.ErrUnregistered)

	for i, a := range p.aliases {
		if a != alias {
			continue
		}
		p.aliases[i] = p.aliases[0]
		p.aliases = p.aliases[1:]
		break
	}
	return nil
}

func (p *process) Aliases() []gen.Alias {
	aliases := make([]gen.Alias, len(p.aliases))
	copy(aliases, p.aliases)
	return aliases
}

func (p *process) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	return p.sendTo(to, message, priority, p.important.Load())
}

func (p *process) Send(to any, message any) error {
	return p.sendTo(to, message, gen.MessagePriority(p.priority.Load()), p.important.Load())
}

func (p *process) SendImportant(to any, message any) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}
	return p.sendTo(to, message, gen.MessagePriority(p.priority.Load()), true)
}

// sendTo dispatches an immediate send by target type with the given priority and important flag.
func (p *process) sendTo(to any, message any, priority gen.MessagePriority, important bool) error {
	switch t := to.(type) {
	case gen.PID:
		return p.sendPID(t, message, priority, important)
	case gen.ProcessID:
		return p.sendProcessID(t, message, priority, important)
	case gen.Alias:
		return p.sendAlias(t, message, priority, important)
	case gen.Atom:
		return p.sendProcessID(gen.ProcessID{Name: t, Node: p.node.name}, message, priority, important)
	}

	return gen.ErrUnsupported
}

func (p *process) SendPID(to gen.PID, message any) error {
	return p.sendPID(to, message, gen.MessagePriority(p.priority.Load()), p.important.Load())
}

func (p *process) sendPID(to gen.PID, message any, priority gen.MessagePriority, important bool) error {
	// allow to send in Init, Running, Terminated states
	if p.isStateIRT() == false {
		return gen.ErrNotAllowed
	}
	if lib.Verbose() {
		p.log.Trace("SendPID to %s", to)
	}

	options := p.messageOptions(priority, important, true)

	if options.ImportantDelivery {
		ref := p.node.MakeRef()
		options.Ref = ref
		options.Ref.ID[0] = ref.ID[0] + ref.ID[1] + ref.ID[2]
		options.Ref.ID[1] = 0
		options.Ref.ID[2] = 0
	}

	if err := p.core.RouteSendPID(p.pid, to, options, message); err != nil {
		return err
	}

	atomic.AddUint64(&p.messagesOut, 1)

	if options.ImportantDelivery == false {
		return nil
	}

	if to.Node == p.node.name {
		// local delivery
		return nil
	}

	// sent to remote node and 'important' flag was set. waiting for response
	// from the remote node

	_, err := p.waitResponse(options.Ref, gen.DefaultRequestTimeout)
	return err
}

func (p *process) SendProcessID(to gen.ProcessID, message any) error {
	return p.sendProcessID(to, message, gen.MessagePriority(p.priority.Load()), p.important.Load())
}

func (p *process) sendProcessID(to gen.ProcessID, message any, priority gen.MessagePriority, important bool) error {
	if p.isStateIRT() == false {
		return gen.ErrNotAllowed
	}

	if lib.Verbose() {
		p.log.Trace("SendProcessID to %s", to)
	}

	options := p.messageOptions(priority, important, true)

	if options.ImportantDelivery {
		ref := p.node.MakeRef()
		options.Ref = ref
		options.Ref.ID[0] = ref.ID[0] + ref.ID[1] + ref.ID[2]
		options.Ref.ID[1] = 0
		options.Ref.ID[2] = 0
	}

	if err := p.core.RouteSendProcessID(p.pid, to, options, message); err != nil {
		return err
	}

	atomic.AddUint64(&p.messagesOut, 1)

	if options.ImportantDelivery == false {
		return nil
	}

	if to.Node == p.node.name {
		// local delivery
		return nil
	}

	// sent to remote node and 'important' flag was set. waiting for response
	// from the remote node

	_, err := p.waitResponse(options.Ref, gen.DefaultRequestTimeout)
	return err
}

func (p *process) SendAlias(to gen.Alias, message any) error {
	return p.sendAlias(to, message, gen.MessagePriority(p.priority.Load()), p.important.Load())
}

func (p *process) sendAlias(to gen.Alias, message any, priority gen.MessagePriority, important bool) error {
	if p.isStateIRT() == false {
		return gen.ErrNotAllowed
	}

	if lib.Verbose() {
		p.log.Trace("SendAlias to %s", to)
	}

	options := p.messageOptions(priority, important, true)

	if options.ImportantDelivery {
		ref := p.node.MakeRef()
		options.Ref = ref
		options.Ref.ID[0] = ref.ID[0] + ref.ID[1] + ref.ID[2]
		options.Ref.ID[1] = 0
		options.Ref.ID[2] = 0
	}

	if err := p.core.RouteSendAlias(p.pid, to, options, message); err != nil {
		return err
	}

	atomic.AddUint64(&p.messagesOut, 1)

	if options.ImportantDelivery == false {
		return nil
	}

	if to.Node == p.node.name {
		// local delivery
		return nil
	}

	// sent to remote node and 'important' flag was set. waiting for response
	// from the remote node

	_, err := p.waitResponse(options.Ref, gen.DefaultRequestTimeout)
	return err
}

func (p *process) sendHelper(to any, options gen.MessageOptions, message any) func() error {
	switch t := to.(type) {
	case gen.PID:
		return func() error { return p.core.RouteSendPID(p.pid, t, options, message) }
	case gen.ProcessID:
		return func() error { return p.core.RouteSendProcessID(p.pid, t, options, message) }
	case gen.Alias:
		return func() error { return p.core.RouteSendAlias(p.pid, t, options, message) }
	case gen.Atom:
		dst := gen.ProcessID{Name: t, Node: p.node.name}
		return func() error { return p.core.RouteSendProcessID(p.pid, dst, options, message) }
	}
	return nil
}

func (p *process) SendAfter(to any, message any, after time.Duration) (gen.CancelFunc, error) {
	return p.sendDeferred(to, message, gen.MessagePriority(p.priority.Load()), after, false)
}

func (p *process) SendWithPriorityAfter(
	to any,
	message any,
	priority gen.MessagePriority,
	after time.Duration,
) (gen.CancelFunc, error) {
	return p.sendDeferred(to, message, priority, after, false)
}

// sendDeferred schedules a delayed send: once after d (repeat=false) or every d (repeat=true); no important/tracing.
func (p *process) sendDeferred(to any, message any, priority gen.MessagePriority, d time.Duration, repeat bool) (gen.CancelFunc, error) {
	if p.isStateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	if repeat && d <= 0 {
		return nil, gen.ErrIncorrect
	}
	// snapshot options once at schedule time; every tick reuses the same priority/order/compression
	options := p.messageOptions(priority, false, false)
	send := p.sendHelper(to, options, message)
	if send == nil {
		return nil, gen.ErrIncorrect
	}

	if repeat == false {
		return time.AfterFunc(d, func() {
			if lib.Verbose() {
				p.log.Trace("send after %s to %s (priority %s)", d, to, priority)
			}
			if send() == nil {
				atomic.AddUint64(&p.messagesOut, 1)
			}
		}).Stop, nil
	}

	var stopped atomic.Bool
	var t *time.Timer
	// arm far out first, then Reset, so the callback can't read t before it is set
	t = time.AfterFunc(time.Hour, func() {
		if stopped.Load() || p.isAlive() == false {
			t.Stop()
			return
		}
		if lib.Verbose() {
			p.log.Trace("send every %s to %s (priority %s)", d, to, priority)
		}
		if send() == nil {
			atomic.AddUint64(&p.messagesOut, 1)
		}
		if stopped.Load() == false {
			t.Reset(d)
		}
	})
	t.Reset(d)
	return func() bool {
		t.Stop()
		return stopped.Swap(true) == false
	}, nil
}

func (p *process) SendEvery(to any, message any, period time.Duration) (gen.CancelFunc, error) {
	return p.sendDeferred(to, message, gen.MessagePriority(p.priority.Load()), period, true)
}

func (p *process) SendWithPriorityEvery(
	to any,
	message any,
	priority gen.MessagePriority,
	period time.Duration,
) (gen.CancelFunc, error) {
	return p.sendDeferred(to, message, priority, period, true)
}

func (p *process) SendEvent(name gen.Atom, token gen.Ref, message any) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if lib.Verbose() {
		p.log.Trace("process SendEvent %s with token %s", name, token)
	}

	options := p.messageOptions(gen.MessagePriority(p.priority.Load()), false, true)

	em := gen.MessageEvent{
		Event:     gen.Event{Name: name, Node: p.node.name},
		Timestamp: time.Now().UnixNano(),
		Message:   message,
	}

	if err := p.core.RouteSendEvent(p.pid, token, options, em); err != nil {
		return err
	}

	return nil
}

func (p *process) SendExit(to gen.PID, reason error) error {
	if p.isStateIRT() == false {
		return gen.ErrNotAllowed
	}

	if reason == nil {
		return gen.ErrIncorrect
	}

	switch to {
	case p.pid, p.parent, p.leader:
		p.log.Warning("sending exit-signal to itself, parent, or leader process is not allowed")
		return gen.ErrNotAllowed
	}

	if lib.Verbose() {
		p.log.Trace("SendExit to %s", to)
	}
	err := p.core.RouteSendExit(p.pid, to, reason)
	if err != nil {
		return err
	}
	atomic.AddUint64(&p.messagesOut, 1)
	return nil
}

func (p *process) SendExitAfter(to gen.PID, reason error, after time.Duration) (gen.CancelFunc, error) {
	if p.isStateIR() == false {
		return nil, gen.ErrNotAllowed
	}

	if reason == nil {
		return nil, gen.ErrIncorrect
	}

	switch to {
	case p.pid, p.parent, p.leader:
		p.log.Warning("sending exit-signal to itself, parent, or leader process is not allowed")
		return nil, gen.ErrNotAllowed
	}

	return time.AfterFunc(after, func() {
		if lib.Verbose() {
			p.log.Trace("SendExitAfter %s to %s with reason %q", after, to, reason)
		}

		err := p.core.RouteSendExit(p.pid, to, reason)
		if err == nil {
			atomic.AddUint64(&p.messagesOut, 1)
		}
	}).Stop, nil
}

func (p *process) SendExitMeta(alias gen.Alias, reason error) error {
	if p.isStateIRT() == false {
		return gen.ErrNotAllowed
	}

	if reason == nil {
		return gen.ErrIncorrect
	}

	value, found := p.node.aliases.Load(alias)
	if found == false {
		return gen.ErrAliasUnknown
	}

	metap := value.(*process)
	if alive := metap.isAlive(); alive == false {
		return gen.ErrProcessTerminated
	}

	value, found = metap.metas.Load(alias)
	if found == false {
		return gen.ErrMetaUnknown
	}

	m := value.(*meta)
	// send exit signal to the meta processes
	qm := gen.TakeMailboxMessage()
	qm.From = p.pid
	qm.Type = gen.MailboxMessageTypeExit
	qm.Message = reason

	if ok := m.system.Push(qm); ok == false {
		return gen.ErrMetaMailboxFull
	}

	atomic.AddUint64(&m.messagesIn, 1)
	atomic.AddUint64(&metap.messagesIn, 1)
	atomic.AddUint64(&p.messagesOut, 1)
	m.handle()
	return nil
}

func (p *process) SendExitMetaAfter(alias gen.Alias, reason error, after time.Duration) (gen.CancelFunc, error) {
	if p.isStateIR() == false {
		return nil, gen.ErrNotAllowed
	}

	if reason == nil {
		return nil, gen.ErrIncorrect
	}

	// Validate alias exists before scheduling
	value, found := p.node.aliases.Load(alias)
	if found == false {
		return nil, gen.ErrAliasUnknown
	}

	metap := value.(*process)
	if alive := metap.isAlive(); alive == false {
		return nil, gen.ErrProcessTerminated
	}

	_, found = metap.metas.Load(alias)
	if found == false {
		return nil, gen.ErrMetaUnknown
	}

	return time.AfterFunc(after, func() {
		if lib.Verbose() {
			p.log.Trace("SendExitMetaAfter %s to %s with reason %q", after, alias, reason)
		}

		value, found := p.node.aliases.Load(alias)
		if found == false {
			return
		}

		metap := value.(*process)
		if alive := metap.isAlive(); alive == false {
			return
		}

		value, found = metap.metas.Load(alias)
		if found == false {
			return
		}

		m := value.(*meta)
		// send exit signal to the meta process
		qm := gen.TakeMailboxMessage()
		qm.From = p.pid
		qm.Type = gen.MailboxMessageTypeExit
		qm.Message = reason

		if ok := m.system.Push(qm); ok == false {
			return
		}

		atomic.AddUint64(&m.messagesIn, 1)
		atomic.AddUint64(&metap.messagesIn, 1)
		atomic.AddUint64(&p.messagesOut, 1)
		m.handle()
	}).Stop, nil
}

func (p *process) SendResponse(to gen.PID, ref gen.Ref, message any) error {
	return p.sendResponse(to, ref, message, p.important.Load())
}

func (p *process) SendResponseImportant(to gen.PID, ref gen.Ref, message any) error {
	return p.sendResponse(to, ref, message, true)
}

func (p *process) sendResponse(to gen.PID, ref gen.Ref, message any, important bool) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}
	if lib.Verbose() {
		p.log.Trace("send response to %s with %s", to, ref)
	}
	options := p.messageOptions(gen.MessagePriority(p.priority.Load()), important, true)
	options.Ref = ref
	if err := p.core.RouteSendResponse(p.pid, to, options, message); err != nil {
		return err
	}
	atomic.AddUint64(&p.messagesOut, 1)

	if important == false {
		return nil
	}
	// The caller-supplied ref may carry a remote node prefix when responding to a
	// cross-node Call (or a ref wrapped across several nodes). The peer's auto-ack
	// travels back with just the IDs; our side reconstructs the incoming ref with the
	// local node prefix before delivering it to waitResponse. So wait on a local-prefix
	// view of the same IDs; the caller's ref stays unchanged on the wire.
	ackRef := gen.Ref{
		Node:     p.node.Name(),
		Creation: p.node.Creation(),
		ID:       options.Ref.ID,
	}
	_, err := p.waitResponse(ackRef, gen.DefaultRequestTimeout)
	return err
}

func (p *process) SendResponseError(to gen.PID, ref gen.Ref, err error) error {
	return p.sendResponseError(to, ref, err, p.important.Load())
}

func (p *process) SendResponseErrorImportant(to gen.PID, ref gen.Ref, err error) error {
	return p.sendResponseError(to, ref, err, true)
}

func (p *process) sendResponseError(to gen.PID, ref gen.Ref, e error, important bool) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}
	if lib.Verbose() {
		p.log.Trace("send response error to %s with %s", to, ref)
	}
	options := p.messageOptions(gen.MessagePriority(p.priority.Load()), important, true)
	options.Ref = ref
	if routeErr := p.core.RouteSendResponseError(p.pid, to, options, e); routeErr != nil {
		return routeErr
	}
	atomic.AddUint64(&p.messagesOut, 1)

	if important == false {
		return nil
	}
	// See sendResponse for the ack-ref reconstruction rationale.
	ackRef := gen.Ref{
		Node:     p.node.Name(),
		Creation: p.node.Creation(),
		ID:       options.Ref.ID,
	}
	_, ackErr := p.waitResponse(ackRef, gen.DefaultRequestTimeout)
	return ackErr
}

func (p *process) CallWithPriority(to any, request any, priority gen.MessagePriority) (any, error) {
	return p.callTo(to, request, gen.DefaultRequestTimeout, priority, p.important.Load())
}

func (p *process) CallImportant(to any, request any) (any, error) {
	return p.callTo(to, request, gen.DefaultRequestTimeout, gen.MessagePriority(p.priority.Load()), true)
}

func (p *process) Call(to any, request any) (any, error) {
	return p.CallWithTimeout(to, request, gen.DefaultRequestTimeout)
}
func (p *process) CallWithTimeout(to any, request any, timeout int) (any, error) {
	return p.callTo(to, request, timeout, gen.MessagePriority(p.priority.Load()), p.important.Load())
}

// callTo dispatches a synchronous call by target type with the given priority and important flag.
func (p *process) callTo(to any, request any, timeout int, priority gen.MessagePriority, important bool) (any, error) {
	switch t := to.(type) {
	case gen.Atom:
		return p.callProcessID(gen.ProcessID{Name: t, Node: p.node.name}, request, timeout, priority, important)
	case gen.PID:
		return p.callPID(t, request, timeout, priority, important)
	case gen.ProcessID:
		return p.callProcessID(t, request, timeout, priority, important)
	case gen.Alias:
		return p.callAlias(t, request, timeout, priority, important)
	}

	return nil, gen.ErrUnsupported
}

func (p *process) CallPID(to gen.PID, message any, timeout int) (any, error) {
	return p.callPID(to, message, timeout, gen.MessagePriority(p.priority.Load()), p.important.Load())
}

func (p *process) callPID(to gen.PID, message any, timeout int, priority gen.MessagePriority, important bool) (any, error) {
	if p.isStateIR() == false {
		return nil, gen.ErrNotAllowed
	}
	if to == p.pid {
		return nil, gen.ErrNotAllowed
	}

	if timeout < 1 {
		timeout = gen.DefaultRequestTimeout
	}

	deadline := time.Now().Unix() + int64(timeout)
	ref, err := p.node.MakeRefWithDeadline(deadline)
	if err != nil {
		return nil, err
	}

	options := p.messageOptions(priority, important, true)
	options.Ref = ref

	if lib.Verbose() {
		p.log.Trace("CallPID to %s with %s", to, options.Ref)
	}

	if err := p.core.RouteCallPID(p.pid, to, options, message); err != nil {
		return nil, err
	}

	atomic.AddUint64(&p.messagesOut, 1)
	return p.waitResponse(options.Ref, timeout)
}

func (p *process) CallProcessID(to gen.ProcessID, message any, timeout int) (any, error) {
	return p.callProcessID(to, message, timeout, gen.MessagePriority(p.priority.Load()), p.important.Load())
}

func (p *process) callProcessID(to gen.ProcessID, message any, timeout int, priority gen.MessagePriority, important bool) (any, error) {
	if p.isStateIR() == false {
		return nil, gen.ErrNotAllowed
	}

	if timeout < 1 {
		timeout = gen.DefaultRequestTimeout
	}

	deadline := time.Now().Unix() + int64(timeout)
	ref, err := p.node.MakeRefWithDeadline(deadline)
	if err != nil {
		return nil, err
	}

	options := p.messageOptions(priority, important, true)
	options.Ref = ref

	if lib.Verbose() {
		p.log.Trace("CallProcessID %s with %s", to, options.Ref)
	}
	if err := p.core.RouteCallProcessID(p.pid, to, options, message); err != nil {
		return nil, err
	}
	atomic.AddUint64(&p.messagesOut, 1)
	return p.waitResponse(options.Ref, timeout)
}

func (p *process) CallAlias(to gen.Alias, message any, timeout int) (any, error) {
	return p.callAlias(to, message, timeout, gen.MessagePriority(p.priority.Load()), p.important.Load())
}

func (p *process) callAlias(to gen.Alias, message any, timeout int, priority gen.MessagePriority, important bool) (any, error) {
	if p.isStateIR() == false {
		return nil, gen.ErrNotAllowed
	}

	if timeout < 1 {
		timeout = gen.DefaultRequestTimeout
	}

	deadline := time.Now().Unix() + int64(timeout)
	ref, err := p.node.MakeRefWithDeadline(deadline)
	if err != nil {
		return nil, err
	}

	options := p.messageOptions(priority, important, true)
	options.Ref = ref

	if lib.Verbose() {
		p.log.Trace("CallAlias %s with %s", to, options.Ref)
	}

	if err := p.core.RouteCallAlias(p.pid, to, options, message); err != nil {
		return nil, err
	}
	atomic.AddUint64(&p.messagesOut, 1)
	return p.waitResponse(options.Ref, timeout)
}

func (p *process) Inspect(target gen.PID, item ...string) (map[string]string, error) {
	if p.isStateIR() == false {
		return nil, gen.ErrNotAllowed
	}

	if target.Node != p.pid.Node {
		// inspecting remote process is not allowed
		return nil, gen.ErrNotAllowed
	}

	ref := p.node.MakeRef()

	value, found := p.node.processes.Load(target)
	if found == false {
		return nil, gen.ErrProcessUnknown
	}
	targetp := value.(*process)

	if alive := targetp.isAlive(); alive == false {
		return nil, gen.ErrProcessTerminated
	}

	qm := gen.TakeMailboxMessage()
	qm.Ref = ref
	qm.From = p.pid
	qm.Type = gen.MailboxMessageTypeInspect
	qm.Message = item

	if ok := targetp.mailbox.Urgent.Push(qm); ok == false {
		return nil, gen.ErrProcessMailboxFull
	}
	atomic.AddUint64(&p.messagesOut, 1)
	atomic.AddUint64(&targetp.messagesIn, 1)

	if lib.Verbose() {
		p.log.Trace("Inspect %s with %s", target, ref)
	}

	targetp.run()

	value, err := p.waitResponse(ref, gen.DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}
	return value.(map[string]string), nil
}

func (p *process) InspectMeta(alias gen.Alias, item ...string) (map[string]string, error) {
	if p.isStateIR() == false {
		return nil, gen.ErrNotAllowed
	}

	if alias.Node != p.pid.Node {
		// inspecting remote meta process is not allowed
		return nil, gen.ErrNotAllowed
	}

	value, found := p.node.aliases.Load(alias)
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
	ref := p.node.MakeRef()

	qm := gen.TakeMailboxMessage()
	qm.Ref = ref
	qm.From = p.pid
	qm.Type = gen.MailboxMessageTypeInspect
	qm.Message = item

	if ok := m.system.Push(qm); ok == false {
		return nil, gen.ErrProcessMailboxFull
	}
	atomic.AddUint64(&p.messagesOut, 1)
	atomic.AddUint64(&m.messagesIn, 1)
	atomic.AddUint64(&metap.messagesIn, 1)

	if lib.Verbose() {
		m.log.Trace("Inspect meta %s with %s", alias, ref)
	}

	m.handle()

	v, err := p.waitResponse(ref, gen.DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}
	return v.(map[string]string), nil
}

func (p *process) RegisterEvent(name gen.Atom, options gen.EventOptions) (gen.Ref, error) {
	var empty gen.Ref
	if p.isStateIR() == false {
		return empty, gen.ErrNotAllowed
	}

	if lib.Verbose() {
		p.log.Trace("process RegisterEvent %s", name)
	}

	return p.node.registerEvent(name, p.pid, options)
}

func (p *process) UnregisterEvent(name gen.Atom) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if lib.Verbose() {
		p.log.Trace("process UnregisterEvent %s", name)
	}

	return p.node.unregisterEvent(name, p.pid)
}

func (p *process) Events() []gen.Atom {
	events := p.node.targets.EventsFor(p.pid)
	names := make([]gen.Atom, len(events))
	for i, event := range events {
		names[i] = event.Name
	}
	return names
}

func (p *process) Link(target any) error {
	switch t := target.(type) {
	case gen.Atom:
		return p.LinkProcessID(gen.ProcessID{Name: t, Node: p.node.name})
	case gen.PID:
		return p.LinkPID(t)
	case gen.ProcessID:
		return p.LinkProcessID(t)
	case gen.Alias:
		return p.LinkAlias(t)
	}

	return gen.ErrUnsupported
}
func (p *process) Unlink(target any) error {
	switch t := target.(type) {
	case gen.Atom:
		return p.UnlinkProcessID(gen.ProcessID{Name: t, Node: p.node.name})
	case gen.PID:
		return p.UnlinkPID(t)
	case gen.ProcessID:
		return p.UnlinkProcessID(t)
	case gen.Alias:
		return p.UnlinkAlias(t)
	}

	return gen.ErrUnsupported
}

func (p *process) LinkPID(target gen.PID) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if target == p.pid {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasLink(p.pid, target) {
		return gen.ErrTargetExist
	}

	if lib.Verbose() {
		p.log.Trace("LinkPID with %s", target)
	}

	if err := p.core.RouteLinkPID(p.pid, target); err != nil {
		return err
	}

	// recheck state - process could have been killed during the operation
	if p.isStateIR() == false {
		p.node.targets.UnlinkPID(p.pid, target)
		return gen.ErrProcessTerminated
	}

	return nil
}

func (p *process) UnlinkPID(target gen.PID) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasLink(p.pid, target) == false {
		return gen.ErrTargetUnknown
	}

	if lib.Verbose() {
		p.log.Trace("UnlinkPID with %s", target)
	}

	if err := p.core.RouteUnlinkPID(p.pid, target); err != nil {
		return err
	}

	return nil
}

func (p *process) LinkProcessID(target gen.ProcessID) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if target.Name == p.name && target.Node == p.node.name {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasLink(p.pid, target) {
		return gen.ErrTargetExist
	}

	if lib.Verbose() {
		p.log.Trace("LinkProcessID with %s", target)
	}

	if err := p.core.RouteLinkProcessID(p.pid, target); err != nil {
		return err
	}

	// recheck state - process could have been killed during the operation
	if p.isStateIR() == false {
		p.node.targets.UnlinkProcessID(p.pid, target)
		return gen.ErrProcessTerminated
	}

	return nil
}

func (p *process) UnlinkProcessID(target gen.ProcessID) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasLink(p.pid, target) == false {
		return gen.ErrTargetUnknown
	}

	if err := p.core.RouteUnlinkProcessID(p.pid, target); err != nil {
		return err
	}

	return nil
}

func (p *process) LinkAlias(target gen.Alias) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	for _, a := range p.aliases {
		if a == target {
			return gen.ErrNotAllowed
		}
	}

	if p.node.targets.HasLink(p.pid, target) {
		return gen.ErrTargetExist
	}

	if err := p.core.RouteLinkAlias(p.pid, target); err != nil {
		return err
	}

	// recheck state - process could have been killed during the operation
	if p.isStateIR() == false {
		p.node.targets.UnlinkAlias(p.pid, target)
		return gen.ErrProcessTerminated
	}

	return nil
}

func (p *process) UnlinkAlias(target gen.Alias) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasLink(p.pid, target) == false {
		return gen.ErrTargetUnknown
	}

	if err := p.core.RouteUnlinkAlias(p.pid, target); err != nil {
		return err
	}

	return nil
}

func (p *process) LinkEvent(target gen.Event) ([]gen.MessageEvent, error) {

	if p.isStateIR() == false {
		return nil, gen.ErrNotAllowed
	}

	if target.Node == "" {
		target.Node = p.node.name
	}

	if p.node.targets.HasLink(p.pid, target) {
		return nil, gen.ErrTargetExist
	}

	lastEventMessages, err := p.core.RouteLinkEvent(p.pid, target)
	if err != nil {
		return nil, err
	}

	// recheck state - process could have been killed during the operation
	if p.isStateIR() == false {
		p.node.targets.UnlinkEvent(p.pid, target)
		return nil, gen.ErrProcessTerminated
	}

	return lastEventMessages, nil
}

func (p *process) UnlinkEvent(target gen.Event) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasLink(p.pid, target) == false {
		return gen.ErrTargetUnknown
	}

	if err := p.core.RouteUnlinkEvent(p.pid, target); err != nil {
		return err
	}

	return nil
}

func (p *process) LinkNode(target gen.Atom) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasLink(p.pid, target) {
		return gen.ErrTargetExist
	}

	if _, err := p.Node().Network().GetNode(target); err != nil {
		return err
	}

	if err := p.node.targets.LinkNode(p.pid, target); err != nil {
		return err
	}

	// recheck state - process could have been killed during the operation
	if p.isStateIR() == false {
		p.node.targets.UnlinkNode(p.pid, target)
		return gen.ErrProcessTerminated
	}

	return nil
}

func (p *process) UnlinkNode(target gen.Atom) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasLink(p.pid, target) == false {
		return gen.ErrTargetUnknown
	}

	p.node.targets.UnlinkNode(p.pid, target)

	return nil
}

func (p *process) Monitor(target any) error {
	switch t := target.(type) {
	case gen.Atom:
		return p.MonitorProcessID(gen.ProcessID{Name: t, Node: p.node.name})
	case gen.PID:
		return p.MonitorPID(t)
	case gen.ProcessID:
		return p.MonitorProcessID(t)
	case gen.Alias:
		return p.MonitorAlias(t)
	}

	return gen.ErrUnsupported
}

func (p *process) Demonitor(target any) error {
	switch t := target.(type) {
	case gen.Atom:
		return p.DemonitorProcessID(gen.ProcessID{Name: t, Node: p.node.name})
	case gen.PID:
		return p.DemonitorPID(t)
	case gen.ProcessID:
		return p.DemonitorProcessID(t)
	case gen.Alias:
		return p.DemonitorAlias(t)
	}

	return gen.ErrUnsupported
}

func (p *process) MonitorPID(target gen.PID) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasMonitor(p.pid, target) {
		return gen.ErrTargetExist
	}

	if err := p.core.RouteMonitorPID(p.pid, target); err != nil {
		return err
	}

	// recheck state - process could have been killed during the operation
	if p.isStateIR() == false {
		p.node.targets.DemonitorPID(p.pid, target)
		return gen.ErrProcessTerminated
	}

	return nil
}

func (p *process) DemonitorPID(target gen.PID) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasMonitor(p.pid, target) == false {
		return gen.ErrTargetUnknown
	}

	if err := p.core.RouteDemonitorPID(p.pid, target); err != nil {
		return err
	}

	return nil
}

func (p *process) MonitorProcessID(target gen.ProcessID) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasMonitor(p.pid, target) {
		return gen.ErrTargetExist
	}

	if err := p.core.RouteMonitorProcessID(p.pid, target); err != nil {
		return err
	}

	// recheck state - process could have been killed during the operation
	if p.isStateIR() == false {
		p.node.targets.DemonitorProcessID(p.pid, target)
		return gen.ErrProcessTerminated
	}

	return nil
}

func (p *process) DemonitorProcessID(target gen.ProcessID) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasMonitor(p.pid, target) == false {
		return gen.ErrTargetUnknown
	}

	if err := p.core.RouteDemonitorProcessID(p.pid, target); err != nil {
		return err
	}

	return nil
}

func (p *process) MonitorAlias(target gen.Alias) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasMonitor(p.pid, target) {
		return gen.ErrTargetExist
	}

	if err := p.core.RouteMonitorAlias(p.pid, target); err != nil {
		return err
	}

	// recheck state - process could have been killed during the operation
	if p.isStateIR() == false {
		p.node.targets.DemonitorAlias(p.pid, target)
		return gen.ErrProcessTerminated
	}

	return nil
}

func (p *process) DemonitorAlias(target gen.Alias) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasMonitor(p.pid, target) == false {
		return gen.ErrTargetUnknown
	}

	if err := p.core.RouteDemonitorAlias(p.pid, target); err != nil {
		return err
	}

	return nil
}

func (p *process) MonitorEvent(target gen.Event) ([]gen.MessageEvent, error) {

	if p.isStateIR() == false {
		return nil, gen.ErrNotAllowed
	}

	if target.Node == "" {
		target.Node = p.node.name
	}

	if p.node.targets.HasMonitor(p.pid, target) {
		return nil, gen.ErrTargetExist
	}

	lastEventMessages, err := p.core.RouteMonitorEvent(p.pid, target)
	if err != nil {
		return nil, err
	}

	// recheck state - process could have been killed during the operation
	if p.isStateIR() == false {
		p.node.targets.DemonitorEvent(p.pid, target)
		return nil, gen.ErrProcessTerminated
	}

	return lastEventMessages, nil
}

func (p *process) DemonitorEvent(target gen.Event) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasMonitor(p.pid, target) == false {
		return gen.ErrTargetUnknown
	}

	if err := p.core.RouteDemonitorEvent(p.pid, target); err != nil {
		return err
	}

	return nil
}

func (p *process) MonitorNode(target gen.Atom) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}

	if p.node.targets.HasMonitor(p.pid, target) {
		return gen.ErrTargetExist
	}

	if _, err := p.Node().Network().GetNode(target); err != nil {
		return err
	}

	if err := p.node.targets.MonitorNode(p.pid, target); err != nil {
		return err
	}

	// recheck state - process could have been killed during the operation
	if p.isStateIR() == false {
		p.node.targets.DemonitorNode(p.pid, target)
		return gen.ErrProcessTerminated
	}

	return nil
}

func (p *process) DemonitorNode(target gen.Atom) error {
	if p.isStateIR() == false {
		return gen.ErrNotAllowed
	}
	if p.node.targets.HasMonitor(p.pid, target) == false {
		return gen.ErrTargetUnknown
	}

	return p.node.targets.DemonitorNode(p.pid, target)
}

func (p *process) Log() gen.Log {
	return p.log
}

func (p *process) Info() (gen.ProcessInfo, error) {
	if p.isStateIR() == false {
		return gen.ProcessInfo{}, gen.ErrNotAllowed
	}
	return p.node.ProcessInfo(p.pid)
}

func (p *process) MetaInfo(m gen.Alias) (gen.MetaInfo, error) {
	if p.isStateIR() == false {
		return gen.MetaInfo{}, gen.ErrNotAllowed
	}
	return p.node.MetaInfo(m)
}

func (p *process) Mailbox() gen.ProcessMailbox {
	return p.mailbox
}

func (p *process) wrapPreserveMailbox(reason error) error {
	if p.preserveMailbox == false {
		return reason
	}
	if reason == gen.TerminateReasonNormal || reason == gen.TerminateReasonShutdown {
		return reason
	}
	if ge, ok := reason.(*gen.Error); ok {
		if ge.Mailbox == nil {
			ge.Mailbox = &p.mailbox
		}
		return ge
	}
	return &gen.Error{Msg: reason.Error(), Wrapped: []error{reason}, Mailbox: &p.mailbox}
}

func (p *process) Behavior() gen.ProcessBehavior {
	return p.behavior
}

func (p *process) BehaviorName() string {
	return p.sbehavior
}

func (p *process) SetTracingAttribute(key, value string) {
	if strings.HasPrefix(key, "ergo.") {
		return
	}
	// COW: copy slice, overwrite existing key or append
	cur := *p.tracingAttrs.Load()
	for i, a := range cur {
		if a.Key == key {
			attrs := make([]gen.TracingAttribute, len(cur))
			copy(attrs, cur)
			attrs[i] = gen.TracingAttribute{Key: key, Value: value}
			p.tracingAttrs.Store(&attrs)
			return
		}
	}
	attrs := make([]gen.TracingAttribute, len(cur)+1)
	copy(attrs, cur)
	attrs[len(attrs)-1] = gen.TracingAttribute{Key: key, Value: value}
	p.tracingAttrs.Store(&attrs)
}

func (p *process) RemoveTracingAttribute(key string) {
	cur := *p.tracingAttrs.Load()
	for i, a := range cur {
		if a.Key == key {
			attrs := make([]gen.TracingAttribute, len(cur)-1)
			copy(attrs, cur[:i])
			copy(attrs[i:], cur[i+1:])
			p.tracingAttrs.Store(&attrs)
			return
		}
	}
}

func (p *process) SetTracingSpanAttribute(key, value string) {
	if strings.HasPrefix(key, "ergo.") {
		return
	}
	for i, a := range p.tracingSpanAttrs {
		if a.Key == key {
			p.tracingSpanAttrs[i].Value = value
			return
		}
	}
	p.tracingSpanAttrs = append(p.tracingSpanAttrs, gen.TracingAttribute{Key: key, Value: value})
}

func (p *process) TracingAttributes() []gen.TracingAttribute {
	attrs := *p.tracingAttrs.Load()
	if len(attrs) == 0 {
		return p.tracingSpanAttrs
	}
	if len(p.tracingSpanAttrs) == 0 {
		return attrs
	}
	m := make([]gen.TracingAttribute, 0, len(attrs)+len(p.tracingSpanAttrs))
	return append(append(m, attrs...), p.tracingSpanAttrs...)
}

func (p *process) ClearTracingSpanAttributes() {
	if p.tracingSpanAttrs != nil {
		p.tracingSpanAttrs = nil
	}
}

func (p *process) applyTracingAttrs(options *gen.MessageOptions) {
	attrs := p.TracingAttributes()
	if len(attrs) == 0 {
		return
	}
	options.TracingAttributes = attrs
}

func (p *process) SendTracingSpan(span gen.TracingSpan) {
	p.node.sendTracingSpan(span)
}

func (p *process) StartTracingSpan(name string) gen.TracingSpanScope {
	saved := p.tracing
	if p.tracing.ID == [2]uint64{} {
		// no active trace - start one via the sampler so a business span at an
		// initiator becomes a trace root (severed/delayed contexts land here)
		t := p.propagatingTrace()
		if t.ID == [2]uint64{} {
			return gen.TracingSpanScopeNoop
		}
		p.tracing = t
	}
	parentPoint := gen.TracingPointProcessed
	if len(p.tracingSpanStack) > 0 {
		parentPoint = gen.TracingPointSpan
	}
	sc := &tracingSpanScope{
		p:           p,
		saved:       saved,
		traceID:     p.tracing.ID,
		spanID:      atomic.AddUint64(&p.node.spanID, 1),
		parentPoint: parentPoint,
		name:        name,
		start:       time.Now().UnixNano(),
	}
	p.tracingSpanStack = append(p.tracingSpanStack, sc)
	p.tracing.SpanID = sc.spanID
	return sc
}

// CloseTracingSpans emits any business span left open by the handler (marked
// ergo.span.unended) and restores the trace position. Called by behavior loops.
func (p *process) CloseTracingSpans() {
	n := len(p.tracingSpanStack)
	if n == 0 {
		return
	}
	end := time.Now().UnixNano()
	restore := p.tracingSpanStack[0].saved
	for i := n - 1; i >= 0; i-- {
		s := p.tracingSpanStack[i]
		s.ended = true
		s.attrs = append(s.attrs, gen.TracingAttribute{Key: "ergo.span.unended", Value: "true"})
		p.emitSpanScope(s, "", end)
	}
	p.tracingSpanStack = p.tracingSpanStack[:0]
	p.tracing = restore
}

func (p *process) emitSpanScope(s *tracingSpanScope, errStr string, end int64) {
	p.node.sendTracingSpan(gen.TracingSpan{
		TraceID:      s.traceID,
		SpanID:       s.spanID,
		ParentSpanID: s.saved.SpanID,
		ParentPoint:  s.parentPoint,
		Point:        gen.TracingPointSpan,
		Timestamp:    s.start,
		EndTimestamp: end,
		Node:         p.node.name,
		From:         p.pid,
		To:           p.pid,
		Behavior:     p.sbehavior,
		Message:      s.name,
		Error:        errStr,
		Attributes:   p.spanScopeAttrs(s),
	})
}

func (p *process) spanScopeAttrs(s *tracingSpanScope) []gen.TracingAttribute {
	perm := *p.tracingAttrs.Load()
	if len(perm) == 0 {
		return s.attrs
	}
	if len(s.attrs) == 0 {
		return perm
	}
	merged := make([]gen.TracingAttribute, 0, len(perm)+len(s.attrs))
	merged = append(merged, perm...)
	return append(merged, s.attrs...)
}

func (p *process) popSpanScope(s *tracingSpanScope) {
	n := len(p.tracingSpanStack)
	if n == 0 {
		return
	}
	if p.tracingSpanStack[n-1] == s {
		p.tracingSpanStack = p.tracingSpanStack[:n-1]
		p.tracing = s.saved
		return
	}
	for i := range p.tracingSpanStack {
		if p.tracingSpanStack[i] == s {
			p.tracingSpanStack = append(p.tracingSpanStack[:i], p.tracingSpanStack[i+1:]...)
			return
		}
	}
}

type tracingSpanScope struct {
	p           *process
	saved       gen.Tracing // p.tracing as it was before this span (restored on End)
	traceID     [2]uint64
	spanID      uint64
	parentPoint gen.TracingPoint
	name        string
	start       int64
	attrs       []gen.TracingAttribute
	ended       bool
}

func (s *tracingSpanScope) SetAttribute(key, value string) {
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

func (s *tracingSpanScope) End() { s.end("") }

func (s *tracingSpanScope) EndError(err error) {
	if err == nil {
		s.end("")
		return
	}
	s.end(err.Error())
}

func (s *tracingSpanScope) end(errStr string) {
	if s.ended {
		return
	}
	s.ended = true
	s.p.emitSpanScope(s, errStr, time.Now().UnixNano())
	s.p.popSpanScope(s)
}

func (p *process) Forward(
	to gen.PID,
	message *gen.MailboxMessage,
	priority gen.MessagePriority,
) error {
	if p.State() != gen.ProcessStateRunning {
		return gen.ErrNotAllowed
	}

	var queue lib.QueueMPSC

	// local
	value, found := p.node.processes.Load(to)
	if found == false {
		return gen.ErrProcessUnknown
	}
	fp := value.(*process)

	if alive := fp.isAlive(); alive == false {
		return gen.ErrProcessTerminated
	}

	switch priority {
	case gen.MessagePriorityHigh:
		queue = fp.mailbox.System
	case gen.MessagePriorityMax:
		queue = fp.mailbox.Urgent
	default:
		queue = fp.mailbox.Main
	}
	if ok := queue.Push(message); ok == false {
		return gen.ErrProcessMailboxFull
	}
	atomic.AddUint64(&p.messagesOut, 1)
	atomic.AddUint64(&fp.messagesIn, 1)
	fp.run()
	return nil
}

// internal

func (p *process) isAlive() bool {
	state := atomic.LoadInt32(&p.state)
	alive := int32(gen.ProcessStateInit) |
		int32(gen.ProcessStateSleep) |
		int32(gen.ProcessStateRunning) |
		int32(gen.ProcessStateWaitResponse)
	return (state & alive) == state
}

// isStateIR - Init or Running
// Used for: Property setters, Spawn*, SendAfter, SendExitAfter
func (p *process) isStateIR() bool {
	state := atomic.LoadInt32(&p.state)
	mask := int32(gen.ProcessStateInit) |
		int32(gen.ProcessStateRunning)
	return (state & mask) == state
}

// isStateIRT - Init, Running, or Terminated
// Used for: Send*, SendExit, SendExitMeta
func (p *process) isStateIRT() bool {
	state := atomic.LoadInt32(&p.state)
	mask := int32(gen.ProcessStateInit) |
		int32(gen.ProcessStateRunning) |
		int32(gen.ProcessStateTerminated)
	return (state & mask) == state
}

func (p *process) waitResponse(ref gen.Ref, timeout int) (any, error) {
	var result any
	var err error

	// swap to wait response state
	prevState := atomic.SwapInt32(&p.state, int32(gen.ProcessStateWaitResponse))
	if prevState != int32(gen.ProcessStateRunning) && prevState != int32(gen.ProcessStateInit) {
		// not allowed, restore previous state
		atomic.StoreInt32(&p.state, prevState)
		return nil, gen.ErrNotAllowed
	}
	atomic.StoreInt64(&p.stateEntered, time.Now().UnixNano())

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)

	if timeout == 0 {
		timer.Reset(time.Second * time.Duration(gen.DefaultRequestTimeout))
	} else {
		timer.Reset(time.Second * time.Duration(timeout))
	}

	// take consumes one response and decides if it matches the awaited ref.
	// Returns true if applied. Stale responses (different ref) are dropped
	// and traced; these come from a previous waitResponse that timed out
	// after the peer had already produced its reply.
	take := func(r response) bool {
		if r.ref != ref {
			if lib.Verbose() {
				p.log.Trace("got late response on request with ref %s (exp %s). dropped", r.ref, ref)
			}
			return false
		}
		result = r.message
		err = r.err
		if r.important {
			options := gen.MessageOptions{
				Tracing: p.propagatingTrace(),
				Ref:     r.ref,
			}
			if options.Tracing.ID != [2]uint64{} {
				p.applyTracingAttrs(&options)
			}
			p.core.RouteSendResponseError(p.pid, r.from, options, nil)
		}
		return true
	}

	for {
		select {
		case r := <-p.response:
			if take(r) {
				goto done
			}
		case <-timer.C:
			// Timer/response race: select may have picked timer.C while a
			// matching response was already in the buffer. Drain non-blocking
			// before declaring timeout.
			for {
				select {
				case r := <-p.response:
					if take(r) {
						goto done
					}
				default:
					if lib.Verbose() {
						p.log.Trace("request with ref %s is timed out", ref)
					}
					err = gen.ErrTimeout
					goto done
				}
			}
		}
	}

done:
	// restore to previous state (init or running)
	if swapped := atomic.CompareAndSwapInt32(&p.state, int32(gen.ProcessStateWaitResponse), prevState); swapped == false {
		return nil, gen.ErrProcessTerminated
	}
	atomic.StoreInt64(&p.stateEntered, time.Now().UnixNano())
	return result, err
}
