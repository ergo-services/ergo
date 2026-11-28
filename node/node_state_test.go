package node

// What the node does with its own state. What a process sees of it is checked
// through the harness, in testing/tests/local.

import (
	"testing"

	"ergo.services/ergo/gen"
)

// Only one caller runs the teardown, and a node still coming up is one of the
// states it runs from: StartNode calls StopForce when it fails partway.
func TestBeginStopAdmitsOneCallerFromEveryLiveState(t *testing.T) {
	for _, from := range []gen.NodeState{gen.NodeStateStarting, gen.NodeStateRunning} {
		t.Run(from.String(), func(t *testing.T) {
			n := &node{}
			n.state.Store(int32(from))

			if n.beginStop() == false {
				t.Fatalf("a node that is %s refused to begin stopping", from)
			}
			if got := n.State(); got != gen.NodeStateStopping {
				t.Fatalf("after beginning to stop the node is %s, want stopping", got)
			}
			if n.beginStop() {
				t.Fatal("a second caller was let into the teardown")
			}
		})
	}
}

// A node that is gone does not begin again.
func TestBeginStopRefusesWhatIsOver(t *testing.T) {
	n := &node{}
	n.state.Store(int32(gen.NodeStateStopped))

	if n.beginStop() {
		t.Fatal("a stopped node began stopping")
	}
}

// The zero value is no state at all, and a node that has not been built is not
// running: guards that bail on that are what keep a half built node out of the
// paths that would use it.
func TestZeroStateIsNotRunning(t *testing.T) {
	n := &node{}

	if n.isRunning() {
		t.Fatal("a node that was never started reports itself running")
	}
	if got := n.State().String(); got != "state#0" {
		t.Fatalf("the zero state is %q, want it to name no state", got)
	}
}
