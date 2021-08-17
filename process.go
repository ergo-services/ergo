package ergo

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
)

type ProcessType = string

const (
	DefaultProcessMailboxSize = 100
	DefaultCallTimeout        = 5
)

type Process struct {
	sync.RWMutex

	mailBox      chan mailboxMessage
	ready        chan error
	gracefulExit chan gracefulExitRequest
	direct       chan directMessage
	stopped      chan bool
	self         etf.Pid
	groupLeader  *Process
	Context      context.Context
	Kill         context.CancelFunc
	Exit         ProcessExitFunc
	name         string
	Node         *Node

	object ProcessBehavior

	replyMutex sync.Mutex
	reply      map[etf.Ref]chan etf.Term

	env map[string]interface{}

	aliases []etf.Alias

	parent          *Process
	reductions      uint64 // we use this term to count total number of processed messages from mailBox
	currentFunction string

	trapExit bool
}

type mailboxMessage struct {
	from    etf.Pid
	message interface{}
}

type directMessage struct {
	id      string
	message interface{}
	err     error
	reply   chan directMessage
}

type gracefulExitRequest struct {
	from   etf.Pid
	reason string
}

// ProcessInfo struct with process details
type ProcessInfo struct {
	PID             etf.Pid
	Name            string
	CurrentFunction string
	Status          string
	MessageQueueLen int
	Links           []etf.Pid
	Monitors        []etf.Pid
	MonitoredBy     []etf.Pid
	Aliases         []etf.Alias
	Dictionary      etf.Map
	TrapExit        bool
	GroupLeader     etf.Pid
	Reductions      uint64
}

type ProcessOptions struct {
	MailboxSize uint16
	GroupLeader *Process
	Context     context.Context
}

// ProcessExitFunc initiate a graceful stopping process
type ProcessExitFunc func(from etf.Pid, reason string)

// ProcessBehavior interface contains methods you should implement to make own process behaviour
type ProcessBehavior interface {
	Loop(*Process, ...etf.Term) string // method which implements control flow of process
}

// Self returns self Pid
func (p *Process) Self() etf.Pid {
	return p.self
}

// Name returns registered name of the process
func (p *Process) Name() string {
	return p.name
}

// Info returns detailed information about the process
func (p *Process) Info() ProcessInfo {
	gl := p.self
	if p.groupLeader != nil {
		gl = p.groupLeader.self
	}
	links := p.Node.monitor.GetLinks(p.self)
	monitors := p.Node.monitor.GetMonitors(p.self)
	monitoredBy := p.Node.monitor.GetMonitoredBy(p.self)
	aliases := append(make([]etf.Alias, len(p.aliases)), p.aliases...)
	return ProcessInfo{
		PID:             p.self,
		Name:            p.name,
		CurrentFunction: p.currentFunction,
		GroupLeader:     gl,
		Links:           links,
		Monitors:        monitors,
		MonitoredBy:     monitoredBy,
		Aliases:         aliases,
		Status:          "running",
		MessageQueueLen: len(p.mailBox),
		TrapExit:        p.trapExit,
		Reductions:      p.reductions,
	}
}

// Call makes outgoing sync request in fashion of 'gen_call'.
// 'to' can be Pid, registered local name or a tuple {RegisteredName, NodeName}.
// This method shouldn't be used outside of the actor. Use Direct method instead.
func (p *Process) Call(to interface{}, message etf.Term) (etf.Term, error) {
	return p.CallWithTimeout(to, message, DefaultCallTimeout)
}

// CallWithTimeout makes outgoing sync request in fashiod of 'gen_call' with given timeout.
// This method shouldn't be used outside of the actor. Use DirectWithTimeout method instead.
func (p *Process) CallWithTimeout(to interface{}, message etf.Term, timeout int) (etf.Term, error) {
	if !p.IsAlive() {
		return nil, ErrProcessTerminated
	}

	ref := p.Node.MakeRef()
	from := etf.Tuple{p.self, ref}
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_call"), from, message})
	p.sendSyncRequest(ref, to, msg)

	return p.waitSyncReply(ref, timeout)

}

// CallRPC evaluate rpc call with given node/MFA
func (p *Process) CallRPC(node, module, function string, args ...etf.Term) (etf.Term, error) {
	return p.CallRPCWithTimeout(DefaultCallTimeout, node, module, function, args...)
}

// CallRPCWithTimeout evaluate rpc call with given node/MFA and timeout
func (p *Process) CallRPCWithTimeout(timeout int, node, module, function string, args ...etf.Term) (etf.Term, error) {
	lib.Log("[%s] RPC calling: %s:%s:%s", p.Node.Name(), node, module, function)
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
func (p *Process) CastRPC(node, module, function string, args ...etf.Term) {
	lib.Log("[%s] RPC casting: %s:%s:%s", p.Node.Name(), node, module, function)
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
func (p *Process) Send(to interface{}, message etf.Term) error {
	if !p.IsAlive() {
		return ErrProcessTerminated
	}
	p.Node.registrar.route(p.self, to, message)
	return nil
}

// SendAfter starts a timer. When the timer expires, the message sends to the process identified by 'to'.
// 'to' can be a Pid, registered local name or a tuple {RegisteredName, NodeName}.
// Returns cancel function in order to discard sending a message
func (p *Process) SendAfter(to interface{}, message etf.Term, after time.Duration) context.CancelFunc {
	//TODO: should we control the number of timers/goroutines have been created this way?
	ctx, cancel := context.WithCancel(p.Context)
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
				p.Node.registrar.route(p.self, to, message)
			}
		}
	}()
	return cancel
}

// CastAfter simple wrapper for SendAfter to send '$gen_cast' message
func (p *Process) CastAfter(to interface{}, message etf.Term, after time.Duration) context.CancelFunc {
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), message})
	return p.SendAfter(to, msg, after)
}

// Cast sends a message in fashion of 'gen_cast'.
// 'to' can be a Pid, registered local name
// or a tuple {RegisteredName, NodeName}
func (p *Process) Cast(to interface{}, message etf.Term) error {
	if !p.IsAlive() {
		return ErrProcessTerminated
	}
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), message})
	p.Node.registrar.route(p.self, to, msg)
	return nil
}

// MonitorProcess creates monitor between the processes.
// 'process' value can be: etf.Pid, registered local name etf.Atom or
// remote registered name etf.Tuple{Name etf.Atom, Node etf.Atom}
// When a process monitor is triggered, a 'DOWN' message sends that has the following
// pattern: {'DOWN', MonitorRef, Type, Object, Info} where
// Info has following values:
//  - the exit reason of the process
//  - 'noproc' (process did not exist at the time of monitor creation)
//  - 'noconnection' (no connection to the node where the monitored process resides)
// Note: The monitor request is an asynchronous signal. That is, it takes time before the signal reaches its destination.
func (p *Process) MonitorProcess(process interface{}) etf.Ref {
	ref := p.Node.MakeRef()
	p.Node.monitor.MonitorProcessWithRef(p.self, process, ref)
	return ref
}

// Link creates a link between the calling process and another process
func (p *Process) Link(with etf.Pid) {
	p.Node.monitor.Link(p.self, with)
}

// Unlink removes the link, if there is one, between the calling process and the process referred to by Pid.
func (p *Process) Unlink(with etf.Pid) {
	p.Node.monitor.Unlink(p.self, with)
}

// MonitorNode creates monitor between the current process and node. If Node fails or does not exist,
// the message {nodedown, Node} is delivered to the process.
func (p *Process) MonitorNode(name string) etf.Ref {
	return p.Node.monitor.MonitorNode(p.self, name)
}

// DemonitorProcess removes monitor. Returns false if the given reference 'ref' wasn't found
func (p *Process) DemonitorProcess(ref etf.Ref) bool {
	return p.Node.monitor.DemonitorProcess(ref)
}

// DemonitorNode removes monitor
func (p *Process) DemonitorNode(ref etf.Ref) {
	p.Node.monitor.DemonitorNode(ref)
}

// CreateAlias creates a new alias for the Process
func (p *Process) CreateAlias() (etf.Alias, error) {
	return p.Node.registrar.createNewAlias(p)
}

// DeleteAlias deletes the given alias
func (p *Process) DeleteAlias(alias etf.Alias) error {
	return p.Node.registrar.deleteAlias(p, alias)
}

// ListEnv returns map of configured environment variables.
// Process' environment is also inherited from environment variables
// of groupLeader (if its started as a child of Application/Supervisor)
func (p *Process) ListEnv() map[string]interface{} {
	var env map[string]interface{}
	if p.groupLeader == nil {
		env = make(map[string]interface{})
	} else {
		env = p.groupLeader.ListEnv()
	}

	p.RLock()
	defer p.RUnlock()
	for key, value := range p.env {
		env[key] = value
	}
	return env
}

// SetEnv set environment variable with given name
func (p *Process) SetEnv(name string, value interface{}) {
	p.Lock()
	defer p.Unlock()
	if p.env == nil {
		p.env = make(map[string]interface{})
	}
	p.env[name] = value
}

// GetEnv returns value associated with given environment name.
func (p *Process) GetEnv(name string) interface{} {
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
func (p *Process) Wait() {
	<-p.stopped
}

// WaitWithTimeout waits until process stopped. Return ErrTimeout
// if given timeout is exceeded
func (p *Process) WaitWithTimeout(d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return ErrTimeout
	case <-p.stopped:
		// 'stopped' channel is closing after unregistering this process
		// and releasing the name related to this process.
		// we should wait here otherwise you wont be able to
		// start another process with the same name
		return nil
	}
}

// IsAlive returns whether the process is alive
func (p *Process) IsAlive() bool {
	return p.Context.Err() == nil
}

// GetChildren returns list of children pid (Application, Supervisor)
func (p *Process) GetChildren() []etf.Pid {
	c, err := p.directRequest("$getChildren", nil, 5)
	if err == nil {
		return c.([]etf.Pid)
	}
	return []etf.Pid{}
}

// SetTrapExit enables/disables the trap on terminate process
func (p *Process) SetTrapExit(trap bool) {
	p.trapExit = trap
}

// GetTrapExit returns whether the trap was enabled on this process
func (p *Process) GetTrapExit() bool {
	return p.trapExit
}

// GetObject returns object this process runs on.
func (p *Process) GetObject() interface{} {
	return p.object
}

// Direct make a direct request to the actor (Application, Supervisor, GenServer or inherited from GenServer actor) with default timeout 5 seconds
func (p *Process) Direct(request interface{}) (interface{}, error) {
	return p.directRequest("", request, DefaultCallTimeout)
}

// DirectWithTimeout make a direct request to the actor with the given timeout (in seconds)
func (p *Process) DirectWithTimeout(request interface{}, timeout int) (interface{}, error) {
	if timeout < 1 {
		timeout = 5
	}
	return p.directRequest("", request, timeout)
}

// RemoteSpawnOptions defines options for RemoteSpawn method
type RemoteSpawnOptions struct {
	// RegisterName
	RegisterName string
	// Monitor enables monitor on the spawned process using provided reference
	Monitor etf.Ref
	// Link enables link between the calling and spawned processes
	Link bool
	// Function in order to support {M,F,A} request to the Erlang node
	Function string
	// Timeout
	Timeout int
}

func (p *Process) RemoteSpawn(node string, object string, opts RemoteSpawnOptions, args ...etf.Term) (etf.Pid, error) {
	ref := p.Node.MakeRef()
	optlist := etf.List{}
	if opts.RegisterName != "" {
		optlist = append(optlist, etf.Tuple{etf.Atom("name"), etf.Atom(opts.RegisterName)})

	}
	if opts.Timeout == 0 {
		opts.Timeout = DefaultCallTimeout
	}
	control := etf.Tuple{distProtoSPAWN_REQUEST, ref, p.self, p.self,
		// {M,F,A}
		etf.Tuple{etf.Atom(object), etf.Atom(opts.Function), len(args)},
		optlist,
	}
	p.sendSyncRequestRaw(ref, etf.Atom(node), append([]etf.Term{control}, args)...)
	reply, err := p.waitSyncReply(ref, opts.Timeout)
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
			p.Node.monitor.MonitorProcessWithRef(p.self, r, opts.Monitor)
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

func (p *Process) directRequest(id string, request interface{}, timeout int) (interface{}, error) {
	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)

	direct := directMessage{
		id:      id,
		message: request,
		reply:   make(chan directMessage),
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
	case response := <-direct.reply:
		if response.err != nil {
			return nil, response.err
		}

		return response.message, nil
	case <-timer.C:
		return nil, ErrTimeout
	}
}

func (p *Process) sendSyncRequestRaw(ref etf.Ref, node etf.Atom, messages ...etf.Term) {
	reply := make(chan etf.Term, 2)
	p.replyMutex.Lock()
	defer p.replyMutex.Unlock()
	p.reply[ref] = reply
	p.Node.registrar.routeRaw(node, messages...)
}
func (p *Process) sendSyncRequest(ref etf.Ref, to interface{}, message etf.Term) {
	reply := make(chan etf.Term, 2)
	p.replyMutex.Lock()
	defer p.replyMutex.Unlock()
	p.reply[ref] = reply
	p.Send(to, message)
}

func (p *Process) putSyncReply(ref etf.Ref, reply etf.Term) {
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

func (p *Process) waitSyncReply(ref etf.Ref, timeout int) (etf.Term, error) {
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
		case <-p.Context.Done():
			return nil, ErrProcessTerminated
		}
	}

}
