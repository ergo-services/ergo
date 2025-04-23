package local

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

//
// this is the template for writing new tests
//

var (
	t21cases []*testcase
)

func factory_t21() gen.ProcessBehavior {
	return &t21{}
}

type t21 struct {
	act.Actor

	testcase *testcase
}

func (t *t21) HandleMessage(from gen.PID, message any) error {
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

func factory_t21_state_timeouts() gen.ProcessBehavior {
	return &t21_state_timeouts{}
}

type t21_state_timeouts struct {
	act.StateMachine[t21_state_timeouts_data]
}

type t21_state_timeouts_data struct {
	timeouts int
}

type t21_state1 struct {
}

type t21_state2 struct {
	timeoutAfter time.Duration
}

type t21_state3 struct {
}

type t21_trigger_second_timeout struct {
}

type t21_second_timeout struct {
}

type t21_state_and_timeouts struct {
	timeouts int
	state    gen.Atom
}

func (sm *t21_state_timeouts) Init(args ...any) (act.StateMachineSpec[t21_state_timeouts_data], error) {
	spec := act.NewStateMachineSpec(gen.Atom("state1"),
		// Set up the initial data.
		act.WithData(t21_state_timeouts_data{}),

		// Set up a message handler for the transition [state1] -> [state2] that
		// configures a state timeout with a configurable duration and sends
		// `t21_state1` on timeout.
		act.WithStateMessageHandler(gen.Atom("state1"), t21_trigger_timeout_from_message),

		// Set up a message handler for state2 which does _not_ trigger a state
		// transition but sets up a second state timeout timer that sends
		// `t21_state3` on timeout. We want to test that setting up a state
		// timeout cancels any previous state timeouts. If we were to change
		// the state, then the state change would cause any previously set up
		// state timeout to be canceled.
		act.WithStateMessageHandler(gen.Atom("state2"), t21_trigger_second_timeout_from_message),

		// Set up a message handler for state2 that handles the second timeout
		// message.
		act.WithStateMessageHandler(gen.Atom("state2"), t21_handle_second_timeout_message),

		// Set up a call handler for the transition [state1] -> [state2] that
		// configures a state timeout with a configurable duration and sends
		// `t21_state1` on timeout.
		act.WithStateCallHandler(gen.Atom("state1"), t21_trigger_timeout_from_call),

		// Set up a message handler for the transition [state2] -> [state1], we
		// will send this message when the state timeout times out.
		act.WithStateMessageHandler(gen.Atom("state2"), t21_move_to_state1),

		// Set up a message handler for the transition [state2] -> [state3].
		act.WithStateMessageHandler(gen.Atom("state2"), t21_move_to_state3),

		// Set up a call handler to query the state and the number of state
		// timeouts
		act.WithStateCallHandler(gen.Atom("state1"), t21_get_state_and_timeouts),
		act.WithStateCallHandler(gen.Atom("state2"), t21_get_state_and_timeouts),
		act.WithStateCallHandler(gen.Atom("state3"), t21_get_state_and_timeouts),

		// Set up event handler that configures a state timeout.
		act.WithEventHandler(gen.Event{Name: "testEvent", Node: "t21node@localhost"}, t21_trigger_timeout_from_event),
	)

	return spec, nil
}

func t21_trigger_timeout_from_message(state gen.Atom, data t21_state_timeouts_data, message t21_state2, proc gen.Process) (gen.Atom, t21_state_timeouts_data, []act.Action, error) {
	timeout := act.StateTimeout{
		Duration: message.timeoutAfter,
		Message:  t21_state1{},
	}
	return gen.Atom("state2"), data, []act.Action{timeout}, nil
}

func t21_trigger_second_timeout_from_message(state gen.Atom, data t21_state_timeouts_data, message t21_trigger_second_timeout, proc gen.Process) (gen.Atom, t21_state_timeouts_data, []act.Action, error) {
	timeout := act.StateTimeout{
		Duration: 10 * time.Millisecond,
		Message:  t21_second_timeout{},
	}
	return state, data, []act.Action{timeout}, nil
}

func t21_trigger_timeout_from_call(state gen.Atom, data t21_state_timeouts_data, message t21_state2, proc gen.Process) (gen.Atom, t21_state_timeouts_data, int, []act.Action, error) {
	timeout := act.StateTimeout{
		Duration: message.timeoutAfter,
		Message:  t21_state1{},
	}
	return gen.Atom("state2"), data, 0, []act.Action{timeout}, nil
}

func t21_trigger_timeout_from_event(state gen.Atom, data t21_state_timeouts_data, message t21_state2, proc gen.Process) (gen.Atom, t21_state_timeouts_data, []act.Action, error) {
	timeout := act.StateTimeout{
		Duration: message.timeoutAfter,
		Message:  t21_state1{},
	}
	return gen.Atom("state2"), data, []act.Action{timeout}, nil
}

func t21_move_to_state1(state gen.Atom, data t21_state_timeouts_data, message t21_state1, proc gen.Process) (gen.Atom, t21_state_timeouts_data, []act.Action, error) {
	data.timeouts++
	return gen.Atom("state1"), data, nil, nil
}

func t21_move_to_state3(state gen.Atom, data t21_state_timeouts_data, message t21_state3, proc gen.Process) (gen.Atom, t21_state_timeouts_data, []act.Action, error) {
	return gen.Atom("state3"), data, nil, nil
}

func t21_handle_second_timeout_message(state gen.Atom, data t21_state_timeouts_data, message t21_second_timeout, proc gen.Process) (gen.Atom, t21_state_timeouts_data, []act.Action, error) {
	data.timeouts++
	return state, data, nil, nil
}

func t21_get_state_and_timeouts(state gen.Atom, data t21_state_timeouts_data, message t21_state_and_timeouts, proc gen.Process) (gen.Atom, t21_state_timeouts_data, t21_state_and_timeouts, []act.Action, error) {
	result := t21_state_and_timeouts{data.timeouts, state}
	return state, data, result, nil, nil
}

func (t *t21) TestStateTimeoutConfiguredFromMessageHandlerTimingOut(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t21_state_timeouts, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn statemachine process: %s", err)
		t.testcase.err <- err
		return
	}

	// Ensure no timeouts have occured and we are in state 1
	result, err := t.Call(pid, t21_state_and_timeouts{})
	if err != nil {
		t.Log().Error("call 't21_get_state_and_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}
	data := result.(t21_state_and_timeouts)
	if data.timeouts != 0 {
		t.testcase.err <- fmt.Errorf("expected 0 timeout, got %d", data.timeouts)
		return
	}
	if data.state != gen.Atom("state1") {
		t.testcase.err <- fmt.Errorf("expected state2, got %v", data.state)
		return
	}

	// Send a message to transition to state 2. The message should start a state
	// timeout for 10 ms which transitions back to state 1 on timeout.
	err = t.Send(pid, t21_state2{timeoutAfter: 10 * time.Millisecond})
	if err != nil {
		t.Log().Error("send 't21_state2' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// Wait for 20 millis
	time.Sleep(20 * time.Millisecond)

	// Query the data from the state machine (and test StateCallHandler behavior)
	result, err = t.Call(pid, t21_state_and_timeouts{})
	if err != nil {
		t.Log().Error("call 't21_get_state_and_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// We expect the state to have been updated to state1 and we expect 1 timeout
	data = result.(t21_state_and_timeouts)
	if data.timeouts != 1 {
		t.testcase.err <- fmt.Errorf(">> expected 1 timeout, got %d", data.timeouts)
		return
	}
	if data.state != gen.Atom("state1") {
		t.testcase.err <- fmt.Errorf("expected state1, got %v", data.state)
		return
	}

	t.testcase.err <- nil
}

func (t *t21) TestStateTimeoutConfiguredFromCallHandlerTimingOut(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t21_state_timeouts, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn statemachine process: %s", err)
		t.testcase.err <- err
		return
	}

	// Ensure no timeouts have occured and we are in state 1
	result, err := t.Call(pid, t21_state_and_timeouts{})
	if err != nil {
		t.Log().Error("call 't21_get_state_and_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}
	data := result.(t21_state_and_timeouts)
	if data.timeouts != 0 {
		t.testcase.err <- fmt.Errorf("expected 0 timeout, got %d", data.timeouts)
		return
	}
	if data.state != gen.Atom("state1") {
		t.testcase.err <- fmt.Errorf("expected state2, got %v", data.state)
		return
	}

	// Send a message to transition to state 2. The message should start a state
	// timeout for 10 ms which transitions back to state 1 on timeout.
	_, err = t.Call(pid, t21_state2{timeoutAfter: 10 * time.Millisecond})
	if err != nil {
		t.Log().Error("send 't21_state2' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// Wait for 20 millis
	time.Sleep(20 * time.Millisecond)

	// Query the data from the state machine (and test StateCallHandler behavior)
	result, err = t.Call(pid, t21_state_and_timeouts{})
	if err != nil {
		t.Log().Error("call 't21_get_state_and_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// We expect the state to have been updated to state1 and we expect 1 timeout
	data = result.(t21_state_and_timeouts)
	if data.timeouts != 1 {
		t.testcase.err <- fmt.Errorf("expected 1 timeout, got %d", data.timeouts)
		return
	}
	if data.state != gen.Atom("state1") {
		t.testcase.err <- fmt.Errorf("expected state1, got %v", data.state)
		return
	}

	t.testcase.err <- nil
}

func (t *t21) TestStateTimeoutConfiguredFromEventHandlerTimingOut(input any) {
	defer func() {
		t.testcase = nil
	}()
	token := t.testcase.input.(gen.Ref)

	pid, err := t.Spawn(factory_t21_state_timeouts, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn statemachine process: %s", err)
		t.testcase.err <- err
		return
	}

	// Ensure no timeouts have occured and we are in state 1
	result, err := t.Call(pid, t21_state_and_timeouts{})
	if err != nil {
		t.Log().Error("call 't21_get_state_and_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}
	data := result.(t21_state_and_timeouts)
	if data.timeouts != 0 {
		t.testcase.err <- fmt.Errorf("expected 0 timeout, got %d", data.timeouts)
		return
	}
	if data.state != gen.Atom("state1") {
		t.testcase.err <- fmt.Errorf("expected state2, got %v", data.state)
		return
	}

	// Send a message to transition to state 2. The message should start a state
	// timeout for 10 ms which transitions back to state 1 on timeout.
	event := t21_state2{timeoutAfter: 10 * time.Millisecond}
	t.SendEvent(gen.Atom("testEvent"), token, event)

	// Wait for 20 millis
	time.Sleep(20 * time.Millisecond)

	// Query the data from the state machine (and test StateCallHandler behavior)
	result, err = t.Call(pid, t21_state_and_timeouts{})
	if err != nil {
		t.Log().Error("call 't21_get_state_and_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// We expect the state to have been updated to state1 and we expect 1 timeout
	data = result.(t21_state_and_timeouts)
	if data.timeouts != 1 {
		t.testcase.err <- fmt.Errorf("expected 1 timeout, got %d", data.timeouts)
		return
	}
	if data.state != gen.Atom("state1") {
		t.testcase.err <- fmt.Errorf("expected state1, got %v", data.state)
		return
	}

	t.testcase.err <- nil
}

func (t *t21) TestStateTimeoutCanceledByStateChange(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t21_state_timeouts, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn statemachine process: %s", err)
		t.testcase.err <- err
		return
	}

	// Ensure no timeouts have occured and we are in state 1
	result, err := t.Call(pid, t21_state_and_timeouts{})
	if err != nil {
		t.Log().Error("call 't21_get_state_and_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}
	data := result.(t21_state_and_timeouts)
	if data.timeouts != 0 {
		t.testcase.err <- fmt.Errorf("expected 0 timeout, got %d", data.timeouts)
		return
	}
	if data.state != gen.Atom("state1") {
		t.testcase.err <- fmt.Errorf("expected state1, got %v", data.state)
		return
	}

	// Send a message to transition to state 2. Schedule a timeout after 500ms
	// allowing time to cancel the timeout.
	err = t.Send(pid, t21_state2{timeoutAfter: 500 * time.Millisecond})
	if err != nil {
		t.Log().Error("send 't21_state2' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// Send a message to transition to state 3. This should cancel the state
	// timeout for state 2.
	err = t.Send(pid, t21_state3{})
	if err != nil {
		t.Log().Error("send 't21_state3' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// Wait for 500 millis, which was the timeout set up fo state 2.
	time.Sleep(500 * time.Millisecond)

	// Query the data from the state machine (and test StateCallHandler behavior)
	result, err = t.Call(pid, t21_state_and_timeouts{})
	if err != nil {
		t.Log().Error("call 't21_get_state_and_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// We expect the state to have been updated to state3 and we don't expect
	// eny timeouts
	data = result.(t21_state_and_timeouts)
	if data.timeouts != 0 {
		t.testcase.err <- fmt.Errorf("expected 0 timeout, got %d", data.timeouts)
		return
	}
	if data.state != gen.Atom("state3") {
		t.testcase.err <- fmt.Errorf("expected state3, got %v", data.state)
		return
	}

	t.testcase.err <- nil
}

func (t *t21) TestStateTimeoutCanceledByNewStateTimeoutTimer(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t21_state_timeouts, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn statemachine process: %s", err)
		t.testcase.err <- err
		return
	}

	// Ensure no timeouts have occured and we are in state 1
	result, err := t.Call(pid, t21_state_and_timeouts{})
	if err != nil {
		t.Log().Error("call 't21_get_state_and_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}
	data := result.(t21_state_and_timeouts)
	if data.timeouts != 0 {
		t.testcase.err <- fmt.Errorf("expected 0 timeout, got %d", data.timeouts)
		return
	}
	if data.state != gen.Atom("state1") {
		t.testcase.err <- fmt.Errorf("expected state1, got %v", data.state)
		return
	}

	// Send a message to transition to state 2. Schedule a timeout after 500ms
	// allowing time to cancel the timeout.
	err = t.Send(pid, t21_state2{timeoutAfter: 50 * time.Millisecond})
	if err != nil {
		t.Log().Error("send 't21_state2' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// Send a message to transition that creates a new timeout but does not
	// change the state. We want to validate that creating a new stat timeout
	// cancels the previous state timeout. On timeout this second timer sends
	// `t21_trigger_second_timeout` transitioning the state machine to state 4.
	err = t.Send(pid, t21_trigger_second_timeout{})
	if err != nil {
		t.Log().Error("send 't21_trigger_second_timeout' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// Wait for 500 millis, which was the timeout set up fo state 2.
	time.Sleep(500 * time.Millisecond)

	// Query the data from the state machine (and test StateCallHandler behavior)
	result, err = t.Call(pid, t21_state_and_timeouts{})
	if err != nil {
		t.Log().Error("call 't21_get_state_and_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// We expect the state to be state2 and we expect 1 timeout. The first timer,
	// which would have triggered a transition [state2] -> [state1] should have
	// been canceled. The second timer which does not trigger a state transition
	// should have timed out and leave the StateMachine in state2.
	data = result.(t21_state_and_timeouts)
	if data.timeouts != 1 {
		t.testcase.err <- fmt.Errorf("expected 1 timeout, got %d", data.timeouts)
		return
	}
	if data.state != gen.Atom("state2") {
		t.testcase.err <- fmt.Errorf("expected state2, got %v", data.state)
		return
	}

	t.testcase.err <- nil
}

func TestT21StateMachineStateTimeouts(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	// nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t21node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t21, popt)
	if err != nil {
		panic(err)
	}

	// Register the event first, otherwise the StateMachine will not be able
	// to start monitoring.
	testEvent := gen.Atom("testEvent")
	token, err := node.RegisterEvent(testEvent, gen.EventOptions{})
	if err != nil {
		t.Fatal(err)
	}

	t21cases = []*testcase{
		// Start with the events test as we share a `StateMachineSpec` and other
		// StateMachine processes would start handling events after their test
		{"TestStateTimeoutConfiguredFromEventHandlerTimingOut", token, nil, make(chan error)},
		{"TestStateTimeoutConfiguredFromMessageHandlerTimingOut", nil, nil, make(chan error)},
		{"TestStateTimeoutConfiguredFromCallHandlerTimingOut", nil, nil, make(chan error)},
		{"TestStateTimeoutCanceledByStateChange", nil, nil, make(chan error)},
		{"TestStateTimeoutCanceledByNewStateTimeoutTimer", nil, nil, make(chan error)},
	}
	for _, tc := range t21cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(3); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
