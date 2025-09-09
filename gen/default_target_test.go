package gen

import (
	"testing"
)

func TestDefaultTargetManager_BasicOperations(t *testing.T) {
	tm := CreateTargetManager()

	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	target := PID{Node: "node2", ID: 2, Creation: 1}

	// Test AddLink
	err := tm.AddLink(consumer, target)
	if err != nil {
		t.Fatalf("AddLink failed: %v", err)
	}

	// Test HasLink
	if !tm.HasLink(consumer, target) {
		t.Fatal("HasLink should return true after AddLink")
	}

	// Test duplicate AddLink
	err = tm.AddLink(consumer, target)
	if err != ErrTargetExist {
		t.Fatal("AddLink should return ErrTargetExist for duplicate")
	}

	// Test RemoveLink
	err = tm.RemoveLink(consumer, target)
	if err != nil {
		t.Fatalf("RemoveLink failed: %v", err)
	}

	// Test HasLink after removal
	if tm.HasLink(consumer, target) {
		t.Fatal("HasLink should return false after RemoveLink")
	}

	// Test RemoveLink for non-existent link
	err = tm.RemoveLink(consumer, target)
	if err != ErrTargetUnknown {
		t.Fatal("RemoveLink should return ErrTargetUnknown for non-existent link")
	}
}

func TestDefaultTargetManager_MonitorOperations(t *testing.T) {
	tm := CreateTargetManager()

	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	target := Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	// Test AddMonitor
	err := tm.AddMonitor(consumer, target)
	if err != nil {
		t.Fatalf("AddMonitor failed: %v", err)
	}

	// Test HasMonitor
	if !tm.HasMonitor(consumer, target) {
		t.Fatal("HasMonitor should return true after AddMonitor")
	}

	// Test duplicate AddMonitor
	err = tm.AddMonitor(consumer, target)
	if err != ErrTargetExist {
		t.Fatal("AddMonitor should return ErrTargetExist for duplicate")
	}

	// Test RemoveMonitor
	err = tm.RemoveMonitor(consumer, target)
	if err != nil {
		t.Fatalf("RemoveMonitor failed: %v", err)
	}

	// Test HasMonitor after removal
	if tm.HasMonitor(consumer, target) {
		t.Fatal("HasMonitor should return false after RemoveMonitor")
	}

	// Test RemoveMonitor for non-existent monitor
	err = tm.RemoveMonitor(consumer, target)
	if err != ErrTargetUnknown {
		t.Fatal("RemoveMonitor should return ErrTargetUnknown for non-existent monitor")
	}
}

func TestDefaultTargetManager_CleanupConsumer(t *testing.T) {
	tm := CreateTargetManager()

	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	otherConsumer := PID{Node: "node1", ID: 2, Creation: 1}
	target1 := PID{Node: "node2", ID: 1, Creation: 1}
	target2 := Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}
	target3 := Event{Node: "node2", Name: "test_event"}

	// Setup relationships
	tm.AddLink(consumer, target1)
	tm.AddLink(consumer, target2)
	tm.AddLink(otherConsumer, target1) // This should remain

	tm.AddMonitor(consumer, target2)
	tm.AddMonitor(consumer, target3)
	tm.AddMonitor(otherConsumer, target3) // This should remain

	// Cleanup consumer
	linkTargets, monitorTargets := tm.CleanupConsumer(consumer)

	// Check returned targets
	if len(linkTargets) != 2 {
		t.Fatalf("Expected 2 link targets, got %d", len(linkTargets))
	}
	if len(monitorTargets) != 2 {
		t.Fatalf("Expected 2 monitor targets, got %d", len(monitorTargets))
	}

	// Verify relationships are removed
	if tm.HasLink(consumer, target1) {
		t.Fatal("Link should be removed after CleanupConsumer")
	}
	if tm.HasLink(consumer, target2) {
		t.Fatal("Link should be removed after CleanupConsumer")
	}
	if tm.HasMonitor(consumer, target2) {
		t.Fatal("Monitor should be removed after CleanupConsumer")
	}
	if tm.HasMonitor(consumer, target3) {
		t.Fatal("Monitor should be removed after CleanupConsumer")
	}

	// Verify other consumer's relationships remain
	if !tm.HasLink(otherConsumer, target1) {
		t.Fatal("Other consumer's link should remain")
	}
	if !tm.HasMonitor(otherConsumer, target3) {
		t.Fatal("Other consumer's monitor should remain")
	}

}

func TestDefaultTargetManager_CleanupTarget(t *testing.T) {
	tm := CreateTargetManager()

	consumer1 := PID{Node: "node1", ID: 1, Creation: 1}
	consumer2 := PID{Node: "node1", ID: 2, Creation: 1}
	target := PID{Node: "node2", ID: 1, Creation: 1}
	otherTarget := Alias{Node: "node2", ID: [3]uint64{4, 5, 6}}

	// Setup relationships
	tm.AddLink(consumer1, target)
	tm.AddLink(consumer2, target)
	tm.AddLink(consumer1, otherTarget) // This should remain

	tm.AddMonitor(consumer1, target)
	tm.AddMonitor(consumer2, target)
	tm.AddMonitor(consumer2, otherTarget) // This should remain

	// Cleanup target
	linkConsumers, monitorConsumers := tm.CleanupTarget(target)

	// Check returned consumers
	if len(linkConsumers) != 2 {
		t.Fatalf("Expected 2 link consumers, got %d", len(linkConsumers))
	}
	if len(monitorConsumers) != 2 {
		t.Fatalf("Expected 2 monitor consumers, got %d", len(monitorConsumers))
	}

	// Verify relationships are removed
	if tm.HasLink(consumer1, target) {
		t.Fatal("Link should be removed after CleanupTarget")
	}
	if tm.HasLink(consumer2, target) {
		t.Fatal("Link should be removed after CleanupTarget")
	}
	if tm.HasMonitor(consumer1, target) {
		t.Fatal("Monitor should be removed after CleanupTarget")
	}
	if tm.HasMonitor(consumer2, target) {
		t.Fatal("Monitor should be removed after CleanupTarget")
	}

	// Verify other target relationships remain
	if !tm.HasLink(consumer1, otherTarget) {
		t.Fatal("Other target link should remain")
	}
	if !tm.HasMonitor(consumer2, otherTarget) {
		t.Fatal("Other target monitor should remain")
	}

}

func TestDefaultTargetManager_CleanupNode(t *testing.T) {
	tm := CreateTargetManager()

	node1 := Atom("node1")
	node2 := Atom("node2")
	
	consumer1 := PID{Node: node1, ID: 1, Creation: 1}
	consumer2 := PID{Node: node2, ID: 1, Creation: 1}
	target1 := PID{Node: node1, ID: 2, Creation: 1}
	target2 := Alias{Node: node2, ID: [3]uint64{1, 2, 3}}
	target3 := PID{Node: "node3", ID: 1, Creation: 1}

	// Setup relationships involving node1
	tm.AddLink(consumer1, target2)  // consumer from node1
	tm.AddLink(consumer2, target1)  // target from node1
	tm.AddLink(consumer2, target3)  // should remain (no node1 involvement)

	tm.AddMonitor(consumer1, target3)  // consumer from node1
	tm.AddMonitor(consumer2, target1)  // target from node1
	tm.AddMonitor(consumer2, target2)  // should remain (no node1 involvement)

	// Cleanup node1
	linkTargets, monitorTargets := tm.CleanupNode(node1)

	// Should find targets from relationships involving node1
	if len(linkTargets) != 2 {
		t.Fatalf("Expected 2 link targets from node cleanup, got %d", len(linkTargets))
	}
	if len(monitorTargets) != 2 {
		t.Fatalf("Expected 2 monitor targets from node cleanup, got %d", len(monitorTargets))
	}

	// Verify node1-related relationships are removed
	if tm.HasLink(consumer1, target2) {
		t.Fatal("Link with node1 consumer should be removed")
	}
	if tm.HasLink(consumer2, target1) {
		t.Fatal("Link with node1 target should be removed")
	}
	if tm.HasMonitor(consumer1, target3) {
		t.Fatal("Monitor with node1 consumer should be removed")
	}
	if tm.HasMonitor(consumer2, target1) {
		t.Fatal("Monitor with node1 target should be removed")
	}

	// Verify unrelated relationships remain
	if !tm.HasLink(consumer2, target3) {
		t.Fatal("Link not involving node1 should remain")
	}
	if !tm.HasMonitor(consumer2, target2) {
		t.Fatal("Monitor not involving node1 should remain")
	}

}

func TestDefaultTargetManager_InspectionOperations(t *testing.T) {
	tm := CreateTargetManager()

	consumer1 := PID{Node: "node1", ID: 1, Creation: 1}
	consumer2 := PID{Node: "node1", ID: 2, Creation: 1}
	target1 := PID{Node: "node2", ID: 1, Creation: 1}
	target2 := Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	// Setup relationships
	tm.AddLink(consumer1, target1)
	tm.AddLink(consumer1, target2)
	tm.AddMonitor(consumer1, target1)

	tm.AddLink(consumer2, target1)
	tm.AddMonitor(consumer2, target2)

	// Test GetTargetsForConsumer
	links, monitors := tm.GetTargetsForConsumer(consumer1)
	if len(links) != 2 {
		t.Fatalf("Expected 2 links for consumer1, got %d", len(links))
	}
	if len(monitors) != 1 {
		t.Fatalf("Expected 1 monitor for consumer1, got %d", len(monitors))
	}

	links2, monitors2 := tm.GetTargetsForConsumer(consumer2)
	if len(links2) != 1 {
		t.Fatalf("Expected 1 link for consumer2, got %d", len(links2))
	}
	if len(monitors2) != 1 {
		t.Fatalf("Expected 1 monitor for consumer2, got %d", len(monitors2))
	}

	// Test GetConsumersForTarget
	linkConsumers, monitorConsumers := tm.GetConsumersForTarget(target1)
	if len(linkConsumers) != 2 {
		t.Fatalf("Expected 2 link consumers for target1, got %d", len(linkConsumers))
	}
	if len(monitorConsumers) != 1 {
		t.Fatalf("Expected 1 monitor consumer for target1, got %d", len(monitorConsumers))
	}

	linkConsumers2, monitorConsumers2 := tm.GetConsumersForTarget(target2)
	if len(linkConsumers2) != 1 {
		t.Fatalf("Expected 1 link consumer for target2, got %d", len(linkConsumers2))
	}
	if len(monitorConsumers2) != 1 {
		t.Fatalf("Expected 1 monitor consumer for target2, got %d", len(monitorConsumers2))
	}
}

func TestDefaultTargetManager_TargetEquality(t *testing.T) {
	tm := CreateTargetManager()

	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	
	// Test different target types
	pidTarget1 := PID{Node: "node2", ID: 1, Creation: 1}
	pidTarget2 := PID{Node: "node2", ID: 1, Creation: 1} // Same as pidTarget1
	
	aliasTarget1 := Alias{Node: "node2", ID: [3]uint64{7, 8, 9}}
	aliasTarget2 := Alias{Node: "node2", ID: [3]uint64{7, 8, 9}} // Same as aliasTarget1
	
	eventTarget := Event{Node: "node2", Name: "event"}
	atomTarget := Atom("test_atom")

	// Test PID equality
	tm.AddLink(consumer, pidTarget1)
	if !tm.HasLink(consumer, pidTarget2) {
		t.Fatal("PID targets with same values should be considered equal")
	}

	// Test Alias equality
	tm.AddMonitor(consumer, aliasTarget1)
	if !tm.HasMonitor(consumer, aliasTarget2) {
		t.Fatal("Alias targets with same values should be considered equal")
	}

	// Test different target types are not equal
	tm.AddLink(consumer, eventTarget)
	tm.AddLink(consumer, atomTarget)
	tm.AddLink(consumer, aliasTarget1)  // Add alias as link too

	// Verify distinct targets
	links, _ := tm.GetTargetsForConsumer(consumer)
	if len(links) != 4 { // pidTarget1, eventTarget, atomTarget, aliasTarget1
		t.Fatalf("Expected 4 distinct link targets, got %d", len(links))
	}
}

func TestDefaultTargetManager_ConcurrentSafety(t *testing.T) {
	tm := CreateTargetManager()

	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	target := PID{Node: "node2", ID: 1, Creation: 1}

	// Test concurrent operations
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			tm.AddLink(consumer, target)
			tm.RemoveLink(consumer, target)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			tm.HasLink(consumer, target)
		}
		done <- true
	}()

	<-done
	<-done

	// Should end in consistent state
	if tm.HasLink(consumer, target) {
		t.Fatal("Expected no link to exist after concurrent operations")
	}
}