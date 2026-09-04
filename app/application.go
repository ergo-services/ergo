// Package app provides the base type for application behaviors.
//
// Users compose application behaviors by embedding Application and
// implementing Load. The base provides:
//   - PreLoad (framework entry point, do not override)
//   - Default no-op Init, Start, Stop, Terminate
//   - Promoted methods of gen.Application via embed (Name, Log, AddTag, etc.)
package app

import (
	"fmt"

	"ergo.services/ergo/gen"
)

// Application is the embeddable base for gen.ApplicationBehavior implementations.
//
//	type MyApp struct {
//	    app.Application
//	}
//
//	func (a *MyApp) Load(args ...any) (gen.ApplicationSpec, error) {
//	    a.Log().Info("loading")
//	    return gen.ApplicationSpec{Name: "myapp", ...}, nil
//	}
//
// Override Init, Start, Stop, Terminate as needed.
type Application struct {
	gen.Application

	bound bool
}

// PreLoad is the framework entry point invoked by the node during
// ApplicationLoad. It binds the runtime application and dispatches to Load.
//
// DO NOT OVERRIDE THIS METHOD. Overriding breaks the runtime binding;
// subsequent default callbacks will panic on access to bound state.
func (a *Application) PreLoad(app gen.Application, args ...any) (gen.ApplicationSpec, error) {
	a.Application = app
	a.bound = true
	return app.Behavior().Load(args...)
}

func (a *Application) requireBound(callback string) {
	if a.bound == false {
		panic(fmt.Sprintf(
			"app.Application not bound; %s called before PreLoad. "+
				"Did you override PreLoad? See app.Application docs.",
			callback,
		))
	}
}

// Init is the default no-op pre-start callback. Override as needed.
func (a *Application) Init(ref gen.Ref, mode gen.ApplicationMode) error {
	a.requireBound("Init")
	return nil
}

// Start is the default no-op post-start callback. Override as needed.
func (a *Application) Start(ref gen.Ref, mode gen.ApplicationMode) {
	a.requireBound("Start")
}

// Stop is the default no-op pre-stop callback. Override as needed.
func (a *Application) Stop(ref gen.Ref, reason error) {
	a.requireBound("Stop")
}

// Terminate is the default no-op post-stop callback. Override as needed.
func (a *Application) Terminate(reason error) {
	a.requireBound("Terminate")
}
