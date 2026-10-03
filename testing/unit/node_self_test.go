package unit_test

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

type nodeSelfActor struct {
	act.Actor

	linkErr    error
	monitorErr error
}

func factoryNodeSelfActor() gen.ProcessBehavior { return &nodeSelfActor{} }

func (a *nodeSelfActor) HandleMessage(from gen.PID, message any) error {
	node := message.(gen.Atom)
	a.linkErr = a.LinkNode(node)
	a.monitorErr = a.MonitorNode(node)
	return nil
}

func TestLinkMonitorOwnNodeRefused(t *testing.T) {
	a, err := unit.Spawn(t, factoryNodeSelfActor, gen.ProcessOptions{})
	check.NoError(t, err)

	own := a.Node().Name()
	a.SendMessage(gen.PID{}, own)

	actor := a.Behavior().(*nodeSelfActor)
	check.ErrorIs(t, actor.linkErr, gen.ErrNotAllowed)
	check.ErrorIs(t, actor.monitorErr, gen.ErrNotAllowed)
	a.ShouldLink().Target(own).ErrorIs(gen.ErrNotAllowed).Once().Assert()
	a.ShouldMonitor().Target(own).ErrorIs(gen.ErrNotAllowed).Once().Assert()

	other := gen.Atom("other@localhost")
	a.SendMessage(gen.PID{}, other)
	check.NoError(t, actor.linkErr)
	check.NoError(t, actor.monitorErr)
	a.ShouldLink().Target(other).Once().Assert()
	a.ShouldMonitor().Target(other).Once().Assert()
}
