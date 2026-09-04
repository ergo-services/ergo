package inspect

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

func nodeInspectorNode(t *testing.T) *unit.MockNode {
	t.Helper()

	node := unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
	node.OnInfo(func() (gen.NodeInfo, error) {
		return gen.NodeInfo{Name: "inspect@localhost"}, nil
	})
	return node
}

func spawnNodeInspector(t *testing.T) *unit.Subject {
	t.Helper()

	sub, err := nodeInspectorNode(t).Spawn(factory_node, gen.ProcessOptions{})
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}
	return sub
}

func TestNodeInspectorRegistersItsEventOnInit(t *testing.T) {
	spawnNodeInspector(t).ShouldRegisterEvent().Name(gen.Atom(inspectNode)).Once().Assert()
}

func TestNodeInspectorArmsItsIdleShutdownOnInit(t *testing.T) {
	spawnNodeInspector(t).ShouldSendAfter().Message(shutdown{}).Once().Assert()
}

func TestNodeInspectorStaysQuietUntilSomebodySubscribes(t *testing.T) {
	spawnNodeInspector(t).ShouldSendEvent().None().Assert()
}

func TestNodeInspectorPublishesOnTheFirstSubscriber(t *testing.T) {
	sub := spawnNodeInspector(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: gen.Atom(inspectNode)})
	sub.Drain()

	sub.ShouldSendEvent().Name(gen.Atom(inspectNode)).AtLeast(1).Assert()
}

func TestNodeInspectorKeepsPublishingOnItsPeriod(t *testing.T) {
	sub := spawnNodeInspector(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: gen.Atom(inspectNode)})
	sub.Drain()
	sub.FireTimers()
	sub.Drain()

	sub.ShouldSendEvent().Name(gen.Atom(inspectNode)).AtLeast(2).Assert()
}

func TestNodeInspectorStopsPublishingWhenTheLastSubscriberLeaves(t *testing.T) {
	sub := spawnNodeInspector(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: gen.Atom(inspectNode)})
	sub.Drain()
	sub.SendMessage(gen.PID{}, gen.MessageEventStop{Name: gen.Atom(inspectNode)})

	mark := sub.Mark()
	sub.FireTimers()
	sub.Drain()

	sub.ShouldSendEvent().Since(mark).None().Assert()
}

func TestNodeInspectorShutsDownWhenNobodyEverSubscribed(t *testing.T) {
	sub := spawnNodeInspector(t)

	sub.SendMessage(gen.PID{}, shutdown{})

	if sub.Terminated() == false {
		t.Fatal("an inspector nobody subscribed to stayed alive, holding its event forever")
	}
}

func TestNodeInspectorIgnoresShutdownWhileSomebodyIsWatching(t *testing.T) {
	sub := spawnNodeInspector(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: gen.Atom(inspectNode)})
	sub.Drain()
	sub.SendMessage(gen.PID{}, shutdown{})

	if sub.Terminated() {
		t.Fatal("the inspector shut down under a live subscriber")
	}
}

func TestNodeInspectorAnswersAnInspectRequest(t *testing.T) {
	sub := spawnNodeInspector(t)

	client := gen.PID{Node: "inspect@localhost", ID: 100}
	sub.SendMessage(gen.PID{}, requestInspect{pid: client, ref: gen.Ref{}})

	sub.ShouldSendResponse().To(client).Once().Assert()
}

func TestNodeInspectorReportsAFailedSnapshot(t *testing.T) {
	node := nodeInspectorNode(t)
	node.OnInfo(func() (gen.NodeInfo, error) {
		return gen.NodeInfo{}, errors.New("unavailable")
	})
	sub, err := node.Spawn(factory_node, gen.ProcessOptions{})
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}

	client := gen.PID{Node: "inspect@localhost", ID: 100}
	sub.SendMessage(gen.PID{}, requestInspect{pid: client, ref: gen.Ref{}})

	sub.ShouldSendResponse().To(client).Once().Assert()
}

func TestNodeInspectorIgnoresAStaleGenerate(t *testing.T) {
	sub := spawnNodeInspector(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: gen.Atom(inspectNode)})
	sub.Drain()

	mark := sub.Mark()
	sub.SendMessage(gen.PID{}, generate{id: 0})

	sub.ShouldSendEvent().Since(mark).None().Assert()
}
