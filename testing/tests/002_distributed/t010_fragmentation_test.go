package distributed

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
	t10pongCh chan any
)

func factory_t10pong() gen.ProcessBehavior {
	return &t10pong{}
}

type t10pong struct {
	act.Actor
}

func (t *t10pong) HandleMessage(from gen.PID, message any) error {
	select {
	case t10pongCh <- message:
	default:
	}
	return nil
}

func factory_t10() gen.ProcessBehavior {
	return &t10{}
}

type t10 struct {
	act.Actor

	remote   gen.Atom
	testcase *testcase
}

func (t *t10) Init(args ...any) error {
	t.remote = args[0].(gen.Atom)
	return nil
}

func (t *t10) HandleMessage(from gen.PID, message any) error {
	if t.testcase == nil {
		t.testcase = message.(*testcase)
		message = initcase{}
	}

	method := reflect.ValueOf(t).MethodByName(t.testcase.name)
	if method.IsValid() == false {
		t.testcase.err <- fmt.Errorf("unknown method %q", t.testcase.name)
		t.testcase = nil
		return nil
	}
	method.Call([]reflect.Value{reflect.ValueOf(message)})
	return nil
}

// TestFragmentSendOrdered sends a large message with KeepNetworkOrder (order > 0).
// All fragments go to the same pool item -> same TCP -> same recv queue.
func (t *t10) TestFragmentSendOrdered(input any) {
	defer func() { t.testcase = nil }()

	pid, err := t.RemoteSpawn(t.remote, "t10pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	t10pongCh = make(chan any, 1)

	// 20000 bytes string, FragmentSize=4096 -> ~5 fragments
	pingvalue := lib.RandomString(20000)

	if err := t.Send(pid, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t10pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch (ordered)")
			return
		}
	case <-time.NewTimer(5 * time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	t.testcase.err <- nil
}

// TestFragmentSendUnordered sends a large message without KeepNetworkOrder (order = 0).
// Fragments round-robin across pool items -> different recv queues.
func (t *t10) TestFragmentSendUnordered(input any) {
	defer func() { t.testcase = nil }()

	pid, err := t.RemoteSpawn(t.remote, "t10pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	t10pongCh = make(chan any, 1)

	pingvalue := lib.RandomString(20000)

	t.SetKeepNetworkOrder(false)
	defer t.SetKeepNetworkOrder(true)

	if err := t.Send(pid, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t10pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch (unordered)")
			return
		}
	case <-time.NewTimer(5 * time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	t.testcase.err <- nil
}

// TestFragmentSendCompressed sends a large compressed message that still exceeds FragmentSize.
func (t *t10) TestFragmentSendCompressed(input any) {
	defer func() { t.testcase = nil }()

	pid, err := t.RemoteSpawn(t.remote, "t10pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	t10pongCh = make(chan any, 1)

	// large enough that even compressed it exceeds FragmentSize=4096
	pingvalue := lib.RandomString(40000)

	t.SetCompression(true)
	defer t.SetCompression(false)

	if err := t.Send(pid, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t10pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch (compressed+fragmented)")
			return
		}
	case <-time.NewTimer(5 * time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	t.testcase.err <- nil
}

// TestFragmentSendImportant sends a large important message.
// ACK must arrive after full reassembly.
func (t *t10) TestFragmentSendImportant(input any) {
	defer func() { t.testcase = nil }()

	pid, err := t.RemoteSpawn(t.remote, "t10pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	t10pongCh = make(chan any, 1)

	pingvalue := lib.RandomString(5000)

	if err := t.SendImportant(pid, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t10pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch (important)")
			return
		}
	case <-time.NewTimer(5 * time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	t.testcase.err <- nil
}

// TestFragmentSendSmall sends a message smaller than FragmentSize.
// Should NOT be fragmented, just sent normally.
func (t *t10) TestFragmentSendSmall(input any) {
	defer func() { t.testcase = nil }()

	pid, err := t.RemoteSpawn(t.remote, "t10pong", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	t10pongCh = make(chan any, 1)

	// small message, no fragmentation
	pingvalue := "hello"

	if err := t.Send(pid, pingvalue); err != nil {
		t.testcase.err <- err
		return
	}

	select {
	case pong := <-t10pongCh:
		if reflect.DeepEqual(pingvalue, pong) == false {
			t.testcase.err <- fmt.Errorf("pong value mismatch (small)")
			return
		}
	case <-time.NewTimer(5 * time.Second).C:
		t.testcase.err <- gen.ErrTimeout
		return
	}

	t.testcase.err <- nil
}

func TestT10Fragmentation(t *testing.T) {
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "fragtest"
	options1.Network.FragmentSize = 4096
	options1.Log.DefaultLogger.Disable = false
	options1.Log.Level = gen.LogLevelTrace
	node1, err := ergo.StartNode("distT10node1Frag@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "fragtest"
	options2.Network.FragmentSize = 4096
	options2.Log.DefaultLogger.Disable = false
	options2.Log.Level = gen.LogLevelTrace
	node2, err := ergo.StartNode("distT10node2Frag@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if err := node2.Network().EnableSpawn("t10pong", factory_t10pong); err != nil {
		t.Fatal(err)
	}

	// establish connection
	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	pid, err := node1.Spawn(factory_t10, gen.ProcessOptions{}, node2.Name())
	if err != nil {
		panic(err)
	}

	t10cases := []*testcase{
		{"TestFragmentSendSmall", nil, nil, make(chan error)},
		{"TestFragmentSendOrdered", nil, nil, make(chan error)},
		{"TestFragmentSendUnordered", nil, nil, make(chan error)},
		{"TestFragmentSendCompressed", nil, nil, make(chan error)},
		{"TestFragmentSendImportant", nil, nil, make(chan error)},
	}
	for _, tc := range t10cases {
		t.Run(tc.name, func(t *testing.T) {
			node1.Send(pid, tc)
			if err := tc.wait(10); err != nil {
				t.Fatal(err)
			}
		})
	}
}
