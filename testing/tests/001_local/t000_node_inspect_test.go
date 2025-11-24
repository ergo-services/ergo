package local

import (
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func TestNodeInspect(t *testing.T) {
	node, err := ergo.StartNode("test@localhost", gen.NodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Stop()

	// Test 1: Inspect supervisor process
	supPID, err := node.Spawn(factory_test_inspect_supervisor, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Kill(supPID)

	// Wait for children to start
	time.Sleep(100 * time.Millisecond)

	info, err := node.Inspect(supPID)
	if err != nil {
		t.Fatalf("Inspect failed: %s", err)
	}

	if info["type"] != "One For One" {
		t.Fatalf("expected type 'One For One', got %q", info["type"])
	}
	if info["children_total"] != "2" {
		t.Fatalf("expected 2 children, got %q", info["children_total"])
	}

	// Test 2: Inspect non-existent process
	fakePID := gen.PID{Node: node.Name(), ID: 99999, Creation: 1}
	_, err = node.Inspect(fakePID)
	if err != gen.ErrProcessUnknown {
		t.Fatalf("expected ErrProcessUnknown, got %v", err)
	}

	// Test 3: Inspect remote process (should fail)
	remotePID := gen.PID{Node: "remote@host", ID: 1, Creation: 1}
	_, err = node.Inspect(remotePID)
	if err != gen.ErrNotAllowed {
		t.Fatalf("expected ErrNotAllowed for remote process, got %v", err)
	}

	// Test 4: Inspect terminated process
	actorPID, err := node.Spawn(factory_test_inspect_actor, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	node.Kill(actorPID)
	// Give it time to terminate
	time.Sleep(100 * time.Millisecond)

	_, err = node.Inspect(actorPID)
	if err != gen.ErrProcessTerminated && err != gen.ErrProcessUnknown {
		t.Fatalf("expected ErrProcessTerminated or ErrProcessUnknown, got %v", err)
	}
}

func TestNodeInspectMeta(t *testing.T) {
	node, err := ergo.StartNode("test@localhost", gen.NodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Stop()

	// Spawn actor to create meta process
	actorPID, err := node.Spawn(factory_test_inspect_meta_actor, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Kill(actorPID)

	// Request to create meta process
	aliasReply := make(chan gen.Alias, 1)
	node.Send(actorPID, aliasReply)

	var alias gen.Alias
	select {
	case alias = <-aliasReply:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for meta creation")
	}

	// Test 1: Inspect meta process
	info, err := node.InspectMeta(alias)
	if err != nil {
		t.Fatalf("InspectMeta failed: %s", err)
	}

	// Check that info contains expected data
	if info["test_meta"] != "ok" {
		t.Fatalf("expected test_meta=ok, got %q", info["test_meta"])
	}

	// Test 2: Inspect non-existent meta
	fakeAlias := gen.Alias(gen.Ref{Node: node.Name(), ID: [3]uint64{99999, 0, 0}, Creation: 1})
	_, err = node.InspectMeta(fakeAlias)
	if err != gen.ErrMetaUnknown {
		t.Fatalf("expected ErrMetaUnknown, got %v", err)
	}

	// Test 3: Inspect remote meta (should fail)
	remoteAlias := gen.Alias(gen.Ref{Node: "remote@host", ID: [3]uint64{1, 0, 0}, Creation: 1})
	_, err = node.InspectMeta(remoteAlias)
	if err != gen.ErrNotAllowed {
		t.Fatalf("expected ErrNotAllowed for remote meta, got %v", err)
	}
}

// Test supervisor for inspection
func factory_test_inspect_supervisor() gen.ProcessBehavior {
	return &testInspectSupervisor{}
}

type testInspectSupervisor struct {
	act.Supervisor
}

func (s *testInspectSupervisor) Init(args ...any) (act.SupervisorSpec, error) {
	return act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{
				Name:    "child1",
				Factory: factory_test_inspect_actor,
			},
			{
				Name:    "child2",
				Factory: factory_test_inspect_actor,
			},
		},
		Restart: act.SupervisorRestart{
			Strategy: act.SupervisorStrategyTransient,
		},
	}, nil
}

// Test actor for inspection
func factory_test_inspect_actor() gen.ProcessBehavior {
	return &testInspectActor{}
}

type testInspectActor struct {
	act.Actor
}

// Actor that creates meta process
func factory_test_inspect_meta_actor() gen.ProcessBehavior {
	return &testInspectMetaActor{}
}

type testInspectMetaActor struct {
	act.Actor
}

func (a *testInspectMetaActor) HandleMessage(from gen.PID, message any) error {
	if reply, ok := message.(chan gen.Alias); ok {
		alias, err := a.SpawnMeta(factory_test_inspect_meta(), gen.MetaOptions{})
		if err != nil {
			close(reply)
			return err
		}
		reply <- alias
		close(reply)
	}
	return nil
}

// Test meta behavior for inspection
func factory_test_inspect_meta() gen.MetaBehavior {
	return &testInspectMeta{
		stop: make(chan struct{}),
	}
}

type testInspectMeta struct {
	gen.MetaProcess
	stop chan struct{}
}

func (tm *testInspectMeta) Init(meta gen.MetaProcess) error {
	tm.MetaProcess = meta
	return nil
}

func (tm *testInspectMeta) Start() error {
	<-tm.stop
	return nil
}

func (tm *testInspectMeta) HandleMessage(from gen.PID, message any) error {
	return nil
}

func (tm *testInspectMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (tm *testInspectMeta) Terminate(reason error) {
	close(tm.stop)
}

func (tm *testInspectMeta) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{
		"test_meta": "ok",
	}
}
