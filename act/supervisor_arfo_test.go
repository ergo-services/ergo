package act

import (
	"errors"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

// setupARFO builds a supARFO from spec and simulates the sequential start
// of every child. Returns the supervisor and the assigned PID per spec name.
// supType selects All-For-One or Rest-For-One.
func setupARFO(t *testing.T, supType SupervisorType, spec SupervisorSpec) (*supARFO, map[gen.Atom]gen.PID) {
	t.Helper()
	sup := createSupAllRestForOne().(*supARFO)
	spec = normSpec(spec)
	spec.Type = supType

	action, err := sup.init(spec)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	pids := map[gen.Atom]gen.PID{}
	var nextID uint64 = 100
	for action.do == supActionStartChild {
		pid := makePID(nextID)
		nextID++
		pids[action.spec.Name] = pid
		action = sup.childStarted(action.spec, pid)
	}
	return sup, pids
}

//
// per-child Strategy override on ARFO
//

func TestARFOPerChildPermanentTriggersGroupOnNormalExit(t *testing.T) {
	// supervisor=Transient (default) + child=Permanent: Normal exit MUST trigger group restart.
	sup, pids := setupARFO(t, SupervisorTypeAllForOne, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyTransient},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{
				Name:    "b",
				Factory: dummyFactory,
				Restart: SupervisorChildRestart{Strategy: SupervisorStrategyPermanent},
			},
		},
	})

	action := sup.childTerminated("b", pids["b"], gen.TerminateReasonNormal)
	if action.do != supActionTerminateChildren {
		t.Fatalf("expected TerminateChildren on Permanent override + Normal exit, got do=%d", action.do)
	}
	// expect "a" pid in the terminate list (sibling under group restart)
	containsA := false
	for _, p := range action.terminate {
		if p == pids["a"] {
			containsA = true
		}
	}
	if containsA == false {
		t.Errorf("group restart must terminate sibling 'a'")
	}
}

func TestARFOPerChildTemporaryDoesNotTriggerGroup(t *testing.T) {
	// supervisor=Permanent + child=Temporary: abnormal exit MUST NOT trigger group restart.
	sup, pids := setupARFO(t, SupervisorTypeAllForOne, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{
				Name:    "b",
				Factory: dummyFactory,
				Restart: SupervisorChildRestart{Strategy: SupervisorStrategyTemporary},
			},
		},
	})

	action := sup.childTerminated("b", pids["b"], errors.New("boom"))
	if action.do != supActionDoNothing {
		t.Fatalf("Temporary override must NOT trigger group restart, got do=%d", action.do)
	}
	if sup.mode == 3 {
		t.Errorf("supervisor must not be in shutdown mode")
	}
}

func TestARFOPerChildTransientNormalExitDoesNotTriggerGroup(t *testing.T) {
	// supervisor=Permanent + child=Transient: Normal exit must NOT trigger group restart.
	sup, pids := setupARFO(t, SupervisorTypeAllForOne, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{
				Name:    "b",
				Factory: dummyFactory,
				Restart: SupervisorChildRestart{Strategy: SupervisorStrategyTransient},
			},
		},
	})

	action := sup.childTerminated("b", pids["b"], gen.TerminateReasonNormal)
	if action.do != supActionDoNothing {
		t.Fatalf("Transient + Normal must NOT trigger group restart, got do=%d", action.do)
	}
}

func TestARFOPerChildTransientAbnormalTriggersGroup(t *testing.T) {
	// supervisor=Transient (default) + child=Transient explicitly + abnormal exit: triggers group.
	sup, pids := setupARFO(t, SupervisorTypeAllForOne, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyTransient},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{
				Name:    "b",
				Factory: dummyFactory,
				Restart: SupervisorChildRestart{Strategy: SupervisorStrategyTransient},
			},
		},
	})

	action := sup.childTerminated("b", pids["b"], errors.New("boom"))
	if action.do != supActionTerminateChildren {
		t.Fatalf("Transient + abnormal must trigger group restart, got do=%d", action.do)
	}
}

//
// ROFO per-child Strategy
//

func TestROFOPerChildTemporaryMiddleDoesNotTriggerGroup(t *testing.T) {
	// supervisor=Permanent + middle child Temporary; abnormal of middle must
	// NOT trigger group restart in Rest-For-One either.
	sup, pids := setupARFO(t, SupervisorTypeRestForOne, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{
				Name:    "b",
				Factory: dummyFactory,
				Restart: SupervisorChildRestart{Strategy: SupervisorStrategyTemporary},
			},
			{Name: "c", Factory: dummyFactory},
		},
	})

	action := sup.childTerminated("b", pids["b"], errors.New("boom"))
	if action.do != supActionDoNothing {
		t.Fatalf("Temporary middle must not trigger ROFO group restart, got do=%d", action.do)
	}
	// "a" and "c" must remain
	for _, c := range sup.spec {
		if c.Name == "a" && c.pid != pids["a"] {
			t.Errorf("a must be unaffected")
		}
		if c.Name == "c" && c.pid != pids["c"] {
			t.Errorf("c must be unaffected")
		}
	}
}

func TestROFOPermanentFailureRestartsRest(t *testing.T) {
	// Default ROFO: middle child fails abnormal -> terminate rest (b + c),
	// preserve a.
	sup, pids := setupARFO(t, SupervisorTypeRestForOne, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{Name: "b", Factory: dummyFactory},
			{Name: "c", Factory: dummyFactory},
		},
	})

	action := sup.childTerminated("b", pids["b"], errors.New("boom"))
	if action.do != supActionTerminateChildren {
		t.Fatalf("ROFO Permanent abnormal must trigger restart, got do=%d", action.do)
	}
	// a must NOT be in terminate list, c MUST be in terminate list
	for _, p := range action.terminate {
		if p == pids["a"] {
			t.Errorf("a (preceding) must NOT be terminated in ROFO")
		}
	}
	containsC := false
	for _, p := range action.terminate {
		if p == pids["c"] {
			containsC = true
		}
	}
	if containsC == false {
		t.Errorf("c (following) MUST be terminated in ROFO")
	}
}

//
// global counter exceeded -> gen.Error wrap on shutdownReason
//

func TestARFOGlobalExceededWrapsReason(t *testing.T) {
	sup, pids := setupARFO(t, SupervisorTypeAllForOne, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent, Intensity: 5, Period: 60},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{Name: "b", Factory: dummyFactory},
		},
	})

	// Pre-populate the global counter with 5 fresh entries; the next abnormal
	// exit makes len=6 > intensity=5 and triggers the exceeded branch.
	now := time.Now().UnixMilli()
	sup.restarts = []int64{now - 50, now - 40, now - 30, now - 20, now - 10}

	bootReason := errors.New("boom")
	action := sup.childTerminated("a", pids["a"], bootReason)

	if action.do != supActionTerminateChildren {
		t.Fatalf("expected TerminateChildren on exceeded, got do=%d", action.do)
	}
	if action.reason != gen.ErrExceeded {
		t.Errorf("action.reason must be gen.ErrExceeded for children, got %v", action.reason)
	}
	if errors.Is(sup.shutdownReason, gen.ErrExceeded) == false {
		t.Errorf("shutdownReason must match gen.ErrExceeded via errors.Is; got %v", sup.shutdownReason)
	}
	if errors.Is(sup.shutdownReason, bootReason) == false {
		t.Errorf("shutdownReason must wrap original child reason; got %v", sup.shutdownReason)
	}
}

//
// per-child Significant remains usable on ARFO
//

func TestARFOInspectRecordsTriggerInHistory(t *testing.T) {
	sup, pids := setupARFO(t, SupervisorTypeAllForOne, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{Name: "b", Factory: dummyFactory},
		},
	})

	sup.childTerminated("a", pids["a"], errors.New("boom-from-a"))

	out := sup.inspect()
	if out["ergo:history:count"] != "1" {
		t.Fatalf("expected exactly one history entry for the triggering child, got %q", out["ergo:history:count"])
	}
	if out["ergo:history:0:child"] != "a" || out["ergo:history:0:reason"] != "boom-from-a" {
		t.Errorf("history[0] mismatch: child=%q reason=%q", out["ergo:history:0:child"], out["ergo:history:0:reason"])
	}
}

func TestARFOSignificantChildTerminatesGroup(t *testing.T) {
	// Temporary supervisor + Significant Temporary child: when child dies
	// (any reason, since Temporary), supervisor + siblings must shutdown.
	sup, pids := setupARFO(t, SupervisorTypeAllForOne, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyTemporary},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{Name: "b", Factory: dummyFactory, Significant: true},
		},
	})

	bootReason := errors.New("boom")
	action := sup.childTerminated("b", pids["b"], bootReason)

	if action.do != supActionTerminateChildren {
		t.Fatalf("Significant Temporary child must take supervisor down, got do=%d", action.do)
	}
	if sup.mode != 3 {
		t.Errorf("supervisor must enter shutdown mode (3)")
	}
	if errors.Is(sup.shutdownReason, bootReason) == false {
		t.Errorf("Significant-induced shutdown should preserve original reason; got %v", sup.shutdownReason)
	}
}
