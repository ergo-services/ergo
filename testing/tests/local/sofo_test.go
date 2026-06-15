package local

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// sofoBasicSup is a SimpleOneForOne supervisor used to exercise the child-spec API.
type sofoBasicSup struct{ act.Supervisor }

func factorySofoBasicSup() gen.ProcessBehavior { return &sofoBasicSup{} }

func (s *sofoBasicSup) Init(args ...any) (act.SupervisorSpec, error) {
	return act.SupervisorSpec{
		Type:     act.SupervisorTypeSimpleOneForOne,
		Children: []act.SupervisorChildSpec{{Name: "child", Factory: factoryEcho}},
	}, nil
}

func (s *sofoBasicSup) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return errText(s.basicCheck()), nil
}

func (s *sofoBasicSup) basicCheck() error {
	if err := s.StartChild("child"); err != nil {
		return err
	}
	c0 := s.Children()
	if len(c0) != 1 {
		return fmt.Errorf("children != 1")
	}
	if err := s.StartChild("nope"); err != act.ErrSupervisorChildUnknown {
		return fmt.Errorf("StartChild(unknown): %v", err)
	}
	if reflect.DeepEqual(c0, s.Children()) == false {
		return fmt.Errorf("children changed after unknown StartChild")
	}
	if err := s.AddChild(act.SupervisorChildSpec{Name: "child1", Factory: factoryEcho}); err != nil {
		return err
	}
	if reflect.DeepEqual(c0, s.Children()) == false {
		return fmt.Errorf("AddChild started a child")
	}
	if err := s.StartChild("child1"); err != nil {
		return err
	}
	c3 := s.Children()
	if len(c3) != 2 {
		return fmt.Errorf("children != 2")
	}
	if reflect.DeepEqual(c3[0], c0[0]) == false {
		return fmt.Errorf("first child changed")
	}
	if err := s.DisableChild("nope"); err != act.ErrSupervisorChildUnknown {
		return fmt.Errorf("DisableChild(unknown): %v", err)
	}
	if err := s.DisableChild("child1"); err != nil {
		return err
	}
	if err := s.StartChild("child1"); err != act.ErrSupervisorChildDisabled {
		return fmt.Errorf("StartChild(disabled): %v", err)
	}
	if err := s.EnableChild("child1"); err != nil {
		return fmt.Errorf("EnableChild: %v", err)
	}
	if err := s.StartChild("child1"); err != nil {
		return fmt.Errorf("StartChild after enable: %v", err)
	}
	return nil
}

// childStarted / childStopped are markers the supervisor emits from its
// child-lifecycle callbacks (carrying name / pid+reason) so the test verifies
// each terminate and restart exactly, deterministically.
type childStarted struct{ Name gen.Atom }
type childStopped struct {
	PID    gen.PID
	Reason error
}

// sofoSup is a SimpleOneForOne supervisor with a configurable restart strategy.
type sofoSup struct{ act.Supervisor }

func factorySofoSup() gen.ProcessBehavior { return &sofoSup{} }

func (s *sofoSup) Init(args ...any) (act.SupervisorSpec, error) {
	return act.SupervisorSpec{
		Type:              act.SupervisorTypeSimpleOneForOne,
		Children:          []act.SupervisorChildSpec{{Name: "child", Factory: factoryEcho}},
		Restart:           act.SupervisorRestart{Strategy: args[0].(act.SupervisorStrategy)},
		EnableHandleChild: true,
	}, nil
}

func (s *sofoSup) HandleChildStart(name gen.Atom, pid gen.PID) error {
	return s.Send(s.PID(), childStarted{Name: name})
}

func (s *sofoSup) HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error {
	return s.Send(s.PID(), childStopped{PID: pid, Reason: reason})
}

func (s *sofoSup) HandleMessage(from gen.PID, message any) error { return nil }

func (s *sofoSup) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "start":
		if err := s.StartChild("child"); err != nil {
			return nil, err
		}
		ch := s.Children()
		return ch[len(ch)-1].PID, nil
	case "children":
		ch := s.Children()
		pids := make([]gen.PID, len(ch))
		for i, c := range ch {
			pids[i] = c.PID
		}
		return pids, nil
	}
	return "ok", nil
}

// TestLocalSupervisorSOFOBasic: the SimpleOneForOne child-spec API: StartChild,
// AddChild, DisableChild, Children, with ErrSupervisorChildUnknown and
// ErrSupervisorChildDisabled for the error paths.
func TestLocalSupervisorSOFOBasic(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	sup := n.Spawn(factorySofoBasicSup, gen.ProcessOptions{})
	v, err := n.Call(sup, "check")
	check.NoError(t, err)
	check.Equal(t, "", v)
}

// TestLocalSupervisorSOFOStrategy: a SimpleOneForOne supervisor with five children;
// terminating the first child with each reason fires HandleChildTerminate with the
// exact pid and reason, and a restart (HandleChildStart for the spec) happens per
// strategy and reason: Permanent always, Temporary never, Transient only on
// abnormal termination (Kill). Verified deterministically (a synchronous barrier
// guarantees the supervisor finished handling the termination before asserting).
func TestLocalSupervisorSOFOStrategy(t *testing.T) {
	normal, shutdown, kill := gen.TerminateReasonNormal, gen.TerminateReasonShutdown, gen.TerminateReasonKill

	cases := []struct {
		strategy act.SupervisorStrategy
		restart  map[error]bool
	}{
		{act.SupervisorStrategyPermanent, map[error]bool{normal: true, shutdown: true, kill: true}},
		{act.SupervisorStrategyTemporary, map[error]bool{normal: false, shutdown: false, kill: false}},
		{act.SupervisorStrategyTransient, map[error]bool{normal: false, shutdown: false, kill: true}},
	}

	for _, c := range cases {
		s := stage.New(t)
		n := s.Node("n")
		sup := n.Spawn(factorySofoSup, gen.ProcessOptions{}, c.strategy)

		// start five children
		for i := 0; i < 5; i++ {
			_, err := n.Call(sup, "start")
			check.NoError(t, err)
			n.ShouldSend().From(sup).Message(childStarted{Name: "child"}).
				Times(i + 1).Within(time.Second).Must()
		}
		childrenAny, err := n.Call(sup, "children")
		check.NoError(t, err)
		check.Equal(t, 5, len(childrenAny.([]gen.PID)))

		for _, reason := range []error{normal, shutdown, kill} {
			// the current first child is the victim
			cur, err := n.Call(sup, "children")
			check.NoError(t, err)
			victim := cur.([]gen.PID)[0]

			mk := n.Mark()
			check.NoError(t, n.SendExit(victim, reason))

			// HandleChildTerminate fires with the exact pid and reason
			n.ShouldSend().From(sup).Message(childStopped{PID: victim, Reason: reason}).
				Since(mk).Once().Within(time.Second).Must()

			// barrier: once this returns, the supervisor finished the termination
			// (including the restart, if any) and moved on to this call
			_, err = n.Call(sup, "ping")
			check.NoError(t, err)

			// restart happens per strategy/reason, asserted with no time window
			if c.restart[reason] {
				n.ShouldSend().From(sup).Message(childStarted{Name: "child"}).
					Since(mk).Once().Assert()
			} else {
				n.ShouldSend().From(sup).Message(childStarted{Name: "child"}).
					Since(mk).None().Assert()
			}
		}
	}
}

// TestLocalSupervisorSOFOExit: an exit signal sent to the supervisor terminates it.
func TestLocalSupervisorSOFOExit(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	w := n.Spawn(factoryWatcher, gen.ProcessOptions{})
	sup := n.Spawn(factorySofoSup, gen.ProcessOptions{}, act.SupervisorStrategyPermanent)

	n.Send(w, monitorCmd{Target: sup})
	n.ShouldMonitor().From(w).Target(sup).Once().Within(time.Second).Must()

	myExit := errors.New("my exit")
	mk := n.Mark()
	check.NoError(t, n.SendExit(sup, myExit))
	n.ShouldReceiveDown().To(w).About(sup).ReasonIs(myExit).Since(mk).Once().Within(time.Second).Must()
}
