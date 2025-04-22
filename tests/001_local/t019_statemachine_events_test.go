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
	t19cases []*testcase
)

func factory_t19() gen.ProcessBehavior {
	return &t19{}
}

type t19 struct {
	act.Actor

	testcase *testcase
}

func (t *t19) HandleMessage(from gen.PID, message any) error {
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

func factory_t19_events() gen.ProcessBehavior {
	return &t19_events{}
}

type t19_events struct {
	act.StateMachine[t19_events_data]
	tc *testcase
}

type t19_events_data struct {
	eventsReceived int
}

type t19_get_events_received struct {
}

type t19_event struct {
	payload string
}

func (sm *t19_events) Init(args ...any) (act.StateMachineSpec[t19_events_data], error) {
	spec := act.NewStateMachineSpec(gen.Atom("state1"),
		// initial data
		act.WithData(t19_events_data{}),

		// register event handler
		act.WithEventHandler(gen.Event{Name: "testEvent", Node: "t19node@localhost"}, t20_handle_test_event),

		// register call handler
		act.WithStateCallHandler(gen.Atom("state1"), t19_event_received),
	)

	return spec, nil
}

func t19_event_received(state gen.Atom, data t19_events_data, event t19_get_events_received, proc gen.Process) (gen.Atom, t19_events_data, int, error) {
	return state, data, data.eventsReceived, nil
}

func t20_handle_test_event(state gen.Atom, data t19_events_data, event t19_event, proc gen.Process) (gen.Atom, t19_events_data, error) {
	data.eventsReceived++
	return state, data, nil
}

func (t *t19) TestEvents(input any) {
	defer func() {
		t.testcase = nil
	}()

	// Register the event first, otherwise the StateMachine will not be able
	// to start monitoring.
	testEvent := gen.Atom("testEvent")
	token, err := t.RegisterEvent(testEvent, gen.EventOptions{})

	pid, err := t.Spawn(factory_t19_events, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn statemachine process: %s", err)
		t.testcase.err <- err
		return
	}

	// Ensure we have not received any events prior to the test
	result, err := t.Call(pid, t19_get_events_received{})
	if err != nil {
		t.Log().Error("call 't19_get_events_received' failed: %s", err)
		t.testcase.err <- err
		return
	}
	data := result.(int)
	if data != 0 {
		t.testcase.err <- fmt.Errorf("expected 0 test events to be received, got: %d", data)
		return
	}

	// Send event
	event := t19_event{lib.RandomString(8)}
	t.SendEvent(testEvent, token, event)

	// Send another event
	event = t19_event{lib.RandomString(8)}
	t.SendEvent(testEvent, token, event)

	// Ensure we received both events
	result, err = t.Call(pid, t19_get_events_received{})
	if err != nil {
		t.Log().Error("call 't19_get_events_received' failed: %s", err)
		t.testcase.err <- err
		return
	}
	data = result.(int)
	if data != 2 {
		t.testcase.err <- fmt.Errorf("expected 2 test events to be received, got: %d", data)
		return
	}

	t.testcase.err <- nil
}

func TestT19template(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	//nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t19node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t19, popt)
	if err != nil {
		panic(err)
	}

	t19cases = []*testcase{
		&testcase{"TestEvents", nil, nil, make(chan error)},
	}
	for _, tc := range t19cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(3); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
