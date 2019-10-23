package ergonode

// http://erlang.org/doc/apps/kernel/application.html

import (
	"fmt"
	"time"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type ApplicationStrategy = string

const (
	// Restart types:

	// ApplicationStrategyPermanent If a permanent application terminates,
	// all other applications and the runtime system (node) are also terminated.
	ApplicationStrategyPermanent = "permanent"

	// ApplicationStrategyTemporary If a temporary application terminates,
	// this is reported but no other applications are terminated.
	ApplicationStrategyTemporary = "temporary"

	// ApplicationStrategyTransient If a transient application terminates
	// with reason normal, this is reported but no other applications are
	// terminated. If a transient application terminates abnormally, that
	// is with any other reason than normal, all other applications and
	// the runtime system (node) are also terminated.
	ApplicationStrategyTransient = "transient"
)

// SupervisorBehavior interface
type ApplicationBehavior interface {
	Start(process Process, args ...interface{}) ApplicationSpec
}

type ApplicationSpec struct {
	Name         string
	Description  string
	ID           string
	Version      string
	MaxTime      time.Duration
	Applications []Application
	Environment  map[string]interface{}
	// Depends		[]
	children []ApplicationChildSpec
	Strategy ApplicationStrategy
}

type ApplicationChildSpec struct {
	child   ProcessBehaviour
	process *Process
}

// Application is implementation of ProcessBehavior interface
type Application struct {
	process Process
}

type requestAppSetEnv struct {
	name  string
	value interface{}
}

type requestAppGetEnv struct {
	name  string
	reply chan interface{}
}

type requestAppListEnv struct {
	reply chan map[string]interface{}
}

func (sv *Application) loop(p *Process, object interface{}, args ...interface{}) string {
	env := make(map[string]interface{})
	spec := object.(ApplicationBehavior).Start(*p, args...)
	children := []Process{}
	lib.Log("Application spec %#v\n", spec)
	p.ready <- true
	if spec.MaxTime == 0 {
		spec.MaxTime = time.Second * 31536000 * 100 // let's define default lifespan 100 years :)
	}
	for {
		select {
		case ex := <-p.gracefulExit:
			for i := range children {
				children[i].Exit(p.Self(), ex.reason)
			}
		case <-p.Context.Done():
			// node is down or killed using p.Kill()
			return "kill"
		case <-time.After(spec.MaxTime):
			// time to die
			p.Exit(p.Self(), "normal")
			return "shutdown"
		case msg := <-p.mailBox:
			if len(msg) == 0 {
				continue // ignore
			}
			switch r := msg[0].(type) {
			case requestAppSetEnv:
				env[r.name] = r.value
			case requestAppGetEnv:
				r.reply <- env[r.name]
			case requestAppListEnv:
				// make a copy of the original env
				newEnv := make(map[string]interface{})
				for key, value := range env {
					newEnv[key] = value
				}
				r.reply <- newEnv
			case etf.Tuple:
				var terminatedProcess *Process
				// waiting for {'EXIT', Pid, Reason}
				if len(r) != 3 || r.Element(1) != etf.Atom("EXIT") {
					// unknown. ignoring
					continue
				}
				terminated := r.Element(2).(etf.Pid)
				reason := r.Element(3).(etf.Atom)

				for i := range spec.children {
					child := spec.children[i].process
					if child.Self() == terminated {
						terminatedProcess = child
						spec.children[i] = spec.children[0]
						spec.children = spec.children[1:]
						break
					}
				}

				switch spec.Strategy {
				case ApplicationStrategyPermanent:
					stopChildren(terminated, spec.children, string(reason))
					p.Node.Stop()
					return "shutdown"

				case ApplicationStrategyTransient:
					if reason == etf.Atom("normal") || reason == etf.Atom("shutdown") {
						continue
					}
					stopChildren(terminated, spec.children, "normal")
					fmt.Printf("Application (process) %s stopped with reason %s. Node %s is shutting down",
						terminatedProcess.Name(), reason, p.Node.FullName)
					p.Node.Stop()
					return string(reason)

				case ApplicationStrategyTemporary:
					fmt.Printf("Application (process) %s stopped with reason %s", terminatedProcess.Name(), reason)
				}

			}
		}

	}
}

func stopChildren(from etf.Pid, children []ApplicationChildSpec, reason string) {
	for i := range children {
		children[i].process.Exit(from, reason)
	}
}
