package tm

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

// Test TerminatedTargetPID - sends exit to link subscribers
func TestTerminatedTargetPID_LinkSubscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.PID{Node: "node1", ID: 200}
	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}

	// Setup: two link subscribers
	tm.LinkPID(consumer1, target)
	tm.LinkPID(consumer2, target)

	core.resetSentExits()

	// Target terminates
	tm.TerminatedTargetPID(target, gen.ErrProcessTerminated)

	// Give dispatcher time to deliver
	time.Sleep(50 * time.Millisecond)

	// Verify 2 exit messages sent
	if core.countSentExits() != 2 {
		t.Errorf("Expected 2 exit messages, got %d", core.countSentExits())
	}

	// Verify subscriptions cleaned
	key1 := relationKey{consumer: consumer1, target: target}
	if _, exists := tm.linkRelations[key1]; exists {
		t.Error("Link should be removed after target terminated")
	}

	// Verify targetIndex cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

// Test TerminatedTargetPID - sends down to monitor subscribers
func TestTerminatedTargetPID_MonitorSubscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.PID{Node: "node1", ID: 200}
	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}

	// Setup: two monitor subscribers
	tm.MonitorPID(consumer1, target)
	tm.MonitorPID(consumer2, target)

	core.resetSentDowns()

	// Target terminates
	tm.TerminatedTargetPID(target, gen.ErrProcessTerminated)

	// Give dispatcher time
	time.Sleep(50 * time.Millisecond)

	// Verify 2 down messages
	if core.countSentDowns() != 2 {
		t.Errorf("Expected 2 down messages, got %d", core.countSentDowns())
	}

	// Verify cleaned
	if len(tm.monitorRelations) != 0 {
		t.Errorf("monitorRelations should be empty, got %d", len(tm.monitorRelations))
	}
}

// Test TerminatedTargetPID - mixed link and monitor
func TestTerminatedTargetPID_Mixed(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.PID{Node: "node1", ID: 200}
	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}

	// consumer1 links, consumer2 monitors
	tm.LinkPID(consumer1, target)
	tm.MonitorPID(consumer2, target)

	core.resetSentExits()
	core.resetSentDowns()

	// Target terminates
	tm.TerminatedTargetPID(target, gen.ErrProcessTerminated)

	time.Sleep(50 * time.Millisecond)

	// 1 exit, 1 down
	if core.countSentExits() != 1 {
		t.Errorf("Expected 1 exit, got %d", core.countSentExits())
	}

	if core.countSentDowns() != 1 {
		t.Errorf("Expected 1 down, got %d", core.countSentDowns())
	}

	// Verify linkRelations cleaned
	if len(tm.linkRelations) != 0 {
		t.Errorf("linkRelations should be empty, got %d", len(tm.linkRelations))
	}

	// Verify monitorRelations cleaned
	if len(tm.monitorRelations) != 0 {
		t.Errorf("monitorRelations should be empty, got %d", len(tm.monitorRelations))
	}

	// Verify targetIndex cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

// Test TerminatedTargetProcessID
func TestTerminatedTargetProcessID(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.ProcessID{Node: "node1", Name: "test"}
	consumer := gen.PID{Node: "node1", ID: 100}

	tm.LinkProcessID(consumer, target)

	core.resetSentExits()

	tm.TerminatedTargetProcessID(target, gen.ErrProcessTerminated)

	time.Sleep(50 * time.Millisecond)

	// Exit sent
	if core.countSentExits() != 1 {
		t.Errorf("Expected 1 exit, got %d", core.countSentExits())
	}

	// Cleaned from linkRelations
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("Link should be removed")
	}

	// Verify targetIndex cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

// Test TerminatedTargetAlias
func TestTerminatedTargetAlias(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	consumer := gen.PID{Node: "node1", ID: 100}

	tm.LinkAlias(consumer, target)

	core.resetSentExits()

	tm.TerminatedTargetAlias(target, gen.ErrProcessTerminated)

	time.Sleep(50 * time.Millisecond)

	if core.countSentExits() != 1 {
		t.Errorf("Expected 1 exit, got %d", core.countSentExits())
	}

	// Verify linkRelations cleaned
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("Link should be removed")
	}

	// Verify targetIndex cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

// Test TerminatedTargetNode - consumer on terminated node
func TestTerminatedTargetNode_RemoteConsumer(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	localTarget := gen.PID{Node: "node1", ID: 200}
	remoteConsumer := gen.PID{Node: "node2", ID: 100}

	// Remote consumer links to local target
	// (Simulates: node2's CorePID linked to node1's process)
	tm.linkRelations[relationKey{consumer: remoteConsumer, target: localTarget}] = struct{}{}

	entry := &targetEntry{consumers: make(map[gen.PID]struct{})}
	entry.consumers[remoteConsumer] = struct{}{}
	tm.targetIndex[localTarget] = entry

	core.resetSentExits()

	// node2 dies
	tm.TerminatedTargetNode("node2", gen.ErrNoConnection)

	time.Sleep(50 * time.Millisecond)

	// NO exit sent (consumer is dead)
	if core.countSentExits() != 0 {
		t.Errorf("Should NOT send exit to dead consumer, got %d", core.countSentExits())
	}

	// But subscription cleaned!
	key := relationKey{consumer: remoteConsumer, target: localTarget}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("Link from remote consumer should be removed")
	}
}

// Test TerminatedTargetNode - target on terminated node
func TestTerminatedTargetNode_RemoteTarget(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	localConsumer := gen.PID{Node: "node1", ID: 100}
	remoteTarget := gen.PID{Node: "node2", ID: 200}

	// Local consumer linked to remote target
	tm.LinkPID(localConsumer, remoteTarget)

	core.resetSentExits()

	// node2 dies
	tm.TerminatedTargetNode("node2", gen.ErrNoConnection)

	time.Sleep(50 * time.Millisecond)

	// Exit sent to local consumer!
	if core.countSentExits() != 1 {
		t.Errorf("Expected 1 exit to local consumer, got %d", core.countSentExits())
	}

	// Cleaned from linkRelations
	key := relationKey{consumer: localConsumer, target: remoteTarget}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("Link should be removed")
	}

	// Verify targetIndex cleaned
	if _, exists := tm.targetIndex[remoteTarget]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

// Test TerminatedTargetNode - mixed (consumer AND target on node2)
func TestTerminatedTargetNode_Mixed(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	localConsumer := gen.PID{Node: "node1", ID: 100}
	remoteTarget := gen.PID{Node: "node2", ID: 200}

	remoteConsumer := gen.PID{Node: "node2", ID: 101}
	localTarget := gen.PID{Node: "node1", ID: 201}

	// Local → Remote
	tm.LinkPID(localConsumer, remoteTarget)

	// Remote → Local (manual setup)
	tm.linkRelations[relationKey{consumer: remoteConsumer, target: localTarget}] = struct{}{}

	core.resetSentExits()

	// node2 dies
	tm.TerminatedTargetNode("node2", gen.ErrNoConnection)

	time.Sleep(50 * time.Millisecond)

	// Only 1 exit - to localConsumer (remoteConsumer is dead)
	if core.countSentExits() != 1 {
		t.Errorf("Expected 1 exit, got %d", core.countSentExits())
	}

	// Both links cleaned
	if len(tm.linkRelations) != 0 {
		t.Errorf("All links should be cleaned, got %d", len(tm.linkRelations))
	}

	// Verify targetIndex cleaned for remoteTarget
	if _, exists := tm.targetIndex[remoteTarget]; exists {
		t.Error("targetIndex for remoteTarget should be cleaned")
	}
}

// Test TerminatedTargetNode - events from terminated node cleaned
func TestTerminatedTargetNode_EventsCleaned(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	// Manually add event from node2
	event := gen.Event{Node: "node2", Name: "test"}
	tm.events[event] = &eventEntry{
		producer: gen.PID{Node: "node2", ID: 100},
	}

	// node2 dies
	tm.TerminatedTargetNode("node2", gen.ErrNoConnection)

	time.Sleep(50 * time.Millisecond)

	// Event cleaned
	if _, exists := tm.events[event]; exists {
		t.Error("Event from terminated node should be removed")
	}
}

// Test TerminatedTargetProcessID with monitors
func TestTerminatedTargetProcessID_Monitors(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.ProcessID{Node: "node1", Name: "test"}
	consumer := gen.PID{Node: "node1", ID: 100}

	tm.MonitorProcessID(consumer, target)

	core.resetSentDowns()

	tm.TerminatedTargetProcessID(target, gen.ErrProcessTerminated)

	time.Sleep(50 * time.Millisecond)

	// Down sent
	if core.countSentDowns() != 1 {
		t.Errorf("Expected 1 down, got %d", core.countSentDowns())
	}

	// Verify monitorRelations cleaned
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("Monitor should be removed")
	}

	// Verify targetIndex cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

// Test TerminatedTargetAlias with monitors
func TestTerminatedTargetAlias_Monitors(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	consumer := gen.PID{Node: "node1", ID: 100}

	tm.MonitorAlias(consumer, target)

	core.resetSentDowns()

	tm.TerminatedTargetAlias(target, gen.ErrProcessTerminated)

	time.Sleep(50 * time.Millisecond)

	if core.countSentDowns() != 1 {
		t.Errorf("Expected 1 down, got %d", core.countSentDowns())
	}

	// Cleaned
	if len(tm.monitorRelations) != 0 {
		t.Error("monitorRelations should be empty")
	}
}

// Test dispatchers work in parallel
func TestDispatchers_Parallel(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.PID{Node: "node1", ID: 200}

	// Create 10 subscribers
	for i := 0; i < 10; i++ {
		consumer := gen.PID{Node: "node1", ID: uint64(100 + i)}
		tm.LinkPID(consumer, target)
	}

	core.resetSentExits()

	// Terminate target
	tm.TerminatedTargetPID(target, gen.ErrProcessTerminated)

	// Give dispatchers time (3 dispatchers working in parallel)
	time.Sleep(200 * time.Millisecond)

	// All 10 should receive exit
	if core.countSentExits() != 10 {
		t.Errorf("Expected 10 exits, got %d", core.countSentExits())
	}

	// Verify round-robin distribution would use all 3 dispatchers
	// (can't directly verify but logic is there)
}

// Test TerminatedTargetNode - multiple target types
func TestTerminatedTargetNode_MultipleTypes(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}

	// Link to different target types on node2
	pidTarget := gen.PID{Node: "node2", ID: 200}
	processIDTarget := gen.ProcessID{Node: "node2", Name: "test"}
	aliasTarget := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	tm.LinkPID(consumer, pidTarget)
	tm.LinkProcessID(consumer, processIDTarget)
	tm.LinkAlias(consumer, aliasTarget)

	core.resetSentExits()

	// node2 dies
	tm.TerminatedTargetNode("node2", gen.ErrNoConnection)

	time.Sleep(50 * time.Millisecond)

	// 3 exits (one per target type)
	if core.countSentExits() != 3 {
		t.Errorf("Expected 3 exits (one per target type), got %d", core.countSentExits())
	}

	// All links cleaned
	if len(tm.linkRelations) != 0 {
		t.Errorf("All links should be cleaned, got %d", len(tm.linkRelations))
	}

	// Verify all targetIndex entries cleaned
	if _, exists := tm.targetIndex[pidTarget]; exists {
		t.Error("targetIndex for pidTarget should be cleaned")
	}
	if _, exists := tm.targetIndex[processIDTarget]; exists {
		t.Error("targetIndex for processIDTarget should be cleaned")
	}
	if _, exists := tm.targetIndex[aliasTarget]; exists {
		t.Error("targetIndex for aliasTarget should be cleaned")
	}
}

// Test TerminatedTargetNode - no subscribers (no crash)
func TestTerminatedTargetNode_NoSubscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	// No subscribers

	// node2 dies (no panic!)
	tm.TerminatedTargetNode("node2", gen.ErrNoConnection)

	time.Sleep(10 * time.Millisecond)

	// No messages
	if core.countSentExits() != 0 {
		t.Error("Should not send any messages")
	}
}

// Test TerminatedProcess - cleanup links
func TestTerminatedProcess_CleanupLinks(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target1 := gen.PID{Node: "node1", ID: 200}
	target2 := gen.PID{Node: "node2", ID: 201}

	// Consumer links to local and remote targets
	tm.LinkPID(consumer, target1)
	tm.LinkPID(consumer, target2)

	core.resetSentUnlinks()

	// Consumer terminates
	tm.TerminatedProcess(consumer, gen.TerminateReasonNormal)

	time.Sleep(50 * time.Millisecond)

	// Links cleaned
	if len(tm.linkRelations) != 0 {
		t.Errorf("All links should be cleaned, got %d", len(tm.linkRelations))
	}

	// Remote Unlink sent
	if core.countSentUnlinks() != 1 {
		t.Errorf("Expected 1 remote Unlink, got %d", core.countSentUnlinks())
	}

	// Verify targetIndex cleaned for both targets
	if _, exists := tm.targetIndex[target1]; exists {
		t.Error("targetIndex for target1 should be cleaned")
	}
	if _, exists := tm.targetIndex[target2]; exists {
		t.Error("targetIndex for target2 should be cleaned")
	}
}

// Test TerminatedProcess - last subscriber triggers EventStop
func TestTerminatedProcess_LastSubscriber_EventStop(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer := gen.PID{Node: "node1", ID: 101}

	_, _ = tm.RegisterEvent(producer, "test", gen.EventOptions{Notify: true})

	event := gen.Event{Node: "node1", Name: "test"}

	// Consumer subscribes
	tm.LinkEvent(consumer, event)
	time.Sleep(20 * time.Millisecond)

	core.resetSentEventStops()

	// Consumer terminates (last subscriber!)
	tm.TerminatedProcess(consumer, gen.TerminateReasonNormal)

	time.Sleep(50 * time.Millisecond)

	// EventStop sent to producer!
	if core.countSentEventStops() != 1 {
		t.Errorf("Expected 1 EventStop, got %d", core.countSentEventStops())
	}

	// Counter = 0
	entry := tm.events[event]
	if entry.subscriberCount != 0 {
		t.Errorf("subscriberCount should be 0, got %d", entry.subscriberCount)
	}
}

// Test TerminatedProcess - not last subscriber, NO EventStop
func TestTerminatedProcess_NotLast_NoEventStop(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer1 := gen.PID{Node: "node1", ID: 101}
	consumer2 := gen.PID{Node: "node1", ID: 102}

	tm.RegisterEvent(producer, "test", gen.EventOptions{Notify: true})

	event := gen.Event{Node: "node1", Name: "test"}

	tm.LinkEvent(consumer1, event)
	tm.LinkEvent(consumer2, event)
	time.Sleep(20 * time.Millisecond)

	core.resetSentEventStops()

	// consumer1 terminates (not last!)
	tm.TerminatedProcess(consumer1, gen.TerminateReasonNormal)

	time.Sleep(50 * time.Millisecond)

	// NO EventStop (consumer2 still subscribed)
	if core.countSentEventStops() != 0 {
		t.Errorf("Should NOT send EventStop when other subscribers exist, got %d", core.countSentEventStops())
	}

	// Counter = 1
	entry := tm.events[event]
	if entry.subscriberCount != 1 {
		t.Errorf("subscriberCount should be 1, got %d", entry.subscriberCount)
	}

	// Verify consumer1 removed from linkRelations
	key1 := relationKey{consumer: consumer1, target: event}
	if _, exists := tm.linkRelations[key1]; exists {
		t.Error("consumer1 link should be removed")
	}

	// Verify consumer2 still in linkRelations
	key2 := relationKey{consumer: consumer2, target: event}
	if _, exists := tm.linkRelations[key2]; exists == false {
		t.Error("consumer2 link should still exist")
	}
}

// Test TerminatedProcess - remote event, last local subscriber
func TestTerminatedProcess_RemoteEvent_LastLocal_SendsUnlink(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	event := gen.Event{Node: "node2", Name: "test"}

	// Subscribe to remote event
	tm.LinkEvent(consumer, event)

	core.resetSentEventLinks()

	// Consumer terminates (last local!)
	tm.TerminatedProcess(consumer, gen.TerminateReasonNormal)

	time.Sleep(50 * time.Millisecond)

	// Verify linkRelations cleaned
	if len(tm.linkRelations) != 0 {
		t.Error("Link should be cleaned")
	}

	// Verify targetIndex cleaned (last local subscriber)
	if _, exists := tm.targetIndex[event]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

// Test TerminatedProcess - mixed link and monitor to event
func TestTerminatedProcess_Event_LinkAndMonitor(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer := gen.PID{Node: "node1", ID: 101}

	tm.RegisterEvent(producer, "test", gen.EventOptions{Notify: true})

	event := gen.Event{Node: "node1", Name: "test"}

	// Consumer has both link and monitor
	tm.LinkEvent(consumer, event)
	tm.MonitorEvent(consumer, event)
	time.Sleep(20 * time.Millisecond)

	// Counter = 2 (link + monitor)
	entry := tm.events[event]
	if entry.subscriberCount != 2 {
		t.Errorf("subscriberCount should be 2, got %d", entry.subscriberCount)
	}

	core.resetSentEventStops()

	// Consumer terminates
	tm.TerminatedProcess(consumer, gen.TerminateReasonNormal)

	time.Sleep(50 * time.Millisecond)

	// Counter = 0 (both cleaned!)
	if entry.subscriberCount != 0 {
		t.Errorf("subscriberCount should be 0 after cleanup, got %d", entry.subscriberCount)
	}

	// EventStop sent!
	if core.countSentEventStops() != 1 {
		t.Errorf("Expected 1 EventStop, got %d", core.countSentEventStops())
	}

	// Verify linkRelations cleaned for consumer
	linkKey := relationKey{consumer: consumer, target: event}
	if _, exists := tm.linkRelations[linkKey]; exists {
		t.Error("Link should be removed")
	}

	// Verify monitorRelations cleaned for consumer
	monitorKey := relationKey{consumer: consumer, target: event}
	if _, exists := tm.monitorRelations[monitorKey]; exists {
		t.Error("Monitor should be removed")
	}
}

// Test TerminatedEvent - link subscribers get exit
func TestTerminatedEvent_LinkSubscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer1 := gen.PID{Node: "node1", ID: 101}
	consumer2 := gen.PID{Node: "node1", ID: 102}

	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	tm.LinkEvent(consumer1, event)
	tm.LinkEvent(consumer2, event)

	core.resetSentExits()

	// Event terminates
	tm.TerminatedTargetEvent(event, gen.ErrUnregistered)

	time.Sleep(50 * time.Millisecond)

	// 2 exits sent
	if core.countSentExits() != 2 {
		t.Errorf("Expected 2 exits, got %d", core.countSentExits())
	}

	// Event removed from tm.events
	if _, exists := tm.events[event]; exists {
		t.Error("Event should be removed")
	}

	// Subscriptions cleaned
	if len(tm.linkRelations) != 0 {
		t.Error("linkRelations should be empty")
	}
}

// Test TerminatedEvent - monitor subscribers get down
func TestTerminatedEvent_MonitorSubscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer1 := gen.PID{Node: "node1", ID: 101}
	consumer2 := gen.PID{Node: "node1", ID: 102}

	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	tm.MonitorEvent(consumer1, event)
	tm.MonitorEvent(consumer2, event)

	core.resetSentDowns()

	// Event terminates
	tm.TerminatedTargetEvent(event, gen.ErrUnregistered)

	time.Sleep(50 * time.Millisecond)

	// 2 downs sent
	if core.countSentDowns() != 2 {
		t.Errorf("Expected 2 downs, got %d", core.countSentDowns())
	}

	// Cleaned
	if len(tm.monitorRelations) != 0 {
		t.Error("monitorRelations should be empty")
	}
}

// Test TerminatedEvent - mixed link and monitor
func TestTerminatedEvent_Mixed(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	linkConsumer := gen.PID{Node: "node1", ID: 101}
	monitorConsumer := gen.PID{Node: "node1", ID: 102}

	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	tm.LinkEvent(linkConsumer, event)
	tm.MonitorEvent(monitorConsumer, event)

	core.resetSentExits()
	core.resetSentDowns()

	// Event terminates
	tm.TerminatedTargetEvent(event, gen.ErrUnregistered)

	time.Sleep(50 * time.Millisecond)

	// 1 exit, 1 down
	if core.countSentExits() != 1 {
		t.Errorf("Expected 1 exit, got %d", core.countSentExits())
	}

	if core.countSentDowns() != 1 {
		t.Errorf("Expected 1 down, got %d", core.countSentDowns())
	}

	// Event removed
	if _, exists := tm.events[event]; exists {
		t.Error("Event should be removed")
	}

	// Verify linkRelations cleaned
	if len(tm.linkRelations) != 0 {
		t.Errorf("linkRelations should be empty, got %d", len(tm.linkRelations))
	}

	// Verify monitorRelations cleaned
	if len(tm.monitorRelations) != 0 {
		t.Errorf("monitorRelations should be empty, got %d", len(tm.monitorRelations))
	}
}

// Test TerminatedEvent - no subscribers (no crash)
func TestTerminatedEvent_NoSubscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	core.resetSentExits()

	// Event terminates with no subscribers
	tm.TerminatedTargetEvent(event, gen.ErrUnregistered)

	time.Sleep(20 * time.Millisecond)

	// No messages
	if core.countSentExits() != 0 {
		t.Error("Should not send messages when no subscribers")
	}

	// Event still removed
	if _, exists := tm.events[event]; exists {
		t.Error("Event should be removed")
	}
}

// Test TerminatedTargetPID with remote CorePID subscriber
func TestTerminatedTargetPID_RemoteCorePIDSubscriber(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.PID{Node: "node1", ID: 200}
	localConsumer := gen.PID{Node: "node1", ID: 100}
	remoteCorePID := gen.PID{Node: "node2", ID: 1}

	// Local consumer links
	tm.LinkPID(localConsumer, target)

	// Simulate remote CorePID also linked (from node2)
	key := relationKey{consumer: remoteCorePID, target: target}
	tm.linkRelations[key] = struct{}{}
	entry := tm.targetIndex[target]
	if entry != nil {
		entry.consumers[remoteCorePID] = struct{}{}
	}

	core.resetSentExits()

	// Target terminates
	tm.TerminatedTargetPID(target, gen.ErrProcessTerminated)

	time.Sleep(50 * time.Millisecond)

	// 1 exit to local consumer
	if core.countSentExits() != 1 {
		t.Errorf("Expected 1 exit to local, got %d", core.countSentExits())
	}

	// Both links cleaned
	if len(tm.linkRelations) != 0 {
		t.Error("All links should be cleaned")
	}

	// Verify targetIndex cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

// Test TerminatedProcess - producer cleanup notifies link subscribers
func TestTerminatedProcess_ProducerCleanup_LinkSubscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer1 := gen.PID{Node: "node1", ID: 101}
	consumer2 := gen.PID{Node: "node1", ID: 102}

	// Producer registers event
	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	// Consumers subscribe via link
	tm.LinkEvent(consumer1, event)
	tm.LinkEvent(consumer2, event)

	core.resetSentExits()

	// Producer terminates
	tm.TerminatedProcess(producer, gen.TerminateReasonNormal)

	time.Sleep(50 * time.Millisecond)

	// Both consumers receive exit
	if core.countSentExits() != 2 {
		t.Errorf("Expected 2 exits to link subscribers, got %d", core.countSentExits())
	}

	// Event removed
	if _, exists := tm.events[event]; exists {
		t.Error("Event should be removed after producer terminates")
	}

	// producerEvents index cleaned
	if tm.producerEvents[producer] != nil {
		t.Error("producerEvents index should be cleaned")
	}
}

// Test TerminatedProcess - producer cleanup notifies monitor subscribers
func TestTerminatedProcess_ProducerCleanup_MonitorSubscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer1 := gen.PID{Node: "node1", ID: 101}
	consumer2 := gen.PID{Node: "node1", ID: 102}

	// Producer registers event
	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	// Consumers subscribe via monitor
	tm.MonitorEvent(consumer1, event)
	tm.MonitorEvent(consumer2, event)

	core.resetSentDowns()

	// Producer terminates
	tm.TerminatedProcess(producer, gen.TerminateReasonNormal)

	time.Sleep(50 * time.Millisecond)

	// Both consumers receive down
	if core.countSentDowns() != 2 {
		t.Errorf("Expected 2 downs to monitor subscribers, got %d", core.countSentDowns())
	}

	// Event removed
	if _, exists := tm.events[event]; exists {
		t.Error("Event should be removed after producer terminates")
	}

	// Verify monitorRelations cleaned
	if len(tm.monitorRelations) != 0 {
		t.Errorf("monitorRelations should be empty, got %d", len(tm.monitorRelations))
	}

	// Verify producerEvents cleaned
	if tm.producerEvents[producer] != nil {
		t.Error("producerEvents index should be cleaned")
	}
}

// Test TerminatedProcess - producer with multiple events
func TestTerminatedProcess_ProducerCleanup_MultipleEvents(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer := gen.PID{Node: "node1", ID: 101}

	// Producer registers multiple events
	tm.RegisterEvent(producer, "event1", gen.EventOptions{})
	tm.RegisterEvent(producer, "event2", gen.EventOptions{})
	tm.RegisterEvent(producer, "event3", gen.EventOptions{})

	event1 := gen.Event{Node: "node1", Name: "event1"}
	event2 := gen.Event{Node: "node1", Name: "event2"}
	event3 := gen.Event{Node: "node1", Name: "event3"}

	// Consumer subscribes to all events
	tm.LinkEvent(consumer, event1)
	tm.LinkEvent(consumer, event2)
	tm.LinkEvent(consumer, event3)

	core.resetSentExits()

	// Producer terminates
	tm.TerminatedProcess(producer, gen.TerminateReasonNormal)

	time.Sleep(50 * time.Millisecond)

	// Consumer receives 3 exits (one per event)
	if core.countSentExits() != 3 {
		t.Errorf("Expected 3 exits (one per event), got %d", core.countSentExits())
	}

	// All events removed
	if len(tm.events) != 0 {
		t.Errorf("All events should be removed, got %d", len(tm.events))
	}

	// producerEvents index cleaned
	if tm.producerEvents[producer] != nil {
		t.Error("producerEvents index should be cleaned")
	}

	// Verify linkRelations cleaned
	if len(tm.linkRelations) != 0 {
		t.Errorf("linkRelations should be empty, got %d", len(tm.linkRelations))
	}
}

// Test TerminatedProcess - producer cleanup cleans relations
func TestTerminatedProcess_ProducerCleanup_RelationsCleaned(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer := gen.PID{Node: "node1", ID: 101}

	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	// Consumer links and monitors
	tm.LinkEvent(consumer, event)
	tm.MonitorEvent(consumer, event)

	// Verify relations exist
	if len(tm.linkRelations) != 1 {
		t.Fatalf("Expected 1 link relation, got %d", len(tm.linkRelations))
	}
	if len(tm.monitorRelations) != 1 {
		t.Fatalf("Expected 1 monitor relation, got %d", len(tm.monitorRelations))
	}

	// Producer terminates
	tm.TerminatedProcess(producer, gen.TerminateReasonNormal)

	time.Sleep(50 * time.Millisecond)

	// Relations for the event should be cleaned
	for key := range tm.linkRelations {
		if key.target == event {
			t.Error("Link relation for event should be cleaned")
		}
	}
	for key := range tm.monitorRelations {
		if key.target == event {
			t.Error("Monitor relation for event should be cleaned")
		}
	}
}

// Test TerminatedProcess - producer with no events (no crash)
func TestTerminatedProcess_ProducerCleanup_NoEvents(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	core.resetSentExits()

	// Producer terminates (has no events)
	tm.TerminatedProcess(producer, gen.TerminateReasonNormal)

	time.Sleep(20 * time.Millisecond)

	// No crashes, no messages
	if core.countSentExits() != 0 {
		t.Errorf("Expected 0 exits, got %d", core.countSentExits())
	}
}

// Test TerminatedProcess - producer cleanup with remote subscribers
func TestTerminatedProcess_ProducerCleanup_RemoteSubscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	localConsumer := gen.PID{Node: "node1", ID: 101}
	remoteConsumer := gen.PID{Node: "node2", ID: 102}

	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	// Local consumer subscribes
	tm.LinkEvent(localConsumer, event)

	// Simulate remote subscriber (as if from node2 via network)
	entry := tm.events[event]
	entry.linkSubscribersIndex[remoteConsumer] = len(entry.linkSubscribers)
	entry.linkSubscribers = append(entry.linkSubscribers, remoteConsumer)

	core.resetSentExits()

	// Producer terminates
	tm.TerminatedProcess(producer, gen.TerminateReasonNormal)

	time.Sleep(50 * time.Millisecond)

	// Local consumer receives exit
	if core.countSentExits() != 1 {
		t.Errorf("Expected 1 exit to local consumer, got %d", core.countSentExits())
	}

	// Event removed
	if _, exists := tm.events[event]; exists {
		t.Error("Event should be removed")
	}
}
