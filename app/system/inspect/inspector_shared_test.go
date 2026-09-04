package inspect

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

type inspectorCase struct {
	name    string
	factory gen.ProcessFactory
	event   gen.Atom
	args    []any
	node    func(t *testing.T) *unit.MockNode
}

func plainNode(t *testing.T) *unit.MockNode {
	t.Helper()
	return unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
}

func (c inspectorCase) spawn(t *testing.T) *unit.Subject {
	t.Helper()

	build := c.node
	if build == nil {
		build = plainNode
	}
	sub, err := build(t).Spawn(c.factory, gen.ProcessOptions{}, c.args...)
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}
	return sub
}

func runInspectorContract(t *testing.T, c inspectorCase) {
	t.Helper()

	t.Run(c.name+"/registers its event", func(t *testing.T) {
		c.spawn(t).ShouldRegisterEvent().Name(c.event).Once().Assert()
	})

	t.Run(c.name+"/arms its idle shutdown", func(t *testing.T) {
		c.spawn(t).ShouldSendAfter().Message(shutdown{}).Once().Assert()
	})

	t.Run(c.name+"/stays quiet with no subscriber", func(t *testing.T) {
		c.spawn(t).ShouldSendEvent().None().Assert()
	})

	t.Run(c.name+"/publishes on the first subscriber", func(t *testing.T) {
		sub := c.spawn(t)
		sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: c.event})
		sub.Drain()
		sub.ShouldSendEvent().Name(c.event).AtLeast(1).Assert()
	})

	t.Run(c.name+"/keeps publishing on its period", func(t *testing.T) {
		sub := c.spawn(t)
		sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: c.event})
		sub.Drain()
		sub.FireTimers()
		sub.Drain()
		sub.ShouldSendEvent().Name(c.event).AtLeast(2).Assert()
	})

	t.Run(c.name+"/stops when the last subscriber leaves", func(t *testing.T) {
		sub := c.spawn(t)
		sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: c.event})
		sub.Drain()
		sub.SendMessage(gen.PID{}, gen.MessageEventStop{Name: c.event})

		mark := sub.Mark()
		sub.FireTimers()
		sub.Drain()
		sub.ShouldSendEvent().Since(mark).None().Assert()
	})

	t.Run(c.name+"/rearms the idle shutdown when the last subscriber leaves", func(t *testing.T) {
		sub := c.spawn(t)
		sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: c.event})
		sub.Drain()

		mark := sub.Mark()
		sub.SendMessage(gen.PID{}, gen.MessageEventStop{Name: c.event})
		sub.ShouldSendAfter().Message(shutdown{}).Since(mark).Once().Assert()
	})

	t.Run(c.name+"/shuts down when nobody ever subscribed", func(t *testing.T) {
		sub := c.spawn(t)
		sub.SendMessage(gen.PID{}, shutdown{})
		if sub.Terminated() == false {
			t.Fatal("an inspector nobody subscribed to stayed alive, holding its event forever")
		}
	})

	t.Run(c.name+"/ignores shutdown under a live subscriber", func(t *testing.T) {
		sub := c.spawn(t)
		sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: c.event})
		sub.Drain()
		sub.SendMessage(gen.PID{}, shutdown{})
		if sub.Terminated() {
			t.Fatal("the inspector shut down under a live subscriber")
		}
	})

	t.Run(c.name+"/answers an inspect request", func(t *testing.T) {
		sub := c.spawn(t)
		client := gen.PID{Node: "inspect@localhost", ID: 100}
		sub.SendMessage(gen.PID{}, requestInspect{pid: client, ref: gen.Ref{}})
		sub.ShouldSendResponse().To(client).Once().Assert()
	})

	t.Run(c.name+"/ignores a stale generate", func(t *testing.T) {
		sub := c.spawn(t)
		sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: c.event})
		sub.Drain()

		mark := sub.Mark()
		sub.SendMessage(gen.PID{}, generate{id: 0})
		sub.ShouldSendEvent().Since(mark).None().Assert()
	})
}
