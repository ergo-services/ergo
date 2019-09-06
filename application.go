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
	Init(process *Process, args ...interface{}) ApplicationSpec
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

func (sv *Application) loop(p *Process, object interface{}, args ...interface{}) {
	spec := object.(ApplicationBehavior).Init(p, args...)
	lib.Log("Application spec %#v\n", spec)
	p.ready <- true

	stop := make(chan struct{}, 2)

	for {
		// var message etf.Term
		select {
		case <-stop:
			// object.(SupervisorBehavior).Terminate(reason, state)
			return

		case <-p.Context.Done():
			// object.(GenServerBehavior).Terminate("immediate", p.state)
			return
		}

		// lib.Log("[%#v]. Message from %#v\n", p.self, fromPid)

	}
}
