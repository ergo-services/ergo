package node

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
)

type enFakeProc struct{}

func (enFakeProc) ProcessInit(gen.Process, ...any) error { return nil }
func (enFakeProc) ProcessRun() error                     { return nil }
func (enFakeProc) ProcessTerminate(error)                {}
func (enFakeProc) ProcessKind() gen.ProcessKind          { return gen.ProcessKindCustom }

type enFakeProcA struct{ enFakeProc }
type enFakeProcB struct{ enFakeProc }

// Re-enabling EnableSpawn with the same factory updates the allow-list without
// error; a different factory for the same name is a conflict -> gen.ErrTaken.
func TestNetworkEnableSpawnReenableAndConflict(t *testing.T) {
	n := &network{}
	fa := func() gen.ProcessBehavior { return &enFakeProcA{} }
	fb := func() gen.ProcessBehavior { return &enFakeProcB{} }

	if err := n.EnableSpawn("svc", fa); err != nil {
		t.Fatalf("first EnableSpawn: %v", err)
	}
	if err := n.EnableSpawn("svc", fa, "node1@localhost"); err != nil {
		t.Fatalf("re-enable with same factory must not error, got: %v", err)
	}
	if err := n.EnableSpawn("svc", fb); errors.Is(err, gen.ErrTaken) == false {
		t.Fatalf("different factory must return gen.ErrTaken, got: %v", err)
	}
}

// Re-enabling EnableApplicationStart updates the allow-list without error
// (no factory, so no ErrTaken conflict is possible).
func TestNetworkEnableApplicationStartReenable(t *testing.T) {
	n := &network{}
	if err := n.EnableApplicationStart("app"); err != nil {
		t.Fatalf("first EnableApplicationStart: %v", err)
	}
	if err := n.EnableApplicationStart("app", "node1@localhost"); err != nil {
		t.Fatalf("re-enable must not error, got: %v", err)
	}
}
