package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/lib"
)

const (
	DefaultProcessMailboxSize = 100
	DefaultCallTimeout        = 5
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

// Self returns self Pid
func (p *process) Self() etf.Pid {
	return p.self
}

// Name returns registered name of the process
func (p *process) Name() string {
	return p.name
}

func (p *process) Kill() {
	p.kill()
}

func (p *process) Exit(reason string) error {
	return p.exit(p.self, reason)
}

func (p *process) Context() context.Context {
	return p.context
}

func (p *process) GetParent() gen.Process {
	return p.parent
}

func (p *process) GetGroupLeader() gen.Process {
	return p.groupLeader
}

// Info returns detailed information about the process
func (p *process) Info() gen.ProcessInfo {
	gl := p.self
	if p.groupLeader != nil {
		gl = p.groupLeader.Self()
	}
	links := p.GetLinks(p.self)
	monitors := p.GetMonitors(p.self)
	monitoredBy := p.GetMonitoredBy(p.self)
	aliases := append(make([]etf.Alias, len(p.aliases)), p.aliases...)
	return gen.ProcessInfo{
		PID:             p.self,
		Name:            p.name,
		GroupLeader:     gl,
		Links:           links,
		Monitors:        monitors,
		MonitoredBy:     monitoredBy,
		Aliases:         aliases,
		Status:          "running",
		MessageQueueLen: len(p.mailBox),
		TrapExit:        p.trapExit,
	}
}

// Call makes outgoing sync request in fashion of 'gen_call'.
// 'to' can be Pid, registered local name or a tuple {RegisteredName, NodeName}.
// This method shouldn't be used outside of the actor. Use Direct method instead.
func (p *process) Call(to interface{}, message etf.Term) (etf.Term, error) {
	return p.CallWithTimeout(to, message, DefaultCallTimeout)
}

// CallWithTimeout makes outgoing sync request in fashiod of 'gen_call' with given timeout.
// This method shouldn't be used outside of the actor. Use DirectWithTimeout method instead.
func (p *process) CallWithTimeout(to interface{}, message etf.Term, timeout int) (etf.Term, error) {
	if !p.IsAlive() {
		return nil, ErrProcessTerminated
	}

	ref := p.MakeRef()
	from := etf.Tuple{p.self, ref}
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_call"), from, message})
	p.SendSyncRequest(ref, to, msg)

	return p.WaitSyncReply(ref, timeout)

}

// CallRPC evaluate rpc call with given node/MFA
func (p *process) CallRPC(node, module, function string, args ...etf.Term) (etf.Term, error) {
	return p.CallRPCWithTimeout(DefaultCallTimeout, node, module, function, args...)
}

// CallRPCWithTimeout evaluate rpc call with given node/MFA and timeout
func (p *process) CallRPCWithTimeout(timeout int, node, module, function string, args ...etf.Term) (etf.Term, error) {
	lib.Log("[%s] RPC calling: %s:%s:%s", p.NodeName(), node, module, function)
	message := etf.Tuple{
		etf.Atom("call"),
		etf.Atom(module),
		etf.Atom(function),
		etf.List(args),
		p.Self(),
	}
	to := etf.Tuple{etf.Atom("rex"), etf.Atom(node)}
	return p.CallWithTimeout(to, message, timeout)
}

// CastRPC evaluate rpc cast with given node/MFA
func (p *process) CastRPC(node, module, function string, args ...etf.Term) {
	lib.Log("[%s] RPC casting: %s:%s:%s", p.NodeName(), node, module, function)
	message := etf.Tuple{
		etf.Atom("cast"),
		etf.Atom(module),
		etf.Atom(function),
		etf.List(args),
	}
	to := etf.Tuple{etf.Atom("rex"), etf.Atom(node)}
	p.Cast(to, message)
}

// Send sends a message. 'to' can be a Pid, registered local name
// or a tuple {RegisteredName, NodeName}
func (p *process) Send(to interface{}, message etf.Term) error {
	if !p.IsAlive() {
		return ErrProcessTerminated
	}
	p.Route(p.self, to, message)
	return nil
}

// SendAfter starts a timer. When the timer expires, the message sends to the process identified by 'to'.
// 'to' can be a Pid, registered local name or a tuple {RegisteredName, NodeName}.
// Returns cancel function in order to discard sending a message
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
				p.Route(p.self, to, message)
			}
		}
	}()
	return cancel
}

// CastAfter simple wrapper for SendAfter to send '$gen_cast' message
func (p *process) CastAfter(to interface{}, message etf.Term, after time.Duration) context.CancelFunc {
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), message})
	return p.SendAfter(to, msg, after)
}

// Cast sends a message in fashion of 'gen_cast'.
// 'to' can be a Pid, registered local name
// or a tuple {RegisteredName, NodeName}
func (p *process) Cast(to interface{}, message etf.Term) error {
	if !p.IsAlive() {
		return ErrProcessTerminated
	}
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), message})
	p.Route(p.self, to, msg)
	return nil
}

// CreateAlias creates a new alias for the Process
func (p *process) CreateAlias() (etf.Alias, error) {
	return p.newAlias(p)
}

// DeleteAlias deletes the given alias
func (p *process) DeleteAlias(alias etf.Alias) error {
	return p.deleteAlias(p, alias)
}

// ListEnv returns a map of configured environment variables.
// It also includes environment variables from the GroupLeader and Parent.
// which are overlapped by priority: Process(Parent(GroupLeader))
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

// SetEnv set environment variable with given name. Use nil value to remove variable with given name.
func (p *process) SetEnv(name string, value interface{}) {
	p.Lock()
	defer p.Unlock()
	if p.env == nil {
		p.env = make(map[string]interface{})
	}
	if value == nil {
		delete(p.env, name)
		return
	}
	p.env[name] = value
}

// GetEnv returns value associated with given environment name.
func (p *process) GetEnv(name string) interface{} {
	p.RLock()
	defer p.RUnlock()

	if value, ok := p.env[name]; ok {
		return value
	}

	if p.groupLeader != nil {
		return p.groupLeader.GetEnv(name)
	}

	return nil
}

// Wait waits until process stopped
func (p *process) Wait() {
	<-p.context.Done()
}

// WaitWithTimeout waits until process stopped. Return ErrTimeout
// if given timeout is exceeded
func (p *process) WaitWithTimeout(d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return ErrTimeout
	case <-p.context.Done():
		return nil
	}
}

// Link creates a link between the calling process and another process
func (p *process) Link(with etf.Pid) {
	p.link(p.self, with)
}

// Unlink removes the link, if there is one, between the calling process and the process referred to by Pid.
func (p *process) Unlink(with etf.Pid) {
	p.unlink(p.self, with)
}

// IsAlive returns whether the process is alive
func (p *process) IsAlive() bool {
	return p.context.Err() == nil
}

// GetChildren returns list of children pid (Application, Supervisor)
func (p *process) GetChildren() []etf.Pid {
	c, err := p.directRequest("$getChildren", nil, 5)
	if err == nil {
		return c.([]etf.Pid)
	}
	return []etf.Pid{}
}

// SetTrapExit enables/disables the trap on terminate process
func (p *process) SetTrapExit(trap bool) {
	p.trapExit = trap
}

// GetTrapExit returns whether the trap was enabled on this process
func (p *process) GetTrapExit() bool {
	return p.trapExit
}

// GetObject returns object this process runs on.
func (p *process) GetProcessBehavior() gen.ProcessBehavior {
	return p.behavior
}

// Direct make a direct request to the actor (Application, Supervisor, GenServer or inherited from GenServer actor) with default timeout 5 seconds
func (p *process) Direct(request interface{}) (interface{}, error) {
	return p.directRequest("", request, DefaultCallTimeout)
}

// DirectWithTimeout make a direct request to the actor with the given timeout (in seconds)
func (p *process) DirectWithTimeout(request interface{}, timeout int) (interface{}, error) {
	if timeout < 1 {
		timeout = 5
	}
	return p.directRequest("", request, timeout)
}

// MonitorNode creates monitor between the current process and node. If Node fails or does not exist,
// the message {nodedown, Node} is delivered to the process.
func (p *process) MonitorNode(name string) etf.Ref {
	return p.monitorNode(p.self, name)
}

// DemonitorNode removes monitor. Returns false if the given reference wasn't found
func (p *process) DemonitorNode(ref etf.Ref) bool {
	return p.demonitorNode(ref)
}

// MonitorProcess creates monitor between the processes.
// 'process' value can be: etf.Pid, registered local name etf.Atom or
// remote registered name etf.Tuple{Name etf.Atom, Node etf.Atom}
// When a process monitor is triggered, a 'DOWN' message sends with the following
// pattern: {'DOWN', MonitorRef, Type, Object, Info} where
// Info has the following values:
//  - the exit reason of the process
//  - 'noproc' (process did not exist at the time of monitor creation)
//  - 'noconnection' (no connection to the node where the monitored process resides)
// Note: The monitor request is an asynchronous signal. That is, it takes time before the signal reaches its destination.
func (p *process) MonitorProcess(process interface{}) etf.Ref {
	ref := p.MakeRef()
	p.monitorProcess(p.self, process, ref)
	return ref
}

// DemonitorProcess removes monitor. Returns false if the given reference wasn't found
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
		opts.Timeout = DefaultCallTimeout
	}
	// distProtoSPAWN_REQUEST = 29
	control := etf.Tuple{29, ref, p.self, p.self,
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

func (p *process) directRequest(id string, request interface{}, timeout int) (interface{}, error) {
	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)

	direct := gen.ProcessDirectMessage{
		ID:      id,
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

func (p *process) SendSyncRequestRaw(ref etf.Ref, node etf.Atom, messages ...etf.Term) {
	reply := make(chan etf.Term, 2)
	p.replyMutex.Lock()
	defer p.replyMutex.Unlock()
	p.reply[ref] = reply
	p.RouteRaw(node, messages...)
}
func (p *process) SendSyncRequest(ref etf.Ref, to interface{}, message etf.Term) {
	reply := make(chan etf.Term, 2)
	p.replyMutex.Lock()
	defer p.replyMutex.Unlock()
	p.reply[ref] = reply
	p.Send(to, message)
}

func (p *process) PutSyncReply(ref etf.Ref, reply etf.Term) {
	p.replyMutex.Lock()
	rep, ok := p.reply[ref]
	p.replyMutex.Unlock()
	if !ok {
		// ignored, no process waiting for the reply
		return
	}
	select {
	case rep <- reply:
	}

}

func (p *process) WaitSyncReply(ref etf.Ref, timeout int) (etf.Term, error) {
	p.replyMutex.Lock()
	reply, wait_for_reply := p.reply[ref]
	p.replyMutex.Unlock()

	if !wait_for_reply {
		return nil, fmt.Errorf("Unknown request")
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

func (p *process) GetProcessChannels() gen.ProcessChannels {
	return gen.ProcessChannels{
		Mailbox:      p.mailBox,
		Direct:       p.direct,
		GracefulExit: p.gracefulExit,
	}
}
