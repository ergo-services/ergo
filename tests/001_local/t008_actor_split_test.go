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
	t8cases []*testcase
)

func factory_t8() gen.ProcessBehavior {
	return &t8{}
}

type t8 struct {
	act.Actor

	testcase *testcase
}

func (t *t8) Init(args ...any) error {
	return nil
}

func (t *t8) HandleMessage(from gen.PID, message any) error {
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

func (t *t8) HandleMessageName(name gen.Atom, from gen.PID, message any) error {
	t.testcase.output = name
	t.testcase.err <- nil
	return nil
}

func (t *t8) HandleMessageAlias(alias gen.Alias, from gen.PID, message any) error {
	t.testcase.output = alias
	t.testcase.err <- nil
	return nil
}

func (t *t8) HandleCall(from gen.PID, ref gen.Ref, message any) (any, error) {
	return t.PID(), nil
}

func (t *t8) HandleCallName(name gen.Atom, from gen.PID, ref gen.Ref, message any) (any, error) {
	return name, nil
}

func (t *t8) HandleCallAlias(alias gen.Alias, from gen.PID, ref gen.Ref, message any) (any, error) {
	return alias, nil
}

//
// test methods
//

func (t *t8) TestSplitHandle(input any) {
	defer func() {
		t.testcase = nil
	}()

	// set/unset split
	if t.SplitHandle() != false {
		t.testcase.err <- errIncorrect
		return
	}
	t.SetSplitHandle(true)
	if t.SplitHandle() != true {
		t.testcase.err <- errIncorrect
		return
	}
	t.SetSplitHandle(false)
	if t.SplitHandle() != false {
		t.testcase.err <- errIncorrect
		return
	}

	for _, split := range []bool{false, true} {
		name := gen.Atom(lib.RandomString(10))
		pid, err := t.SpawnRegister(name, factory_t8, gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}
		defer t.Node().Kill(pid)

		targetTC := &testcase{"TargetSplit", split, nil, make(chan error)}
		t.Send(pid, targetTC)
		if err := targetTC.wait(1); err != nil {
			t.testcase.err <- err
			return
		}

		target := []any{pid, name, targetTC.output.(gen.Alias)}
		for _, tar := range target {
			if err := t.Send(tar, "test"); err != nil {
				t.testcase.err <- err
				return
			}

			if err := targetTC.wait(1); err != nil {
				t.testcase.err <- err
				return
			}

			x := tar
			if split == false {
				// must be HandleMessage(...) invoked
				x = pid
			}
			if reflect.DeepEqual(x, targetTC.output) == false {
				t.testcase.err <- errIncorrect
				return
			}
			res, err := t.Call(tar, "test")
			if err != nil {
				t.testcase.err <- err
				return
			}

			if reflect.DeepEqual(x, res) == false {
				t.testcase.err <- errIncorrect
				return
			}
		}
	}

	t.testcase.err <- nil
}

func (t *t8) TargetSplit(input any) {
	if _, ok := input.(initcase); ok {
		alias, err := t.CreateAlias()
		if err != nil {
			t.testcase.err <- err
			return
		}
		split := t.testcase.input.(bool)
		t.SetSplitHandle(split)

		t.testcase.output = alias
		t.testcase.err <- nil
		return
	}

	t.testcase.output = t.PID()
	t.testcase.err <- nil
}

func TestT8ActorSplit(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	node, err := ergo.StartNode("t8node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t8, popt)
	if err != nil {
		panic(err)
	}

	t8cases = []*testcase{
		{"TestSplitHandle", nil, nil, make(chan error)},
	}
	for _, tc := range t8cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
