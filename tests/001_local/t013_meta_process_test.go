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
	t13cases []*testcase
)

func createTestMeta() gen.MetaBehavior {
	return &testMeta{
		stop: make(chan struct{}),
	}
}

type testMeta struct {
	gen.MetaProcess
	testcase *testcase
	stop     chan struct{}
}

func (tm *testMeta) Init(meta gen.MetaProcess) error {
	tm.MetaProcess = meta
	return nil
}

func (tm *testMeta) Start() error {
	<-tm.stop
	return nil
}

func (tm *testMeta) HandleMessage(from gen.PID, message any) error {
	tm.Log().Info("got message %s", message)
	switch x := message.(type) {
	case *testcase:
		tm.testcase = x
		x.err <- nil
	case gen.PID:
		if err := tm.Send(x, "forward"); err == nil {
			tm.Log().Info("sent 'forward' to %s", x)
		} else {
			tm.Log().Error("unable to send 'forward' to %s", x)
		}
	}
	return nil
}

func (tm *testMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if _, ok := request.(bool); ok {
		// spawn request
		id, err := tm.Spawn(createTestMeta(), gen.MetaOptions{})
		if err != nil {
			tm.Log().Error("unable to start child meta %s", err)
			return false, nil
		}
		return id, nil
	}
	tm.Log().Info("got request %s from %s", request, from)
	return request, nil
}

func (tm *testMeta) Terminate(reason error) {
	tm.Log().Info("terminate with reason %s", reason)
	close(tm.stop)
	if tm.testcase != nil {
		select {
		case tm.testcase.err <- reason:
		default:
		}
	}
}

func (tm *testMeta) HandleInspect(from gen.PID, item ...string) map[string]string {
	tm.Log().Info("got inspect request from %s", from)
	result := map[string]string{
		from.String(): "ok",
	}
	return result
}

func factory_t13() gen.ProcessBehavior {
	return &t13{}
}

type t13 struct {
	act.Actor

	testcase *testcase
}

func (t *t13) HandleMessage(from gen.PID, message any) error {
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

func (t *t13) TestBasic(input any) {
	defer func() {
		t.testcase = nil
	}()

	meta := createTestMeta()
	id, err := t.SpawnMeta(meta, gen.MetaOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}
	t.Log().Info("started meta %s", id)

	// test call
	req := "ping-pong"
	t.Log().Info("making call to %s", id)
	resp, err := t.Call(id, req)
	if err != nil {
		t.Log().Error("test call failed: %s", err)
		select {
		case t.testcase.err <- err:
		default:
		}
		return
	}

	if reflect.DeepEqual(req, resp) == false {
		t.Log().Error("test call failed. incorrect response")
		t.testcase.err <- errIncorrect
		return
	}

	// test send
	x := &testcase{"x", nil, nil, make(chan error)}
	t.Log().Info("send message to %s", id)
	if err := t.Send(id, x); err != nil {
		t.Log().Error("unable to send message to %s: %s", id, err)
		t.testcase.err <- errIncorrect
		return
	}
	if err := x.wait(1); err != nil {
		t.Log().Error("seems meta process hasnt recv this message")
		t.testcase.err <- err
		return
	}

	exp := map[string]string{
		t.PID().String(): "ok",
	}
	// test inspect
	insp, err := t.InspectMeta(id)
	if err != nil {
		t.Log().Error("inspect error: %s", err)
		t.testcase.err <- err
		return

	}
	if reflect.DeepEqual(insp, exp) == false {
		t.Log().Error("inspect failed. incorrect response")
		t.testcase.err <- errIncorrect
		return
	}

	// test spawn child meta
	v, err := t.Call(id, true)
	if err != nil {
		t.Log().Error("test call failed. incorrect response")
		t.testcase.err <- errIncorrect
		return
	}
	if _, ok := v.(gen.Alias); ok == false {
		t.testcase.err <- errIncorrect
		return
	}

	// test exit-signal
	xterm := errors.New("test meta exit")
	t.SendExitMeta(id, xterm)
	if err := x.wait(1); err != xterm {
		t.Log().Error("seems meta process hasnt recv exit signal")
		t.testcase.err <- err
		return
	}

	t.testcase.err <- nil
	t.Log().Info("TestBasic done")
}

func (t *t13) TestLinkMonitor(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.Spawn(factory_t13, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to start new process: %s", err)
		t.testcase.err <- err
		return
	}
	metatc := &testcase{"LinkMonitorMeta", nil, nil, make(chan error)}
	t.Send(pid, metatc)
	if err := metatc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	// ask to start meta
	t.Send(pid, "start")
	if err := metatc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	id := metatc.output.(gen.Alias)

	// ask meta to send message to the pid
	t.Send(id, pid)
	if err := metatc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	// must be "forward"
	if metatc.output.(string) != "forward" {
		t.Log().Error("incorrect value")
		t.testcase.err <- errIncorrect
		return
	}

	// ask to link with meta
	metatc.input = id
	t.Send(pid, "link")
	if err := metatc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	// send exit to the meta
	idterm := errors.New("term meta")
	t.SendExitMeta(id, idterm)

	// waiting for the gen.MessageExitAlias
	if err := metatc.wait(1); err != nil {
		t.Log().Error("waiting for gen.MessageExitAlias failed: %s", err)
		t.testcase.err <- err
		return
	}
	expexit := gen.MessageExitAlias{
		Alias:  id,
		Reason: idterm,
	}
	if reflect.DeepEqual(metatc.output, expexit) == false {
		t.Log().Error("incorrect exit message. got %#v", metatc.output)
		t.testcase.err <- errIncorrect
		return
	}

	// ask to start meta
	t.Send(pid, "start")
	if err := metatc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	id = metatc.output.(gen.Alias)

	// ask to create monitor with meta
	metatc.input = id
	t.Send(pid, "monitor")
	if err := metatc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	// send exit to the meta
	t.SendExitMeta(id, idterm)

	// waiting for the gen.MessageDownAlias
	if err := metatc.wait(1); err != nil {
		t.Log().Error("waiting for gen.MessageDownAlias failed: %s", err)
		t.testcase.err <- err
		return
	}
	expdown := gen.MessageDownAlias{
		Alias:  id,
		Reason: idterm,
	}
	if reflect.DeepEqual(metatc.output, expdown) == false {
		t.Log().Error("incorrect down message. got %#v", metatc.output)
		t.testcase.err <- errIncorrect
		return
	}

	// case: terminate parent process

	// ask to start meta
	t.Send(pid, "start")
	if err := metatc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}
	id = metatc.output.(gen.Alias)
	// send this case to the meta process
	t.Send(id, metatc)
	if err := metatc.wait(1); err != nil {
		t.testcase.err <- err
		return
	}

	pidterm := errors.New("blabla")
	t.SendExit(pid, pidterm)
	// meta should be terminated with the same reason
	if err := metatc.wait(1); err != pidterm {
		t.Log().Error("incorrect value. expected: %s", pidterm)
		t.testcase.err <- err
		return
	}

	t.testcase.err <- nil

	t.Log().Info("TestLinkMonitor done")
}

func (t *t13) LinkMonitorMeta(input any) {
	switch x := input.(type) {
	case initcase:
		t.SetTrapExit(true)
		t.testcase.err <- nil
		return
	case string:
		switch x {
		case "start":
			id, err := t.SpawnMeta(createTestMeta(), gen.MetaOptions{})
			if err != nil {
				t.Log().Error("unable to start meta: %s", err)
				t.testcase.err <- err
				return
			}
			t.Log().Info("started meta %s", id)
			t.testcase.output = id
			t.testcase.err <- nil
			return
		case "forward":
			t.testcase.output = x
			t.testcase.err <- nil
			return
		case "link":
			if err := t.Link(t.testcase.input); err != nil {
				t.Log().Error("unable to link with %v: %s", t.testcase.input, err)
				t.testcase.err <- err
				return
			}
			t.testcase.err <- nil
			t.Log().Info("created link with %s", t.testcase.input)
			return
		case "monitor":
			if err := t.Monitor(t.testcase.input); err != nil {
				t.Log().Error("unable to monitor %v: %s", t.testcase.input, err)
				t.testcase.err <- err
				return
			}
			t.testcase.err <- nil
			t.Log().Info("created monitor with %s", t.testcase.input)
			return
		}
	case gen.MessageExitAlias, gen.MessageDownAlias:
		t.testcase.output = x
		t.testcase.err <- nil
		return
	}
	t.Log().Error("got unknown message %#v", input)
	panic("shouldnt be here")
}

func TestT13Meta(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	//nopt.Log.Level = gen.LogLevelTrace
	node, err := ergo.StartNode("t13node@localhost", nopt)
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t13, popt)
	if err != nil {
		panic(err)
	}

	t13cases = []*testcase{
		{"TestBasic", nil, nil, make(chan error)},
		{"TestLinkMonitor", nil, nil, make(chan error)},
	}
	for _, tc := range t13cases {
		t.Run(tc.name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(3); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
