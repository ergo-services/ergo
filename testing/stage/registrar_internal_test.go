package stage

import (
	"testing"

	"ergo.services/ergo/gen"
)

// The stage in-memory registrar enforces node-name uniqueness like the embedded
// one: a duplicate name is rejected with ErrTaken, empty routes with ErrIncorrect,
// and a name frees on del so the owner can be replaced after it leaves.
func TestMemStoreEnforcesUniqueness(t *testing.T) {
	s := newMemStore(false)
	routes := []gen.Route{{Host: "localhost", Port: 1}}

	if err := s.put("a@localhost", routes); err != nil {
		t.Fatalf("first put: got %v want nil", err)
	}
	if err := s.put("a@localhost", routes); err != gen.ErrTaken {
		t.Fatalf("duplicate name: got %v want ErrTaken", err)
	}
	if err := s.put("b@localhost", nil); err != gen.ErrIncorrect {
		t.Fatalf("empty routes: got %v want ErrIncorrect", err)
	}
	s.del("a@localhost")
	if err := s.put("a@localhost", routes); err != nil {
		t.Fatalf("put after del: got %v want nil", err)
	}
}

// In full mode the store tracks application routes (ResolveApplication) with their
// real state and frees them on del; minimal mode reports ErrUnsupported.
func TestMemStoreApplicationRoutes(t *testing.T) {
	min := &memRegistrar{store: newMemStore(false)}
	if _, err := min.ResolveApplication("app"); err != gen.ErrUnsupported {
		t.Fatalf("minimal ResolveApplication: got %v want ErrUnsupported", err)
	}
	if _, err := min.Event(); err != gen.ErrUnsupported {
		t.Fatalf("minimal Event: got %v want ErrUnsupported", err)
	}

	s := newMemStore(true)
	run := gen.ApplicationRoute{Name: "app", Node: "n1@h", State: gen.ApplicationStateRunning}
	s.putApp(run)
	got := s.resolveApp("app")
	if len(got) != 1 || got[0].Node != "n1@h" || got[0].State != gen.ApplicationStateRunning {
		t.Fatalf("resolveApp: got %+v want one running route on n1@h", got)
	}
	if _, ok := s.delApp("app", "n1@h"); ok == false {
		t.Fatal("delApp: expected the route to exist")
	}
	if got := s.resolveApp("app"); got != nil {
		t.Fatalf("resolveApp after del: got %+v want nil", got)
	}
}
