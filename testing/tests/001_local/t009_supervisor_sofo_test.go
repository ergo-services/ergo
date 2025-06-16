package local

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

var (
	t9casesSOFO []*testcase
)

func factory_t9_sofo() gen.ProcessBehavior {
	return &t9_sofo{}
}

type t9_sofo struct {
	act.Actor

	testcase *testcase
}

func (t *t9_sofo) HandleMessage(from gen.PID, message any) error {
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

func (t *t9_sofo) TestBasic(input any) {
	defer func() {
		t.testcase = nil
	}()

	// start supervisor
	pid, err := t.Spawn(factory_t9_sup_sofo_basic, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
	}
	defer t.Node().Kill(pid)

	childtc := &testcase{"TestBasicSOFO", nil, nil, make(chan error)}
	t.Send(pid, childtc)
	if err := childtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	t.Send(pid, 12345)
	if err := childtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	t.testcase.err <- nil
}

func (t *t9_sofo) TestStrategy(input any) {
	defer func() {
		t.testcase = nil
	}()

	// start supervisor with the given restart strategy
	pid, err := t.Spawn(factory_t9_sup_sofo, gen.ProcessOptions{}, t.testcase.input)
	if err != nil {
		t.testcase.err <- err
	}

	childtc := &testcase{"TestSOFO", 5, nil, make(chan error)}
	t.Send(pid, childtc)
	if err := childtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	reasons := []error{gen.TerminateReasonNormal, gen.TerminateReasonShutdown, gen.TerminateReasonKill}
	for _, reason := range reasons {
		t.Send(pid, reason)
		if err := childtc.wait(1); err != nil {
			t.testcase.err <- err
			return
		}
	}

	// send exit signal to the supervisor process
	exit := errors.New("my exit")
	t.SendExit(pid, exit)
	if err := childtc.wait(1); err != exit {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	t.testcase.err <- nil
}

// SOFO supervisors

// t9_sup_sofo_basic
func factory_t9_sup_sofo_basic() gen.ProcessBehavior {
	return &t9_sup_sofo_basic{}
}

type t9_sup_sofo_basic struct {
	act.Supervisor

	testcase *testcase
}

func (t *t9_sup_sofo_basic) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec
	spec.Type = act.SupervisorTypeSimpleOneForOne
	spec.Children = []act.SupervisorChildSpec{
		{
			Name:    "child_sofo",
			Factory: factory_t9_child_sofo,
		},
	}
	return spec, nil
}

func (t *t9_sup_sofo_basic) HandleMessage(from gen.PID, message any) error {
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

func (t *t9_sup_sofo_basic) TestBasicSOFO(input any) {
	if _, ok := input.(initcase); ok {
		// start child
		if err := t.StartChild("child_sofo"); err != nil {
			t.testcase.err <- err
			return
		}
		// check children
		children := t.Children()
		if len(children) != 1 {
			t.testcase.err <- errIncorrect
			return
		}

		// start child with unknown spec
		if err := t.StartChild("child_sofo1"); err != act.ErrSupervisorChildUnknown {
			t.testcase.err <- errIncorrect
			return
		}

		// check children. must be the same
		children1 := t.Children()
		if reflect.DeepEqual(children, children1) == false {
			t.testcase.err <- errIncorrect
			return
		}

		// add new child spec
		newChildSpec := act.SupervisorChildSpec{
			Name:    "child_sofo1",
			Factory: factory_t9_child_sofo,
		}
		if err := t.AddChild(newChildSpec); err != nil {
			t.testcase.err <- err
			return
		}

		// check children. must be the same
		children2 := t.Children()
		if reflect.DeepEqual(children, children2) == false {
			t.testcase.err <- errIncorrect
			return
		}

		// start child with new child spec
		if err := t.StartChild("child_sofo1"); err != nil {
			t.testcase.err <- err
			return
		}

		children3 := t.Children()
		if len(children3) != 2 {
			t.testcase.err <- errIncorrect
			return
		}

		if reflect.DeepEqual(children3[0], children[0]) == false {
			t.testcase.err <- errIncorrect
			return
		}

		// disable unknown child spec
		if err := t.DisableChild("child_sofo111"); err != act.ErrSupervisorChildUnknown {
			t.testcase.err <- errIncorrect
			return
		}

		// disable known child spec
		if err := t.DisableChild("child_sofo1"); err != nil {
			t.testcase.err <- errIncorrect
			return
		}

		t.testcase.err <- nil
		return
	}

	// try to start disabled child spec
	if err := t.StartChild("child_sofo1"); err != act.ErrSupervisorChildDisabled {
		t.testcase.err <- err
		return
	}
	t.testcase.err <- nil
}

//
// t9_sup_sofo
//

func factory_t9_sup_sofo() gen.ProcessBehavior {
	return &t9_sup_sofo{}
}

type t9_child_stop struct {
	pid    gen.PID
	reason error
}
type t9_sup_sofo struct {
	act.Supervisor

	strategy  act.SupervisorStrategy
	testcase  *testcase
	waitStop  []t9_child_stop
	waitStart []gen.Atom
}

func (t *t9_sup_sofo) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec
	spec.Type = act.SupervisorTypeSimpleOneForOne
	spec.Children = []act.SupervisorChildSpec{
		{
			Name:    "child_sofo",
			Factory: factory_t9_child_sofo,
		},
	}
	spec.Restart.Strategy = args[0].(act.SupervisorStrategy)
	spec.EnableHandleChild = true
	t.strategy = spec.Restart.Strategy
	return spec, nil
}

func (t *t9_sup_sofo) HandleChildStart(name gen.Atom, pid gen.PID) error {

	// shouldn't start before stopping
	if len(t.waitStop) > 0 {
		t.testcase.err <- errIncorrect
		return nil
	}
	waitName := t.waitStart[0]
	if waitName != name {
		t.testcase.err <- errIncorrect
		return nil
	}

	t.waitStart = t.waitStart[1:]
	if len(t.waitStart) > 0 {
		return nil
	}

	// done
	t.testcase.err <- nil
	return nil
}

func (t *t9_sup_sofo) HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error {
	if len(t.waitStop) == 0 {
		// do nothing
		return nil
	}

	stop := t.waitStop[0]
	if stop.pid != pid {
		t.testcase.err <- errIncorrect
		return nil
	}
	if stop.reason != reason {
		t.testcase.err <- errIncorrect
		return nil
	}
	t.waitStop = t.waitStop[1:]
	if len(t.waitStart) > 0 {
		return nil
	}
	if len(t.waitStop) > 0 {
		return nil
	}

	t.testcase.err <- nil
	return nil
}

func (t *t9_sup_sofo) Terminate(reason error) {
	select {
	case t.testcase.err <- reason:
	default:
	}
}

func (t *t9_sup_sofo) HandleMessage(from gen.PID, message any) error {
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

func (t *t9_sup_sofo) TestSOFO(input any) {
	if _, ok := input.(initcase); ok {
		n := t.testcase.input.(int)
		childrentc := &testcase{"TestChildrenSOFO", nil, nil, make(chan error, 5)}
		// start n child processes
		for i := 0; i < n; i++ {
			if err := t.StartChild("child_sofo", childrentc); err != nil {
				t.testcase.err <- err
				return
			}
			if err := childrentc.wait(1); err != nil {
				t.testcase.err <- err
				return
			}
		}

		t.waitStart = []gen.Atom{"child_sofo", "child_sofo", "child_sofo", "child_sofo", "child_sofo"}

		// check the number of children
		children := t.Children()
		if len(children) != n {
			t.testcase.err <- errIncorrect
			return
		}

		return
	}

	// terminate child process with a given reason
	reason := input.(error)
	children := t.Children()
	if len(children) == 0 {
		t.testcase.err <- errIncorrect
		return
	}
	if err := t.SendExit(children[0].PID, reason); err != nil {
		t.testcase.err <- err
		return
	}

	stop := t9_child_stop{
		pid:    children[0].PID,
		reason: reason,
	}
	t.waitStop = append(t.waitStop, stop)

	switch t.strategy {
	case act.SupervisorStrategyTransient:
		if reason == gen.TerminateReasonKill {
			t.waitStart = append(t.waitStart, children[0].Spec)
		}
	case act.SupervisorStrategyTemporary:
		break
	case act.SupervisorStrategyPermanent:
		t.waitStart = append(t.waitStart, children[0].Spec)
	}

}

//
// t9_child_sofo
//

func factory_t9_child_sofo() gen.ProcessBehavior {
	return &t9_child_sofo{}
}

type t9_child_sofo struct {
	act.Actor

	testcase *testcase
}

func (t *t9_child_sofo) Init(args ...any) error {
	if len(args) > 0 {
		t.testcase = args[0].(*testcase)
		t.testcase.err <- nil
	}
	return nil
}

func (t *t9_child_sofo) HandleMessage(from gen.PID, message any) error {
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

func (t *t9_child_sofo) TestChildrenSOFO(input any) {
	if _, ok := input.(initcase); ok {
		t.testcase.err <- nil
	}
	panic(input)
}

// tests
func TestT9SupervisorSOFO(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	node, err := ergo.StartNode("t9nodeSOFO@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t9_sofo, popt)
	if err != nil {
		panic(err)
	}

	t9casesSOFO = []*testcase{
		{"TestBasic", nil, nil, make(chan error)},
		{"TestStrategy", act.SupervisorStrategyTransient, nil, make(chan error)},
		{"TestStrategy", act.SupervisorStrategyTemporary, nil, make(chan error)},
		{"TestStrategy", act.SupervisorStrategyPermanent, nil, make(chan error)},
	}
	for _, tc := range t9casesSOFO {
		name := tc.name
		if tc.input != nil {
			name = fmt.Sprintf("%s:%s", tc.name, tc.input)
		}
		t.Run(name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(3); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
