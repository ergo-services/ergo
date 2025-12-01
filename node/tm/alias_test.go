package tm

import (
	"testing"

	"ergo.services/ergo/gen"
)

// LinkAlias tests

func TestLinkAlias_Local(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	err := tm.LinkAlias(consumer, target)
	if err != nil {
		t.Fatalf("LinkAlias failed: %v", err)
	}

	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists == false {
		t.Error("Link should be stored")
	}

	if core.countSentLinks() != 0 {
		t.Errorf("Expected 0 network requests for local, got %d", core.countSentLinks())
	}
}

func TestLinkAlias_Remote_First(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	err := tm.LinkAlias(consumer, target)
	if err != nil {
		t.Fatalf("LinkAlias failed: %v", err)
	}

	// Verify link stored in linkRelations
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists == false {
		t.Error("Link should be stored in linkRelations")
	}

	// Verify targetIndex created
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex entry should be created")
	}
	if _, exists := entry.consumers[consumer]; exists == false {
		t.Error("Consumer should be in targetIndex.consumers")
	}

	// Verify ONE network request
	if core.countSentLinks() != 1 {
		t.Fatalf("Expected 1 network request, got %d", core.countSentLinks())
	}

	// Verify CorePID used
	if sent, ok := core.getFirstSentLink(); ok {
		if sent.from != core.pid {
			t.Error("LinkAlias should use CorePID")
		}
	}
}

func TestLinkAlias_Remote_Second(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	tm.LinkAlias(consumer1, target)
	core.resetSentLinks()

	err := tm.LinkAlias(consumer2, target)
	if err != nil {
		t.Fatalf("Second LinkAlias failed: %v", err)
	}

	// Verify NO network for second subscriber (CorePID optimization)
	if core.countSentLinks() != 0 {
		t.Errorf("Second subscriber should NOT send network, got %d", core.countSentLinks())
	}

	// Verify both links stored in linkRelations
	if len(tm.linkRelations) != 2 {
		t.Errorf("Expected 2 link relations, got %d", len(tm.linkRelations))
	}
	key1 := relationKey{consumer: consumer1, target: target}
	if _, exists := tm.linkRelations[key1]; exists == false {
		t.Error("consumer1 link should exist in linkRelations")
	}
	key2 := relationKey{consumer: consumer2, target: target}
	if _, exists := tm.linkRelations[key2]; exists == false {
		t.Error("consumer2 link should exist in linkRelations")
	}

	// Verify targetIndex has both consumers
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex entry should exist")
	}
	if len(entry.consumers) != 2 {
		t.Errorf("Expected 2 consumers in targetIndex, got %d", len(entry.consumers))
	}
}

func TestLinkAlias_Duplicate_Error(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	tm.LinkAlias(consumer, target)

	err := tm.LinkAlias(consumer, target)
	if err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist, got %v", err)
	}
}

func TestLinkAlias_NetworkError_Rollback(t *testing.T) {
	core := newMockCore("node1")
	core.connectionError = gen.ErrNoConnection

	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	err := tm.LinkAlias(consumer, target)

	if err != gen.ErrNoConnection {
		t.Errorf("Expected ErrNoConnection, got %v", err)
	}

	// Verify link rolled back from linkRelations
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("Link should be rolled back from linkRelations")
	}

	// Verify targetIndex cleaned after rollback
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned after rollback")
	}
}

func TestLinkAlias_RemoteCorePID_Duplicate_Ignored(t *testing.T) {
	core := newMockCore("node2")
	tm := Create(core, Options{}).(*targetManager)

	localTarget := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}
	remoteCorePID := gen.PID{Node: "node1", ID: 1}

	err := tm.LinkAlias(remoteCorePID, localTarget)
	if err != nil {
		t.Fatalf("First link failed: %v", err)
	}

	err = tm.LinkAlias(remoteCorePID, localTarget)
	if err != nil {
		t.Errorf("Duplicate from remote CorePID should be ignored, got: %v", err)
	}

	// Verify only ONE relation exists (duplicate was ignored)
	if len(tm.linkRelations) != 1 {
		t.Errorf("Expected 1 link relation, got %d", len(tm.linkRelations))
	}

	// Verify targetIndex has only one consumer
	entry := tm.targetIndex[localTarget]
	if entry == nil {
		t.Fatal("targetIndex entry should exist")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("Expected 1 consumer in targetIndex, got %d", len(entry.consumers))
	}
}

// UnlinkAlias tests

func TestUnlinkAlias_Local(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	tm.LinkAlias(consumer, target)

	err := tm.UnlinkAlias(consumer, target)
	if err != nil {
		t.Fatalf("UnlinkAlias failed: %v", err)
	}

	// Verify link removed from linkRelations
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("Link should be removed from linkRelations")
	}

	// Verify targetIndex cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

func TestUnlinkAlias_NotLast(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	tm.LinkAlias(consumer1, target)
	tm.LinkAlias(consumer2, target)

	core.resetSentUnlinks()

	tm.UnlinkAlias(consumer1, target)

	// Verify consumer1 link removed from linkRelations
	key1 := relationKey{consumer: consumer1, target: target}
	if _, exists := tm.linkRelations[key1]; exists {
		t.Error("consumer1 link should be removed from linkRelations")
	}

	// Verify consumer2 link still exists
	key2 := relationKey{consumer: consumer2, target: target}
	if _, exists := tm.linkRelations[key2]; exists == false {
		t.Error("consumer2 link should still exist in linkRelations")
	}

	// Verify targetIndex still exists with only consumer2
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex entry should still exist")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("Expected 1 consumer in targetIndex, got %d", len(entry.consumers))
	}
	if _, exists := entry.consumers[consumer2]; exists == false {
		t.Error("consumer2 should still be in targetIndex.consumers")
	}

	// Verify NO network unlink sent
	if core.countSentUnlinks() != 0 {
		t.Errorf("NO UnlinkAlias should be sent, got %d", core.countSentUnlinks())
	}
}

func TestUnlinkAlias_Last(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	tm.LinkAlias(consumer, target)
	core.resetSentUnlinks()

	err := tm.UnlinkAlias(consumer, target)
	if err != nil {
		t.Fatalf("UnlinkAlias failed: %v", err)
	}

	// Verify link removed from linkRelations
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("Link should be removed from linkRelations")
	}

	// Verify targetIndex cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned when last consumer removed")
	}

	// Verify network unlink sent
	if core.countSentUnlinks() != 1 {
		t.Errorf("Expected 1 UnlinkAlias, got %d", core.countSentUnlinks())
	}
}

func TestUnlinkAlias_NonExistent_IsIdempotent(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	err := tm.UnlinkAlias(consumer, target)
	if err != nil {
		t.Fatalf("UnlinkAlias non-existent should be idempotent, got: %v", err)
	}
}

// MonitorAlias tests

func TestMonitorAlias_Local(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	err := tm.MonitorAlias(consumer, target)
	if err != nil {
		t.Fatalf("MonitorAlias failed: %v", err)
	}

	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists == false {
		t.Error("Monitor should be stored")
	}

	if core.countSentMonitors() != 0 {
		t.Error("No network for local")
	}
}

func TestMonitorAlias_Remote_First(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	err := tm.MonitorAlias(consumer, target)
	if err != nil {
		t.Fatalf("MonitorAlias failed: %v", err)
	}

	// Verify monitor stored in monitorRelations
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists == false {
		t.Error("Monitor should be stored in monitorRelations")
	}

	// Verify targetIndex created
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex entry should be created")
	}
	if _, exists := entry.consumers[consumer]; exists == false {
		t.Error("Consumer should be in targetIndex.consumers")
	}

	// Verify network request sent
	if core.countSentMonitors() != 1 {
		t.Fatalf("Expected 1 network request, got %d", core.countSentMonitors())
	}

	// Verify CorePID used
	if sent, ok := core.getFirstSentMonitor(); ok {
		if sent.from != core.pid {
			t.Error("MonitorAlias should use CorePID")
		}
	}
}

func TestMonitorAlias_Remote_Second(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	tm.MonitorAlias(consumer1, target)
	core.resetSentMonitors()

	err := tm.MonitorAlias(consumer2, target)
	if err != nil {
		t.Fatalf("Second MonitorAlias failed: %v", err)
	}

	// Verify NO network for second subscriber (CorePID optimization)
	if core.countSentMonitors() != 0 {
		t.Errorf("Second subscriber should NOT send network, got %d", core.countSentMonitors())
	}

	// Verify both monitors stored in monitorRelations
	if len(tm.monitorRelations) != 2 {
		t.Errorf("Expected 2 monitor relations, got %d", len(tm.monitorRelations))
	}
	key1 := relationKey{consumer: consumer1, target: target}
	if _, exists := tm.monitorRelations[key1]; exists == false {
		t.Error("consumer1 monitor should exist in monitorRelations")
	}
	key2 := relationKey{consumer: consumer2, target: target}
	if _, exists := tm.monitorRelations[key2]; exists == false {
		t.Error("consumer2 monitor should exist in monitorRelations")
	}

	// Verify targetIndex has both consumers
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex entry should exist")
	}
	if len(entry.consumers) != 2 {
		t.Errorf("Expected 2 consumers in targetIndex, got %d", len(entry.consumers))
	}
}

func TestMonitorAlias_Duplicate_Error(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	tm.MonitorAlias(consumer, target)

	err := tm.MonitorAlias(consumer, target)
	if err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist, got %v", err)
	}
}

func TestMonitorAlias_NetworkError_Rollback(t *testing.T) {
	core := newMockCore("node1")
	core.connectionError = gen.ErrNoConnection

	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	err := tm.MonitorAlias(consumer, target)

	if err != gen.ErrNoConnection {
		t.Errorf("Expected ErrNoConnection, got %v", err)
	}

	// Verify monitor rolled back from monitorRelations
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("Monitor should be rolled back from monitorRelations")
	}

	// Verify targetIndex cleaned after rollback
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned after rollback")
	}
}

func TestMonitorAlias_RemoteCorePID_Duplicate_Ignored(t *testing.T) {
	core := newMockCore("node2")
	tm := Create(core, Options{}).(*targetManager)

	localTarget := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}
	remoteCorePID := gen.PID{Node: "node1", ID: 1}

	err := tm.MonitorAlias(remoteCorePID, localTarget)
	if err != nil {
		t.Fatalf("First monitor failed: %v", err)
	}

	err = tm.MonitorAlias(remoteCorePID, localTarget)
	if err != nil {
		t.Errorf("Duplicate from remote CorePID should be ignored, got: %v", err)
	}

	// Verify only ONE relation exists (duplicate was ignored)
	if len(tm.monitorRelations) != 1 {
		t.Errorf("Expected 1 monitor relation, got %d", len(tm.monitorRelations))
	}

	// Verify targetIndex has only one consumer
	entry := tm.targetIndex[localTarget]
	if entry == nil {
		t.Fatal("targetIndex entry should exist")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("Expected 1 consumer in targetIndex, got %d", len(entry.consumers))
	}
}

// DemonitorAlias tests

func TestDemonitorAlias_Local(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	tm.MonitorAlias(consumer, target)

	err := tm.DemonitorAlias(consumer, target)
	if err != nil {
		t.Fatalf("DemonitorAlias failed: %v", err)
	}

	// Verify monitor removed from monitorRelations
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("Monitor should be removed from monitorRelations")
	}

	// Verify targetIndex cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

func TestDemonitorAlias_NotLast(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	tm.MonitorAlias(consumer1, target)
	tm.MonitorAlias(consumer2, target)

	core.resetSentDemonitors()

	tm.DemonitorAlias(consumer1, target)

	// Verify consumer1 monitor removed from monitorRelations
	key1 := relationKey{consumer: consumer1, target: target}
	if _, exists := tm.monitorRelations[key1]; exists {
		t.Error("consumer1 monitor should be removed from monitorRelations")
	}

	// Verify consumer2 monitor still exists
	key2 := relationKey{consumer: consumer2, target: target}
	if _, exists := tm.monitorRelations[key2]; exists == false {
		t.Error("consumer2 monitor should still exist in monitorRelations")
	}

	// Verify targetIndex still exists with only consumer2
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex entry should still exist")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("Expected 1 consumer in targetIndex, got %d", len(entry.consumers))
	}
	if _, exists := entry.consumers[consumer2]; exists == false {
		t.Error("consumer2 should still be in targetIndex.consumers")
	}

	// Verify NO network demonitor sent
	if core.countSentDemonitors() != 0 {
		t.Errorf("NO DemonitorAlias should be sent, got %d", core.countSentDemonitors())
	}
}

func TestDemonitorAlias_Last(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	tm.MonitorAlias(consumer, target)
	core.resetSentDemonitors()

	err := tm.DemonitorAlias(consumer, target)
	if err != nil {
		t.Fatalf("DemonitorAlias failed: %v", err)
	}

	// Verify monitor removed from monitorRelations
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("Monitor should be removed from monitorRelations")
	}

	// Verify targetIndex cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned when last consumer removed")
	}

	// Verify network demonitor sent
	if core.countSentDemonitors() != 1 {
		t.Errorf("Expected 1 DemonitorAlias, got %d", core.countSentDemonitors())
	}
}

func TestDemonitorAlias_Complete(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	tm.MonitorAlias(consumer1, target)
	tm.MonitorAlias(consumer2, target)

	// Demonitor first
	err := tm.DemonitorAlias(consumer1, target)
	if err != nil {
		t.Fatalf("DemonitorAlias failed: %v", err)
	}

	// consumer2 still exists
	key2 := relationKey{consumer: consumer2, target: target}
	if _, exists := tm.monitorRelations[key2]; exists == false {
		t.Error("Second monitor should still exist")
	}

	// Demonitor second (last)
	err = tm.DemonitorAlias(consumer2, target)
	if err != nil {
		t.Fatalf("DemonitorAlias second failed: %v", err)
	}

	// All cleaned
	if len(tm.monitorRelations) != 0 {
		t.Error("All monitors should be cleaned")
	}

	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned")
	}
}

func TestDemonitorAlias_NonExistent_IsIdempotent(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	err := tm.DemonitorAlias(consumer, target)
	if err != nil {
		t.Fatalf("DemonitorAlias non-existent should be idempotent, got: %v", err)
	}
}
