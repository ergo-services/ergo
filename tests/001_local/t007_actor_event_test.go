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

var (
	t7cases []*testcase
)

func factory_t7() gen.ProcessBehavior {
	return &t7{}
}

type t7 struct {
	act.Actor

	testcase *testcase
}

func (t *t7) Init(args ...any) error {
	return nil
}

func (t *t7) HandleMessage(from gen.PID, message any) error {
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
func (t *t7) HandleEvent(message gen.MessageEvent) error {
	var event gen.Event
	switch m := t.testcase.input.(type) {
	case gen.MessageEvent:
		event = m.Event
	case gen.Event:
		event = m
	default:
		t.testcase.err <- errIncorrect
		return nil
	}
	if message.Event != event {
		t.testcase.err <- errIncorrect
		return nil
	}

	if message.Timestamp == 0 {
		t.testcase.err <- errIncorrect
		return nil
	}

	t.testcase.output = message
	t.testcase.err <- nil
	return nil
}

func (t *t7) Terminate(reason error) {
	tc := t.testcase
	if tc == nil {
		return
	}
	tc.output = reason

	// we shouldn't be blocked by the channel, so we use select
	select {
	case tc.err <- nil:
	default:
	}
}

func (t *t7) TestEvent(input any) {
	defer func() {
		t.testcase = nil
	}()

	producerPID, err := t.Spawn(factory_t7, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	producerTC := &testcase{"ProducerNotify", nil, nil, make(chan error)}
	t.Send(producerPID, producerTC)
	if err := producerTC.wait(1); err != nil {
		t.Node().Kill(producerPID)
		t.testcase.err <- err
		return
	}
	defer t.Node().Kill(producerPID)

	consumer1PID, err := t.Spawn(factory_t7, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}
	defer t.Node().Kill(consumer1PID)

	// producerTC.output has the name of created event
	name := producerTC.output.(gen.Atom)
	event := gen.Event{Name: name, Node: t.Node().Name()}
	consumer1TC := &testcase{"Consumer1", event, nil, make(chan error)}
	t.Send(consumer1PID, consumer1TC)
	if err := consumer1TC.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	// producer should receive gen.MessageEventStart
	if err := producerTC.wait(1); err != nil {
		t.Node().Kill(producerPID)
		t.testcase.err <- err
		return
	}

	t.Send(producerPID, int8(123))
	// producer sending event. check it
	if err := producerTC.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	// consumer1 should receive this event
	if err := consumer1TC.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	mev := consumer1TC.output.(gen.MessageEvent)
	if mev.Event != event {
		t.testcase.err <- errIncorrect
		return
	}

	if mev.Message.(int8) != int8(123) {
		t.testcase.err <- errIncorrect
		return
	}

	// start yet another consumer
	consumer2PID, err := t.Spawn(factory_t7, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}
	defer t.Node().Kill(consumer2PID)
	consumer2TC := &testcase{"Consumer2", mev, nil, make(chan error)}
	t.Send(consumer2PID, consumer2TC)
	if err := consumer2TC.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	t.Send(producerPID, int16(1234))
	// producer sending event. check it
	if err := producerTC.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	// consumer1 should receive this event
	if err := consumer1TC.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	mev = consumer1TC.output.(gen.MessageEvent)
	if mev.Event != event {
		t.testcase.err <- errIncorrect
		return
	}
	if mev.Message.(int16) != int16(1234) {
		t.testcase.err <- errIncorrect
		return
	}
	// consumer2 should also receive this event
	if err := consumer2TC.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	mev = consumer2TC.output.(gen.MessageEvent)
	if mev.Event != event {
		t.testcase.err <- errIncorrect
		return
	}
	if mev.Message.(int16) != int16(1234) {
		t.testcase.err <- errIncorrect
		return
	}

	// send both consumer int value so they should Unlink/Demonitor this event
	t.Send(consumer1PID, 1)
	if err := consumer1TC.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	t.Send(consumer2PID, 1)
	if err := consumer2TC.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	// ... and producer should receive MessageEventStop message since there are no
	// consumers for this event anymore
	if err := producerTC.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	t.testcase.err <- nil
}

func (t *t7) ProducerNotify(input any) {
	if _, ok := input.(initcase); ok {
		name := gen.Atom(lib.RandomString(10))
		opts := gen.EventOptions{
			Notify: true,
			Buffer: 10,
		}
		if token, err := t.RegisterEvent(name, opts); err != nil {
			t.testcase.err <- err
			return
		} else {
			t.testcase.input = token
			t.testcase.output = name
			t.testcase.err <- nil
			return
		}
	}

	switch m := input.(type) {
	case gen.MessageEventStart:
		name := t.testcase.output.(gen.Atom)
		if m.Name != name {
			t.testcase.err <- errIncorrect
		}
		t.testcase.err <- nil

	case gen.MessageEventStop:
		name := t.testcase.output.(gen.Atom)
		if m.Name != name {
			t.testcase.err <- errIncorrect
		}
		t.testcase.err <- nil
	case int8:
		token := t.testcase.input.(gen.Ref)
		name := t.testcase.output.(gen.Atom)
		if err := t.SendEvent(name, token, m); err != nil {
			t.testcase.err <- err
		}
		t.testcase.err <- nil
	case int16:
		token := t.testcase.input.(gen.Ref)
		name := t.testcase.output.(gen.Atom)
		if err := t.SendEvent(name, token, m); err != nil {
			t.testcase.err <- err
		}
		t.testcase.err <- nil
	}

	// wait messages
}

func (t *t7) Consumer1(input any) {
	if _, ok := input.(initcase); ok {
		event := t.testcase.input.(gen.Event)
		lastevents, err := t.LinkEvent(event)
		if err != nil {
			t.testcase.err <- errIncorrect
			return
		}
		if len(lastevents) != 0 {
			t.testcase.err <- errIncorrect
			return
		}
		t.testcase.err <- nil

		// waiting event
		return
	}

	switch input.(type) {
	case int:
		event := t.testcase.input.(gen.Event)
		t.UnlinkEvent(event)
		t.testcase.err <- nil
		return
	}
	panic(input)
}

func (t *t7) Consumer2(input any) {
	if _, ok := input.(initcase); ok {
		mev := t.testcase.input.(gen.MessageEvent)
		lastevents, err := t.MonitorEvent(mev.Event)
		if err != nil || len(lastevents) != 1 {
			t.testcase.err <- errIncorrect
			return
		}

		// here must be the last MessageEvent value
		if lastevents[0] != mev {
			t.testcase.err <- errIncorrect
			return
		}
		t.testcase.err <- nil

		// waiting event
		return
	}

	switch input.(type) {
	case int:
		mev := t.testcase.input.(gen.MessageEvent)
		t.DemonitorEvent(mev.Event)
		t.testcase.err <- nil
		return
	}
	panic(input)
}

func TestT7ActorEvent(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	node, err := ergo.StartNode("t7node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t7, popt)
	if err != nil {
		panic(err)
	}

	t7cases = []*testcase{
		{"TestEvent", nil, nil, make(chan error)},
	}
	for _, tc := range t7cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
