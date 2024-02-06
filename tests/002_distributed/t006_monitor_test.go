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

func factory_t6pong() gen.ProcessBehavior {
	return &t6pong{}
}

type t6pong struct {
	act.Actor
}

func (t *t6pong) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
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

func factory_t6() gen.ProcessBehavior {
	return &t6{}
}

type t6 struct {
	act.Actor

	remote   gen.Atom
	testcase *testcase
}

func (t *t6) Init(args ...any) error {
	t.remote = args[0].(gen.Atom)
	return nil
}
func (t *t6) HandleMessage(from gen.PID, message any) error {
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

func (t *t6) TestMonitorRemotePID(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t6, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"MonitoringPID", nil, nil, make(chan error)}
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

	exit := newtc.output.(gen.MessageDownPID)
	t.Node().Kill(pid)
	if exit.Reason != reason {
		t.testcase.err <- fmt.Errorf("incorrect down reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t6) TestMonitorRemotePIDNodeDown(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t6, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"MonitoringPIDNodeDown", nil, nil, make(chan error)}
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

	exit := newtc.output.(gen.MessageDownPID)
	t.Node().Kill(pid)
	if exit.Reason != gen.ErrNoConnection {
		t.testcase.err <- fmt.Errorf("incorrect down reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t6) MonitoringPID(input any) {
	switch m := input.(type) {
	case initcase:
		pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		if err := t.MonitorPID(pid); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.MonitorsPID[0] != pid {
			t.testcase.err <- fmt.Errorf("monitor pid is incorrect: %s (must be: %s)", info.MonitorsPID[0], pid)
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

	case gen.MessageDownPID:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t6) MonitoringPIDNodeDown(input any) {
	switch m := input.(type) {
	case initcase:
		pid, err := t.RemoteSpawn(t.remote, "pong", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		if err := t.MonitorPID(pid); err != nil {
			fmt.Println("AAA1 err", err)
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.MonitorsPID[0] != pid {
			t.testcase.err <- fmt.Errorf("monitor pid is incorrect: %s (must be: %s)", info.MonitorsPID[0], pid)
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

	case gen.MessageDownPID:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t6) TestMonitorRemoteProcessID(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t6, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"MonitoringProcessID", nil, nil, make(chan error)}
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

	exit := newtc.output.(gen.MessageDownProcessID)
	t.Node().Kill(pid)
	if exit.Reason != reason {
		t.testcase.err <- fmt.Errorf("incorrect termination reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t6) TestMonitorRemoteProcessIDNodeDown(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t6, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"MonitoringProcessIDNodeDown", nil, nil, make(chan error)}
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

	exit := newtc.output.(gen.MessageDownProcessID)
	t.Node().Kill(pid)
	if exit.Reason != gen.ErrNoConnection {
		t.testcase.err <- fmt.Errorf("incorrect down reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t6) MonitoringProcessID(input any) {
	switch m := input.(type) {
	case initcase:
		pid, err := t.RemoteSpawnRegister(t.remote, "pong", "regpong", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		processid := gen.ProcessID{Name: "regpong", Node: t.remote}
		if err := t.MonitorProcessID(processid); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.MonitorsProcessID[0] != processid {
			t.testcase.err <- fmt.Errorf("monitor process id is incorrect: %s (must be: %s)", info.MonitorsProcessID[0], processid)
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

	case gen.MessageDownProcessID:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t6) MonitoringProcessIDNodeDown(input any) {
	switch m := input.(type) {
	case initcase:
		pid, err := t.RemoteSpawnRegister(t.remote, "pong", "regpongdown", gen.ProcessOptions{})
		if err != nil {
			t.testcase.err <- err
			return
		}

		processid := gen.ProcessID{Name: "regpongdown", Node: t.remote}
		if err := t.MonitorProcessID(processid); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.MonitorsProcessID[0] != processid {
			t.testcase.err <- fmt.Errorf("monitor process id is incorrect: %s (must be: %s)", info.MonitorsProcessID[0], processid)
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

	case gen.MessageDownProcessID:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t6) TestMonitorRemoteAlias(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t6, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"MonitoringAlias", nil, nil, make(chan error)}
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

	exit := newtc.output.(gen.MessageDownAlias)
	t.Node().Kill(pid)
	if exit.Reason != reason {
		t.testcase.err <- fmt.Errorf("incorrect termination reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t6) TestMonitorRemoteAliasNodeDown(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t6, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"MonitoringAliasNodeDown", nil, nil, make(chan error)}
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

	exit := newtc.output.(gen.MessageDownAlias)
	t.Node().Kill(pid)
	if exit.Reason != gen.ErrNoConnection {
		t.testcase.err <- fmt.Errorf("incorrect down reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t6) MonitoringAlias(input any) {
	switch m := input.(type) {
	case initcase:
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

		if err := t.MonitorAlias(alias); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.MonitorsAlias[0] != alias {
			t.testcase.err <- fmt.Errorf("monitor alias is incorrect: %s (must be: %s)", info.MonitorsAlias[0], alias)
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

	case gen.MessageDownAlias:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t6) MonitoringAliasNodeDown(input any) {
	switch m := input.(type) {
	case initcase:
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

		if err := t.MonitorAlias(alias); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.MonitorsAlias[0] != alias {
			t.testcase.err <- fmt.Errorf("monitor alias is incorrect: %s (must be: %s)", info.MonitorsAlias[0], alias)
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

	case gen.MessageDownAlias:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t6) TestMonitorRemoteEvent(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t6, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"MonitoringEvent", nil, nil, make(chan error)}
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

	exit := newtc.output.(gen.MessageDownEvent)
	t.Node().Kill(pid)
	if exit.Reason != reason {
		t.testcase.err <- fmt.Errorf("incorrect termination reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t6) TestMonitorRemoteEventNodeDown(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t6, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"MonitoringEventNodeDown", nil, nil, make(chan error)}
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

	exit := newtc.output.(gen.MessageDownEvent)
	t.Node().Kill(pid)
	if exit.Reason != gen.ErrNoConnection {
		t.testcase.err <- fmt.Errorf("incorrect down reason: %s", exit.Reason)
		return
	}

	t.testcase.err <- nil
}

func (t *t6) MonitoringEvent(input any) {
	switch m := input.(type) {
	case initcase:
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

		if _, err := t.MonitorEvent(ev); err != nil {
			t.testcase.err <- err
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
		t.testcase.input = pid
		t.testcase.err <- nil

		// waiting exit reason for sending it to the child
		return
	case error:
		pid := t.testcase.input.(gen.PID)
		t.SendExit(pid, m)
		return

	case gen.MessageDownEvent:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t6) MonitoringEventNodeDown(input any) {
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

		if _, err := t.MonitorEvent(ev); err != nil {
			t.testcase.err <- err
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

	case gen.MessageDownEvent:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func (t *t6) TestMonitorRemoteNodeDown(input any) {
	defer func() {
		t.testcase = nil
	}()
	pid, err := t.Spawn(factory_t6, gen.ProcessOptions{}, t.remote)
	if err != nil {
		t.testcase.err <- err
		return
	}
	newtc := &testcase{"MonitoringNodeDown", nil, nil, make(chan error)}
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

	exit := newtc.output.(gen.MessageDownNode)
	t.Node().Kill(pid)
	if exit.Name != t.remote {
		t.testcase.err <- fmt.Errorf("incorrect down node name: %s", exit.Name)
		return
	}

	t.testcase.err <- nil
}

func (t *t6) MonitoringNodeDown(input any) {
	switch m := input.(type) {
	case initcase:

		if err := t.MonitorNode(t.remote); err != nil {
			t.testcase.err <- err
			return
		}

		info, err := t.Info()
		if err != nil {
			t.testcase.err <- err
			return
		}
		if info.MonitorsNode[0] != t.remote {
			t.testcase.err <- fmt.Errorf("monitor node is incorrect: %s (must be: %s)", info.MonitorsNode[0], t.remote)
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

	case gen.MessageDownNode:
		t.testcase.output = m
		t.testcase.err <- nil
		return
	}

	panic(input)
}

func TestT6MonitorRemote(t *testing.T) {
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "123"
	options1.Network.MaxMessageSize = 567
	options1.Log.DefaultLogger.Disable = true
	options1.Log.Level = gen.LogLevelTrace
	node1, err := ergo.StartNode("distT0node1MonitorRemote@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "123"
	options2.Network.MaxMessageSize = 765
	options2.Log.DefaultLogger.Disable = true
	options2.Log.Level = gen.LogLevelTrace
	node2, err := ergo.StartNode("distT0node2MonitorRemote@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	if err := node2.Network().EnableSpawn("pong", factory_t6pong); err != nil {
		t.Fatal(err)
	}

	// make connection to the node2
	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	pid, err := node1.Spawn(factory_t6, gen.ProcessOptions{}, node2.Name())
	if err != nil {
		panic(err)
	}

	t6cases := []*testcase{
		{"TestMonitorRemotePID", nil, nil, make(chan error)},
		{"TestMonitorRemotePIDNodeDown", nil, nil, make(chan error)},
		{"TestMonitorRemoteProcessID", nil, nil, make(chan error)},
		{"TestMonitorRemoteProcessIDNodeDown", nil, nil, make(chan error)},
		{"TestMonitorRemoteAlias", nil, nil, make(chan error)},
		{"TestMonitorRemoteAliasNodeDown", nil, nil, make(chan error)},
		{"TestMonitorRemoteEvent", nil, nil, make(chan error)},
		{"TestMonitorRemoteEventNodeDown", nil, nil, make(chan error)},
		{"TestMonitorRemoteNodeDown", nil, nil, make(chan error)},
	}
	for _, tc := range t6cases {
		t.Run(tc.name, func(t *testing.T) {
			node1.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}
}
