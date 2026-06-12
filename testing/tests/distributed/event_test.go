package distributed

import (
	"fmt"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// eventProducer registers an event (buffered) on creation and emits a buffered
// value immediately; it emits a live value on command. Notify is configurable.
type eventProducer struct {
	act.Actor
	notify bool
	name   gen.Atom
	token  gen.Ref
}

func factoryEventProducer() gen.ProcessBehavior { return &eventProducer{} }

func (p *eventProducer) Init(args ...any) error {
	if len(args) > 0 {
		p.notify = args[0].(bool)
	}
	return nil
}

func (p *eventProducer) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "create":
		p.name = gen.Atom(fmt.Sprintf("ev-%d", p.PID().ID))
		token, err := p.RegisterEvent(p.name, gen.EventOptions{Buffer: 1, Notify: p.notify})
		if err != nil {
			return err, nil
		}
		p.token = token
		p.SendEvent(p.name, token, "buffered")
		return gen.Event{Name: p.name, Node: p.Node().Name()}, nil
	case "send":
		return errText(p.SendEvent(p.name, p.token, "live")), nil
	}
	return "ok", nil
}

// eventConsumer subscribes to a (remote) event and exposes the buffered events
// returned by the subscription. Live events arrive in HandleEvent and are observed
// via the node recorder.
type eventConsumer struct {
	act.Actor
	ev gen.Event
}

func factoryEventConsumer() gen.ProcessBehavior { return &eventConsumer{} }

type monitorEv struct{ Event gen.Event }
type linkEv struct{ Event gen.Event }
type unmonitorEv struct{}
type unlinkEv struct{}

func (c *eventConsumer) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch r := request.(type) {
	case monitorEv:
		c.ev = r.Event
		buf, err := c.MonitorEvent(r.Event)
		if err != nil {
			return err, nil
		}
		return buf, nil
	case linkEv:
		c.ev = r.Event
		buf, err := c.LinkEvent(r.Event)
		if err != nil {
			return err, nil
		}
		return buf, nil
	case unmonitorEv:
		return errText(c.DemonitorEvent(c.ev)), nil
	case unlinkEv:
		return errText(c.UnlinkEvent(c.ev)), nil
	}
	return "ok", nil
}

func (c *eventConsumer) HandleEvent(message gen.MessageEvent) error { return nil }

// firstMessage returns the Message of the first buffered event, or nil.
func firstMessage(v any) any {
	evs, ok := v.([]gen.MessageEvent)
	if ok == false || len(evs) == 0 {
		return nil
	}
	return evs[0].Message
}

// TestDistEvent: a consumer on one node subscribes to an event produced on
// another. The subscription returns the producer's buffered event across the wire;
// subsequent live events are delivered to the consumer's HandleEvent. With Notify
// the producer learns of the remote subscribe (MessageEventStart) and unsubscribe
// (MessageEventStop). Covers both MonitorEvent and LinkEvent.
func TestDistEvent(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	s.Connect(n1, n2)

	subscribe := func(t *testing.T, kind string) {
		prod := n2.Spawn(factoryEventProducer, gen.ProcessOptions{}, false)
		evAny, err := n2.Call(prod, "create")
		check.NoError(t, err)
		ev := evAny.(gen.Event)

		cons := n1.Spawn(factoryEventConsumer, gen.ProcessOptions{})
		var req any
		if kind == "monitor" {
			req = monitorEv{Event: ev}
		} else {
			req = linkEv{Event: ev}
		}
		buf, err := n1.Call(cons, req)
		check.NoError(t, err)
		// the producer's buffered event crossed the wire with the subscription
		check.Equal(t, "buffered", firstMessage(buf))

		// the remote subscription is visible in ProcessInfo
		info, err := n1.Native().ProcessInfo(cons)
		check.NoError(t, err)
		check.True(t, len(info.MonitorsEvent) == 1 || len(info.LinksEvent) == 1)

		// a live event is delivered cross-node to HandleEvent
		mk := n1.Mark()
		_, err = n2.Call(prod, "send")
		check.NoError(t, err)
		n1.ShouldReceiveEvent().To(cons).Message("live").Since(mk).Once().Within(time.Second).Must()

		// cross-node unsubscribe also works
		if kind == "monitor" {
			_, err = n1.Call(cons, unmonitorEv{})
		} else {
			_, err = n1.Call(cons, unlinkEv{})
		}
		check.NoError(t, err)
	}

	t.Run("Monitor", func(t *testing.T) { subscribe(t, "monitor") })
	t.Run("Link", func(t *testing.T) { subscribe(t, "link") })

	// with Notify the producer is told when a remote consumer subscribes / leaves
	t.Run("Notify", func(t *testing.T) {
		prod := n2.Spawn(factoryEventProducer, gen.ProcessOptions{}, true)
		evAny, err := n2.Call(prod, "create")
		check.NoError(t, err)
		ev := evAny.(gen.Event)

		cons := n1.Spawn(factoryEventConsumer, gen.ProcessOptions{})

		mk := n2.Mark()
		_, err = n1.Call(cons, monitorEv{Event: ev})
		check.NoError(t, err)
		n2.ShouldDeliver().To(prod).Message(gen.MessageEventStart{Name: ev.Name}).
			Since(mk).Once().Within(time.Second).Must()

		mk = n2.Mark()
		res, err := n1.Call(cons, unmonitorEv{})
		check.NoError(t, err)
		check.Equal(t, "", res)
		n2.ShouldDeliver().To(prod).Message(gen.MessageEventStop{Name: ev.Name}).
			Since(mk).Once().Within(time.Second).Must()
	})
}
