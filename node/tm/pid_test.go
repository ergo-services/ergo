package tm

import (
	"testing"

	"ergo.services/ergo/gen"
)

// LinkPID tests

func TestLinkPID_Local_Basic(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	err := tm.LinkPID(consumer, target)
	if err != nil {
		t.Fatalf("LinkPID failed: %v", err)
	}

	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists == false {
		t.Error("Link should be stored in linkRelations")
	}

	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetEntry should be created")
	}

	if _, exists := entry.consumers[consumer]; exists == false {
		t.Error("Consumer should be in targetEntry.consumers")
	}

	if core.countSentLinks() != 0 {
		t.Errorf("Expected 0 network requests for local, got %d", core.countSentLinks())
	}
}

func TestLinkPID_Remote_FirstSubscriber(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	err := tm.LinkPID(consumer, target)
	if err != nil {
		t.Fatalf("LinkPID failed: %v", err)
	}

	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists == false {
		t.Error("Link should be stored locally")
	}

	if core.countSentLinks() != 1 {
		t.Fatalf("Expected 1 network request, got %d", core.countSentLinks())
	}

	item := core.sentLinks.Item()
	if item != nil {
		sent := item.Value().(linkRequest)
		if sent.from != core.pid {
			t.Errorf("Expected CorePID as from, got %v", sent.from)
		}
		if sent.to != target {
			t.Errorf("Expected target %v, got %v", target, sent.to)
		}
	}

	entry := tm.targetIndex[target]
	if entry.allowAlwaysFirst == true {
		t.Error("allowAlwaysFirst should be false after successful remote link")
	}
}

func TestLinkPID_Remote_SecondSubscriber_NoNetwork(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.PID{Node: "node2", ID: 200}

	err := tm.LinkPID(consumer1, target)
	if err != nil {
		t.Fatalf("First LinkPID failed: %v", err)
	}

	if core.countSentLinks() != 1 {
		t.Fatalf("First should send network request, got %d", core.countSentLinks())
	}

	core.resetSentLinks()

	err = tm.LinkPID(consumer2, target)
	if err != nil {
		t.Fatalf("Second LinkPID failed: %v", err)
	}

	key2 := relationKey{consumer: consumer2, target: target}
	if _, exists := tm.linkRelations[key2]; exists == false {
		t.Error("Second link should be stored locally")
	}

	if core.countSentLinks() != 0 {
		t.Errorf("Second subscriber should NOT send network request (CorePID optimization), got %d", core.countSentLinks())
	}

	entry := tm.targetIndex[target]
	if len(entry.consumers) != 2 {
		t.Errorf("Expected 2 consumers in targetIndex, got %d", len(entry.consumers))
	}
}

func TestLinkPID_Remote_ThreeSubscribers_OneNetworkRequest(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	consumer3 := gen.PID{Node: "node1", ID: 102}
	target := gen.PID{Node: "node2", ID: 200}

	tm.LinkPID(consumer1, target)
	tm.LinkPID(consumer2, target)
	tm.LinkPID(consumer3, target)

	if core.countSentLinks() != 1 {
		t.Errorf("Expected exactly 1 network request for 3 subscribers, got %d", core.countSentLinks())
	}

	if len(tm.linkRelations) != 3 {
		t.Errorf("Expected 3 links stored locally, got %d", len(tm.linkRelations))
	}

	if sent, ok := core.getFirstSentLink(); ok {
		if sent.from != core.pid {
			t.Error("Network request should use CorePID, not individual process")
		}
	}
}

func TestLinkPID_Duplicate_Error(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	err := tm.LinkPID(consumer, target)
	if err != nil {
		t.Fatalf("First LinkPID failed: %v", err)
	}

	err = tm.LinkPID(consumer, target)
	if err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist for duplicate link, got %v", err)
	}
}

func TestLinkPID_NetworkError_Rollback(t *testing.T) {
	core := newMockCore("node1")
	core.connectionError = gen.ErrNoConnection

	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	err := tm.LinkPID(consumer, target)

	if err != gen.ErrNoConnection {
		t.Errorf("Expected ErrNoConnection, got %v", err)
	}

	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("Link should be rolled back after network error")
	}

	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned after rollback")
	}
}

func TestLinkPID_RemoteError_Rollback(t *testing.T) {
	core := newMockCore("node1")
	core.linkError = gen.ErrProcessUnknown

	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	err := tm.LinkPID(consumer, target)

	if err != gen.ErrProcessUnknown {
		t.Errorf("Expected ErrProcessUnknown, got %v", err)
	}

	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("Link should be rolled back after remote error")
	}
}

func TestLinkPID_RemoteCorePID_Duplicate_Ignored(t *testing.T) {
	core := newMockCore("node2")
	tm := Create(core, Options{}).(*targetManager)

	localTarget := gen.PID{Node: "node2", ID: 200}
	remoteCorePID := gen.PID{Node: "node1", ID: 1}

	err := tm.LinkPID(remoteCorePID, localTarget)
	if err != nil {
		t.Fatalf("First link from remote CorePID failed: %v", err)
	}

	err = tm.LinkPID(remoteCorePID, localTarget)
	if err != nil {
		t.Errorf("Duplicate from remote CorePID should be ignored, got error: %v", err)
	}

	// Verify only ONE relation exists (duplicate was ignored, not added twice)
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

// UnlinkPID tests

func TestUnlinkPID_NotLastLocal(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.PID{Node: "node2", ID: 200}

	tm.LinkPID(consumer1, target)
	tm.LinkPID(consumer2, target)

	core.resetSentUnlinks()

	err := tm.UnlinkPID(consumer1, target)
	if err != nil {
		t.Fatalf("UnlinkPID failed: %v", err)
	}

	// Verify consumer1 link removed from linkRelations
	key1 := relationKey{consumer: consumer1, target: target}
	if _, exists := tm.linkRelations[key1]; exists {
		t.Error("consumer1 link should be removed from linkRelations")
	}

	// Verify consumer2 link still exists in linkRelations
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

	// Verify NO network unlink sent (other local consumers still exist)
	if core.countSentUnlinks() != 0 {
		t.Errorf("NO UnlinkPID should be sent when other local consumers exist, got %d", core.countSentUnlinks())
	}
}

func TestUnlinkPID_LastLocal_SendsUnlink(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	tm.LinkPID(consumer, target)

	core.resetSentUnlinks()

	err := tm.UnlinkPID(consumer, target)
	if err != nil {
		t.Fatalf("UnlinkPID failed: %v", err)
	}

	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("Link should be removed")
	}

	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned when last consumer removed")
	}

	if core.countSentUnlinks() != 1 {
		t.Errorf("Expected 1 UnlinkPID when last local consumer removed, got %d", core.countSentUnlinks())
	}

	if core.countSentUnlinks() > 0 {
		item := core.sentUnlinks.Item()
		if item != nil {
			sent := item.Value().(linkRequest)
			if sent.from != core.pid {
				t.Errorf("UnlinkPID should use CorePID, got %v", sent.from)
			}
		}
	}
}

// MonitorPID tests

func TestMonitorPID_Local_Basic(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	err := tm.MonitorPID(consumer, target)
	if err != nil {
		t.Fatalf("MonitorPID failed: %v", err)
	}

	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists == false {
		t.Error("Monitor should be stored")
	}

	if core.countSentMonitors() != 0 {
		t.Errorf("Expected 0 network requests for local, got %d", core.countSentMonitors())
	}
}

func TestMonitorPID_Remote_FirstSubscriber(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	err := tm.MonitorPID(consumer, target)
	if err != nil {
		t.Fatalf("MonitorPID failed: %v", err)
	}

	if core.countSentMonitors() != 1 {
		t.Fatalf("Expected 1 network request, got %d", core.countSentMonitors())
	}
}

func TestMonitorPID_Remote_SecondSubscriber_NoNetwork(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.PID{Node: "node2", ID: 200}

	tm.MonitorPID(consumer1, target)
	core.resetSentMonitors()

	err := tm.MonitorPID(consumer2, target)
	if err != nil {
		t.Fatalf("Second MonitorPID failed: %v", err)
	}

	if core.countSentMonitors() != 0 {
		t.Errorf("Second subscriber should NOT send network, got %d", core.countSentMonitors())
	}
}

func TestMonitorPID_Remote_ThreeSubscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	consumer3 := gen.PID{Node: "node1", ID: 102}
	target := gen.PID{Node: "node2", ID: 200}

	tm.MonitorPID(consumer1, target)
	tm.MonitorPID(consumer2, target)
	tm.MonitorPID(consumer3, target)

	// Verify exactly 1 network request (CorePID optimization)
	if core.countSentMonitors() != 1 {
		t.Errorf("Expected exactly 1 network request for 3 subscribers, got %d", core.countSentMonitors())
	}

	// Verify all 3 monitors stored in monitorRelations
	if len(tm.monitorRelations) != 3 {
		t.Errorf("Expected 3 monitor relations, got %d", len(tm.monitorRelations))
	}

	// Verify targetIndex has all 3 consumers
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex entry should exist")
	}
	if len(entry.consumers) != 3 {
		t.Errorf("Expected 3 consumers in targetIndex, got %d", len(entry.consumers))
	}

	// Verify CorePID was used for network request
	if sent, ok := core.getFirstSentMonitor(); ok {
		if sent.from != core.pid {
			t.Errorf("Network request should use CorePID, got %v", sent.from)
		}
	}
}

func TestMonitorPID_Duplicate_Error(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	tm.MonitorPID(consumer, target)

	err := tm.MonitorPID(consumer, target)
	if err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist, got %v", err)
	}
}

func TestMonitorPID_NetworkError_Rollback(t *testing.T) {
	core := newMockCore("node1")
	core.connectionError = gen.ErrNoConnection

	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	err := tm.MonitorPID(consumer, target)

	if err != gen.ErrNoConnection {
		t.Errorf("Expected ErrNoConnection, got %v", err)
	}

	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("Monitor should be rolled back")
	}
}

func TestMonitorPID_RemoteError_Rollback(t *testing.T) {
	core := newMockCore("node1")
	core.linkError = gen.ErrProcessUnknown

	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	err := tm.MonitorPID(consumer, target)

	if err != gen.ErrProcessUnknown {
		t.Errorf("Expected ErrProcessUnknown, got %v", err)
	}

	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("Monitor should be rolled back")
	}
}

func TestMonitorPID_RemoteCorePID_Duplicate(t *testing.T) {
	core := newMockCore("node2")
	tm := Create(core, Options{}).(*targetManager)

	localTarget := gen.PID{Node: "node2", ID: 200}
	remoteCorePID := gen.PID{Node: "node1", ID: 1}

	err := tm.MonitorPID(remoteCorePID, localTarget)
	if err != nil {
		t.Fatalf("First monitor from remote CorePID failed: %v", err)
	}

	err = tm.MonitorPID(remoteCorePID, localTarget)
	if err != nil {
		t.Errorf("Duplicate from remote CorePID should be ignored, got: %v", err)
	}

	// Verify only ONE relation exists (duplicate was ignored, not added twice)
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

func TestMonitorPID_ReSubscribe(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	tm.MonitorPID(consumer, target)

	if core.countSentMonitors() != 1 {
		t.Fatalf("First should send network, got %d", core.countSentMonitors())
	}

	// Verify monitor stored
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists == false {
		t.Error("Monitor should be stored after first MonitorPID")
	}

	core.resetSentMonitors()

	tm.DemonitorPID(consumer, target)

	// Verify monitor removed after demonitor
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("Monitor should be removed after DemonitorPID")
	}
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned after DemonitorPID")
	}

	err := tm.MonitorPID(consumer, target)
	if err != nil {
		t.Fatalf("Re-subscribe MonitorPID failed: %v", err)
	}

	// Verify network sent on re-subscribe
	if core.countSentMonitors() != 1 {
		t.Errorf("Re-subscribe should send network, got %d", core.countSentMonitors())
	}

	// Verify monitor stored again
	if _, exists := tm.monitorRelations[key]; exists == false {
		t.Error("Monitor should be stored after re-subscribe")
	}

	// Verify targetIndex recreated
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex entry should be recreated after re-subscribe")
	}
	if _, exists := entry.consumers[consumer]; exists == false {
		t.Error("Consumer should be in targetIndex.consumers after re-subscribe")
	}
}

// DemonitorPID tests

func TestDemonitorPID_NotLast(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.PID{Node: "node2", ID: 200}

	tm.MonitorPID(consumer1, target)
	tm.MonitorPID(consumer2, target)

	core.resetSentDemonitors()

	tm.DemonitorPID(consumer1, target)

	// Verify consumer1 monitor removed from monitorRelations
	key1 := relationKey{consumer: consumer1, target: target}
	if _, exists := tm.monitorRelations[key1]; exists {
		t.Error("consumer1 monitor should be removed from monitorRelations")
	}

	// Verify consumer2 monitor still exists in monitorRelations
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

	// Verify NO network demonitor sent (other local consumers still exist)
	if core.countSentDemonitors() != 0 {
		t.Errorf("NO DemonitorPID should be sent when other local consumers exist, got %d", core.countSentDemonitors())
	}
}

func TestDemonitorPID_LastLocal_SendsDemonitor(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	tm.MonitorPID(consumer, target)

	core.resetSentDemonitors()

	tm.DemonitorPID(consumer, target)

	if core.countSentDemonitors() != 1 {
		t.Errorf("Expected 1 DemonitorPID, got %d", core.countSentDemonitors())
	}

	if core.countSentDemonitors() > 0 {
		item := core.sentDemonitors.Item()
		if item != nil {
			sent := item.Value().(monitorRequest)
			if sent.from != core.pid {
				t.Errorf("DemonitorPID should use CorePID, got %v", sent.from)
			}
		}
	}
}
