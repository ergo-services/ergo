package unit_test

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

type prNoopActor struct{ act.Actor }

func factoryPrNoop() gen.ProcessBehavior { return &prNoopActor{} }

// prDepActor mirrors the radar case: Init makes a best-effort outbound Call to a
// dependency, tolerates ErrProcessUnknown, then spawns a child and records its arg.
type prDepActor struct {
	act.Actor
	arg      string
	gotResp  any
	callErr  error
	childPID gen.PID
	ready    bool
}

func (a *prDepActor) Init(args ...any) error {
	if len(args) > 0 {
		a.arg, _ = args[0].(string)
	}
	resp, err := a.Call("dep", "register")
	a.gotResp = resp
	a.callErr = err
	if err != nil && err != gen.ErrProcessUnknown {
		return err
	}
	pid, serr := a.Spawn(factoryPrNoop, gen.ProcessOptions{})
	if serr != nil {
		return serr
	}
	a.childPID = pid
	a.ready = true
	return nil
}

func factoryPrDep() gen.ProcessBehavior { return &prDepActor{} }

// Prepare returns a pre-Init Subject; egress stubbed on the process before Run.
func TestPrepareRunStubsBeforeInit(t *testing.T) {
	sub := unit.Prepare(t, factoryPrDep, gen.ProcessOptions{}, "ARG1")
	sub.OnCall("dep").Fail(gen.ErrProcessUnknown)
	stubbed := gen.PID{Node: "unit@localhost", ID: 7777, Creation: 1}
	sub.OnSpawn(factoryPrNoop).Return(stubbed)

	if err := sub.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	b := sub.Behavior().(*prDepActor)
	check.Equal(t, "ARG1", b.arg)
	check.ErrorIs(t, b.callErr, gen.ErrProcessUnknown)
	check.Equal(t, stubbed, b.childPID)
	check.True(t, b.ready)
}

// Node and process stub scopes are independent: a process stub answers the
// actor's own Call, a node stub answers the node's own Call, and neither leaks.
func TestNodeProcessStubsIsolated(t *testing.T) {
	sub := unit.Prepare(t, factoryPrDep, gen.ProcessOptions{})
	sub.OnCall("dep").Respond("PROC")        // process scope
	sub.Node().OnCall("dep").Respond("NODE") // node scope
	check.NoError(t, sub.Run())

	// the actor's Init Call resolved the process stub
	check.Equal(t, "PROC", sub.Behavior().(*prDepActor).gotResp)
	// the node's own Call resolves the node stub
	got, err := sub.Node().Call("dep", "register")
	check.NoError(t, err)
	check.Equal(t, "NODE", got)
}

// gen.Node's own Call is overridable via the node-level OnCall.
func TestNodeOwnCallOverridable(t *testing.T) {
	n := unit.StartNode(t, "unit@localhost", gen.NodeOptions{})
	n.OnCall("svc").Respond("OK")
	sub, err := n.Spawn(factoryPrNoop, gen.ProcessOptions{})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	got, err := sub.Node().Call("svc", "ping")
	check.NoError(t, err)
	check.Equal(t, "OK", got)
}

// prArgActor records its Init args without any outbound egress.
type prArgActor struct {
	act.Actor
	arg string
}

func (a *prArgActor) Init(args ...any) error {
	if len(args) > 0 {
		a.arg, _ = args[0].(string)
	}
	return nil
}
func factoryPrArg() gen.ProcessBehavior { return &prArgActor{} }

// Spawn one-liner still works after the Prepare+Run refactor and forwards args.
func TestSpawnStillWorksAndForwardsArgs(t *testing.T) {
	n := unit.StartNode(t, "unit@localhost", gen.NodeOptions{})
	sub, err := n.Spawn(factoryPrArg, gen.ProcessOptions{}, "ARG2")
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	check.Equal(t, "ARG2", sub.Behavior().(*prArgActor).arg)
}

// After Prepare+Run the subject drives deliveries normally.
func TestPrepareRunThenDeliver(t *testing.T) {
	n := unit.StartNode(t, "unit@localhost", gen.NodeOptions{})
	sub := n.Prepare(factoryPrNoop, gen.ProcessOptions{})
	if err := sub.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	sub.SendMessage(gen.PID{}, "hi")
	check.False(t, sub.Terminated())
}
