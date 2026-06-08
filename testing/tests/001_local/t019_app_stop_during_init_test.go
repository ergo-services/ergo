package local

import (
	"fmt"
	"testing"
	"time"

	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/node"
)

type slowInitApp struct {
	app.Application
	sleep time.Duration
}

func (a *slowInitApp) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        "vslow_init",
		InitTimeout: 5 * time.Second,
		Group:       []gen.ApplicationMemberSpec{{Factory: factory_t12}},
	}, nil
}
func (a *slowInitApp) Init(ref gen.Ref, mode gen.ApplicationMode) error {
	time.Sleep(a.sleep)
	return nil
}

func waitState(n gen.Node, name gen.Atom, want gen.ApplicationState, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		info, err := n.ApplicationInfo(name)
		if err == nil && info.State == want {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	info, _ := n.ApplicationInfo(name)
	return fmt.Errorf("state %v, want %v", info.State, want)
}

func TestT19ApplicationStopDuringInit(t *testing.T) {
	for i, force := range []bool{false, true} {
		nodeName := gen.Atom(fmt.Sprintf("vstopinit%d@localhost", i))
		nopt := gen.NodeOptions{}
		nopt.Log.DefaultLogger.Disable = true
		n, err := node.Start(nodeName, nopt, gen.Version{})
		if err != nil {
			t.Fatal(err)
		}

		name, err := n.ApplicationLoad(&slowInitApp{sleep: 300 * time.Millisecond})
		if err != nil {
			n.Stop()
			t.Fatal(err)
		}

		go n.ApplicationStart(name, gen.ApplicationOptions{})

		// let it enter Initializing (Init is sleeping 300ms)
		if err := waitState(n, name, gen.ApplicationStateInitializing, time.Second); err != nil {
			n.Stop()
			t.Fatalf("force=%v: app did not reach Initializing: %v", force, err)
		}

		var stopErr error
		if force {
			stopErr = n.ApplicationStopForce(name)
		} else {
			stopErr = n.ApplicationStop(name)
		}
		t.Logf("force=%v: stop during init returned %v", force, stopErr)

		// the bug: graceful stop during init returned ErrApplicationState
		if stopErr == gen.ErrApplicationState {
			n.Stop()
			t.Fatalf("force=%v: stop during init rejected with ErrApplicationState", force)
		}

		// must end up stopped (Loaded), not hung in Initializing/Running
		if err := waitState(n, name, gen.ApplicationStateLoaded, 3*time.Second); err != nil {
			n.Stop()
			t.Fatalf("force=%v: app not stopped after stop-during-init: %v", force, err)
		}

		n.Stop()
	}
}
