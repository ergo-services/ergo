package act

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
)

// setupOFO builds a supOFO from spec and simulates the sequential start of
// every child. Returns the supervisor and the assigned PID for each spec name.
func setupOFO(t *testing.T, spec SupervisorSpec) (*supOFO, map[gen.Atom]gen.PID) {
	t.Helper()
	sup := createSupOneForOne().(*supOFO)
	spec = normSpec(spec)
	spec.Type = SupervisorTypeOneForOne

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

// killOFO drives one child termination and simulates the resulting restart
// (if any) so that subsequent calls run against fresh PIDs. Returns the
// final action so tests can inspect the outcome of the very last termination.
func killOFO(t *testing.T, sup *supOFO, pids map[gen.Atom]gen.PID, name gen.Atom, reason error) supAction {
	t.Helper()
	pid := pids[name]
	action := sup.childTerminated(name, pid, reason)

	if action.do == supActionStartChild {
		// simulate the restart spawn
		newPID := makePID(pid.ID + 1000)
		pids[name] = newPID
		next := sup.childStarted(action.spec, newPID)
		// after a single restart in OFO, the next action is DoNothing
		_ = next
		action.do = supActionStartChild // preserve original outcome for the caller
	}
	return action
}

//
// per-child Strategy override
//

func TestOFOPerChildStrategyPermanentOverridesNormalExit(t *testing.T) {
	// supervisor Transient + child Permanent: child must be restarted even on Normal exit
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyTransient},
		Children: []SupervisorChildSpec{{
			Name:    "child",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Strategy: SupervisorStrategyPermanent},
		}},
	})

	action := sup.childTerminated("child", pids["child"], gen.TerminateReasonNormal)
	if action.do != supActionStartChild {
		t.Fatalf("expected restart on Normal exit (Permanent override), got do=%d", action.do)
	}
}

func TestOFOPerChildStrategyTemporaryOverridesAbnormal(t *testing.T) {
	// supervisor Permanent + child Temporary + autoshutdown disabled:
	// child must NOT be restarted even on abnormal exit; supervisor stays alive.
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{{
			Name:    "child",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Strategy: SupervisorStrategyTemporary},
		}},
	})

	action := sup.childTerminated("child", pids["child"], errors.New("boom"))
	if action.do != supActionDoNothing {
		t.Fatalf("expected DoNothing, got do=%d", action.do)
	}
	if sup.shutdown {
		t.Fatalf("supervisor unexpectedly shutdown")
	}
}

func TestOFOPerChildStrategyInheritUsesSupervisor(t *testing.T) {
	// supervisor=Permanent, child=Inherit (zero) -> child is Permanent
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		Children: []SupervisorChildSpec{{
			Name:    "child",
			Factory: dummyFactory,
		}},
	})

	action := sup.childTerminated("child", pids["child"], gen.TerminateReasonNormal)
	if action.do != supActionStartChild {
		t.Fatalf("Permanent must restart on Normal too (got do=%d)", action.do)
	}
}

//
// per-spec counter selection (global vs local)
//

func TestOFOLocalCounterDoesNotTouchGlobal(t *testing.T) {
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent, Intensity: 10, Period: 5},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{{
			Name:    "child",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Intensity: 5, Period: 60},
		}},
	})

	killOFO(t, sup, pids, "child", errors.New("boom"))

	if len(sup.restarts) != 0 {
		t.Errorf("global counter should be untouched, got %d", len(sup.restarts))
	}
	if len(sup.spec[0].localRestarts) != 1 {
		t.Errorf("local counter should be 1, got %d", len(sup.spec[0].localRestarts))
	}
}

func TestOFOGlobalCounterUsedWhenNoLocal(t *testing.T) {
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent, Intensity: 10, Period: 5},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{{
			Name:    "child",
			Factory: dummyFactory,
		}},
	})

	killOFO(t, sup, pids, "child", errors.New("boom"))

	if len(sup.restarts) != 1 {
		t.Errorf("global counter should be 1, got %d", len(sup.restarts))
	}
	if sup.spec[0].localRestarts != nil {
		t.Errorf("local counter must remain nil, got %v", sup.spec[0].localRestarts)
	}
}

//
// per-spec counter exceeded behavior
//

func TestOFOLocalCounterExceededDefaultTerminatesSupervisor(t *testing.T) {
	// OnExceed defaults to TerminateSupervisor
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{
				Name:    "b",
				Factory: dummyFactory,
				Restart: SupervisorChildRestart{Intensity: 2, Period: 60},
			},
		},
	})

	// 3 abnormal exits of "b" => exceeds Intensity=2 (we keep last 2; appending the 3rd triggers exceeded)
	var last supAction
	bootReason := errors.New("boom")
	for i := 0; i < 3; i++ {
		last = killOFO(t, sup, pids, "b", bootReason)
	}

	if last.do != supActionTerminateChildren {
		t.Fatalf("expected TerminateChildren on exceeded, got do=%d", last.do)
	}
	if sup.shutdown == false {
		t.Errorf("supervisor must be in shutdown state")
	}
	ge, ok := sup.shutdownReason.(*gen.Error)
	if ok == false {
		t.Fatalf("shutdownReason must be *gen.Error, got %T", sup.shutdownReason)
	}
	if ge.Msg != "restart intensity exceeded" {
		t.Errorf("Msg = %q, want %q", ge.Msg, "restart intensity exceeded")
	}
	if errors.Is(ge.Inner, bootReason) == false {
		t.Errorf("Inner should be the original child reason; got %v", ge.Inner)
	}
}

func TestOFOLocalCounterExceededDisableKeepsSupervisor(t *testing.T) {
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{
				Name:    "b",
				Factory: dummyFactory,
				Restart: SupervisorChildRestart{Intensity: 2, Period: 60, OnExceed: OnExceedDisable},
			},
		},
	})

	for i := 0; i < 3; i++ {
		killOFO(t, sup, pids, "b", errors.New("boom"))
	}

	if sup.shutdown {
		t.Errorf("supervisor must remain alive when OnExceedDisable")
	}
	// "b" must be disabled and its local counter cleared
	var b *supChildSpec
	for _, c := range sup.spec {
		if c.Name == "b" {
			b = c
		}
	}
	if b.disabled == false {
		t.Errorf("b must be disabled after exceeded with OnExceedDisable")
	}
	if b.localRestarts != nil {
		t.Errorf("local counter must be cleared after disable, got %v", b.localRestarts)
	}
	// "a" must remain running with empty pid set unchanged
	var a *supChildSpec
	for _, c := range sup.spec {
		if c.Name == "a" {
			a = c
		}
	}
	if a.pid != pids["a"] {
		t.Errorf("a's pid should be unchanged")
	}
}

func TestOFOLocalCounterExceededDisableLastChildTriggersAutoshutdown(t *testing.T) {
	// Single child with OnExceedDisable. After exceeded, no children remain;
	// with autoshutdown enabled (default) the supervisor terminates.
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		// autoshutdown defaults to ON
		Children: []SupervisorChildSpec{{
			Name:    "lonely",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Intensity: 2, Period: 60, OnExceed: OnExceedDisable},
		}},
	})

	bootReason := errors.New("boom")
	var last supAction
	for i := 0; i < 3; i++ {
		last = killOFO(t, sup, pids, "lonely", bootReason)
	}

	if last.do != supActionTerminate {
		t.Fatalf("expected supActionTerminate on autoshutdown, got do=%d", last.do)
	}
	ge, ok := last.reason.(*gen.Error)
	if ok == false {
		t.Fatalf("autoshutdown reason should be wrapped gen.Error, got %T", last.reason)
	}
	if errors.Is(ge.Inner, bootReason) == false {
		t.Errorf("Inner must be the original child reason")
	}
}

func TestOFOLocalCounterExceededDisableLastChildAutoshutdownOff(t *testing.T) {
	// DisableAutoShutdown -> supervisor stays alive even with empty pool
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{{
			Name:    "lonely",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Intensity: 2, Period: 60, OnExceed: OnExceedDisable},
		}},
	})

	for i := 0; i < 3; i++ {
		killOFO(t, sup, pids, "lonely", errors.New("boom"))
	}

	if sup.shutdown {
		t.Errorf("supervisor must remain alive with DisableAutoShutdown")
	}
	if sup.spec[0].disabled == false {
		t.Errorf("child must be disabled")
	}
}

//
// EnableChild / DisableChild interactions with localRestarts
//

func TestOFOEnableAfterDisableResetsCounter(t *testing.T) {
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{
				Name:    "b",
				Factory: dummyFactory,
				Restart: SupervisorChildRestart{Intensity: 2, Period: 60, OnExceed: OnExceedDisable},
			},
		},
	})

	for i := 0; i < 3; i++ {
		killOFO(t, sup, pids, "b", errors.New("boom"))
	}

	action, err := sup.childEnable("b")
	if err != nil {
		t.Fatalf("childEnable: %v", err)
	}
	if action.do != supActionStartChild {
		t.Fatalf("EnableChild after disable must trigger StartChild, got do=%d", action.do)
	}

	var b *supChildSpec
	for _, c := range sup.spec {
		if c.Name == "b" {
			b = c
		}
	}
	if b.disabled {
		t.Errorf("b must be enabled after EnableChild")
	}
	if b.localRestarts != nil {
		t.Errorf("local counter must be cleared on enable, got %v", b.localRestarts)
	}
}

func TestOFODisableClearsLocalRestarts(t *testing.T) {
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{{
			Name:    "child",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Intensity: 5, Period: 60},
		}},
	})

	// Drive one abnormal exit to populate local counter.
	killOFO(t, sup, pids, "child", errors.New("boom"))
	if len(sup.spec[0].localRestarts) != 1 {
		t.Fatalf("expected localRestarts=1 before disable")
	}

	if _, err := sup.childDisable("child"); err != nil {
		t.Fatalf("childDisable: %v", err)
	}
	if sup.spec[0].localRestarts != nil {
		t.Errorf("local counter must be cleared on DisableChild")
	}
}

//
// global-counter exceeded with sibling having local counter
//

func TestOFOGlobalExceededTerminatesEvenChildrenWithLocal(t *testing.T) {
	sup, pids := setupOFO(t, SupervisorSpec{
		// Intensity=2 on global, easy to overflow.
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent, Intensity: 2, Period: 60},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{
				Name:    "b",
				Factory: dummyFactory,
				Restart: SupervisorChildRestart{Intensity: 100, Period: 60, OnExceed: OnExceedDisable},
			},
		},
	})

	bootReason := errors.New("boom")
	var last supAction
	// "a" uses the global counter; 3 abnormal exits saturate Intensity=2.
	for i := 0; i < 3; i++ {
		last = killOFO(t, sup, pids, "a", bootReason)
	}

	if last.do != supActionTerminateChildren {
		t.Fatalf("global exceeded must trigger TerminateChildren, got do=%d", last.do)
	}
	if sup.shutdown == false {
		t.Errorf("supervisor must be shutdown")
	}
	// "b" should be in the terminate list (it was running)
	containsBPID := false
	for _, p := range last.terminate {
		if p == pids["b"] {
			containsBPID = true
		}
	}
	if containsBPID == false {
		t.Errorf("b's pid must be in terminate list when global exceeded")
	}
	ge, ok := sup.shutdownReason.(*gen.Error)
	if ok == false {
		t.Fatalf("shutdownReason must be *gen.Error, got %T", sup.shutdownReason)
	}
	if errors.Is(ge.Inner, bootReason) == false {
		t.Errorf("Inner must be the original child reason")
	}
}

//
// inspect output exposes per-spec restart counter
//

//
// PreserveMailbox: extraction in childTerminated
//

func TestOFOAdoptsMailboxFromGenError(t *testing.T) {
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		Children: []SupervisorChildSpec{{
			Name:    "child",
			Factory: dummyFactory,
		}},
	})

	pmb := &gen.ProcessMailbox{}
	reason := &gen.Error{
		Msg:     "panic: boom",
		Inner:   gen.TerminateReasonPanic,
		Mailbox: pmb,
	}
	action := sup.childTerminated("child", pids["child"], reason)

	if action.do != supActionStartChild {
		t.Fatalf("expected restart, got do=%d", action.do)
	}
	if action.adoptMailbox != pmb {
		t.Errorf("adoptMailbox should carry the captured mailbox; got %v", action.adoptMailbox)
	}
	if reason.Mailbox != nil {
		t.Errorf("Mailbox must be cleared on the *gen.Error after extraction (HandleChildTerminate race)")
	}
}

func TestOFONoAdoptionWhenReasonHasNoMailbox(t *testing.T) {
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		Children: []SupervisorChildSpec{{
			Name:    "child",
			Factory: dummyFactory,
		}},
	})

	action := sup.childTerminated("child", pids["child"], errors.New("plain error"))

	if action.do != supActionStartChild {
		t.Fatalf("expected restart, got do=%d", action.do)
	}
	if action.adoptMailbox != nil {
		t.Errorf("no Mailbox in reason -> no adoption; got %v", action.adoptMailbox)
	}
}

func TestOFOInspectIncludesLocalRestarts(t *testing.T) {
	sup, pids := setupOFO(t, SupervisorSpec{
		Restart:             SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []SupervisorChildSpec{
			{Name: "a", Factory: dummyFactory},
			{
				Name:    "b",
				Factory: dummyFactory,
				Restart: SupervisorChildRestart{Intensity: 5, Period: 60},
			},
		},
	})

	killOFO(t, sup, pids, "b", errors.New("boom"))

	out := sup.inspect()
	// gen.Atom.String() wraps the name in single quotes (matches existing inspect keys)
	if v := out["child:'b':restarts"]; v != "1" {
		t.Errorf("child:'b':restarts = %q, want 1", v)
	}
	// "a" has no local Restart, must NOT show the key
	if _, ok := out["child:'a':restarts"]; ok {
		t.Errorf("child:'a':restarts must not appear when no local counter is configured")
	}
}
