package tm

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
)

func TestManagerLinksAndMonitorsFor(t *testing.T) {
	m, _ := newManager()
	consumer := localPID(10)
	t1 := localPID(100)
	t2 := localPID(101)

	m.LinkPID(consumer, t1)
	m.LinkPID(consumer, t2)
	m.MonitorPID(consumer, t1)

	links := m.LinksFor(consumer)
	if len(links) != 2 {
		t.Fatalf("LinksFor = %v want 2 entries", links)
	}
	monitors := m.MonitorsFor(consumer)
	if len(monitors) != 1 || monitors[0] != t1 {
		t.Fatalf("MonitorsFor = %v want [%v]", monitors, t1)
	}
}

func TestManagerInfoCountsLinksMonitorsEvents(t *testing.T) {
	m, _ := newManager()
	m.LinkPID(localPID(10), localPID(100))
	m.MonitorPID(localPID(11), localPID(101))
	m.RegisterEvent(localPID(42), "tick", gen.EventOptions{})

	info := m.Info()
	if info.Links != 1 {
		t.Fatalf("Links = %d want 1", info.Links)
	}
	if info.Monitors != 1 {
		t.Fatalf("Monitors = %d want 1", info.Monitors)
	}
	if info.Events != 1 {
		t.Fatalf("Events = %d want 1", info.Events)
	}
}

func TestManagerInfoExitsAndDowns(t *testing.T) {
	m, _ := newManager()
	target := localPID(100)
	m.LinkPID(localPID(10), target)
	m.LinkPID(localPID(11), target)
	m.MonitorPID(localPID(20), target)

	m.TerminatedTargetPID(target, errors.New("boom"))

	info := m.Info()
	if info.ExitSignalsProduced != 1 {
		t.Fatalf("ExitSignalsProduced = %d want 1", info.ExitSignalsProduced)
	}
	if info.ExitSignalsDelivered != 2 {
		t.Fatalf("ExitSignalsDelivered = %d want 2", info.ExitSignalsDelivered)
	}
	if info.DownMessagesProduced != 1 {
		t.Fatalf("DownMessagesProduced = %d want 1", info.DownMessagesProduced)
	}
	if info.DownMessagesDelivered != 1 {
		t.Fatalf("DownMessagesDelivered = %d want 1", info.DownMessagesDelivered)
	}
}

func TestManagerInfoEventsPublishedAndSent(t *testing.T) {
	m, _ := newManager()
	producer := localPID(42)
	token, _ := m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node@local", Name: "tick"}
	m.LinkEvent(localPID(10), event)
	m.LinkEvent(localPID(11), event)

	for i := 0; i < 3; i++ {
		m.PublishEvent(producer, token, gen.MessageOptions{}, gen.MessageEvent{Event: event})
	}

	info := m.Info()
	if info.EventsPublished != 3 {
		t.Fatalf("EventsPublished = %d want 3", info.EventsPublished)
	}
	if info.EventsLocalSent != 6 {
		t.Fatalf("EventsLocalSent = %d want 6 (3 publishes * 2 subscribers)", info.EventsLocalSent)
	}
}

func TestManagerInfoEventsReceivedOnRemoteProducer(t *testing.T) {
	m, _ := newManager()
	producer := localPID(42)
	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node@local", Name: "tick"}
	m.LinkEvent(localPID(10), event)

	remote := remotePID(99)
	m.PublishEvent(remote, gen.Ref{}, gen.MessageOptions{}, gen.MessageEvent{Event: event})

	info := m.Info()
	if info.EventsReceived != 1 {
		t.Fatalf("EventsReceived = %d want 1", info.EventsReceived)
	}
	if info.EventsLocalSent != 1 {
		t.Fatalf("EventsLocalSent = %d want 1", info.EventsLocalSent)
	}
}

func TestManagerEventsCountDecrementsOnUnregister(t *testing.T) {
	m, _ := newManager()
	producer := localPID(42)
	m.RegisterEvent(producer, "a", gen.EventOptions{})
	m.RegisterEvent(producer, "b", gen.EventOptions{})

	if m.Info().Events != 2 {
		t.Fatalf("Events = %d want 2", m.Info().Events)
	}
	m.UnregisterEvent(producer, "a")
	if m.Info().Events != 1 {
		t.Fatalf("Events after Unregister = %d want 1", m.Info().Events)
	}
}
