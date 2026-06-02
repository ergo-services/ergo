package tm

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

func TestWireLinkPIDFirstConsumerCallsWireOnce(t *testing.T) {
	m, core := newManager()
	remoteNode := gen.Atom("remote@local")
	conn := core.registerConn(remoteNode)
	target := gen.PID{Node: remoteNode, ID: 100, Creation: 1}

	a := localPID(10)
	b := localPID(11)
	if err := m.LinkPID(a, target); err != nil {
		t.Fatal(err)
	}
	if err := m.LinkPID(b, target); err != nil {
		t.Fatal(err)
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.linkPIDs) != 1 {
		t.Fatalf("expected 1 wire LinkPID, got %d", len(conn.linkPIDs))
	}
	if conn.linkPIDs[0] != target {
		t.Fatalf("wire-link target mismatch: %v", conn.linkPIDs[0])
	}
}

func TestWireUnlinkPIDLastConsumerCallsWire(t *testing.T) {
	m, core := newManager()
	remoteNode := gen.Atom("remote@local")
	conn := core.registerConn(remoteNode)
	target := gen.PID{Node: remoteNode, ID: 100, Creation: 1}

	a := localPID(10)
	b := localPID(11)
	m.LinkPID(a, target)
	m.LinkPID(b, target)

	// First Unlink: still have b locally, no wire-Unlink.
	m.UnlinkPID(a, target)
	conn.mu.Lock()
	if len(conn.unlinkPIDs) != 0 {
		conn.mu.Unlock()
		t.Fatalf("wire UnlinkPID should not fire while local consumers remain, got %d", len(conn.unlinkPIDs))
	}
	conn.mu.Unlock()

	// Last Unlink: wire-Unlink should fire.
	m.UnlinkPID(b, target)
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.unlinkPIDs) != 1 {
		t.Fatalf("expected 1 wire UnlinkPID after last local consumer, got %d", len(conn.unlinkPIDs))
	}
}

func TestWireMonitorAndDemonitorPID(t *testing.T) {
	m, core := newManager()
	remoteNode := gen.Atom("remote@local")
	conn := core.registerConn(remoteNode)
	target := gen.PID{Node: remoteNode, ID: 100, Creation: 1}

	a := localPID(10)
	m.MonitorPID(a, target)

	conn.mu.Lock()
	if len(conn.monitorPIDs) != 1 {
		conn.mu.Unlock()
		t.Fatalf("expected 1 wire MonitorPID, got %d", len(conn.monitorPIDs))
	}
	conn.mu.Unlock()

	m.DemonitorPID(a, target)
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.demonitorPIDs) != 1 {
		t.Fatalf("expected 1 wire DemonitorPID, got %d", len(conn.demonitorPIDs))
	}
}

func TestWireLinkPIDFailureRollsBack(t *testing.T) {
	m, _ := newManager()
	// Do NOT register a connection. GetConnection returns ErrNoConnection.
	remoteNode := gen.Atom("missing@local")
	target := gen.PID{Node: remoteNode, ID: 100, Creation: 1}

	a := localPID(10)
	err := m.LinkPID(a, target)
	if err != gen.ErrNoConnection {
		t.Fatalf("expected ErrNoConnection, got %v", err)
	}
	if m.HasLink(a, target) == true {
		t.Fatal("LinkPID should have rolled back the local registration on wire failure")
	}
}

func TestWireLinkEventNonBufferedPiggybacks(t *testing.T) {
	m, core := newManager()
	remoteNode := gen.Atom("remote@local")
	conn := core.registerConn(remoteNode)
	conn.linkEventBuffer = nil // signal non-buffered event
	event := gen.Event{Node: remoteNode, Name: "evt"}

	m.LinkEvent(localPID(10), event)
	m.LinkEvent(localPID(11), event)
	m.LinkEvent(localPID(12), event)

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.linkEvents) != 1 {
		t.Fatalf("non-buffered event: expected 1 wire-link, got %d", len(conn.linkEvents))
	}
}

func TestWireLinkEventBufferedFiresWireEveryTime(t *testing.T) {
	m, core := newManager()
	remoteNode := gen.Atom("remote@local")
	conn := core.registerConn(remoteNode)
	// Non-nil buffer signals "buffered remote event".
	conn.linkEventBuffer = []gen.MessageEvent{{Timestamp: time.Now().UnixNano()}}
	event := gen.Event{Node: remoteNode, Name: "evt"}

	for i := 0; i < 5; i++ {
		buf, err := m.LinkEvent(localPID(uint64(10+i)), event)
		if err != nil {
			t.Fatal(err)
		}
		if len(buf) != 1 {
			t.Fatalf("LinkEvent %d returned buffer of len %d, want 1", i, len(buf))
		}
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.linkEvents) != 5 {
		t.Fatalf("buffered event: expected 5 wire-link calls, got %d", len(conn.linkEvents))
	}
}

func TestWireUnlinkEventLastConsumerCallsWire(t *testing.T) {
	m, core := newManager()
	remoteNode := gen.Atom("remote@local")
	conn := core.registerConn(remoteNode)
	conn.linkEventBuffer = nil
	event := gen.Event{Node: remoteNode, Name: "evt"}

	a := localPID(10)
	b := localPID(11)
	m.LinkEvent(a, event)
	m.LinkEvent(b, event)

	m.UnlinkEvent(a, event)
	conn.mu.Lock()
	if len(conn.unlinkEvents) != 0 {
		conn.mu.Unlock()
		t.Fatalf("UnlinkEvent should not fire while local consumers remain")
	}
	conn.mu.Unlock()

	m.UnlinkEvent(b, event)
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.unlinkEvents) != 1 {
		t.Fatalf("expected 1 wire UnlinkEvent after last consumer, got %d", len(conn.unlinkEvents))
	}
}

func TestWireTerminatedProcessSendsRemoteUnlink(t *testing.T) {
	m, core := newManager()
	remoteNode := gen.Atom("remote@local")
	conn := core.registerConn(remoteNode)
	target := gen.PID{Node: remoteNode, ID: 100, Creation: 1}

	consumer := localPID(10)
	m.LinkPID(consumer, target)

	m.TerminatedProcess(consumer, gen.TerminateReasonNormal)

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.unlinkPIDs) != 1 || conn.unlinkPIDs[0] != target {
		t.Fatalf("expected wire UnlinkPID after TerminatedProcess, got %+v", conn.unlinkPIDs)
	}
}

func TestWireResetAfterTerminatedTarget(t *testing.T) {
	m, core := newManager()
	remoteNode := gen.Atom("remote@local")
	conn := core.registerConn(remoteNode)
	target := gen.PID{Node: remoteNode, ID: 100, Creation: 1}

	consumer := localPID(10)
	m.LinkPID(consumer, target)

	// One wire-LinkPID happened.
	conn.mu.Lock()
	if len(conn.linkPIDs) != 1 {
		conn.mu.Unlock()
		t.Fatalf("setup: expected 1 wire-LinkPID")
	}
	conn.mu.Unlock()

	// Target died. Wire-state should reset.
	m.TerminatedTargetPID(target, gen.ErrNoConnection)

	// Re-register the same target. Wire-LinkPID should fire AGAIN.
	consumer2 := localPID(11)
	if err := m.LinkPID(consumer2, target); err != nil {
		t.Fatal(err)
	}
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.linkPIDs) != 2 {
		t.Fatalf("expected 2 wire-LinkPIDs after reset, got %d", len(conn.linkPIDs))
	}
}
