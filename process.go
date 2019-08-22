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
	Stop    context.CancelFunc
	name    string
	Node    *Node

	processType ProcessType
	object      interface{}
	state       interface{}
}

// Behaviour interface contains methods you should implement to make own process behaviour
type ProcessBehaviour interface {
	loop(*Process, interface{}, ...interface{}) // method which implements control flow of process
}
