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
)

type Process struct {
	sync.RWMutex

	mailBox      chan etf.Tuple
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

	object interface{}
	state  interface{}
	reply  chan etf.Tuple

	env map[string]interface{}

	parent          *Process
	reductions      uint64 // we use this term to count total number of processed messages from mailBox
	currentFunction string

	trapExit bool
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
	Dictionary      etf.Map
	TrapExit        bool
	GroupLeader     etf.Pid
	Reductions      uint64
}

type ProcessOptions struct {
	MailboxSize uint16
	GroupLeader *Process
	parent      *Process
}

// ProcessExitFunc initiate a graceful stopping process
type ProcessExitFunc func(from etf.Pid, reason string)

// ProcessBehaviour interface contains methods you should implement to make own process behaviour
type ProcessBehaviour interface {
	Loop(*Process, ...interface{}) string // method which implements control flow of process
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
		gl = p.groupLeader.Self()
	}
	links := p.Node.monitor.GetLinks(p.self)
	monitors := p.Node.monitor.GetMonitors(p.self)
	monitoredBy := p.Node.monitor.GetMonitoredBy(p.self)
	return ProcessInfo{
		PID:             p.self,
		Name:            p.name,
		CurrentFunction: p.currentFunction,
		GroupLeader:     gl,
		Links:           links,
		Monitors:        monitors,
		MonitoredBy:     monitoredBy,
		Status:          "running",
		MessageQueueLen: len(p.mailBox),
		TrapExit:        p.trapExit,
		Reductions:      p.reductions,
	}
}

// Call makes outgoing sync request in fashion of 'gen_call'.
// 'to' can be Pid, registered local name or a tuple {RegisteredName, NodeName}
func (p *Process) Call(to interface{}, message etf.Term) (etf.Term, error) {
	return p.CallWithTimeout(to, message, DefaultCallTimeout)
}

// CallWithTimeout makes outgoing sync request in fashiod of 'gen_call' with given timeout
func (p *Process) CallWithTimeout(to interface{}, message etf.Term, timeout int) (etf.Term, error) {
	var timer *time.Timer

	ref := p.Node.MakeRef()
	from := etf.Tuple{p.self, ref}
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_call"), from, message})
	p.Send(to, msg)

	timer = lib.TakeTimer()
	defer lib.ReleaseTimer(timer)
	timer.Reset(time.Second * time.Duration(timeout))

	for {
		select {
		case m := <-p.reply:
			ref1 := m[0].(etf.Ref)
			val := m[1].(etf.Term)
			// check message Ref
			if len(ref.ID) == 3 && ref.ID[0] == ref1.ID[0] && ref.ID[1] == ref1.ID[1] && ref.ID[2] == ref1.ID[2] {
				return val, nil
			}
			// ignore this message. waiting for the next one
		case <-timer.C:
			return nil, fmt.Errorf("timeout")
		case <-p.Context.Done():
			return nil, fmt.Errorf("stopped")
		}
	}
}

// CallRPC evaluate rpc call with given node/MFA
func (p *Process) CallRPC(node, module, function string, args ...etf.Term) (etf.Term, error) {
	return p.CallRPCWithTimeout(DefaultCallTimeout, node, module, function, args...)
}

// CallRPCWithTimeout evaluate rpc call with given node/MFA and timeout
func (p *Process) CallRPCWithTimeout(timeout int, node, module, function string, args ...etf.Term) (etf.Term, error) {
	lib.Log("[%s] RPC calling: %s:%s:%s", p.Node.FullName, node, module, function)
	message := etf.Tuple{
		etf.Atom("call"),
		etf.Atom(module),
		etf.Atom(function),
		etf.List(args),
	}
	to := etf.Tuple{etf.Atom("rex"), etf.Atom(node)}
	return p.CallWithTimeout(to, message, timeout)
}

// CastRPC evaluate rpc cast with given node/MFA
func (p *Process) CastRPC(node, module, function string, args ...etf.Term) {
	lib.Log("[%s] RPC casting: %s:%s:%s", p.Node.FullName, node, module, function)
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
func (p *Process) Send(to interface{}, message etf.Term) {
	p.Node.registrar.route(p.self, to, message)
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

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			p.Node.registrar.route(p.self, to, message)
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
func (p *Process) Cast(to interface{}, message etf.Term) {
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), message})
	p.Node.registrar.route(p.self, to, msg)
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
	return p.Node.monitor.MonitorProcess(p.self, process)
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
	c, err := p.directRequest("getChildren", nil)
	if err == nil {
		return c.([]etf.Pid)
	}
	return []etf.Pid{}
}

// GetState returns string representation of the process state (GenServer)
func (p *Process) GetState() string {
	p.RLock()
	defer p.RUnlock()
	return fmt.Sprintf("%#v", p.state)
}

// SetTrapExit enables/disables the trap on terminate process
func (p *Process) SetTrapExit(trap bool) {
	p.trapExit = trap
}

// GetTrapExit returns whether the trap was enabled on this process
func (p *Process) GetTrapExit() bool {
	return p.trapExit
}

func (p *Process) directRequest(id string, request interface{}) (interface{}, error) {
	reply := make(chan directMessage)
	t := time.Second * time.Duration(5)
	m := directMessage{
		id:      id,
		message: request,
		reply:   reply,
	}
	timer := time.NewTimer(t)
	defer timer.Stop()
	// sending request
	select {
	case p.direct <- m:
		timer.Reset(t)
	case <-timer.C:
		return nil, ErrProcessBusy
	}

	// receiving response
	select {
	case response := <-reply:
		if response.err != nil {
			return nil, response.err
		}

		return response.message, nil
	case <-timer.C:
		return nil, ErrTimeout
	}
}
