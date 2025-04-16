package local

import (
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

//
// this is the template for writing new tests
//

var (
	t18cases []*testcase
)

func factory_t18() gen.ProcessBehavior {
	return &t18{}
}

type t18 struct {
	act.Actor

	testcase *testcase
}

func (t *t18) HandleMessage(from gen.PID, message any) error {
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

func factory_t18statemachine() gen.ProcessBehavior {
	return &t18statemachine{}
}

type t18statemachine struct {
	act.StateMachine[t18data]
	tc *testcase
}

type t18data struct {
	count int
}

type t18transitionState1toState2 struct {
}

type t18transitionState2toState1 struct {
}

func (sm *t18statemachine) Init(args ...any) (act.StateMachineSpec[t18data], error) {
	spec := act.NewStateMachineSpec(gen.Atom("state1"),
		act.WithData(t18data{count: 1}),
		act.WithStateMessageHandler(gen.Atom("state1"), state1to2),
		act.WithStateCallHandler(gen.Atom("state2"), state2to1),
	)

	return spec, nil
}

func state1to2(sm *act.StateMachine[t18data], message t18transitionState1toState2) error {
	sm.SetCurrentState(gen.Atom("state2"))
	data := sm.Data()
	data.count++
	sm.SetData(data)
	return nil
}

func state2to1(sm *act.StateMachine[t18data], message t18transitionState2toState1) (int, error) {
	sm.SetCurrentState(gen.Atom("state1"))
	data := sm.Data()
	data.count++
	sm.SetData(data)
	return data.count, nil
}

func (t *t18) TestStateMachine(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t18statemachine, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn statemachine process: %s", err)
		t.testcase.err <- err
		return
	}

	// send message to transition from state 1 to 2
	err = t.Send(pid, t18transitionState1toState2{})

	if err != nil {
		t.Log().Error("sending to the statemachine process failed: %s", err)
		t.testcase.err <- err
		return
	}

	// send call to transition from result 2 to 1 (not working yet)
	//  result, err := t.Call(pid, t18transitionState2toState1{})
	//  if err != nil {
	//      t.Log().Error("call to the statemachine process failed: %s", err)
	//      t.testcase.err <- err
	//      return
	//  }
	//  if result != 3 {
	//      t.testcase.err <- fmt.Errorf("expected 3, got %v", result)
	//      return
	//  }

	// statemachine process should crash on invalid state transition
	err = t.testcase.expectProcessToTerminate(pid, t, func(p gen.Process) error {
		return p.Send(pid, t18transitionState2toState1{}) // we are in state1
	})
	if err != nil {
		t.testcase.err <- err
		return
	}

	t.testcase.err <- nil
}

func TestTt18template(t *testing.T) {
	nopt := gen.NodeOptions{}
	//nopt.Log.DefaultLogger.Disable = true
	//nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t18node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t18, popt)
	if err != nil {
		panic(err)
	}

	t18cases = []*testcase{
		{"TestStateMachine", nil, nil, make(chan error)},
	}
	for _, tc := range t18cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(3); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
