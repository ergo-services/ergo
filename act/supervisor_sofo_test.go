package act

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
)

// setupSOFO builds a supSOFO from spec. SOFO does not auto-start children.
func setupSOFO(t *testing.T, spec SupervisorSpec) *supSOFO {
	t.Helper()
	sup := createSupSimpleOneForOne().(*supSOFO)
	spec = normSpec(spec)
	spec.Type = SupervisorTypeSimpleOneForOne

	if _, err := sup.init(spec); err != nil {
		t.Fatalf("init: %v", err)
	}
	return sup
}

// startSOFO mimics one full StartChild round: childSpec -> override Args ->
// (simulated) Spawn -> childStarted. Returns the assigned PID.
func startSOFO(t *testing.T, sup *supSOFO, name gen.Atom, id uint64, args ...any) gen.PID {
	t.Helper()
	action, err := sup.childSpec(name)
	if err != nil {
		t.Fatalf("childSpec: %v", err)
	}
	if len(args) > 0 {
		action.spec.Args = args
	}
	pid := makePID(id)
	sup.childStarted(action.spec, pid)
	return pid
}

// killSOFO drives one termination + restart (if any) and returns the
// resulting action and the new PID (or empty PID if no restart happened).
func killSOFO(t *testing.T, sup *supSOFO, name gen.Atom, pid gen.PID, reason error) (supAction, gen.PID) {
	t.Helper()
	action := sup.childTerminated(name, pid, reason)
	if action.do == supActionStartChild {
		newPID := makePID(pid.ID + 5000)
		sup.childStarted(action.spec, newPID)
		return action, newPID
	}
	return action, gen.PID{}
}

//
// per-spec Strategy override
//

func TestSOFOPerSpecStrategyTemporaryNoRestart(t *testing.T) {
	// supervisor=Transient + spec.Strategy=Temporary: abnormal dies, no restart.
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyTransient},
		Children: []SupervisorChildSpec{{
			Name:    "worker",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Strategy: SupervisorStrategyTemporary},
		}},
	})

	pid := startSOFO(t, sup, "worker", 100)
	action, newPID := killSOFO(t, sup, "worker", pid, errors.New("boom"))

	if action.do != supActionDoNothing {
		t.Fatalf("Temporary must NOT restart, got do=%d", action.do)
	}
	if newPID != (gen.PID{}) {
		t.Errorf("no respawn expected")
	}
	if _, alive := sup.instances[pid]; alive {
		t.Errorf("instance must be removed")
	}
}

func TestSOFOPerSpecStrategyPermanentRestartsOnNormal(t *testing.T) {
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyTransient},
		Children: []SupervisorChildSpec{{
			Name:    "worker",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Strategy: SupervisorStrategyPermanent},
		}},
	})

	pid := startSOFO(t, sup, "worker", 100)
	action, newPID := killSOFO(t, sup, "worker", pid, gen.TerminateReasonNormal)

	if action.do != supActionStartChild {
		t.Fatalf("Permanent must restart on Normal too, got do=%d", action.do)
	}
	if newPID == (gen.PID{}) {
		t.Errorf("expected respawn")
	}
}

func TestSOFOInheritUsesSupervisorStrategy(t *testing.T) {
	// supervisor=Permanent, spec.Strategy=Inherit (zero) -> Permanent.
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		Children: []SupervisorChildSpec{{
			Name:    "worker",
			Factory: dummyFactory,
		}},
	})

	pid := startSOFO(t, sup, "worker", 100)
	action, _ := killSOFO(t, sup, "worker", pid, gen.TerminateReasonNormal)

	if action.do != supActionStartChild {
		t.Fatalf("Inherit -> Permanent must restart on Normal, got do=%d", action.do)
	}
}

//
// per-instance counter
//

func TestSOFOPerInstanceCounterIncrements(t *testing.T) {
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		Children: []SupervisorChildSpec{{
			Name:    "worker",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Intensity: 5, Period: 60},
		}},
	})

	pid := startSOFO(t, sup, "worker", 100)
	_, pid = killSOFO(t, sup, "worker", pid, errors.New("boom"))
	_, pid = killSOFO(t, sup, "worker", pid, errors.New("boom"))

	// global counter must remain empty
	if len(sup.restarts) != 0 {
		t.Errorf("global counter must be untouched, got %d", len(sup.restarts))
	}
	// the (current) instance must carry a per-instance history of length 2
	inst, ok := sup.instances[pid]
	if ok == false {
		t.Fatalf("expected an instance for current pid")
	}
	if len(inst.restarts) != 2 {
		t.Errorf("inst.restarts = %d, want 2", len(inst.restarts))
	}
}

func TestSOFOPerInstanceCounterExceededTerminateSupervisor(t *testing.T) {
	// default OnExceed = TerminateSupervisor: all instances die, supervisor dies wrapped.
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		Children: []SupervisorChildSpec{{
			Name:    "worker",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Intensity: 2, Period: 60},
		}},
	})

	a := startSOFO(t, sup, "worker", 100)
	b := startSOFO(t, sup, "worker", 200)
	_ = b

	// drive 3 abnormal exits of "a" -> exceeds Intensity=2
	bootReason := errors.New("boom")
	pid := a
	var last supAction
	for i := 0; i < 3; i++ {
		last, pid = killSOFO(t, sup, "worker", pid, bootReason)
	}

	if last.do != supActionTerminateChildren {
		t.Fatalf("expected TerminateChildren on exceeded, got do=%d", last.do)
	}
	if sup.shutdown == false {
		t.Errorf("supervisor must be in shutdown")
	}
	ge, ok := sup.shutdownReason.(*gen.Error)
	if ok == false {
		t.Fatalf("shutdownReason must be *gen.Error, got %T", sup.shutdownReason)
	}
	if errors.Is(ge.Inner, bootReason) == false {
		t.Errorf("Inner must wrap original child reason; got %v", ge.Inner)
	}
}

func TestSOFOPerInstanceCounterExceededDisableDropsInstance(t *testing.T) {
	// OnExceedDisable: just this instance is dropped, supervisor + others alive.
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		Children: []SupervisorChildSpec{{
			Name:    "worker",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Intensity: 2, Period: 60, OnExceed: OnExceedDisable},
		}},
	})

	a := startSOFO(t, sup, "worker", 100)
	b := startSOFO(t, sup, "worker", 200)

	// drive 3 abnormal exits of "a"
	pid := a
	var last supAction
	for i := 0; i < 3; i++ {
		last, pid = killSOFO(t, sup, "worker", pid, errors.New("boom"))
	}

	if last.do != supActionDoNothing {
		t.Fatalf("OnExceedDisable must not trigger termination, got do=%d", last.do)
	}
	if sup.shutdown {
		t.Errorf("supervisor must remain alive")
	}
	// "b" must still be in instances
	if _, alive := sup.instances[b]; alive == false {
		t.Errorf("b instance must remain alive")
	}
	// "a" lineage must NOT be in instances any more
	if _, alive := sup.instances[pid]; alive {
		t.Errorf("a instance must be dropped")
	}
	// spec must NOT be disabled (SOFO drops the instance, keeps spec usable)
	if sup.spec["worker"].disabled {
		t.Errorf("SOFO must keep spec enabled after OnExceedDisable")
	}
}

func TestSOFOInstancesAreIndependent(t *testing.T) {
	// Two instances of the same spec; A flaps, B unaffected.
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		Children: []SupervisorChildSpec{{
			Name:    "worker",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Intensity: 5, Period: 60},
		}},
	})

	a := startSOFO(t, sup, "worker", 100)
	b := startSOFO(t, sup, "worker", 200)

	pid := a
	for i := 0; i < 3; i++ {
		_, pid = killSOFO(t, sup, "worker", pid, errors.New("boom"))
	}

	// A's instance has 3 restarts in history; B has 0
	instA, ok := sup.instances[pid]
	if ok == false {
		t.Fatalf("A instance lost")
	}
	if len(instA.restarts) != 3 {
		t.Errorf("A instance restarts = %d, want 3", len(instA.restarts))
	}
	instB := sup.instances[b]
	if len(instB.restarts) != 0 {
		t.Errorf("B instance restarts = %d, want 0", len(instB.restarts))
	}
}

func TestSOFOInstanceArgsSurviveRestart(t *testing.T) {
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		Children: []SupervisorChildSpec{{
			Name:    "worker",
			Factory: dummyFactory,
		}},
	})

	pid := startSOFO(t, sup, "worker", 100, "task-42", 7)
	// kick a restart
	_, pid = killSOFO(t, sup, "worker", pid, errors.New("boom"))

	inst, ok := sup.instances[pid]
	if ok == false {
		t.Fatalf("expected instance after restart")
	}
	if len(inst.args) != 2 || inst.args[0] != "task-42" || inst.args[1] != 7 {
		t.Errorf("args lost across restart: %v", inst.args)
	}
}

func TestSOFOInstanceCounterSurvivesRestart(t *testing.T) {
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		Children: []SupervisorChildSpec{{
			Name:    "worker",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Intensity: 5, Period: 60},
		}},
	})

	pid := startSOFO(t, sup, "worker", 100)
	_, pid = killSOFO(t, sup, "worker", pid, errors.New("boom"))
	_, pid = killSOFO(t, sup, "worker", pid, errors.New("boom"))
	_, pid = killSOFO(t, sup, "worker", pid, errors.New("boom"))

	inst, ok := sup.instances[pid]
	if ok == false {
		t.Fatalf("instance lost")
	}
	if len(inst.restarts) != 3 {
		t.Errorf("counter must survive restarts, got %d", len(inst.restarts))
	}
}

//
// global counter on SOFO
//

func TestSOFOGlobalExceededWrapsReason(t *testing.T) {
	// Without per-instance Restart, all instances share global counter.
	// Pre-populate to force exceeded on first abnormal.
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent, Intensity: 3, Period: 60},
		Children: []SupervisorChildSpec{{
			Name:    "worker",
			Factory: dummyFactory,
		}},
	})

	pid := startSOFO(t, sup, "worker", 100)
	bootReason := errors.New("boom")
	var last supAction
	for i := 0; i < 4; i++ {
		last, pid = killSOFO(t, sup, "worker", pid, bootReason)
	}

	if last.do != supActionTerminateChildren {
		t.Fatalf("exceeded must trigger TerminateChildren, got do=%d", last.do)
	}
	ge, ok := sup.shutdownReason.(*gen.Error)
	if ok == false {
		t.Fatalf("shutdownReason must be *gen.Error, got %T", sup.shutdownReason)
	}
	if errors.Is(ge.Inner, bootReason) == false {
		t.Errorf("Inner must wrap original child reason")
	}
}

//
// DisableChild on SOFO
//

func TestSOFODisableChildTerminatesAllInstances(t *testing.T) {
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		Children: []SupervisorChildSpec{
			{Name: "worker", Factory: dummyFactory},
			{Name: "other", Factory: dummyFactory},
		},
	})

	a := startSOFO(t, sup, "worker", 100)
	b := startSOFO(t, sup, "worker", 200)
	c := startSOFO(t, sup, "other", 300)

	action, err := sup.childDisable("worker")
	if err != nil {
		t.Fatalf("childDisable: %v", err)
	}
	if action.do != supActionTerminateChildren {
		t.Fatalf("expected TerminateChildren, got do=%d", action.do)
	}
	if action.reason != gen.TerminateReasonShutdown {
		t.Errorf("disable must use Shutdown reason, got %v", action.reason)
	}

	// terminate list must contain a and b but not c
	got := map[gen.PID]bool{}
	for _, p := range action.terminate {
		got[p] = true
	}
	if got[a] == false || got[b] == false {
		t.Errorf("worker instances must be in terminate list")
	}
	if got[c] {
		t.Errorf("other spec must NOT be in terminate list")
	}
}

//
// Temporary supervisor + per-instance Restart is a no-op (Restart never fires)
//

func TestSOFOTemporaryWithPerInstanceRestartIsNoOp(t *testing.T) {
	// Temporary -> instance always removed regardless of Restart settings.
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyTemporary},
		Children: []SupervisorChildSpec{{
			Name:    "task",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Intensity: 2, OnExceed: OnExceedDisable},
		}},
	})

	pid := startSOFO(t, sup, "task", 100)
	action, newPID := killSOFO(t, sup, "task", pid, errors.New("boom"))

	if action.do != supActionDoNothing {
		t.Fatalf("Temporary must remove instance without restart, got do=%d", action.do)
	}
	if newPID != (gen.PID{}) {
		t.Errorf("no respawn expected for Temporary")
	}
	// instance gone
	if _, alive := sup.instances[pid]; alive {
		t.Errorf("instance must be gone")
	}
}

//
// inspect output exposes per-spec aggregated counter
//

func TestSOFOInspectIncludesLocalRestarts(t *testing.T) {
	sup := setupSOFO(t, SupervisorSpec{
		Restart: SupervisorRestart{Strategy: SupervisorStrategyPermanent},
		Children: []SupervisorChildSpec{{
			Name:    "worker",
			Factory: dummyFactory,
			Restart: SupervisorChildRestart{Intensity: 5, Period: 60},
		}},
	})

	pid := startSOFO(t, sup, "worker", 100)
	_, _ = killSOFO(t, sup, "worker", pid, errors.New("boom"))

	out := sup.inspect()
	if v := out["child:'worker':restarts"]; v != "1" {
		t.Errorf("child:'worker':restarts = %q, want 1", v)
	}
}
