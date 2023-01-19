package gen

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

// SupervisorBehavior interface
type SupervisorBehavior interface {
	ProcessBehavior
	Init(args ...etf.Term) (SupervisorSpec, error)
}

// SupervisorStrategy
type SupervisorStrategy struct {
	Type      SupervisorStrategyType
	Intensity uint16
	Period    uint16
	Restart   SupervisorStrategyRestart
}

// SupervisorStrategyType
type SupervisorStrategyType = string

// SupervisorStrategyRestart
type SupervisorStrategyRestart = string

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

	// SupervisorStrategyRestartPermanent child process is always restarted
	SupervisorStrategyRestartPermanent = SupervisorStrategyRestart("permanent")

	// SupervisorStrategyRestartTemporary child process is never restarted
	// (not even when the supervisor restart strategy is rest_for_one
	// or one_for_all and a sibling death causes the temporary process
	// to be terminated)
	SupervisorStrategyRestartTemporary = SupervisorStrategyRestart("temporary")

	// SupervisorStrategyRestartTransient child process is restarted only if
	// it terminates abnormally, that is, with an exit reason other
	// than normal, shutdown.
	SupervisorStrategyRestartTransient = SupervisorStrategyRestart("transient")

	supervisorChildStateStart    = 0
	supervisorChildStateRunning  = 1
	supervisorChildStateDisabled = -1
)

type supervisorChildState int

// SupervisorSpec
type SupervisorSpec struct {
	Name     string
	Children []SupervisorChildSpec
	Strategy SupervisorStrategy
	restarts []int64
}

// SupervisorChildSpec
type SupervisorChildSpec struct {
	Name    string
	Child   ProcessBehavior
	Options ProcessOptions
	Args    []etf.Term

	state   supervisorChildState // for internal usage
	process Process
}

// Supervisor is implementation of ProcessBehavior interface
type Supervisor struct{}

type messageStartChild struct {
	name string
	args []etf.Term
}

type messageStartChildBySpec struct {
	childSpec SupervisorChildSpec
}

type messageStopChildByName struct {
	processName string
}

// ProcessInit
func (sv *Supervisor) ProcessInit(p Process, args ...etf.Term) (ProcessState, error) {
	behavior, ok := p.Behavior().(SupervisorBehavior)
	if !ok {
		return ProcessState{}, fmt.Errorf("ProcessInit: not a SupervisorBehavior")
	}
	spec, err := behavior.Init(args...)
	if err != nil {
		return ProcessState{}, err
	}
	lib.Log("[%s] SUPERVISOR %q with restart strategy: %s[%s] ", p.NodeName(), p.Name(), spec.Strategy.Type, spec.Strategy.Restart)

	p.SetTrapExit(true)
	return ProcessState{
		Process: p,
		State:   &spec,
	}, nil
}

// ProcessLoop
func (sv *Supervisor) ProcessLoop(ps ProcessState, started chan<- bool) string {
	spec := ps.State.(*SupervisorSpec)
	if spec.Strategy.Type != SupervisorStrategySimpleOneForOne {
		startChildren(ps, spec)
	}

	waitTerminatingProcesses := []etf.Pid{}
	chs := ps.ProcessChannels()

	started <- true
	for {
		select {
		case ex := <-chs.GracefulExit:
			if ex.From == ps.Self() {
				// stop supervisor gracefully
				for i := range spec.Children {
					p := spec.Children[i].process
					if p != nil && p.IsAlive() {
						p.Exit(ex.Reason)
					}
				}
				return ex.Reason
			}
			waitTerminatingProcesses = handleMessageExit(ps, ex, spec, waitTerminatingProcesses)

		case <-ps.Context().Done():
			return "kill"

		case direct := <-chs.Direct:
			value, err := handleDirect(ps, spec, direct.Message)
			ps.PutSyncReply(direct.Ref, value, err)

		case <-chs.Mailbox:
			// do nothing
		}
	}
}

// StartChild dynamically starts a child process with given name of child spec which is defined by Init call.
func (sv *Supervisor) StartChild(supervisor Process, name string, args ...etf.Term) (Process, error) {
	message := messageStartChild{
		name: name,
		args: args,
	}
	value, err := supervisor.Direct(message)
	if err != nil {
		return nil, err
	}
	process, ok := value.(Process)
	if !ok {
		return nil, fmt.Errorf("internal error: can't start child %#v", value)
	}
	return process, nil
}

// StartChildBySpec dynamically adds child spec and starts a child process with given spec which is undefined by Init call.
// not surport simple_one_for_one strategy type.
// Similar to erlang[supervisor:start_child/2].
func StartChildBySpec(supervisor Process, spec SupervisorChildSpec) (Process, error) {
	if spec.state != supervisorChildStateStart {
		return nil, fmt.Errorf("spec state is invalid %#v", spec.state)
	}
	message := messageStartChildBySpec{
		childSpec: spec,
	}
	value, err := supervisor.Direct(message)
	if err != nil {
		return nil, err
	}
	process, ok := value.(Process)
	if !ok {
		return nil, fmt.Errorf("internal error: can't start child %#v", value)
	}
	return process, nil
}

// StopChildByName dynamically deletes child spec and stop a child process with given process name.
// not surport simple_one_for_one strategy type.
// Similar to erlang[supervisor:terminate_child(Sup, ID), supervisor:delete_child(Sup, ID)].
func StopChildByName(supervisor Process, processName string) (SupervisorChildSpec, error) {
	message := messageStopChildByName{
		processName: processName,
	}
	value, err := supervisor.Direct(message)
	if err != nil {
		return SupervisorChildSpec{}, err
	}
	childSpec, ok := value.(SupervisorChildSpec)
	if !ok {
		return SupervisorChildSpec{}, fmt.Errorf("internal error:%#v", value)
	}
	return childSpec, nil
}

func startChildren(supervisor Process, spec *SupervisorSpec) {
	spec.restarts = append(spec.restarts, time.Now().Unix())
	if len(spec.restarts) > int(spec.Strategy.Intensity) {
		period := time.Now().Unix() - spec.restarts[0]
		if period <= int64(spec.Strategy.Period) {
			lib.Warning("Supervisor %q. Restart intensity is exceeded (%d restarts for %d seconds)",
				spec.Name, spec.Strategy.Intensity, spec.Strategy.Period)
			supervisor.Kill()
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
			process := startChild(supervisor, spec.Children[i].Name, spec.Children[i].Child, spec.Children[i].Options, spec.Children[i].Args...)
			spec.Children[i].process = process
		default:
			panic("Incorrect supervisorChildState")
		}
	}
}

func startChild(supervisor Process, name string, child ProcessBehavior, opts ProcessOptions, args ...etf.Term) Process {

	opts.GroupLeader = supervisor
	if leader := supervisor.GroupLeader(); leader != nil {
		opts.GroupLeader = leader
	}

	// Child process shouldn't ignore supervisor termination (via TrapExit).
	// Using the supervisor's Context makes the child terminate if the supervisor is terminated.
	opts.Context = supervisor.Context()

	process, err := supervisor.Spawn(name, opts, child, args...)

	if err != nil {
		panic(err)
	}

	supervisor.Link(process.Self())

	return process
}

func handleDirect(supervisor Process, spec *SupervisorSpec, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case MessageDirectChildren:
		children := []etf.Pid{}
		for i := range spec.Children {
			if spec.Children[i].process == nil {
				continue
			}
			children = append(children, spec.Children[i].process.Self())
		}

		return children, nil
	case messageStartChild:
		childSpec, err := lookupSpecByName(m.name, spec.Children)
		if err != nil {
			return nil, err
		}
		childSpec.state = supervisorChildStateStart
		if len(m.args) > 0 {
			childSpec.Args = m.args
		}
		// Dinamically started child can't be registered with a name.
		childSpec.Name = ""
		process := startChild(supervisor, childSpec.Name, childSpec.Child, childSpec.Options, childSpec.Args...)
		childSpec.process = process
		spec.Children = append(spec.Children, childSpec)
		return process, nil

	case messageStartChildBySpec:
		if spec.Strategy.Type == SupervisorStrategySimpleOneForOne {
			return nil, fmt.Errorf("startSpecChild not surport simple_one_for_one")
		}
		childSpec := m.childSpec
		_, err := lookupSpecByName(childSpec.Name, spec.Children)
		if err == nil {
			return nil, fmt.Errorf("%s specChild is exist", childSpec.Name)
		}
		childSpec.state = supervisorChildStateRunning
		process := startChild(supervisor, childSpec.Name, childSpec.Child, childSpec.Options, childSpec.Args...)
		childSpec.process = process
		spec.Children = append(spec.Children, childSpec)
		return process, nil

	case messageStopChildByName:
		if spec.Strategy.Type == SupervisorStrategySimpleOneForOne {
			return nil, fmt.Errorf("startSpecChild not surport simple_one_for_one")
		}
		processName := m.processName
		childSpec, err := lookupSpecByName(processName, spec.Children)
		if err != nil {
			return nil, err
		}
		childProc := childSpec.process
		if childSpec.state != supervisorChildStateRunning && childProc != nil {
			return nil, fmt.Errorf("childSpec:%v can not stop", childSpec)
		}
		//delete spec
		supervisor.Unlink(childProc.Self())
		spec.Children = deleteSpecByName(processName, spec.Children)
		//terminate child
		//Similar to erlang[supervisor:shutdown/1](exit(Pid, shutdown)).
		childProc.Exit("shutdown")
		return childSpec, nil

	default:
	}

	return nil, lib.ErrUnsupportedRequest
}

func handleMessageExit(p Process, exit ProcessGracefulExitRequest, spec *SupervisorSpec, wait []etf.Pid) []etf.Pid {

	terminated := exit.From
	reason := exit.Reason

	isChild := false
	// We should make sure if it was an exit message from the supervisor's child
	for i := range spec.Children {
		child := spec.Children[i].process
		if child == nil {
			continue
		}
		if child.Self() == terminated {
			isChild = true
			break
		}
	}

	if !isChild && reason != "restart" {
		return wait
	}

	if len(wait) > 0 {
		for i := range wait {
			if wait[i] == terminated {
				wait[i] = wait[0]
				wait = wait[1:]
				break
			}
		}

		if len(wait) == 0 {
			// it was the last one. lets restart all terminated children
			// which hasn't supervisorChildStateDisabled state
			startChildren(p, spec)
		}

		return wait
	}

	switch spec.Strategy.Type {

	case SupervisorStrategyOneForAll:
		for i := range spec.Children {
			if spec.Children[i].state != supervisorChildStateRunning {
				continue
			}

			child := spec.Children[i].process
			if child == nil {
				continue
			}

			spec.Children[i].process = nil
			if child.Self() == terminated {
				if haveToDisableChild(spec.Strategy.Restart, reason) {
					spec.Children[i].state = supervisorChildStateDisabled
				} else {
					spec.Children[i].state = supervisorChildStateStart
				}

				if len(spec.Children) == i+1 && len(wait) == 0 {
					// it was the last one. nothing to waiting for
					startChildren(p, spec)
				}
				continue
			}

			if haveToDisableChild(spec.Strategy.Restart, "restart") {
				spec.Children[i].state = supervisorChildStateDisabled
			} else {
				spec.Children[i].state = supervisorChildStateStart
			}
			child.Exit("restart")

			wait = append(wait, child.Self())
		}

	case SupervisorStrategyRestForOne:
		isRest := false
		for i := range spec.Children {
			child := spec.Children[i].process
			if child == nil {
				continue
			}
			if child.Self() == terminated {
				isRest = true
				spec.Children[i].process = nil
				if haveToDisableChild(spec.Strategy.Restart, reason) {
					spec.Children[i].state = supervisorChildStateDisabled
				} else {
					spec.Children[i].state = supervisorChildStateStart
				}

				if len(spec.Children) == i+1 && len(wait) == 0 {
					// it was the last one. nothing to waiting for
					startChildren(p, spec)
				}

				continue
			}

			if isRest && spec.Children[i].state == supervisorChildStateRunning {
				child.Exit("restart")
				spec.Children[i].process = nil
				wait = append(wait, child.Self())
				if haveToDisableChild(spec.Strategy.Restart, "restart") {
					spec.Children[i].state = supervisorChildStateDisabled
				} else {
					spec.Children[i].state = supervisorChildStateStart
				}
			}
		}

	case SupervisorStrategyOneForOne:
		for i := range spec.Children {
			child := spec.Children[i].process
			if child == nil {
				continue
			}
			if child.Self() == terminated {
				spec.Children[i].process = nil
				if haveToDisableChild(spec.Strategy.Restart, reason) {
					spec.Children[i].state = supervisorChildStateDisabled
				} else {
					spec.Children[i].state = supervisorChildStateStart
				}

				startChildren(p, spec)
				break
			}
		}

	case SupervisorStrategySimpleOneForOne:
		for i := range spec.Children {
			child := spec.Children[i].process
			if child == nil {
				continue
			}
			if child.Self() == terminated {

				if haveToDisableChild(spec.Strategy.Restart, reason) {
					// wont be restarted due to restart strategy
					spec.Children[i] = spec.Children[0]
					spec.Children = spec.Children[1:]
					break
				}

				process := startChild(p, spec.Children[i].Name, spec.Children[i].Child, spec.Children[i].Options, spec.Children[i].Args...)
				spec.Children[i].process = process
				break
			}
		}
	}
	return wait
}

func haveToDisableChild(strategy SupervisorStrategyRestart, reason string) bool {
	switch strategy {
	case SupervisorStrategyRestartTransient:
		if reason == "shutdown" || reason == "normal" {
			return true
		}

	case SupervisorStrategyRestartTemporary:
		return true
	}

	return false
}

func lookupSpecByName(specName string, spec []SupervisorChildSpec) (SupervisorChildSpec, error) {
	for i := range spec {
		if spec[i].Name == specName {
			return spec[i], nil
		}
	}
	return SupervisorChildSpec{}, fmt.Errorf("unknown child")
}

func deleteSpecByName(specName string, spec []SupervisorChildSpec) []SupervisorChildSpec {
	ret := make([]SupervisorChildSpec, 0, len(spec))
	for i := range spec {
		if spec[i].Name != specName {
			ret = append(ret, spec[i])
		}
	}
	return ret
}
