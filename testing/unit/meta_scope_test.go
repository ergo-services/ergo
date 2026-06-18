package unit_test

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

// msParent sends to "x" in HandleMessage and records the error.
type msParent struct {
	act.Actor
	sendErr error
}

func (a *msParent) HandleMessage(from gen.PID, message any) error {
	a.sendErr = a.Send("x", "p")
	return nil
}
func factoryMsParent() gen.ProcessBehavior { return &msParent{} }

// msMeta sends to "x" in its Init and records the error.
type msMeta struct {
	mp      gen.MetaProcess
	initErr error
}

func (m *msMeta) Init(p gen.MetaProcess) error {
	m.mp = p
	m.initErr = p.Send("x", "init")
	return nil
}
func (m *msMeta) Start() error                                  { return nil }
func (m *msMeta) HandleMessage(from gen.PID, message any) error { return nil }
func (m *msMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}
func (m *msMeta) Terminate(reason error)                                       {}
func (m *msMeta) HandleInspect(from gen.PID, item ...string) map[string]string { return nil }

// A meta's own stub, set on a prepared MetaSubject before Run, applies to the
// meta's Init-time egress.
func TestMetaOwnStubAppliesToInit(t *testing.T) {
	n := unit.Node(t, "unit@localhost", gen.NodeOptions{})
	sub, err := n.Spawn(factoryMsParent, gen.ProcessOptions{})
	check.NoError(t, err)

	mb := &msMeta{}
	ms := sub.PrepareMeta(mb, gen.MetaOptions{})
	ms.OnSend("x").Fail(gen.ErrProcessMailboxFull)
	check.NoError(t, ms.Run())
	check.ErrorIs(t, mb.initErr, gen.ErrProcessMailboxFull)
}

// A stub on the parent actor does not leak into the meta's scope.
func TestParentStubDoesNotLeakToMeta(t *testing.T) {
	n := unit.Node(t, "unit@localhost", gen.NodeOptions{})
	sub, err := n.Spawn(factoryMsParent, gen.ProcessOptions{})
	check.NoError(t, err)
	sub.OnSend("x").Fail(gen.ErrProcessMailboxFull) // parent scope only

	mb := &msMeta{}
	ms := sub.PrepareMeta(mb, gen.MetaOptions{})
	check.NoError(t, ms.Run())
	check.NoError(t, mb.initErr) // meta Init send stays permissive
}

// A stub on the meta does not leak into the parent's scope.
func TestMetaStubDoesNotLeakToParent(t *testing.T) {
	n := unit.Node(t, "unit@localhost", gen.NodeOptions{})
	sub, err := n.Spawn(factoryMsParent, gen.ProcessOptions{})
	check.NoError(t, err)

	ms := sub.PrepareMeta(&msMeta{}, gen.MetaOptions{})
	ms.OnSend("x").Fail(gen.ErrProcessMailboxFull) // meta scope only
	check.NoError(t, ms.Run())

	sub.SendMessage(gen.PID{}, "go") // parent sends to "x"
	check.NoError(t, sub.Behavior().(*msParent).sendErr)
}
