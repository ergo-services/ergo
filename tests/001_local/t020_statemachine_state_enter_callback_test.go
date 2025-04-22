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
	t20cases []*testcase
)

func factory_t20() gen.ProcessBehavior {
	return &t20{}
}

type t20 struct {
	act.Actor

	testcase *testcase
}

func (t *t20) HandleMessage(from gen.PID, message any) error {
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

func factory_t20_state_enter_callback() gen.ProcessBehavior {
	return &t20_state_enter_callback{}
}

type t20_state_enter_callback struct {
	act.StateMachine[t20_state_enter_callback_data]
	tc *testcase
}

type t20_state_enter_callback_data struct {
	callbackCount int
	currentState  gen.Atom
}

type t20_state_enter_callback_query struct {
}

type t20_state2 struct{}

func (sm *t20_state_enter_callback) Init(args ...any) (act.StateMachineSpec[t20_state_enter_callback_data], error) {
	spec := act.NewStateMachineSpec(gen.Atom("state1"),
		// initial data
		act.WithData(t20_state_enter_callback_data{currentState: gen.Atom("state1")}),

		// with message handler for [state1] -> [state2]
		act.WithStateMessageHandler(gen.Atom("state1"), t20_move_to_state2),

		// with call handler to query the data
		act.WithStateCallHandler(gen.Atom("state1"), t20_state_and_callback_count),
		act.WithStateCallHandler(gen.Atom("state3"), t20_state_and_callback_count),

		// with state enter callback
		act.WithStateEnterCallback(t20_state_enter),
	)

	return spec, nil
}

func t20_move_to_state2(state gen.Atom, data t20_state_enter_callback_data, message t20_state2, proc gen.Process) (gen.Atom, t20_state_enter_callback_data, error) {
	return gen.Atom("state2"), data, nil
}

func t20_state_and_callback_count(state gen.Atom, data t20_state_enter_callback_data, message t20_state_enter_callback_query, proc gen.Process) (gen.Atom, t20_state_enter_callback_data, t20_state_enter_callback_data, error) {
	return state, data, data, nil
}

func t20_state_enter(oldState gen.Atom, newState gen.Atom, data t20_state_enter_callback_data, proc gen.Process) (gen.Atom, t20_state_enter_callback_data, error) {
	data.callbackCount++
	data.currentState = newState
	if newState == gen.Atom("state2") {
		return gen.Atom("state3"), data, nil
	}
	return newState, data, nil
}

func (t *t20) TestStateEnterCallback(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t20_state_enter_callback, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn statemachine process: %s", err)
		t.testcase.err <- err
		return
	}

	// Ensure callback count is 0 and we are in state1 prior to the test
	result, err := t.Call(pid, t20_state_enter_callback_query{})
	if err != nil {
		t.Log().Error("call 't20_state_enter_callback_query' failed: %s", err)
		t.testcase.err <- err
		return
	}
	data := result.(t20_state_enter_callback_data)
	if data.callbackCount != 0 {
		t.testcase.err <- fmt.Errorf("expected 0 callback invocations, got %v", data.callbackCount)
		return
	}
	if data.currentState != gen.Atom("state1") {
		t.testcase.err <- fmt.Errorf("expected current state to be state1, got %v", data.currentState)
		return
	}

	// Send a message to transition to state 2. The state enter callback should
	// automatically transition to state 3. The subsequent state enter callback
	// does not trigger any further state transitions.
	err = t.Send(pid, t20_state2{})
	if err != nil {
		t.Log().Error("send 'state2' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// Ensure callback count is 2 and we are in state3 prior to the test
	result, err = t.Call(pid, t20_state_enter_callback_query{})
	if err != nil {
		t.Log().Error("call 't20_state_enter_callback_query' failed: %s", err)
		t.testcase.err <- err
		return
	}
	data = result.(t20_state_enter_callback_data)
	if data.callbackCount != 2 {
		t.testcase.err <- fmt.Errorf("expected 2 callback invocations, got %v", data.callbackCount)
		return
	}
	if data.currentState != gen.Atom("state3") {
		t.testcase.err <- fmt.Errorf("expected current state to be state3, got %v", data.currentState)
		return
	}

	t.testcase.err <- nil
}

func (t *t20) TestFeature1(input any) {
	defer func() {
		t.testcase = nil
	}()

	t.testcase.err <- nil
}

func (t *t20) TestFeatureX(input any) {
	defer func() {
		t.testcase = nil
	}()

	t.testcase.err <- nil
}

func TestT20template(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	//nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t20node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t20, popt)
	if err != nil {
		panic(err)
	}

	t20cases = []*testcase{
		&testcase{"TestStateEnterCallback", nil, nil, make(chan error)},
	}
	for _, tc := range t20cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(3); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
