package tm

import (
	"testing"

	"ergo.services/ergo/gen"
)

// A local target can terminate between the caller's liveness check and Register.
// Register must then either hand the relation to the target's death fan-out (nil,
// exactly one signal) or take it back (error, no signal), never leave it unclaimed.

func TestMonitorPID_Local_TargetGone(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}
	core.alive = func(any) bool { return false }

	if err := m.MonitorPID(consumer, target); err != gen.ErrProcessTerminated {
		t.Fatalf("expected ErrProcessTerminated, got %v", err)
	}
	if m.hasMonitorRelation(consumer, target) {
		t.Error("relation must be taken back")
	}
	if m.getTargetEntry(target) != nil {
		t.Error("entry of a dead PID must be dropped")
	}
	if m.totalMonitors() != 0 || len(m.MonitorsFor(consumer)) != 0 {
		t.Errorf("monitors=%d reverse=%v, want none", m.totalMonitors(), m.MonitorsFor(consumer))
	}
	if core.countSentDowns() != 0 {
		t.Errorf("expected no down, got %d", core.countSentDowns())
	}
}

func TestLinkPID_Local_TargetGone(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}
	core.alive = func(any) bool { return false }

	if err := m.LinkPID(consumer, target); err != gen.ErrProcessTerminated {
		t.Fatalf("expected ErrProcessTerminated, got %v", err)
	}
	if m.hasLinkRelation(consumer, target) {
		t.Error("relation must be taken back")
	}
	if m.getTargetEntry(target) != nil {
		t.Error("entry of a dead PID must be dropped")
	}
	if m.totalLinks() != 0 || len(m.LinksFor(consumer)) != 0 {
		t.Errorf("links=%d reverse=%v, want none", m.totalLinks(), m.LinksFor(consumer))
	}
	if core.countSentExits() != 0 {
		t.Errorf("expected no exit, got %d", core.countSentExits())
	}
}

// The target's whole termination runs after Register: the death fan-out claims the
// relation, so MonitorPID succeeds and the down is the only signal.
func TestMonitorPID_Local_TargetDiesDuringRegister(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}
	core.alive = func(any) bool {
		m.TerminatedTargetPID(target, gen.TerminateReasonNormal)
		return false
	}

	if err := m.MonitorPID(consumer, target); err != nil {
		t.Fatalf("expected nil, the down is on its way: %v", err)
	}
	if core.countSentDowns() != 1 {
		t.Fatalf("expected exactly one down, got %d", core.countSentDowns())
	}
	if down, _ := core.getFirstSentDown(); down.to != consumer {
		t.Errorf("down sent to %v, want %v", down.to, consumer)
	}
	if m.totalMonitors() != 0 || m.getTargetEntry(target) != nil {
		t.Errorf("monitors=%d entry=%v, want none", m.totalMonitors(), m.getTargetEntry(target))
	}
}

// The termination detached the entry and finished its walk before the relation
// landed in it: nobody will walk that entry again, so it must be taken back.
func TestMonitorPID_Local_EntryDetached(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}
	core.alive = func(any) bool {
		m.storage.targets.Delete(target)
		return false
	}

	if err := m.MonitorPID(consumer, target); err != gen.ErrProcessTerminated {
		t.Fatalf("expected ErrProcessTerminated, got %v", err)
	}
	if m.totalMonitors() != 0 || len(m.MonitorsFor(consumer)) != 0 {
		t.Errorf("monitors=%d reverse=%v, want none", m.totalMonitors(), m.MonitorsFor(consumer))
	}
	if core.countSentDowns() != 0 {
		t.Errorf("expected no down, got %d", core.countSentDowns())
	}
}

// The name died with the relation's entry detached and is already registered again:
// the name is alive, but the relation sits in an entry nobody will walk.
func TestMonitorProcessID_Local_NameRegisteredAgain(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Name: "worker", Node: "node1"}
	core.alive = func(any) bool {
		m.storage.targets.Delete(target)
		return true
	}

	if err := m.MonitorProcessID(consumer, target); err != gen.ErrProcessUnknown {
		t.Fatalf("expected ErrProcessUnknown, got %v", err)
	}
	if m.totalMonitors() != 0 || len(m.MonitorsFor(consumer)) != 0 {
		t.Errorf("monitors=%d reverse=%v, want none", m.totalMonitors(), m.MonitorsFor(consumer))
	}
	if core.countSentDowns() != 0 {
		t.Errorf("expected no down, got %d", core.countSentDowns())
	}
}

func TestMonitorProcessID_Local_TargetGone(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Name: "worker", Node: "node1"}
	core.alive = func(any) bool { return false }

	if err := m.MonitorProcessID(consumer, target); err != gen.ErrProcessUnknown {
		t.Fatalf("expected ErrProcessUnknown, got %v", err)
	}
	if m.hasMonitorRelation(consumer, target) || m.totalMonitors() != 0 {
		t.Errorf("relation must be taken back, monitors=%d", m.totalMonitors())
	}
	if core.countSentDowns() != 0 {
		t.Errorf("expected no down, got %d", core.countSentDowns())
	}
}

func TestMonitorAlias_Local_TargetGone(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	core.alive = func(any) bool { return false }

	if err := m.MonitorAlias(consumer, target); err != gen.ErrAliasUnknown {
		t.Fatalf("expected ErrAliasUnknown, got %v", err)
	}
	if m.hasMonitorRelation(consumer, target) {
		t.Error("relation must be taken back")
	}
	if m.getTargetEntry(target) != nil {
		t.Error("entry of a deleted alias must be dropped")
	}
	if m.totalMonitors() != 0 || core.countSentDowns() != 0 {
		t.Errorf("monitors=%d downs=%d, want none", m.totalMonitors(), core.countSentDowns())
	}
}

func TestLinkProcessID_Local_TargetGone(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Name: "worker", Node: "node1"}
	core.alive = func(any) bool { return false }

	if err := m.LinkProcessID(consumer, target); err != gen.ErrProcessUnknown {
		t.Fatalf("expected ErrProcessUnknown, got %v", err)
	}
	if m.hasLinkRelation(consumer, target) || m.totalLinks() != 0 {
		t.Errorf("relation must be taken back, links=%d", m.totalLinks())
	}
	if core.countSentExits() != 0 {
		t.Errorf("expected no exit, got %d", core.countSentExits())
	}
}

func TestLinkProcessID_Local_NameRegisteredAgain(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Name: "worker", Node: "node1"}
	core.alive = func(any) bool {
		m.storage.targets.Delete(target)
		return true
	}

	if err := m.LinkProcessID(consumer, target); err != gen.ErrProcessUnknown {
		t.Fatalf("expected ErrProcessUnknown, got %v", err)
	}
	if m.totalLinks() != 0 || len(m.LinksFor(consumer)) != 0 {
		t.Errorf("links=%d reverse=%v, want none", m.totalLinks(), m.LinksFor(consumer))
	}
	if core.countSentExits() != 0 {
		t.Errorf("expected no exit, got %d", core.countSentExits())
	}
}

func TestLinkAlias_Local_TargetGone(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}
	core.alive = func(any) bool { return false }

	if err := m.LinkAlias(consumer, target); err != gen.ErrAliasUnknown {
		t.Fatalf("expected ErrAliasUnknown, got %v", err)
	}
	if m.hasLinkRelation(consumer, target) {
		t.Error("relation must be taken back")
	}
	if m.getTargetEntry(target) != nil {
		t.Error("entry of a deleted alias must be dropped")
	}
	if m.totalLinks() != 0 || core.countSentExits() != 0 {
		t.Errorf("links=%d exits=%d, want none", m.totalLinks(), core.countSentExits())
	}
}

func TestMonitorNode_TargetGone(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")
	core.alive = func(any) bool { return false }

	if err := m.MonitorNode(consumer, target); err != gen.ErrNoConnection {
		t.Fatalf("expected ErrNoConnection, got %v", err)
	}
	if m.hasMonitorRelation(consumer, target) || m.totalMonitors() != 0 {
		t.Errorf("relation must be taken back, monitors=%d", m.totalMonitors())
	}
	if core.countSentDowns() != 0 {
		t.Errorf("expected no down, got %d", core.countSentDowns())
	}
}

func TestLinkNode_TargetGone(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")
	core.alive = func(any) bool { return false }

	if err := m.LinkNode(consumer, target); err != gen.ErrNoConnection {
		t.Fatalf("expected ErrNoConnection, got %v", err)
	}
	if m.hasLinkRelation(consumer, target) || m.totalLinks() != 0 {
		t.Errorf("relation must be taken back, links=%d", m.totalLinks())
	}
	if core.countSentExits() != 0 {
		t.Errorf("expected no exit, got %d", core.countSentExits())
	}
}

// The disconnect runs after Register: its fan-out claims the relation.
func TestMonitorNode_DisconnectDuringRegister(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")
	core.alive = func(any) bool {
		m.TerminatedTargetNode(target, gen.ErrNoConnection)
		return false
	}

	if err := m.MonitorNode(consumer, target); err != nil {
		t.Fatalf("expected nil, the down is on its way: %v", err)
	}
	if core.countSentDowns() != 1 {
		t.Fatalf("expected exactly one down, got %d", core.countSentDowns())
	}
	if m.totalMonitors() != 0 || m.getTargetEntry(target) != nil {
		t.Errorf("monitors=%d entry=%v, want none", m.totalMonitors(), m.getTargetEntry(target))
	}
}

// The node disconnected with the relation's entry detached and is connected again.
func TestMonitorNode_ConnectedAgain(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.Atom("node2")
	core.alive = func(any) bool {
		m.storage.targets.Delete(target)
		return true
	}

	if err := m.MonitorNode(consumer, target); err != gen.ErrNoConnection {
		t.Fatalf("expected ErrNoConnection, got %v", err)
	}
	if m.totalMonitors() != 0 || len(m.MonitorsFor(consumer)) != 0 {
		t.Errorf("monitors=%d reverse=%v, want none", m.totalMonitors(), m.MonitorsFor(consumer))
	}
	if core.countSentDowns() != 0 {
		t.Errorf("expected no down, got %d", core.countSentDowns())
	}
}
