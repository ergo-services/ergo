package tm

import (
	"testing"

	"ergo.services/ergo/gen"
)

// Node-target methods (LinkNode/UnlinkNode/MonitorNode/DemonitorNode)
// operate on gen.Atom and are local-only; no wire propagation.

func TestLinkNode_Basic(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	if err := m.LinkNode(consumer, target); err != nil {
		t.Fatalf("LinkNode failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) == false {
		t.Error("Node link should be stored")
	}
	if m.getTargetEntry(target) == nil {
		t.Fatal("target entry should be created")
	}
	if m.storage.Has(target, consumer, KindLink) == false {
		t.Error("Consumer should be in target index")
	}
}

func TestLinkNode_Duplicate_Error(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	m.LinkNode(consumer, target)
	if err := m.LinkNode(consumer, target); err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist, got %v", err)
	}
	if m.hasLinkRelation(consumer, target) == false {
		t.Error("Original link should still be stored")
	}
	if m.totalLinks() != 1 {
		t.Errorf("Expected 1 link total, got %d", m.totalLinks())
	}
	if m.consumerCount(target) != 1 {
		t.Errorf("Expected 1 consumer, got %d", m.consumerCount(target))
	}
}

func TestLinkNode_MultipleConsumers(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	c3 := gen.PID{Node: "node1", ID: 102}
	target := gen.Atom("node2")

	m.LinkNode(c1, target)
	m.LinkNode(c2, target)
	m.LinkNode(c3, target)

	if m.HasLink(c1, target) == false {
		t.Error("c1 should be linked")
	}
	if m.HasLink(c2, target) == false {
		t.Error("c2 should be linked")
	}
	if m.HasLink(c3, target) == false {
		t.Error("c3 should be linked")
	}
	if m.consumerCount(target) != 3 {
		t.Errorf("Expected 3 consumers, got %d", m.consumerCount(target))
	}
}

func TestUnlinkNode_Basic(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	m.LinkNode(consumer, target)
	if err := m.UnlinkNode(consumer, target); err != nil {
		t.Fatalf("UnlinkNode failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) {
		t.Error("Node link should be removed")
	}
}

func TestUnlinkNode_NotLast(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Atom("node2")

	m.LinkNode(c1, target)
	m.LinkNode(c2, target)

	m.UnlinkNode(c1, target)
	if m.hasLinkRelation(c1, target) {
		t.Error("c1 link should be removed")
	}
	if m.hasLinkRelation(c2, target) == false {
		t.Error("c2 link should still exist")
	}
	if m.consumerCount(target) != 1 {
		t.Errorf("Expected 1 consumer, got %d", m.consumerCount(target))
	}
}

func TestUnlinkNode_NonExistent_IsIdempotent(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	if err := m.UnlinkNode(consumer, target); err != nil {
		t.Fatalf("UnlinkNode non-existent should be idempotent, got %v", err)
	}
}

func TestMonitorNode_Basic(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	if err := m.MonitorNode(consumer, target); err != nil {
		t.Fatalf("MonitorNode failed: %v", err)
	}
	if m.hasMonitorRelation(consumer, target) == false {
		t.Error("Node monitor should be stored")
	}
	if m.consumerCount(target) != 1 {
		t.Errorf("Expected 1 consumer, got %d", m.consumerCount(target))
	}
}

func TestMonitorNode_Duplicate_Error(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	m.MonitorNode(consumer, target)
	if err := m.MonitorNode(consumer, target); err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist, got %v", err)
	}
	if m.hasMonitorRelation(consumer, target) == false {
		t.Error("Original monitor should still be stored")
	}
	if m.totalMonitors() != 1 {
		t.Errorf("Expected 1 monitor total, got %d", m.totalMonitors())
	}
}

func TestMonitorNode_MultipleConsumers(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Atom("node2")

	m.MonitorNode(c1, target)
	m.MonitorNode(c2, target)

	if m.HasMonitor(c1, target) == false {
		t.Error("c1 should be monitoring")
	}
	if m.HasMonitor(c2, target) == false {
		t.Error("c2 should be monitoring")
	}
	if m.consumerCount(target) != 2 {
		t.Errorf("Expected 2 consumers, got %d", m.consumerCount(target))
	}
}

func TestDemonitorNode_Basic(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	m.MonitorNode(consumer, target)
	if err := m.DemonitorNode(consumer, target); err != nil {
		t.Fatalf("DemonitorNode failed: %v", err)
	}
	if m.hasMonitorRelation(consumer, target) {
		t.Error("Node monitor should be removed")
	}
}

func TestDemonitorNode_NotLast(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Atom("node2")

	m.MonitorNode(c1, target)
	m.MonitorNode(c2, target)

	m.DemonitorNode(c1, target)
	if m.hasMonitorRelation(c1, target) {
		t.Error("c1 monitor should be removed")
	}
	if m.hasMonitorRelation(c2, target) == false {
		t.Error("c2 monitor should still exist")
	}
	if m.consumerCount(target) != 1 {
		t.Errorf("Expected 1 consumer, got %d", m.consumerCount(target))
	}
}

func TestDemonitorNode_NonExistent_IsIdempotent(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	if err := m.DemonitorNode(consumer, target); err != nil {
		t.Fatalf("DemonitorNode non-existent should be idempotent, got %v", err)
	}
}

func TestLinkAndMonitorNode_SameTarget(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	if err := m.LinkNode(consumer, target); err != nil {
		t.Fatalf("LinkNode failed: %v", err)
	}
	if err := m.MonitorNode(consumer, target); err != nil {
		t.Fatalf("MonitorNode failed: %v", err)
	}
	if m.HasLink(consumer, target) == false {
		t.Error("should have link")
	}
	if m.HasMonitor(consumer, target) == false {
		t.Error("should have monitor")
	}

	links := m.LinksFor(consumer)
	monitors := m.MonitorsFor(consumer)
	if len(links) != 1 {
		t.Errorf("Expected 1 link, got %d", len(links))
	}
	if len(monitors) != 1 {
		t.Errorf("Expected 1 monitor, got %d", len(monitors))
	}
}
