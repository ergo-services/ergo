package ergonode

import (
	"fmt"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type SupervisorStrategy struct {
	Type      SupervisorStrategyType
	Intensity uint16
	Period    uint16
}

type SupervisorStrategyType = string
type SupervisorChildRestart = string
type SupervisorChild = string

const (
	// Restart strategies:

	// SupervisorRestartIntensity
	SupervisorRestartIntensity = uint16(10)

	// SupervisorRestartPeriod
	SupervisorRestartPeriod = uint16(10)

	// SupervisorStrategyOneForOne If one child process terminates and is to be restarted, only
	// that child process is affected. This is the default restart strategy.
	SupervisorStrategyOneForOne = "one_for_one"

	// SupervisorStrategyOneForAll If one child process terminates and is to be restarted, all other
	// child processes are terminated and then all child processes are restarted.
	SupervisorStrategyOneForAll = "one_for_all"

	// SupervisorStrategyRestForOne If one child process terminates and is to be restarted,
	// the 'rest' of the child processes (that is, the child
	// processes after the terminated child process in the start order)
	// are terminated. Then the terminated child process and all
	// child processes after it are restarted
	SupervisorStrategyRestForOne = "rest_for_one"

	// SupervisorStrategySimpleOneForOne A simplified one_for_one supervisor, where all
	// child processes are dynamically added instances
	// of the same process type, that is, running the same code.
	SupervisorStrategySimpleOneForOne = "simple_one_for_one"

	// Restart types:

	// SupervisorChildRestartPermanent child process is always restarted
	SupervisorChildRestartPermanent = "permanent"

	// SupervisorChildRestartTemporary child process is never restarted
	// (not even when the supervisor restart strategy is rest_for_one
	// or one_for_all and a sibling death causes the temporary process
	// to be terminated)
	SupervisorChildRestartTemporary = "temporary"

	// SupervisorChildRestartTransient child process is restarted only if
	// it terminates abnormally, that is, with an exit reason other
	// than normal, shutdown, or {shutdown,Term}.
	SupervisorChildRestartTransient = "transient"
)

// SupervisorBehavior interface
type SupervisorBehavior interface {
	Init(process *Process, args ...interface{}) SupervisorSpec
}

type SupervisorSpec struct {
	children []SupervisorChildSpec
	strategy SupervisorStrategy
}

type SupervisorChildSpec struct {
	name    string
	child   ProcessBehaviour
	args    []interface{}
	restart SupervisorChildRestart
}

// Supervisor is implementation of ProcessBehavior interface
type Supervisor struct{}

func (sv *Supervisor) loop(p *Process, object interface{}, args ...interface{}) string {
	spec := object.(SupervisorBehavior).Init(p, args...)
	lib.Log("Supervisor spec %#v\n", spec)
	p.ready <- true

	children := sv.InitChildren(p, spec.children)
	fmt.Println("CHILDREN", children)
	stop := make(chan string, 2)

	for {
		var message etf.Term
		var fromPid etf.Pid

		select {
		case reason := <-stop:
			return reason
		case msg := <-p.mailBox:
			fromPid = msg.Element(1).(etf.Pid)
			message = msg.Element(2)
		case <-p.Context.Done():
			return "immediate"
		}

		lib.Log("[%#v]. Message from %#v\n", p.self, fromPid)
		switch m := message.(type) {
		case etf.Tuple:
			switch m.Element(1) {
			case etf.Atom("EXIT"):
				terminated := m.Element(2).(etf.Pid)
				fmt.Println("CHILDREN TERMINATED", terminated)
				switch spec.strategy.Type {
				case SupervisorStrategyOneForAll:

				case SupervisorStrategyRestForOne:

				default: // SupervisorStrategySimpleOneForOne, SupervisorStrategyOneForOne

				}

			default:
				lib.Log("m: %#v", m)
			}

		default:
			lib.Log("m: %#v", m)
		}
	}
}

func (sv *Supervisor) InitChildren(parent *Process, specs []SupervisorChildSpec) []Process {
	children := make([]Process, len(specs))
	for i := range specs {
		spec := specs[i]
		opts := ProcessOptions{}
		emptyPid := etf.Pid{}
		if parent.groupLeader == emptyPid {
			// leader is not set
			opts.GroupLeader = parent.self
		} else {
			opts.GroupLeader = parent.groupLeader
		}
		process := parent.Node.Spawn(spec.name, opts, spec.child, spec.args...)
		parent.Link(process.self)
		children[i] = process
	}

	return children
}
