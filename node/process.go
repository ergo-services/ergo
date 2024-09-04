package node

import (
	"errors"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type process struct {
	node *node
	pid  gen.PID

	name        gen.Atom
	registered  atomic.Bool
	application gen.Atom

	// used for the process Uptime method only. PID value uses node creation value.
	creation int64

	// registered aliases
	aliases []gen.Alias
	// registered events
	events sync.Map // gen.Atom ->..

	behavior  gen.ProcessBehavior
	sbehavior string

	state int32

	parent   gen.PID
	leader   gen.PID
	fallback gen.ProcessFallback

	mailbox   gen.ProcessMailbox
	priority  gen.MessagePriority
	keeporder bool
	important bool

	messagesIn  uint64
	messagesOut uint64
	runningTime uint64

	compression gen.Compression

	env sync.Map

	// channel for the sync requests made this process
	response chan response

	// created links/monitors
	targets sync.Map // target[PID,ProcessID,Alias,Event] -> true(link), false (monitor)

	// meta processes
	metas sync.Map // metas[Alias] -> *meta

	// gen.Log interface
	log *log

	// if act as a logger
	loggername string
}

type response struct {
	message any
	err     error
	ref     gen.Ref
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

func (p *process) Parent() gen.PID {
	return p.parent
}

func (p *process) Uptime() int64 {
	if p.isAlive() == false {
		return 0
	}
	return time.Now().Unix() - p.creation
}

func (p *process) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	if p.isStateIRW() == false {
		return gen.PID{}, gen.ErrNotAllowed
	}
	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		ParentPID:      p.pid,
		ParentLeader:   p.leader,
		ParentEnv:      p.EnvList(),
		ParentLogLevel: p.log.level,
		Application:    p.application,
		Args:           args,
	}

	pid, err := p.node.spawn(factory, opts)
	if err != nil {
		return pid, err
	}

	if options.LinkChild {
		// method LinkPID is not allowed to be used in the initialization state,
		// so we use linking manually.
		p.node.links.registerConsumer(pid, p.pid)
		p.targets.Store(pid, true)
	}
	return pid, err
}

func (p *process) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {

	if p.isStateIRW() == false {
		return gen.PID{}, gen.ErrNotAllowed
	}

	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		ParentPID:      p.pid,
		ParentLeader:   p.leader,
		ParentEnv:      p.EnvList(),
		Register:       register,
		ParentLogLevel: p.log.level,
		Application:    p.application,
		Args:           args,
	}
	pid, err := p.node.spawn(factory, opts)
	if err != nil {
		return pid, err
	}

	if options.LinkChild {
		// method LinkPID is not allowed to be used in the initialization state,
		// so we use linking manually.
		p.node.links.registerConsumer(pid, p.pid)
		p.targets.Store(pid, true)
	}
	return pid, err
}

func (p *process) SpawnMeta(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	var alias gen.Alias

	// use isAlive instead of isStateIRW because
	// any meta process should be able to spawn other meta process
	if p.isAlive() == false {
		return alias, gen.ErrNotAllowed
	}

	if behavior == nil {
		return alias, errors.New("behavior is nil")
	}

	m := &meta{
		p:        p,
		behavior: behavior,
		state:    int32(gen.MetaStateSleep),
	}
	switch options.SendPriority {
	case gen.MessagePriorityHigh:
		m.priority = gen.MessagePriorityHigh
	case gen.MessagePriorityMax:
		m.priority = gen.MessagePriorityMax
	default:
		m.priority = gen.MessagePriorityNormal
	}
	if options.MailboxSize > 0 {
		m.main = lib.NewQueueLimitMPSC(options.MailboxSize, false)
		m.system = lib.NewQueueLimitMPSC(options.MailboxSize, false)
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

	if err := m.init(); err != nil {
		return alias, err
	}

	// register to be able routing messages to this meta process
	p.metas.Store(m.id, m)
	p.node.aliases.Store(m.id, p)
	go m.start()

	return m.id, nil
}

func (p *process) RemoteSpawn(node gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {

	if p.isStateIRW() == false {
		return gen.PID{}, gen.ErrNotAllowed
	}

	if p.node.Name() == node {
		return gen.PID{}, gen.ErrNotAllowed
	}

	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		ParentPID:      p.pid,
		ParentLeader:   p.leader,
		ParentLogLevel: p.log.level,
		Application:    p.application,
		Args:           args,
	}
	if p.node.Security().ExposeEnvRemoteSpawn {
		opts.ParentEnv = p.EnvList()
	}
	pid, err := p.node.RouteSpawn(node, name, opts, p.Node().Name())
	if err != nil {
		return gen.PID{}, err
	}

	if opts.LinkChild {
		// method LinkPID is not allowed to be used in the initialization state,
		// so we use linking manually.
		p.node.links.registerConsumer(pid, p.pid)
		p.targets.Store(pid, true)
	}

	return pid, err
}

func (p *process) RemoteSpawnRegister(node gen.Atom, name gen.Atom, register gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {

	if p.isStateIRW() == false {
		return gen.PID{}, gen.ErrNotAllowed
	}

	opts := gen.ProcessOptionsExtra{
		ProcessOptions: options,
		ParentPID:      p.pid,
		ParentLeader:   p.leader,
		Register:       register,
		ParentLogLevel: p.log.level,
		Application:    p.application,
		Args:           args,
	}
	if p.node.Security().ExposeEnvRemoteSpawn {
		opts.ParentEnv = p.EnvList()
	}
	pid, err := p.node.RouteSpawn(node, name, opts, p.Node().Name())
	if err != nil {
		return gen.PID{}, err
	}

	if opts.LinkChild {
		// method LinkPID is not allowed to be used in the initialization state,
		// so we use linking manually.
		p.node.links.registerConsumer(pid, p.pid)
		p.targets.Store(pid, true)
	}

	return pid, err
}

func (p *process) State() gen.ProcessState {
	return gen.ProcessState(atomic.LoadInt32(&p.state))
}

func (p *process) RegisterName(name gen.Atom) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}
	if err := p.node.RegisterName(name, p.pid); err != nil {
		return err
	}

	p.log.setSource(gen.MessageLogProcess{Node: p.node.name, PID: p.pid, Name: p.name})
	return nil
}

func (p *process) UnregisterName() error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}
	_, err := p.node.UnregisterName(p.name)
	return err
}

func (p *process) EnvList() map[gen.Env]any {
	if p.isAlive() == false {
		return nil
	}

	env := make(map[gen.Env]any)
	p.env.Range(func(k, v any) bool {
		env[gen.Env(k.(string))] = v
		return true
	})
	return env
}

func (p *process) SetEnv(name gen.Env, value any) {
	if p.isAlive() == false {
		return
	}
	if value == nil {
		p.env.Delete(name.String())
		return
	}
	p.env.Store(name.String(), value)
}

func (p *process) Env(name gen.Env) (any, bool) {
	if p.isAlive() == false {
		return nil, false
	}
	x, y := p.env.Load(name.String())
	return x, y
}

func (p *process) Compression() bool {
	return p.compression.Enable
}

func (p *process) SetCompression(enable bool) error {
	if p.isAlive() == false {
		return gen.ErrProcessTerminated
	}
	p.compression.Enable = enable
	return nil
}

func (p *process) CompressionType() gen.CompressionType {
	return p.compression.Type
}

func (p *process) SetCompressionType(ctype gen.CompressionType) error {
	if p.isAlive() == false {
		return gen.ErrProcessTerminated
	}

	switch ctype {
	case gen.CompressionTypeGZIP:
	case gen.CompressionTypeLZW:
	case gen.CompressionTypeZLIB:
	default:
		return gen.ErrIncorrect
	}

	p.compression.Type = ctype
	return nil
}

func (p *process) CompressionLevel() gen.CompressionLevel {
	return p.compression.Level
}

func (p *process) SetCompressionLevel(level gen.CompressionLevel) error {
	if p.isAlive() == false {
		return gen.ErrProcessTerminated
	}

	switch level {
	case gen.CompressionBestSize:
	case gen.CompressionBestSpeed:
	case gen.CompressionDefault:
	default:
		return gen.ErrIncorrect
	}

	p.compression.Level = level
	return nil
}

func (p *process) CompressionThreshold() int {
	return p.compression.Threshold
}

func (p *process) SetCompressionThreshold(threshold int) error {
	if p.isAlive() == false {
		return gen.ErrProcessTerminated
	}
	if threshold < gen.DefaultCompressionThreshold {
		return gen.ErrIncorrect
	}
	p.compression.Threshold = threshold
	return nil
}

func (p *process) SendPriority() gen.MessagePriority {
	return p.priority
}

func (p *process) SetSendPriority(priority gen.MessagePriority) error {
	if p.isAlive() == false {
		return gen.ErrProcessTerminated
	}

	switch priority {
	case gen.MessagePriorityNormal:
	case gen.MessagePriorityHigh:
	case gen.MessagePriorityMax:
	default:
		return gen.ErrIncorrect
	}
	p.priority = priority
	return nil
}

func (p *process) SetKeepNetworkOrder(order bool) error {
	if p.isAlive() == false {
		return gen.ErrProcessTerminated
	}

	p.keeporder = order
	return nil
}

func (p *process) KeepNetworkOrder() bool {
	return p.keeporder
}

func (p *process) SetImportantDelivery(important bool) error {
	if p.isAlive() == false {
		return gen.ErrProcessTerminated
	}

	p.important = important
	return nil
}

func (p *process) ImportantDelivery() bool {
	return p.important
}

func (p *process) CreateAlias() (gen.Alias, error) {
	if p.isStateRW() == false {
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
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if err := p.node.unregisterAlias(alias, p); err != nil {
		return err
	}

	p.node.RouteTerminateAlias(alias, gen.ErrUnregistered)

	for i, a := range p.aliases {
		if a != alias {
			continue
		}
		p.aliases[0] = p.aliases[i]
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
	var prev gen.MessagePriority
	prev, p.priority = p.priority, priority
	err := p.Send(to, message)
	p.priority = prev
	return err
}

func (p *process) Send(to any, message any) error {
	switch t := to.(type) {
	case gen.PID:
		return p.SendPID(t, message)
	case gen.ProcessID:
		return p.SendProcessID(t, message)
	case gen.Alias:
		return p.SendAlias(t, message)
	case gen.Atom:
		return p.SendProcessID(gen.ProcessID{Name: t, Node: p.node.name}, message)
	case string:
		return p.SendProcessID(gen.ProcessID{Name: gen.Atom(t), Node: p.node.name}, message)
	}

	return gen.ErrUnsupported
}

func (p *process) SendImportant(to any, message any) error {
	var important bool

	important, p.important = p.important, true
	err := p.Send(to, message)
	p.important = important

	return err
}

func (p *process) SendPID(to gen.PID, message any) error {
	// allow to send even being in sleep state (meta-process uses this method)
	if p.isAlive() == false {
		return gen.ErrNotAllowed
	}
	if lib.Trace() {
		p.log.Trace("SendPID to %s", to)
	}

	// Sending to itself being in initialization stage:
	//
	// - we can't route this message to itself (via RouteSendPID) if this process
	//   is in the initialization stage since it isn't registered yet.
	// - message can be routed by the process name (via RouteSendProcessID)
	//   because it is already registered before the invoking ProcessInit callback,
	//   which means we should not do this trick in SendProcessID method.
	//
	// So here, we should check if it is sending to itself and route this message manually
	// right into the process mailbox.

	if to == p.pid {
		// sending to itself
		qm := gen.TakeMailboxMessage()
		qm.From = p.pid
		qm.Type = gen.MailboxMessageTypeRegular
		qm.Target = to
		qm.Message = message

		if ok := p.mailbox.Main.Push(qm); ok == false {
			return gen.ErrProcessMailboxFull
		}

		atomic.AddUint64(&p.messagesIn, 1)
		p.run()
		return nil
	}

	options := gen.MessageOptions{
		Priority:          p.priority,
		Compression:       p.compression,
		KeepNetworkOrder:  p.keeporder,
		ImportantDelivery: p.important,
	}

	if options.ImportantDelivery {
		ref := p.node.MakeRef()
		options.Ref = ref
		options.Ref.ID[0] = ref.ID[0] + ref.ID[1] + ref.ID[2]
		options.Ref.ID[1] = 0
		options.Ref.ID[2] = 0
	}

	if err := p.node.RouteSendPID(p.pid, to, options, message); err != nil {
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
	if p.isAlive() == false {
		return gen.ErrNotAllowed
	}

	if lib.Trace() {
		p.log.Trace("SendProcessID to %s", to)
	}

	options := gen.MessageOptions{
		Priority:          p.priority,
		Compression:       p.compression,
		KeepNetworkOrder:  p.keeporder,
		ImportantDelivery: p.important,
	}

	if options.ImportantDelivery {
		ref := p.node.MakeRef()
		options.Ref = ref
		options.Ref.ID[0] = ref.ID[0] + ref.ID[1] + ref.ID[2]
		options.Ref.ID[1] = 0
		options.Ref.ID[2] = 0
	}

	if err := p.node.RouteSendProcessID(p.pid, to, options, message); err != nil {
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
	if p.isAlive() == false {
		return gen.ErrNotAllowed
	}

	if lib.Trace() {
		p.log.Trace("SendAlias to %s", to)
	}

	options := gen.MessageOptions{
		Priority:          p.priority,
		Compression:       p.compression,
		KeepNetworkOrder:  p.keeporder,
		ImportantDelivery: p.important,
	}

	if options.ImportantDelivery {
		ref := p.node.MakeRef()
		options.Ref = ref
		options.Ref.ID[0] = ref.ID[0] + ref.ID[1] + ref.ID[2]
		options.Ref.ID[1] = 0
		options.Ref.ID[2] = 0
	}

	if err := p.node.RouteSendAlias(p.pid, to, options, message); err != nil {
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

func (p *process) SendAfter(to any, message any, after time.Duration) (gen.CancelFunc, error) {
	if p.isAlive() == false {
		return nil, gen.ErrNotAllowed
	}
	return time.AfterFunc(after, func() {
		var err error
		if lib.Trace() {
			p.log.Trace("SendAfter %s to %s", after, to)
		}
		// we can't use p.Send(...) because it checks the process state
		// and returns gen.ErrNotAllowed, so use p.node.Route* methods for that
		options := gen.MessageOptions{
			Priority:         p.priority,
			Compression:      p.compression,
			KeepNetworkOrder: p.keeporder,
			// ImportantDelivery: ignore on sending with delay
		}
		switch t := to.(type) {
		case gen.Atom:
			err = p.node.RouteSendProcessID(p.pid, gen.ProcessID{Name: t, Node: p.node.name}, options, message)
		case gen.PID:
			err = p.node.RouteSendPID(p.pid, t, options, message)
		case gen.ProcessID:
			err = p.node.RouteSendProcessID(p.pid, t, options, message)
		case gen.Alias:
			err = p.node.RouteSendAlias(p.pid, t, options, message)
		}

		if err == nil {
			atomic.AddUint64(&p.messagesOut, 1)
		}
	}).Stop, nil
}

func (p *process) SendEvent(name gen.Atom, token gen.Ref, message any) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if lib.Trace() {
		p.log.Trace("process SendEvent %s with token %s", name, token)
	}

	options := gen.MessageOptions{
		Priority:         p.priority,
		Compression:      p.compression,
		KeepNetworkOrder: p.keeporder,
	}

	em := gen.MessageEvent{
		Event:     gen.Event{Name: name, Node: p.node.name},
		Timestamp: time.Now().UnixNano(),
		Message:   message,
	}

	if err := p.node.RouteSendEvent(p.pid, token, options, em); err != nil {
		return err
	}

	atomic.AddUint64(&p.messagesOut, 1)
	return nil
}

func (p *process) SendExit(to gen.PID, reason error) error {
	if p.isStateRW() == false {
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

	if lib.Trace() {
		p.log.Trace("SendExit to %s", to)
	}
	err := p.node.RouteSendExit(p.pid, to, reason)
	if err != nil {
		return err
	}
	atomic.AddUint64(&p.messagesOut, 1)
	return nil
}

func (p *process) SendExitMeta(alias gen.Alias, reason error) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
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
	atomic.AddUint64(&p.messagesOut, 1)
	m.handle()
	return nil
}

func (p *process) SendResponse(to gen.PID, ref gen.Ref, message any) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}
	if lib.Trace() {
		p.log.Trace("SendResponse to %s with %s", to, ref)
	}
	options := gen.MessageOptions{
		Ref:              ref,
		Priority:         p.priority,
		Compression:      p.compression,
		KeepNetworkOrder: p.keeporder,
	}
	atomic.AddUint64(&p.messagesOut, 1)
	return p.node.RouteSendResponse(p.pid, to, options, message)
}

func (p *process) SendResponseError(to gen.PID, ref gen.Ref, err error) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}
	if lib.Trace() {
		p.log.Trace("SendResponseError to %s with %s", to, ref)
	}
	options := gen.MessageOptions{
		Ref:              ref,
		Priority:         p.priority,
		Compression:      p.compression,
		KeepNetworkOrder: p.keeporder,
	}
	atomic.AddUint64(&p.messagesOut, 1)
	return p.node.RouteSendResponseError(p.pid, to, options, err)
}

func (p *process) CallWithPriority(to any, request any, priority gen.MessagePriority) (any, error) {
	var prev gen.MessagePriority
	prev, p.priority = p.priority, priority
	value, err := p.CallWithTimeout(to, request, gen.DefaultRequestTimeout)
	p.priority = prev
	return value, err
}

func (p *process) CallImportant(to any, request any) (any, error) {
	var important bool

	important, p.important = p.important, true
	result, err := p.CallWithTimeout(to, request, gen.DefaultRequestTimeout)
	p.important = important

	return result, err
}

func (p *process) Call(to any, request any) (any, error) {
	return p.CallWithTimeout(to, request, gen.DefaultRequestTimeout)
}
func (p *process) CallWithTimeout(to any, request any, timeout int) (any, error) {
	switch t := to.(type) {
	case gen.Atom:
		return p.CallProcessID(gen.ProcessID{Name: t, Node: p.node.name}, request, timeout)
	case gen.PID:
		return p.CallPID(t, request, timeout)
	case gen.ProcessID:
		return p.CallProcessID(t, request, timeout)
	case gen.Alias:
		return p.CallAlias(t, request, timeout)
	}

	return nil, gen.ErrUnsupported

}

func (p *process) CallPID(to gen.PID, message any, timeout int) (any, error) {
	if p.isStateRW() == false {
		return nil, gen.ErrNotAllowed
	}
	if to == p.pid {
		return nil, gen.ErrNotAllowed
	}

	options := gen.MessageOptions{
		Ref:               p.node.MakeRef(),
		Priority:          p.priority,
		Compression:       p.compression,
		KeepNetworkOrder:  p.keeporder,
		ImportantDelivery: p.important,
	}

	if lib.Trace() {
		p.log.Trace("CallPID to %s with %s", to, options.Ref)
	}

	if err := p.node.RouteCallPID(p.pid, to, options, message); err != nil {
		return nil, err
	}

	atomic.AddUint64(&p.messagesOut, 1)
	if timeout < 1 {
		timeout = gen.DefaultRequestTimeout
	}
	return p.waitResponse(options.Ref, timeout)
}

func (p *process) CallProcessID(to gen.ProcessID, message any, timeout int) (any, error) {
	if p.isStateRW() == false {
		return nil, gen.ErrNotAllowed
	}

	options := gen.MessageOptions{
		Ref:               p.node.MakeRef(),
		Priority:          p.priority,
		Compression:       p.compression,
		KeepNetworkOrder:  p.keeporder,
		ImportantDelivery: p.important,
	}
	if lib.Trace() {
		p.log.Trace("CallProcessID %s with %s", to, options.Ref)
	}
	if err := p.node.RouteCallProcessID(p.pid, to, options, message); err != nil {
		return nil, err
	}
	atomic.AddUint64(&p.messagesOut, 1)
	if timeout < 1 {
		timeout = gen.DefaultRequestTimeout
	}
	return p.waitResponse(options.Ref, timeout)
}

func (p *process) CallAlias(to gen.Alias, message any, timeout int) (any, error) {
	if p.isStateRW() == false {
		return nil, gen.ErrNotAllowed
	}

	options := gen.MessageOptions{
		Ref:               p.node.MakeRef(),
		Priority:          p.priority,
		Compression:       p.compression,
		KeepNetworkOrder:  p.keeporder,
		ImportantDelivery: p.important,
	}

	if lib.Trace() {
		p.log.Trace("CallAlias %s with %s", to, options.Ref)
	}

	if err := p.node.RouteCallAlias(p.pid, to, options, message); err != nil {
		return nil, err
	}
	atomic.AddUint64(&p.messagesOut, 1)
	if timeout < 1 {
		timeout = gen.DefaultRequestTimeout
	}
	return p.waitResponse(options.Ref, timeout)
}

func (p *process) Inspect(target gen.PID, item ...string) (map[string]string, error) {
	if p.isStateRW() == false {
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

	if lib.Trace() {
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
	if p.isStateRW() == false {
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

	if lib.Trace() {
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
	if p.isStateRW() == false {
		return empty, gen.ErrNotAllowed
	}

	if lib.Trace() {
		p.log.Trace("process RegisterEvent %s", name)
	}

	token, err := p.node.registerEvent(name, p.pid, options)
	if err != nil {
		return token, err
	}
	p.events.Store(name, true)
	return token, nil
}

func (p *process) UnregisterEvent(name gen.Atom) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if lib.Trace() {
		p.log.Trace("process UnregisterEvent %s", name)
	}

	if err := p.node.unregisterEvent(name, p.pid); err != nil {
		return err
	}

	p.events.Delete(name)
	return nil
}

func (p *process) Events() []gen.Atom {
	events := []gen.Atom{}
	p.events.Range(func(k, _ any) bool {
		events = append(events, k.(gen.Atom))
		return true
	})
	return events
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
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if target == p.pid {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist {
		return gen.ErrTargetExist
	}

	if lib.Trace() {
		p.log.Trace("LinkPID with %s", target)
	}

	if err := p.node.RouteLinkPID(p.pid, target); err != nil {
		return err
	}

	p.targets.Store(target, true)
	return nil
}

func (p *process) UnlinkPID(target gen.PID) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist == false {
		return gen.ErrTargetUnknown
	}

	if lib.Trace() {
		p.log.Trace("UnlinkPID with %s", target)
	}

	if err := p.node.RouteUnlinkPID(p.pid, target); err != nil {
		return err
	}

	p.targets.Delete(target)
	return nil
}

func (p *process) LinkProcessID(target gen.ProcessID) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if target.Name == p.name && target.Node == p.node.name {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist {
		return gen.ErrTargetExist
	}

	if lib.Trace() {
		p.log.Trace("LinkProcessID with %s", target)
	}

	if err := p.node.RouteLinkProcessID(p.pid, target); err != nil {
		return err
	}

	p.targets.Store(target, true)
	return nil
}

func (p *process) UnlinkProcessID(target gen.ProcessID) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist == false {
		return gen.ErrTargetUnknown
	}

	if err := p.node.RouteUnlinkProcessID(p.pid, target); err != nil {
		return err
	}

	p.targets.Delete(target)
	return nil
}

func (p *process) LinkAlias(target gen.Alias) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	for _, a := range p.aliases {
		if a == target {
			return gen.ErrNotAllowed
		}
	}

	if _, exist := p.targets.Load(target); exist {
		return gen.ErrTargetExist
	}

	if err := p.node.RouteLinkAlias(p.pid, target); err != nil {
		return err
	}

	p.targets.Store(target, true)
	return nil
}

func (p *process) UnlinkAlias(target gen.Alias) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist == false {
		return gen.ErrTargetUnknown
	}

	if err := p.node.RouteUnlinkAlias(p.pid, target); err != nil {
		return err
	}

	p.targets.Delete(target)
	return nil
}

func (p *process) LinkEvent(target gen.Event) ([]gen.MessageEvent, error) {

	if p.isStateRW() == false {
		return nil, gen.ErrNotAllowed
	}

	if target.Node == "" {
		target.Node = p.node.name
	}

	if _, exist := p.targets.Load(target); exist {
		return nil, gen.ErrTargetExist
	}

	lastEventMessages, err := p.node.RouteLinkEvent(p.pid, target)
	if err != nil {
		return nil, err
	}

	p.targets.Store(target, true)
	return lastEventMessages, nil
}

func (p *process) UnlinkEvent(target gen.Event) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist == false {
		return gen.ErrTargetUnknown
	}

	if err := p.node.RouteUnlinkEvent(p.pid, target); err != nil {
		return err
	}

	p.targets.Delete(target)
	return nil
}

func (p *process) LinkNode(target gen.Atom) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist {
		return gen.ErrTargetExist
	}

	if _, err := p.Node().Network().GetNode(target); err != nil {
		return err
	}
	p.node.links.registerConsumer(target, p.pid)
	p.targets.Store(target, true)
	return nil
}

func (p *process) UnlinkNode(target gen.Atom) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist == false {
		return gen.ErrTargetUnknown
	}

	p.node.links.unregisterConsumer(target, p.pid)
	p.targets.Delete(target)
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
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist {
		return gen.ErrTargetExist
	}

	if err := p.node.RouteMonitorPID(p.pid, target); err != nil {
		return err
	}

	p.targets.Store(target, false)
	return nil
}

func (p *process) DemonitorPID(target gen.PID) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist == false {
		return gen.ErrTargetUnknown
	}

	if err := p.node.RouteDemonitorPID(p.pid, target); err != nil {
		return err
	}

	p.targets.Delete(target)
	return nil
}

func (p *process) MonitorProcessID(target gen.ProcessID) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist {
		return gen.ErrTargetExist
	}

	if err := p.node.RouteMonitorProcessID(p.pid, target); err != nil {
		return err
	}

	p.targets.Store(target, false)
	return nil
}

func (p *process) DemonitorProcessID(target gen.ProcessID) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist == false {
		return gen.ErrTargetUnknown
	}

	if err := p.node.RouteDemonitorProcessID(p.pid, target); err != nil {
		return err
	}

	p.targets.Delete(target)
	return nil
}

func (p *process) MonitorAlias(target gen.Alias) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist {
		return gen.ErrTargetExist
	}

	if err := p.node.RouteMonitorAlias(p.pid, target); err != nil {
		return err
	}

	p.targets.Store(target, false)
	return nil
}

func (p *process) DemonitorAlias(target gen.Alias) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist == false {
		return gen.ErrTargetUnknown
	}

	if err := p.node.RouteDemonitorAlias(p.pid, target); err != nil {
		return err
	}

	p.targets.Delete(target)
	return nil
}

func (p *process) MonitorEvent(target gen.Event) ([]gen.MessageEvent, error) {

	if p.isStateRW() == false {
		return nil, gen.ErrNotAllowed
	}

	if target.Node == "" {
		target.Node = p.node.name
	}

	if _, exist := p.targets.Load(target); exist {
		return nil, gen.ErrTargetExist
	}

	lastEventMessages, err := p.node.RouteMonitorEvent(p.pid, target)
	if err != nil {
		return nil, err
	}

	p.targets.Store(target, false)
	return lastEventMessages, nil
}

func (p *process) DemonitorEvent(target gen.Event) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist == false {
		return gen.ErrTargetUnknown
	}

	if err := p.node.RouteDemonitorEvent(p.pid, target); err != nil {
		return err
	}

	p.targets.Delete(target)
	return nil
}

func (p *process) MonitorNode(target gen.Atom) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}

	if _, exist := p.targets.Load(target); exist {
		return gen.ErrTargetExist
	}

	if _, err := p.Node().Network().GetNode(target); err != nil {
		return err
	}
	p.node.monitors.registerConsumer(target, p.pid)
	p.targets.Store(target, false)
	return nil
}

func (p *process) DemonitorNode(target gen.Atom) error {
	if p.isStateRW() == false {
		return gen.ErrNotAllowed
	}
	if _, exist := p.targets.Load(target); exist == false {
		return gen.ErrTargetUnknown
	}

	p.node.monitors.unregisterConsumer(target, p.pid)
	p.targets.Delete(target)
	return nil
}

func (p *process) Log() gen.Log {
	return p.log
}

func (p *process) Info() (gen.ProcessInfo, error) {
	if p.isStateRW() == false {
		return gen.ProcessInfo{}, gen.ErrNotAllowed
	}
	return p.node.ProcessInfo(p.pid)
}

func (p *process) MetaInfo(m gen.Alias) (gen.MetaInfo, error) {
	if p.isStateRW() == false {
		return gen.MetaInfo{}, gen.ErrNotAllowed
	}
	return p.node.MetaInfo(m)
}

func (p *process) Mailbox() gen.ProcessMailbox {
	return p.mailbox
}

func (p *process) Behavior() gen.ProcessBehavior {
	return p.behavior
}

func (p *process) Forward(to gen.PID, message *gen.MailboxMessage, priority gen.MessagePriority) error {
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

func (p *process) run() {
	if atomic.CompareAndSwapInt32(&p.state, int32(gen.ProcessStateSleep), int32(gen.ProcessStateRunning)) == false {
		// already running or terminated
		return
	}
	go func() {
		if lib.Recover() {
			defer func() {
				if rcv := recover(); rcv != nil {
					pc, fn, line, _ := runtime.Caller(2)
					p.log.Panic("process terminated - %#v at %s[%s:%d]",
						rcv, runtime.FuncForPC(pc).Name(), fn, line)
					old := atomic.SwapInt32(&p.state, int32(gen.ProcessStateTerminated))
					if old == int32(gen.ProcessStateTerminated) {
						return
					}
					p.node.unregisterProcess(p, gen.TerminateReasonPanic)
					p.behavior.ProcessTerminate(gen.TerminateReasonPanic)
				}
			}()
		}
	next:
		startTime := time.Now().UnixNano()
		// handle mailbox
		if err := p.behavior.ProcessRun(); err != nil {
			p.runningTime = p.runningTime + uint64(time.Now().UnixNano()-startTime)
			e := errors.Unwrap(err)
			if e == nil {
				e = err
			}
			if e != gen.TerminateReasonNormal && e != gen.TerminateReasonShutdown {
				p.log.Error("process terminated abnormally - %s", err)
			}

			old := atomic.SwapInt32(&p.state, int32(gen.ProcessStateTerminated))
			if old == int32(gen.ProcessStateTerminated) {
				return
			}

			p.node.unregisterProcess(p, e)
			p.behavior.ProcessTerminate(err)
			return
		}

		// count the running time
		p.runningTime = p.runningTime + uint64(time.Now().UnixNano()-startTime)

		// change running state to sleep
		if atomic.CompareAndSwapInt32(&p.state, int32(gen.ProcessStateRunning), int32(gen.ProcessStateSleep)) == false {
			// process has been killed (was in zombee state)
			old := atomic.SwapInt32(&p.state, int32(gen.ProcessStateTerminated))
			if old == int32(gen.ProcessStateTerminated) {
				return
			}
			p.node.unregisterProcess(p, gen.TerminateReasonKill)
			p.behavior.ProcessTerminate(gen.TerminateReasonKill)
			return
		}
		// check if something left in the inbox and try to handle it
		if p.mailbox.Main.Item() == nil {
			if p.mailbox.System.Item() == nil {
				if p.mailbox.Urgent.Item() == nil {
					if p.mailbox.Log.Item() == nil {
						// inbox is emtpy
						return
					}
				}
			}
		}
		// we got a new messages. try to use this goroutine again
		if atomic.CompareAndSwapInt32(&p.state, int32(gen.ProcessStateSleep), int32(gen.ProcessStateRunning)) == false {
			// another goroutine is already running
			return
		}
		goto next
	}()
}

func (p *process) isStateIRW() bool {
	state := atomic.LoadInt32(&p.state)
	irw := int32(gen.ProcessStateInit) |
		int32(gen.ProcessStateRunning) |
		int32(gen.ProcessStateWaitResponse)
	return (state & irw) == state
}

func (p *process) isStateRW() bool {
	state := atomic.LoadInt32(&p.state)
	rw := int32(gen.ProcessStateRunning) |
		int32(gen.ProcessStateWaitResponse)
	return (state & rw) == state
}

func (p *process) isAlive() bool {
	state := atomic.LoadInt32(&p.state)
	alive := int32(gen.ProcessStateInit) |
		int32(gen.ProcessStateSleep) |
		int32(gen.ProcessStateRunning) |
		int32(gen.ProcessStateWaitResponse)
	return (state & alive) == state
}

func (p *process) isStateSRW() bool {
	state := atomic.LoadInt32(&p.state)
	alive := int32(gen.ProcessStateSleep) |
		int32(gen.ProcessStateRunning) |
		int32(gen.ProcessStateWaitResponse)
	return (state & alive) == state
}

func (p *process) waitResponse(ref gen.Ref, timeout int) (any, error) {
	var response any
	var err error

	if swapped := atomic.CompareAndSwapInt32(&p.state, int32(gen.ProcessStateRunning), int32(gen.ProcessStateWaitResponse)); swapped == false {
		return nil, gen.ErrNotAllowed
	}

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)

	if timeout == 0 {
		timer.Reset(time.Second * time.Duration(gen.DefaultRequestTimeout))
	} else {
		timer.Reset(time.Second * time.Duration(timeout))
	}

retry:
	select {
	case <-timer.C:
		if lib.Trace() {
			p.log.Trace("request with ref %s is timed out", ref)
		}
		err = gen.ErrTimeout
	case r := <-p.response:
		if r.ref != ref {
			// we got a late response to the previous request that has been timed
			// out earlier and we made another request with the new reference - Ref.
			// just drop it and wait one more time
			if lib.Trace() {
				p.log.Trace("got late response on request with ref %s (exp %s). dropped", r.ref, ref)
			}
			goto retry
		}
		response = r.message
		err = r.err
	}

	if swapped := atomic.CompareAndSwapInt32(&p.state, int32(gen.ProcessStateWaitResponse), int32(gen.ProcessStateRunning)); swapped == false {
		return nil, gen.ErrProcessTerminated
	}
	return response, err
}
