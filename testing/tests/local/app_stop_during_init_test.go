package local

import (
	"fmt"
	"testing"
	"time"

	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// slowInitApp blocks in Init long enough for a stop request to race the start.
type slowInitApp struct {
	app.Application
	sleep time.Duration
}

func (a *slowInitApp) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        "vslow_init",
		InitTimeout: 5 * time.Second,
		Group:       []gen.ApplicationMemberSpec{{Factory: factoryAppMember}},
	}, nil
}

func (a *slowInitApp) Init(ref gen.Ref, mode gen.ApplicationMode) error {
	time.Sleep(a.sleep)
	return nil
}

// waitState polls until the application reaches want or the deadline passes.
func waitState(nn gen.Node, name gen.Atom, want gen.ApplicationState, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		info, err := nn.ApplicationInfo(name)
		if err == nil && info.State == want {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	info, _ := nn.ApplicationInfo(name)
	return fmt.Errorf("state %v, want %v", info.State, want)
}

// TestLocalApplicationStopDuringInit: stopping an application while it is still in
// Init (both graceful Stop and StopForce) must not be rejected with
// ErrApplicationState, and the application must end up stopped (Loaded), never hung
// in Initializing or Running.
func TestLocalApplicationStopDuringInit(t *testing.T) {
	for _, force := range []bool{false, true} {
		s := stage.New(t)
		n := s.Node("n")
		nn := n.Native()

		name, err := nn.ApplicationLoad(&slowInitApp{sleep: 300 * time.Millisecond})
		check.NoError(t, err)

		go nn.ApplicationStart(name, gen.ApplicationOptions{})

		// let it enter Initializing (Init is sleeping)
		check.NoError(t, waitState(nn, name, gen.ApplicationStateInitializing, time.Second))

		var stopErr error
		if force {
			stopErr = nn.ApplicationStopForce(name)
		} else {
			stopErr = nn.ApplicationStop(name)
		}
		t.Logf("force=%v: stop during init returned %v", force, stopErr)

		// the bug: graceful stop during init returned ErrApplicationState
		if stopErr == gen.ErrApplicationState {
			t.Fatalf("force=%v: stop during init rejected with ErrApplicationState", force)
		}

		// must end up stopped (Loaded), not hung in Initializing/Running
		check.NoError(t, waitState(nn, name, gen.ApplicationStateLoaded, 3*time.Second))
	}
}
