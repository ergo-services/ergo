package ergonode

import (
	"context"

	"github.com/halturin/ergonode/etf"
)

type ProcessType = string

const (
	DefaultProcessMailboxSize = 100
	// Process type
	ProcessGenServer  = "gen_server"
	ProcessSupervisor = "supervisor"
)

type Process struct {
	local   chan etf.Term
	remote  chan etf.Tuple
	ready   chan bool
	self    etf.Pid
	context context.Context
	Stop    ProcessStopFunc
	name    string
	Node    *Node

	object interface{}
	state  interface{}
}

type ProcessStopFunc func(reason string)

// Behaviour interface contains methods you should implement to make own process behaviour
type ProcessBehaviour interface {
	loop(Process, interface{}, ...interface{}) // method which implements control flow of process
}

// Self returns self Pid
func (p *Process) Self() etf.Pid {
	return p.self
}

// Send making outgoing message
func (p *Process) Send(to interface{}, message etf.Term) {
	p.Node.registrar.route(p.self, to, message)
}

func (p *Process) MonitorProcess(to etf.Pid) etf.Ref {
	return p.Node.monitor.MonitorProcess(p.self, to)
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
