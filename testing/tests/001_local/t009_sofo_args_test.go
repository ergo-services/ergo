package local

import (
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type sofoArgsTestMsg struct {
	reply chan string
}

func TestSOFOArgsRestart(t *testing.T) {
	node, err := ergo.StartNode("node@localhost", gen.NodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Stop()

	// Start test actor to control the test flow
	testPID, err := node.Spawn(factory_test_sofo_args_main, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for test to complete
	reply := make(chan error, 1)
	node.Send(testPID, reply)

	select {
	case err := <-reply:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("test timeout")
	}
}

func factory_test_sofo_args_main() gen.ProcessBehavior {
	return &testSOFOArgsMain{}
}

type testSOFOArgsMain struct {
	act.Actor
}

func (a *testSOFOArgsMain) HandleMessage(from gen.PID, message any) error {
	reply := message.(chan error)
	defer close(reply)

	// Start supervisor
	supPID, err := a.Spawn(factory_test_sofo_args_sup, gen.ProcessOptions{})
	if err != nil {
		reply <- err
		return nil
	}
	defer a.Node().Kill(supPID)

	// Start child with custom args
	customArg := "test_value_123"
	a.Send(supPID, []any{"start", customArg})
	time.Sleep(100 * time.Millisecond)

	// Get children
	children, err := a.Call(supPID, "children")
	if err != nil {
		reply <- err
		return nil
	}
	childList := children.([]act.SupervisorChild)
	if len(childList) != 1 {
		reply <- errIncorrect
		return nil
	}
	childPID := childList[0].PID

	// Verify child has the custom arg
	argReply := make(chan string, 1)
	a.Send(childPID, sofoArgsTestMsg{argReply})
	select {
	case arg := <-argReply:
		if arg != customArg {
			reply <- errIncorrect
			return nil
		}
	case <-time.After(1 * time.Second):
		reply <- errIncorrect
		return nil
	}

	// Kill child with abnormal reason to trigger restart
	a.SendExit(childPID, gen.TerminateReasonKill)

	// Wait for restart
	time.Sleep(200 * time.Millisecond)

	// Verify child restarted with same args
	children2, err := a.Call(supPID, "children")
	if err != nil {
		reply <- err
		return nil
	}
	childList2 := children2.([]act.SupervisorChild)
	if len(childList2) != 1 {
		reply <- errIncorrect
		return nil
	}
	childPID2 := childList2[0].PID

	if childPID == childPID2 {
		reply <- errIncorrect
		return nil
	}

	// Verify restarted child has the same custom arg
	argReply2 := make(chan string, 1)
	a.Send(childPID2, sofoArgsTestMsg{argReply2})
	select {
	case arg := <-argReply2:
		if arg != customArg {
			reply <- errIncorrect
			return nil
		}
	case <-time.After(1 * time.Second):
		reply <- errIncorrect
		return nil
	}

	reply <- nil
	return nil
}

func factory_test_sofo_args_sup() gen.ProcessBehavior {
	return &testSOFOArgsSup{}
}

type testSOFOArgsSup struct {
	act.Supervisor
}

func (s *testSOFOArgsSup) Init(args ...any) (act.SupervisorSpec, error) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeSimpleOneForOne,
		Children: []act.SupervisorChildSpec{
			{
				Name:    "worker",
				Factory: factory_test_sofo_args_worker,
			},
		},
	}
	spec.Restart.Strategy = act.SupervisorStrategyPermanent
	spec.Restart.Intensity = 10
	spec.Restart.Period = 5
	return spec, nil
}

func (s *testSOFOArgsSup) HandleMessage(from gen.PID, message any) error {
	if args, ok := message.([]any); ok {
		if args[0] == "start" {
			return s.StartChild("worker", args[1])
		}
	}
	return nil
}

func (s *testSOFOArgsSup) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "children" {
		return s.Children(), nil
	}
	return nil, nil
}

func factory_test_sofo_args_worker() gen.ProcessBehavior {
	return &testSOFOArgsWorker{}
}

type testSOFOArgsWorker struct {
	act.Actor
	arg string
}

func (w *testSOFOArgsWorker) Init(args ...any) error {
	if len(args) > 0 {
		w.arg = args[0].(string)
	}
	return nil
}

func (w *testSOFOArgsWorker) HandleMessage(from gen.PID, message any) error {
	if msg, ok := message.(sofoArgsTestMsg); ok {
		msg.reply <- w.arg
		close(msg.reply)
	}
	return nil
}
