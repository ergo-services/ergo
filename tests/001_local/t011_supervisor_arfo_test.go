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
	t11casesARFO []*testcase
)

func factory_t11_arfo() gen.ProcessBehavior {
	return &t11_arfo{}
}

type t11_arfo struct {
	act.Actor

	supType  act.SupervisorType
	testcase *testcase
}

func (t *t11_arfo) Init(args ...any) error {
	supType := args[0].(act.SupervisorType)
	t.supType = supType
	return nil
}

func (t *t11_arfo) HandleMessage(from gen.PID, message any) error {
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

func (t *t11_arfo) TestBasic(input any) {
	defer func() {
		t.testcase = nil
	}()

	// start supervisor
	pid, err := t.Spawn(factory_t11_sup_arfo_basic, gen.ProcessOptions{}, t.supType)
	if err != nil {
		t.testcase.err <- err
	}
	defer t.Node().Kill(pid)

	childtc := &testcase{"TestBasicARFO", nil, nil, make(chan error)}
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

func (t *t11_arfo) TestStrategy(input any) {
	var childtc *testcase

	exit := errors.New("my exit")
	defer func() {
		t.testcase = nil
	}()

	// start supervisor with the given supevision type, restart strategy
	// and ask the supervisor to send the termination reason to the child process
	pid, err := t.Spawn(factory_t11_sup_arfo, gen.ProcessOptions{},
		t.supType, t.testcase.input) // args
	if err != nil {
		t.testcase.err <- err
	}

	if t.supType == act.SupervisorTypeAllForOne {
		childtc = &testcase{"TestAFO", nil, nil, make(chan error)}
	} else {
		childtc = &testcase{"TestRFO", nil, nil, make(chan error)}
	}

	t.Send(pid, childtc)
	if err := childtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	// test stopping/restarting child processes
	// depending on a termination reason and restart strategy
	reasons := []error{gen.TerminateReasonNormal, gen.TerminateReasonShutdown, gen.TerminateReasonKill}
	for _, reason := range reasons {
		t.Send(pid, reason)
		if err := childtc.wait(1); err != nil {
			t.testcase.err <- err
			return
		}
	}

	// send exit signal to the supervisor process
	t.SendExit(pid, exit)
	if err := childtc.wait(1); err != exit {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	// another test... sending kill as a termination reason
	// to the 1st, 2nd and 3rd child process

	if t.supType == act.SupervisorTypeAllForOne {
		childtc = &testcase{"TestAFO", nil, nil, make(chan error)}
	} else {
		childtc = &testcase{"TestRFO", nil, nil, make(chan error)}
	}

	pid1, err := t.Spawn(factory_t11_sup_arfo, gen.ProcessOptions{},
		t.supType, t.testcase.input) // args
	if err != nil {
		t.testcase.err <- err
	}

	t.Send(pid1, childtc)
	if err := childtc.wait(1); err != nil {
		t.Node().Kill(pid1)
		t.testcase.err <- err
		return
	}
	// test restarting children (will be gen.TerminateReasonKill for every child process)
	// depending on a termination reason and restart strategy
	// n - child in the spec
	for n := 0; n < 3; n++ {
		t.Send(pid1, n)
		if err := childtc.wait(1); err != nil {
			t.testcase.err <- err
			return
		}
	}

	t.SendExit(pid1, exit)
	if err := childtc.wait(1); err != exit {
		t.Node().Kill(pid1)
		t.testcase.err <- err
		return
	}
	t.testcase.err <- nil
}

func (t *t11_arfo) TestSignificant(input any) {
	defer func() {
		t.testcase = nil
	}()

	strategy := t.testcase.input.(act.SupervisorStrategy)
	// start supervisor with the given restart strategy
	pid, err := t.Spawn(factory_t11_sup_arfo_significant, gen.ProcessOptions{},
		t.supType, t.testcase.input) // args
	if err != nil {
		t.testcase.err <- err
	}
	childtc := &testcase{"TestARFOSignificant", nil, nil, make(chan error)}
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

// t11_sup_arfo_basic
func factory_t11_sup_arfo_basic() gen.ProcessBehavior {
	return &t11_sup_arfo_basic{}
}

type t11_sup_arfo_basic struct {
	act.Supervisor

	testcase *testcase
}

func (t *t11_sup_arfo_basic) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec
	spec.Type = args[0].(act.SupervisorType)
	spec.Children = []act.SupervisorChildSpec{
		{
			Name:    "child_arfo",
			Factory: factory_t11_child_arfo,
		},
		{
			Name:    "child_arfo1",
			Factory: factory_t11_child_arfo,
		},
		{
			Name:    "child_arfo2",
			Factory: factory_t11_child_arfo,
		},
	}
	return spec, nil
}

func (t *t11_sup_arfo_basic) HandleMessage(from gen.PID, message any) error {
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

func (t *t11_sup_arfo_basic) TestBasicARFO(input any) {
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
		if err := t.StartChild("child_arfo"); err != act.ErrSupervisorChildRunning {
			t.testcase.err <- err
			return
		}
		// check children. must be the same
		if reflect.DeepEqual(children, t.Children()) == false {
			t.testcase.err <- errIncorrect
			return
		}

		// start child with unknown spec
		if err := t.StartChild("child_arfo111"); err != act.ErrSupervisorChildUnknown {
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
			Name:    "child_arfo3",
			Factory: factory_t11_child_arfo,
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

		if err := t.StartChild("child_arfo3"); err != act.ErrSupervisorChildRunning {
			t.testcase.err <- err
			return
		}
		// check children. must be the same
		if reflect.DeepEqual(children, t.Children()) == false {
			t.testcase.err <- errIncorrect
			return
		}

		// disable unknown child spec
		if err := t.DisableChild("child_arfo111"); err != act.ErrSupervisorChildUnknown {
			t.testcase.err <- errIncorrect
			return
		}

		// disable known child spec
		if err := t.DisableChild("child_arfo"); err != nil {
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
	if err := t.StartChild("child_arfo"); err != act.ErrSupervisorChildDisabled {
		t.testcase.err <- err
		return
	}
	t.testcase.err <- nil
}

// t11_sup_arfo
func factory_t11_sup_arfo() gen.ProcessBehavior {
	return &t11_sup_arfo{}
}

type t11_child_stop struct {
	pid    gen.PID
	reason error
}

type t11_sup_arfo struct {
	act.Supervisor

	supType  act.SupervisorType
	strategy act.SupervisorStrategy

	testcase  *testcase
	waitStop  []t11_child_stop
	waitStart []gen.Atom
}

func (t *t11_sup_arfo) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec

	t.supType = args[0].(act.SupervisorType)
	spec.Type = t.supType
	t.strategy = args[1].(act.SupervisorStrategy)
	spec.Restart.Strategy = t.strategy
	spec.Restart.KeepOrder = true

	spec.Children = []act.SupervisorChildSpec{
		{
			Name:    "child_arfo10",
			Factory: factory_t11_child_arfo,
		},
		{
			Name:    "child_arfo11",
			Factory: factory_t11_child_arfo,
		},
		{
			Name:    "child_arfo12",
			Factory: factory_t11_child_arfo,
		},
	}
	spec.EnableHandleChild = true
	spec.DisableAutoShutdown = true

	t.waitStart = []gen.Atom{"child_arfo10", "child_arfo11", "child_arfo12"}
	return spec, nil
}

func (t *t11_sup_arfo) HandleChildStart(name gen.Atom, pid gen.PID) error {
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

func (t *t11_sup_arfo) HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error {
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

func (t *t11_sup_arfo) Terminate(reason error) {
	//var empty gen.PID
	select {
	case t.testcase.err <- reason:
	default:
	}
}

func (t *t11_sup_arfo) HandleMessage(from gen.PID, message any) error {
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

func (t *t11_sup_arfo) TestAFO(input any) {
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

	switch v := input.(type) {
	case error:
		// terminate child process with a given reason
		empty := gen.PID{}
		reason := v
		children := t.Children()
		if len(children) == 0 {
			t.testcase.err <- errIncorrect
			return
		}

		n := 0
		switch reason {
		case gen.TerminateReasonShutdown:
			n = 1
		case gen.TerminateReasonKill:
			n = 2
		}
		if err := t.SendExit(children[n].PID, reason); err != nil {
			t.testcase.err <- err
			return
		}
		stop := t11_child_stop{
			pid:    children[n].PID,
			reason: reason,
		}
		t.waitStop = append(t.waitStop, stop)

		for i, c := range children {
			k := len(children) - 1 - i
			switch t.strategy {
			case act.SupervisorStrategyTransient:
				if reason == gen.TerminateReasonKill {
					t.waitStart = append(t.waitStart, c.Spec)
					if children[k].PID == children[n].PID {
						// already added to wait list
						continue
					}

					if children[k].PID == empty {
						// already terminated
						continue
					}
					stop = t11_child_stop{
						pid:    children[k].PID,
						reason: reason,
					}
					t.waitStop = append(t.waitStop, stop)
					continue
				}
				return
			case act.SupervisorStrategyTemporary:
				return
			case act.SupervisorStrategyPermanent:
				t.waitStart = append(t.waitStart, c.Spec)
				if children[k].PID == children[n].PID {
					// already added to wait list
					continue
				}
				stop = t11_child_stop{
					pid:    children[k].PID,
					reason: reason,
				}
				t.waitStop = append(t.waitStop, stop)
			}
		}
		return
	case int:
		children := t.Children()
		if len(children) == 0 {
			t.testcase.err <- errIncorrect
			return
		}
		if err := t.SendExit(children[v].PID, gen.TerminateReasonKill); err != nil {
			t.testcase.err <- err
			return
		}
		stop := t11_child_stop{
			pid:    children[v].PID,
			reason: gen.TerminateReasonKill,
		}
		t.waitStop = append(t.waitStop, stop)
		if t.strategy == act.SupervisorStrategyTemporary {
			return
		}
		for i := range children {
			t.waitStart = append(t.waitStart, children[i].Spec)

			k := len(children) - 1 - i
			if children[k].PID == children[v].PID {
				// already appended
				continue
			}
			stop = t11_child_stop{
				pid:    children[k].PID,
				reason: gen.TerminateReasonKill,
			}
			t.waitStop = append(t.waitStop, stop)
		}
		return
	}
	panic("shouldn't be here")
}

func (t *t11_sup_arfo) TestRFO(input any) {
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

	switch v := input.(type) {
	case error:
		// terminate child process with a given reason
		empty := gen.PID{}
		reason := v
		children := t.Children()
		if len(children) == 0 {
			t.testcase.err <- errIncorrect
			return
		}

		n := 0
		switch reason {
		case gen.TerminateReasonShutdown:
			n = 1
		case gen.TerminateReasonKill:
			n = 2
		}
		if err := t.SendExit(children[n].PID, reason); err != nil {
			t.testcase.err <- err
			return
		}
		stop := t11_child_stop{
			pid:    children[n].PID,
			reason: reason,
		}
		t.waitStop = append(t.waitStop, stop)

		ch := children[n:]
		for i, c := range ch {
			k := len(ch) - 1 - i
			switch t.strategy {
			case act.SupervisorStrategyTransient:
				if reason == gen.TerminateReasonKill {
					t.waitStart = append(t.waitStart, c.Spec)
					if ch[k].PID == children[n].PID {
						// already added to wait list
						continue
					}

					if ch[k].PID == empty {
						// already terminated
						continue
					}
					stop = t11_child_stop{
						pid:    ch[k].PID,
						reason: reason,
					}
					t.waitStop = append(t.waitStop, stop)
					continue
				}
				return
			case act.SupervisorStrategyTemporary:
				return
			case act.SupervisorStrategyPermanent:
				t.waitStart = append(t.waitStart, c.Spec)
				if ch[k].PID == children[n].PID {
					// already added to wait list
					continue
				}
				stop = t11_child_stop{
					pid:    ch[k].PID,
					reason: reason,
				}
				t.waitStop = append(t.waitStop, stop)
			}
		}
		return
	case int:
		children := t.Children()
		if len(children) == 0 {
			t.testcase.err <- errIncorrect
			return
		}
		if err := t.SendExit(children[v].PID, gen.TerminateReasonKill); err != nil {
			t.testcase.err <- err
			return
		}
		stop := t11_child_stop{
			pid:    children[v].PID,
			reason: gen.TerminateReasonKill,
		}
		t.waitStop = append(t.waitStop, stop)
		if t.strategy == act.SupervisorStrategyTemporary {
			return
		}
		ch := children[v:]
		for i := range ch {
			k := len(ch) - 1 - i
			t.waitStart = append(t.waitStart, ch[i].Spec)

			if ch[k].PID == children[v].PID {
				// already appended
				continue
			}
			stop = t11_child_stop{
				pid:    ch[k].PID,
				reason: gen.TerminateReasonKill,
			}
			t.waitStop = append(t.waitStop, stop)
		}
		return
	}
	panic("shouldn't be here")
}

// t11_sup_arfo_significant
func factory_t11_sup_arfo_significant() gen.ProcessBehavior {
	return &t11_sup_arfo_significant{}
}

type t11_sup_arfo_significant struct {
	act.Supervisor

	supType  act.SupervisorType
	strategy act.SupervisorStrategy

	testcase  *testcase
	waitStop  []t11_child_stop
	waitStart []gen.Atom
}

func (t *t11_sup_arfo_significant) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec
	t.supType = args[0].(act.SupervisorType)
	spec.Type = t.supType
	t.strategy = args[1].(act.SupervisorStrategy)
	spec.Restart.Strategy = t.strategy
	spec.Restart.KeepOrder = true

	spec.Children = []act.SupervisorChildSpec{
		{
			Name:    "child_arfo100",
			Factory: factory_t11_child_arfo,
		},
		{
			Name:        "child_arfo110",
			Significant: true,
			Factory:     factory_t11_child_arfo,
		},
		{
			Name:    "child_arfo120",
			Factory: factory_t11_child_arfo,
		},
	}
	spec.EnableHandleChild = true
	t.waitStart = []gen.Atom{"child_arfo100", "child_arfo110", "child_arfo120"}
	return spec, nil
}

func (t *t11_sup_arfo_significant) HandleMessage(from gen.PID, message any) error {
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

func (t *t11_sup_arfo_significant) HandleChildStart(name gen.Atom, pid gen.PID) error {
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

func (t *t11_sup_arfo_significant) HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error {
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

func (t *t11_sup_arfo_significant) TestARFOSignificant(input any) {
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

	stop := t11_child_stop{
		pid:    c.PID,
		reason: reason,
	}
	t.waitStop = append(t.waitStop, stop)

	switch t.strategy {
	case act.SupervisorStrategyTransient:
		if reason != gen.TerminateReasonNormal && reason != gen.TerminateReasonShutdown {
			if t.supType == act.SupervisorTypeAllForOne {
				stop = t11_child_stop{
					pid:    children[2].PID,
					reason: reason,
				}
				t.waitStop = append(t.waitStop, stop)
				stop = t11_child_stop{
					pid:    children[0].PID,
					reason: reason,
				}
				t.waitStop = append(t.waitStop, stop)

				t.waitStart = append(t.waitStart, children[0].Spec)
				t.waitStart = append(t.waitStart, children[1].Spec)
				t.waitStart = append(t.waitStart, children[2].Spec)

			} else {
				stop = t11_child_stop{
					pid:    children[2].PID,
					reason: reason,
				}
				t.waitStop = append(t.waitStop, stop)

				t.waitStart = append(t.waitStart, children[1].Spec)
				t.waitStart = append(t.waitStart, children[2].Spec)

			}
		}

	case act.SupervisorStrategyTemporary:
		stop = t11_child_stop{
			pid:    children[2].PID,
			reason: reason,
		}
		t.waitStop = append(t.waitStop, stop)
		stop = t11_child_stop{
			pid:    children[0].PID,
			reason: reason,
		}
		t.waitStop = append(t.waitStop, stop)
		break

	case act.SupervisorStrategyPermanent:
		if t.supType == act.SupervisorTypeAllForOne {
			stop = t11_child_stop{
				pid:    children[2].PID,
				reason: reason,
			}
			t.waitStop = append(t.waitStop, stop)
			stop = t11_child_stop{
				pid:    children[0].PID,
				reason: reason,
			}
			t.waitStop = append(t.waitStop, stop)

			t.waitStart = append(t.waitStart, children[0].Spec)
			t.waitStart = append(t.waitStart, children[1].Spec)
			t.waitStart = append(t.waitStart, children[2].Spec)
		} else {
			stop = t11_child_stop{
				pid:    children[2].PID,
				reason: reason,
			}
			t.waitStop = append(t.waitStop, stop)

			t.waitStart = append(t.waitStart, children[1].Spec)
			t.waitStart = append(t.waitStart, children[2].Spec)

		}
	}
}

func (t *t11_sup_arfo_significant) Terminate(reason error) {
	select {
	case t.testcase.err <- reason:
	default:
	}
}

//
// t11_child_arfo
//

func factory_t11_child_arfo() gen.ProcessBehavior {
	return &t11_child_arfo{}
}

type t11_child_arfo struct {
	act.Actor
}

// tests
func TestT11SupervisorAFO(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	//nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t11nodeAFO@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t11_arfo, popt, act.SupervisorTypeAllForOne)
	if err != nil {
		panic(err)
	}

	t11casesARFO = []*testcase{
		&testcase{"TestBasic", nil, nil, make(chan error)},
		&testcase{"TestStrategy", act.SupervisorStrategyTransient, nil, make(chan error)},
		&testcase{"TestStrategy", act.SupervisorStrategyTemporary, nil, make(chan error)},
		&testcase{"TestStrategy", act.SupervisorStrategyPermanent, nil, make(chan error)},
		&testcase{"TestSignificant", act.SupervisorStrategyTransient, nil, make(chan error)},
		&testcase{"TestSignificant", act.SupervisorStrategyTemporary, nil, make(chan error)},
		&testcase{"TestSignificant", act.SupervisorStrategyPermanent, nil, make(chan error)},

		//TODO &testcase{"TestAutoShutdown", nil, nil, make(chan error)},
		//TODO &testcase{"TestRestartIntensity", nil, nil, make(chan error)},
	}
	for _, tc := range t11casesARFO {
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

func TestT11SupervisorRFO(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	//nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t11nodeRFO@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t11_arfo, popt, act.SupervisorTypeRestForOne)
	if err != nil {
		panic(err)
	}

	t11casesARFO = []*testcase{
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
	for _, tc := range t11casesARFO {
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
