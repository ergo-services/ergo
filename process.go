package ergonode

import (
	"context"
	"errors"
	"time"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type ProcessType = string

const (
	DefaultProcessMailboxSize = 100
	// Process type
	ProcessGenServer  = "gen_server"
	ProcessSupervisor = "supervisor"
)

type Process struct {
	mailBox      chan etf.Tuple
	ready        chan bool
	gracefulExit chan gracefulExitRequest
	self         etf.Pid
	groupLeader  etf.Pid
	Context      context.Context
	Kill         context.CancelFunc
	Exit         ProcessExitFunc
	name         string
	Node         *Node

	object interface{}
	state  interface{}
	reply  chan etf.Tuple

	children        []*Process
	parent          *Process
	reductions      uint64 // we use this term to count total number of processed messages from mailBox
	currentFunction string

	trapExit bool
}

type gracefulExitRequest struct {
	from   etf.Pid
	reason string
}
type ProcessInfo struct {
	CurrentFunction string
	Status          string
	MessageQueueLen int
	Links           []etf.Pid
	Dictionary      etf.Map
	TrapExit        bool
	GroupLeader     etf.Pid
	Reductions      uint64
}

type ProcessOptions struct {
	MailboxSize uint16
	GroupLeader etf.Pid
	parent      *Process
}

type ProcessExitFunc func(from etf.Pid, reason string)

// Behaviour interface contains methods you should implement to make own process behaviour
type ProcessBehaviour interface {
	loop(*Process, interface{}, ...interface{}) string // method which implements control flow of process
}

// Self returns self Pid
func (p *Process) Self() etf.Pid {
	return p.self
}

func (p *Process) Call(to interface{}, message etf.Term) (etf.Term, error) {
	return p.CallWithTimeout(to, message, DefaultCallTimeout)
}

func (p *Process) CallWithTimeout(to interface{}, message etf.Term, timeout int) (etf.Term, error) {
	ref := p.Node.MakeRef()
	from := etf.Tuple{p.self, ref}
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_call"), from, message})
	p.Send(to, msg)
	for {
		select {
		case m := <-p.reply:
			ref1 := m[0].(etf.Ref)
			val := m[1].(etf.Term)
			// check message Ref
			if len(ref.Id) == 3 && ref.Id[0] == ref1.Id[0] && ref.Id[1] == ref1.Id[1] && ref.Id[2] == ref1.Id[2] {
				return val, nil
			}
			// ignore this message. waiting for the next one
		case <-time.After(time.Second * time.Duration(timeout)):
			return nil, errors.New("timeout")
		case <-p.Context.Done():
			return nil, errors.New("stopped")
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

// CallRPC evaluate rpc cast with given node/MFA
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

// Send making outgoing message
func (p *Process) Send(to interface{}, message etf.Term) {
	p.Node.registrar.route(p.self, to, message)
}

func (p *Process) Cast(to interface{}, message etf.Term) {
	msg := etf.Term(etf.Tuple{etf.Atom("$gen_cast"), message})
	p.Node.registrar.route(p.self, to, msg)
}

func (p *Process) MonitorProcess(to etf.Pid) etf.Ref {
	return p.Node.monitor.MonitorProcess(p.self, to)
}

func (p *Process) Link(with etf.Pid) {
	p.Node.monitor.Link(p.self, with)
}

func (p *Process) Unlink(with etf.Pid) {
	p.Node.monitor.Unink(p.self, with)
}

func (p *Process) MonitorNode(name string) etf.Ref {
	return p.Node.monitor.MonitorNode(p.self, name)
}

func (p *Process) DemonitorProcess(ref etf.Ref) {
	p.Node.monitor.DemonitorProcess(ref)
}

func (p *Process) DemonitorNode(ref etf.Ref) {
	p.Node.monitor.DemonitorNode(ref)
}
