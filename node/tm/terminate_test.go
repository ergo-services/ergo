package tm

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
)

func localPID(id uint64) gen.PID {
	return gen.PID{Node: "node@local", ID: id, Creation: 100}
}

func TestTerminatedTargetPIDDispatchesExitAndDown(t *testing.T) {
	m, core := newManager()
	target := localPID(99)
	linker := localPID(10)
	monitor := localPID(11)
	remoteLink := remotePID(20)

	m.LinkPID(linker, target)
	m.MonitorPID(monitor, target)
	m.LinkPID(remoteLink, target) // remote consumer; GetConnection returns ErrNoConnection so silently skipped

	reason := errors.New("boom")
	m.TerminatedTargetPID(target, reason)

	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.exits) != 1 || len(core.exits[0].To) != 1 || core.exits[0].To[0] != linker {
		t.Fatalf("expected one Exit batch to %v, got %+v", linker, core.exits)
	}
	em, ok := core.exits[0].Message.(gen.MessageExitPID)
	if ok == false || em.PID != target || em.Reason != reason {
		t.Fatalf("exit payload: %+v", core.exits[0].Message)
	}
	if len(core.sent) != 1 || core.sent[0].To != monitor {
		t.Fatalf("expected one Down to %v, got %+v", monitor, core.sent)
	}
	dm, ok := core.sent[0].Message.(gen.MessageDownPID)
	if ok == false || dm.PID != target || dm.Reason != reason {
		t.Fatalf("down payload: %+v", core.sent[0].Message)
	}

	// Target gone from storage.
	if m.storage.Has(target, linker, KindLink) == true {
		t.Fatal("linker relation should be gone after TerminatedTargetPID")
	}
}

func TestTerminatedTargetEventClearsMetadata(t *testing.T) {
	m, core := newManager()
	producer := localPID(42)
	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node@local", Name: "tick"}
	subscriber := localPID(10)
	m.LinkEvent(subscriber, event)

	core.mu.Lock()
	core.exits = nil
	core.sent = nil
	core.mu.Unlock()

	reason := errors.New("event-gone")
	m.TerminatedTargetEvent(event, reason)

	// metadata gone
	if _, err := m.EventInfo(event); err != gen.ErrEventUnknown {
		t.Fatalf("EventInfo after Terminated = %v want ErrEventUnknown", err)
	}
	// subscriber got Exit
	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.exits) != 1 || core.exits[0].To[0] != subscriber {
		t.Fatalf("expected Exit to subscriber, got %+v", core.exits)
	}
}

func TestTerminatedTargetNodeKillsHostedAndDropsConsumers(t *testing.T) {
	m, core := newManager()

	// Targets on the doomed node.
	doomed := gen.Atom("doom@local")
	tgtPID := gen.PID{Node: doomed, ID: 7, Creation: 1}
	tgtEvent := gen.Event{Node: doomed, Name: "evt"}

	// Register a connection to the doomed node so wire-link succeeds and
	// the test's LinkPID/LinkEvent actually install relations.
	core.registerConn(doomed)

	// Local consumers on doomed targets.
	a := localPID(10)
	b := localPID(11)
	m.LinkPID(a, tgtPID)
	m.LinkEvent(b, tgtEvent)

	// A local event with a remote subscriber from doomed node.
	producer := localPID(42)
	m.RegisterEvent(producer, "local", gen.EventOptions{Notify: true})
	localEvent := gen.Event{Node: "node@local", Name: "local"}
	remoteSub := gen.PID{Node: doomed, ID: 99, Creation: 1}
	m.LinkEvent(remoteSub, localEvent)

	// Drop notifications from setup.
	core.mu.Lock()
	core.exits = nil
	core.sent = nil
	core.mu.Unlock()

	m.TerminatedTargetNode(doomed, gen.ErrNoConnection)

	core.mu.Lock()
	defer core.mu.Unlock()

	// Local consumers on doomed targets should have been notified.
	// `a` had Link on tgtPID, `b` had Link on tgtEvent.
	gotPIDExit, gotEventExit := false, false
	for _, e := range core.exits {
		switch msg := e.Message.(type) {
		case gen.MessageExitPID:
			if msg.PID == tgtPID && len(e.To) == 1 && e.To[0] == a {
				gotPIDExit = true
			}
		case gen.MessageExitEvent:
			if msg.Event == tgtEvent && len(e.To) == 1 && e.To[0] == b {
				gotEventExit = true
			}
		}
	}
	if gotPIDExit == false {
		t.Fatalf("expected Exit for tgtPID to %v, got %+v", a, core.exits)
	}
	if gotEventExit == false {
		t.Fatalf("expected Exit for tgtEvent to %v, got %+v", b, core.exits)
	}

	// Local event's remote subscriber should have been dropped and EventStop
	// fired (count went 1→0 with notify=true).
	gotStop := false
	for _, s := range core.sent {
		if _, ok := s.Message.(gen.MessageEventStop); ok && s.To == producer {
			gotStop = true
			break
		}
	}
	if gotStop == false {
		t.Fatalf("expected MessageEventStop to %v after remote subscriber removal, got %+v", producer, core.sent)
	}
	// Local event still exists.
	if _, err := m.EventInfo(localEvent); err != nil {
		t.Fatalf("local event should survive: %v", err)
	}
	// Doomed event metadata gone.
	if _, err := m.EventInfo(tgtEvent); err != gen.ErrEventUnknown {
		t.Fatalf("doomed event should be gone, got %v", err)
	}
}

func TestTerminatedProcessDropsConsumerRelations(t *testing.T) {
	m, _ := newManager()
	consumer := localPID(10)
	target1 := localPID(100)
	target2 := localPID(101)

	m.LinkPID(consumer, target1)
	m.MonitorPID(consumer, target2)

	if m.HasLink(consumer, target1) == false {
		t.Fatal("setup: link should exist")
	}
	if m.HasMonitor(consumer, target2) == false {
		t.Fatal("setup: monitor should exist")
	}

	m.TerminatedProcess(consumer, errors.New("died"))

	if m.HasLink(consumer, target1) == true {
		t.Fatal("link should be gone after TerminatedProcess")
	}
	if m.HasMonitor(consumer, target2) == true {
		t.Fatal("monitor should be gone after TerminatedProcess")
	}
}

func TestTerminatedProcessTearsDownProducedEvents(t *testing.T) {
	m, core := newManager()
	producer := localPID(42)
	subscriber := localPID(10)

	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node@local", Name: "tick"}
	m.LinkEvent(subscriber, event)

	core.mu.Lock()
	core.exits = nil
	core.mu.Unlock()

	reason := errors.New("producer-died")
	m.TerminatedProcess(producer, reason)

	if _, err := m.EventInfo(event); err != gen.ErrEventUnknown {
		t.Fatalf("event metadata should be gone, got %v", err)
	}
	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.exits) != 1 || core.exits[0].To[0] != subscriber {
		t.Fatalf("expected Exit to subscriber on producer death, got %+v", core.exits)
	}
	em, ok := core.exits[0].Message.(gen.MessageExitEvent)
	if ok == false || em.Event != event || em.Reason != reason {
		t.Fatalf("exit payload: %+v", core.exits[0].Message)
	}
}

func TestTerminatedTargetPID_LinkSubscribers(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}

	m.LinkPID(c1, target)
	m.LinkPID(c2, target)
	core.resetSentExits()

	m.TerminatedTargetPID(target, gen.TerminateReasonNormal)
	if core.countSentExits() != 2 {
		t.Errorf("Expected 2 exits, got %d", core.countSentExits())
	}
}

func TestTerminatedTargetPID_MonitorSubscribers(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}

	m.MonitorPID(c1, target)
	m.MonitorPID(c2, target)
	core.resetSentDowns()

	m.TerminatedTargetPID(target, gen.TerminateReasonNormal)
	if core.countSentDowns() != 2 {
		t.Errorf("Expected 2 downs, got %d", core.countSentDowns())
	}
}

func TestTerminatedTargetPID_Mixed(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	linker := gen.PID{Node: "node1", ID: 100}
	mon := gen.PID{Node: "node1", ID: 101}

	m.LinkPID(linker, target)
	m.MonitorPID(mon, target)
	core.resetSentExits()
	core.resetSentDowns()

	m.TerminatedTargetPID(target, gen.TerminateReasonNormal)
	if core.countSentExits() != 1 || core.countSentDowns() != 1 {
		t.Errorf("Expected 1 exit + 1 down, got %d/%d", core.countSentExits(), core.countSentDowns())
	}
}

func TestTerminatedTargetPID_RemoteCorePIDSubscriber(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	remoteCorePID := gen.PID{Node: "node2", ID: 1}

	m.LinkPID(remoteCorePID, target)

	m.TerminatedTargetPID(target, gen.TerminateReasonNormal)
	// Remote subscriber gets terminate via wire.
	if core.countSentTermPIDs() != 1 {
		t.Errorf("Expected 1 wire SendTerminatePID, got %d", core.countSentTermPIDs())
	}
}

func TestTerminatedTargetProcessID_Monitors(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.ProcessID{Node: "node1", Name: "test"}
	c1 := gen.PID{Node: "node1", ID: 100}

	m.MonitorProcessID(c1, target)
	core.resetSentDowns()

	m.TerminatedTargetProcessID(target, gen.TerminateReasonNormal)
	if core.countSentDowns() != 1 {
		t.Errorf("Expected 1 down, got %d", core.countSentDowns())
	}
}

func TestTerminatedTargetAlias_Monitors(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	c1 := gen.PID{Node: "node1", ID: 100}

	m.MonitorAlias(c1, target)
	core.resetSentDowns()

	m.TerminatedTargetAlias(target, gen.TerminateReasonNormal)
	if core.countSentDowns() != 1 {
		t.Errorf("Expected 1 down, got %d", core.countSentDowns())
	}
}

func TestTerminatedEvent_LinkSubscribers(t *testing.T) {
	m, core := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node1", Name: "tick"}

	c1 := gen.PID{Node: "node1", ID: 10}
	c2 := gen.PID{Node: "node1", ID: 11}
	m.LinkEvent(c1, event)
	m.LinkEvent(c2, event)
	core.resetSentExits()

	m.TerminatedTargetEvent(event, gen.TerminateReasonNormal)
	if core.countSentExits() != 2 {
		t.Errorf("Expected 2 exits, got %d", core.countSentExits())
	}
}

func TestTerminatedEvent_MonitorSubscribers(t *testing.T) {
	m, core := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node1", Name: "tick"}

	c1 := gen.PID{Node: "node1", ID: 10}
	m.MonitorEvent(c1, event)
	core.resetSentDowns()

	m.TerminatedTargetEvent(event, gen.TerminateReasonNormal)
	if core.countSentDowns() != 1 {
		t.Errorf("Expected 1 down, got %d", core.countSentDowns())
	}
}

func TestTerminatedEvent_Mixed(t *testing.T) {
	m, core := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node1", Name: "tick"}

	linker := gen.PID{Node: "node1", ID: 10}
	mon := gen.PID{Node: "node1", ID: 11}
	m.LinkEvent(linker, event)
	m.MonitorEvent(mon, event)
	core.resetSentExits()
	core.resetSentDowns()

	m.TerminatedTargetEvent(event, gen.TerminateReasonNormal)
	if core.countSentExits() != 1 || core.countSentDowns() != 1 {
		t.Errorf("Expected 1 exit + 1 down, got %d/%d", core.countSentExits(), core.countSentDowns())
	}
}

func TestTerminatedEvent_NoSubscribers(t *testing.T) {
	m, core := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node1", Name: "tick"}

	m.TerminatedTargetEvent(event, gen.TerminateReasonNormal)
	if core.countSentExits() != 0 || core.countSentDowns() != 0 {
		t.Errorf("No subscribers, should be no exits/downs, got %d/%d", core.countSentExits(), core.countSentDowns())
	}
	if _, err := m.EventInfo(event); err != gen.ErrEventUnknown {
		t.Error("Event metadata should be cleared")
	}
}

func TestTerminatedTargetNode_NoSubscribers(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	// No subscriptions; should not panic.
	m.TerminatedTargetNode(gen.Atom("doomed"), gen.ErrNoConnection)
}

func TestTerminatedTargetNode_RemoteConsumer(t *testing.T) {
	m, core := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{Notify: true})
	event := gen.Event{Node: "node1", Name: "tick"}

	// One remote subscriber from doomed node.
	doomed := gen.Atom("doomed")
	remoteSub := gen.PID{Node: doomed, ID: 99}
	m.LinkEvent(remoteSub, event)
	core.resetSentEventStops()

	m.TerminatedTargetNode(doomed, gen.ErrNoConnection)
	// Last local consumer (none) reaches 0 from 1, EventStop should fire to producer.
	if core.countSentEventStops() != 1 {
		t.Errorf("Expected 1 EventStop after remote subscriber removal, got %d", core.countSentEventStops())
	}
	// Event still exists.
	if _, err := m.EventInfo(event); err != nil {
		t.Errorf("Local event should survive node-down: %v", err)
	}
}

func TestTerminatedTargetNode_EventsCleaned(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	doomed := gen.Atom("doomed")
	doomedEvent := gen.Event{Node: doomed, Name: "ext"}
	consumer := gen.PID{Node: "node1", ID: 10}
	m.LinkEvent(consumer, doomedEvent)

	m.TerminatedTargetNode(doomed, gen.ErrNoConnection)
	if m.hasLinkRelation(consumer, doomedEvent) {
		t.Error("relation should be cleared for events hosted on doomed node")
	}
}

func TestTerminatedProcess_LastSubscriber_EventStop(t *testing.T) {
	m, core := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{Notify: true})
	event := gen.Event{Node: "node1", Name: "tick"}

	consumer := gen.PID{Node: "node1", ID: 10}
	m.LinkEvent(consumer, event)
	core.resetSentEventStops()

	m.TerminatedProcess(consumer, gen.TerminateReasonNormal)
	if core.countSentEventStops() != 1 {
		t.Errorf("Expected EventStop after last local subscriber terminates, got %d", core.countSentEventStops())
	}
}

func TestTerminatedProcess_NotLast_NoEventStop(t *testing.T) {
	m, core := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{Notify: true})
	event := gen.Event{Node: "node1", Name: "tick"}

	c1 := gen.PID{Node: "node1", ID: 10}
	c2 := gen.PID{Node: "node1", ID: 11}
	m.LinkEvent(c1, event)
	m.LinkEvent(c2, event)
	core.resetSentEventStops()

	m.TerminatedProcess(c1, gen.TerminateReasonNormal)
	if core.countSentEventStops() != 0 {
		t.Errorf("EventStop should NOT fire while c2 still subscribed, got %d", core.countSentEventStops())
	}
}

func TestTerminatedProcess_Event_LinkAndMonitor(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node1", Name: "tick"}

	consumer := gen.PID{Node: "node1", ID: 10}
	m.LinkEvent(consumer, event)
	m.MonitorEvent(consumer, event)
	m.TerminatedProcess(consumer, gen.TerminateReasonNormal)

	if m.hasLinkRelation(consumer, event) || m.hasMonitorRelation(consumer, event) {
		t.Error("Both link and monitor should be cleaned on TerminatedProcess")
	}
}

func TestTerminatedProcess_ProducerCleanup_NoEvents(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 10}
	// Process with no events; nothing to clean up. Should not panic.
	m.TerminatedProcess(consumer, gen.TerminateReasonNormal)
}

func TestTerminatedProcess_ProducerCleanup_MultipleEvents(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	m.RegisterEvent(producer, "a", gen.EventOptions{})
	m.RegisterEvent(producer, "b", gen.EventOptions{})

	m.TerminatedProcess(producer, gen.TerminateReasonNormal)
	if m.totalEvents() != 0 {
		t.Errorf("All events of terminated producer should be gone, totalEvents=%d", m.totalEvents())
	}
}

func TestTerminatedTargetProcessIDAndAlias(t *testing.T) {
	m, core := newManager()
	pid1 := gen.ProcessID{Node: "node@local", Name: "worker"}
	alias := gen.Alias{Node: "node@local", ID: [3]uint64{1, 2, 3}, Creation: 1}

	a := localPID(10)
	b := localPID(11)
	m.LinkProcessID(a, pid1)
	m.MonitorAlias(b, alias)

	core.mu.Lock()
	core.exits = nil
	core.sent = nil
	core.mu.Unlock()

	reason := errors.New("gone")
	m.TerminatedTargetProcessID(pid1, reason)
	m.TerminatedTargetAlias(alias, reason)

	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.exits) != 1 || core.exits[0].To[0] != a {
		t.Fatalf("expected Exit to %v, got %+v", a, core.exits)
	}
	if _, ok := core.exits[0].Message.(gen.MessageExitProcessID); ok == false {
		t.Fatalf("expected MessageExitProcessID, got %T", core.exits[0].Message)
	}
	if len(core.sent) != 1 || core.sent[0].To != b {
		t.Fatalf("expected Down to %v, got %+v", b, core.sent)
	}
	if _, ok := core.sent[0].Message.(gen.MessageDownAlias); ok == false {
		t.Fatalf("expected MessageDownAlias, got %T", core.sent[0].Message)
	}
}
