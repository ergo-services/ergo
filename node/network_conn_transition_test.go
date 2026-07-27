package node

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

// TestNetworkRegisterConnectionTransition guards #288: registerConnection announces the
// peer (established++/RouteNodeUp) only on a real not-routed->routed transition, and
// unregisterConnection counts the loss / signals node-down only for the connection that
// still owns the routing entry. So a simultaneous-connect takeover (a second
// registerConnection overwriting the same name) neither re-announces nor double-counts,
// and the superseded connection's teardown is suppressed.
//
// The node is not running (creation 0), so RouteNodeUp's SendEvent returns early and the
// superseded (routed=false) unregister skips RouteNodeDown - the test never touches the
// node's event bus or target manager.
func TestNetworkRegisterConnectionTransition(t *testing.T) {
	n := &network{node: &node{name: "n@localhost", log: &log{level: gen.LogLevelDisabled}}}
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
