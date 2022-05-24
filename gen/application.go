package gen

// http://erlang.org/doc/apps/kernel/application.html

import (
	"fmt"
	"sync"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

type ApplicationStartType = string

const (
	// start types:

	// ApplicationStartPermanent If a permanent application terminates,
	// all other applications and the runtime system (node) are also terminated.
	ApplicationStartPermanent = "permanent"

	// ApplicationStartTemporary If a temporary application terminates,
	// this is reported but no other applications are terminated.
	ApplicationStartTemporary = "temporary"

	// ApplicationStartTransient If a transient application terminates
	// with reason normal, this is reported but no other applications are
	// terminated. If a transient application terminates abnormally, that
	// is with any other reason than normal, all other applications and
	// the runtime system (node) are also terminated.
	ApplicationStartTransient = "transient"

	// EnvKeySpec
	EnvKeySpec EnvKey = "ergo:AppSpec"
)

// ApplicationBehavior interface
type ApplicationBehavior interface {
	ProcessBehavior
	Load(args ...etf.Term) (ApplicationSpec, error)
	Start(process Process, args ...etf.Term)
}

// ApplicationSpec
type ApplicationSpec struct {
	sync.Mutex
	Name         string
	Description  string
	Version      string
	Lifespan     time.Duration
	Applications []string
	Env          map[EnvKey]interface{}
	Children     []ApplicationChildSpec
	Process      Process
	StartType    ApplicationStartType
}

// ApplicationChildSpec
type ApplicationChildSpec struct {
	Child   ProcessBehavior
	Options ProcessOptions
	Name    string
	Args    []etf.Term
	process Process
}

// Application is implementation of ProcessBehavior interface
type Application struct{}

// ApplicationInfo
type ApplicationInfo struct {
	Name        string
	Description string
	Version     string
	PID         etf.Pid
}

// ProcessInit
func (a *Application) ProcessInit(p Process, args ...etf.Term) (ProcessState, error) {
	spec, ok := p.Env(EnvKeySpec).(*ApplicationSpec)
	if !ok {
		return ProcessState{}, fmt.Errorf("ProcessInit: not an ApplicationBehavior")
	}
	// remove variable from the env
	p.SetEnv(EnvKeySpec, nil)

	p.SetTrapExit(true)

	if spec.Env != nil {
		for k, v := range spec.Env {
			p.SetEnv(k, v)
		}
	}

	if !a.startChildren(p, spec.Children[:]) {
		a.stopChildren(p.Self(), spec.Children[:], "failed")
		return ProcessState{}, fmt.Errorf("failed")
	}

	behavior, ok := p.Behavior().(ApplicationBehavior)
	if !ok {
		return ProcessState{}, fmt.Errorf("ProcessInit: not an ApplicationBehavior")
	}
	behavior.Start(p, args...)
	spec.Process = p

	return ProcessState{
		Process: p,
		State:   spec,
	}, nil
}

// ProcessLoop
func (a *Application) ProcessLoop(ps ProcessState, started chan<- bool) string {
	spec := ps.State.(*ApplicationSpec)
	defer func() { spec.Process = nil }()

	if spec.Lifespan == 0 {
		spec.Lifespan = time.Hour * 24 * 365 * 100 // let's define default lifespan 100 years :)
	}

	chs := ps.ProcessChannels()

	timer := time.NewTimer(spec.Lifespan)
	// timer must be stopped explicitly to prevent of timer leaks
	// due to its not GCed until the timer fires
	defer timer.Stop()

	started <- true
	for {
		select {
		case ex := <-chs.GracefulExit:
			terminated := ex.From
			reason := ex.Reason
			if ex.From == ps.Self() {
				childrenStopped := a.stopChildren(terminated, spec.Children, reason)
				if !childrenStopped {
					lib.Warning("application %q can't be stopped. Some of the children are still running", spec.Name)
					continue
				}
				return ex.Reason
			}

			unknownChild := true

			for i := range spec.Children {
				child := spec.Children[i].process
				if child == nil {
					continue
				}
				if child.Self() == terminated {
					unknownChild = false
					break
				}
			}

			if unknownChild {
				continue
			}

			switch spec.StartType {
			case ApplicationStartPermanent:
				a.stopChildren(terminated, spec.Children, string(reason))
				lib.Warning("Application child %s (at %s) stopped with reason %s (permanent: node is shutting down)",
					terminated, ps.NodeName(), reason)
				ps.NodeStop()
				return "shutdown"

			case ApplicationStartTransient:
				if reason == "normal" || reason == "shutdown" {
					lib.Warning("Application child %s (at %s) stopped with reason %s (transient)",
						terminated, ps.NodeName(), reason)
					continue
				}
				a.stopChildren(terminated, spec.Children, reason)
				lib.Warning("Application child %s (at %s) stopped with reason %s. (transient: node is shutting down)",
					terminated, ps.NodeName(), reason)
				ps.NodeStop()
				return string(reason)

			case ApplicationStartTemporary:
				fmt.Printf("Application child %s (at %s) stopped with reason %s (temporary)\n",
					terminated, ps.NodeName(), reason)
			}

		case direct := <-chs.Direct:
			switch direct.Message.(type) {
			case MessageDirectChildren:
				pids := []etf.Pid{}
				for i := range spec.Children {
					if spec.Children[i].process == nil {
						continue
					}
					pids = append(pids, spec.Children[i].process.Self())
				}

				ps.PutSyncReply(direct.Ref, pids, nil)

			default:
				ps.PutSyncReply(direct.Ref, nil, ErrUnsupportedRequest)
			}

		case <-ps.Context().Done():
			// node is down or killed using p.Kill()
			return "kill"

		case <-timer.C:
			// time to die
			ps.SetTrapExit(false)
			go ps.Exit("normal")

		case <-chs.Mailbox:
			// do nothing
		}

	}
}

func (a *Application) stopChildren(from etf.Pid, children []ApplicationChildSpec, reason string) bool {
	childrenStopped := true
	for i := range children {
		child := children[i].process
		if child == nil {
			continue
		}

		if child.Self() == from {
			continue
		}

		if !child.IsAlive() {
			continue
		}

		if err := child.Exit(reason); err != nil {
			childrenStopped = false
			continue
		}

		if err := child.WaitWithTimeout(5 * time.Second); err != nil {
			childrenStopped = false
			continue
		}

		children[i].process = nil
	}

	return childrenStopped
}

func (a *Application) startChildren(parent Process, children []ApplicationChildSpec) bool {
	for i := range children {
		// i know, it looks weird to use the funcion from supervisor file.
		// will move it to somewhere else, but let it be there for a while.
		p := startChild(parent, children[i].Name, children[i].Child, children[i].Options, children[i].Args...)
		if p == nil {
			return false
		}
		children[i].process = p
	}
	return true
}
