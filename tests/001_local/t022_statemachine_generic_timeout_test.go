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
	t22cases []*testcase
)

func factory_t22() gen.ProcessBehavior {
	return &t22{}
}

type t22 struct {
	act.Actor

	testcase *testcase
}

func factory_t22_generic_timeouts() gen.ProcessBehavior {
	return &t22_generic_timeouts{}
}

type t22_generic_timeouts struct {
	act.StateMachine[t22_generic_timeouts_data]
}

type t22_generic_timeouts_data struct {
	timeouts int
}

type t22_start_timeout struct {
	duration time.Duration
}

type t22_timeout1_timed_out struct {
}

type t22_get_timeouts struct {
}

func (sm *t22_generic_timeouts) Init(args ...any) (act.StateMachineSpec[t22_generic_timeouts_data], error) {
	spec := act.NewStateMachineSpec(gen.Atom("state1"),
		// initial data
		act.WithData(t22_generic_timeouts_data{}),

		// set up a message handler to start timeout1
		act.WithStateMessageHandler(gen.Atom("state1"), t22_handle_start_timeout),

		// set up a message handler for timeout1_timed_out message
		act.WithStateMessageHandler(gen.Atom("state1"), t22_handle_timeout1_timed_out),

		// set up a call handler to query the number of timeouts
		act.WithStateCallHandler(gen.Atom("state1"), t22_handle_get_timeouts),
	)

	return spec, nil
}

func t22_handle_start_timeout(state gen.Atom, data t22_generic_timeouts_data, message t22_start_timeout, proc gen.Process) (gen.Atom, t22_generic_timeouts_data, []act.Action, error) {
	timeout := act.GenericTimeout{
		Name:     gen.Atom("timeout1"),
		Duration: message.duration,
		Message:  t22_timeout1_timed_out{},
	}
	return state, data, []act.Action{timeout}, nil
}

func t22_handle_timeout1_timed_out(state gen.Atom, data t22_generic_timeouts_data, message t22_timeout1_timed_out, proc gen.Process) (gen.Atom, t22_generic_timeouts_data, []act.Action, error) {
	data.timeouts++
	return state, data, nil, nil
}

func t22_handle_get_timeouts(state gen.Atom, data t22_generic_timeouts_data, message t22_get_timeouts, proc gen.Process) (gen.Atom, t22_generic_timeouts_data, int, []act.Action, error) {
	return state, data, data.timeouts, nil, nil
}

func (t *t22) HandleMessage(from gen.PID, message any) error {
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

func (t *t22) TestGenericTimeoutTimingOut(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t22_generic_timeouts, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn statemachine process: %s", err)
		t.testcase.err <- err
		return
	}

	// Ensure no timeouts have occured and we are in state 1.
	result, err := t.Call(pid, t22_get_timeouts{})
	if err != nil {
		t.Log().Error("call 't21_get_state_and_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}
	timeouts := result.(int)
	if timeouts != 0 {
		t.testcase.err <- fmt.Errorf("expected 0 timeout, got %d", timeouts)
		return
	}

	// Send a message to that configres a generic timeout.
	err = t.Send(pid, t22_start_timeout{duration: 10 * time.Millisecond})
	if err != nil {
		t.Log().Error("send ' t22_start_timeout' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// Wait for 20 millis.
	time.Sleep(20 * time.Millisecond)

	// Query the number of timeouts.
	result, err = t.Call(pid, t22_get_timeouts{})
	if err != nil {
		t.Log().Error("call 't22_get_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// We expect 1 timeout.
	timeouts = result.(int)
	if timeouts != 1 {
		t.testcase.err <- fmt.Errorf(">> expected 1 timeout, got %d", timeouts)
		return
	}

	t.testcase.err <- nil
}

func (t *t22) TestGenericTimeoutCanceledByNewTimerWithSameName(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t22_generic_timeouts, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn statemachine process: %s", err)
		t.testcase.err <- err
		return
	}

	// Ensure no timeouts have occured and we are in state 1.
	result, err := t.Call(pid, t22_get_timeouts{})
	if err != nil {
		t.Log().Error("call 't21_get_state_and_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}
	timeouts := result.(int)
	if timeouts != 0 {
		t.testcase.err <- fmt.Errorf("expected 0 timeout, got %d", timeouts)
		return
	}

	// Send a message to that configres a generic timeout.
	err = t.Send(pid, t22_start_timeout{duration: 20 * time.Millisecond})
	if err != nil {
		t.Log().Error("send ' t22_start_timeout' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// Send a message to that configres a generic timeout.
	err = t.Send(pid, t22_start_timeout{duration: 10 * time.Millisecond})
	if err != nil {
		t.Log().Error("send ' t22_start_timeout' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// Wait for 20 millis.
	time.Sleep(20 * time.Millisecond)

	// Query the number of timeouts.
	result, err = t.Call(pid, t22_get_timeouts{})
	if err != nil {
		t.Log().Error("call 't22_get_timeouts' failed: %s", err)
		t.testcase.err <- err
		return
	}

	// We expect 1 timeout.
	timeouts = result.(int)
	if timeouts != 1 {
		t.testcase.err <- fmt.Errorf(">> expected 1 timeout, got %d", timeouts)
		return
	}

	t.testcase.err <- nil
}

func TestT022StateMachineGenericTimeout(t *testing.T) {
	nopt := gen.NodeOptions{}
	//nopt.Log.DefaultLogger.Disable = true
	nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t22node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t22, popt)
	if err != nil {
		panic(err)
	}

	t22cases = []*testcase{
		{"TestGenericTimeoutTimingOut", nil, nil, make(chan error)},
		{"TestGenericTimeoutCanceledByNewTimerWithSameName", nil, nil, make(chan error)},
	}
	for _, tc := range t22cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(3); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
