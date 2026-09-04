package local

import (
	"errors"
	"testing"

	"ergo.services/ergo/app/system/inspect"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/stage"
)

func lookup(t *testing.T, n *stage.Node, request inspect.RequestGetProcessLookup) inspect.ResponseGetProcessLookup {
	t.Helper()

	target := gen.ProcessID{Name: inspect.Name, Node: n.Name()}
	result, err := n.Native().Call(target, request)
	if err != nil {
		t.Fatalf("lookup request: %s", err)
	}
	response, ok := result.(inspect.ResponseGetProcessLookup)
	if ok == false {
		t.Fatalf("unexpected response %T", result)
	}
	return response
}

// TestSystemProcessLookup: a caller that only has a registered name gets the PID
// every action needs, and a caller that only has a PID gets the name it sees in
// listings. Both directions report the state in the same round trip.
func TestSystemProcessLookup(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	pid, err := n.Native().SpawnRegister("named", factoryEcho, gen.ProcessOptions{})
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}

	byName := lookup(t, n, inspect.RequestGetProcessLookup{Name: "named"})
	if byName.Error != nil {
		t.Fatalf("lookup by name: %s", byName.Error)
	}
	if byName.PID != pid {
		t.Errorf("resolved to %s, expected %s", byName.PID, pid)
	}
	if byName.Name != "named" {
		t.Errorf("name came back as %q", byName.Name)
	}
	if byName.State != gen.ProcessStateSleep && byName.State != gen.ProcessStateRunning {
		t.Errorf("unexpected state %s for a live process", byName.State)
	}

	byPID := lookup(t, n, inspect.RequestGetProcessLookup{PID: pid})
	if byPID.Error != nil {
		t.Fatalf("lookup by pid: %s", byPID.Error)
	}
	if byPID.Name != "named" {
		t.Errorf("reverse lookup gave %q, expected \"named\"", byPID.Name)
	}
}

// TestSystemProcessLookupEdges: an unregistered process is not a failure, while
// an unknown name and an empty request are.
func TestSystemProcessLookupEdges(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	anonymous, err := n.Native().Spawn(factoryEcho, gen.ProcessOptions{})
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}

	// a live process without a registered name resolves, it just has no name
	response := lookup(t, n, inspect.RequestGetProcessLookup{PID: anonymous})
	if response.Error != nil {
		t.Errorf("an unregistered process reported an error: %s", response.Error)
	}
	if response.Name != "" {
		t.Errorf("name came back as %q for an unregistered process", response.Name)
	}
	if response.PID != anonymous {
		t.Errorf("pid came back as %s, expected %s", response.PID, anonymous)
	}

	if response := lookup(t, n, inspect.RequestGetProcessLookup{Name: "nobody"}); response.Error == nil {
		t.Error("an unknown name resolved without an error")
	}

	empty := lookup(t, n, inspect.RequestGetProcessLookup{})
	if errors.Is(empty.Error, gen.ErrIncorrect) == false {
		t.Errorf("an empty request answered %v, expected ErrIncorrect", empty.Error)
	}
}
