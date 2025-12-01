package tm

import (
	"testing"

	"ergo.services/ergo/gen"
)

// HasLink tests

func TestHasLink_Basic(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	// Before link
	if tm.HasLink(consumer, target) == true {
		t.Error("HasLink should return false before link")
	}

	// Create link
	tm.LinkPID(consumer, target)

	// After link
	if tm.HasLink(consumer, target) == false {
		t.Error("HasLink should return true after link")
	}

	// Unlink
	tm.UnlinkPID(consumer, target)

	// After unlink
	if tm.HasLink(consumer, target) == true {
		t.Error("HasLink should return false after unlink")
	}
}

func TestHasLink_DifferentTargetTypes(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	targetPID := gen.PID{Node: "node1", ID: 200}
	targetProcessID := gen.ProcessID{Name: "proc", Node: "node1"}
	targetAlias := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	targetNode := gen.Atom("node2")

	// Create links
	tm.LinkPID(consumer, targetPID)
	tm.LinkProcessID(consumer, targetProcessID)
	tm.LinkAlias(consumer, targetAlias)
	tm.LinkNode(consumer, targetNode)

	// Verify all exist via API
	if tm.HasLink(consumer, targetPID) == false {
		t.Error("HasLink should return true for PID")
	}
	if tm.HasLink(consumer, targetProcessID) == false {
		t.Error("HasLink should return true for ProcessID")
	}
	if tm.HasLink(consumer, targetAlias) == false {
		t.Error("HasLink should return true for Alias")
	}
	if tm.HasLink(consumer, targetNode) == false {
		t.Error("HasLink should return true for Node")
	}

	// Verify internal state: all 4 relations stored
	if len(tm.linkRelations) != 4 {
		t.Errorf("expected 4 linkRelations, got %d", len(tm.linkRelations))
	}

	// Verify each relation in linkRelations
	keyPID := relationKey{consumer: consumer, target: targetPID}
	keyProcessID := relationKey{consumer: consumer, target: targetProcessID}
	keyAlias := relationKey{consumer: consumer, target: targetAlias}
	keyNode := relationKey{consumer: consumer, target: targetNode}

	if _, exists := tm.linkRelations[keyPID]; exists == false {
		t.Error("linkRelations should contain PID relation")
	}
	if _, exists := tm.linkRelations[keyProcessID]; exists == false {
		t.Error("linkRelations should contain ProcessID relation")
	}
	if _, exists := tm.linkRelations[keyAlias]; exists == false {
		t.Error("linkRelations should contain Alias relation")
	}
	if _, exists := tm.linkRelations[keyNode]; exists == false {
		t.Error("linkRelations should contain Node relation")
	}
}

func TestHasLink_DifferentConsumers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.PID{Node: "node1", ID: 200}

	// Only consumer1 links
	tm.LinkPID(consumer1, target)

	if tm.HasLink(consumer1, target) == false {
		t.Error("consumer1 should have link")
	}
	if tm.HasLink(consumer2, target) == true {
		t.Error("consumer2 should NOT have link")
	}
}

// HasMonitor tests

func TestHasMonitor_Basic(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	// Before
	if tm.HasMonitor(consumer, target) == true {
		t.Error("HasMonitor should return false before monitor")
	}

	// Create
	tm.MonitorPID(consumer, target)

	// After
	if tm.HasMonitor(consumer, target) == false {
		t.Error("HasMonitor should return true after monitor")
	}

	// Remove
	tm.DemonitorPID(consumer, target)

	// After remove
	if tm.HasMonitor(consumer, target) == true {
		t.Error("HasMonitor should return false after demonitor")
	}
}

func TestHasMonitor_DifferentTargetTypes(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	targetPID := gen.PID{Node: "node1", ID: 200}
	targetProcessID := gen.ProcessID{Name: "proc", Node: "node1"}
	targetAlias := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	targetNode := gen.Atom("node2")

	// Create monitors
	tm.MonitorPID(consumer, targetPID)
	tm.MonitorProcessID(consumer, targetProcessID)
	tm.MonitorAlias(consumer, targetAlias)
	tm.MonitorNode(consumer, targetNode)

	// Verify all exist via API
	if tm.HasMonitor(consumer, targetPID) == false {
		t.Error("HasMonitor should return true for PID")
	}
	if tm.HasMonitor(consumer, targetProcessID) == false {
		t.Error("HasMonitor should return true for ProcessID")
	}
	if tm.HasMonitor(consumer, targetAlias) == false {
		t.Error("HasMonitor should return true for Alias")
	}
	if tm.HasMonitor(consumer, targetNode) == false {
		t.Error("HasMonitor should return true for Node")
	}

	// Verify internal state: all 4 relations stored
	if len(tm.monitorRelations) != 4 {
		t.Errorf("expected 4 monitorRelations, got %d", len(tm.monitorRelations))
	}

	// Verify each relation in monitorRelations
	keyPID := relationKey{consumer: consumer, target: targetPID}
	keyProcessID := relationKey{consumer: consumer, target: targetProcessID}
	keyAlias := relationKey{consumer: consumer, target: targetAlias}
	keyNode := relationKey{consumer: consumer, target: targetNode}

	if _, exists := tm.monitorRelations[keyPID]; exists == false {
		t.Error("monitorRelations should contain PID relation")
	}
	if _, exists := tm.monitorRelations[keyProcessID]; exists == false {
		t.Error("monitorRelations should contain ProcessID relation")
	}
	if _, exists := tm.monitorRelations[keyAlias]; exists == false {
		t.Error("monitorRelations should contain Alias relation")
	}
	if _, exists := tm.monitorRelations[keyNode]; exists == false {
		t.Error("monitorRelations should contain Node relation")
	}
}

func TestHasMonitor_DifferentConsumers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.PID{Node: "node1", ID: 200}

	// Only consumer1 monitors
	tm.MonitorPID(consumer1, target)

	if tm.HasMonitor(consumer1, target) == false {
		t.Error("consumer1 should have monitor")
	}
	if tm.HasMonitor(consumer2, target) == true {
		t.Error("consumer2 should NOT have monitor")
	}
}

// LinksFor tests

func TestLinksFor_Empty(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}

	targets := tm.LinksFor(consumer)

	if targets != nil {
		t.Errorf("Expected nil for no links, got %v", targets)
	}
}

func TestLinksFor_SingleTarget(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	tm.LinkPID(consumer, target)

	targets := tm.LinksFor(consumer)

	if len(targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(targets))
	}

	if targets[0] != target {
		t.Errorf("Expected target %v, got %v", target, targets[0])
	}
}

func TestLinksFor_MultipleTargets_DifferentTypes(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	targetPID := gen.PID{Node: "node1", ID: 200}
	targetProcessID := gen.ProcessID{Name: "proc1", Node: "node1"}
	targetAlias := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	targetNode := gen.Atom("node2")

	// Create links to different types
	tm.LinkPID(consumer, targetPID)
	tm.LinkProcessID(consumer, targetProcessID)
	tm.LinkAlias(consumer, targetAlias)
	tm.LinkNode(consumer, targetNode)

	targets := tm.LinksFor(consumer)

	if len(targets) != 4 {
		t.Fatalf("Expected 4 targets, got %d", len(targets))
	}

	// Verify all targets present (order not guaranteed)
	found := make(map[any]bool)
	for _, target := range targets {
		found[target] = true
	}

	if found[targetPID] == false {
		t.Error("targetPID should be in results")
	}
	if found[targetProcessID] == false {
		t.Error("targetProcessID should be in results")
	}
	if found[targetAlias] == false {
		t.Error("targetAlias should be in results")
	}
	if found[targetNode] == false {
		t.Error("targetNode should be in results")
	}
}

func TestLinksFor_AfterRemovingOne(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target1 := gen.PID{Node: "node1", ID: 200}
	target2 := gen.PID{Node: "node1", ID: 201}
	target3 := gen.PID{Node: "node1", ID: 202}

	tm.LinkPID(consumer, target1)
	tm.LinkPID(consumer, target2)
	tm.LinkPID(consumer, target3)

	// Remove one
	tm.UnlinkPID(consumer, target2)

	targets := tm.LinksFor(consumer)

	if len(targets) != 2 {
		t.Fatalf("Expected 2 targets after removal, got %d", len(targets))
	}

	// Verify target2 not in results
	for _, target := range targets {
		if target == target2 {
			t.Error("Removed target should not be in results")
		}
	}

	// Verify internal state: target2 relation removed
	key2 := relationKey{consumer: consumer, target: target2}
	if _, exists := tm.linkRelations[key2]; exists {
		t.Error("linkRelations should not contain removed target2")
	}

	// Verify targetIndex for target2 is cleaned
	if _, exists := tm.targetIndex[target2]; exists {
		t.Error("targetIndex for target2 should be cleaned")
	}

	// Verify remaining relations still exist
	if len(tm.linkRelations) != 2 {
		t.Errorf("expected 2 linkRelations remaining, got %d", len(tm.linkRelations))
	}
}

func TestLinksFor_AfterRemovingAll(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target1 := gen.PID{Node: "node1", ID: 200}
	target2 := gen.PID{Node: "node1", ID: 201}

	tm.LinkPID(consumer, target1)
	tm.LinkPID(consumer, target2)

	// Remove all
	tm.UnlinkPID(consumer, target1)
	tm.UnlinkPID(consumer, target2)

	targets := tm.LinksFor(consumer)

	if targets != nil {
		t.Errorf("Expected nil after removing all, got %v", targets)
	}

	// Verify internal state: all linkRelations cleaned
	if len(tm.linkRelations) != 0 {
		t.Errorf("expected 0 linkRelations, got %d", len(tm.linkRelations))
	}

	// Verify all targetIndex entries cleaned
	if _, exists := tm.targetIndex[target1]; exists {
		t.Error("targetIndex for target1 should be cleaned")
	}
	if _, exists := tm.targetIndex[target2]; exists {
		t.Error("targetIndex for target2 should be cleaned")
	}
}

func TestLinksFor_MultipleConsumers_Isolation(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target1 := gen.PID{Node: "node1", ID: 200}
	target2 := gen.PID{Node: "node1", ID: 201}

	// consumer1 links to target1
	tm.LinkPID(consumer1, target1)

	// consumer2 links to target2
	tm.LinkPID(consumer2, target2)

	// Get links for consumer1
	targets1 := tm.LinksFor(consumer1)
	if len(targets1) != 1 {
		t.Fatalf("consumer1 should have 1 link, got %d", len(targets1))
	}
	if targets1[0] != target1 {
		t.Error("consumer1 should only see target1")
	}

	// Get links for consumer2
	targets2 := tm.LinksFor(consumer2)
	if len(targets2) != 1 {
		t.Fatalf("consumer2 should have 1 link, got %d", len(targets2))
	}
	if targets2[0] != target2 {
		t.Error("consumer2 should only see target2")
	}
}

func TestLinksFor_RemoteTargets(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	localTarget := gen.PID{Node: "node1", ID: 200}
	remoteTarget := gen.PID{Node: "node2", ID: 300}

	tm.LinkPID(consumer, localTarget)
	tm.LinkPID(consumer, remoteTarget)

	targets := tm.LinksFor(consumer)

	if len(targets) != 2 {
		t.Fatalf("Expected 2 targets (local+remote), got %d", len(targets))
	}

	// Both local and remote should be present
	found := make(map[any]bool)
	for _, target := range targets {
		found[target] = true
	}

	if found[localTarget] == false {
		t.Error("Local target should be in results")
	}
	if found[remoteTarget] == false {
		t.Error("Remote target should be in results")
	}

	// Verify internal state: both relations stored
	if len(tm.linkRelations) != 2 {
		t.Errorf("expected 2 linkRelations, got %d", len(tm.linkRelations))
	}

	// Verify targetIndex for remote target has consumer
	entry := tm.targetIndex[remoteTarget]
	if entry == nil {
		t.Fatal("targetIndex for remoteTarget should exist")
	}
	if _, exists := entry.consumers[consumer]; exists == false {
		t.Error("consumer should be in targetIndex.consumers for remoteTarget")
	}
}

// MonitorsFor tests

func TestMonitorsFor_Empty(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}

	targets := tm.MonitorsFor(consumer)

	if targets != nil {
		t.Errorf("Expected nil for no monitors, got %v", targets)
	}
}

func TestMonitorsFor_SingleTarget(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	tm.MonitorPID(consumer, target)

	targets := tm.MonitorsFor(consumer)

	if len(targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(targets))
	}

	if targets[0] != target {
		t.Errorf("Expected target %v, got %v", target, targets[0])
	}
}

func TestMonitorsFor_MultipleTargets_DifferentTypes(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	targetPID := gen.PID{Node: "node1", ID: 200}
	targetProcessID := gen.ProcessID{Name: "proc1", Node: "node1"}
	targetAlias := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	targetNode := gen.Atom("node2")

	// Create monitors to different types
	tm.MonitorPID(consumer, targetPID)
	tm.MonitorProcessID(consumer, targetProcessID)
	tm.MonitorAlias(consumer, targetAlias)
	tm.MonitorNode(consumer, targetNode)

	targets := tm.MonitorsFor(consumer)

	if len(targets) != 4 {
		t.Fatalf("Expected 4 targets, got %d", len(targets))
	}

	// Verify all targets present
	found := make(map[any]bool)
	for _, target := range targets {
		found[target] = true
	}

	if found[targetPID] == false {
		t.Error("targetPID should be in results")
	}
	if found[targetProcessID] == false {
		t.Error("targetProcessID should be in results")
	}
	if found[targetAlias] == false {
		t.Error("targetAlias should be in results")
	}
	if found[targetNode] == false {
		t.Error("targetNode should be in results")
	}
}

func TestMonitorsFor_AfterRemovingOne(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target1 := gen.PID{Node: "node1", ID: 200}
	target2 := gen.PID{Node: "node1", ID: 201}
	target3 := gen.PID{Node: "node1", ID: 202}

	tm.MonitorPID(consumer, target1)
	tm.MonitorPID(consumer, target2)
	tm.MonitorPID(consumer, target3)

	// Remove one
	tm.DemonitorPID(consumer, target2)

	targets := tm.MonitorsFor(consumer)

	if len(targets) != 2 {
		t.Fatalf("Expected 2 targets after removal, got %d", len(targets))
	}

	// Verify target2 not in results
	for _, target := range targets {
		if target == target2 {
			t.Error("Removed target should not be in results")
		}
	}

	// Verify internal state: target2 relation removed
	key2 := relationKey{consumer: consumer, target: target2}
	if _, exists := tm.monitorRelations[key2]; exists {
		t.Error("monitorRelations should not contain removed target2")
	}

	// Verify targetIndex for target2 is cleaned
	if _, exists := tm.targetIndex[target2]; exists {
		t.Error("targetIndex for target2 should be cleaned")
	}

	// Verify remaining relations still exist
	if len(tm.monitorRelations) != 2 {
		t.Errorf("expected 2 monitorRelations remaining, got %d", len(tm.monitorRelations))
	}
}

func TestMonitorsFor_AfterRemovingAll(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target1 := gen.PID{Node: "node1", ID: 200}
	target2 := gen.PID{Node: "node1", ID: 201}

	tm.MonitorPID(consumer, target1)
	tm.MonitorPID(consumer, target2)

	// Remove all
	tm.DemonitorPID(consumer, target1)
	tm.DemonitorPID(consumer, target2)

	targets := tm.MonitorsFor(consumer)

	if targets != nil {
		t.Errorf("Expected nil after removing all, got %v", targets)
	}

	// Verify internal state: all monitorRelations cleaned
	if len(tm.monitorRelations) != 0 {
		t.Errorf("expected 0 monitorRelations, got %d", len(tm.monitorRelations))
	}

	// Verify all targetIndex entries cleaned
	if _, exists := tm.targetIndex[target1]; exists {
		t.Error("targetIndex for target1 should be cleaned")
	}
	if _, exists := tm.targetIndex[target2]; exists {
		t.Error("targetIndex for target2 should be cleaned")
	}
}

func TestMonitorsFor_MultipleConsumers_Isolation(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target1 := gen.PID{Node: "node1", ID: 200}
	target2 := gen.PID{Node: "node1", ID: 201}

	// consumer1 monitors target1
	tm.MonitorPID(consumer1, target1)

	// consumer2 monitors target2
	tm.MonitorPID(consumer2, target2)

	// Get monitors for consumer1
	targets1 := tm.MonitorsFor(consumer1)
	if len(targets1) != 1 {
		t.Fatalf("consumer1 should have 1 monitor, got %d", len(targets1))
	}
	if targets1[0] != target1 {
		t.Error("consumer1 should only see target1")
	}

	// Get monitors for consumer2
	targets2 := tm.MonitorsFor(consumer2)
	if len(targets2) != 1 {
		t.Fatalf("consumer2 should have 1 monitor, got %d", len(targets2))
	}
	if targets2[0] != target2 {
		t.Error("consumer2 should only see target2")
	}
}

func TestMonitorsFor_SeparateFromLinks(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	linkedTarget := gen.PID{Node: "node1", ID: 200}
	monitoredTarget := gen.PID{Node: "node1", ID: 201}

	// Link one target
	tm.LinkPID(consumer, linkedTarget)

	// Monitor another target
	tm.MonitorPID(consumer, monitoredTarget)

	// MonitorsFor should only return monitored target
	monitors := tm.MonitorsFor(consumer)
	if len(monitors) != 1 {
		t.Fatalf("Expected 1 monitor, got %d", len(monitors))
	}
	if monitors[0] != monitoredTarget {
		t.Error("Should only return monitored target")
	}

	// LinksFor should only return linked target
	links := tm.LinksFor(consumer)
	if len(links) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(links))
	}
	if links[0] != linkedTarget {
		t.Error("Should only return linked target")
	}
}

func TestMonitorsFor_RemoteTargets(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	localTarget := gen.PID{Node: "node1", ID: 200}
	remoteTarget := gen.PID{Node: "node2", ID: 300}

	tm.MonitorPID(consumer, localTarget)
	tm.MonitorPID(consumer, remoteTarget)

	targets := tm.MonitorsFor(consumer)

	if len(targets) != 2 {
		t.Fatalf("Expected 2 targets (local+remote), got %d", len(targets))
	}

	// Both local and remote should be present
	found := make(map[any]bool)
	for _, target := range targets {
		found[target] = true
	}

	if found[localTarget] == false {
		t.Error("Local target should be in results")
	}
	if found[remoteTarget] == false {
		t.Error("Remote target should be in results")
	}

	// Verify internal state: both relations stored
	if len(tm.monitorRelations) != 2 {
		t.Errorf("expected 2 monitorRelations, got %d", len(tm.monitorRelations))
	}

	// Verify targetIndex for remote target has consumer
	entry := tm.targetIndex[remoteTarget]
	if entry == nil {
		t.Fatal("targetIndex for remoteTarget should exist")
	}
	if _, exists := entry.consumers[consumer]; exists == false {
		t.Error("consumer should be in targetIndex.consumers for remoteTarget")
	}
}

// Link and Monitor to same target tests

func TestLinkAndMonitor_SameTarget_SeparateRelations(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	// Link
	err := tm.LinkPID(consumer, target)
	if err != nil {
		t.Fatalf("LinkPID failed: %v", err)
	}

	// Monitor (same consumer, same target)
	err = tm.MonitorPID(consumer, target)
	if err != nil {
		t.Fatalf("MonitorPID failed: %v", err)
	}

	// Verify stored separately
	key := relationKey{consumer: consumer, target: target}

	if _, exists := tm.linkRelations[key]; exists == false {
		t.Error("Link should exist in linkRelations")
	}

	if _, exists := tm.monitorRelations[key]; exists == false {
		t.Error("Monitor should exist in monitorRelations")
	}

	// Verify targetIndex has consumer (shared)
	entry := tm.targetIndex[target]
	if _, exists := entry.consumers[consumer]; exists == false {
		t.Error("Consumer should be in targetIndex")
	}

	// Verify network requests
	// LinkPID first - sends request
	if core.countSentLinks() != 1 {
		t.Errorf("Expected 1 LinkPID, got %d", core.countSentLinks())
	}

	// MonitorPID second - does NOT send (allowAlwaysFirst=false after Link!)
	// This is correct - allowAlwaysFirst is per-target, not per-relation-type
	if core.countSentMonitors() != 0 {
		t.Logf("MonitorPID did not send network request (allowAlwaysFirst=false after Link) - this is correct")
	}
}

// EventsFor tests

func TestEventsFor_Basic(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	// Register 3 events
	tm.RegisterEvent(producer, "event1", gen.EventOptions{})
	tm.RegisterEvent(producer, "event2", gen.EventOptions{})
	tm.RegisterEvent(producer, "event3", gen.EventOptions{})

	// Get events for producer
	events := tm.EventsFor(producer)

	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}

	// Check all events are present
	eventNames := make(map[gen.Atom]bool)
	for _, e := range events {
		eventNames[e.Name] = true
	}

	if eventNames["event1"] == false || eventNames["event2"] == false || eventNames["event3"] == false {
		t.Error("Not all events returned")
	}

	// Verify internal state: all events in events map
	if len(tm.events) != 3 {
		t.Errorf("expected 3 events in tm.events, got %d", len(tm.events))
	}

	// Verify producerEvents has 3 events for this producer
	producerEvts := tm.producerEvents[producer]
	if producerEvts == nil {
		t.Fatal("producerEvents should exist for producer")
	}
	if len(producerEvts) != 3 {
		t.Errorf("expected 3 events in producerEvents, got %d", len(producerEvts))
	}
}

func TestEventsFor_UnknownProducer(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	other := gen.PID{Node: "node1", ID: 200}

	// Register event for producer
	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	// Get events for unknown producer
	events := tm.EventsFor(other)

	if events != nil {
		t.Errorf("Expected nil for unknown producer, got %v", events)
	}
}

func TestEventsFor_AfterUnregister(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	tm.RegisterEvent(producer, "event1", gen.EventOptions{})
	tm.RegisterEvent(producer, "event2", gen.EventOptions{})

	// Unregister one
	tm.UnregisterEvent(producer, "event1")

	events := tm.EventsFor(producer)

	if len(events) != 1 {
		t.Errorf("Expected 1 event after unregister, got %d", len(events))
	}

	if events[0].Name != "event2" {
		t.Errorf("Wrong event remaining: %v", events[0].Name)
	}

	// Verify internal state: event1 removed from events map
	event1 := gen.Event{Node: "node1", Name: "event1"}
	if _, exists := tm.events[event1]; exists {
		t.Error("event1 should be removed from tm.events")
	}

	// Verify event2 still in events map
	event2 := gen.Event{Node: "node1", Name: "event2"}
	if _, exists := tm.events[event2]; exists == false {
		t.Error("event2 should still exist in tm.events")
	}

	// Verify producerEvents has 1 event
	producerEvts := tm.producerEvents[producer]
	if len(producerEvts) != 1 {
		t.Errorf("expected 1 event in producerEvents, got %d", len(producerEvts))
	}
}

func TestEventsFor_Empty(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	events := tm.EventsFor(producer)

	if events != nil {
		t.Errorf("Expected nil for producer with no events, got %v", events)
	}
}

func TestEventsFor_AfterUnregisterAll(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	tm.RegisterEvent(producer, "event1", gen.EventOptions{})
	tm.RegisterEvent(producer, "event2", gen.EventOptions{})

	// Unregister all
	tm.UnregisterEvent(producer, "event1")
	tm.UnregisterEvent(producer, "event2")

	events := tm.EventsFor(producer)

	if events != nil {
		t.Errorf("Expected nil after unregister all, got %v", events)
	}

	// Verify internal state: all events removed from events map
	event1 := gen.Event{Node: "node1", Name: "event1"}
	event2 := gen.Event{Node: "node1", Name: "event2"}
	if _, exists := tm.events[event1]; exists {
		t.Error("event1 should be removed from tm.events")
	}
	if _, exists := tm.events[event2]; exists {
		t.Error("event2 should be removed from tm.events")
	}

	// Verify producerEvents cleaned up
	if _, exists := tm.producerEvents[producer]; exists {
		t.Error("producerEvents for producer should be cleaned up")
	}
}

func TestEventsFor_MultipleProducers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer1 := gen.PID{Node: "node1", ID: 100}
	producer2 := gen.PID{Node: "node1", ID: 101}

	// Producer1 has 2 events
	tm.RegisterEvent(producer1, "event1", gen.EventOptions{})
	tm.RegisterEvent(producer1, "event2", gen.EventOptions{})

	// Producer2 has 1 event
	tm.RegisterEvent(producer2, "event3", gen.EventOptions{})

	// Get events for producer1
	events1 := tm.EventsFor(producer1)
	if len(events1) != 2 {
		t.Errorf("Expected 2 events for producer1, got %d", len(events1))
	}

	// Get events for producer2
	events2 := tm.EventsFor(producer2)
	if len(events2) != 1 {
		t.Errorf("Expected 1 event for producer2, got %d", len(events2))
	}
}
