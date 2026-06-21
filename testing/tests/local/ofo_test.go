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

// OFO basic: child-spec API
type ofoBasicSup struct{ act.Supervisor }

func factoryOfoBasicSup() gen.ProcessBehavior { return &ofoBasicSup{} }

func (s *ofoBasicSup) Init(args ...any) (act.SupervisorSpec, error) {
	return act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "c0", Factory: factoryEcho},
			{Name: "c1", Factory: factoryEcho},
			{Name: "c2", Factory: factoryEcho},
		},
	}, nil
}

func (s *ofoBasicSup) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return errText(s.basicCheck()), nil
}

func (s *ofoBasicSup) basicCheck() error {
	c := s.Children()
	if len(c) != 3 {
		return fmt.Errorf("children != 3 (%d)", len(c))
	}
	for _, x := range c {
		if x.Spec != x.Name {
			return fmt.Errorf("spec %q != name %q", x.Spec, x.Name)
		}
	}
	if err := s.StartChild("c0"); err != act.ErrSupervisorChildRunning {
		return fmt.Errorf("StartChild(running): %v", err)
	}
	if reflect.DeepEqual(c, s.Children()) == false {
		return fmt.Errorf("children changed after running StartChild")
	}
	if err := s.StartChild("nope"); err != act.ErrSupervisorChildUnknown {
		return fmt.Errorf("StartChild(unknown): %v", err)
	}
	if err := s.AddChild(act.SupervisorChildSpec{Name: "c3", Factory: factoryEcho}); err != nil {
		return err
	}
	c = s.Children()
	if len(c) != 4 {
		return fmt.Errorf("children != 4 after AddChild (%d)", len(c))
	}
	if c[3].Spec != c[3].Name {
		return fmt.Errorf("added child spec != name")
	}
	if err := s.StartChild("c3"); err != act.ErrSupervisorChildRunning {
		return fmt.Errorf("StartChild(c3 running): %v", err)
	}
	if err := s.DisableChild("nope"); err != act.ErrSupervisorChildUnknown {
		return fmt.Errorf("DisableChild(unknown): %v", err)
	}
	if err := s.DisableChild("c0"); err != nil {
		return err
	}
	disabled := false
	for _, x := range s.Children() {
		if x.Name == "c0" && x.Disabled {
			disabled = true
		}
	}
	if disabled == false {
		return fmt.Errorf("c0 not marked disabled")
	}
	if err := s.StartChild("c0"); err != act.ErrSupervisorChildDisabled {
		return fmt.Errorf("StartChild(disabled): %v", err)
	}
	return nil
}

// TestLocalSupervisorOFOBasic: the OneForOne child-spec API: children auto-start
// and carry registered names; StartChild on a running child is ErrSupervisorChildRunning,
// on an unknown spec ErrSupervisorChildUnknown; AddChild starts the new child;
// DisableChild disables it; starting a disabled child is ErrSupervisorChildDisabled.
func TestLocalSupervisorOFOBasic(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	sup := n.Spawn(factoryOfoBasicSup, gen.ProcessOptions{})
	v, err := n.Call(sup, "check")
	check.NoError(t, err)
	check.Equal(t, "", v)
}

// OFO strategy: one-for-one restart
type ofoSup struct{ act.Supervisor }

func factoryOfoSup() gen.ProcessBehavior { return &ofoSup{} }

func (s *ofoSup) Init(args ...any) (act.SupervisorSpec, error) {
	return act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "c0", Factory: factoryEcho},
			{Name: "c1", Factory: factoryEcho},
			{Name: "c2", Factory: factoryEcho},
		},
		Restart:             act.SupervisorRestart{Strategy: args[0].(act.SupervisorStrategy)},
		EnableHandleChild:   true,
		DisableAutoShutdown: true,
	}, nil
}

func (s *ofoSup) HandleChildStart(name gen.Atom, pid gen.PID) error {
	return s.Send(s.PID(), childStarted{Name: name})
}
func (s *ofoSup) HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error {
	return s.Send(s.PID(), childStopped{PID: pid, Reason: reason})
}
func (s *ofoSup) HandleMessage(from gen.PID, message any) error { return nil }
func (s *ofoSup) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "children" {
		ch := s.Children()
		out := make([]childInfo, len(ch))
		for i, c := range ch {
			out[i] = childInfo{Name: c.Spec, PID: c.PID}
		}
		return out, nil
	}
	return "ok", nil
}

// childInfo is a child's spec name + current pid, returned by a supervisor for tests.
type childInfo struct {
	Name gen.Atom
	PID  gen.PID
}

// TestLocalSupervisorOFOStrategy: OneForOne restarts only the terminated child,
// per strategy and reason; HandleChildTerminate carries the exact pid/reason and
// HandleChildStart the spec name. Deterministic (synchronous barrier, no windows).
func TestLocalSupervisorOFOStrategy(t *testing.T) {
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
		n := s.StartNode("n")
		sup := n.Spawn(factoryOfoSup, gen.ProcessOptions{}, c.strategy)

		// three children auto-started
		n.ShouldSend().From(sup).Message(childStarted{Name: "c0"}).Once().Within(time.Second).Must()
		ch, err := n.Call(sup, "children")
		check.NoError(t, err)
		check.Equal(t, 3, len(ch.([]childInfo)))

		for _, reason := range []error{normal, shutdown, kill} {
			cur, err := n.Call(sup, "children")
			check.NoError(t, err)
			var victim childInfo
			for _, ci := range cur.([]childInfo) {
				if ci.PID != (gen.PID{}) {
					victim = ci
					break
				}
			}
			check.True(t, victim.PID != gen.PID{})

			mk := n.Mark()
			check.NoError(t, n.SendExit(victim.PID, reason))
			// exactly the terminated child reported, with the exact pid and reason
			n.ShouldSend().From(sup).Message(childStopped{PID: victim.PID, Reason: reason}).
				Since(mk).Once().Within(time.Second).Must()
			// barrier: the supervisor finished handling the termination (and restart, if any)
			_, err = n.Call(sup, "ping")
			check.NoError(t, err)
			// only the terminated child restarts, and only per strategy/reason
			if c.restart[reason] {
				n.ShouldSend().From(sup).Message(childStarted{Name: victim.Name}).Since(mk).Once().Assert()
			} else {
				n.ShouldSend().From(sup).Message(childStarted{Name: victim.Name}).Since(mk).None().Assert()
			}
		}
	}
}

// OFO significant: a significant child's non-restart terminates the supervisor
type ofoSignificantSup struct{ act.Supervisor }

func factoryOfoSignificantSup() gen.ProcessBehavior { return &ofoSignificantSup{} }

func (s *ofoSignificantSup) Init(args ...any) (act.SupervisorSpec, error) {
	return act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "c0", Factory: factoryEcho},
			{Name: "sig", Significant: true, Factory: factoryEcho},
		},
		Restart:           act.SupervisorRestart{Strategy: args[0].(act.SupervisorStrategy)},
		EnableHandleChild: true,
	}, nil
}

func (s *ofoSignificantSup) HandleChildStart(name gen.Atom, pid gen.PID) error {
	return s.Send(s.PID(), childStarted{Name: name})
}
func (s *ofoSignificantSup) HandleMessage(from gen.PID, message any) error { return nil }
func (s *ofoSignificantSup) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "sig" {
		for _, c := range s.Children() {
			if c.Significant {
				return c.PID, nil
			}
		}
		return gen.PID{}, nil
	}
	return "ok", nil
}

// TestLocalSupervisorOFOSignificant: terminating a significant child either gets
// it restarted (supervisor survives) or, when the strategy does not restart it,
// shuts down the whole supervisor with the child's termination reason.
func TestLocalSupervisorOFOSignificant(t *testing.T) {
	custom := errors.New("custom reason")
	normal := gen.TerminateReasonNormal

	// restart=true -> supervisor survives; restart=false -> supervisor terminates with reason
	cases := []struct {
		strategy act.SupervisorStrategy
		reason   error
		restart  bool
	}{
		{act.SupervisorStrategyTransient, custom, true},  // abnormal -> restart
		{act.SupervisorStrategyTransient, normal, false}, // normal -> shutdown supervisor
		{act.SupervisorStrategyTemporary, custom, false}, // never restart -> shutdown
		{act.SupervisorStrategyPermanent, normal, true},  // always restart -> survive
	}

	for _, c := range cases {
		s := stage.New(t)
		n := s.StartNode("n")
		w := n.Spawn(factoryWatcher, gen.ProcessOptions{})
		sup := n.Spawn(factoryOfoSignificantSup, gen.ProcessOptions{}, c.strategy)

		n.Send(w, monitorCmd{Target: sup})
		n.ShouldMonitor().From(w).Target(sup).Once().Within(time.Second).Must()

		sigAny, err := n.Call(sup, "sig")
		check.NoError(t, err)
		sig := sigAny.(gen.PID)

		mk := n.Mark()
		check.NoError(t, n.SendExit(sig, c.reason))

		if c.restart {
			// the significant child is restarted; the supervisor stays alive
			n.ShouldSend().From(sup).Message(childStarted{Name: "sig"}).Since(mk).Once().Within(time.Second).Must()
			_, err := n.Call(sup, "ping")
			check.NoError(t, err)
			n.ShouldReceiveDown().To(w).About(sup).Since(mk).None().Assert()
		} else {
			// the supervisor shuts down with the significant child's reason
			n.ShouldReceiveDown().To(w).About(sup).ReasonIs(c.reason).Since(mk).Once().Within(time.Second).Must()
		}
	}
}
