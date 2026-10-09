package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// Depends makes the start order deterministic: a dependency starts first.
type soApp struct {
	app.Application
	name    gen.Atom
	depends []gen.Atom
}

func createSoApp(name gen.Atom, depends ...gen.Atom) gen.ApplicationBehavior {
	return &soApp{name: name, depends: depends}
}

func (a *soApp) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:    a.name,
		Mode:    gen.ApplicationModeTemporary,
		Depends: gen.ApplicationDepends{Applications: a.depends},
		Group: []gen.ApplicationMemberSpec{
			{Name: a.name + "_member", Factory: factorySoMember},
		},
	}, nil
}

func (a *soApp) Terminate(reason error) { teardownRecord("stop:" + string(a.name)) }

type soMember struct{ act.Actor }

func factorySoMember() gen.ProcessBehavior { return &soMember{} }

type soWatcher struct{ act.Actor }

func factorySoWatcher() gen.ProcessBehavior { return &soWatcher{} }

func (w *soWatcher) Init(args ...any) error {
	_, err := w.MonitorEvent(gen.Event{Name: gen.CoreEvent})
	return err
}

func (w *soWatcher) HandleEvent(event gen.MessageEvent) error {
	if _, ok := event.Message.(gen.MessageCoreNodeStopping); ok {
		teardownRecord("event:stopping")
	}
	return nil
}

func startSoApps(t *testing.T, n *stage.Node) {
	t.Helper()
	nn := n.Native()
	for _, a := range []gen.ApplicationBehavior{
		createSoApp("so_a"),
		createSoApp("so_b", "so_a"),
		createSoApp("so_c", "so_b"),
	} {
		name, err := nn.ApplicationLoad(a)
		check.NoError(t, err)
		_ = name
	}
	// starting the last one starts its dependencies first: so_a, so_b, so_c
	check.NoError(t, nn.ApplicationStart("so_c", gen.ApplicationOptions{}))
}

// applications are torn down latest-started first, so a dependency outlives its dependent
func TestNodeShutdownStopsApplicationsInReverseStartOrder(t *testing.T) {
	teardownCollected()

	s := stage.New(t)
	n := s.StartNode("n")
	startSoApps(t, n)

	n.Native().Stop()

	events := teardownCollected()
	want := []string{"stop:so_c", "stop:so_b", "stop:so_a"}
	if len(events) != len(want) {
		t.Fatalf("teardown events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("teardown order = %v, want %v", events, want)
		}
	}
}

// the announcement reaches a subscriber before any application is torn down
func TestNodeShutdownAnnouncesBeforeStoppingApplications(t *testing.T) {
	teardownCollected()

	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{ShutdownDrain: 300 * time.Millisecond})
	n.Spawn(factorySoWatcher, gen.ProcessOptions{})
	startSoApps(t, n)

	n.Native().Stop()

	events := teardownCollected()
	announced := teardownIndex(events, "event:stopping")
	if announced < 0 {
		t.Fatalf("MessageCoreNodeStopping did not reach the subscriber: %v", events)
	}
	if announced != 0 {
		t.Fatalf("the announcement is not the first event: %v", events)
	}
}

// the drain is a real wait, and zero keeps the node as fast as it was
func TestNodeShutdownDrainDelaysTheTeardown(t *testing.T) {
	drain := 400 * time.Millisecond

	s := stage.New(t)

	slow := s.StartNode("slow", stage.NodeOptions{ShutdownDrain: drain})
	startSoApps(t, slow)
	start := time.Now()
	slow.Native().Stop()
	if elapsed := time.Since(start); elapsed < drain {
		t.Fatalf("stop with a %s drain took %s, want at least the drain", drain, elapsed)
	}

	fast := s.StartNode("fast")
	startSoApps(t, fast)
	start = time.Now()
	fast.Native().Stop()
	if elapsed := time.Since(start); elapsed >= drain {
		t.Fatalf("stop without a drain took %s, want well under %s", elapsed, drain)
	}
}

// StopForce neither announces nor waits
func TestNodeShutdownForceSkipsTheDrain(t *testing.T) {
	teardownCollected()
	drain := time.Second

	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{ShutdownDrain: drain})
	n.Spawn(factorySoWatcher, gen.ProcessOptions{})
	startSoApps(t, n)

	start := time.Now()
	n.Native().StopForce()
	if elapsed := time.Since(start); elapsed >= drain {
		t.Fatalf("StopForce took %s, want no drain", elapsed)
	}
	if i := teardownIndex(teardownCollected(), "event:stopping"); i >= 0 {
		t.Fatalf("StopForce announced the shutdown")
	}
}

// a force call while the node is draining ends the wait
func TestNodeShutdownForceCutsTheDrain(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{ShutdownDrain: 10 * time.Second})
	startSoApps(t, n)

	stopped := make(chan struct{})
	go func() {
		n.Native().Stop()
		close(stopped)
	}()

	time.Sleep(200 * time.Millisecond)
	n.Native().StopForce()

	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("the drain was not cut by StopForce")
	}
}
