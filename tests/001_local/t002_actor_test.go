package local

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

var (
	t2cases []*testcase
)

func factory_t2() gen.ProcessBehavior {
	return &t2{}
}

type t2 struct {
	act.Actor

	testcase *testcase
}

func (t *t2) Init(args ...any) error {
	return nil
}

func (t *t2) HandleMessage(from gen.PID, message any) error {
	if t.testcase == nil {
		t.testcase = message.(*testcase)
		message = initcase{}
	}
	// get method by name
	method := reflect.ValueOf(t).MethodByName(t.testcase.name)
	if method.IsValid() == false {
		t.testcase.err <- fmt.Errorf("unknown method %q", t.testcase.name)
		t.testcase = nil
		return nil
	}
	method.Call([]reflect.Value{reflect.ValueOf(message)})
	return nil
}

func (t *t2) Terminate(reason error) {
	tc := t.testcase
	if tc == nil {
		return
	}
	tc.output = reason
	tc.err <- nil
}

//
// test methods
//

func (t *t2) TestTrapExit(input any) {
	defer func() {
		t.testcase = nil
	}()

	// set/unset trap exit
	if t.TrapExit() != false {
		t.testcase.err <- errIncorrect
		return
	}
	t.SetTrapExit(true)
	if t.TrapExit() != true {
		t.testcase.err <- errIncorrect
		return
	}
	t.SetTrapExit(false)
	if t.TrapExit() != false {
		t.testcase.err <- errIncorrect
		return
	}

	// spawn a child to check this flag in action
	pid, err := t.Spawn(factory_t2, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}
	defer t.Node().Kill(pid)

	targetTC := &testcase{"TargetTrapExit", nil, nil, make(chan error)}
	t.Send(pid, targetTC)
	if err := targetTC.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	// spawn the other process for sending gen.TerminateReasonShutdown
	alien, err := t.Spawn(factory_t2, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}
	defer t.Node().Kill(alien)

	alienTC := &testcase{"Alien", pid, nil, make(chan error)}
	t.Send(alien, alienTC)
	if err := alienTC.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	// target must receive gen.MessageExitPID with gen.TerminateReasonShutdown as a reason
	if err := targetTC.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	m := targetTC.output.(gen.MessageExitPID)
	if m.Reason != gen.TerminateReasonShutdown {
		t.testcase.err <- errIncorrect
		return
	}
	if m.PID != alien {
		t.testcase.err <- errIncorrect
		return
	}

	// send "shutdown" by parent. child process should be terminated
	// invoking Terminate callback
	t.SendExit(pid, gen.TerminateReasonShutdown)
	if err := targetTC.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	reason := errors.Unwrap(targetTC.output.(error))
	if reason != gen.TerminateReasonShutdown {
		t.testcase.err <- errIncorrect
		return
	}

	t.testcase.err <- nil
}

func (t *t2) TargetTrapExit(input any) {
	if _, ok := input.(initcase); ok {
		t.SetTrapExit(true)
		t.testcase.err <- nil
		return
	}

	// got message
	t.testcase.output = input
	t.testcase.err <- nil
}

func (t *t2) Alien(input any) {
	if _, ok := input.(initcase); ok {
		pid := t.testcase.input.(gen.PID)
		t.SendExit(pid, gen.TerminateReasonShutdown)
		t.testcase.err <- nil
		return
	}
	panic(input)
}

func TestT2Actor(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	node, err := ergo.StartNode("t2node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t2, popt)
	if err != nil {
		panic(err)
	}

	t2cases = []*testcase{
		{"TestTrapExit", nil, nil, make(chan error)},
	}
	for _, tc := range t2cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
