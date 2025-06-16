package local

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

var (
	t4cases []*testcase
)

func factory_t4() gen.ProcessBehavior {
	return &t4{}
}

type forward struct {
	from    gen.PID
	ref     gen.Ref
	request any
}

type t4 struct {
	act.Actor

	testcase *testcase
}

func (t *t4) Init(args ...any) error {
	return nil
}

func (t *t4) HandleMessage(from gen.PID, message any) error {
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

func (t *t4) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if pid, ok := t.testcase.output.(gen.PID); ok {
		if err := t.Send(pid, forward{from, ref, request}); err != nil {
			return nil, err
		}
		// return nil to skip sending response. and stop this process
		return nil, gen.TerminateReasonNormal
	}
	// send response and terminate this process normaly
	return request, gen.TerminateReasonNormal
}

// test cases

func (t *t4) TestCallPID(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t4, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	// send new testcase for the spawned process
	newtc := &testcase{"PongPID", 1234, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	output, err := t.Call(pid, newtc.input)
	if err != nil {
		t.testcase.err <- err
		return
	}
	if reflect.DeepEqual(newtc.input, output) == false {
		t.testcase.err <- errIncorrect
		return
	}

	// must be stopped here
	t.testcase.waitForCondition(1*time.Second, func() bool {
		_, err := t.Node().ProcessInfo(pid)
		return err == gen.ErrProcessUnknown
	})

	t.testcase.err <- nil
}

func (t *t4) TestCallProcessID(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t4, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	// send new testcase for the spawned process
	newtc := &testcase{"PongProcessID", 1.234, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	// we must have process name in the output field
	if _, ok := newtc.output.(gen.Atom); ok == false {
		t.testcase.err <- errIncorrect
	}
	output, err := t.Call(newtc.output, newtc.input)
	if err != nil {
		t.testcase.err <- err
		return
	}
	if reflect.DeepEqual(newtc.input, output) == false {
		t.testcase.err <- errIncorrect
		return
	}

	// must be stopped here
	t.testcase.waitForCondition(1*time.Second, func() bool {
		_, err := t.Node().ProcessInfo(pid)
		return err == gen.ErrProcessUnknown
	})

	t.testcase.err <- nil
}

func (t *t4) TestCallAlias(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t4, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	// send new testcase for the spawned process
	newtc := &testcase{"PongAlias", "hello", nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	// we must have process alias in the output field
	if _, ok := newtc.output.(gen.Alias); ok == false {
		t.testcase.err <- errIncorrect
	}
	output, err := t.Call(newtc.output, newtc.input)
	if err != nil {
		t.testcase.err <- err
		return
	}
	if reflect.DeepEqual(newtc.input, output) == false {
		t.testcase.err <- errIncorrect
		return
	}

	// must be stopped here
	t.testcase.waitForCondition(1*time.Second, func() bool {
		_, err := t.Node().ProcessInfo(pid)
		return err == gen.ErrProcessUnknown
	})

	t.testcase.err <- nil
}

func (t *t4) TestCallForward(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t4, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	// send new testcase for the spawned process
	newtc := &testcase{"ForwardCall", 12.34, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	output, err := t.Call(pid, newtc.input)
	if err != nil {
		t.testcase.err <- err
		return
	}
	if reflect.DeepEqual(newtc.input, output) == false {
		t.testcase.err <- errIncorrect
		return
	}

	// must be stopped here
	t.testcase.waitForCondition(1*time.Second, func() bool {
		_, err := t.Node().ProcessInfo(pid)
		return err == gen.ErrProcessUnknown
	})

	t.testcase.err <- nil
}

func (t *t4) TestCallUnknown(input any) {
	defer func() {
		t.testcase = nil
	}()
	if _, err := t.Call(gen.Atom("unknown process"), 1); err != gen.ErrProcessUnknown {
		t.testcase.err <- errIncorrect
		return
	}
	t.testcase.err <- nil
}

func (t *t4) PongPID(input any) {
	if _, init := input.(initcase); init {
		t.testcase.err <- nil
		return
	}
	t.testcase.err <- errIncorrect
}

func (t *t4) PongProcessID(input any) {
	if _, init := input.(initcase); init {
		name := gen.Atom(lib.RandomString(10))
		if err := t.RegisterName(name); err != nil {
			t.testcase.err <- err
			return
		}
		t.testcase.output = name
		t.testcase.err <- nil
		return
	}
	t.testcase.err <- errIncorrect
}

func (t *t4) PongAlias(input any) {
	if _, init := input.(initcase); init {
		alias, err := t.CreateAlias()
		if err != nil {
			t.testcase.err <- err
			return
		}
		t.testcase.output = alias
		t.testcase.err <- nil
		return
	}
	t.testcase.err <- errIncorrect

}

func (t *t4) ForwardCall(input any) {
	if _, init := input.(initcase); init == false {
		t.testcase.err <- errIncorrect
		return
	}
	pid, err := t.Spawn(factory_t4, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"PongForwardCall", nil, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	t.testcase.output = pid
	t.testcase.err <- nil
}

func (t *t4) PongForwardCall(input any) {
	if _, init := input.(initcase); init {
		t.testcase.err <- nil
		return
	}

	fwd := input.(forward)
	if err := t.SendResponse(fwd.from, fwd.ref, fwd.request); err != nil {
		t.testcase.err <- err
		return
	}

	return
}

func TestT4ActorCall(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	node, err := ergo.StartNode("t4node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t4, popt)
	if err != nil {
		panic(err)
	}

	t4cases = []*testcase{
		{"TestCallPID", nil, nil, make(chan error)},
		{"TestCallProcessID", nil, nil, make(chan error)},
		{"TestCallAlias", nil, nil, make(chan error)},
		{"TestCallForward", nil, nil, make(chan error)},
		{"TestCallUnknown", nil, nil, make(chan error)},
	}
	for _, tc := range t4cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
