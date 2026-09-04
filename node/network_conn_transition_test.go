package node

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

func TestNetworkRegisterConnectionTransition(t *testing.T) {
	n := &network{node: &node{name: "n@localhost", log: createLog(gen.LogLevelDisabled, nil)}}
	name := gen.Atom("peer@localhost")
	c1 := mock.NewConnection()
	c2 := mock.NewConnection()

	// first registration: a real not-routed -> routed transition
	n.registerConnection(name, c1)
	if got := n.connectionsEstablished.Load(); got != 1 {
		t.Fatalf("established after first register = %d, want 1", got)
	}

	// takeover: overwrite the same name -> repoint only, no second up/established
	n.registerConnection(name, c2)
	if got := n.connectionsEstablished.Load(); got != 1 {
		t.Fatalf("established after takeover register = %d, want 1 (takeover must not double-count)", got)
	}
	if cur, ok := n.connections.Load(name); ok == false || cur.(*mock.Connection) != c2 {
		t.Fatal("routing must point at the taking-over connection")
	}

	// the superseded connection tears down: it no longer owns the name (routed=false), so it
	// must not count a loss or fire RouteNodeDown
	n.unregisterConnection(name, c1, "", nil)
	if got := n.connectionsLost.Load(); got != 0 {
		t.Fatalf("lost after superseded teardown = %d, want 0 (superseded must not count)", got)
	}
}
