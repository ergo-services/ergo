package inspect

import (
	"fmt"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

func TestConnectionInspectorReportsAPeerThatWentAwayAndStops(t *testing.T) {
	remote := gen.Atom("peer@localhost")
	event := gen.Atom(fmt.Sprintf("%s_%s", inspectConnection, remote))

	node := unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
	sub, err := node.Spawn(factory_connection, gen.ProcessOptions{}, remote)
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: event})
	sub.Drain()

	sub.ShouldSendEvent().Name(event).AtLeast(1).Assert()
	if sub.Terminated() == false {
		t.Fatal("the inspector outlived the connection it watches, holding an event nobody can refresh")
	}
}

func TestConnectionInspectorKeepsWatchingALivePeer(t *testing.T) {
	remote := gen.Atom("peer@localhost")
	event := gen.Atom(fmt.Sprintf("%s_%s", inspectConnection, remote))

	node := unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
	node.Network().OnGetNode(remote)
	sub, err := node.Spawn(factory_connection, gen.ProcessOptions{}, remote)
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: event})
	sub.Drain()

	if sub.Terminated() {
		t.Fatal("the inspector stopped although the peer is connected")
	}
	sub.ShouldSendEvent().Name(event).AtLeast(1).Assert()
}

func TestConnectionInspectorStopsAfterAnsweringAboutAGonePeer(t *testing.T) {
	remote := gen.Atom("peer@localhost")

	node := unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
	sub, err := node.Spawn(factory_connection, gen.ProcessOptions{}, remote)
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}

	client := gen.PID{Node: "inspect@localhost", ID: 100}
	sub.SendMessage(gen.PID{}, requestInspect{pid: client, ref: gen.Ref{}})

	sub.ShouldSendResponse().To(client).Once().Assert()
	if sub.Terminated() == false {
		t.Fatal("the inspector kept running after reporting the peer gone")
	}
}
