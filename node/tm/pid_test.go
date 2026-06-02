package tm

import (
	"testing"

	"ergo.services/ergo/gen"
)

// LinkPID tests

func TestLinkPID_Local_Basic(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	if err := m.LinkPID(consumer, target); err != nil {
		t.Fatalf("LinkPID failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) == false {
		t.Error("Link should be stored in linkRelations")
	}
	if m.getTargetEntry(target) == nil {
		t.Fatal("targetEntry should be created")
	}
	if m.storage.Has(target, consumer, KindLink) == false {
		t.Error("Consumer should be in target's index")
	}
	if core.countSentLinks() != 0 {
		t.Errorf("Expected 0 network requests for local, got %d", core.countSentLinks())
	}
}

func TestLinkPID_Remote_FirstSubscriber(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	if err := m.LinkPID(consumer, target); err != nil {
		t.Fatalf("LinkPID failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) == false {
		t.Error("Link should be stored locally")
	}
	if core.countSentLinks() != 1 {
		t.Fatalf("Expected 1 network request, got %d", core.countSentLinks())
	}
	if sent, ok := core.getFirstSentLink(); ok {
		if sent.from != core.pid {
			t.Errorf("Expected CorePID as from, got %v", sent.from)
		}
		if sent.to != target {
			t.Errorf("Expected target %v, got %v", target, sent.to)
		}
	}
	if m.wireEstablished(target, KindLink) == false {
		t.Error("wire should be marked established after successful remote link")
	}
}

func TestLinkPID_Remote_SecondSubscriber_NoNetwork(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.PID{Node: "node2", ID: 200}

	if err := m.LinkPID(consumer1, target); err != nil {
		t.Fatalf("First LinkPID failed: %v", err)
	}
	if core.countSentLinks() != 1 {
		t.Fatalf("First should send network request, got %d", core.countSentLinks())
	}
	core.resetSentLinks()

	if err := m.LinkPID(consumer2, target); err != nil {
		t.Fatalf("Second LinkPID failed: %v", err)
	}
	if m.hasLinkRelation(consumer2, target) == false {
		t.Error("Second link should be stored locally")
	}
	if core.countSentLinks() != 0 {
		t.Errorf("Second subscriber should NOT send network request, got %d", core.countSentLinks())
	}
	if m.consumerCount(target) != 2 {
		t.Errorf("Expected 2 consumers in target index, got %d", m.consumerCount(target))
	}
}

func TestLinkPID_Remote_ThreeSubscribers_OneNetworkRequest(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	consumer3 := gen.PID{Node: "node1", ID: 102}
	target := gen.PID{Node: "node2", ID: 200}

	m.LinkPID(consumer1, target)
	m.LinkPID(consumer2, target)
	m.LinkPID(consumer3, target)

	if core.countSentLinks() != 1 {
		t.Errorf("Expected exactly 1 network request for 3 subscribers, got %d", core.countSentLinks())
	}
	if m.totalLinks() != 3 {
		t.Errorf("Expected 3 links stored locally, got %d", m.totalLinks())
	}
	if sent, ok := core.getFirstSentLink(); ok {
		if sent.from != core.pid {
			t.Error("Network request should use CorePID, not individual process")
		}
	}
}

func TestLinkPID_Duplicate_Error(t *testing.T) {
	m, _ := newManagerWithMock("node1")

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	if err := m.LinkPID(consumer, target); err != nil {
		t.Fatalf("First LinkPID failed: %v", err)
	}
	if err := m.LinkPID(consumer, target); err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist for duplicate link, got %v", err)
	}
}

func TestLinkPID_NetworkError_Rollback(t *testing.T) {
	m, core := newManagerWithMock("node1")
	core.connectionError = gen.ErrNoConnection

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	err := m.LinkPID(consumer, target)
	if err != gen.ErrNoConnection {
		t.Errorf("Expected ErrNoConnection, got %v", err)
	}
	if m.hasLinkRelation(consumer, target) {
		t.Error("Link should be rolled back after network error")
	}
	if m.getTargetEntry(target) != nil && m.consumerCount(target) != 0 {
		t.Error("target index should have no consumers after rollback")
	}
}

func TestLinkPID_RemoteError_Rollback(t *testing.T) {
	m, core := newManagerWithMock("node1")
	core.linkError = gen.ErrProcessUnknown

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	err := m.LinkPID(consumer, target)
	if err != gen.ErrProcessUnknown {
		t.Errorf("Expected ErrProcessUnknown, got %v", err)
	}
	if m.hasLinkRelation(consumer, target) {
		t.Error("Link should be rolled back after remote error")
	}
}

func TestLinkPID_RemoteCorePID_Duplicate_Ignored(t *testing.T) {
	m, _ := newManagerWithMock("node2")

	localTarget := gen.PID{Node: "node2", ID: 200}
	remoteCorePID := gen.PID{Node: "node1", ID: 1}

	if err := m.LinkPID(remoteCorePID, localTarget); err != nil {
		t.Fatalf("First link from remote CorePID failed: %v", err)
	}
	if err := m.LinkPID(remoteCorePID, localTarget); err != nil {
		t.Errorf("Duplicate from remote CorePID should be ignored, got error: %v", err)
	}
	if m.totalLinks() != 1 {
		t.Errorf("Expected 1 link relation, got %d", m.totalLinks())
	}
	if m.consumerCount(localTarget) != 1 {
		t.Errorf("Expected 1 consumer in target index, got %d", m.consumerCount(localTarget))
	}
}

// UnlinkPID tests

func TestUnlinkPID_NotLastLocal(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.PID{Node: "node2", ID: 200}

	m.LinkPID(consumer1, target)
	m.LinkPID(consumer2, target)
	core.resetSentUnlinks()

	if err := m.UnlinkPID(consumer1, target); err != nil {
		t.Fatalf("UnlinkPID failed: %v", err)
	}
	if m.hasLinkRelation(consumer1, target) {
		t.Error("consumer1 link should be removed")
	}
	if m.hasLinkRelation(consumer2, target) == false {
		t.Error("consumer2 link should still exist")
	}
	if m.consumerCount(target) != 1 {
		t.Errorf("Expected 1 consumer remaining, got %d", m.consumerCount(target))
	}
	if core.countSentUnlinks() != 0 {
		t.Errorf("NO UnlinkPID should be sent while other local consumers exist, got %d", core.countSentUnlinks())
	}
}

func TestUnlinkPID_LastLocal_SendsUnlink(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	m.LinkPID(consumer, target)
	core.resetSentUnlinks()

	if err := m.UnlinkPID(consumer, target); err != nil {
		t.Fatalf("UnlinkPID failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) {
		t.Error("Link should be removed")
	}
	if core.countSentUnlinks() != 1 {
		t.Errorf("Expected 1 UnlinkPID when last local consumer removed, got %d", core.countSentUnlinks())
	}
	if sent, ok := core.getFirstSentUnlink(); ok {
		if sent.from != core.pid {
			t.Errorf("UnlinkPID should use CorePID, got %v", sent.from)
		}
	}
}

// MonitorPID tests

func TestMonitorPID_Local_Basic(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	if err := m.MonitorPID(consumer, target); err != nil {
		t.Fatalf("MonitorPID failed: %v", err)
	}
	if m.hasMonitorRelation(consumer, target) == false {
		t.Error("Monitor should be stored")
	}
	if core.countSentMonitors() != 0 {
		t.Errorf("Expected 0 network requests for local, got %d", core.countSentMonitors())
	}
}

func TestMonitorPID_Remote_FirstSubscriber(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	if err := m.MonitorPID(consumer, target); err != nil {
		t.Fatalf("MonitorPID failed: %v", err)
	}
	if core.countSentMonitors() != 1 {
		t.Fatalf("Expected 1 network request, got %d", core.countSentMonitors())
	}
}

func TestMonitorPID_Remote_SecondSubscriber_NoNetwork(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.PID{Node: "node2", ID: 200}

	m.MonitorPID(consumer1, target)
	core.resetSentMonitors()

	if err := m.MonitorPID(consumer2, target); err != nil {
		t.Fatalf("Second MonitorPID failed: %v", err)
	}
	if core.countSentMonitors() != 0 {
		t.Errorf("Second subscriber should NOT send network, got %d", core.countSentMonitors())
	}
}

func TestMonitorPID_Remote_ThreeSubscribers(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	consumer3 := gen.PID{Node: "node1", ID: 102}
	target := gen.PID{Node: "node2", ID: 200}

	m.MonitorPID(consumer1, target)
	m.MonitorPID(consumer2, target)
	m.MonitorPID(consumer3, target)

	if core.countSentMonitors() != 1 {
		t.Errorf("Expected exactly 1 network request for 3 subscribers, got %d", core.countSentMonitors())
	}
	if m.totalMonitors() != 3 {
		t.Errorf("Expected 3 monitor relations, got %d", m.totalMonitors())
	}
	if m.consumerCount(target) != 3 {
		t.Errorf("Expected 3 consumers in target index, got %d", m.consumerCount(target))
	}
	if sent, ok := core.getFirstSentMonitor(); ok {
		if sent.from != core.pid {
			t.Errorf("Network request should use CorePID, got %v", sent.from)
		}
	}
}

func TestMonitorPID_Duplicate_Error(t *testing.T) {
	m, _ := newManagerWithMock("node1")

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	m.MonitorPID(consumer, target)
	if err := m.MonitorPID(consumer, target); err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist, got %v", err)
	}
}

func TestMonitorPID_NetworkError_Rollback(t *testing.T) {
	m, core := newManagerWithMock("node1")
	core.connectionError = gen.ErrNoConnection

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	err := m.MonitorPID(consumer, target)
	if err != gen.ErrNoConnection {
		t.Errorf("Expected ErrNoConnection, got %v", err)
	}
	if m.hasMonitorRelation(consumer, target) {
		t.Error("Monitor should be rolled back")
	}
}

func TestMonitorPID_RemoteError_Rollback(t *testing.T) {
	m, core := newManagerWithMock("node1")
	core.linkError = gen.ErrProcessUnknown

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	err := m.MonitorPID(consumer, target)
	if err != gen.ErrProcessUnknown {
		t.Errorf("Expected ErrProcessUnknown, got %v", err)
	}
	if m.hasMonitorRelation(consumer, target) {
		t.Error("Monitor should be rolled back")
	}
}

func TestMonitorPID_RemoteCorePID_Duplicate(t *testing.T) {
	m, _ := newManagerWithMock("node2")

	localTarget := gen.PID{Node: "node2", ID: 200}
	remoteCorePID := gen.PID{Node: "node1", ID: 1}

	if err := m.MonitorPID(remoteCorePID, localTarget); err != nil {
		t.Fatalf("First monitor from remote CorePID failed: %v", err)
	}
	if err := m.MonitorPID(remoteCorePID, localTarget); err != nil {
		t.Errorf("Duplicate from remote CorePID should be ignored, got: %v", err)
	}
	if m.totalMonitors() != 1 {
		t.Errorf("Expected 1 monitor relation, got %d", m.totalMonitors())
	}
	if m.consumerCount(localTarget) != 1 {
		t.Errorf("Expected 1 consumer in target index, got %d", m.consumerCount(localTarget))
	}
}

func TestMonitorPID_ReSubscribe(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	m.MonitorPID(consumer, target)
	if core.countSentMonitors() != 1 {
		t.Fatalf("First should send network, got %d", core.countSentMonitors())
	}
	if m.hasMonitorRelation(consumer, target) == false {
		t.Error("Monitor should be stored after first MonitorPID")
	}
	core.resetSentMonitors()

	m.DemonitorPID(consumer, target)
	if m.hasMonitorRelation(consumer, target) {
		t.Error("Monitor should be removed after DemonitorPID")
	}

	if err := m.MonitorPID(consumer, target); err != nil {
		t.Fatalf("Re-subscribe MonitorPID failed: %v", err)
	}
	if core.countSentMonitors() != 1 {
		t.Errorf("Re-subscribe should send network, got %d", core.countSentMonitors())
	}
	if m.hasMonitorRelation(consumer, target) == false {
		t.Error("Monitor should be stored after re-subscribe")
	}
}

// DemonitorPID tests

func TestDemonitorPID_NotLast(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	target := gen.PID{Node: "node2", ID: 200}

	m.MonitorPID(consumer1, target)
	m.MonitorPID(consumer2, target)
	core.resetSentDemonitors()

	m.DemonitorPID(consumer1, target)
	if m.hasMonitorRelation(consumer1, target) {
		t.Error("consumer1 monitor should be removed")
	}
	if m.hasMonitorRelation(consumer2, target) == false {
		t.Error("consumer2 monitor should still exist")
	}
	if m.consumerCount(target) != 1 {
		t.Errorf("Expected 1 consumer remaining, got %d", m.consumerCount(target))
	}
	if core.countSentDemonitors() != 0 {
		t.Errorf("NO DemonitorPID should be sent while other local consumers exist, got %d", core.countSentDemonitors())
	}
}

func TestDemonitorPID_LastLocal_SendsDemonitor(t *testing.T) {
	m, core := newManagerWithMock("node1")

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	m.MonitorPID(consumer, target)
	core.resetSentDemonitors()

	m.DemonitorPID(consumer, target)
	if core.countSentDemonitors() != 1 {
		t.Errorf("Expected 1 DemonitorPID, got %d", core.countSentDemonitors())
	}
	if sent, ok := core.getFirstSentDemonitor(); ok {
		if sent.from != core.pid {
			t.Errorf("DemonitorPID should use CorePID, got %v", sent.from)
		}
	}
}
