package ergonode

// http://erlang.org/doc/apps/kernel/application.html

import (
	"time"

	"github.com/halturin/ergonode/lib"
)

type ApplicationRestart = string

const (
	// Restart types:

	// ApplicationRestartPermanent child process is always restarted
	ApplicationRestartPermanent = "permanent"

	// ApplicationRestartTemporary child process is never restarted
	// (not even when the supervisor restart strategy is rest_for_one
	// or one_for_all and a sibling death causes the temporary process
	// to be terminated)
	ApplicationRestartTemporary = "temporary"

	// ApplicationRestartTransient child process is restarted only if
	// it terminates abnormally, that is, with an exit reason other
	// than normal, shutdown, or {shutdown,Term}.
	ApplicationRestartTransient = "transient"
)

// SupervisorBehavior interface
type ApplicationBehavior interface {
	Init(process Process, args ...interface{}) ApplicationSpec
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
	strategy ApplicationRestart
}

type ApplicationChildSpec struct {
	child ProcessBehaviour
}

// Supervisor is implementation of ProcessBehavior interface
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

type requsetAppListEnv struct {
	reply chan map[string]interface{}
}

func (sv *Application) loop(p *Process, object interface{}, args ...interface{}) {
	env := make(map[string]interface{})
	spec := object.(ApplicationBehavior).Init(*p, args...)
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
			// TODO: wait for all children' EXITs
		case <-p.Context.Done():
			// node is down or killed using p.Kill()
			return
		case <-time.After(spec.MaxTime):
			// time to die
			p.Exit(p.Self(), "normal")
			return
		case msg := <-p.mailBox:
			if len(msg) == 0 {
				continue // ignore
			}
			switch r := msg[0].(type) {
			case requestAppSetEnv:
				env[r.name] = r.value
			case requestAppGetEnv:
				r.reply <- env[r.name]
			case requsetAppListEnv:
				// make a copy of the original env
				newEnv := make(map[string]interface{})
				for key, value := range env {
					newEnv[key] = value
				}
				r.reply <- newEnv
			}
		}

	}
}
