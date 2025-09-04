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

// Linik PID, ProcessID, Alias
// handle Terminate linked process
// handle terminated network connection
// send Event
// Link node

func factory_t5pong() gen.ProcessBehavior {
	return &t5pong{}
}

type t5pong struct {
	act.Actor
}

func (t *t5pong) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	req := request.(string)
	switch req {
	case "alias":
		alias, _ := t.CreateAlias()
		return alias, nil
	case "event":
		name := gen.Atom(lib.RandomString(10))
		if _, err := t.RegisterEvent(name, gen.EventOptions{}); err != nil {
			t.Log().Error("unable to register event: %s", err)
			break
		}
		ev := gen.Event{Name: name, Node: t.Node().Name()}
		return ev, nil
	}
	return nil, nil
}

func factory_t5() gen.ProcessBehavior {
	return &t5{}
}

type t5 struct {
	act.Actor

	remote   gen.Atom
	testcase *testcase
}

func (t *t5) Init(args ...any) error {
	t.remote = args[0].(gen.Atom)
	return nil
}
func (t *t5) HandleMessage(from gen.PID, message any) error {
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

func (t *t5) TestLinkRemotePID(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t5, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"LinkingPID", nil, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}
	reason := gen.TerminateReasonShutdown
	t.Send(pid, reason)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	exit := newtc.output.(gen.MessageExitPID)
	t.Node().Kill(pid)
	if exit.Reason != reason {
		t.testcase.err <- fmt.Errorf("incorrect termination reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t5) TestLinkRemotePIDNodeDown(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t5, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"LinkingPIDNodeDown", nil, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}
	t.Send(pid, fmt.Errorf("doDisconnect"))
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	exit := newtc.output.(gen.MessageExitPID)
	t.Node().Kill(pid)
	if exit.Reason != gen.ErrNoConnection {
		t.testcase.err <- fmt.Errorf("incorrect exit reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t5) TestLinkRemoteProcessID(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t5, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"LinkingProcessID", nil, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}
	reason := gen.TerminateReasonShutdown
	t.Send(pid, reason)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	exit := newtc.output.(gen.MessageExitProcessID)
	t.Node().Kill(pid)
	if exit.Reason != reason {
		t.testcase.err <- fmt.Errorf("incorrect termination reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t5) TestLinkRemoteProcessIDNodeDown(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t5, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"LinkingProcessIDNodeDown", nil, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}
	t.Send(pid, fmt.Errorf("doDisconnect"))
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	exit := newtc.output.(gen.MessageExitProcessID)
	t.Node().Kill(pid)
	if exit.Reason != gen.ErrNoConnection {
		t.testcase.err <- fmt.Errorf("incorrect exit reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t5) TestLinkRemoteAlias(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t5, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"LinkingAlias", nil, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}
	reason := gen.TerminateReasonShutdown
	t.Send(pid, reason)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	exit := newtc.output.(gen.MessageExitAlias)
	t.Node().Kill(pid)
	if exit.Reason != reason {
		t.testcase.err <- fmt.Errorf("incorrect termination reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t5) TestLinkRemoteAliasNodeDown(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t5, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"LinkingAliasNodeDown", nil, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	t.Send(pid, fmt.Errorf("doDisconnect"))
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	exit := newtc.output.(gen.MessageExitAlias)
	t.Node().Kill(pid)
	if exit.Reason != gen.ErrNoConnection {
		t.testcase.err <- fmt.Errorf("incorrect exit reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t5) TestLinkRemoteEvent(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t5, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"LinkingEvent", nil, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}
	reason := gen.TerminateReasonShutdown
	t.Send(pid, reason)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	exit := newtc.output.(gen.MessageExitEvent)
	t.Node().Kill(pid)
	if exit.Reason != reason {
		t.testcase.err <- fmt.Errorf("incorrect termination reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t5) TestLinkRemoteEventNodeDown(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t5, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"LinkingEventNodeDown", nil, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}
	t.Send(pid, fmt.Errorf("doDisconnect"))
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	exit := newtc.output.(gen.MessageExitEvent)
	t.Node().Kill(pid)
	if exit.Reason != gen.ErrNoConnection {
		t.testcase.err <- fmt.Errorf("incorrect exit reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t5) TestLinkRemoteNodeDown(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t5, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"LinkingNodeDown", nil, nil, make(chan error)}
	t.Send(pid, newtc)
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}
	t.Send(pid, fmt.Errorf("doDisconnect"))
	if err := newtc.wait(1); err != nil {
		t.Node().Kill(pid)
		t.testcase.err <- err
		return
	}

	exit := newtc.output.(gen.MessageExitNode)
	t.Node().Kill(pid)
	if exit.Name != t.remote {
		t.testcase.err <- fmt.Errorf("incorrect exit node name: %s", exit.Name)
		return
	}

	t.testcase.err <- nil
}

func (t *t5) LinkingPID(input any) {
	switch m := input.(type) {
	case initcase:
		t.SetTrapExit(true)
		pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		if err := t.LinkPID(pid); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.LinksPID[0] != pid {
			t.testcase.err <- fmt.Errorf("link pid is incorrect: %s (must be: %s)", info.LinksPID[0], pid)
			return
		}
		t.testcase.input = pid
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	case error:
		pid := t.testcase.input.(gen.PID)
		t.SendExit(pid, m)
		return

	case gen.MessageExitPID:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t5) LinkingPIDNodeDown(input any) {
	switch m := input.(type) {
	case initcase:
		t.SetTrapExit(true)
		pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		if err := t.LinkPID(pid); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.LinksPID[0] != pid {
			t.testcase.err <- fmt.Errorf("link pid is incorrect: %s (must be: %s)", info.LinksPID[0], pid)
			return
		}
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	case error:
		remoteNode, err := t.Node().Network().Node(t.remote)
		if err != nil {
			t.testcase.err <- err
			return

		}
		remoteNode.Disconnect()
		return

	case gen.MessageExitPID:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t5) LinkingProcessID(input any) {
	switch m := input.(type) {
	case initcase:
		t.SetTrapExit(true)
		pid, err := t.RemoteSpawnRegister(t.remote, "pong", "regpong", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		processid := gen.ProcessID{Name: "regpong", Node: t.remote}
		if err := t.LinkProcessID(processid); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.LinksProcessID[0] != processid {
			t.testcase.err <- fmt.Errorf("link process id is incorrect: %s (must be: %s)", info.LinksProcessID[0], processid)
			return
		}
		t.testcase.input = pid
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	case error:
		pid := t.testcase.input.(gen.PID)
		t.SendExit(pid, m)
		return

	case gen.MessageExitProcessID:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t5) LinkingProcessIDNodeDown(input any) {
	switch m := input.(type) {
	case initcase:
		t.SetTrapExit(true)
		pid, err := t.RemoteSpawnRegister(t.remote, "pong", "regpongdown", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		processid := gen.ProcessID{Name: "regpongdown", Node: t.remote}
		if err := t.LinkProcessID(processid); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.LinksProcessID[0] != processid {
			t.testcase.err <- fmt.Errorf("link process id is incorrect: %s (must be: %s)", info.LinksProcessID[0], processid)
			return
		}
		t.testcase.input = pid
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return

	case error:
		remoteNode, err := t.Node().Network().Node(t.remote)
		if err != nil {
			t.testcase.err <- err
			return

		}
		remoteNode.Disconnect()
		return

	case gen.MessageExitProcessID:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t5) LinkingAlias(input any) {
	switch m := input.(type) {
	case initcase:
		t.SetTrapExit(true)
		pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		v, err := t.Call(pid, "alias")
		if err != nil {
			t.testcase.err <- err
			return
		}
		alias := v.(gen.Alias)

		if err := t.LinkAlias(alias); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.LinksAlias[0] != alias {
			t.testcase.err <- fmt.Errorf("link alias is incorrect: %s (must be: %s)", info.LinksAlias[0], alias)
			return
		}
		t.testcase.input = pid
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	case error:
		pid := t.testcase.input.(gen.PID)
		t.SendExit(pid, m)
		return

	case gen.MessageExitAlias:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t5) LinkingAliasNodeDown(input any) {
	switch m := input.(type) {
	case initcase:
		t.SetTrapExit(true)
		pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		v, err := t.Call(pid, "alias")
		if err != nil {
			t.testcase.err <- err
			return
		}
		alias := v.(gen.Alias)

		if err := t.LinkAlias(alias); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.LinksAlias[0] != alias {
			t.testcase.err <- fmt.Errorf("link alias is incorrect: %s (must be: %s)", info.LinksAlias[0], alias)
			return
		}
		t.testcase.input = pid
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	case error:
		remoteNode, err := t.Node().Network().Node(t.remote)
		if err != nil {
			t.testcase.err <- err
			return

		}
		remoteNode.Disconnect()
		return

	case gen.MessageExitAlias:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t5) LinkingEvent(input any) {
	switch m := input.(type) {
	case initcase:
		t.SetTrapExit(true)
		pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		v, err := t.Call(pid, "event")
		if err != nil {
			t.testcase.err <- err
			return
		}
		ev := v.(gen.Event)

		if _, err := t.LinkEvent(ev); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.LinksEvent[0] != ev {
			t.testcase.err <- fmt.Errorf("link event is incorrect: %s (must be: %s)", info.LinksEvent[0], ev)
			return
		}
		t.testcase.input = pid
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	case error:
		pid := t.testcase.input.(gen.PID)
		t.SendExit(pid, m)
		return

	case gen.MessageExitEvent:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t5) LinkingEventNodeDown(input any) {
	switch m := input.(type) {
	case initcase:
		t.SetTrapExit(true)
		pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		v, err := t.Call(pid, "event")
		if err != nil {
			t.testcase.err <- err
			return
		}
		ev := v.(gen.Event)

		if _, err := t.LinkEvent(ev); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.LinksEvent[0] != ev {
			t.testcase.err <- fmt.Errorf("link event is incorrect: %s (must be: %s)", info.LinksEvent[0], ev)
			return
		}
		t.testcase.input = pid
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	case error:
		remoteNode, err := t.Node().Network().Node(t.remote)
		if err != nil {
			t.testcase.err <- err
			return

		}
		remoteNode.Disconnect()
		return

	case gen.MessageExitEvent:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t5) LinkingNodeDown(input any) {
	switch m := input.(type) {
	case initcase:
		t.SetTrapExit(true)

		if err := t.LinkNode(t.remote); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.LinksNode[0] != t.remote {
			t.testcase.err <- fmt.Errorf("link node is incorrect: %s (must be: %s)", info.LinksNode[0], t.remote)
			return
		}
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	case error:
		remoteNode, err := t.Node().Network().Node(t.remote)
		if err != nil {
			t.testcase.err <- err
			return

		}
		remoteNode.Disconnect()
		return

	case gen.MessageExitNode:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func TestT5LinkRemote(t *testing.T) {
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "123"
	options1.Network.MaxMessageSize = 567
	options1.Log.DefaultLogger.Disable = true
	options1.Log.Level = gen.LogLevelTrace
	node1, err := ergo.StartNode("distT0node1LinkRemote@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "123"
	options2.Network.MaxMessageSize = 765
	options2.Log.DefaultLogger.Disable = true
	options2.Log.Level = gen.LogLevelTrace
	node2, err := ergo.StartNode("distT0node2LinkRemote@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if err := node2.Network().EnableSpawn("pong", factory_t5pong); err != nil {
		t.Fatal(err)
	}

	// make connection to the node2
	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	pid, err := node1.Spawn(factory_t5, gen.ProcessOptions{}, node2.Name())
	if err != nil {
		panic(err)
	}

	t5cases := []*testcase{
		{"TestLinkRemotePID", nil, nil, make(chan error)},
		{"TestLinkRemotePIDNodeDown", nil, nil, make(chan error)},
		{"TestLinkRemoteProcessID", nil, nil, make(chan error)},
		{"TestLinkRemoteProcessIDNodeDown", nil, nil, make(chan error)},
		{"TestLinkRemoteAlias", nil, nil, make(chan error)},
		{"TestLinkRemoteAliasNodeDown", nil, nil, make(chan error)},
		{"TestLinkRemoteEvent", nil, nil, make(chan error)},
		{"TestLinkRemoteEventNodeDown", nil, nil, make(chan error)},
		{"TestLinkRemoteNodeDown", nil, nil, make(chan error)},
	}
	for _, tc := range t5cases {
		t.Run(tc.name, func(t *testing.T) {
			node1.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}
}
