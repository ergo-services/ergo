package tm

import (
	"testing"

	"ergo.services/ergo/gen"
)

// LinkNode tests

func TestLinkNode_Basic(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	err := tm.LinkNode(consumer, target)
	if err != nil {
		t.Fatalf("LinkNode failed: %v", err)
	}

	// Stored
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists == false {
		t.Error("Node link should be stored")
	}

	// Verify in targetIndex
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetEntry should be created")
	}

	if _, exists := entry.consumers[consumer]; exists == false {
		t.Error("Consumer should be in targetEntry.consumers")
	}
}

func TestLinkNode_Duplicate_Error(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	tm.LinkNode(consumer, target)

	err := tm.LinkNode(consumer, target)
	if err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist, got %v", err)
	}

	// Verify first link still stored (not corrupted by duplicate attempt)
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists == false {
		t.Error("Original link should still be stored")
	}

	// Verify only 1 link in linkRelations for this consumer
	count := 0
	for k := range tm.linkRelations {
		if k.consumer == consumer {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Expected 1 link for consumer, got %d", count)
	}

	// Verify targetIndex has exactly 1 consumer
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex should exist")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("targetIndex should have 1 consumer, got %d", len(entry.consumers))
	}
}

func TestLinkNode_MultipleConsumers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	consumer3 := gen.PID{Node: "node1", ID: 102}
	target := gen.Atom("node2")

	tm.LinkNode(consumer1, target)
	tm.LinkNode(consumer2, target)
	tm.LinkNode(consumer3, target)

	// All should be linked
	if tm.HasLink(consumer1, target) == false {
		t.Error("consumer1 should be linked")
	}
	if tm.HasLink(consumer2, target) == false {
		t.Error("consumer2 should be linked")
	}
	if tm.HasLink(consumer3, target) == false {
		t.Error("consumer3 should be linked")
	}

	// Verify targetIndex has all consumers
	entry := tm.targetIndex[target]
	if len(entry.consumers) != 3 {
		t.Errorf("Expected 3 consumers, got %d", len(entry.consumers))
	}
}

// UnlinkNode tests

func TestUnlinkNode_Basic(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	tm.LinkNode(consumer, target)

	err := tm.UnlinkNode(consumer, target)
	if err != nil {
		t.Fatalf("UnlinkNode failed: %v", err)
	}

	// Removed
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("Node link should be removed")
	}

	// targetIndex cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

func TestUnlinkNode_NotLast(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Atom("node2")

	tm.LinkNode(consumer1, target)
	tm.LinkNode(consumer2, target)

	tm.UnlinkNode(consumer1, target)

	// consumer1 removed
	key1 := relationKey{consumer: consumer1, target: target}
	if _, exists := tm.linkRelations[key1]; exists {
		t.Error("consumer1 link should be removed")
	}

	// consumer2 still exists
	key2 := relationKey{consumer: consumer2, target: target}
	if _, exists := tm.linkRelations[key2]; exists == false {
		t.Error("consumer2 link should still exist")
	}

	// targetIndex still exists with consumer2
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex should still exist")
	}
	if _, exists := entry.consumers[consumer2]; exists == false {
		t.Error("consumer2 should still be in targetIndex.consumers")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("targetIndex should have 1 consumer, got %d", len(entry.consumers))
	}
}

func TestUnlinkNode_NonExistent_IsIdempotent(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	err := tm.UnlinkNode(consumer, target)
	if err != nil {
		t.Fatalf("UnlinkNode non-existent should be idempotent, got: %v", err)
	}
}

// MonitorNode tests

func TestMonitorNode_Basic(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	err := tm.MonitorNode(consumer, target)
	if err != nil {
		t.Fatalf("MonitorNode failed: %v", err)
	}

	// Stored in monitorRelations
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists == false {
		t.Error("Node monitor should be stored")
	}

	// Verify in targetIndex
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetEntry should be created")
	}

	// Verify consumer is in targetIndex.consumers
	if _, exists := entry.consumers[consumer]; exists == false {
		t.Error("Consumer should be in targetIndex.consumers")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("targetIndex should have 1 consumer, got %d", len(entry.consumers))
	}
}

func TestMonitorNode_Duplicate_Error(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	tm.MonitorNode(consumer, target)

	err := tm.MonitorNode(consumer, target)
	if err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist, got %v", err)
	}

	// Verify first monitor still stored (not corrupted by duplicate attempt)
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists == false {
		t.Error("Original monitor should still be stored")
	}

	// Verify only 1 monitor in monitorRelations for this consumer
	count := 0
	for k := range tm.monitorRelations {
		if k.consumer == consumer {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Expected 1 monitor for consumer, got %d", count)
	}

	// Verify targetIndex has exactly 1 consumer
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex should exist")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("targetIndex should have 1 consumer, got %d", len(entry.consumers))
	}
}

func TestMonitorNode_MultipleConsumers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Atom("node2")

	tm.MonitorNode(consumer1, target)
	tm.MonitorNode(consumer2, target)

	// All should be monitoring
	if tm.HasMonitor(consumer1, target) == false {
		t.Error("consumer1 should be monitoring")
	}
	if tm.HasMonitor(consumer2, target) == false {
		t.Error("consumer2 should be monitoring")
	}

	// Verify both stored in monitorRelations
	key1 := relationKey{consumer: consumer1, target: target}
	if _, exists := tm.monitorRelations[key1]; exists == false {
		t.Error("consumer1 should be in monitorRelations")
	}
	key2 := relationKey{consumer: consumer2, target: target}
	if _, exists := tm.monitorRelations[key2]; exists == false {
		t.Error("consumer2 should be in monitorRelations")
	}

	// Verify targetIndex has both consumers
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex should exist")
	}
	if len(entry.consumers) != 2 {
		t.Errorf("targetIndex should have 2 consumers, got %d", len(entry.consumers))
	}
}

// DemonitorNode tests

func TestDemonitorNode_Basic(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	tm.MonitorNode(consumer, target)

	err := tm.DemonitorNode(consumer, target)
	if err != nil {
		t.Fatalf("DemonitorNode failed: %v", err)
	}

	// Removed
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("Node monitor should be removed")
	}

	// targetIndex cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

func TestDemonitorNode_NotLast(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Atom("node2")

	tm.MonitorNode(consumer1, target)
	tm.MonitorNode(consumer2, target)

	tm.DemonitorNode(consumer1, target)

	// consumer1 removed
	key1 := relationKey{consumer: consumer1, target: target}
	if _, exists := tm.monitorRelations[key1]; exists {
		t.Error("consumer1 monitor should be removed")
	}

	// consumer2 still exists
	key2 := relationKey{consumer: consumer2, target: target}
	if _, exists := tm.monitorRelations[key2]; exists == false {
		t.Error("consumer2 monitor should still exist")
	}

	// targetIndex still exists with consumer2
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex should still exist")
	}
	if _, exists := entry.consumers[consumer2]; exists == false {
		t.Error("consumer2 should still be in targetIndex.consumers")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("targetIndex should have 1 consumer, got %d", len(entry.consumers))
	}
}

func TestDemonitorNode_NonExistent_IsIdempotent(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	err := tm.DemonitorNode(consumer, target)
	if err != nil {
		t.Fatalf("DemonitorNode non-existent should be idempotent, got: %v", err)
	}
}

// Mixed Link and Monitor to Node tests

func TestLinkAndMonitorNode_SameTarget(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")

	// Link to node
	err := tm.LinkNode(consumer, target)
	if err != nil {
		t.Fatalf("LinkNode failed: %v", err)
	}

	// Also monitor node
	err = tm.MonitorNode(consumer, target)
	if err != nil {
		t.Fatalf("MonitorNode failed: %v", err)
	}

	// Both should exist
	if tm.HasLink(consumer, target) == false {
		t.Error("should have link")
	}
	if tm.HasMonitor(consumer, target) == false {
		t.Error("should have monitor")
	}

	// Links and monitors should be separate
	links := tm.LinksFor(consumer)
	monitors := tm.MonitorsFor(consumer)

	if len(links) != 1 {
		t.Errorf("Expected 1 link, got %d", len(links))
	}
	if len(monitors) != 1 {
		t.Errorf("Expected 1 monitor, got %d", len(monitors))
	}
}
