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
	t6cases []*testcase
)

func factory_t6() gen.ProcessBehavior {
	return &t6{}
}

type t6 struct {
	act.Actor

	testcase *testcase
}

func (t *t6) Init(args ...any) error {
	return nil
}

func (t *t6) HandleMessage(from gen.PID, message any) error {
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

func (t *t6) Terminate(reason error) {
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

// test methods

func (t *t6) TestLinkPID(input any) {
	defer func() {
		t.testcase = nil
	}()

	for _, trapexit := range []bool{false, true} {
		for _, reason := range []error{gen.TerminateReasonKill, errors.New("custom"), gen.TerminateReasonShutdown} {
			pid, err := t.Spawn(factory_t6, gen.ProcessOptions{})
			if err != nil {
				t.testcase.err <- err
				return
			}

			newtc := &testcase{"LinkingPID", trapexit, nil, make(chan error)}
			t.Send(pid, newtc)
			if err := newtc.wait(1); err != nil {
				t.Node().Kill(pid)
				t.testcase.err <- err
				return
			}
			t.Send(pid, reason)

			if err := newtc.wait(1); err != nil {
				t.Node().Kill(pid)
				t.testcase.err <- err
				return
			}

			// check newtc.output
			if trapexit {
				exit := newtc.output.(gen.MessageExitPID)
				if exit.Reason != reason {
					t.Node().Kill(pid)
					t.testcase.err <- errIncorrect
					return
				}
			} else {
				e := errors.Unwrap(newtc.output.(error))
				if e != reason {
					t.Node().Kill(pid)
					t.testcase.err <- errIncorrect
					return
				}
			}

			t.Node().Kill(pid)
		}
	}
	t.testcase.err <- nil
}

func (t *t6) TestLinkProcessID(input any) {
	defer func() {
		t.testcase = nil
	}()
	for _, trapexit := range []bool{false, true} {
		for _, reason := range []error{gen.TerminateReasonKill, errors.New("custom"), gen.TerminateReasonShutdown, gen.ErrUnregistered} {
			pid, err := t.Spawn(factory_t6, gen.ProcessOptions{})
			if err != nil {
				t.testcase.err <- err
				return
			}

			newtc := &testcase{"LinkingProcessID", trapexit, nil, make(chan error)}
			t.Send(pid, newtc)
			if err := newtc.wait(1); err != nil {
				t.Node().Kill(pid)
				t.testcase.err <- err
				return
			}
			t.Send(pid, reason)

			if err := newtc.wait(1); err != nil {
				t.Node().Kill(pid)
				t.testcase.err <- err
				return
			}

			// check newtc.output
			if trapexit {
				exit := newtc.output.(gen.MessageExitProcessID)
				if exit.Reason != reason {
					t.Node().Kill(pid)
					t.testcase.err <- errIncorrect
					return
				}
			} else {
				e := errors.Unwrap(newtc.output.(error))
				if e != reason {
					t.Node().Kill(pid)
					t.testcase.err <- errIncorrect
					return
				}
			}

			t.Node().Kill(pid)
		}
	}
	t.testcase.err <- nil
}

func (t *t6) TestLinkAlias(input any) {
	defer func() {
		t.testcase = nil
	}()
	for _, trapexit := range []bool{false, true} {
		for _, reason := range []error{gen.TerminateReasonKill, errors.New("custom"), gen.TerminateReasonShutdown, gen.ErrUnregistered} {
			pid, err := t.Spawn(factory_t6, gen.ProcessOptions{})
			if err != nil {
				t.testcase.err <- err
				return
			}

			newtc := &testcase{"LinkingAlias", trapexit, nil, make(chan error)}
			t.Send(pid, newtc)
			if err := newtc.wait(1); err != nil {
				t.Node().Kill(pid)
				t.testcase.err <- err
				return
			}
			t.Send(pid, reason)

			if err := newtc.wait(1); err != nil {
				t.Node().Kill(pid)
				t.testcase.err <- err
				return
			}

			// check newtc.output
			if trapexit {
				exit := newtc.output.(gen.MessageExitAlias)
				if exit.Reason != reason {
					t.Node().Kill(pid)
					t.testcase.err <- errIncorrect
					return
				}
			} else {
				e := errors.Unwrap(newtc.output.(error))
				if e != reason {
					t.Node().Kill(pid)
					t.testcase.err <- errIncorrect
					return
				}
			}

			t.Node().Kill(pid)
		}
	}
	t.testcase.err <- nil
}

func (t *t6) TestLinkEvent(input any) {
	defer func() {
		t.testcase = nil
	}()
	for _, trapexit := range []bool{false, true} {
		for _, reason := range []error{gen.TerminateReasonKill, errors.New("custom"), gen.TerminateReasonShutdown, gen.ErrUnregistered} {
			pid, err := t.Spawn(factory_t6, gen.ProcessOptions{})
			if err != nil {
				t.testcase.err <- err
				return
			}

			newtc := &testcase{"LinkingEvent", trapexit, nil, make(chan error)}
			t.Send(pid, newtc)
			if err := newtc.wait(1); err != nil {
				t.Node().Kill(pid)
				t.testcase.err <- err
				return
			}
			t.Send(pid, reason)

			if err := newtc.wait(1); err != nil {
				t.Node().Kill(pid)
				t.testcase.err <- err
				return
			}

			// check newtc.output
			if trapexit {
				exit := newtc.output.(gen.MessageExitEvent)
				if exit.Reason != reason {
					t.Node().Kill(pid)
					t.testcase.err <- errIncorrect
					return
				}
			} else {
				e := errors.Unwrap(newtc.output.(error))
				if e != reason {
					t.Node().Kill(pid)
					t.testcase.err <- errIncorrect
					return
				}
			}

			t.Node().Kill(pid)
		}
	}
	t.testcase.err <- nil
}

func (t *t6) TestLinkUnknown(input any) {
	defer func() {
		t.testcase = nil
	}()
	// unknown PID
	pid := t.PID()
	pid.ID += 10000
	if err := t.LinkPID(pid); err != gen.ErrProcessUnknown {
		t.testcase.err <- errIncorrect
		return
	}

	// unknown ProcessID
	pname := gen.ProcessID{Name: "test", Node: t.Node().Name()}
	if err := t.LinkProcessID(pname); err != gen.ErrProcessUnknown {
		t.testcase.err <- errIncorrect
		return
	}

	// unknown Alias
	alias := gen.Alias{Node: t.Node().Name()}
	if err := t.LinkAlias(alias); err != gen.ErrAliasUnknown {
		t.testcase.err <- errIncorrect
		return
	}

	// unknown Event
	event := gen.Event{Name: "test"}
	if _, err := t.LinkEvent(event); err != gen.ErrEventUnknown {
		t.testcase.err <- errIncorrect
		return
	}

	t.testcase.err <- nil
}

func (t *t6) LinkingPID(input any) {

	if _, ok := input.(initcase); ok {
		t.SetTrapExit(t.testcase.input.(bool))

		// start child
		pid, err := t.Spawn(factory_t6, gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}
		newtc := &testcase{"TargetPID", nil, nil, make(chan error)}
		t.Send(pid, newtc)
		if err := newtc.wait(1); err != nil {
			t.testcase.err <- err
			return
		}

		// link with it
		if err := t.LinkPID(pid); err != nil {
			t.testcase.err <- err
			return
		}
		if info, e := t.Info(); e != nil || len(info.LinksPID) != 1 {
			t.testcase.err <- errIncorrect
			return
		} else {
			if info.LinksPID[0] != pid {
				t.testcase.err <- errIncorrect
				return
			}
		}
		t.testcase.input = pid
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	}

	switch m := input.(type) {
	case error:
		// got exit reason for the child
		if m == gen.TerminateReasonKill {
			pid := t.testcase.input.(gen.PID)
			t.Node().Kill(pid)
			return
		} else {
			pid := t.testcase.input.(gen.PID)
			t.SendExit(pid, m)
			return
		}
	case gen.MessageExitPID:
		// if trap exit is true we receive gen.MessageExitPID
		t.testcase.output = m
		t.testcase.err <- nil
		return

	}
	panic(input)
}

func (t *t6) LinkingProcessID(input any) {

	if _, ok := input.(initcase); ok {
		t.SetTrapExit(t.testcase.input.(bool))

		// start child
		pid, err := t.Spawn(factory_t6, gen.ProcessOptions{})
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

		// link with it
		pname := gen.ProcessID{Name: name, Node: t.Node().Name()}
		if err := t.LinkProcessID(pname); err != nil {
			t.testcase.err <- err
			return
		}
		if info, e := t.Info(); e != nil || len(info.LinksProcessID) != 1 {
			t.testcase.err <- errIncorrect
			return
		} else {
			if info.LinksProcessID[0] != pname {
				t.testcase.err <- errIncorrect
				return
			}
		}
		t.testcase.input = pid
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	}

	switch m := input.(type) {
	case error:
		pid := t.testcase.input.(gen.PID)
		switch m {
		case gen.TerminateReasonKill:
			t.Node().Kill(pid)
			return
		case gen.ErrUnregistered:
			t.Send(pid, 1)
			return
		default:
			t.SendExit(pid, m)
			return
		}

	case gen.MessageExitProcessID:
		// if trap exit is true we receive gen.MessageExitPID
		t.testcase.output = m
		t.testcase.err <- nil
		return

	}
	panic(input)
}

func (t *t6) LinkingAlias(input any) {

	if _, ok := input.(initcase); ok {
		t.SetTrapExit(t.testcase.input.(bool))

		// start child
		pid, err := t.Spawn(factory_t6, gen.ProcessOptions{})
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

		// link with it
		if err := t.LinkAlias(alias); err != nil {
			t.testcase.err <- err
			return
		}
		if info, e := t.Info(); e != nil || len(info.LinksAlias) != 1 {
			t.testcase.err <- errIncorrect
			return
		} else {
			if info.LinksAlias[0] != alias {
				t.testcase.err <- errIncorrect
				return
			}
		}
		t.testcase.input = pid
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	}

	switch m := input.(type) {
	case error:
		pid := t.testcase.input.(gen.PID)
		switch m {
		case gen.TerminateReasonKill:
			t.Node().Kill(pid)
			return
		case gen.ErrUnregistered:
			t.Send(pid, 1)
			return
		default:
			t.SendExit(pid, m)
			return
		}

	case gen.MessageExitAlias:
		// if trap exit is true we receive gen.MessageExitAlias
		t.testcase.output = m
		t.testcase.err <- nil
		return

	}
	panic(input)
}

func (t *t6) LinkingEvent(input any) {

	if _, ok := input.(initcase); ok {
		t.SetTrapExit(t.testcase.input.(bool))

		// start child
		pid, err := t.Spawn(factory_t6, gen.ProcessOptions{})
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
		event := gen.Event{Name: name, Node: t.Node().Name()}

		// link with it
		if _, err := t.LinkEvent(event); err != nil {
			t.testcase.err <- err
			return
		}
		if info, e := t.Info(); e != nil || len(info.LinksEvent) != 1 {
			t.testcase.err <- errIncorrect
			return
		} else {
			if info.LinksEvent[0] != event {
				t.testcase.err <- errIncorrect
				return
			}
		}
		t.testcase.input = pid
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	}

	switch m := input.(type) {
	case error:
		pid := t.testcase.input.(gen.PID)
		switch m {
		case gen.TerminateReasonKill:
			t.Node().Kill(pid)
			return
		case gen.ErrUnregistered:
			t.Send(pid, 1)
			return
		default:
			t.SendExit(pid, m)
			return
		}

	case gen.MessageExitEvent:
		// if trap exit is true we receive gen.MessageExitAlias
		t.testcase.output = m
		t.testcase.err <- nil
		return

	}
	panic(input)
}

func (t *t6) TargetPID(input any) {
	if _, ok := input.(initcase); ok {
		t.testcase.err <- nil
		return
	}
	panic(input)
}

func (t *t6) TargetProcessID(input any) {
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
			panic(err)
		}
		return
	}
	panic(input)
}

func (t *t6) TargetAlias(input any) {
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

	if _, ok := input.(int); ok {
		alias := t.testcase.output.(gen.Alias)
		if err := t.DeleteAlias(alias); err != nil {
			t.testcase.err <- err
			return
		}
		return
	}
	panic(input)
}

func (t *t6) TargetEvent(input any) {
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

	if _, ok := input.(int); ok {
		name := t.testcase.output.(gen.Atom)
		if err := t.UnregisterEvent(name); err != nil {
			t.testcase.err <- err
			return
		}
		return
	}
	panic(input)
}

func TestT6ActorLink(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	node, err := ergo.StartNode("t6node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t6, popt)
	if err != nil {
		panic(err)
	}

	t6cases = []*testcase{
		{"TestLinkPID", nil, nil, make(chan error)},
		{"TestLinkProcessID", nil, nil, make(chan error)},
		{"TestLinkAlias", nil, nil, make(chan error)},
		{"TestLinkEvent", nil, nil, make(chan error)},
		{"TestLinkUnknown", nil, nil, make(chan error)},
	}
	for _, tc := range t6cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
