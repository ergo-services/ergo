package tm

import (
	"testing"

	"ergo.services/ergo/gen"
)

func TestLinkProcessID_Local(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node1", Name: "test"}

	if err := m.LinkProcessID(consumer, target); err != nil {
		t.Fatalf("LinkProcessID failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) == false {
		t.Error("Link should be stored")
	}
	if core.countSentLinks() != 0 {
		t.Error("No network for local")
	}
}

func TestLinkProcessID_Local_EmptyNode(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Name: "test"} // Empty node = local

	if err := m.LinkProcessID(consumer, target); err != nil {
		t.Fatalf("LinkProcessID failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) == false {
		t.Error("Link should be stored")
	}
	if core.countSentLinks() != 0 {
		t.Error("No network for local (empty node)")
	}
}

func TestLinkProcessID_Remote_First(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node2", Name: "test"}

	if err := m.LinkProcessID(consumer, target); err != nil {
		t.Fatalf("LinkProcessID failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) == false {
		t.Error("Link should be stored")
	}
	if m.getTargetEntry(target) == nil {
		t.Fatal("target entry should be created")
	}
	if core.countSentLinks() != 1 {
		t.Fatalf("Expected 1 network request, got %d", core.countSentLinks())
	}
	if sent, ok := core.getFirstSentLink(); ok {
		if sent.from != core.pid {
			t.Error("LinkProcessID should use CorePID")
		}
	}
}

func TestLinkProcessID_Remote_Second(t *testing.T) {
	m, core := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.ProcessID{Node: "node2", Name: "test"}

	m.LinkProcessID(c1, target)
	core.resetSentLinks()

	if err := m.LinkProcessID(c2, target); err != nil {
		t.Fatalf("Second LinkProcessID failed: %v", err)
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

func TestLinkProcessID_Duplicate_Error(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node1", Name: "test"}

	m.LinkProcessID(consumer, target)
	if err := m.LinkProcessID(consumer, target); err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist, got %v", err)
	}
}

func TestLinkProcessID_NetworkError_Rollback(t *testing.T) {
	m, core := newManagerWithMock("node1")
	core.connectionError = gen.ErrNoConnection

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node2", Name: "test"}

	err := m.LinkProcessID(consumer, target)
	if err != gen.ErrNoConnection {
		t.Errorf("Expected ErrNoConnection, got %v", err)
	}
	if m.hasLinkRelation(consumer, target) {
		t.Error("Link should be rolled back")
	}
}

func TestLinkProcessID_RemoteCorePID_Duplicate_Ignored(t *testing.T) {
	m, _ := newManagerWithMock("node2")
	localTarget := gen.ProcessID{Node: "node2", Name: "test"}
	remoteCorePID := gen.PID{Node: "node1", ID: 1}

	if err := m.LinkProcessID(remoteCorePID, localTarget); err != nil {
		t.Fatalf("First link failed: %v", err)
	}
	if err := m.LinkProcessID(remoteCorePID, localTarget); err != nil {
		t.Errorf("Duplicate should be ignored, got %v", err)
	}
	if m.totalLinks() != 1 {
		t.Errorf("Expected 1 link, got %d", m.totalLinks())
	}
}

func TestUnlinkProcessID_Local(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node1", Name: "test"}

	m.LinkProcessID(consumer, target)
	if err := m.UnlinkProcessID(consumer, target); err != nil {
		t.Fatalf("UnlinkProcessID failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) {
		t.Error("Link should be removed")
	}
}

func TestUnlinkProcessID_NotLast(t *testing.T) {
	m, core := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.ProcessID{Node: "node2", Name: "test"}

	m.LinkProcessID(c1, target)
	m.LinkProcessID(c2, target)
	core.resetSentUnlinks()

	m.UnlinkProcessID(c1, target)
	if m.hasLinkRelation(c1, target) {
		t.Error("c1 link should be removed")
	}
	if m.hasLinkRelation(c2, target) == false {
		t.Error("c2 link should still exist")
	}
	if core.countSentUnlinks() != 0 {
		t.Errorf("NO UnlinkProcessID should be sent, got %d", core.countSentUnlinks())
	}
}

func TestUnlinkProcessID_Last_SendsUnlink(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node2", Name: "test"}

	m.LinkProcessID(consumer, target)
	core.resetSentUnlinks()

	if err := m.UnlinkProcessID(consumer, target); err != nil {
		t.Fatalf("UnlinkProcessID failed: %v", err)
	}
	if m.hasLinkRelation(consumer, target) {
		t.Error("Link should be removed")
	}
	if core.countSentUnlinks() != 1 {
		t.Errorf("Expected 1 wire UnlinkProcessID, got %d", core.countSentUnlinks())
	}
}

func TestUnlinkProcessID_NonExistent_IsIdempotent(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node1", Name: "test"}

	if err := m.UnlinkProcessID(consumer, target); err != nil {
		t.Fatalf("UnlinkProcessID non-existent should be idempotent, got %v", err)
	}
}

func TestMonitorProcessID_Local(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node1", Name: "test"}

	if err := m.MonitorProcessID(consumer, target); err != nil {
		t.Fatalf("MonitorProcessID failed: %v", err)
	}
	if m.hasMonitorRelation(consumer, target) == false {
		t.Error("Monitor should be stored")
	}
	if core.countSentMonitors() != 0 {
		t.Error("No network for local")
	}
}

func TestMonitorProcessID_Local_EmptyNode(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Name: "test"}

	if err := m.MonitorProcessID(consumer, target); err != nil {
		t.Fatalf("MonitorProcessID failed: %v", err)
	}
	if m.hasMonitorRelation(consumer, target) == false {
		t.Error("Monitor should be stored")
	}
	if core.countSentMonitors() != 0 {
		t.Error("No network for local (empty node)")
	}
}

func TestMonitorProcessID_Remote_First(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node2", Name: "test"}

	if err := m.MonitorProcessID(consumer, target); err != nil {
		t.Fatalf("MonitorProcessID failed: %v", err)
	}
	if m.hasMonitorRelation(consumer, target) == false {
		t.Error("Monitor should be stored")
	}
	if core.countSentMonitors() != 1 {
		t.Fatalf("Expected 1 wire MonitorProcessID, got %d", core.countSentMonitors())
	}
}

func TestMonitorProcessID_Remote_Second(t *testing.T) {
	m, core := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.ProcessID{Node: "node2", Name: "test"}

	m.MonitorProcessID(c1, target)
	core.resetSentMonitors()

	if err := m.MonitorProcessID(c2, target); err != nil {
		t.Fatalf("Second MonitorProcessID failed: %v", err)
	}
	if core.countSentMonitors() != 0 {
		t.Errorf("Second subscriber should NOT send network, got %d", core.countSentMonitors())
	}
	if m.totalMonitors() != 2 {
		t.Errorf("Expected 2 monitor relations, got %d", m.totalMonitors())
	}
}

func TestMonitorProcessID_Duplicate_Error(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node1", Name: "test"}

	m.MonitorProcessID(consumer, target)
	if err := m.MonitorProcessID(consumer, target); err != gen.ErrTargetExist {
		t.Errorf("Expected ErrTargetExist, got %v", err)
	}
}

func TestMonitorProcessID_NetworkError_Rollback(t *testing.T) {
	m, core := newManagerWithMock("node1")
	core.connectionError = gen.ErrNoConnection

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node2", Name: "test"}

	err := m.MonitorProcessID(consumer, target)
	if err != gen.ErrNoConnection {
		t.Errorf("Expected ErrNoConnection, got %v", err)
	}
	if m.hasMonitorRelation(consumer, target) {
		t.Error("Monitor should be rolled back")
	}
}

func TestMonitorProcessID_RemoteCorePID_Duplicate_Ignored(t *testing.T) {
	m, _ := newManagerWithMock("node2")
	localTarget := gen.ProcessID{Node: "node2", Name: "test"}
	remoteCorePID := gen.PID{Node: "node1", ID: 1}

	if err := m.MonitorProcessID(remoteCorePID, localTarget); err != nil {
		t.Fatalf("First monitor failed: %v", err)
	}
	if err := m.MonitorProcessID(remoteCorePID, localTarget); err != nil {
		t.Errorf("Duplicate should be ignored, got %v", err)
	}
	if m.totalMonitors() != 1 {
		t.Errorf("Expected 1 monitor, got %d", m.totalMonitors())
	}
}

func TestDemonitorProcessID_Local(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node1", Name: "test"}

	m.MonitorProcessID(consumer, target)
	if err := m.DemonitorProcessID(consumer, target); err != nil {
		t.Fatalf("DemonitorProcessID failed: %v", err)
	}
	if m.hasMonitorRelation(consumer, target) {
		t.Error("Monitor should be removed")
	}
}

func TestDemonitorProcessID_NotLast(t *testing.T) {
	m, core := newManagerWithMock("node1")
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	target := gen.ProcessID{Node: "node2", Name: "test"}

	m.MonitorProcessID(c1, target)
	m.MonitorProcessID(c2, target)
	core.resetSentDemonitors()

	m.DemonitorProcessID(c1, target)
	if m.hasMonitorRelation(c1, target) {
		t.Error("c1 monitor should be removed")
	}
	if m.hasMonitorRelation(c2, target) == false {
		t.Error("c2 monitor should still exist")
	}
	if core.countSentDemonitors() != 0 {
		t.Errorf("NO DemonitorProcessID should be sent, got %d", core.countSentDemonitors())
	}
}

func TestDemonitorProcessID_Last_SendsDemonitor(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node2", Name: "test"}

	m.MonitorProcessID(consumer, target)
	core.resetSentDemonitors()

	if err := m.DemonitorProcessID(consumer, target); err != nil {
		t.Fatalf("DemonitorProcessID failed: %v", err)
	}
	if m.hasMonitorRelation(consumer, target) {
		t.Error("Monitor should be removed")
	}
	if core.countSentDemonitors() != 1 {
		t.Errorf("Expected 1 wire DemonitorProcessID, got %d", core.countSentDemonitors())
	}
}

func TestDemonitorProcessID_NonExistent_IsIdempotent(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "node1", Name: "test"}

	if err := m.DemonitorProcessID(consumer, target); err != nil {
		t.Fatalf("DemonitorProcessID non-existent should be idempotent, got %v", err)
	}
}
