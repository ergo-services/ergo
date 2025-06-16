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
	t10casesOFO []*testcase
)

func factory_t10_ofo() gen.ProcessBehavior {
	return &t10_ofo{}
}

type t10_ofo struct {
	act.Actor

	testcase *testcase
}

func (t *t10_ofo) HandleMessage(from gen.PID, message any) error {
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

func (t *t10_ofo) TestBasic(input any) {
	defer func() {
		t.testcase = nil
	}()

	// start supervisor
	pid, err := t.Spawn(factory_t10_sup_ofo_basic, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
	}
	defer t.Node().Kill(pid)

	childtc := &testcase{"TestBasicOFO", nil, nil, make(chan error)}
	t.Send(pid, childtc)
	if err := childtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	// to test starting disabled spec. send random message
	t.Send(pid, 12345)
	if err := childtc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	t.testcase.err <- nil
}

func (t *t10_ofo) TestStrategy(input any) {
	defer func() {
		t.testcase = nil
	}()

	// start supervisor with the given restart strategy
	pid, err := t.Spawn(factory_t10_sup_ofo, gen.ProcessOptions{}, t.testcase.input)
	if err != nil {
		t.testcase.err <- err
	}

	childtc := &testcase{"TestOFO", nil, nil, make(chan error)}
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

func (t *t10_ofo) TestSignificant(input any) {
	defer func() {
		t.testcase = nil
	}()

	strategy := t.testcase.input.(act.SupervisorStrategy)
	// start supervisor with the given restart strategy
	pid, err := t.Spawn(factory_t10_sup_ofo_significant, gen.ProcessOptions{}, strategy)
	if err != nil {
		t.testcase.err <- err
	}
	childtc := &testcase{"TestOFOSignificant", nil, nil, make(chan error)}
	t.Send(pid, childtc)
	if err := childtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	switch strategy {
	case act.SupervisorStrategyTransient:
		reason := errors.New("custom reason1")
		t.Send(pid, reason)
		// supervisor will use this reason for SendExit to the significant child process.
		// due to abnormal reason and transient strategy the child process will be restarted
		// and 'wait' should return nil
		if err := childtc.wait(1); err != nil {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}
		reason = gen.TerminateReasonNormal
		// this reason makes the significant child process to be terminated with normal reason
		// wich makes the supervisor shutdown with the same reason
		t.Send(pid, reason)
		if err := childtc.wait(1); err != gen.TerminateReasonNormal {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}
		t.testcase.err <- nil

	case act.SupervisorStrategyTemporary:
		reason := errors.New("custom reason2")
		// any reason will cause supervisor termination with the reason of
		// significant child termination
		t.Send(pid, reason)
		if err := childtc.wait(1); err != reason {
			t.Node().Kill(pid)
			t.testcase.err <- err
			return
		}
		t.testcase.err <- nil

	case act.SupervisorStrategyPermanent:
		reasons := []error{
			gen.TerminateReasonNormal,
			gen.TerminateReasonShutdown,
			errors.New("cusrom reason3"),
		}
		for _, reason := range reasons {
			t.Send(pid, reason)
			if err := childtc.wait(1); err != nil {
				t.Node().Kill(pid)
				t.testcase.err <- err
				return
			}

		}
		t.testcase.err <- nil

	}
}

// t10_sup_ofo_basic
func factory_t10_sup_ofo_basic() gen.ProcessBehavior {
	return &t10_sup_ofo_basic{}
}

type t10_sup_ofo_basic struct {
	act.Supervisor

	testcase *testcase
}

func (t *t10_sup_ofo_basic) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec
	spec.Type = act.SupervisorTypeOneForOne
	spec.Children = []act.SupervisorChildSpec{
		{
			Name:    "child_ofo",
			Factory: factory_t10_child_ofo,
		},
		{
			Name:    "child_ofo1",
			Factory: factory_t10_child_ofo,
		},
		{
			Name:    "child_ofo2",
			Factory: factory_t10_child_ofo,
		},
	}
	return spec, nil
}

func (t *t10_sup_ofo_basic) HandleMessage(from gen.PID, message any) error {
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

func (t *t10_sup_ofo_basic) TestBasicOFO(input any) {
	if _, ok := input.(initcase); ok {
		// check children
		children := t.Children()
		if len(children) != 3 {
			t.testcase.err <- errIncorrect
			return
		}

		// all children should have registered name (the same as a spec one)
		for _, c := range children {
			if c.Spec != c.Name {
				t.testcase.err <- errIncorrect
				return
			}
		}

		// start child
		if err := t.StartChild("child_ofo"); err != act.ErrSupervisorChildRunning {
			t.testcase.err <- err
			return
		}
		// check children. must be the same
		if reflect.DeepEqual(children, t.Children()) == false {
			t.testcase.err <- errIncorrect
			return
		}

		// start child with unknown spec
		if err := t.StartChild("child_ofo111"); err != act.ErrSupervisorChildUnknown {
			t.testcase.err <- errIncorrect
			return
		}

		// check children. must be the same
		if reflect.DeepEqual(children, t.Children()) == false {
			t.testcase.err <- errIncorrect
			return
		}

		// add new child spec
		newChildSpec := act.SupervisorChildSpec{
			Name:    "child_ofo3",
			Factory: factory_t10_child_ofo,
		}
		if err := t.AddChild(newChildSpec); err != nil {
			t.testcase.err <- err
			return
		}

		children = t.Children()
		if len(children) != 4 {
			t.testcase.err <- errIncorrect
			return
		}
		// it should have registered name
		if children[3].Spec != children[3].Name {
			t.testcase.err <- errIncorrect
			return
		}

		if err := t.StartChild("child_ofo3"); err != act.ErrSupervisorChildRunning {
			t.testcase.err <- err
			return
		}
		// check children. must be the same
		if reflect.DeepEqual(children, t.Children()) == false {
			t.testcase.err <- errIncorrect
			return
		}

		// disable unknown child spec
		if err := t.DisableChild("child_ofo111"); err != act.ErrSupervisorChildUnknown {
			t.testcase.err <- errIncorrect
			return
		}

		// disable known child spec
		if err := t.DisableChild("child_ofo"); err != nil {
			t.testcase.err <- errIncorrect
			return
		}

		children[0].Disabled = true

		// we do not wait the child termination. just align the expected values
		children[0].PID = gen.PID{}
		children1 := t.Children()
		children1[0].PID = gen.PID{}
		// check children. must be the same
		if reflect.DeepEqual(children, children1) == false {
			t.testcase.err <- errIncorrect
			return
		}

		t.testcase.err <- nil
		return
	}

	// try to start disabled child spec
	if err := t.StartChild("child_ofo"); err != act.ErrSupervisorChildDisabled {
		t.testcase.err <- err
		return
	}
	t.testcase.err <- nil
}

// t10_sup_ofo
func factory_t10_sup_ofo() gen.ProcessBehavior {
	return &t10_sup_ofo{}
}

type t10_child_stop struct {
	pid    gen.PID
	reason error
}

type t10_sup_ofo struct {
	act.Supervisor

	strategy  act.SupervisorStrategy
	testcase  *testcase
	waitStop  []t10_child_stop
	waitStart []gen.Atom
}

func (t *t10_sup_ofo) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec
	spec.Type = act.SupervisorTypeOneForOne
	spec.Children = []act.SupervisorChildSpec{
		{
			Name:    "child_ofo10",
			Factory: factory_t10_child_ofo,
		},
		{
			Name:    "child_ofo11",
			Factory: factory_t10_child_ofo,
		},
		{
			Name:    "child_ofo12",
			Factory: factory_t10_child_ofo,
		},
	}
	spec.Restart.Strategy = args[0].(act.SupervisorStrategy)
	spec.EnableHandleChild = true
	spec.DisableAutoShutdown = true
	t.strategy = spec.Restart.Strategy

	t.waitStart = []gen.Atom{"child_ofo10", "child_ofo11", "child_ofo12"}
	return spec, nil
}

func (t *t10_sup_ofo) HandleChildStart(name gen.Atom, pid gen.PID) error {
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
	if t.testcase != nil {
		t.testcase.err <- nil
	}
	return nil
}

func (t *t10_sup_ofo) HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error {
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

func (t *t10_sup_ofo) Terminate(reason error) {
	//var empty gen.PID
	select {
	case t.testcase.err <- reason:
	default:
	}
}

func (t *t10_sup_ofo) HandleMessage(from gen.PID, message any) error {
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

func (t *t10_sup_ofo) TestOFO(input any) {
	if _, ok := input.(initcase); ok {
		// check children
		children := t.Children()
		if len(children) != 3 {
			t.testcase.err <- errIncorrect
			return
		}

		t.testcase.err <- nil
		return
	}

	// terminate child process with a given reason
	reason := input.(error)
	children := t.Children()
	if len(children) == 0 {
		t.testcase.err <- errIncorrect
		return
	}
	empty := gen.PID{}
	for _, c := range children {
		if c.PID == empty {
			continue
		}
		if err := t.SendExit(c.PID, reason); err != nil {
			t.testcase.err <- err
			return
		}

		stop := t10_child_stop{
			pid:    c.PID,
			reason: reason,
		}
		t.waitStop = append(t.waitStop, stop)

		switch t.strategy {
		case act.SupervisorStrategyTransient:
			if reason == gen.TerminateReasonKill {
				t.waitStart = append(t.waitStart, c.Spec)
			}
		case act.SupervisorStrategyTemporary:
			break
		case act.SupervisorStrategyPermanent:
			t.waitStart = append(t.waitStart, c.Spec)
		}
		return
	}
	panic("shouldn't be here")
}

// t10_sup_ofo_significant
func factory_t10_sup_ofo_significant() gen.ProcessBehavior {
	return &t10_sup_ofo_significant{}
}

type t10_sup_ofo_significant struct {
	act.Supervisor

	strategy  act.SupervisorStrategy
	testcase  *testcase
	waitStop  []t10_child_stop
	waitStart []gen.Atom
}

func (t *t10_sup_ofo_significant) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec
	spec.Type = act.SupervisorTypeOneForOne
	spec.Children = []act.SupervisorChildSpec{
		{
			Name:    "child_ofo100",
			Factory: factory_t10_child_ofo,
		},
		{
			Name:        "child_ofo110",
			Significant: true,
			Factory:     factory_t10_child_ofo,
		},
	}
	spec.Restart.Strategy = args[0].(act.SupervisorStrategy)
	spec.EnableHandleChild = true
	t.strategy = spec.Restart.Strategy
	t.waitStart = []gen.Atom{"child_ofo100", "child_ofo110"}
	return spec, nil
}

func (t *t10_sup_ofo_significant) HandleMessage(from gen.PID, message any) error {
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

func (t *t10_sup_ofo_significant) HandleChildStart(name gen.Atom, pid gen.PID) error {
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
	if t.testcase != nil {
		t.testcase.err <- nil
	}
	return nil
}

func (t *t10_sup_ofo_significant) HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error {
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

func (t *t10_sup_ofo_significant) TestOFOSignificant(input any) {
	if _, ok := input.(initcase); ok {
		// check children
		children := t.Children()
		if len(children) != 2 {
			t.testcase.err <- errIncorrect
			return
		}

		t.testcase.err <- nil
		return
	}

	// terminate significant child with a given reason
	reason := input.(error)
	children := t.Children()
	c := children[1]
	if c.Significant != true {
		t.Log().Error("it is not a significant spec")
		t.testcase.err <- errIncorrect
		return
	}
	if err := t.SendExit(c.PID, reason); err != nil {
		t.testcase.err <- err
		return
	}

	stop := t10_child_stop{
		pid:    c.PID,
		reason: reason,
	}
	t.waitStop = append(t.waitStop, stop)

	switch t.strategy {
	case act.SupervisorStrategyTransient:
		if reason != gen.TerminateReasonNormal && reason != gen.TerminateReasonShutdown {
			t.waitStart = append(t.waitStart, c.Spec)
		}
	case act.SupervisorStrategyTemporary:
		break
	case act.SupervisorStrategyPermanent:
		t.waitStart = append(t.waitStart, c.Spec)
	}
}

func (t *t10_sup_ofo_significant) Terminate(reason error) {
	select {
	case t.testcase.err <- reason:
	default:
	}
}

//
// t10_child_ofo
//

func factory_t10_child_ofo() gen.ProcessBehavior {
	return &t10_child_ofo{}
}

type t10_child_ofo struct {
	act.Actor
}

// tests
func TestT10SupervisorOFO(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	//nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t10nodeOFO@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t10_ofo, popt)
	if err != nil {
		panic(err)
	}

	t10casesOFO = []*testcase{
		{"TestBasic", nil, nil, make(chan error)},
		{"TestStrategy", act.SupervisorStrategyTransient, nil, make(chan error)},
		{"TestStrategy", act.SupervisorStrategyTemporary, nil, make(chan error)},
		{"TestStrategy", act.SupervisorStrategyPermanent, nil, make(chan error)},
		{"TestSignificant", act.SupervisorStrategyTransient, nil, make(chan error)},
		{"TestSignificant", act.SupervisorStrategyTemporary, nil, make(chan error)},
		{"TestSignificant", act.SupervisorStrategyPermanent, nil, make(chan error)},
		//TODO {"TestAutoShutdown", nil, nil, make(chan error)},
		//TODO {"TestRestartIntensity", nil, nil, make(chan error)},
	}
	for _, tc := range t10casesOFO {
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
