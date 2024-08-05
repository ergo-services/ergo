package distributed

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// send by pid
//      by process name
//      by alias

// send to unknown pid/process id/alias

// send compressed
// send exit (with custom reason. must be registered in edf)

// TODO test send by process name with registered name in atom cache
// using edf.RegisterAtom.

var (
	t3pongCh    chan any
	t3pongAlias gen.Alias
)

func factory_t3pong() gen.ProcessBehavior {
	return &t3pong{}
}

type t3pong struct {
	act.Actor
}

func (t *t3pong) HandleMessage(from gen.PID, message any) error {
	select {
	case t3pongCh <- message:
	default:
	}
	t3pongAlias, _ = t.CreateAlias()
	return nil
}

func (t *t3pong) Terminate(reason error) {
	err := errors.Unwrap(reason)
	select {
	case t3pongCh <- err:
	default:
	}
}

func factory_t3() gen.ProcessBehavior {
	return &t3{}
}

type t3 struct {
	act.Actor

	remote   gen.Atom
	testcase *testcase
}

func (t *t3) Init(args ...any) error {
	t.remote = args[0].(gen.Atom)
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

func (t *t3) TestSendRemotePID(input any) {
	var pingvalue any
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	t3pongCh = make(chan any, 1)
	pingvalue = 123
	if err := t.Send(pid, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t3pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch")
			return
		}
	case <-time.NewTimer(time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	t.testcase.err <- nil
}

func (t *t3) TestSendImportantRemotePID(input any) {
	var pingvalue any
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	t3pongCh = make(chan any, 1)
	pingvalue = 123
	if err := t.SendImportant(pid, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t3pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch")
			return
		}
	case <-time.NewTimer(time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	// send to unknown pid
	pid.ID = 100000 // unknown pid
	if err := t.SendImportant(pid, pingvalue); err != gen.ErrProcessUnknown {
		t.testcase.err <- gen.ErrIncorrect
		return
	}

	t.testcase.err <- nil
}

func (t *t3) TestSendRemoteProcessID(input any) {
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

	t3pongCh = make(chan any, 1)
	pingvalue = 123.456
	pingProcessID := gen.ProcessID{Name: regName, Node: t.remote}
	if err := t.Send(pingProcessID, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t3pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch")
			return
		}
	case <-time.NewTimer(time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	t.testcase.err <- nil
}

func (t *t3) TestSendImportantRemoteProcessID(input any) {
	var pingvalue any
	defer func() {
		t.testcase = nil
	}()

	regName := gen.Atom("regpongimportant")
	_, err := t.RemoteSpawnRegister(t.remote, "pong", regName, gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	t3pongCh = make(chan any, 1)
	pingvalue = 123.456
	pingProcessID := gen.ProcessID{Name: regName, Node: t.remote}
	if err := t.SendImportant(pingProcessID, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t3pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch")
			return
		}
	case <-time.NewTimer(time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	pingProcessID.Name = "unknown_name"
	if err := t.SendImportant(pingProcessID, pingvalue); err != gen.ErrProcessUnknown {
		t.testcase.err <- gen.ErrIncorrect
		return
	}

	t.testcase.err <- nil
}

func (t *t3) TestSendRemoteAlias(input any) {
	var pingvalue any
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	t3pongCh = make(chan any)
	pingvalue = "test value"
	if err := t.Send(pid, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t3pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch")
			return
		}
	case <-time.NewTimer(time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	emptyAlias := gen.Alias{}
	if t3pongAlias == emptyAlias {
		t.testcase.err <- fmt.Errorf("alias hasn't been created")
		return
	}
	if err := t.Send(t3pongAlias, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t3pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch")
			return
		}
	case <-time.NewTimer(time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	t.testcase.err <- nil
}

func (t *t3) TestSendImportantRemoteAlias(input any) {
	var pingvalue any
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	t3pongCh = make(chan any, 1)
	pingvalue = "test value"
	if err := t.SendImportant(pid, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t3pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch")
			return
		}
	case <-time.NewTimer(time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	emptyAlias := gen.Alias{}
	if t3pongAlias == emptyAlias {
		t.testcase.err <- fmt.Errorf("alias hasn't been created")
		return
	}
	if err := t.SendImportant(t3pongAlias, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t3pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch")
			return
		}
	case <-time.NewTimer(time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	t3pongAlias.ID[1] = 0 // unknown alias
	if err := t.SendImportant(t3pongAlias, pingvalue); err != gen.ErrProcessUnknown {
		t.testcase.err <- err
		return
	}

	t.testcase.err <- nil
}

func (t *t3) TestSendRemoteTooLarge(input any) {
	var pingvalue any
	defer func() {
		t.testcase = nil
	}()

	remote, err := t.Node().Network().Node(t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	info := remote.Info()
	if info.MaxMessageSize == 0 {
		t.testcase.err <- fmt.Errorf("MaxMessageSize is not set on the remote node. Unable to test")
		return
	}
	// exceed the limit by +1
	pingvalue = lib.RandomString(info.MaxMessageSize + 1)
	pingProcessID := gen.ProcessID{Name: "whatever", Node: t.remote}
	if err := t.Send(pingProcessID, pingvalue); err != gen.ErrTooLarge {
		t.testcase.err <- err
		return
	}

	t.testcase.err <- nil
}

func (t *t3) TestSendRemoteCompress(input any) {
	var pingvalue any
	defer func() {
		t.testcase = nil
	}()

	remote, err := t.Node().Network().Node(t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	info := remote.Info()
	if info.MaxMessageSize == 0 {
		t.testcase.err <- fmt.Errorf("MaxMessageSize is not set on the remote node. Unable to test")
		return
	}

	pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	if info.MaxMessageSize*2 < gen.DefaultCompressionThreshold {
		t.testcase.err <- fmt.Errorf("MaxMessageSize too small. Unable to test compression")
		return
	}
	// string value has good compression ratio
	s := lib.RandomString(info.MaxMessageSize + info.MaxMessageSize/2)
	if len(s) < gen.DefaultCompressionThreshold {
		t.testcase.err <- fmt.Errorf("MaxMessageSize too small. Unable to test compression")
		return
	}
	pingvalue = s

	if err := t.Send(pid, pingvalue); err != gen.ErrTooLarge {
		t.testcase.err <- fmt.Errorf("expected gen.ErrTooLarge, but got: %v", err)
		return
	}

	t.SetCompression(true)

	t3pongCh = make(chan any)
	if err := t.Send(pid, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t3pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch")
			return
		}
	case <-time.NewTimer(time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	t.testcase.err <- nil
}

func (t *t3) TestSendRemoteExit(input any) {
	defer func() {
		t.testcase = nil
	}()

	pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	t3pongCh = make(chan any)
	// use gen.ErrTaken as a sample reason. This error is already
	// registered in edf for sending over the network
	if err := t.SendExit(pid, gen.ErrTaken); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t3pongCh:
		// pong must be gen.ErrTaken
		if pong != gen.ErrTaken {
			t.testcase.err <- fmt.Errorf("pong value mismatch, expected gen.ErrTaken, got: %v", pong)
			return
		}
	case <-time.NewTimer(time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	t.testcase.err <- nil
}

func TestT3SendRemote(t *testing.T) {
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "123"
	options1.Network.MaxMessageSize = 567
	options1.Log.DefaultLogger.Disable = true
	options1.Log.Level = gen.LogLevelTrace
	node1, err := ergo.StartNode("distT0node1SendRemote@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "123"
	options2.Network.MaxMessageSize = 765
	options2.Log.DefaultLogger.Disable = true
	options2.Log.Level = gen.LogLevelTrace
	node2, err := ergo.StartNode("distT0node2SendRemote@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if err := node2.Network().EnableSpawn("pong", factory_t3pong); err != nil {
		t.Fatal(err)
	}

	// make connection to the node2
	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	pid, err := node1.Spawn(factory_t3, gen.ProcessOptions{}, node2.Name())
	if err != nil {
		panic(err)
	}

	t3cases := []*testcase{
		{"TestSendRemotePID", nil, nil, make(chan error)},
		{"TestSendImportantRemotePID", nil, nil, make(chan error)},
		{"TestSendRemoteProcessID", nil, nil, make(chan error)},
		{"TestSendImportantRemoteProcessID", nil, nil, make(chan error)},
		{"TestSendRemoteAlias", nil, nil, make(chan error)},
		{"TestSendImportantRemoteAlias", nil, nil, make(chan error)},
		{"TestSendRemoteTooLarge", nil, nil, make(chan error)},
		{"TestSendRemoteCompress", nil, nil, make(chan error)},
		{"TestSendRemoteExit", nil, nil, make(chan error)},
	}
	for _, tc := range t3cases {
		t.Run(tc.name, func(t *testing.T) {
			node1.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}
}
