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

func (p *Process) Call() (error, etf.Term) {

	return nil, nil
}

func (p *Process) Cast() {

}

func (p *Process) Send() {

}
