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
	t3cases []*testcase
)

func factory_t3() gen.ProcessBehavior {
	return &t3{}
}

type t3 struct {
	act.Actor

	testcase *testcase
}

func (t *t3) Init(args ...any) error {
	return nil
}

func (t *t3) HandleMessage(from gen.PID, message any) error {
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

//
// test methods
//

func (t *t3) TestSendPID(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t3, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}
	defer t.Node().Kill(pid)

	// send the new testcase for the spawned process
	newtc := &testcase{"PongPID", 1.234, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	t.Send(pid, newtc.input)
	if err := newtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	if reflect.DeepEqual(newtc.input, newtc.output) == false {
		t.testcase.err <- errIncorrect
		return
	}
	t.testcase.err <- nil
}

func (t *t3) TestSendProcessID(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t3, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}
	defer t.Node().Kill(pid)

	// send the new testcase for the spawned process
	newtc := &testcase{"PongProcessID", 1234, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	// as a result newtc.output should have a process name here
	t.Send(newtc.output, newtc.input)
	if err := newtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	// as a final result newtc.output should have the same value we sent (input value)
	if reflect.DeepEqual(newtc.input, newtc.output) == false {
		t.testcase.err <- errIncorrect
		return
	}
	t.testcase.err <- nil
}

func (t *t3) TestSendAlias(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t3, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}
	defer t.Node().Kill(pid)

	newtc := &testcase{"PongAlias", "hello", nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	// as a result newtc.output should have the alias of this process here
	t.Send(newtc.output, newtc.input)
	if err := newtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	// as a final result newtc.output should have the same value we sent (input value)
	if reflect.DeepEqual(newtc.input, newtc.output) == false {
		t.testcase.err <- errIncorrect
		return
	}
	t.testcase.err <- nil
}
func (t *t3) TestSendUnknown(input any) {
	defer func() {
		t.testcase = nil
	}()

	if err := t.Send(gen.Atom("unknown process"), 1); err != gen.ErrProcessUnknown {
		t.testcase.err <- errIncorrect
		return
	}
	t.testcase.err <- nil
}

func (t *t3) PongPID(input any) {
	if _, init := input.(initcase); init {
		t.testcase.err <- nil
		return
	}

	// received message by pid. check this input with the testcase.input
	if reflect.DeepEqual(t.testcase.input, input) == false {
		t.testcase.err <- errIncorrect
		return
	}

	// pong the input
	t.testcase.output = input
	t.testcase.err <- nil
}

func (t *t3) PongProcessID(input any) {
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
	// received message by process name. check this input with the testcase.input
	if reflect.DeepEqual(t.testcase.input, input) == false {
		t.testcase.err <- errIncorrect
		return
	}

	// pong the input
	t.testcase.output = input
	t.testcase.err <- nil
}

func (t *t3) PongAlias(input any) {
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

	// received message by process alias. check this input with the testcase.input
	if reflect.DeepEqual(t.testcase.input, input) == false {
		t.testcase.err <- errIncorrect
		return
	}
	t.testcase.output = t.testcase.input
	t.testcase.err <- nil
}

func TestT3ActorSend(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	node, err := ergo.StartNode("t3node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t3, popt)
	if err != nil {
		panic(err)
	}

	t3cases = []*testcase{
		{"TestSendPID", nil, nil, make(chan error)},
		{"TestSendProcessID", nil, nil, make(chan error)},
		{"TestSendAlias", nil, nil, make(chan error)},
		{"TestSendUnknown", nil, nil, make(chan error)},
	}
	for _, tc := range t3cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
