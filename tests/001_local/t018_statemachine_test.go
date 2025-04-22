package local

import (
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

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

// Test state transitions with messages and calls
func factory_t18_state_transitions() gen.ProcessBehavior {
	return &t18_state_transitions{}
}

type t18_state_transitions struct {
	act.StateMachine[t18_state_transitions_data]
}

type t18_state_transitions_data struct {
	transitions int
}

type t18_state2 struct {
}

type t18_get_transitions struct {
}

func (sm *t18_state_transitions) Init(args ...any) (act.StateMachineSpec[t18_state_transitions_data], error) {
	spec := act.NewStateMachineSpec(gen.Atom("state1"),
		// initial data
		act.WithData(t18_state_transitions_data{}),

		// set up a message handler for the transition [state1] -> [state2]
		act.WithStateMessageHandler(gen.Atom("state1"), t18_move_to_state2),

		// set up a call handler to query the number of state transitions
		act.WithStateCallHandler(gen.Atom("state2"), t18_total_transitions),
	)

	return spec, nil
}

func t18_move_to_state2(state gen.Atom, data t18_state_transitions_data, message t18_state2, proc gen.Process) (gen.Atom, t18_state_transitions_data, error) {
	data.transitions++
	return gen.Atom("state2"), data, nil
}

func t18_total_transitions(state gen.Atom, data t18_state_transitions_data, message t18_get_transitions, proc gen.Process) (gen.Atom, t18_state_transitions_data, int, error) {
	return state, data, data.transitions, nil
}

func (t *t18) TestStateMachine(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t18_state_transitions, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn statemachine process: %s", err)
		t.testcase.err <- err
		return
	}

	// Send a message to transition to state 2.
	err = t.Send(pid, t18_state2{})
	if err != nil {
		t.Log().Error("send 't18_state2' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// Query the data from the state machine (and test StateCallHandler behavior)
	result, err := t.Call(pid, t18_get_transitions{})
	if err != nil {
		t.Log().Error("call 't18_get_transitions' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// We expect 1 state transitions: [state1] -> [state2]
	data := result.(int)
	if data != 1 {
		t.testcase.err <- fmt.Errorf("expected 1 state transitions, got %v", result)
		return
	}

	// Statemachine process should crash on invalid state transition
	err = t.testcase.expectProcessToTerminate(pid, t, func(p gen.Process) error {
		return p.Send(pid, t18_state2{}) // we are in state2
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
