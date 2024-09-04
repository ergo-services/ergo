package distributed

import (
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

func factory_t7pong() gen.ProcessBehavior {
	return &t7pong{}
}

type t7pong struct {
	act.Actor

	token gen.Ref
	name  gen.Atom
}

func (t *t7pong) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	req := request.(string)
	switch req {
	case "createEvent":
		name := gen.Atom(lib.RandomString(10))
		token, err := t.RegisterEvent(name, gen.EventOptions{Buffer: 1})
		if err != nil {
			t.Log().Error("unable to register event: %s", err)
			break
		}

		t.token = token
		t.name = name
		ev := gen.Event{Name: name, Node: t.Node().Name()}

		t.SendEvent(name, token, "testevent1")
		return ev, nil

	case "sendEvent":
		t.SendEvent(t.name, t.token, "testevent2")
		return true, nil
	}
	return nil, nil
}

func factory_t7() gen.ProcessBehavior {
	return &t7{}
}

type t7 struct {
	act.Actor

	remote   gen.Atom
	testcase *testcase
}

func (t *t7) Init(args ...any) error {
	t.remote = args[0].(gen.Atom)
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

func (t *t7) TestRemoteEvent(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t7, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"EventProducing", nil, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	if s, ok := newtc.output.(string); ok {
		if s != "testevent2" {
			t.testcase.err <- fmt.Errorf("incorrect event message: %#v", newtc.output)
			return
		}
	} else {
		t.testcase.err <- fmt.Errorf("incorrect value: %#v", newtc.output)
		return
	}
	t.Node().Kill(pid)

	t.testcase.err <- nil
}

func (t *t7) EventProducing(input any) {
	switch input.(type) {
	case initcase:
		pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		v, err := t.Call(pid, "createEvent")
		if err != nil {
			t.testcase.err <- err
			return
		}
		ev := v.(gen.Event)

		evlist, err := t.MonitorEvent(ev)
		if err != nil {
			t.testcase.err <- err
			return
		}

		if len(evlist) != 1 {
			t.testcase.err <- fmt.Errorf("there must be at least 1 event")
			return
		}
		if s, ok := evlist[0].Message.(string); ok {
			if s != "testevent1" {
				t.testcase.err <- fmt.Errorf("incorrect event message: %#v", evlist[0].Message)
				return
			}
		} else {
			t.testcase.err <- fmt.Errorf("incorrect value: %#v", evlist[0])
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.MonitorsEvent[0] != ev {
			t.testcase.err <- fmt.Errorf("monitor event is incorrect: %s (must be: %s)", info.MonitorsEvent[0], ev)
			return
		}

		// ask producer to send event
		if _, err := t.Call(pid, "sendEvent"); err != nil {
			t.testcase.err <- err
			return
		}

		// waiting for event in HandleEvent callback
		return
	}

	panic(input)
}

func (t *t7) HandleEvent(message gen.MessageEvent) error {
	t.testcase.output = message.Message
	t.testcase.err <- nil
	return nil
}

func TestT7EventRemote(t *testing.T) {
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "123"
	options1.Network.MaxMessageSize = 567
	options1.Log.DefaultLogger.Disable = true
	options1.Log.Level = gen.LogLevelTrace
	node1, err := ergo.StartNode("distT0node1EventRemote@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "123"
	options2.Network.MaxMessageSize = 765
	options2.Log.DefaultLogger.Disable = true
	options2.Log.Level = gen.LogLevelTrace
	node2, err := ergo.StartNode("distT0node2EventRemote@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if err := node2.Network().EnableSpawn("pong", factory_t7pong); err != nil {
		t.Fatal(err)
	}

	// make connection to the node2
	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	pid, err := node1.Spawn(factory_t7, gen.ProcessOptions{}, node2.Name())
	if err != nil {
		panic(err)
	}

	t7cases := []*testcase{
		{"TestRemoteEvent", nil, nil, make(chan error)},
	}
	for _, tc := range t7cases {
		t.Run(tc.name, func(t *testing.T) {
			node1.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}
}
