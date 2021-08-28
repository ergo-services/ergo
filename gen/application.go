package gen

// http://erlang.org/doc/apps/kernel/application.html

import (
	"fmt"
	"sync"
	"time"

	"github.com/halturin/ergo/etf"
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
)

// ApplicationBehavior interface
type ApplicationBehavior interface {
	ProcessBehavior
	Load(args ...etf.Term) (ApplicationSpec, error)
	Start(process Process, args ...etf.Term)
}

type ApplicationSpec struct {
	sync.Mutex
	Name         string
	Description  string
	Version      string
	Lifespan     time.Duration
	Applications []string
	Environment  map[string]interface{}
	Children     []ApplicationChildSpec
	Process      Process
	StartType    ApplicationStartType
}

type ApplicationChildSpec struct {
	Child   ProcessBehavior
	Name    string
	Args    []etf.Term
	process Process
}

// Application is implementation of ProcessBehavior interface
type Application struct{}

type ApplicationInfo struct {
	Name        string
	Description string
	Version     string
	PID         etf.Pid
}

func (a *Application) ProcessInit(p Process, args ...etf.Term) (ProcessState, error) {

	spec, ok := p.GetEnv("spec").(*ApplicationSpec)
	if !ok {
		return ProcessState{}, fmt.Errorf("ProcessInit: not an ApplicationBehavior")
	}
	p.SetTrapExit(true)

	if spec.Environment != nil {
		for k, v := range spec.Environment {
			p.SetEnv(k, v)
		}
	}

	if !a.startChildren(p, spec.Children[:]) {
		a.stopChildren(p.Self(), spec.Children[:], "failed")
		return ProcessState{}, fmt.Errorf("failed")
	}

	behavior, ok := p.GetProcessBehavior().(ApplicationBehavior)
	if !ok {
		return ProcessState{}, fmt.Errorf("ProcessInit: not an ApplicationBehavior")
	}
	behavior.Start(p, args...)

	return ProcessState{
		Process: p,
		State:   spec,
	}, nil
}

func (a *Application) ProcessLoop(ps ProcessState) string {

	spec := ps.State.(*ApplicationSpec)
	defer func() { spec.Process = nil }()

	if spec.Lifespan == 0 {
		spec.Lifespan = time.Hour * 24 * 365 * 100 // let's define default lifespan 100 years :)
	}

	chs := ps.GetProcessChannels()

	// to prevent of timer leaks due to its not GCed until the timer fires
	timer := time.NewTimer(spec.Lifespan)
	defer timer.Stop()

	for {
		var message etf.Term

		select {
		case ex := <-chs.GracefulExit:
			childrenStopped := a.stopChildren(ex.From, spec.Children, string(ex.Reason))
			if !childrenStopped {
				fmt.Printf("Warining: application can't be stopped. Some of the children are still running")
				continue
			}
			return ex.Reason

		case direct := <-chs.Direct:
			switch direct.ID {
			case "$getChildren":
				pids := []etf.Pid{}
				for i := range spec.Children {
					if spec.Children[i].process == nil {
						continue
					}
					pids = append(pids, spec.Children[i].process.Self())
				}

				direct.Message = pids
				direct.Err = nil
				direct.Reply <- direct

			default:
				direct.Message = nil
				direct.Err = ErrUnsupportedRequest
				direct.Reply <- direct
			}
			continue

		case <-ps.Context().Done():
			// node is down or killed using p.Kill()
			return "kill"
		case <-timer.C:
			// time to die
			go ps.Exit("normal")
			continue
		case msg := <-chs.Mailbox:
			message = msg.Message
		}

		//fromPid := msg.Element(1).(etf.Pid)
		switch r := message.(type) {
		case etf.Tuple:
			// waiting for {'EXIT', Pid, Reason}
			if len(r) != 3 || r.Element(1) != etf.Atom("EXIT") {
				// unknown. ignoring
				continue
			}
			terminated := r.Element(2).(etf.Pid)
			terminatedName := terminated.String()
			reason := r.Element(3).(etf.Atom)
			alienPid := true

			for i := range spec.Children {
				child := spec.Children[i].process
				if child == nil {
					continue
				}
				if child.Self() == terminated {
					terminatedName = child.Name()
					alienPid = false
					break
				}
			}

			// Application process has trapExit = true it means
			// the calling Exit methond will never send a message into
			// the gracefulExit channel, it will send an 'EXIT' message.
			// Checking bellow whether the terminated process was found
			// among the our children.
			if alienPid {
				// so we should proceed it as a graceful exit request and
				// terminate this Application process (if all children will
				// be stopped correctly)
				go func() {
					//ex := gracefulExitRequest{
					//	from:   terminated,
					//	reason: string(reason),
					//}
					//p.gracefulExit <- ex
				}()
				continue
			}

			switch spec.StartType {
			case ApplicationStartPermanent:
				a.stopChildren(terminated, spec.Children, string(reason))
				fmt.Printf("Application child %s (at %s) stopped with reason %s (permanent: node is shutting down)\n",
					terminatedName, ps.NodeName(), reason)
				//FIXME
				//ps.NodeStop()
				return "shutdown"

			case ApplicationStartTransient:
				if reason == etf.Atom("normal") || reason == etf.Atom("shutdown") {
					fmt.Printf("Application child %s (at %s) stopped with reason %s (transient)\n",
						terminatedName, ps.NodeName(), reason)
					continue
				}
				a.stopChildren(terminated, spec.Children, "normal")
				fmt.Printf("Application child %s (at %s) stopped with reason %s. (transient: node is shutting down)\n",
					terminatedName, ps.NodeName(), reason)
				//FIXME
				//p.Node.Stop()
				return string(reason)

			case ApplicationStartTemporary:
				fmt.Printf("Application child %s (at %s) stopped with reason %s (temporary)\n",
					terminatedName, ps.NodeName(), reason)
			}

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

		p := children[i].process
		if p == nil {
			continue
		}
		if !p.IsAlive() {
			continue
		}

		p.Exit(reason)
		if err := p.WaitWithTimeout(5 * time.Second); err != nil {
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
		p := startChild(parent, children[i].Name, children[i].Child, children[i].Args...)
		if p == nil {
			return false
		}
		children[i].process = p
	}
	return true
}
