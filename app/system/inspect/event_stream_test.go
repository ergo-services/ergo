package inspect

import (
	"errors"
	"fmt"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

func eventStreamTarget() gen.Event {
	return gen.Event{Name: "prices", Node: "inspect@localhost"}
}

func spawnEventStream(t *testing.T) *unit.Subject {
	t.Helper()

	target := eventStreamTarget()
	node := unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
	node.OnEventInfo(func(gen.Event) (gen.EventInfo, error) {
		return gen.EventInfo{Event: target, Subscribers: 1}, nil
	})

	sub, err := node.Spawn(factory_event_stream, gen.ProcessOptions{},
		eventStreamArgs{Name: target.Name, Limit: 10, Hash: "h5"})
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}
	return sub
}

func eventStreamEvent() gen.Atom {
	return gen.Atom(fmt.Sprintf("%s_%s", inspectEventStream, "h5"))
}

func TestEventStreamRegistersItsEventOnInit(t *testing.T) {
	spawnEventStream(t).ShouldRegisterEvent().Name(eventStreamEvent()).Once().Assert()
}

func TestEventStreamArmsItsIdleShutdownOnInit(t *testing.T) {
	spawnEventStream(t).ShouldSendAfter().Message(shutdown{}).Once().Assert()
}

func TestEventStreamStaysQuietUntilSomebodySubscribes(t *testing.T) {
	spawnEventStream(t).ShouldSendEvent().None().Assert()
}

func TestEventStreamWatchesTheTargetOnlyWhileWatched(t *testing.T) {
	sub := spawnEventStream(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: eventStreamEvent()})
	sub.Drain()

	sub.SendMessage(gen.PID{}, gen.MessageEventStop{Name: eventStreamEvent()})

	if sub.Terminated() {
		t.Fatal("the inspector died when the last subscriber left instead of idling")
	}
	sub.ShouldSendAfter().Message(shutdown{}).AtLeast(1).Assert()
}

func TestEventStreamStopsWhenTheTargetGoesAway(t *testing.T) {
	sub := spawnEventStream(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: eventStreamEvent()})
	sub.Drain()
	sub.SendMessage(gen.PID{}, gen.MessageDownEvent{Event: eventStreamTarget(), Reason: gen.ErrUnregistered})

	if sub.Terminated() == false {
		t.Fatal("the inspector kept streaming an event that no longer exists")
	}
	sub.ShouldSendEvent().Name(eventStreamEvent()).AtLeast(1).Assert()
}

func TestEventStreamIgnoresADownOfAnotherEvent(t *testing.T) {
	sub := spawnEventStream(t)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: eventStreamEvent()})
	sub.Drain()
	sub.SendMessage(gen.PID{}, gen.MessageDownEvent{
		Event:  gen.Event{Name: "other", Node: "inspect@localhost"},
		Reason: gen.ErrUnregistered,
	})

	if sub.Terminated() {
		t.Fatal("the inspector stopped on the death of an event it does not watch")
	}
}

func TestEventStreamStopsWhenTheTargetCannotBeRead(t *testing.T) {
	node := unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
	node.OnEventInfo(func(gen.Event) (gen.EventInfo, error) {
		return gen.EventInfo{}, errors.New("unknown event")
	})
	sub, err := node.Spawn(factory_event_stream, gen.ProcessOptions{},
		eventStreamArgs{Name: "prices", Limit: 10, Hash: "h5"})
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: eventStreamEvent()})
	sub.Drain()

	if sub.Terminated() == false {
		t.Fatal("the inspector kept polling an event the node cannot report on")
	}
}

func TestEventStreamAnswersAnInspectRequest(t *testing.T) {
	sub := spawnEventStream(t)

	client := gen.PID{Node: "inspect@localhost", ID: 100}
	sub.SendMessage(gen.PID{}, requestInspect{pid: client, ref: gen.Ref{}})

	sub.ShouldSendResponse().To(client).Once().Assert()
}

func TestEventStreamStopsAfterAnsweringAboutAnUnreadableTarget(t *testing.T) {
	node := unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
	node.OnEventInfo(func(gen.Event) (gen.EventInfo, error) {
		return gen.EventInfo{}, errors.New("unknown event")
	})
	sub, err := node.Spawn(factory_event_stream, gen.ProcessOptions{},
		eventStreamArgs{Name: "prices", Limit: 10, Hash: "h5"})
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}

	client := gen.PID{Node: "inspect@localhost", ID: 100}
	sub.SendMessage(gen.PID{}, requestInspect{pid: client, ref: gen.Ref{}})

	sub.ShouldSendResponse().To(client).Once().Assert()
	if sub.Terminated() == false {
		t.Fatal("the inspector kept running after reporting the target unreadable")
	}
}

func TestEventStreamShutsDownWhenNobodyEverSubscribed(t *testing.T) {
	sub := spawnEventStream(t)

	sub.SendMessage(gen.PID{}, shutdown{})

	if sub.Terminated() == false {
		t.Fatal("an inspector nobody subscribed to stayed alive, holding its event forever")
	}
}
