package ergonode

import (
	"context"

	"github.com/halturin/ergonode/etf"
)

type Process struct {
	local   chan etf.Term
	remote  chan etf.Tuple
	ready   chan bool
	self    etf.Pid
	context context.Context
	stop    context.CancelFunc
	name    string
}

const (
	DefaultProcessMailboxSize = 100
)

// Behaviour interface contains methods you should implement to make own process behaviour
type ProcessBehaviour interface {
	ProcessLoop(Process, interface{}, ...interface{}) // method which implements control flow of process
	Options() map[string]interface{}                  // method returns process-related options
}
