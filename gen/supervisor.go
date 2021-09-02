package gen

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
	Restart   SupervisorStrategyRestart
}

type SupervisorStrategyType = string
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

// SupervisorBehavior interface
type SupervisorBehavior interface {
	ProcessBehavior
	Init(args ...etf.Term) (SupervisorSpec, error)
}

type SupervisorSpec struct {
	Name     string
	Children []SupervisorChildSpec
	Strategy SupervisorStrategy
	restarts []int64
}

type SupervisorChildSpec struct {
	// Node to run child on remote node
	Node    string
	Name    string
	Child   ProcessBehavior
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

func (sv *Supervisor) ProcessInit(p Process, args ...etf.Term) (ProcessState, error) {
	behavior, ok := p.GetProcessBehavior().(SupervisorBehavior)
	if !ok {
		return ProcessState{}, fmt.Errorf("ProcessInit: not a SupervisorBehavior")
	}
	spec, err := behavior.Init(args...)
	if err != nil {
		return ProcessState{}, err
	}
	lib.Log("Supervisor spec %#v\n", spec)

	p.SetTrapExit(true)
	return ProcessState{
		Process: p,
		State:   &spec,
	}, nil
}

func (sv *Supervisor) ProcessLoop(ps ProcessState, started chan<- bool) string {
	spec := ps.State.(*SupervisorSpec)
	if spec.Strategy.Type != SupervisorStrategySimpleOneForOne {
		startChildren(ps, spec)
	}

	waitTerminatingProcesses := []etf.Pid{}
	chs := ps.GetProcessChannels()

	started <- true
	for {
		fmt.Println("WAITING FOR", waitTerminatingProcesses)
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
			if err != nil {
				direct.Message = nil
				direct.Err = err
				direct.Reply <- direct
				continue
			}
			direct.Message = value
			direct.Err = nil
			direct.Reply <- direct

		case <-chs.Mailbox:
			// do nothing
		}
	}
}

// StartChild dynamically starts a child process with given name of child spec which is defined by Init call.
func (sv *Supervisor) StartChild(supervisor Process, name string, args ...etf.Term) (etf.Pid, error) {
	message := messageStartChild{
		name: name,
		args: args,
	}
	value, err := supervisor.Direct(message)
	if err != nil {
		return etf.Pid{}, err
	}
	pid, ok := value.(etf.Pid)
	if !ok {
		return etf.Pid{}, fmt.Errorf("internal error: can't start child %#v", value)
	}
	return pid, nil
}

func startChildren(supervisor Process, spec *SupervisorSpec) {
	spec.restarts = append(spec.restarts, time.Now().Unix())
	if len(spec.restarts) > int(spec.Strategy.Intensity) {
		period := time.Now().Unix() - spec.restarts[0]
		if period <= int64(spec.Strategy.Period) {
			fmt.Printf("ERROR: Restart intensity is exceeded (%d restarts for %d seconds)\n",
				spec.Strategy.Intensity, spec.Strategy.Period)
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
			process := startChild(supervisor, spec.Children[i].Name, spec.Children[i].Child, spec.Children[i].Args...)
			spec.Children[i].process = process
		default:
			panic("Incorrect supervisorChildState")
		}
	}
}

func startChild(supervisor Process, name string, child ProcessBehavior, args ...etf.Term) Process {
	opts := ProcessOptions{}

	if leader := supervisor.GetGroupLeader(); leader != nil {
		opts.GroupLeader = leader
	} else {
		// leader is not set
		opts.GroupLeader = supervisor
	}
	process, err := supervisor.Spawn(name, opts, child, args...)

	if err != nil {
		panic(err)
	}

	supervisor.Link(process.Self())

	return process
}

func handleDirect(supervisor Process, spec *SupervisorSpec, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case MessageDirectGetChildren:
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
		process := startChild(supervisor, childSpec.Name, childSpec.Child, childSpec.Args...)
		childSpec.process = process
		spec.Children = append(spec.Children, childSpec)
		return process.Self(), nil

	default:
	}

	return nil, ErrUnsupportedRequest
}

func handleMessageExit(p Process, exit ProcessGracefulExitRequest, spec *SupervisorSpec, wait []etf.Pid) []etf.Pid {

	terminated := exit.From
	reason := exit.Reason
	fmt.Println("SUPERVISOR GOT EXIT from ", terminated)

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

	fmt.Println("EXIT REASON", reason, wait)
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
			fmt.Println("ADDDD WAIT", child.Self, wait)
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

				process := startChild(p, spec.Children[i].Name, spec.Children[i].Child, spec.Children[i].Args...)
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
