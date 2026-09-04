package unit_test

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

// eventRegistrar registers two events with different options.
type eventRegistrar struct{ act.Actor }

func factoryEventRegistrar() gen.ProcessBehavior { return &eventRegistrar{} }

func (e *eventRegistrar) Init(args ...any) error {
	if _, err := e.RegisterEvent("notifying", gen.EventOptions{Notify: true, Buffer: 10, Open: true}); err != nil {
		return err
	}
	_, err := e.RegisterEvent("quiet", gen.EventOptions{})
	return err
}

// The RegisterEvent record carries the options the actor registered with, so a
// test can tell a notifying buffered event from a plain one.
func TestRegisterEventOptions(t *testing.T) {
	a, err := unit.Spawn(t, factoryEventRegistrar, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	a.ShouldRegisterEvent().Name("notifying").Notify(true).Buffer(10).Open(true).Once().Assert()
	a.ShouldRegisterEvent().Name("quiet").Notify(false).Buffer(0).Open(false).Once().Assert()

	// the filters discriminate: neither event matches the other's options
	a.ShouldRegisterEvent().Name("quiet").Notify(true).None().Assert()
	a.ShouldRegisterEvent().Name("notifying").Buffer(0).None().Assert()
	a.ShouldRegisterEvent().Notify(true).Once().Assert()

	rec, ok := a.ShouldRegisterEvent().Name("notifying").Capture()
	check.Equal(t, true, ok)
	check.Equal(t, gen.EventOptions{Notify: true, Buffer: 10, Open: true}, rec.Options)
}

// lazyProducer is the notify-gated producer pattern: it registers a notifying
// buffered event and publishes only while it has subscribers.
type lazyProducer struct {
	act.Actor
	token  gen.Ref
	active bool
}

func factoryLazyProducer() gen.ProcessBehavior { return &lazyProducer{} }

func (p *lazyProducer) Init(args ...any) error {
	token, err := p.RegisterEvent("metrics", gen.EventOptions{Notify: true, Buffer: 5})
	p.token = token
	return err
}

func (p *lazyProducer) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case gen.MessageEventStart:
		p.active = true
	case gen.MessageEventStop:
		p.active = false
	default:
		if p.active {
			return p.SendEvent("metrics", p.token, message)
		}
	}
	return nil
}

// The whole notify-gated producer pattern is unit-testable as it stands: the
// registration options are on the record, and the producer notifications are
// ordinary mailbox messages the drivers deliver.
func TestNotifyGatedProducer(t *testing.T) {
	a, err := unit.Spawn(t, factoryLazyProducer, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// it asked for notifications and a warm-up buffer
	a.ShouldRegisterEvent().Name("metrics").Notify(true).Buffer(5).Once().Assert()

	// no subscribers yet: silent
	a.SendMessage(gen.PID{}, "sample-1")
	a.ShouldSendEvent().None().Assert()

	// first subscriber arrives
	a.SendMessage(gen.PID{}, gen.MessageEventStart{Name: "metrics"})
	a.SendMessage(gen.PID{}, "sample-2")
	a.ShouldSendEvent().Name("metrics").Message("sample-2").
		Token(a.Behavior().(*lazyProducer).token).Once().Assert()

	// last subscriber leaves: silent again
	a.SendMessage(gen.PID{}, gen.MessageEventStop{Name: "metrics"})
	a.SendMessage(gen.PID{}, "sample-3")
	a.ShouldSendEvent().Message("sample-3").None().Assert()
	a.ShouldSendEvent().Once().Assert()
}
