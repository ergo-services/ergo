package tm

import (
	"testing"

	"ergo.services/ergo/gen"
)

// HasLink

func TestHasLink_Basic(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	if m.HasLink(consumer, target) {
		t.Error("HasLink should return false before link")
	}
	m.LinkPID(consumer, target)
	if m.HasLink(consumer, target) == false {
		t.Error("HasLink should return true after link")
	}
	m.UnlinkPID(consumer, target)
	if m.HasLink(consumer, target) {
		t.Error("HasLink should return false after unlink")
	}
}

func TestHasLink_DifferentTargetTypes(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	tPID := gen.PID{Node: "node1", ID: 200}
	tProc := gen.ProcessID{Name: "proc", Node: "node1"}
	tAlias := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	tNode := gen.Atom("node2")

	m.LinkPID(consumer, tPID)
	m.LinkProcessID(consumer, tProc)
	m.LinkAlias(consumer, tAlias)
	m.LinkNode(consumer, tNode)

	if m.HasLink(consumer, tPID) == false {
		t.Error("HasLink should return true for PID")
	}
	if m.HasLink(consumer, tProc) == false {
		t.Error("HasLink should return true for ProcessID")
	}
	if m.HasLink(consumer, tAlias) == false {
		t.Error("HasLink should return true for Alias")
	}
	if m.HasLink(consumer, tNode) == false {
		t.Error("HasLink should return true for Node")
	}
	if m.totalLinks() != 4 {
		t.Errorf("Expected 4 link relations, got %d", m.totalLinks())
	}
}

func TestHasLink_DifferentConsumers(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.PID{Node: "node1", ID: 200}

	m.LinkPID(c1, target)
	if m.HasLink(c1, target) == false {
		t.Error("c1 should have link")
	}
	if m.HasLink(c2, target) {
		t.Error("c2 should NOT have link")
	}
}

// HasMonitor

func TestHasMonitor_Basic(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	if m.HasMonitor(consumer, target) {
		t.Error("HasMonitor should return false before monitor")
	}
	m.MonitorPID(consumer, target)
	if m.HasMonitor(consumer, target) == false {
		t.Error("HasMonitor should return true after monitor")
	}
	m.DemonitorPID(consumer, target)
	if m.HasMonitor(consumer, target) {
		t.Error("HasMonitor should return false after demonitor")
	}
}

func TestHasMonitor_DifferentTargetTypes(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	tPID := gen.PID{Node: "node1", ID: 200}
	tProc := gen.ProcessID{Name: "proc", Node: "node1"}
	tAlias := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	tNode := gen.Atom("node2")

	m.MonitorPID(consumer, tPID)
	m.MonitorProcessID(consumer, tProc)
	m.MonitorAlias(consumer, tAlias)
	m.MonitorNode(consumer, tNode)

	if m.HasMonitor(consumer, tPID) == false {
		t.Error("HasMonitor should return true for PID")
	}
	if m.HasMonitor(consumer, tProc) == false {
		t.Error("HasMonitor should return true for ProcessID")
	}
	if m.HasMonitor(consumer, tAlias) == false {
		t.Error("HasMonitor should return true for Alias")
	}
	if m.HasMonitor(consumer, tNode) == false {
		t.Error("HasMonitor should return true for Node")
	}
	if m.totalMonitors() != 4 {
		t.Errorf("Expected 4 monitor relations, got %d", m.totalMonitors())
	}
}

func TestHasMonitor_DifferentConsumers(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.PID{Node: "node1", ID: 200}

	m.MonitorPID(c1, target)
	if m.HasMonitor(c1, target) == false {
		t.Error("c1 should have monitor")
	}
	if m.HasMonitor(c2, target) {
		t.Error("c2 should NOT have monitor")
	}
}

// LinksFor

func TestLinksFor_Empty(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}

	if targets := m.LinksFor(consumer); targets != nil {
		t.Errorf("Expected nil for no links, got %v", targets)
	}
}

func TestLinksFor_SingleTarget(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	m.LinkPID(consumer, target)
	targets := m.LinksFor(consumer)
	if len(targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(targets))
	}
	if targets[0] != target {
		t.Errorf("Expected %v, got %v", target, targets[0])
	}
}

func TestLinksFor_MultipleTargets_DifferentTypes(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	tPID := gen.PID{Node: "node1", ID: 200}
	tProc := gen.ProcessID{Name: "proc1", Node: "node1"}
	tAlias := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	tNode := gen.Atom("node2")

	m.LinkPID(consumer, tPID)
	m.LinkProcessID(consumer, tProc)
	m.LinkAlias(consumer, tAlias)
	m.LinkNode(consumer, tNode)

	targets := m.LinksFor(consumer)
	if len(targets) != 4 {
		t.Fatalf("Expected 4 targets, got %d", len(targets))
	}
	found := map[any]bool{}
	for _, x := range targets {
		found[x] = true
	}
	if found[tPID] == false || found[tProc] == false || found[tAlias] == false || found[tNode] == false {
		t.Errorf("missing targets in LinksFor: %v", targets)
	}
}

func TestLinksFor_AfterRemovingOne(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	t1 := gen.PID{Node: "node1", ID: 200}
	t2 := gen.PID{Node: "node1", ID: 201}
	t3 := gen.PID{Node: "node1", ID: 202}

	m.LinkPID(consumer, t1)
	m.LinkPID(consumer, t2)
	m.LinkPID(consumer, t3)
	m.UnlinkPID(consumer, t2)

	targets := m.LinksFor(consumer)
	if len(targets) != 2 {
		t.Fatalf("Expected 2 targets after removal, got %d", len(targets))
	}
	for _, x := range targets {
		if x == t2 {
			t.Error("Removed target should not be in results")
		}
	}
	if m.hasLinkRelation(consumer, t2) {
		t.Error("linkRelations should not contain removed t2")
	}
	if m.totalLinks() != 2 {
		t.Errorf("expected 2 linkRelations remaining, got %d", m.totalLinks())
	}
}

func TestLinksFor_AfterRemovingAll(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	t1 := gen.PID{Node: "node1", ID: 200}
	t2 := gen.PID{Node: "node1", ID: 201}

	m.LinkPID(consumer, t1)
	m.LinkPID(consumer, t2)
	m.UnlinkPID(consumer, t1)
	m.UnlinkPID(consumer, t2)
	// After all unlinks the reverse-index entry stays but is empty;
	// LinksFor returns nil.
	if targets := m.LinksFor(consumer); len(targets) != 0 {
		t.Errorf("Expected empty after removing all, got %v", targets)
	}
	if m.totalLinks() != 0 {
		t.Errorf("expected 0 linkRelations, got %d", m.totalLinks())
	}
}

func TestLinksFor_MultipleConsumers_Isolation(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	t1 := gen.PID{Node: "node1", ID: 200}
	t2 := gen.PID{Node: "node1", ID: 201}

	m.LinkPID(c1, t1)
	m.LinkPID(c2, t2)

	t1List := m.LinksFor(c1)
	if len(t1List) != 1 || t1List[0] != t1 {
		t.Fatalf("c1 isolation: got %v", t1List)
	}
	t2List := m.LinksFor(c2)
	if len(t2List) != 1 || t2List[0] != t2 {
		t.Fatalf("c2 isolation: got %v", t2List)
	}
}

func TestLinksFor_RemoteTargets(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	local := gen.PID{Node: "node1", ID: 200}
	remote := gen.PID{Node: "node2", ID: 300}

	m.LinkPID(consumer, local)
	m.LinkPID(consumer, remote)

	targets := m.LinksFor(consumer)
	if len(targets) != 2 {
		t.Fatalf("Expected 2 targets, got %d", len(targets))
	}
	found := map[any]bool{}
	for _, x := range targets {
		found[x] = true
	}
	if found[local] == false || found[remote] == false {
		t.Errorf("missing targets: %v", targets)
	}
}

// MonitorsFor

func TestMonitorsFor_Empty(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}

	if targets := m.MonitorsFor(consumer); targets != nil {
		t.Errorf("Expected nil for no monitors, got %v", targets)
	}
}

func TestMonitorsFor_SingleTarget(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	m.MonitorPID(consumer, target)
	targets := m.MonitorsFor(consumer)
	if len(targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(targets))
	}
	if targets[0] != target {
		t.Errorf("Expected %v, got %v", target, targets[0])
	}
}

func TestMonitorsFor_MultipleTargets_DifferentTypes(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	tPID := gen.PID{Node: "node1", ID: 200}
	tProc := gen.ProcessID{Name: "proc1", Node: "node1"}
	tAlias := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	tNode := gen.Atom("node2")

	m.MonitorPID(consumer, tPID)
	m.MonitorProcessID(consumer, tProc)
	m.MonitorAlias(consumer, tAlias)
	m.MonitorNode(consumer, tNode)

	targets := m.MonitorsFor(consumer)
	if len(targets) != 4 {
		t.Fatalf("Expected 4 targets, got %d", len(targets))
	}
	found := map[any]bool{}
	for _, x := range targets {
		found[x] = true
	}
	if found[tPID] == false || found[tProc] == false || found[tAlias] == false || found[tNode] == false {
		t.Errorf("missing targets: %v", targets)
	}
}

func TestMonitorsFor_AfterRemovingOne(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	t1 := gen.PID{Node: "node1", ID: 200}
	t2 := gen.PID{Node: "node1", ID: 201}
	t3 := gen.PID{Node: "node1", ID: 202}

	m.MonitorPID(consumer, t1)
	m.MonitorPID(consumer, t2)
	m.MonitorPID(consumer, t3)
	m.DemonitorPID(consumer, t2)

	targets := m.MonitorsFor(consumer)
	if len(targets) != 2 {
		t.Fatalf("Expected 2 monitors after removal, got %d", len(targets))
	}
	if m.hasMonitorRelation(consumer, t2) {
		t.Error("monitorRelations should not contain removed t2")
	}
	if m.totalMonitors() != 2 {
		t.Errorf("expected 2 monitorRelations remaining, got %d", m.totalMonitors())
	}
}

func TestMonitorsFor_AfterRemovingAll(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	t1 := gen.PID{Node: "node1", ID: 200}
	t2 := gen.PID{Node: "node1", ID: 201}

	m.MonitorPID(consumer, t1)
	m.MonitorPID(consumer, t2)
	m.DemonitorPID(consumer, t1)
	m.DemonitorPID(consumer, t2)

	if targets := m.MonitorsFor(consumer); len(targets) != 0 {
		t.Errorf("Expected empty after removing all, got %v", targets)
	}
	if m.totalMonitors() != 0 {
		t.Errorf("expected 0 monitorRelations, got %d", m.totalMonitors())
	}
}

func TestMonitorsFor_MultipleConsumers_Isolation(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	t1 := gen.PID{Node: "node1", ID: 200}
	t2 := gen.PID{Node: "node1", ID: 201}

	m.MonitorPID(c1, t1)
	m.MonitorPID(c2, t2)

	t1List := m.MonitorsFor(c1)
	if len(t1List) != 1 || t1List[0] != t1 {
		t.Fatalf("c1 isolation: got %v", t1List)
	}
	t2List := m.MonitorsFor(c2)
	if len(t2List) != 1 || t2List[0] != t2 {
		t.Fatalf("c2 isolation: got %v", t2List)
	}
}

func TestMonitorsFor_SeparateFromLinks(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	linked := gen.PID{Node: "node1", ID: 200}
	monitored := gen.PID{Node: "node1", ID: 201}

	m.LinkPID(consumer, linked)
	m.MonitorPID(consumer, monitored)

	monitors := m.MonitorsFor(consumer)
	if len(monitors) != 1 || monitors[0] != monitored {
		t.Fatalf("MonitorsFor: got %v", monitors)
	}
	links := m.LinksFor(consumer)
	if len(links) != 1 || links[0] != linked {
		t.Fatalf("LinksFor: got %v", links)
	}
}

func TestMonitorsFor_RemoteTargets(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	local := gen.PID{Node: "node1", ID: 200}
	remote := gen.PID{Node: "node2", ID: 300}

	m.MonitorPID(consumer, local)
	m.MonitorPID(consumer, remote)

	targets := m.MonitorsFor(consumer)
	if len(targets) != 2 {
		t.Fatalf("Expected 2 targets, got %d", len(targets))
	}
	found := map[any]bool{}
	for _, x := range targets {
		found[x] = true
	}
	if found[local] == false || found[remote] == false {
		t.Errorf("missing targets: %v", targets)
	}
}

func TestLinkAndMonitor_SameTarget_SeparateRelations(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	if err := m.LinkPID(consumer, target); err != nil {
		t.Fatalf("LinkPID failed: %v", err)
	}
	if err := m.MonitorPID(consumer, target); err != nil {
		t.Fatalf("MonitorPID failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) == false {
		t.Error("Link should exist")
	}
	if m.hasMonitorRelation(consumer, target) == false {
		t.Error("Monitor should exist")
	}

	// Both wire-link and wire-monitor go out because they live in
	// separate (target, kind) wirePresence slots.
	if core.countSentLinks() != 1 {
		t.Errorf("Expected 1 wire LinkPID, got %d", core.countSentLinks())
	}
	if core.countSentMonitors() != 1 {
		t.Errorf("Expected 1 wire MonitorPID, got %d", core.countSentMonitors())
	}
}

// EventsFor

func TestEventsFor_Basic(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}

	m.RegisterEvent(producer, "event1", gen.EventOptions{})
	m.RegisterEvent(producer, "event2", gen.EventOptions{})
	m.RegisterEvent(producer, "event3", gen.EventOptions{})

	events := m.EventsFor(producer)
	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}
	names := map[gen.Atom]bool{}
	for _, e := range events {
		names[e.Name] = true
	}
	if names["event1"] == false || names["event2"] == false || names["event3"] == false {
		t.Errorf("missing event names: %v", names)
	}
	if m.totalEvents() != 3 {
		t.Errorf("expected 3 events, got %d", m.totalEvents())
	}
	if pe := m.producerEvents(producer); len(pe) != 3 {
		t.Errorf("expected 3 producer events, got %d", len(pe))
	}
}

func TestEventsFor_UnknownProducer(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	other := gen.PID{Node: "node1", ID: 200}

	m.RegisterEvent(producer, "test", gen.EventOptions{})

	if events := m.EventsFor(other); events != nil {
		t.Errorf("Expected nil for unknown producer, got %v", events)
	}
}

func TestEventsFor_AfterUnregister(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}

	m.RegisterEvent(producer, "event1", gen.EventOptions{})
	m.RegisterEvent(producer, "event2", gen.EventOptions{})
	m.UnregisterEvent(producer, "event1")

	events := m.EventsFor(producer)
	if len(events) != 1 {
		t.Errorf("Expected 1 event after unregister, got %d", len(events))
	}
	if events[0].Name != "event2" {
		t.Errorf("Wrong event remaining: %v", events[0].Name)
	}
	if m.getEventEntry(gen.Event{Node: "node1", Name: "event1"}) != nil {
		t.Error("event1 should be removed from m.events")
	}
	if m.getEventEntry(gen.Event{Node: "node1", Name: "event2"}) == nil {
		t.Error("event2 should still exist in m.events")
	}
}

func TestEventsFor_Empty(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}

	if events := m.EventsFor(producer); events != nil {
		t.Errorf("Expected nil, got %v", events)
	}
}

func TestEventsFor_AfterUnregisterAll(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}

	m.RegisterEvent(producer, "event1", gen.EventOptions{})
	m.RegisterEvent(producer, "event2", gen.EventOptions{})
	m.UnregisterEvent(producer, "event1")
	m.UnregisterEvent(producer, "event2")

	if events := m.EventsFor(producer); events != nil {
		t.Errorf("Expected nil, got %v", events)
	}
	if m.totalEvents() != 0 {
		t.Errorf("expected 0 events, got %d", m.totalEvents())
	}
}

func TestEventsFor_MultipleProducers(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	p1 := gen.PID{Node: "node1", ID: 100}
	p2 := gen.PID{Node: "node1", ID: 101}

	m.RegisterEvent(p1, "event1", gen.EventOptions{})
	m.RegisterEvent(p1, "event2", gen.EventOptions{})
	m.RegisterEvent(p2, "event3", gen.EventOptions{})

	if events := m.EventsFor(p1); len(events) != 2 {
		t.Errorf("Expected 2 events for p1, got %d", len(events))
	}
	if events := m.EventsFor(p2); len(events) != 1 {
		t.Errorf("Expected 1 event for p2, got %d", len(events))
	}
}
