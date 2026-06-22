package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// startArg asks the supervisor to start a worker carrying Arg.
type startArg struct{ Arg string }

// sofoArgsSup is a SimpleOneForOne supervisor that starts workers with args.
type sofoArgsSup struct{ act.Supervisor }

func factorySofoArgsSup() gen.ProcessBehavior { return &sofoArgsSup{} }

func (s *sofoArgsSup) Init(args ...any) (act.SupervisorSpec, error) {
	return act.SupervisorSpec{
		Type:     act.SupervisorTypeSimpleOneForOne,
		Children: []act.SupervisorChildSpec{{Name: "worker", Factory: factorySofoArgsWorker}},
		Restart:  act.SupervisorRestart{Strategy: act.SupervisorStrategyPermanent, Intensity: 10, Period: 5},
	}, nil
}

func (s *sofoArgsSup) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch c := request.(type) {
	case startArg:
		return "ok", s.StartChild("worker", c.Arg)
	case string:
		if c == "children" {
			ch := s.Children()
			out := make([]childInfo, len(ch))
			for i, x := range ch {
				out[i] = childInfo{Name: x.Spec, PID: x.PID}
			}
			return out, nil
		}
	}
	return "ok", nil
}

// sofoArgsWorker stores its spawn arg and reports it on request.
type sofoArgsWorker struct {
	act.Actor
	arg string
}

func factorySofoArgsWorker() gen.ProcessBehavior { return &sofoArgsWorker{} }

func (w *sofoArgsWorker) Init(args ...any) error {
	if len(args) > 0 {
		w.arg = args[0].(string)
	}
	return nil
}

func (w *sofoArgsWorker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return w.arg, nil
}

// TestLocalSupervisorSOFOArgs: a SimpleOneForOne child started with spawn args
// keeps those args when restarted (a new pid, the same arg value).
func TestLocalSupervisorSOFOArgs(t *testing.T) {
	const customArg = "test_value_123"

	s := stage.New(t)
	n := s.StartNode("n")
	sup := n.Spawn(factorySofoArgsSup, gen.ProcessOptions{})

	_, err := n.Call(sup, startArg{Arg: customArg})
	check.NoError(t, err)

	chAny, err := n.Call(sup, "children")
	check.NoError(t, err)
	ch := chAny.([]childInfo)
	check.Equal(t, 1, len(ch))
	child := ch[0].PID

	arg, err := n.Call(child, "getarg")
	check.NoError(t, err)
	check.Equal(t, customArg, arg)

	// kill the worker -> Permanent restart with the same args, a new pid
	mk := n.Mark()
	check.NoError(t, n.SendExit(child, gen.TerminateReasonKill))
	_, ok := n.ShouldSpawn().From(sup).Since(mk).Within(time.Second).Capture()
	check.True(t, ok)

	ch2Any, err := n.Call(sup, "children")
	check.NoError(t, err)
	ch2 := ch2Any.([]childInfo)
	check.Equal(t, 1, len(ch2))
	child2 := ch2[0].PID
	check.True(t, child != child2)

	arg2, err := n.Call(child2, "getarg")
	check.NoError(t, err)
	check.Equal(t, customArg, arg2)
}
