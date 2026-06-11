package local

import (
	"fmt"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type evtRegister struct{}
type evtSend struct{ Val any }
type evtLink struct{ Event gen.Event }
type evtMonitor struct{ Event gen.Event }
type evtUnlink struct{ Event gen.Event }
type evtDemonitor struct{ Event gen.Event }

// producer registers a notifying, buffered event and publishes on command.
type producer struct {
	act.Actor
	name  gen.Atom
	token gen.Ref
}

func factoryProducer() gen.ProcessBehavior { return &producer{} }

func (p *producer) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch c := request.(type) {
	case evtRegister:
		p.name = gen.Atom(fmt.Sprintf("ev-%d", p.PID().ID))
		tok, err := p.RegisterEvent(p.name, gen.EventOptions{Notify: true, Buffer: 10})
		p.token = tok
		return gen.Event{Name: p.name, Node: p.Node().Name()}, err
	case evtSend:
		return "ok", p.SendEvent(p.name, p.token, c.Val)
	}
	return nil, nil
}

func (p *producer) HandleMessage(from gen.PID, message any) error { return nil }

// consumer subscribes/unsubscribes on command and receives events.
type consumer struct{ act.Actor }

func factoryConsumer() gen.ProcessBehavior { return &consumer{} }

func (c *consumer) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch m := request.(type) {
	case evtLink:
		return c.LinkEvent(m.Event)
	case evtMonitor:
		return c.MonitorEvent(m.Event)
	case evtUnlink:
		return "ok", c.UnlinkEvent(m.Event)
	case evtDemonitor:
		return "ok", c.DemonitorEvent(m.Event)
	}
	return nil, nil
}

func (c *consumer) HandleEvent(message gen.MessageEvent) error { return nil }

// TestLocalEvent: a producer registers a notifying buffered event; subscribing
// (link/monitor) returns the buffered last events and notifies the producer
// (MessageEventStart); published events fan out to all subscribers carrying a
// timestamp; unsubscribing the last consumer notifies the producer (MessageEventStop).
func TestLocalEvent(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")

	prod := n.Spawn(factoryProducer)
	evAny, err := n.Call(prod, evtRegister{})
	check.NoError(t, err)
	event := evAny.(gen.Event)

	// consumer1 links: no buffered events yet; producer gets a start notification
	c1 := n.Spawn(factoryConsumer)
	leAny, err := n.Call(c1, evtLink{Event: event})
	check.NoError(t, err)
	check.Equal(t, 0, len(leAny.([]gen.MessageEvent)))
	n.ShouldDeliver().To(prod).Message(gen.MessageEventStart{Name: event.Name}).
		Once().Within(time.Second).Must()

	// publish: consumer1 receives it with a non-zero timestamp
	_, err = n.Call(prod, evtSend{Val: int8(123)})
	check.NoError(t, err)
	rec, ok := n.ShouldReceiveEvent().To(c1).Message(int8(123)).Within(time.Second).Capture()
	check.True(t, ok)
	check.Equal(t, event, rec.Event)
	check.True(t, rec.Timestamp != 0)

	// consumer2 monitors: the buffer hands it the last event
	c2 := n.Spawn(factoryConsumer)
	le2Any, err := n.Call(c2, evtMonitor{Event: event})
	check.NoError(t, err)
	le2 := le2Any.([]gen.MessageEvent)
	check.Equal(t, 1, len(le2))
	check.Equal(t, event, le2[0].Event)
	check.Equal(t, int8(123), le2[0].Message)
	check.Equal(t, rec.Timestamp, le2[0].Timestamp) // buffer preserved the delivered event

	// publish again: both subscribers receive it, each carrying a non-zero timestamp
	m := n.Mark()
	_, err = n.Call(prod, evtSend{Val: int16(1234)})
	check.NoError(t, err)
	r1, ok1 := n.ShouldReceiveEvent().To(c1).Message(int16(1234)).Since(m).Within(time.Second).Capture()
	check.True(t, ok1)
	check.Equal(t, event, r1.Event)
	check.True(t, r1.Timestamp != 0)
	r2, ok2 := n.ShouldReceiveEvent().To(c2).Message(int16(1234)).Since(m).Within(time.Second).Capture()
	check.True(t, ok2)
	check.Equal(t, event, r2.Event)
	check.True(t, r2.Timestamp != 0)

	// unsubscribe both: producer gets a stop notification
	_, err = n.Call(c1, evtUnlink{Event: event})
	check.NoError(t, err)
	_, err = n.Call(c2, evtDemonitor{Event: event})
	check.NoError(t, err)
	n.ShouldDeliver().To(prod).Message(gen.MessageEventStop{Name: event.Name}).
		Once().Within(time.Second).Must()
}
