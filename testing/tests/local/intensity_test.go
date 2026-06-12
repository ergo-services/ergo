package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// intensitySup is a OneForOne supervisor with a low restart intensity (2 in 5s).
//
// local=false: a single Permanent child on the supervisor's global counter;
// exceeding it terminates the supervisor.
//
// local=true: child "c0" uses a per-spec counter with OnExceedDisable (exceeding
// it disables c0); a second child "c1" stays running so the supervisor does not
// auto-shutdown and survives.
type intensitySup struct{ act.Supervisor }

func factoryIntensitySup() gen.ProcessBehavior { return &intensitySup{} }

func (s *intensitySup) Init(args ...any) (act.SupervisorSpec, error) {
	local := args[0].(bool)
	spec := act.SupervisorSpec{
		Type:              act.SupervisorTypeOneForOne,
		Restart:           act.SupervisorRestart{Strategy: act.SupervisorStrategyPermanent, Intensity: 2, Period: 5},
		EnableHandleChild: true,
	}
	if local {
		spec.Children = []act.SupervisorChildSpec{
			{Name: "c0", Factory: factoryEcho, Restart: act.SupervisorChildRestart{Intensity: 2, Period: 5, OnExceed: act.OnExceedDisable}},
			{Name: "c1", Factory: factoryEcho},
		}
	} else {
		spec.Children = []act.SupervisorChildSpec{{Name: "c", Factory: factoryEcho}}
	}
	return spec, nil
}

func (s *intensitySup) HandleChildStart(name gen.Atom, pid gen.PID) error {
	return s.Send(s.PID(), childStarted{Name: name})
}
func (s *intensitySup) HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error {
	return s.Send(s.PID(), childStopped{PID: pid, Reason: reason})
}
func (s *intensitySup) HandleMessage(from gen.PID, message any) error { return nil }

type childPidReq struct{ Name gen.Atom }
type startReq struct{ Name gen.Atom }
type enableReq struct{ Name gen.Atom }

func (s *intensitySup) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch r := request.(type) {
	case childPidReq:
		for _, c := range s.Children() {
			if c.Name == r.Name {
				return c.PID, nil
			}
		}
		return gen.PID{}, nil
	case startReq:
		return errText(s.StartChild(r.Name)), nil
	case enableReq:
		return errText(s.EnableChild(r.Name)), nil
	}
	return "ok", nil
}

func intensityChildPID(t *testing.T, n *stage.Node, sup gen.PID, name gen.Atom) gen.PID {
	t.Helper()
	v, err := n.Call(sup, childPidReq{Name: name})
	check.NoError(t, err)
	return v.(gen.PID)
}

// killUntilExceed kills the named child twice (each restarts, staying under the
// intensity), then a third time which exceeds it. It returns the mark taken right
// before the third kill and the pid that was killed, so the caller can scope its
// assertions and wait for that termination to be handled.
func killUntilExceed(t *testing.T, n *stage.Node, sup gen.PID, name gen.Atom) (int, gen.PID) {
	t.Helper()
	n.ShouldSend().From(sup).Message(childStarted{Name: name}).AtLeast(1).Within(time.Second).Must()
	for i := 0; i < 2; i++ {
		pid := intensityChildPID(t, n, sup, name)
		mk := n.Mark()
		check.NoError(t, n.SendExit(pid, gen.TerminateReasonKill))
		n.ShouldSend().From(sup).Message(childStarted{Name: name}).Since(mk).Once().Within(time.Second).Must()
	}
	pid := intensityChildPID(t, n, sup, name)
	mk := n.Mark()
	check.NoError(t, n.SendExit(pid, gen.TerminateReasonKill))
	return mk, pid
}

// TestLocalSupervisorRestartIntensityTerminate: exceeding the supervisor's global
// restart intensity terminates the supervisor with a reason wrapping
// gen.ErrExceeded. (The old t010/t011 left this as //TODO TestRestartIntensity.)
func TestLocalSupervisorRestartIntensityTerminate(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	w := n.Spawn(factoryWatcher, gen.ProcessOptions{})
	sup := n.Spawn(factoryIntensitySup, gen.ProcessOptions{}, false)

	n.Send(w, monitorCmd{Target: sup})
	n.ShouldMonitor().From(w).Target(sup).Once().Within(time.Second).Must()

	mk, _ := killUntilExceed(t, n, sup, "c")
	n.ShouldReceiveDown().To(w).About(sup).ReasonIs(gen.ErrExceeded).Since(mk).Once().Within(time.Second).Must()
}

// TestLocalSupervisorRestartIntensityDisable: with a per-spec counter and
// OnExceedDisable, exceeding the intensity disables the offending child but leaves
// the supervisor (and its other child) alive; the child can be re-enabled.
func TestLocalSupervisorRestartIntensityDisable(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	w := n.Spawn(factoryWatcher, gen.ProcessOptions{})
	sup := n.Spawn(factoryIntensitySup, gen.ProcessOptions{}, true)

	n.Send(w, monitorCmd{Target: sup})
	n.ShouldMonitor().From(w).Target(sup).Once().Within(time.Second).Must()

	mk, killed := killUntilExceed(t, n, sup, "c0")

	// barrier: wait until the supervisor handled the exceeding termination of c0
	// (the death notification reaches the supervisor independently of our calls,
	// so a plain Call would race it). childStopped fires from HandleChildTerminate.
	n.ShouldSend().From(sup).Where(func(r check.Sent) bool {
		cs, ok := r.Message.(childStopped)
		return ok && cs.PID == killed
	}).Since(mk).Once().Within(time.Second).Must()

	// the supervisor survived and c0 was not restarted
	n.ShouldReceiveDown().To(w).About(sup).Since(mk).None().Assert()
	n.ShouldSend().From(sup).Message(childStarted{Name: "c0"}).Since(mk).None().Assert()

	// c0 is disabled (StartChild rejected); c1 still runs
	check.Equal(t, gen.PID{}, intensityChildPID(t, n, sup, "c0"))
	check.True(t, intensityChildPID(t, n, sup, "c1") != gen.PID{})
	sv, err := n.Call(sup, startReq{Name: "c0"})
	check.NoError(t, err)
	check.Equal(t, act.ErrSupervisorChildDisabled.Error(), sv)

	// EnableChild brings c0 back
	mk2 := n.Mark()
	ev, err := n.Call(sup, enableReq{Name: "c0"})
	check.NoError(t, err)
	check.Equal(t, "", ev)
	n.ShouldSend().From(sup).Message(childStarted{Name: "c0"}).Since(mk2).Once().Within(time.Second).Must()
}
