package distributed

// call by pid, process name, alias
// unknown process

// TODO test call by process name with registered name in atom cache
// using edf.RegisterAtom.

import (
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

var (
	t4pongAlias gen.Alias
)

func factory_t4pong() gen.ProcessBehavior {
	return &t4pong{}
}

type t4pong struct {
	act.Actor
}

func (t *t4pong) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	t4pongAlias, _ = t.CreateAlias()
	return request, nil // just send back this request as a response value
}

func factory_t4() gen.ProcessBehavior {
	return &t4{}
}

type t4 struct {
	act.Actor

	remote   gen.Atom
	testcase *testcase
}

func (t *t4) Init(args ...any) error {
	t.remote = args[0].(gen.Atom)
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

func (t *t4) TestCallRemotePID(input any) {
	var pingvalue any
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	pingvalue = 123
	pong, err := t.Call(pid, pingvalue)
	if err != nil {
		t.testcase.err <- err
		return
	}
	if reflect.DeepEqual(pingvalue, pong) == false {
		t.testcase.err <- fmt.Errorf("pong value mismatch")
		return
	}

	t.testcase.err <- nil
}

func (t *t4) TestCallRemoteProcessID(input any) {
	var pingvalue any
	defer func() {
		t.testcase = nil
	}()

	regName := gen.Atom("regpong")
	_, err := t.RemoteSpawnRegister(t.remote, "pong", regName, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	pingvalue = 123.456
	pingProcessID := gen.ProcessID{Name: regName, Node: t.remote}
	pong, err := t.Call(pingProcessID, pingvalue)
	if err != nil {
		t.testcase.err <- err
		return
	}
	if reflect.DeepEqual(pingvalue, pong) == false {
		t.testcase.err <- fmt.Errorf("pong value mismatch")
		return
	}

	t.testcase.err <- nil
}

func (t *t4) TestCallRemoteAlias(input any) {
	var pingvalue any
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	pingvalue = "test value"
	if _, err := t.Call(pid, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	emptyAlias := gen.Alias{}
	if t4pongAlias == emptyAlias {
		t.testcase.err <- fmt.Errorf("alias hasn't been created")
		return
	}
	pong, err := t.Call(t4pongAlias, pingvalue)

	if err != nil {
		t.testcase.err <- err
		return
	}

	if reflect.DeepEqual(pingvalue, pong) == false {
		t.testcase.err <- fmt.Errorf("pong value mismatch")
		return
	}

	t.testcase.err <- nil
}

func TestT4CallRemote(t *testing.T) {
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "123"
	options1.Network.MaxMessageSize = 567
	options1.Log.DefaultLogger.Disable = true
	options1.Log.Level = gen.LogLevelTrace
	node1, err := ergo.StartNode("distT0node1CallRemote@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "123"
	options2.Network.MaxMessageSize = 765
	options2.Log.DefaultLogger.Disable = true
	options2.Log.Level = gen.LogLevelTrace
	node2, err := ergo.StartNode("distT0node2CallRemote@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if err := node2.Network().EnableSpawn("pong", factory_t4pong); err != nil {
		t.Fatal(err)
	}

	// make connection to the node2
	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	pid, err := node1.Spawn(factory_t4, gen.ProcessOptions{}, node2.Name())
	if err != nil {
		panic(err)
	}

	t4cases := []*testcase{
		{"TestCallRemotePID", nil, nil, make(chan error)},
		{"TestCallRemoteProcessID", nil, nil, make(chan error)},
		{"TestCallRemoteAlias", nil, nil, make(chan error)},
	}
	for _, tc := range t4cases {
		t.Run(tc.name, func(t *testing.T) {
			node1.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}
}
