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

// arfoSup is an All/Rest-For-One supervisor (type+strategy from args) with three
// ordered children, reporting child lifecycle via markers.
type arfoSup struct{ act.Supervisor }

func factoryArfoSup() gen.ProcessBehavior { return &arfoSup{} }

func (s *arfoSup) Init(args ...any) (act.SupervisorSpec, error) {
	return act.SupervisorSpec{
		Type: args[0].(act.SupervisorType),
		Children: []act.SupervisorChildSpec{
			{Name: "c0", Factory: factoryEcho},
			{Name: "c1", Factory: factoryEcho},
			{Name: "c2", Factory: factoryEcho},
		},
		Restart:             act.SupervisorRestart{Strategy: args[1].(act.SupervisorStrategy), KeepOrder: true},
		EnableHandleChild:   true,
		DisableAutoShutdown: true,
	}, nil
}

func (s *arfoSup) HandleChildStart(name gen.Atom, pid gen.PID) error {
	return s.Send(s.PID(), childStarted{Name: name})
}
func (s *arfoSup) HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error {
	return s.Send(s.PID(), childStopped{PID: pid, Reason: reason})
}
func (s *arfoSup) HandleMessage(from gen.PID, message any) error { return nil }

type disableReq struct{ Name gen.Atom }

func (s *arfoSup) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch r := request.(type) {
	case string:
		if r == "children" {
			ch := s.Children()
			out := make([]childInfo, len(ch))
			for i, c := range ch {
				out[i] = childInfo{Name: c.Spec, PID: c.PID}
			}
			return out, nil
		}
	case disableReq:
		return errText(s.DisableChild(r.Name)), nil
	case enableReq:
		return errText(s.EnableChild(r.Name)), nil
	}
	return "ok", nil
}

func arfoChildPID(t *testing.T, n *stage.Node, sup gen.PID, name gen.Atom) gen.PID {
	t.Helper()
	v, err := n.Call(sup, "children")
	check.NoError(t, err)
	for _, c := range v.([]childInfo) {
		if c.Name == name {
			return c.PID
		}
	}
	return gen.PID{}
}

// TestLocalSupervisorARFOEnable: for AllForOne and RestForOne, DisableChild stops
// the named child (the supervisor and the others survive); EnableChild re-enables
// it and the supervisor spawns it again with a fresh pid.
func TestLocalSupervisorARFOEnable(t *testing.T) {
	for _, typ := range []act.SupervisorType{act.SupervisorTypeAllForOne, act.SupervisorTypeRestForOne} {
		s := stage.New(t)
		n := s.StartNode("n")
		sup := n.Spawn(factoryArfoSup, gen.ProcessOptions{}, typ, act.SupervisorStrategyPermanent)

		n.ShouldSend().From(sup).Message(childStarted{Name: "c0"}).AtLeast(1).Within(time.Second).Must()
		c0 := arfoChildPID(t, n, sup, "c0")
		check.True(t, c0 != gen.PID{})

		// disable c0 -> it terminates (barrier: wait for that childStopped)
		mk := n.Mark()
		dv, err := n.Call(sup, disableReq{Name: "c0"})
		check.NoError(t, err)
		check.Equal(t, "", dv)
		n.ShouldSend().From(sup).Where(func(r check.Send) bool {
			cs, ok := r.Message.(childStopped)
			return ok && cs.PID == c0
		}).Since(mk).Once().Within(time.Second).Must()

		// enable c0 -> the supervisor spawns it again with a new pid
		mk2 := n.Mark()
		ev, err := n.Call(sup, enableReq{Name: "c0"})
		check.NoError(t, err)
		check.Equal(t, "", ev)
		n.ShouldSend().From(sup).Message(childStarted{Name: "c0"}).Since(mk2).Once().Within(time.Second).Must()
		c0b := arfoChildPID(t, n, sup, "c0")
		check.True(t, c0b != gen.PID{} && c0b != c0)
	}
}

func isStop(r check.Send) bool  { _, ok := r.Message.(childStopped); return ok }
func isStart(r check.Send) bool { _, ok := r.Message.(childStarted); return ok }

// runArfo kills children[idx] with reason and verifies the cascade: stopCount
// children terminate and startCount restart (0 if no restart), the killed child's
// terminate carries the exact pid/reason, and survivors keep their pids.
func runArfo(t *testing.T, supType act.SupervisorType, strategy act.SupervisorStrategy, idx int, reason error, stopCount, startCount int, survivors []int) {
	t.Helper()
	s := stage.New(t)
	n := s.StartNode("n")
	sup := n.Spawn(factoryArfoSup, gen.ProcessOptions{}, supType, strategy)

	n.ShouldSend().From(sup).Message(childStarted{Name: "c0"}).Once().Within(time.Second).Must()
	curAny, err := n.Call(sup, "children")
	check.NoError(t, err)
	before := curAny.([]childInfo)
	check.Equal(t, 3, len(before))
	victim := before[idx]

	mk := n.Mark()
	check.NoError(t, n.SendExit(victim.PID, reason))

	// the killed child terminates with the exact pid and reason
	n.ShouldSend().From(sup).Message(childStopped{PID: victim.PID, Reason: reason}).
		Since(mk).Once().Within(time.Second).Must()
	// barrier: the supervisor finished the whole cascade
	_, err = n.Call(sup, "ping")
	check.NoError(t, err)

	// exactly stopCount terminations and startCount restarts, no window
	n.ShouldSend().From(sup).Where(isStop).Since(mk).Times(stopCount).Assert()
	n.ShouldSend().From(sup).Where(isStart).Since(mk).Times(startCount).Assert()

	// survivors keep their original pids (not restarted)
	afterAny, err := n.Call(sup, "children")
	check.NoError(t, err)
	after := afterAny.([]childInfo)
	for _, si := range survivors {
		check.Equal(t, before[si].PID, after[si].PID)
	}
}

// TestLocalSupervisorAFOStrategy: AllForOne restarts ALL children when one
// terminates and the strategy restarts: Permanent always, Transient on abnormal
// (Kill), Temporary never (only the terminated child stops).
func TestLocalSupervisorAFOStrategy(t *testing.T) {
	afo := act.SupervisorTypeAllForOne
	kill := gen.TerminateReasonKill
	normal := gen.TerminateReasonNormal

	// Permanent: kill the middle child -> all three stop and restart
	runArfo(t, afo, act.SupervisorStrategyPermanent, 1, kill, 3, 3, nil)
	// Transient + Kill (abnormal) -> all restart
	runArfo(t, afo, act.SupervisorStrategyTransient, 1, kill, 3, 3, nil)
	// Transient + Normal -> no restart, only the one child stops
	runArfo(t, afo, act.SupervisorStrategyTransient, 1, normal, 1, 0, []int{0, 2})
	// Temporary -> never restart, only the one child stops
	runArfo(t, afo, act.SupervisorStrategyTemporary, 1, kill, 1, 0, []int{0, 2})
}

// TestLocalSupervisorRFOStrategy: RestForOne restarts the terminated child and all
// children after it (not the ones before), per strategy.
func TestLocalSupervisorRFOStrategy(t *testing.T) {
	rfo := act.SupervisorTypeRestForOne
	kill := gen.TerminateReasonKill
	normal := gen.TerminateReasonNormal

	// Permanent: kill the middle child -> child1 and child2 restart, child0 survives
	runArfo(t, rfo, act.SupervisorStrategyPermanent, 1, kill, 2, 2, []int{0})
	// Transient + Kill -> rest restart
	runArfo(t, rfo, act.SupervisorStrategyTransient, 1, kill, 2, 2, []int{0})
	// Transient + Normal -> no restart
	runArfo(t, rfo, act.SupervisorStrategyTransient, 1, normal, 1, 0, []int{0, 2})
	// Temporary -> never restart
	runArfo(t, rfo, act.SupervisorStrategyTemporary, 1, kill, 1, 0, []int{0, 2})
}

// AFO/RFO basic: child-spec API (identical across these supervisor types)
type arfoBasicSup struct{ act.Supervisor }

func factoryArfoBasicSup() gen.ProcessBehavior { return &arfoBasicSup{} }

func (s *arfoBasicSup) Init(args ...any) (act.SupervisorSpec, error) {
	return act.SupervisorSpec{
		Type: args[0].(act.SupervisorType),
		Children: []act.SupervisorChildSpec{
			{Name: "c0", Factory: factoryEcho},
			{Name: "c1", Factory: factoryEcho},
			{Name: "c2", Factory: factoryEcho},
		},
	}, nil
}

func (s *arfoBasicSup) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return errText(s.basicCheck()), nil
}

func (s *arfoBasicSup) basicCheck() error {
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

// TestLocalSupervisorARFOBasic: the child-spec API for AllForOne and RestForOne.
func TestLocalSupervisorARFOBasic(t *testing.T) {
	for _, typ := range []act.SupervisorType{act.SupervisorTypeAllForOne, act.SupervisorTypeRestForOne} {
		s := stage.New(t)
		n := s.StartNode("n")
		sup := n.Spawn(factoryArfoBasicSup, gen.ProcessOptions{}, typ)
		v, err := n.Call(sup, "check")
		check.NoError(t, err)
		check.Equal(t, "", v)
	}
}

// AFO/RFO significant: a significant child's non-restart terminates the supervisor
type arfoSignificantSup struct{ act.Supervisor }

func factoryArfoSignificantSup() gen.ProcessBehavior { return &arfoSignificantSup{} }

func (s *arfoSignificantSup) Init(args ...any) (act.SupervisorSpec, error) {
	return act.SupervisorSpec{
		Type: args[0].(act.SupervisorType),
		Children: []act.SupervisorChildSpec{
			{Name: "c0", Factory: factoryEcho},
			{Name: "sig", Significant: true, Factory: factoryEcho},
		},
		Restart:             act.SupervisorRestart{Strategy: args[1].(act.SupervisorStrategy), KeepOrder: true},
		EnableHandleChild:   true,
		DisableAutoShutdown: true,
	}, nil
}

func (s *arfoSignificantSup) HandleChildStart(name gen.Atom, pid gen.PID) error {
	return s.Send(s.PID(), childStarted{Name: name})
}
func (s *arfoSignificantSup) HandleMessage(from gen.PID, message any) error { return nil }
func (s *arfoSignificantSup) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
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

// TestLocalSupervisorARFOSignificant: for AllForOne and RestForOne, terminating a
// significant child either restarts it (supervisor survives) or, when the strategy
// does not restart it, shuts the supervisor down with the child's reason.
func TestLocalSupervisorARFOSignificant(t *testing.T) {
	custom := errors.New("custom")
	normal := gen.TerminateReasonNormal

	cases := []struct {
		strategy act.SupervisorStrategy
		reason   error
		restart  bool
	}{
		{act.SupervisorStrategyTransient, custom, true},
		{act.SupervisorStrategyTransient, normal, false},
		{act.SupervisorStrategyTemporary, custom, false},
		{act.SupervisorStrategyPermanent, normal, true},
	}

	for _, typ := range []act.SupervisorType{act.SupervisorTypeAllForOne, act.SupervisorTypeRestForOne} {
		for _, c := range cases {
			s := stage.New(t)
			n := s.StartNode("n")
			w := n.Spawn(factoryWatcher, gen.ProcessOptions{})
			sup := n.Spawn(factoryArfoSignificantSup, gen.ProcessOptions{}, typ, c.strategy)

			n.Send(w, monitorCmd{Target: sup})
			n.ShouldMonitor().From(w).Target(sup).Once().Within(time.Second).Must()

			sigAny, err := n.Call(sup, "sig")
			check.NoError(t, err)
			sig := sigAny.(gen.PID)

			mk := n.Mark()
			check.NoError(t, n.SendExit(sig, c.reason))

			if c.restart {
				n.ShouldSend().From(sup).Message(childStarted{Name: "sig"}).Since(mk).Once().Within(time.Second).Must()
				_, err := n.Call(sup, "ping")
				check.NoError(t, err)
				n.ShouldReceiveDown().To(w).About(sup).Since(mk).None().Assert()
			} else {
				n.ShouldReceiveDown().To(w).About(sup).ReasonIs(c.reason).Since(mk).Once().Within(time.Second).Must()
			}
		}
	}
}
