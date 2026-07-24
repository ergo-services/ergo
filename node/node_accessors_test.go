package node

import (
	"testing"
)

// Network() and Cron() are documented available in all states: they must return
// the always-allocated network/cron even when the node is not running, not nil
// (the network/cron methods themselves report ErrNetworkStopped when stopped).
func TestNodeNetworkCronAvailableWhenNotRunning(t *testing.T) {
	n := &node{} // creation == 0 -> not running
	n.network = &network{}
	n.cron = &cron{}

	if n.isRunning() {
		t.Fatal("precondition: fresh &node{} must not be running")
	}
	if n.Network() == nil {
		t.Fatal("Network() must be non-nil when the node is not running")
	}
	if n.Cron() == nil {
		t.Fatal("Cron() must be non-nil when the node is not running")
	}
}
