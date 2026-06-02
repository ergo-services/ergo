package tm

import (
	"testing"

	"ergo.services/ergo/gen"
)

func TestLinkAlias_Local(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	if err := m.LinkAlias(consumer, target); err != nil {
		t.Fatalf("LinkAlias failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) == false {
		t.Error("Link should be stored")
	}
	if core.countSentLinks() != 0 {
		t.Errorf("Expected 0 network requests for local, got %d", core.countSentLinks())
	}
}

func TestLinkAlias_Remote_First(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	if err := m.LinkAlias(consumer, target); err != nil {
		t.Fatalf("LinkAlias failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) == false {
		t.Error("Link should be stored")
	}
	if m.getTargetEntry(target) == nil {
		t.Fatal("target entry should be created")
	}
	if m.storage.Has(target, consumer, KindLink) == false {
		t.Error("Consumer should be in target index")
	}
	if core.countSentLinks() != 1 {
		t.Fatalf("Expected 1 network request, got %d", core.countSentLinks())
	}
	if sent, ok := core.getFirstSentLink(); ok {
		if sent.from != core.pid {
			t.Error("LinkAlias should use CorePID")
		}
	}
}

func TestLinkAlias_Remote_Second(t *testing.T) {
	m, core := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	m.LinkAlias(c1, target)
	core.resetSentLinks()

	if err := m.LinkAlias(c2, target); err != nil {
		t.Fatalf("Second LinkAlias failed: %v", err)
	}
	if core.countSentLinks() != 0 {
		t.Errorf("Second subscriber should NOT send network, got %d", core.countSentLinks())
	}
	if m.totalLinks() != 2 {
		t.Errorf("Expected 2 link relations, got %d", m.totalLinks())
	}
	if m.consumerCount(target) != 2 {
		t.Errorf("Expected 2 consumers, got %d", m.consumerCount(target))
	}
}

func TestLinkAlias_Duplicate_Error(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	m.LinkAlias(consumer, target)
	if err := m.LinkAlias(consumer, target); err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist, got %v", err)
	}
}

func TestLinkAlias_NetworkError_Rollback(t *testing.T) {
	m, core := newManagerWithMock("node1")
	core.connectionError = gen.ErrNoConnection

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	err := m.LinkAlias(consumer, target)
	if err != gen.ErrNoConnection {
		t.Errorf("Expected ErrNoConnection, got %v", err)
	}
	if m.hasLinkRelation(consumer, target) {
		t.Error("Link should be rolled back")
	}
}

func TestLinkAlias_RemoteCorePID_Duplicate_Ignored(t *testing.T) {
	m, _ := newManagerWithMock("node2")
	localTarget := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}
	remoteCorePID := gen.PID{Node: "node1", ID: 1}

	if err := m.LinkAlias(remoteCorePID, localTarget); err != nil {
		t.Fatalf("First link failed: %v", err)
	}
	if err := m.LinkAlias(remoteCorePID, localTarget); err != nil {
		t.Errorf("Duplicate should be ignored, got %v", err)
	}
	if m.totalLinks() != 1 {
		t.Errorf("Expected 1 link, got %d", m.totalLinks())
	}
}

func TestUnlinkAlias_Local(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	m.LinkAlias(consumer, target)
	if err := m.UnlinkAlias(consumer, target); err != nil {
		t.Fatalf("UnlinkAlias failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) {
		t.Error("Link should be removed")
	}
}

func TestUnlinkAlias_NotLast(t *testing.T) {
	m, core := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	m.LinkAlias(c1, target)
	m.LinkAlias(c2, target)
	core.resetSentUnlinks()

	m.UnlinkAlias(c1, target)
	if m.hasLinkRelation(c1, target) {
		t.Error("c1 link should be removed")
	}
	if m.hasLinkRelation(c2, target) == false {
		t.Error("c2 link should still exist")
	}
	if m.consumerCount(target) != 1 {
		t.Errorf("Expected 1 consumer, got %d", m.consumerCount(target))
	}
	if core.countSentUnlinks() != 0 {
		t.Errorf("NO wire UnlinkAlias should be sent, got %d", core.countSentUnlinks())
	}
}

func TestUnlinkAlias_Last(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	m.LinkAlias(consumer, target)
	core.resetSentUnlinks()

	if err := m.UnlinkAlias(consumer, target); err != nil {
		t.Fatalf("UnlinkAlias failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) {
		t.Error("Link should be removed")
	}
	if core.countSentUnlinks() != 1 {
		t.Errorf("Expected 1 wire UnlinkAlias, got %d", core.countSentUnlinks())
	}
}

func TestUnlinkAlias_NonExistent_IsIdempotent(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	if err := m.UnlinkAlias(consumer, target); err != nil {
		t.Fatalf("UnlinkAlias non-existent should be idempotent, got %v", err)
	}
}

func TestMonitorAlias_Local(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	if err := m.MonitorAlias(consumer, target); err != nil {
		t.Fatalf("MonitorAlias failed: %v", err)
	}
	if m.hasMonitorRelation(consumer, target) == false {
		t.Error("Monitor should be stored")
	}
	if core.countSentMonitors() != 0 {
		t.Error("No network for local")
	}
}

func TestMonitorAlias_Remote_First(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	if err := m.MonitorAlias(consumer, target); err != nil {
		t.Fatalf("MonitorAlias failed: %v", err)
	}
	if m.hasMonitorRelation(consumer, target) == false {
		t.Error("Monitor should be stored")
	}
	if core.countSentMonitors() != 1 {
		t.Fatalf("Expected 1 wire MonitorAlias, got %d", core.countSentMonitors())
	}
	if sent, ok := core.getFirstSentMonitor(); ok {
		if sent.from != core.pid {
			t.Error("MonitorAlias should use CorePID")
		}
	}
}

func TestMonitorAlias_Remote_Second(t *testing.T) {
	m, core := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	m.MonitorAlias(c1, target)
	core.resetSentMonitors()

	if err := m.MonitorAlias(c2, target); err != nil {
		t.Fatalf("Second MonitorAlias failed: %v", err)
	}
	if core.countSentMonitors() != 0 {
		t.Errorf("Second subscriber should NOT send network, got %d", core.countSentMonitors())
	}
	if m.totalMonitors() != 2 {
		t.Errorf("Expected 2 monitor relations, got %d", m.totalMonitors())
	}
	if m.consumerCount(target) != 2 {
		t.Errorf("Expected 2 consumers, got %d", m.consumerCount(target))
	}
}

func TestMonitorAlias_Duplicate_Error(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	m.MonitorAlias(consumer, target)
	if err := m.MonitorAlias(consumer, target); err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist, got %v", err)
	}
}

func TestMonitorAlias_NetworkError_Rollback(t *testing.T) {
	m, core := newManagerWithMock("node1")
	core.connectionError = gen.ErrNoConnection

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	err := m.MonitorAlias(consumer, target)
	if err != gen.ErrNoConnection {
		t.Errorf("Expected ErrNoConnection, got %v", err)
	}
	if m.hasMonitorRelation(consumer, target) {
		t.Error("Monitor should be rolled back")
	}
}

func TestMonitorAlias_RemoteCorePID_Duplicate_Ignored(t *testing.T) {
	m, _ := newManagerWithMock("node2")
	localTarget := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}
	remoteCorePID := gen.PID{Node: "node1", ID: 1}

	if err := m.MonitorAlias(remoteCorePID, localTarget); err != nil {
		t.Fatalf("First monitor failed: %v", err)
	}
	if err := m.MonitorAlias(remoteCorePID, localTarget); err != nil {
		t.Errorf("Duplicate should be ignored, got %v", err)
	}
	if m.totalMonitors() != 1 {
		t.Errorf("Expected 1 monitor, got %d", m.totalMonitors())
	}
}

func TestDemonitorAlias_Local(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	m.MonitorAlias(consumer, target)
	if err := m.DemonitorAlias(consumer, target); err != nil {
		t.Fatalf("DemonitorAlias failed: %v", err)
	}
	if m.hasMonitorRelation(consumer, target) {
		t.Error("Monitor should be removed")
	}
}

func TestDemonitorAlias_NotLast(t *testing.T) {
	m, core := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	m.MonitorAlias(c1, target)
	m.MonitorAlias(c2, target)
	core.resetSentDemonitors()

	m.DemonitorAlias(c1, target)
	if m.hasMonitorRelation(c1, target) {
		t.Error("c1 monitor should be removed")
	}
	if m.hasMonitorRelation(c2, target) == false {
		t.Error("c2 monitor should still exist")
	}
	if core.countSentDemonitors() != 0 {
		t.Errorf("NO wire DemonitorAlias should be sent, got %d", core.countSentDemonitors())
	}
}

func TestDemonitorAlias_Last(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	m.MonitorAlias(consumer, target)
	core.resetSentDemonitors()

	if err := m.DemonitorAlias(consumer, target); err != nil {
		t.Fatalf("DemonitorAlias failed: %v", err)
	}
	if m.hasMonitorRelation(consumer, target) {
		t.Error("Monitor should be removed")
	}
	if core.countSentDemonitors() != 1 {
		t.Errorf("Expected 1 wire DemonitorAlias, got %d", core.countSentDemonitors())
	}
}

func TestDemonitorAlias_Complete(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.Alias{Node: "node2", ID: [3]uint64{1, 2, 3}}

	m.MonitorAlias(c1, target)
	m.MonitorAlias(c2, target)

	if err := m.DemonitorAlias(c1, target); err != nil {
		t.Fatalf("DemonitorAlias failed: %v", err)
	}
	if m.hasMonitorRelation(c2, target) == false {
		t.Error("c2 monitor should still exist")
	}
	if err := m.DemonitorAlias(c2, target); err != nil {
		t.Fatalf("DemonitorAlias second failed: %v", err)
	}
	if m.totalMonitors() != 0 {
		t.Error("All monitors should be cleaned")
	}
}

func TestDemonitorAlias_NonExistent_IsIdempotent(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	if err := m.DemonitorAlias(consumer, target); err != nil {
		t.Fatalf("DemonitorAlias non-existent should be idempotent, got %v", err)
	}
}
