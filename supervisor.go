package ergo

import (
	"fmt"
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
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
	SupervisorStrategyOneForOne = SupervisorStrategyType("one_for_one")

	// SupervisorStrategyOneForAll If one child process terminates and is to be restarted, all other
	// child processes are terminated and then all child processes are restarted.
	SupervisorStrategyOneForAll = SupervisorStrategyType("one_for_all")

	// SupervisorStrategyRestForOne If one child process terminates and is to be restarted,
	// the 'rest' of the child processes (that is, the child
	// processes after the terminated child process in the start order)
	// are terminated. Then the terminated child process and all
	// child processes after it are restarted
	SupervisorStrategyRestForOne = SupervisorStrategyType("rest_for_one")

	// SupervisorStrategySimpleOneForOne A simplified one_for_one supervisor, where all
	// child processes are dynamically added instances
	// of the same process type, that is, running the same code.
	SupervisorStrategySimpleOneForOne = SupervisorStrategyType("simple_one_for_one")

	// Restart types:

	// SupervisorChildRestartPermanent child process is always restarted
	SupervisorChildRestartPermanent = SupervisorChildRestart("permanent")

	// SupervisorChildRestartTemporary child process is never restarted
	// (not even when the supervisor restart strategy is rest_for_one
	// or one_for_all and a sibling death causes the temporary process
	// to be terminated)
	SupervisorChildRestartTemporary = SupervisorChildRestart("temporary")

	// SupervisorChildRestartTransient child process is restarted only if
	// it terminates abnormally, that is, with an exit reason other
	// than normal, shutdown, or {shutdown,Term}.
	SupervisorChildRestartTransient = SupervisorChildRestart("transient")

	supervisorChildStateStart    = 0
	supervisorChildStateRunning  = 1
	supervisorChildStateDisabled = -1

	// shutdown defines how a child process must be terminated. (TODO: not implemented yet)

	// SupervisorChildShutdownBrutal means that the child process is
	// unconditionally terminated using process' Kill method
	SupervisorChildShutdownBrutal = -1

	// SupervisorChildShutdownInfinity means that the supervisor will
	// wait for an exit signal as long as child takes
	SupervisorChildShutdownInfinity = 0 // default shutdown behavior

	// SupervisorChildShutdownTimeout5sec predefined timeout value
	SupervisorChildShutdownTimeout5sec = 5
)

type supervisorChildState int

// SupervisorChildShutdown is an integer time-out value means that the supervisor tells
// the child process to terminate by calling Stop method and then
// wait for an exit signal with reason shutdown back from the
// child process. If no exit signal is received within the
// specified number of seconds, the child process is unconditionally
// terminated using Kill method.
// There are predefined values:
//   SupervisorChildShutdownBrutal (-1)
//   SupervisorChildShutdownInfinity (0) - default value
//   SupervisorChildShutdownTimeout5sec (5)
type SupervisorChildShutdown int

// SupervisorBehavior interface
type SupervisorBehavior interface {
	ProcessBehavior
	Init(args ...etf.Term) SupervisorSpec
}

type SupervisorSpec struct {
	Name     string
	Children []SupervisorChildSpec
	Strategy SupervisorStrategy
	restarts []int64
}

type SupervisorChildSpec struct {
	// Node to run child on remote node
	Node     string
	Name     string
	Child    ProcessBehavior
	Args     []etf.Term
	Restart  SupervisorChildRestart
	Shutdown SupervisorChildShutdown
	state    supervisorChildState // for internal usage
	process  *Process
}

// Supervisor is implementation of ProcessBehavior interface
type Supervisor struct {
	spec *SupervisorSpec
}

func (sv *Supervisor) Loop(svp *Process, args ...etf.Term) string {
	object := svp.object
	spec := object.(SupervisorBehavior).Init(args...)
	lib.Log("Supervisor spec %#v\n", spec)
	svp.ready <- nil

	sv.spec = &spec

	if spec.Strategy.Type != SupervisorStrategySimpleOneForOne {
		startChildren(svp, &spec)
	}

	svp.SetTrapExit(true)
	svp.currentFunction = "Supervisor:loop"
	waitTerminatingProcesses := []etf.Pid{}

	for {
		var message etf.Term
		var fromPid etf.Pid
		select {
		case ex := <-svp.gracefulExit:
			for i := range spec.Children {
				p := spec.Children[i].process
				if p == nil {
					continue
				}
				if p.IsAlive() {
					// in order to get rid of race condition when Node goes down
					// via node.Stop() cancaling the node's context which
					// triggering all the processes to kill themselves
					p.Exit(svp.Self(), ex.reason)
				}
			}
			return ex.reason

		case msg := <-svp.mailBox:
			fromPid = msg.from
			message = msg.message

		case <-svp.Context.Done():
			return "kill"
		case direct := <-svp.direct:
			switch direct.id {
			case "getChildren":
				children := []etf.Pid{}
				for i := range sv.spec.Children {
					if sv.spec.Children[i].process == nil {
						continue
					}
					children = append(children, sv.spec.Children[i].process.self)
				}

				direct.message = children
				direct.err = nil
				direct.reply <- direct

			default:
				direct.message = nil
				direct.err = ErrUnsupportedRequest
				direct.reply <- direct
			}
			continue
		}

		svp.reductions++

		lib.Log("[%#v]. Message from %#v\n", svp.self, fromPid)

		switch m := message.(type) {

		case etf.Tuple:

			switch m.Element(1) {

			case etf.Atom("EXIT"):
				terminated := m.Element(2).(etf.Pid)
				reason := m.Element(3).(etf.Atom)
				itWasChild := false
				// We should make sure if it was real call for exit.
				// 'EXIT' message shouldn't be sent by the child of this supervisor
				for i := range spec.Children {
					child := spec.Children[i].process
					if child == nil {
						continue
					}
					if child.Self() == terminated {
						itWasChild = true
						break
					}
				}
				if !itWasChild && reason != etf.Atom("restart") {
					// so we should proceed it as a graceful exit request and
					// terminate this Application process (if all children will
					// be stopped correctly)
					go func() {
						ex := gracefulExitRequest{
							from:   terminated,
							reason: string(reason),
						}
						svp.gracefulExit <- ex
					}()
					continue
				}
				if len(waitTerminatingProcesses) > 0 {

					for i := range waitTerminatingProcesses {
						if waitTerminatingProcesses[i] == terminated {
							waitTerminatingProcesses[i] = waitTerminatingProcesses[0]
							waitTerminatingProcesses = waitTerminatingProcesses[1:]
							break
						}
					}

					if len(waitTerminatingProcesses) == 0 {
						// it was the last one. lets restart all terminated children
						startChildren(svp, &spec)
					}

					continue
				}

				switch spec.Strategy.Type {

				case SupervisorStrategyOneForAll:
					for i := range spec.Children {
						if spec.Children[i].state != supervisorChildStateRunning {
							continue
						}

						p := spec.Children[i].process
						if p == nil {
							continue
						}

						spec.Children[i].process = nil
						if p.Self() == terminated {
							if haveToDisableChild(spec.Children[i].Restart, reason) {
								spec.Children[i].state = supervisorChildStateDisabled
							} else {
								spec.Children[i].state = supervisorChildStateStart
							}

							if len(spec.Children) == i+1 && len(waitTerminatingProcesses) == 0 {
								// it was the last one. nothing to waiting for
								startChildren(svp, &spec)
							}
							continue
						}

						if haveToDisableChild(spec.Children[i].Restart, "restart") {
							spec.Children[i].state = supervisorChildStateDisabled
						} else {
							spec.Children[i].state = supervisorChildStateStart
						}
						p.Exit(p.Self(), "restart")

						waitTerminatingProcesses = append(waitTerminatingProcesses, p.Self())
					}

				case SupervisorStrategyRestForOne:
					isRest := false
					for i := range spec.Children {
						p := spec.Children[i].process
						if p == nil {
							continue
						}
						if p.Self() == terminated {
							isRest = true
							spec.Children[i].process = nil
							if haveToDisableChild(spec.Children[i].Restart, reason) {
								spec.Children[i].state = supervisorChildStateDisabled
							} else {
								spec.Children[i].state = supervisorChildStateStart
							}

							if len(spec.Children) == i+1 && len(waitTerminatingProcesses) == 0 {
								// it was the last one. nothing to waiting for
								startChildren(svp, &spec)
							}

							continue
						}

						if isRest && spec.Children[i].state == supervisorChildStateRunning {
							p.Exit(p.Self(), "restart")
							spec.Children[i].process = nil
							waitTerminatingProcesses = append(waitTerminatingProcesses, p.Self())
							if haveToDisableChild(spec.Children[i].Restart, "restart") {
								spec.Children[i].state = supervisorChildStateDisabled
							} else {
								spec.Children[i].state = supervisorChildStateStart
							}
						}
					}

				case SupervisorStrategyOneForOne:
					for i := range spec.Children {
						p := spec.Children[i].process
						if p == nil {
							continue
						}
						if p.Self() == terminated {
							spec.Children[i].process = nil
							if haveToDisableChild(spec.Children[i].Restart, reason) {
								spec.Children[i].state = supervisorChildStateDisabled
							} else {
								spec.Children[i].state = supervisorChildStateStart
							}

							startChildren(svp, &spec)
							break
						}
					}

				case SupervisorStrategySimpleOneForOne:
					for i := range spec.Children {
						p := spec.Children[i].process
						if p == nil {
							continue
						}
						if p.Self() == terminated {

							if haveToDisableChild(spec.Children[i].Restart, reason) {
								// wont be restarted due to restart strategy
								spec.Children[i] = spec.Children[0]
								spec.Children = spec.Children[1:]
								break
							}

							process := startChild(svp, spec.Children[i].Name, spec.Children[i].Child, spec.Children[i].Args...)
							spec.Children[i].process = process
							break
						}
					}
				}

			case etf.Atom("$startByName"):
				// dynamically start child process
				specName := m.Element(2).(string)
				args := m.Element(3)
				reply := m.Element(4).(chan etf.Tuple)

				s := lookupSpecByName(specName, spec.Children)
				if s == nil {
					reply <- etf.Tuple{etf.Atom("error"), "unknown_spec"}
					continue
				}
				specChild := *s
				specChild.process = nil
				specChild.state = supervisorChildStateStart

				m := etf.Tuple{
					etf.Atom("$startBySpec"),
					specChild,
					args,
					reply,
				}
				svp.mailBox <- mailboxMessage{etf.Pid{}, m}

			case etf.Atom("$startBySpec"):
				specChild := m.Element(2).(SupervisorChildSpec)
				args := m.Element(3).([]etf.Term)
				reply := m.Element(4).(chan etf.Tuple)

				if len(args) > 0 {
					specChild.Args = args
				}

				process := startChild(svp, "", specChild.Child, specChild.Args...)
				specChild.process = process
				specChild.Name = ""
				spec.Children = append(spec.Children, specChild)

				reply <- etf.Tuple{etf.Atom("ok"), process.self}
			default:
				lib.Log("m: %#v", m)
			}

		default:
			lib.Log("m: %#v", m)
		}
	}
}

// StartChild dynamically starts a child process with given name of child spec which is defined by Init call.
// Created process will use the same object (GenServer/Supervisor) you have defined in spec as a Child since it
// keeps pointer. You might use this object as a shared among the process you will create using this spec.
func (sv *Supervisor) StartChild(parent *Process, specName string, args ...etf.Term) (etf.Pid, error) {
	reply := make(chan etf.Tuple)
	m := etf.Tuple{
		etf.Atom("$startByName"),
		specName,
		args,
		reply,
	}
	parent.mailBox <- mailboxMessage{etf.Pid{}, m}
	r := <-reply
	switch r.Element(1) {
	case etf.Atom("ok"):
		return r.Element(2).(etf.Pid), nil
	case etf.Atom("error"):
		return etf.Pid{}, fmt.Errorf("%s", r.Element(2).(string))
	default:
		panic("internal error at Supervisor.StartChild")
	}
}

// StartChildWithSpec dynamically starts a child process with given child spec
func (sv *Supervisor) StartChildWithSpec(parent *Process, spec SupervisorChildSpec, args ...etf.Term) (etf.Pid, error) {
	reply := make(chan etf.Tuple)
	m := etf.Tuple{
		etf.Atom("$startBySpec"),
		spec,
		args,
		reply,
	}
	parent.mailBox <- mailboxMessage{etf.Pid{}, m}
	r := <-reply
	switch r.Element(1) {
	case etf.Atom("ok"):
		return r.Element(2).(etf.Pid), nil
	default:
		return etf.Pid{}, fmt.Errorf(r.Element(1).(string))
	}
}

func startChildren(parent *Process, spec *SupervisorSpec) {
	spec.restarts = append(spec.restarts, time.Now().Unix())
	if len(spec.restarts) > int(spec.Strategy.Intensity) {
		period := time.Now().Unix() - spec.restarts[0]
		if period <= int64(spec.Strategy.Period) {
			fmt.Printf("ERROR: Restart intensity is exceeded (%d restarts for %d seconds)\n",
				spec.Strategy.Intensity, spec.Strategy.Period)
			parent.Kill()
			return
		}
		spec.restarts = spec.restarts[1:]
	}

	for i := range spec.Children {
		switch spec.Children[i].state {
		case supervisorChildStateDisabled:
			spec.Children[i].process = nil
		case supervisorChildStateRunning:
			continue
		case supervisorChildStateStart:
			spec.Children[i].state = supervisorChildStateRunning
			process := startChild(parent, spec.Children[i].Name, spec.Children[i].Child, spec.Children[i].Args...)
			spec.Children[i].process = process
		default:
			panic("Incorrect supervisorChildState")
		}
	}
}

func startChild(parent *Process, name string, child ProcessBehavior, args ...etf.Term) *Process {
	opts := ProcessOptions{}

	if parent.groupLeader == nil {
		// leader is not set
		opts.GroupLeader = parent
	} else {
		opts.GroupLeader = parent.groupLeader
	}
	opts.Parent = parent
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
		if spec[i].Name == specName {
			return &spec[i]
		}
	}
	return nil
}
