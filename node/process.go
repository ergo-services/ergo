package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

const (
	DefaultProcessMailboxSize = 100
)

type process struct {
	registrarInternal
	sync.RWMutex

	name     string
	self     etf.Pid
	behavior gen.ProcessBehavior
	env      map[string]interface{}

	parent      *process
	groupLeader gen.Process
	aliases     []etf.Alias

	mailBox      chan gen.ProcessMailboxMessage
	gracefulExit chan gen.ProcessGracefulExitRequest
	direct       chan gen.ProcessDirectMessage

	context context.Context
	kill    context.CancelFunc
	exit    processExitFunc

	replyMutex sync.Mutex
	reply      map[etf.Ref]chan etf.Term

	trapExit bool
}

type processOptions struct {
	gen.ProcessOptions
	parent *process
}

type processExitFunc func(from etf.Pid, reason string) error

func (p *process) Self() etf.Pid {
	return p.self
}

func (p *process) Name() string {
	return p.name
}

func (p *process) RegisterName(name string) error {
	if p.behavior == nil {
		return ErrProcessTerminated
	}
	return p.registerName(name, p.self)
}

func (p *process) UnregisterName(name string) error {
	if p.behavior == nil {
		return ErrProcessTerminated
	}
	prc := p.ProcessByName(name)
	if prc == nil {
		return ErrNameUnknown
	}
	if prc.Self() != p.self {
		return ErrNameOwner
	}
	return p.unregisterName(name)
}

func (p *process) Kill() {
	if p.behavior == nil {
		return
	}
	p.kill()
}

func (p *process) Exit(reason string) error {
	if p.behavior == nil {
		return ErrProcessTerminated
	}
	return p.exit(p.self, reason)
}

func (p *process) Context() context.Context {
	return p.context
}

func (p *process) Parent() gen.Process {
	if p.parent == nil {
		return nil
	}
	return p.parent
}

func (p *process) GroupLeader() gen.Process {
	if p.groupLeader == nil {
		return nil
	}
	return p.groupLeader
}

func (p *process) Links() []etf.Pid {
	return p.processLinks(p.self)
}
func (p *process) Monitors() []etf.Pid {
	return p.processMonitors(p.self)
}
func (p *process) MonitorsByName() []gen.ProcessID {
	return p.processMonitorsByName(p.self)
}
func (p *process) MonitoredBy() []etf.Pid {
	return p.processMonitoredBy(p.self)
}
func (p *process) Aliases() []etf.Alias {
	return p.aliases
}

func (p *process) Info() gen.ProcessInfo {
	if p.behavior == nil {
		return gen.ProcessInfo{}
	}

	gl := p.self
	if p.groupLeader != nil {
		gl = p.groupLeader.Self()
	}
	links := p.Links()
	monitors := p.Monitors()
	monitorsByName := p.MonitorsByName()
	monitoredBy := p.MonitoredBy()
	return gen.ProcessInfo{
		PID:             p.self,
		Name:            p.name,
		GroupLeader:     gl,
		Links:           links,
		Monitors:        monitors,
		MonitorsByName:  monitorsByName,
		MonitoredBy:     monitoredBy,
		Aliases:         p.aliases,
		Status:          "running",
		MessageQueueLen: len(p.mailBox),
		TrapExit:        p.trapExit,
	}
}

func (p *process) Send(to interface{}, message etf.Term) error {
	if p.behavior == nil {
		return ErrProcessTerminated
	}
	return p.route(p.self, to, message)
}

func (p *process) SendAfter(to interface{}, message etf.Term, after time.Duration) context.CancelFunc {
	//TODO: should we control the number of timers/goroutines have been created this way?
	ctx, cancel := context.WithCancel(p.context)
	go func() {
		// to prevent of timer leaks due to its not GCed until the timer fires
		timer := time.NewTimer(after)
		defer timer.Stop()
		defer cancel()

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if p.IsAlive() {
				p.route(p.self, to, message)
			}
		}
	}()
	return cancel
}

func (p *process) CreateAlias() (etf.Alias, error) {
	if p.behavior == nil {
		return etf.Alias{}, ErrProcessTerminated
	}
	return p.newAlias(p)
}

func (p *process) DeleteAlias(alias etf.Alias) error {
	if p.behavior == nil {
		return ErrProcessTerminated
	}
	return p.deleteAlias(p, alias)
}

func (p *process) ListEnv() map[string]interface{} {
	p.RLock()
	defer p.RUnlock()

	env := make(map[string]interface{})

	if p.groupLeader != nil {
		for key, value := range p.groupLeader.ListEnv() {
			env[key] = value
		}
	}
	if p.parent != nil {
		for key, value := range p.parent.ListEnv() {
			env[key] = value
		}
	}
	for key, value := range p.env {
		env[key] = value
	}

	return env
}

func (p *process) SetEnv(name string, value interface{}) {
	p.Lock()
	defer p.Unlock()
	if value == nil {
		delete(p.env, name)
		return
	}
	p.env[name] = value
}

func (p *process) Env(name string) interface{} {
	p.RLock()
	defer p.RUnlock()

	if value, ok := p.env[name]; ok {
		return value
	}

	if p.groupLeader != nil {
		return p.groupLeader.Env(name)
	}

	return nil
}

func (p *process) Wait() {
	if p.IsAlive() {
		<-p.context.Done()
	}
}

func (p *process) WaitWithTimeout(d time.Duration) error {
	if !p.IsAlive() {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return ErrTimeout
	case <-p.context.Done():
		return nil
	}
}

func (p *process) Link(with etf.Pid) {
	if p.behavior == nil {
		return
	}
	p.link(p.self, with)
}

func (p *process) Unlink(with etf.Pid) {
	p.Lock()
	defer p.Unlock()
	if p.behavior == nil {
		return
	}
	p.unlink(p.self, with)
}

func (p *process) IsAlive() bool {
	p.Lock()
	defer p.Unlock()
	if p.behavior == nil {
		return false
	}
	return p.context.Err() == nil
}

func (p *process) Children() ([]etf.Pid, error) {
	c, err := p.directRequest(gen.MessageDirectChildren{}, 5)
	if err == nil {
		return c.([]etf.Pid), nil
	}
	return []etf.Pid{}, err
}

func (p *process) SetTrapExit(trap bool) {
	p.trapExit = trap
}

func (p *process) TrapExit() bool {
	return p.trapExit
}

func (p *process) Behavior() gen.ProcessBehavior {
	p.Lock()
	defer p.Unlock()
	if p.behavior == nil {
		return nil
	}
	return p.behavior
}

func (p *process) Direct(request interface{}) (interface{}, error) {
	return p.directRequest(request, gen.DefaultCallTimeout)
}

func (p *process) DirectWithTimeout(request interface{}, timeout int) (interface{}, error) {
	if timeout < 1 {
		timeout = 5
	}
	return p.directRequest(request, timeout)
}

func (p *process) MonitorNode(name string) etf.Ref {
	return p.monitorNode(p.self, name)
}

func (p *process) DemonitorNode(ref etf.Ref) bool {
	return p.demonitorNode(ref)
}

func (p *process) MonitorProcess(process interface{}) etf.Ref {
	ref := p.MakeRef()
	p.monitorProcess(p.self, process, ref)
	return ref
}

func (p *process) DemonitorProcess(ref etf.Ref) bool {
	return p.demonitorProcess(ref)
}

func (p *process) RemoteSpawn(node string, object string, opts gen.RemoteSpawnOptions, args ...etf.Term) (etf.Pid, error) {
	ref := p.MakeRef()
	optlist := etf.List{}
	if opts.RegisterName != "" {
		optlist = append(optlist, etf.Tuple{etf.Atom("name"), etf.Atom(opts.RegisterName)})

	}
	if opts.Timeout == 0 {
		opts.Timeout = gen.DefaultCallTimeout
	}
	control := etf.Tuple{distProtoSPAWN_REQUEST, ref, p.self, p.self,
		// {M,F,A}
		etf.Tuple{etf.Atom(object), etf.Atom(opts.Function), len(args)},
		optlist,
	}
	p.SendSyncRequestRaw(ref, etf.Atom(node), append([]etf.Term{control}, args)...)
	reply, err := p.WaitSyncReply(ref, opts.Timeout)
	if err != nil {
		return etf.Pid{}, err
	}

	// Result of the operation. If Result is a process identifier,
	// the operation succeeded and the process identifier is the
	// identifier of the newly created process. If Result is an atom,
	// the operation failed and the atom identifies failure reason.
	switch r := reply.(type) {
	case etf.Pid:
		m := etf.Ref{} // empty reference
		if opts.Monitor != m {
			p.monitorProcess(p.self, r, opts.Monitor)
		}
		if opts.Link {
			p.Link(r)
		}
		return r, nil
	case etf.Atom:
		switch string(r) {
		case ErrTaken.Error():
			return etf.Pid{}, ErrTaken

		}
		return etf.Pid{}, fmt.Errorf(string(r))
	}

	return etf.Pid{}, fmt.Errorf("unknown result: %#v", reply)
}

func (p *process) Spawn(name string, opts gen.ProcessOptions, behavior gen.ProcessBehavior, args ...etf.Term) (gen.Process, error) {
	options := processOptions{
		ProcessOptions: opts,
		parent:         p,
	}
	return p.spawn(name, options, behavior, args...)
}

func (p *process) directRequest(request interface{}, timeout int) (interface{}, error) {
	if p.direct == nil {
		return nil, ErrProcessTerminated
	}

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)

	direct := gen.ProcessDirectMessage{
		Message: request,
		Reply:   make(chan gen.ProcessDirectMessage, 1),
	}

	// sending request
	select {
	case p.direct <- direct:
		timer.Reset(time.Second * time.Duration(timeout))
	case <-timer.C:
		return nil, ErrProcessBusy
	}

	// receiving response
	select {
	case response := <-direct.Reply:
		if response.Err != nil {
			return nil, response.Err
		}

		return response.Message, nil
	case <-timer.C:
		return nil, ErrTimeout
	}
}

func (p *process) SendSyncRequestRaw(ref etf.Ref, node etf.Atom, messages ...etf.Term) error {
	if p.reply == nil {
		return ErrProcessTerminated
	}
	reply := make(chan etf.Term, 2)
	p.replyMutex.Lock()
	defer p.replyMutex.Unlock()
	p.reply[ref] = reply
	return p.routeRaw(node, messages...)
}
func (p *process) SendSyncRequest(ref etf.Ref, to interface{}, message etf.Term) error {
	if p.reply == nil {
		return ErrProcessTerminated
	}
	p.replyMutex.Lock()
	defer p.replyMutex.Unlock()

	reply := make(chan etf.Term, 2)
	p.reply[ref] = reply

	return p.Send(to, message)
}

func (p *process) PutSyncReply(ref etf.Ref, reply etf.Term) error {
	if p.reply == nil {
		return ErrProcessTerminated
	}
	p.replyMutex.Lock()
	rep, ok := p.reply[ref]
	p.replyMutex.Unlock()
	if !ok {
		// ignored, no process waiting for the reply
		return nil
	}
	rep <- reply

	return nil
}

func (p *process) WaitSyncReply(ref etf.Ref, timeout int) (etf.Term, error) {
	p.replyMutex.Lock()
	reply, wait_for_reply := p.reply[ref]
	p.replyMutex.Unlock()

	if !wait_for_reply {
		return nil, fmt.Errorf("unknown request")
	}

	defer func(ref etf.Ref) {
		p.replyMutex.Lock()
		delete(p.reply, ref)
		p.replyMutex.Unlock()
	}(ref)

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)
	timer.Reset(time.Second * time.Duration(timeout))

	for {
		select {
		case m := <-reply:
			return m, nil
		case <-timer.C:
			return nil, ErrTimeout
		case <-p.context.Done():
			return nil, ErrProcessTerminated
		}
	}

}

func (p *process) ProcessChannels() gen.ProcessChannels {
	return gen.ProcessChannels{
		Mailbox:      p.mailBox,
		Direct:       p.direct,
		GracefulExit: p.gracefulExit,
	}
}
