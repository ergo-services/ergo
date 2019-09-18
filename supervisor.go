package ergonode

import (
	"errors"
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

	SupervisorChildStateStart    = 0
	SupervisorChildStateRunning  = 1
	SupervisorChildStateDisabled = -1
)

type SupervisorChildState int

// SupervisorBehavior interface
type SupervisorBehavior interface {
	Init(args ...interface{}) SupervisorSpec
}

type SupervisorSpec struct {
	children []SupervisorChildSpec
	strategy SupervisorStrategy
}

type SupervisorChildSpec struct {
	name    string
	child   interface{}
	args    []interface{}
	restart SupervisorChildRestart
	state   SupervisorChildState // for internal usage
}

// Supervisor is implementation of ProcessBehavior interface
type Supervisor struct{}

func (sv *Supervisor) loop(p *Process, object interface{}, args ...interface{}) string {
	var dynamicChildren map[etf.Pid]SupervisorChildSpec

	spec := object.(SupervisorBehavior).Init(args...)
	lib.Log("Supervisor spec %#v\n", spec)
	p.ready <- true

	if spec.strategy.Type != SupervisorStrategySimpleOneForOne {
		p.children = make([]*Process, len(spec.children))
		sv.startChildren(p, spec.children[:])
	} else {
		dynamicChildren = make(map[etf.Pid]SupervisorChildSpec)
	}

	stop := make(chan string, 2)
	p.currentFunction = "Supervisor:loop"
	waitTerminatingProcesses := []etf.Pid{}

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
			return "shutdown"
		}

		p.reductions++

		lib.Log("[%#v]. Message from %#v\n", p.self, fromPid)

		switch m := message.(type) {

		case etf.Tuple:

			switch m.Element(1) {

			case etf.Atom("EXIT"):

				terminated := m.Element(2).(etf.Pid)
				reason := m.Element(3).(etf.Atom)

				fmt.Println("CHILD TERMINATED:", terminated, "with reason:", reason)

				if len(waitTerminatingProcesses) > 0 {

					for i := range waitTerminatingProcesses {
						if waitTerminatingProcesses[i] == terminated {
							waitTerminatingProcesses[0] = waitTerminatingProcesses[i]
							waitTerminatingProcesses = waitTerminatingProcesses[1:]
						}
					}

					if len(waitTerminatingProcesses) == 0 {
						// it was the last one. lets restart all terminated children
						sv.startChildren(p, spec.children[:])
					}

					continue
				}

				switch spec.strategy.Type {

				case SupervisorStrategyOneForAll:
					for i := range p.children {
						if p.children[i].self == terminated {
							if haveToDisableChild(spec.children[i].restart, reason) {
								spec.children[i].state = SupervisorChildStateDisabled
							} else {
								spec.children[i].state = SupervisorChildStateStart
							}

							continue
						}
						p.children[i].Stop("shutdown")
						waitTerminatingProcesses = append(waitTerminatingProcesses, p.children[i].self)
					}

				case SupervisorStrategyRestForOne:
					isRest := false
					for i := range p.children {
						if p.children[i].self == terminated {
							isRest = true
							if haveToDisableChild(spec.children[i].restart, reason) {
								spec.children[i].state = SupervisorChildStateDisabled
							} else {
								spec.children[i].state = SupervisorChildStateStart
							}
							continue
						}

						if isRest {
							p.children[i].Stop("shutdown")
							waitTerminatingProcesses = append(waitTerminatingProcesses, p.children[i].self)
							spec.children[i].state = SupervisorChildStateStart
						}
					}

				case SupervisorStrategyOneForOne:
					for i := range p.children {
						if p.children[i].self == terminated {
							if haveToDisableChild(spec.children[i].restart, reason) {
								spec.children[i].state = SupervisorChildStateDisabled
							} else {
								spec.children[i].state = SupervisorChildStateStart
							}

							sv.startChildren(p, spec.children[:])
							break
						}
					}
				case SupervisorStrategySimpleOneForOne:
					for i := range p.children {
						if p.children[i].self == terminated {
							// remove child from list
							p.children[0] = p.children[i]
							p.children = p.children[1:]

							if s, ok := dynamicChildren[terminated]; ok {
								delete(dynamicChildren, terminated)

								if haveToDisableChild(s.restart, reason) {
									// wont be restarted due to restart strategy
									break
								}

								sv.StartChild(*p, s.name, s.args)
							}
							break
						}
					}
				}
			case etf.Atom("$startByName"):
				var s *SupervisorChildSpec
				// dynamically start child process
				specName := m.Element(2).(string)
				args := m.Element(3)
				reply := m.Element(4).(chan etf.Tuple)

				s = lookupSpecByName(specName, spec.children[:])
				if s == nil {
					reply <- etf.Tuple{etf.Atom("error"), "unknown_spec"}
				}

				m := etf.Tuple{
					etf.Atom("$startBySpec"),
					*s,
					args,
					reply,
				}
				p.mailBox <- etf.Tuple{etf.Pid{}, m}

			case etf.Atom("$startBySpec"):
				spec := m.Element(2).(SupervisorChildSpec)
				args := m.Element(3).([]interface{})
				reply := m.Element(4).(chan etf.Tuple)

				process := startChild(p, "", spec.child, args)
				p.children = append(p.children, process)
				spec.args = args
				dynamicChildren[process.self] = spec

				reply <- etf.Tuple{etf.Atom("ok"), process.self}
			default:
				lib.Log("m: %#v", m)
			}

		default:
			lib.Log("m: %#v", m)
		}
	}
}

// StartChlid dynamically starts a child process with given name of child spec
// which is defined by Init call.
func (sv *Supervisor) StartChild(parent Process, specName string, args ...interface{}) (etf.Pid, error) {
	reply := make(chan etf.Tuple)
	m := etf.Tuple{
		etf.Atom("$startByName"),
		specName,
		args,
		reply,
	}
	parent.mailBox <- etf.Tuple{etf.Pid{}, m}
	r := <-reply
	switch r.Element(0) {
	case etf.Atom("ok"):
		return r.Element(1).(etf.Pid), nil
	default:
		return etf.Pid{}, errors.New(r.Element(1).(string))
	}
}

// StartChlidWithSpec dynamically starts a child process with given child spec
func (sv *Supervisor) StartChildWithSpec(parent Process, spec SupervisorChildSpec, args ...interface{}) (etf.Pid, error) {
	reply := make(chan etf.Tuple)
	m := etf.Tuple{
		etf.Atom("$startBySpec"),
		spec,
		args,
		reply,
	}
	parent.mailBox <- etf.Tuple{etf.Pid{}, m}
	r := <-reply
	switch r.Element(0) {
	case etf.Atom("ok"):
		return r.Element(1).(etf.Pid), nil
	default:
		return etf.Pid{}, errors.New(r.Element(1).(string))
	}
}

func (sv *Supervisor) startChildren(parent *Process, specs []SupervisorChildSpec) {

	for i := range specs {
		if specs[i].state != SupervisorChildStateStart {
			// its already running or has been disabled due to restart strategy
			continue
		}

		specs[i].state = SupervisorChildStateRunning
		process := startChild(parent, specs[i].name, specs[i].child, specs[i].args)
		parent.children[i] = process
	}
}

func startChild(parent *Process, name string, child interface{}, args ...interface{}) *Process {
	opts := ProcessOptions{}
	emptyPid := etf.Pid{}

	if parent.groupLeader == emptyPid {
		// leader is not set
		opts.GroupLeader = parent.self
	} else {
		opts.GroupLeader = parent.groupLeader
	}
	opts.parent = parent
	process, err := parent.Node.Spawn(name, opts, child, args...)

	if err != nil {
		panic(err)
	}

	process.parent = parent
	parent.Link(process.self)

	return process
}

func haveToDisableChild(restart SupervisorChildRestart, reason etf.Atom) bool {
	switch restart {
	case SupervisorChildRestartTransient:
		if reason == etf.Atom("shutdown") || reason == etf.Atom("normal") {
			return true
		}

	case SupervisorChildRestartTemporary:
		return true
	}

	return false
}

func lookupSpecByName(specName string, spec []SupervisorChildSpec) *SupervisorChildSpec {
	for i := range spec {
		if spec[i].name == specName {
			return &spec[i]
		}
	}
	return nil
}
