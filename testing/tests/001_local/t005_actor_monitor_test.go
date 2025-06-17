package local

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

var (
	t5cases []*testcase
)

func factory_t5() gen.ProcessBehavior {
	return &t5{}
}

type t5 struct {
	act.Actor

	testcase *testcase
}

func (t *t5) Init(args ...any) error {
	return nil
}

func (t *t5) HandleMessage(from gen.PID, message any) error {
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

func (t *t5) Terminate(reason error) {
	tc := t.testcase
	if tc == nil {
		return
	}
	tc.output = reason
	tc.err <- nil
}

// test methods

func (t *t5) TestMonitorPID(input any) {
	defer func() {
		t.testcase = nil
	}()

	for _, reason := range []error{gen.TerminateReasonKill, errors.New("custom"), gen.TerminateReasonShutdown, gen.TerminateReasonPanic} {

		pid, err := t.Spawn(factory_t5, gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}
		newtc := &testcase{"MonitoringPID", reason, nil, make(chan error)}
		t.Send(pid, newtc)
		if err := newtc.wait(1); err != nil {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}
		t.Node().Kill(pid)
	}

	t.testcase.err <- nil
}

func (t *t5) TestMonitorProcessID(input any) {
	defer func() {
		t.testcase = nil
	}()

	for _, reason := range []error{gen.TerminateReasonKill, errors.New("custom"), gen.TerminateReasonShutdown, gen.TerminateReasonPanic, gen.ErrUnregistered} {

		pid, err := t.Spawn(factory_t5, gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}
		newtc := &testcase{"MonitoringProcessID", reason, nil, make(chan error)}
		t.Send(pid, newtc)
		if err := newtc.wait(1); err != nil {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}
		t.Node().Kill(pid)
	}

	t.testcase.err <- nil
}

func (t *t5) TestMonitorAlias(input any) {
	defer func() {
		t.testcase = nil
	}()

	for _, reason := range []error{gen.TerminateReasonKill, errors.New("custom"), gen.TerminateReasonShutdown, gen.TerminateReasonPanic, gen.ErrUnregistered} {

		pid, err := t.Spawn(factory_t5, gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}
		newtc := &testcase{"MonitoringAlias", reason, nil, make(chan error)}
		t.Send(pid, newtc)
		if err := newtc.wait(1); err != nil {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}
		t.Node().Kill(pid)
	}

	t.testcase.err <- nil
}

func (t *t5) TestMonitorEvent(input any) {
	defer func() {
		t.testcase = nil
	}()
	for _, reason := range []error{gen.TerminateReasonKill, errors.New("custom"), gen.TerminateReasonShutdown, gen.TerminateReasonPanic, gen.ErrUnregistered} {

		pid, err := t.Spawn(factory_t5, gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}
		newtc := &testcase{"MonitoringEvent", reason, nil, make(chan error)}
		t.Send(pid, newtc)
		if err := newtc.wait(1); err != nil {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}
		t.Node().Kill(pid)
	}

	t.testcase.err <- nil
}

func (t *t5) TestMonitorUnknown(input any) {
	defer func() {
		t.testcase = nil
	}()

	// unknown PID
	pid := t.PID()
	pid.ID += 10000
	if err := t.MonitorPID(pid); err != gen.ErrProcessUnknown {
		t.testcase.err <- errIncorrect
		return
	}

	// unknown ProcessID
	pname := gen.ProcessID{Name: "test", Node: t.Node().Name()}
	if err := t.MonitorProcessID(pname); err != gen.ErrProcessUnknown {
		t.testcase.err <- errIncorrect
		return
	}

	// unknown Alias
	alias := gen.Alias{Node: t.Node().Name()}
	if err := t.MonitorAlias(alias); err != gen.ErrAliasUnknown {
		t.testcase.err <- errIncorrect
		return
	}

	// unknown Event
	event := gen.Event{Name: "test"}
	if _, err := t.MonitorEvent(event); err != gen.ErrEventUnknown {
		t.testcase.err <- errIncorrect
		return
	}

	t.testcase.err <- nil
}

// test implementations for monitoring by PID

func (t *t5) MonitoringPID(input any) {
	if _, ok := input.(initcase); ok {
		pid, err := t.Spawn(factory_t5, gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}
		t.testcase.output = pid
		newtc := &testcase{"TargetPID", nil, nil, make(chan error)}
		t.Send(pid, newtc)
		if err := newtc.wait(1); err != nil {
			t.testcase.err <- err
			return
		}
		if info, err := t.Node().ProcessInfo(t.PID()); err == nil {
			if len(info.MonitorsPID) != 0 {
				t.Node().Kill(pid)
				t.testcase.err <- errIncorrect
				return
			}
		} else {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}

		if err := t.MonitorPID(pid); err != nil {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}
		if info, err := t.Node().ProcessInfo(t.PID()); err == nil {
			if len(info.MonitorsPID) != 1 && info.MonitorsPID[0] != pid {
				t.Node().Kill(pid)
				t.testcase.err <- errIncorrect
				return
			}
		} else {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}

		reason := t.testcase.input.(error)
		switch reason {
		case gen.TerminateReasonPanic:
			// send it as a regular message. it will cause panic inside of the TargetPID
			t.Send(pid, "custom panic")
			return
		case gen.TerminateReasonKill:
			t.Node().Kill(pid)
			return
		default:
			t.SendExit(pid, reason)
		}

		if err := newtc.wait(1); err != nil {
			t.testcase.err <- err
			return
		}

		// Terminate callback should be invoked
		if errors.Unwrap(newtc.output.(error)) != reason {
			t.testcase.err <- errIncorrect
			return
		}

		if _, err := t.Node().ProcessInfo(pid); err != gen.ErrProcessUnknown {
			t.testcase.err <- errIncorrect
			return
		}
		// wait for down message
		return
	}

	// must receive down message
	if m, ok := input.(gen.MessageDownPID); ok {
		pid := t.testcase.output.(gen.PID)
		if m.PID != pid {
			t.testcase.err <- errIncorrect
			return
		}

		reason := t.testcase.input.(error)
		if m.Reason != reason {
			t.testcase.err <- errIncorrect
			return
		}
		t.testcase.err <- nil
		return
	}

	t.testcase.err <- errIncorrect
}

// test implementations for monitoring by ProcessID

func (t *t5) MonitoringProcessID(input any) {
	if _, ok := input.(initcase); ok {
		pid, err := t.Spawn(factory_t5, gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}
		newtc := &testcase{"TargetProcessID", nil, nil, make(chan error)}
		t.Send(pid, newtc)
		if err := newtc.wait(1); err != nil {
			t.testcase.err <- err
			return
		}

		name, ok := newtc.output.(gen.Atom)
		if ok == false {
			t.testcase.err <- errIncorrect
			return
		}

		if info, err := t.Node().ProcessInfo(t.PID()); err == nil {
			if len(info.MonitorsProcessID) != 0 {
				t.Node().Kill(pid)
				t.testcase.err <- errIncorrect
				return
			}
		} else {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}

		pname := gen.ProcessID{Name: name, Node: t.Node().Name()}
		t.testcase.output = pname
		if err := t.MonitorProcessID(pname); err != nil {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}

		if info, err := t.Node().ProcessInfo(t.PID()); err == nil {
			if len(info.MonitorsProcessID) != 1 && info.MonitorsProcessID[0] != pname {
				t.Node().Kill(pid)
				t.testcase.err <- errIncorrect
				return
			}
		} else {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}
		reason := t.testcase.input.(error)
		switch reason {
		case gen.TerminateReasonPanic:
			// send it as a regular message. it will cause panic inside of the TargetProcessID
			t.Send(pid, "custom panic")
			return
		case gen.TerminateReasonKill:
			t.Node().Kill(pid)
			return
		case gen.ErrUnregistered:
			t.Send(pid, 1)
		default:
			t.SendExit(pid, reason)
		}

		if err := newtc.wait(1); err != nil {
			t.testcase.err <- err
			return
		}

		// Terminate callback should be invoked
		if errors.Unwrap(newtc.output.(error)) != reason {
			t.testcase.err <- errIncorrect
			return
		}

		if reason == gen.ErrUnregistered {
			// must be alive
			if _, err := t.Node().ProcessInfo(pid); err != nil {
				t.testcase.err <- errIncorrect
				return
			}
		} else {
			if _, err := t.Node().ProcessInfo(pid); err != gen.ErrProcessUnknown {
				t.testcase.err <- errIncorrect
				return
			}

		}

		// wait for down message
		return
	}

	// must receive down message
	if m, ok := input.(gen.MessageDownProcessID); ok {
		processid := t.testcase.output.(gen.ProcessID)
		if m.ProcessID != processid {
			t.testcase.err <- errIncorrect
			return
		}

		reason := t.testcase.input.(error)
		if m.Reason != reason {
			t.testcase.err <- errIncorrect
			return
		}
		t.testcase.err <- nil
		return
	}

	t.testcase.err <- errIncorrect
}

// test implementations for monitoring by Alias

func (t *t5) MonitoringAlias(input any) {
	if _, ok := input.(initcase); ok {
		pid, err := t.Spawn(factory_t5, gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}
		newtc := &testcase{"TargetAlias", nil, nil, make(chan error)}
		t.Send(pid, newtc)
		if err := newtc.wait(1); err != nil {
			t.testcase.err <- err
			return
		}
		alias, ok := newtc.output.(gen.Alias)
		if ok == false {
			t.testcase.err <- errIncorrect
			return
		}
		t.testcase.output = alias

		if info, err := t.Node().ProcessInfo(t.PID()); err == nil {
			if len(info.MonitorsAlias) != 0 {
				t.Node().Kill(pid)
				t.testcase.err <- errIncorrect
				return
			}
		} else {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}

		if err := t.MonitorAlias(alias); err != nil {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}

		if info, err := t.Node().ProcessInfo(t.PID()); err == nil {
			if len(info.MonitorsAlias) != 1 && info.MonitorsAlias[0] != alias {
				t.Node().Kill(pid)
				t.testcase.err <- errIncorrect
				return
			}
		} else {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}

		reason := t.testcase.input.(error)
		switch reason {
		case gen.TerminateReasonPanic:
			// send it as a regular message. it will cause panic inside of the TargetAlias
			t.Send(pid, "custom panic")
			return
		case gen.TerminateReasonKill:
			t.Node().Kill(pid)
			return
		case gen.ErrUnregistered:
			t.Send(pid, alias)
		default:
			t.SendExit(pid, reason)
		}

		if err := newtc.wait(1); err != nil {
			t.testcase.err <- err
			return
		}

		// Terminate callback should be invoked
		if errors.Unwrap(newtc.output.(error)) != reason {
			t.testcase.err <- errIncorrect
			return
		}

		if reason == gen.ErrUnregistered {
			// must be alive
			if _, err := t.Node().ProcessInfo(pid); err != nil {
				t.testcase.err <- errIncorrect
				return
			}
		} else {
			if _, err := t.Node().ProcessInfo(pid); err != gen.ErrProcessUnknown {
				t.testcase.err <- errIncorrect
				return
			}

		}

		// wait for down message
		return
	}

	// must receive down message
	if m, ok := input.(gen.MessageDownAlias); ok {
		if m.Alias != t.testcase.output.(gen.Alias) {
			t.testcase.err <- errIncorrect
			return
		}

		reason := t.testcase.input.(error)
		if m.Reason != reason {
			t.testcase.err <- errIncorrect
			return
		}

		t.testcase.err <- nil
		return
	}

	t.testcase.err <- errIncorrect
}

// test implementations for monitoring event

func (t *t5) MonitoringEvent(input any) {
	if _, ok := input.(initcase); ok {
		pid, err := t.Spawn(factory_t5, gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}
		newtc := &testcase{"TargetEvent", nil, nil, make(chan error)}
		t.Send(pid, newtc)
		if err := newtc.wait(1); err != nil {
			t.testcase.err <- err
			return
		}

		name, ok := newtc.output.(gen.Atom)
		if ok == false {
			t.testcase.err <- errIncorrect
			return
		}

		if info, err := t.Node().ProcessInfo(t.PID()); err == nil {
			if len(info.MonitorsEvent) != 0 {
				t.Node().Kill(pid)
				t.testcase.err <- errIncorrect
				return
			}
		} else {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}

		event := gen.Event{Name: name, Node: t.Node().Name()}
		t.testcase.output = event

		if _, err := t.MonitorEvent(event); err != nil {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}

		if info, err := t.Node().ProcessInfo(t.PID()); err == nil {
			if len(info.MonitorsEvent) != 1 && info.MonitorsEvent[0] != event {
				t.Node().Kill(pid)
				t.testcase.err <- errIncorrect
				return
			}
		} else {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}

		reason := t.testcase.input.(error)
		switch reason {
		case gen.TerminateReasonPanic:
			// send it as a regular message. it will cause panic inside of the TargetAlias
			t.Send(pid, "custom panic")
			return
		case gen.TerminateReasonKill:
			t.Node().Kill(pid)
			return
		case gen.ErrUnregistered:
			t.Send(pid, event)
		default:
			t.SendExit(pid, reason)
		}

		if err := newtc.wait(1); err != nil {
			t.testcase.err <- err
			return
		}

		// Terminate callback should be invoked
		if errors.Unwrap(newtc.output.(error)) != reason {
			t.testcase.err <- errIncorrect
			return
		}

		if reason == gen.ErrUnregistered {
			// must be alive
			if _, err := t.Node().ProcessInfo(pid); err != nil {
				t.testcase.err <- errIncorrect
				return
			}
		} else {
			if _, err := t.Node().ProcessInfo(pid); err != gen.ErrProcessUnknown {
				t.testcase.err <- errIncorrect
				return
			}

		}

		// wait for down message
		return
	}

	// must receive down message
	if m, ok := input.(gen.MessageDownEvent); ok {
		if m.Event != t.testcase.output.(gen.Event) {
			t.testcase.err <- errIncorrect
			return
		}

		reason := t.testcase.input.(error)
		if m.Reason != reason {
			t.testcase.err <- errIncorrect
			return
		}
		t.testcase.err <- nil
		return
	}

	t.testcase.err <- errIncorrect
}
func (t *t5) TargetPID(input any) {
	if _, ok := input.(initcase); ok {
		t.testcase.err <- nil
		return
	}
	t.testcase = nil
	panic(input)
}

func (t *t5) TargetProcessID(input any) {
	if _, ok := input.(initcase); ok {
		name := gen.Atom(lib.RandomString(10))
		if err := t.RegisterName(name); err != nil {
			t.testcase.err <- err
			return
		}
		t.testcase.output = name
		t.testcase.err <- nil
		return
	}

	if _, ok := input.(int); ok {
		if err := t.UnregisterName(); err != nil {
			t.testcase.err <- err
			return
		}
		t.testcase.output = fmt.Errorf("bla: %w", gen.ErrUnregistered)
		t.testcase.err <- nil
		return
	}
	t.testcase = nil
	panic(input)
}

func (t *t5) TargetAlias(input any) {
	if _, ok := input.(initcase); ok {
		if alias, err := t.CreateAlias(); err != nil {
			t.testcase.err <- err
			return
		} else {
			t.testcase.output = alias
		}
		t.testcase.err <- nil
		return
	}

	if alias, ok := input.(gen.Alias); ok {
		if err := t.DeleteAlias(alias); err != nil {
			t.testcase.err <- err
			return
		}
		t.testcase.output = fmt.Errorf("%w", gen.ErrUnregistered)
		t.testcase.err <- nil
		return
	}
	t.testcase = nil
	panic(input)
}

func (t *t5) TargetEvent(input any) {
	if _, ok := input.(initcase); ok {
		name := gen.Atom(lib.RandomString(10))

		opts := gen.EventOptions{
			Notify: false,
			Buffer: 10,
		}
		if _, err := t.RegisterEvent(name, opts); err != nil {
			t.testcase.err <- err
			return
		} else {
			t.testcase.output = name
		}
		t.testcase.err <- nil
		return
	}

	if event, ok := input.(gen.Event); ok {
		if err := t.UnregisterEvent(event.Name); err != nil {
			t.testcase.err <- err
			return
		}
		t.testcase.output = fmt.Errorf("%w", gen.ErrUnregistered)
		t.testcase.err <- nil
		return
	}
	t.testcase = nil
	panic(input)
}

func TestT5ActorMonitor(t *testing.T) {
	nopt := gen.NodeOptions{}

	// to suppress Panic messages disable logging
	nopt.Log.DefaultLogger.Disable = true

	node, err := ergo.StartNode("t5node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t5, popt)
	if err != nil {
		panic(err)
	}

	t5cases = []*testcase{
		{"TestMonitorPID", nil, nil, make(chan error)},
		{"TestMonitorProcessID", nil, nil, make(chan error)},
		{"TestMonitorAlias", nil, nil, make(chan error)},
		{"TestMonitorEvent", nil, nil, make(chan error)},
		{"TestMonitorUnknown", nil, nil, make(chan error)},
	}
	for _, tc := range t5cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
