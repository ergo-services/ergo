package inspect

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

func nodeShortNode(t *testing.T) *unit.MockNode {
	t.Helper()

	node := unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
	node.OnShortInfo(func() (gen.NodeShortInfo, error) {
		return gen.NodeShortInfo{Name: "inspect@localhost"}, nil
	})
	return node
}

func spawnNodeShort(t *testing.T) *unit.Subject {
	t.Helper()

	sub, err := nodeShortNode(t).Spawn(factory_node_short, gen.ProcessOptions{},
		gen.Atom(inspectNodeShort), inspectNodeShortPeriod)
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}
	return sub
}

func TestNodeShortRegistersItsEventOnInit(t *testing.T) {
	sub := spawnNodeShort(t)

	sub.ShouldRegisterEvent().Name(gen.Atom(inspectNodeShort)).Once().Assert()
}

func TestNodeShortRefusesToStartWhenTheEventIsTaken(t *testing.T) {
	sub := nodeShortNode(t).Prepare(factory_node_short, gen.ProcessOptions{},
		gen.Atom(inspectNodeShort), inspectNodeShortPeriod)
	sub.OnRegisterEvent(gen.Atom(inspectNodeShort)).Fail(gen.ErrTaken)

	if err := sub.Run(); err == nil {
		t.Fatal("the inspector started without owning its event, so nothing would ever reach a subscriber")
	}
}

func TestNodeShortArmsItsIdleShutdownOnInit(t *testing.T) {
	sub := spawnNodeShort(t)

	sub.ShouldSendAfter().Message(shutdown{}).Once().Assert()
}

func TestNodeShortStaysQuietUntilSomebodySubscribes(t *testing.T) {
	sub := spawnNodeShort(t)

	sub.ShouldSendEvent().None().Assert()
}

func TestNodeShortPublishesOnTheFirstSubscriber(t *testing.T) {
	sub := spawnNodeShort(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: gen.Atom(inspectNodeShort)})
	sub.Drain()

	sub.ShouldSendEvent().Name(gen.Atom(inspectNodeShort)).AtLeast(1).Assert()
}

func TestNodeShortKeepsPublishingOnItsPeriod(t *testing.T) {
	sub := spawnNodeShort(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: gen.Atom(inspectNodeShort)})
	sub.Drain()
	sub.FireTimers()
	sub.Drain()

	sub.ShouldSendEvent().Name(gen.Atom(inspectNodeShort)).AtLeast(2).Assert()
}

func TestNodeShortStopsPublishingWhenTheLastSubscriberLeaves(t *testing.T) {
	sub := spawnNodeShort(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: gen.Atom(inspectNodeShort)})
	sub.Drain()
	sub.SendMessage(gen.PID{}, gen.MessageEventStop{Name: gen.Atom(inspectNodeShort)})

	mark := sub.Mark()
	sub.FireTimers()
	sub.Drain()

	sub.ShouldSendEvent().Since(mark).None().Assert()
}

func TestNodeShortRearmsItsIdleShutdownAfterTheLastSubscriberLeaves(t *testing.T) {
	sub := spawnNodeShort(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: gen.Atom(inspectNodeShort)})
	sub.Drain()

	mark := sub.Mark()
	sub.SendMessage(gen.PID{}, gen.MessageEventStop{Name: gen.Atom(inspectNodeShort)})

	sub.ShouldSendAfter().Message(shutdown{}).Since(mark).Once().Assert()
}

func TestNodeShortShutsDownWhenNobodyEverSubscribed(t *testing.T) {
	sub := spawnNodeShort(t)

	sub.SendMessage(gen.PID{}, shutdown{})

	if sub.Terminated() == false {
		t.Fatal("an inspector nobody subscribed to stayed alive, holding its event forever")
	}
}

func TestNodeShortIgnoresShutdownWhileSomebodyIsWatching(t *testing.T) {
	sub := spawnNodeShort(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: gen.Atom(inspectNodeShort)})
	sub.Drain()
	sub.SendMessage(gen.PID{}, shutdown{})

	if sub.Terminated() {
		t.Fatal("the inspector shut down under a live subscriber")
	}
}

func TestNodeShortAnswersAnInspectRequestWithItsEventAndASnapshot(t *testing.T) {
	sub := spawnNodeShort(t)

	client := gen.PID{Node: "inspect@localhost", ID: 100}
	sub.SendMessage(gen.PID{}, requestInspect{pid: client, ref: gen.Ref{}})

	sub.ShouldSendResponse().To(client).Once().Assert()
}

func TestNodeShortReportsAFailedSnapshotInTheResponse(t *testing.T) {
	node := nodeShortNode(t)
	node.OnShortInfo(func() (gen.NodeShortInfo, error) {
		return gen.NodeShortInfo{}, errors.New("unavailable")
	})
	sub, err := node.Spawn(factory_node_short, gen.ProcessOptions{},
		gen.Atom(inspectNodeShort), inspectNodeShortPeriod)
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}

	client := gen.PID{Node: "inspect@localhost", ID: 100}
	sub.SendMessage(gen.PID{}, requestInspect{pid: client, ref: gen.Ref{}})

	sub.ShouldSendResponse().To(client).Once().Assert()
}

func TestNodeShortIgnoresAStaleGenerateFromAPreviousLoop(t *testing.T) {
	sub := spawnNodeShort(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: gen.Atom(inspectNodeShort)})
	sub.Drain()

	mark := sub.Mark()
	sub.SendMessage(gen.PID{}, generate{id: 0})

	sub.ShouldSendEvent().Since(mark).None().Assert()
}
