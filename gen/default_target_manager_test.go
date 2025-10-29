package gen

import (
	"sync"
	"testing"
)

func TestDefaultTargetManager_AddLink(t *testing.T) {
	tm := CreateDefaultTargetManager()
	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	target := PID{Node: "node2", ID: 1, Creation: 1}

	// Add link
	if err := tm.AddLink(consumer, target); err != nil {
		t.Fatalf("AddLink failed: %v", err)
	}

	// Verify link exists
	if !tm.HasLink(consumer, target) {
		t.Fatalf("Link should exist")
	}

	// Try to add same link again
	if err := tm.AddLink(consumer, target); err != ErrTargetExist {
		t.Fatalf("Expected ErrTargetExist, got: %v", err)
	}
}

func TestDefaultTargetManager_RemoveLink(t *testing.T) {
	tm := CreateDefaultTargetManager()
	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	target := PID{Node: "node2", ID: 1, Creation: 1}

	// Try to remove non-existent link
	if err := tm.RemoveLink(consumer, target); err != ErrTargetUnknown {
		t.Fatalf("Expected ErrTargetUnknown, got: %v", err)
	}

	// Add and remove link
	tm.AddLink(consumer, target)
	if err := tm.RemoveLink(consumer, target); err != nil {
		t.Fatalf("RemoveLink failed: %v", err)
	}

	// Verify link is gone
	if tm.HasLink(consumer, target) {
		t.Fatalf("Link should not exist")
	}
}

func TestDefaultTargetManager_AddMonitor(t *testing.T) {
	tm := CreateDefaultTargetManager()
	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	target := PID{Node: "node2", ID: 1, Creation: 1}

	// Add monitor
	if err := tm.AddMonitor(consumer, target); err != nil {
		t.Fatalf("AddMonitor failed: %v", err)
	}

	// Verify monitor exists
	if !tm.HasMonitor(consumer, target) {
		t.Fatalf("Monitor should exist")
	}

	// Try to add same monitor again
	if err := tm.AddMonitor(consumer, target); err != ErrTargetExist {
		t.Fatalf("Expected ErrTargetExist, got: %v", err)
	}
}

func TestDefaultTargetManager_RemoveMonitor(t *testing.T) {
	tm := CreateDefaultTargetManager()
	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	target := PID{Node: "node2", ID: 1, Creation: 1}

	// Try to remove non-existent monitor
	if err := tm.RemoveMonitor(consumer, target); err != ErrTargetUnknown {
		t.Fatalf("Expected ErrTargetUnknown, got: %v", err)
	}

	// Add and remove monitor
	tm.AddMonitor(consumer, target)
	if err := tm.RemoveMonitor(consumer, target); err != nil {
		t.Fatalf("RemoveMonitor failed: %v", err)
	}

	// Verify monitor is gone
	if tm.HasMonitor(consumer, target) {
		t.Fatalf("Monitor should not exist")
	}
}

func TestDefaultTargetManager_LinkAndMonitorSeparation(t *testing.T) {
	tm := CreateDefaultTargetManager()
	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	target := PID{Node: "node2", ID: 1, Creation: 1}

	// Add both link and monitor for same consumer/target pair
	if err := tm.AddLink(consumer, target); err != nil {
		t.Fatalf("AddLink failed: %v", err)
	}
	if err := tm.AddMonitor(consumer, target); err != nil {
		t.Fatalf("AddMonitor failed: %v", err)
	}

	// Verify both exist independently
	if !tm.HasLink(consumer, target) {
		t.Fatalf("Link should exist")
	}
	if !tm.HasMonitor(consumer, target) {
		t.Fatalf("Monitor should exist")
	}

	// Remove link, monitor should still exist
	tm.RemoveLink(consumer, target)
	if tm.HasLink(consumer, target) {
		t.Fatalf("Link should not exist")
	}
	if !tm.HasMonitor(consumer, target) {
		t.Fatalf("Monitor should still exist")
	}

	// Remove monitor
	tm.RemoveMonitor(consumer, target)
	if tm.HasMonitor(consumer, target) {
		t.Fatalf("Monitor should not exist")
	}
}

func TestDefaultTargetManager_CleanupConsumer(t *testing.T) {
	tm := CreateDefaultTargetManager()
	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	target1 := PID{Node: "node2", ID: 1, Creation: 1}
	target2 := PID{Node: "node2", ID: 2, Creation: 1}
	target3 := "string_target"

	// Add some links and monitors
	tm.AddLink(consumer, target1)
	tm.AddLink(consumer, target2)
	tm.AddMonitor(consumer, target3)

	// Cleanup consumer
	linkTargets, monitorTargets := tm.CleanupConsumer(consumer)

	// Verify returned targets
	if len(linkTargets) != 2 {
		t.Fatalf("Expected 2 link targets, got %d", len(linkTargets))
	}
	if len(monitorTargets) != 1 {
		t.Fatalf("Expected 1 monitor target, got %d", len(monitorTargets))
	}

	// Verify relations are gone
	if tm.HasLink(consumer, target1) || tm.HasLink(consumer, target2) {
		t.Fatalf("Links should be gone")
	}
	if tm.HasMonitor(consumer, target3) {
		t.Fatalf("Monitor should be gone")
	}

	// Cleanup empty consumer should return empty slices
	linkTargets, monitorTargets = tm.CleanupConsumer(consumer)
	if len(linkTargets) != 0 || len(monitorTargets) != 0 {
		t.Fatalf("Cleanup of empty consumer should return empty slices")
	}
}

func TestDefaultTargetManager_CleanupConsumer_SharedTargets(t *testing.T) {
	tm := CreateDefaultTargetManager()
	
	// Multiple consumers linking to shared targets
	consumer1 := PID{Node: "node1", ID: 1, Creation: 1}
	consumer2 := PID{Node: "node1", ID: 2, Creation: 1}
	consumer3 := PID{Node: "node2", ID: 1, Creation: 1}
	
	sharedTarget1 := PID{Node: "target_node", ID: 1, Creation: 1}
	sharedTarget2 := PID{Node: "target_node", ID: 2, Creation: 1}
	consumer1OnlyTarget := "consumer1_only_target"

	// Multiple consumers link to the same targets
	tm.AddLink(consumer1, sharedTarget1)
	tm.AddLink(consumer2, sharedTarget1)  // Same target, different consumer
	tm.AddLink(consumer3, sharedTarget1)  // Same target, third consumer
	
	tm.AddLink(consumer1, sharedTarget2)
	tm.AddLink(consumer2, sharedTarget2)  // Same target, different consumer
	
	tm.AddMonitor(consumer1, sharedTarget1)
	tm.AddMonitor(consumer2, sharedTarget1)  // Same target, different consumer
	
	// consumer1 has exclusive target
	tm.AddLink(consumer1, consumer1OnlyTarget)

	// Cleanup consumer1 only
	linkTargets, monitorTargets := tm.CleanupConsumer(consumer1)

	// Verify consumer1's targets are returned
	if len(linkTargets) != 3 {
		t.Fatalf("Expected 3 link targets for consumer1, got %d", len(linkTargets))
	}
	if len(monitorTargets) != 1 {
		t.Fatalf("Expected 1 monitor target for consumer1, got %d", len(monitorTargets))
	}

	// Check that returned targets include all consumer1's targets
	linkTargetFound := make(map[any]bool)
	for _, target := range linkTargets {
		linkTargetFound[target] = true
	}
	if !linkTargetFound[sharedTarget1] || !linkTargetFound[sharedTarget2] || !linkTargetFound[consumer1OnlyTarget] {
		t.Fatalf("Missing expected link targets from cleanup")
	}
	
	if monitorTargets[0] != sharedTarget1 {
		t.Fatalf("Expected sharedTarget1 in monitor targets")
	}

	// CRITICAL: Verify that consumer1's relations are gone
	if tm.HasLink(consumer1, sharedTarget1) || tm.HasLink(consumer1, sharedTarget2) || tm.HasLink(consumer1, consumer1OnlyTarget) {
		t.Fatalf("Consumer1's links should be gone")
	}
	if tm.HasMonitor(consumer1, sharedTarget1) {
		t.Fatalf("Consumer1's monitor should be gone")
	}

	// CRITICAL: Verify that OTHER consumers' relations to shared targets REMAIN
	if !tm.HasLink(consumer2, sharedTarget1) {
		t.Fatalf("Consumer2's link to sharedTarget1 should remain")
	}
	if !tm.HasLink(consumer3, sharedTarget1) {
		t.Fatalf("Consumer3's link to sharedTarget1 should remain") 
	}
	if !tm.HasLink(consumer2, sharedTarget2) {
		t.Fatalf("Consumer2's link to sharedTarget2 should remain")
	}
	if !tm.HasMonitor(consumer2, sharedTarget1) {
		t.Fatalf("Consumer2's monitor to sharedTarget1 should remain")
	}

	// Verify that we can still query relationships for remaining consumers
	links, monitors := tm.GetTargetsForConsumer(consumer2)
	if len(links) != 2 || len(monitors) != 1 {
		t.Fatalf("Consumer2 should still have 2 links and 1 monitor")
	}
	
	consumers := tm.GetConsumersForTarget(sharedTarget1)
	if len(consumers) != 3 {
		t.Fatalf("sharedTarget1 should still have 3 consumers (2 links + 1 monitor) after cleanup, got %d", len(consumers))
	}
}

func TestDefaultTargetManager_CleanupTarget(t *testing.T) {
	tm := CreateDefaultTargetManager()
	consumer1 := PID{Node: "node1", ID: 1, Creation: 1}
	consumer2 := PID{Node: "node1", ID: 2, Creation: 1}
	consumer3 := PID{Node: "node2", ID: 1, Creation: 1}
	target := PID{Node: "node3", ID: 1, Creation: 1}

	// Add some links and monitors
	tm.AddLink(consumer1, target)
	tm.AddLink(consumer2, target)
	tm.AddMonitor(consumer3, target)

	// Cleanup target
	linkConsumers, monitorConsumers := tm.CleanupTarget(target)

	// Verify returned consumers
	if len(linkConsumers) != 2 {
		t.Fatalf("Expected 2 link consumers, got %d", len(linkConsumers))
	}
	if len(monitorConsumers) != 1 {
		t.Fatalf("Expected 1 monitor consumer, got %d", len(monitorConsumers))
	}

	// Verify relations are gone
	if tm.HasLink(consumer1, target) || tm.HasLink(consumer2, target) {
		t.Fatalf("Links should be gone")
	}
	if tm.HasMonitor(consumer3, target) {
		t.Fatalf("Monitor should be gone")
	}
}

func TestDefaultTargetManager_CleanupNode(t *testing.T) {
	tm := CreateDefaultTargetManager()
	downNode := Atom("down_node")
	consumer1 := PID{Node: "local_node", ID: 1, Creation: 1}
	consumer2 := PID{Node: "local_node", ID: 2, Creation: 1}
	consumer3 := PID{Node: "other_node", ID: 1, Creation: 1}
	target1 := PID{Node: downNode, ID: 1, Creation: 1}  // Target from down node
	target2 := Alias{Node: downNode, ID: [3]uint64{1, 2, 3}, Creation: 1}  // Target from down node

	// Add relations - consumers linking to targets from down node and other nodes
	tm.AddLink(consumer1, target1)  // Should be cleaned (target from down node)
	tm.AddLink(consumer1, target2)  // Should be cleaned (target from down node)
	tm.AddMonitor(consumer2, target1)  // Should be cleaned (target from down node)
	tm.AddLink(consumer3, target1)  // Should be cleaned (target from down node)
	
	// Add relation to target NOT from down node (should remain)
	targetFromOtherNode := PID{Node: "other_target_node", ID: 1, Creation: 1}
	tm.AddLink(consumer3, targetFromOtherNode)  // Should remain

	// Cleanup node
	linkTargetsWithConsumers, monitorTargetsWithConsumers := tm.CleanupNode(downNode)

	// Verify link cleanup - targets from down node should be reported with their consumers
	if len(linkTargetsWithConsumers) != 2 {
		t.Fatalf("Expected 2 link targets, got %d", len(linkTargetsWithConsumers))
	}
	
	// target1 should have 2 consumers (consumer1 and consumer3)
	if len(linkTargetsWithConsumers[target1]) != 2 {
		t.Fatalf("Expected 2 consumers for target1, got %d", len(linkTargetsWithConsumers[target1]))
	}
	
	// target2 should have 1 consumer (consumer1)  
	if len(linkTargetsWithConsumers[target2]) != 1 || linkTargetsWithConsumers[target2][0] != consumer1 {
		t.Fatalf("Unexpected link target cleanup for target2")
	}

	// Verify monitor cleanup
	if len(monitorTargetsWithConsumers) != 1 {
		t.Fatalf("Expected 1 monitor target, got %d", len(monitorTargetsWithConsumers))
	}
	if len(monitorTargetsWithConsumers[target1]) != 1 || monitorTargetsWithConsumers[target1][0] != consumer2 {
		t.Fatalf("Unexpected monitor target cleanup")
	}

	// Verify ALL relations to targets from down node are gone
	if tm.HasLink(consumer1, target1) || tm.HasLink(consumer1, target2) {
		t.Fatalf("Links to targets from down node should be gone")
	}
	if tm.HasMonitor(consumer2, target1) {
		t.Fatalf("Monitor to target from down node should be gone")
	}
	if tm.HasLink(consumer3, target1) {
		t.Fatalf("Link to target from down node should be gone")
	}

	// Verify relations to targets from other nodes remain
	if !tm.HasLink(consumer3, targetFromOtherNode) {
		t.Fatalf("Link to target from other node should remain")
	}
}

func TestDefaultTargetManager_CleanupNode_AllTargetTypes(t *testing.T) {
	tm := CreateDefaultTargetManager()
	downNode := Atom("down_node")
	consumer1 := PID{Node: "local_node", ID: 1, Creation: 1}
	consumer2 := PID{Node: "local_node", ID: 2, Creation: 1}
	consumer3 := PID{Node: "local_node", ID: 3, Creation: 1}
	consumer4 := PID{Node: "local_node", ID: 4, Creation: 1}
	consumer5 := PID{Node: "local_node", ID: 5, Creation: 1}

	// Create targets of all types from the down node
	pidTarget := PID{Node: downNode, ID: 1, Creation: 1}
	processIdTarget := ProcessID{Node: downNode, Name: "test_process"}
	aliasTarget := Alias{Node: downNode, ID: [3]uint64{1, 2, 3}, Creation: 1}
	eventTarget := Event{Node: downNode, Name: "test_event"}
	atomTarget := downNode // Atom target that equals the down node

	// Add links to all target types
	tm.AddLink(consumer1, pidTarget)
	tm.AddLink(consumer2, processIdTarget)
	tm.AddLink(consumer3, aliasTarget)
	tm.AddLink(consumer4, eventTarget)
	tm.AddLink(consumer5, atomTarget)

	// Cleanup the down node
	linkTargetsWithConsumers, _ := tm.CleanupNode(downNode)

	// Verify all target types are reported
	if len(linkTargetsWithConsumers) != 5 {
		t.Fatalf("Expected 5 link targets, got %d", len(linkTargetsWithConsumers))
	}

	// Verify each target type has its consumer
	if len(linkTargetsWithConsumers[pidTarget]) != 1 || linkTargetsWithConsumers[pidTarget][0] != consumer1 {
		t.Fatalf("PID target cleanup failed")
	}
	if len(linkTargetsWithConsumers[processIdTarget]) != 1 || linkTargetsWithConsumers[processIdTarget][0] != consumer2 {
		t.Fatalf("ProcessID target cleanup failed")
	}
	if len(linkTargetsWithConsumers[aliasTarget]) != 1 || linkTargetsWithConsumers[aliasTarget][0] != consumer3 {
		t.Fatalf("Alias target cleanup failed")
	}
	if len(linkTargetsWithConsumers[eventTarget]) != 1 || linkTargetsWithConsumers[eventTarget][0] != consumer4 {
		t.Fatalf("Event target cleanup failed")
	}
	if len(linkTargetsWithConsumers[atomTarget]) != 1 || linkTargetsWithConsumers[atomTarget][0] != consumer5 {
		t.Fatalf("Atom target cleanup failed")
	}

	// Verify all relations are gone
	if tm.HasLink(consumer1, pidTarget) {
		t.Fatalf("PID target link should be gone")
	}
	if tm.HasLink(consumer2, processIdTarget) {
		t.Fatalf("ProcessID target link should be gone")
	}
	if tm.HasLink(consumer3, aliasTarget) {
		t.Fatalf("Alias target link should be gone")
	}
	if tm.HasLink(consumer4, eventTarget) {
		t.Fatalf("Event target link should be gone")
	}
	if tm.HasLink(consumer5, atomTarget) {
		t.Fatalf("Atom target link should be gone")
	}
}

func TestDefaultTargetManager_CleanupNode_ConsumerFromDownNode(t *testing.T) {
	tm := CreateDefaultTargetManager()
	downNode := Atom("down_node")
	consumerFromDownNode := PID{Node: downNode, ID: 1, Creation: 1}
	localConsumer := PID{Node: "local_node", ID: 1, Creation: 1}
	target1 := PID{Node: "other_node", ID: 1, Creation: 1}
	target2 := "string_target"

	// Add relations where consumer is from down node
	tm.AddLink(consumerFromDownNode, target1)
	tm.AddMonitor(consumerFromDownNode, target2)
	
	// Add relation where consumer is NOT from down node (should remain)
	tm.AddLink(localConsumer, target1)

	// Cleanup the down node
	linkTargetsWithConsumers, monitorTargetsWithConsumers := tm.CleanupNode(downNode)

	// Targets should be reported with the consumers that were removed
	// This allows the node to notify local event producers about lost remote subscribers
	if len(linkTargetsWithConsumers) != 1 {
		t.Fatalf("Expected 1 link target (target1 with consumerFromDownNode), got %d", len(linkTargetsWithConsumers))
	}
	if len(monitorTargetsWithConsumers) != 1 {
		t.Fatalf("Expected 1 monitor target (target2 with consumerFromDownNode), got %d", len(monitorTargetsWithConsumers))
	}

	// Verify target1 has the correct consumer listed
	if len(linkTargetsWithConsumers[target1]) != 1 || linkTargetsWithConsumers[target1][0] != consumerFromDownNode {
		t.Fatalf("target1 should have consumerFromDownNode in link list")
	}

	// Verify target2 has the correct consumer listed
	if len(monitorTargetsWithConsumers[target2]) != 1 || monitorTargetsWithConsumers[target2][0] != consumerFromDownNode {
		t.Fatalf("target2 should have consumerFromDownNode in monitor list")
	}

	// Verify relations from down node consumer are gone
	if tm.HasLink(consumerFromDownNode, target1) {
		t.Fatalf("Link from down node consumer should be gone")
	}
	if tm.HasMonitor(consumerFromDownNode, target2) {
		t.Fatalf("Monitor from down node consumer should be gone")
	}

	// Verify relations from local consumer remain
	if !tm.HasLink(localConsumer, target1) {
		t.Fatalf("Link from local consumer should remain")
	}
}


func TestDefaultTargetManager_GetTargetsForConsumer(t *testing.T) {
	tm := CreateDefaultTargetManager()
	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	target1 := PID{Node: "node2", ID: 1, Creation: 1}
	target2 := "string_target"
	target3 := PID{Node: "node2", ID: 3, Creation: 1}

	// Add relations
	tm.AddLink(consumer, target1)
	tm.AddLink(consumer, target2)
	tm.AddMonitor(consumer, target3)

	// Get targets
	links, monitors := tm.GetTargetsForConsumer(consumer)

	// Verify results
	if len(links) != 2 {
		t.Fatalf("Expected 2 links, got %d", len(links))
	}
	if len(monitors) != 1 {
		t.Fatalf("Expected 1 monitor, got %d", len(monitors))
	}

	// Check for specific targets
	linkFound := make(map[any]bool)
	for _, target := range links {
		linkFound[target] = true
	}
	if !linkFound[target1] || !linkFound[target2] {
		t.Fatalf("Expected link targets not found")
	}

	if monitors[0] != target3 {
		t.Fatalf("Expected monitor target not found")
	}
}

func TestDefaultTargetManager_GetConsumersForTarget(t *testing.T) {
	tm := CreateDefaultTargetManager()
	consumer1 := PID{Node: "node1", ID: 1, Creation: 1}
	consumer2 := PID{Node: "node1", ID: 2, Creation: 1}
	consumer3 := PID{Node: "node2", ID: 1, Creation: 1}
	target := "shared_target"

	// Add relations
	tm.AddLink(consumer1, target)
	tm.AddLink(consumer2, target)
	tm.AddMonitor(consumer3, target)

	// Get consumers
	consumers := tm.GetConsumersForTarget(target)

	// Verify results
	if len(consumers) != 3 {
		t.Fatalf("Expected 3 consumers (2 links + 1 monitor), got %d", len(consumers))
	}

	// Check for specific consumers
	consumerSet := make(map[PID]bool)
	for _, consumer := range consumers {
		consumerSet[consumer] = true
	}
	if !consumerSet[consumer1] || !consumerSet[consumer2] || !consumerSet[consumer3] {
		t.Fatal("Expected consumers not found")
	}
}

func TestDefaultTargetManager_ConcurrentAccess(t *testing.T) {
	tm := CreateDefaultTargetManager()
	const numGoroutines = 10
	const numOpsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // readers and writers

	// Writers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			consumer := PID{Node: "node", ID: uint64(id), Creation: 1}
			for j := 0; j < numOpsPerGoroutine; j++ {
				target := PID{Node: "target_node", ID: uint64(j), Creation: 1}
				tm.AddLink(consumer, target)
				tm.AddMonitor(consumer, target)
				tm.RemoveLink(consumer, target)
				tm.RemoveMonitor(consumer, target)
			}
		}(i)
	}

	// Readers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			consumer := PID{Node: "node", ID: uint64(id), Creation: 1}
			for j := 0; j < numOpsPerGoroutine; j++ {
				target := PID{Node: "target_node", ID: uint64(j), Creation: 1}
				tm.HasLink(consumer, target)
				tm.HasMonitor(consumer, target)
				tm.GetTargetsForConsumer(consumer)
				tm.GetConsumersForTarget(target)
			}
		}(i)
	}

	wg.Wait()
}

func TestDefaultTargetManager_ErrorHandling(t *testing.T) {
	tm := CreateDefaultTargetManager()
	consumer := PID{Node: "node1", ID: 1, Creation: 1}
	target := PID{Node: "node2", ID: 1, Creation: 1}

	// Test error cases
	if err := tm.RemoveLink(consumer, target); err != ErrTargetUnknown {
		t.Fatalf("Expected ErrTargetUnknown for non-existent link")
	}

	if err := tm.RemoveMonitor(consumer, target); err != ErrTargetUnknown {
		t.Fatalf("Expected ErrTargetUnknown for non-existent monitor")
	}

	// Add and verify duplicate errors
	tm.AddLink(consumer, target)
	tm.AddMonitor(consumer, target)

	if err := tm.AddLink(consumer, target); err != ErrTargetExist {
		t.Fatalf("Expected ErrTargetExist for duplicate link")
	}

	if err := tm.AddMonitor(consumer, target); err != ErrTargetExist {
		t.Fatalf("Expected ErrTargetExist for duplicate monitor")
	}
}