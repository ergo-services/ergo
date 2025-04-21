package local

import (
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
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
	transitions         int
	stateEnterCallbacks int
	testEventReceived   bool
}

type t18transitionState1toState2 struct {
}

type t18transitionState2toState1 struct {
}

type t18query struct {
}

type t18event struct {
	payload string
}

func (sm *t18statemachine) Init(args ...any) (act.StateMachineSpec[t18data], error) {
	spec := act.NewStateMachineSpec(gen.Atom("state1"),
		// initial data
		act.WithData(t18data{}),

		// set up a message handler for the transition state1 -> state2
		act.WithStateMessageHandler(gen.Atom("state1"), state1to2),

		// set up a call handler to query the data
		act.WithStateCallHandler(gen.Atom("state3"), queryData),

		// set up a state enter callback
		act.WithStateEnterCallback(stateEnter),

		// register event handler
		act.WithEventHandler(gen.Event{Name: "testEvent", Node: "t18node@localhost"}, handleTestEvent),
	)

	return spec, nil
}

func state1to2(state gen.Atom, data t18data, message t18transitionState1toState2, proc gen.Process) (gen.Atom, t18data, error) {
	data.transitions++
	return gen.Atom("state2"), data, nil
}

func queryData(state gen.Atom, data t18data, message t18query, proc gen.Process) (gen.Atom, t18data, t18data, error) {
	return state, data, data, nil
}

func stateEnter(oldState gen.Atom, newState gen.Atom, data t18data, proc gen.Process) (gen.Atom, t18data, error) {
	data.stateEnterCallbacks++

	if newState == gen.Atom("state2") {
		data.transitions++
		return gen.Atom("state3"), data, nil

	}
	return newState, data, nil
}

func handleTestEvent(state gen.Atom, data t18data, event t18event, proc gen.Process) (gen.Atom, t18data, error) {
	data.testEventReceived = true
	return state, data, nil
}

func (t *t18) TestStateMachine(input any) {
	defer func() {
		t.testcase = nil
	}()

	// Register the event first, otherwise the StateMachine will not be able
	// to start monitoring.
	testEvent := gen.Atom("testEvent")
	token, err := t.RegisterEvent(testEvent, gen.EventOptions{})

	pid, err := t.Spawn(factory_t18statemachine, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn statemachine process: %s", err)
		t.testcase.err <- err
		return
	}

	// Send a message to transition to state 2. The state enter callback should
	// automatically transition to state 3 where another state enter callback
	// does not trigger any further state transitions.
	err = t.Send(pid, t18transitionState1toState2{})
	if err != nil {
		t.Log().Error("send 't18transitionState1toState2' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// Query the data from the state machine (and test StateCallHandler behavior)
	result, err := t.Call(pid, t18query{})
	if err != nil {
		t.Log().Error("call 't18query' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// We expect 2 state transitions (state1 -> state2 -> state3)
	data := result.(t18data)
	if data.transitions != 2 {
		t.testcase.err <- fmt.Errorf("expected 2 state transitions, got %v", result)
		return
	}

	// We  expect a chain of 2 state enter callback functions to be called, one
	// for state2 and one for state3.
	if data.stateEnterCallbacks != 2 {
		t.testcase.err <- fmt.Errorf("expected 2 state enter function invocations, got %d", data.stateEnterCallbacks)
		return
	}

	event := t18event{lib.RandomString(8)}
	t.SendEvent(testEvent, token, event)

	// Query the data from the state machine
	result, err = t.Call(pid, t18query{})
	if err != nil {
		t.Log().Error("call 't18query' failed: %s", err)
		t.testcase.err <- err
		return
	}
	data = result.(t18data)
	if data.testEventReceived == false {
		t.testcase.err <- fmt.Errorf("expected test event to be received")
		return
	}

	// Statemachine process should crash on invalid state transition
	err = t.testcase.expectProcessToTerminate(pid, t, func(p gen.Process) error {
		return p.Send(pid, t18transitionState2toState1{}) // we are in state3
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
