package unit_test

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

// warmConsumer subscribes to an event and serves whatever the subscription
// replayed, without waiting for the next publish.
type warmConsumer struct {
	act.Actor
	last    any
	subErr  error
	replays int
}

func factoryWarmConsumer() gen.ProcessBehavior { return &warmConsumer{} }

type subscribe struct {
	event gen.Event
	kind  string
}

func (c *warmConsumer) HandleMessage(from gen.PID, message any) error {
	s, ok := message.(subscribe)
	if ok == false {
		return nil
	}
	var buffer []gen.MessageEvent
	var err error
	if s.kind == "link" {
		buffer, err = c.LinkEvent(s.event)
	} else {
		buffer, err = c.MonitorEvent(s.event)
	}
	if err != nil {
		c.subErr = err
		return nil
	}
	c.replays = len(buffer)
	if len(buffer) > 0 {
		c.last = buffer[len(buffer)-1].Message
	}
	return nil
}

// A stubbed subscription replays a buffer, so the warm-start branch (the consumer
// answers from what the producer had buffered) is reachable in unit.
func TestMonitorEventReplaysBuffer(t *testing.T) {
	event := gen.Event{Name: "metrics", Node: "unit@localhost"}
	buffered := []gen.MessageEvent{
		{Event: event, Message: "old"},
		{Event: event, Message: "recent"},
	}

	for _, kind := range []string{"link", "monitor"} {
		t.Run(kind, func(t *testing.T) {
			a, err := unit.Spawn(t, factoryWarmConsumer, gen.ProcessOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "link" {
				a.OnLinkEvent(event).Return(buffered)
			} else {
				a.OnMonitorEvent(event).Return(buffered)
			}

			a.SendMessage(gen.PID{}, subscribe{event: event, kind: kind})

			b := a.Behavior().(*warmConsumer)
			check.NoError(t, b.subErr)
			check.Equal(t, 2, b.replays)
			check.Equal(t, "recent", b.last)
		})
	}
}

// Without a stub the subscription still succeeds and replays nothing, as an event
// with an empty buffer does.
func TestMonitorEventNoStubReplaysNothing(t *testing.T) {
	event := gen.Event{Name: "metrics", Node: "unit@localhost"}
	a, _ := unit.Spawn(t, factoryWarmConsumer, gen.ProcessOptions{})

	a.SendMessage(gen.PID{}, subscribe{event: event, kind: "monitor"})

	b := a.Behavior().(*warmConsumer)
	check.NoError(t, b.subErr)
	check.Equal(t, 0, b.replays)
	a.ShouldMonitor().Target(event).Once().Assert()
}

// A stubbed failure propagates and is recorded on the subscription.
func TestMonitorEventStubFails(t *testing.T) {
	event := gen.Event{Name: "metrics", Node: "unit@localhost"}
	a, _ := unit.Spawn(t, factoryWarmConsumer, gen.ProcessOptions{})
	a.OnMonitorEvent(event).Fail(gen.ErrTargetExist)

	a.SendMessage(gen.PID{}, subscribe{event: event, kind: "monitor"})

	b := a.Behavior().(*warmConsumer)
	check.ErrorIs(t, b.subErr, gen.ErrTargetExist)
	check.Equal(t, 0, b.replays)
	a.ShouldMonitor().Target(event).ErrorIs(gen.ErrTargetExist).Once().Assert()
}

// A stub with an empty Node matches the event of that name on any node.
func TestMonitorEventStubMatchesAnyNode(t *testing.T) {
	a, _ := unit.Spawn(t, factoryWarmConsumer, gen.ProcessOptions{})
	a.OnMonitorEvent(gen.Event{Name: "metrics"}).Return([]gen.MessageEvent{{Message: 1}})

	a.SendMessage(gen.PID{}, subscribe{event: gen.Event{Name: "metrics", Node: "other@host"}, kind: "monitor"})

	check.Equal(t, 1, a.Behavior().(*warmConsumer).replays)
}

// The harness node epoch is timestamp-shaped, so a PID a test writes by hand does
// not silently collide with one the harness minted.
func TestHarnessEpochIsNotOne(t *testing.T) {
	a, _ := unit.Spawn(t, factoryWarmConsumer, gen.ProcessOptions{})

	handWritten := gen.PID{Node: a.PID().Node, ID: a.PID().ID, Creation: 1}
	if handWritten == a.PID() {
		t.Fatal("a hand-written PID with Creation 1 collides with the subject PID")
	}
	if a.Node().Creation() == 1 {
		t.Fatal("harness node epoch is still 1")
	}
	check.Equal(t, a.Node().Creation(), a.PID().Creation)
}
