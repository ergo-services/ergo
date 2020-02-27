package ergonode

// http://erlang.org/doc/apps/kernel/application.html

import (
	"fmt"
	"sync"
	"time"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
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
	Load(args ...interface{}) (ApplicationSpec, error)
	Start(process *Process, args ...interface{})
}

type ApplicationSpec struct {
	Name         string
	Description  string
	Version      string
	Lifespan     time.Duration
	Applications []string
	Environment  map[string]interface{}
	// Depends		[]
	Children  []ApplicationChildSpec
	startType ApplicationStartType
	app       ApplicationBehavior
	process   *Process
	mutex     sync.Mutex
}

type ApplicationChildSpec struct {
	Child   interface{}
	Name    string
	Args    []interface{}
	process *Process
}

// Application is implementation of ProcessBehavior interface
type Application struct{}

type ApplicationInfo struct {
	Name        string
	Description string
	Version     string
	PID         etf.Pid
}

func (a *Application) loop(p *Process, object interface{}, args ...interface{}) string {
	// some internal agreement that the first argument should be a spec of this application
	// (see ApplicatoinStart for the details)
	spec := args[0].(*ApplicationSpec)

	if spec.Environment != nil {
		for k, v := range spec.Environment {
			p.SetEnv(k, v)
		}
	}

	if !a.startChildren(p, spec.Children[:]) {
		a.stopChildren(p.Self(), spec.Children[:], "failed")
		return "failed"
	}

	p.currentFunction = "Application:Start"

	object.(ApplicationBehavior).Start(p, args[1:]...)
	lib.Log("Application spec %#v\n", spec)
	p.ready <- true

	p.currentFunction = "Application:loop"

	if spec.Lifespan == 0 {
		spec.Lifespan = time.Second * 31536000 * 100 // let's define default lifespan 100 years :)
	}

	// to prevent of timer leaks due to its not GCed until the timer fires
	timer := time.NewTimer(spec.Lifespan)
	defer timer.Stop()

	for {
		select {
		case ex := <-p.gracefulExit:
			a.stopChildren(ex.from, spec.Children, string(ex.reason))
			return ex.reason

		case direct := <-p.direct:
			a.handleDirect(direct, spec.Children)
			continue

		case <-p.Context.Done():
			// node is down or killed using p.Kill()
			fmt.Printf("Warning: application %s has been killed\n", spec.Name)
			return "kill"
		case <-timer.C:
			// time to die
			go p.Exit(p.Self(), "normal")
		case msg := <-p.mailBox:
			if len(msg) == 0 {
				continue // ignore
			}
			switch r := msg[0].(type) {
			case etf.Tuple:
				var terminatedProcess *Process
				// waiting for {'EXIT', Pid, Reason}
				if len(r) != 3 || r.Element(1) != etf.Atom("EXIT") {
					// unknown. ignoring
					continue
				}
				terminated := r.Element(2).(etf.Pid)
				reason := r.Element(3).(etf.Atom)

				for i := range spec.Children {
					child := spec.Children[i].process
					if child != nil && child.Self() == terminated {
						terminatedProcess = child
						break
					}
				}

				switch spec.startType {
				case ApplicationStartPermanent:
					a.stopChildren(terminated, spec.Children, string(reason))
					fmt.Printf("Application (process) %s stopped with reason %s (permanent)", terminatedProcess.Name(), reason)
					p.Node.Stop()
					return "shutdown"

				case ApplicationStartTransient:
					if reason == etf.Atom("normal") || reason == etf.Atom("shutdown") {
						fmt.Printf("Application (process) %s stopped with reason %s (transient)", terminatedProcess.Name(), reason)
						continue
					}
					a.stopChildren(terminated, spec.Children, "normal")
					fmt.Printf("Application (process) %s stopped with reason %s. Node %s is shutting down",
						terminatedProcess.Name(), reason, p.Node.FullName)
					p.Node.Stop()
					return string(reason)

				case ApplicationStartTemporary:
					fmt.Printf("Application (process) %s stopped with reason %s (temporary)", terminatedProcess.Name(), reason)
				}

			}
		}

	}
}
func (a *Application) stopChildren(from etf.Pid, children []ApplicationChildSpec, reason string) {
	for i := range children {
		child := children[i].process
		if child != nil && child.self != from {
			children[i].process.Exit(from, reason)
			children[i].process = nil
		}
	}
}

func (a *Application) startChildren(parent *Process, children []ApplicationChildSpec) bool {
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

func (a *Application) handleDirect(m directMessage, children []ApplicationChildSpec) {
	switch m.id {
	case "getChildren":
		pids := []etf.Pid{}
		for i := range children {
			if children[i].process == nil {
				continue
			}
			pids = append(pids, children[i].process.self)
		}

		m.message = pids
		m.reply <- m

	default:
		if m.reply != nil {
			m.message = ErrUnsupportedRequest
			m.reply <- m
		}
	}
}
