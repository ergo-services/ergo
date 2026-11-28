package local

// What a process sees of the state of its node, through the harness and a real
// node rather than a struct built by hand.

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

func TestNodeStateFollowsTheLifeOfTheNode(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("state")

	check.Equal(t, gen.NodeStateRunning, n.Native().State())
	check.True(t, n.Native().IsAlive(), "a running node is not alive")

	info, err := n.Native().Info()
	check.NoError(t, err)
	check.Equal(t, gen.NodeStateRunning, info.State)

	short, err := n.Native().ShortInfo()
	check.NoError(t, err)
	check.Equal(t, gen.NodeStateRunning, short.State)

	n.Native().Stop()

	check.Equal(t, gen.NodeStateStopped, n.Native().State())
	check.False(t, n.Native().IsAlive(), "a stopped node is alive")

	// stopped never travels in the info: by then there is no info to give
	_, err = n.Native().Info()
	check.ErrorIs(t, err, gen.ErrNodeTerminated)
}

func TestNodeStateNames(t *testing.T) {
	for state, want := range map[gen.NodeState]string{
		gen.NodeStateStarting: "starting",
		gen.NodeStateRunning:  "running",
		gen.NodeStateStopping: "stopping",
		gen.NodeStateStopped:  "stopped",
		gen.NodeState(99):     "state#99",
	} {
		check.Equal(t, want, state.String())
	}
}

// The node says it has come up and names what it started. A process that
// subscribes later reads it out of the buffer the subscription hands back, and
// one that comes later still asks Node.State instead.
func TestNodeStartedIsPublishedWithItsApplications(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("started", stage.NodeOptions{EnableSystemApp: true})

	pid := n.Spawn(factoryCoreWatcher, gen.ProcessOptions{})

	result, err := n.Call(pid, "started")
	check.NoError(t, err)

	started, ok := result.([]gen.Atom)
	check.True(t, ok, "the watcher answered %T", result)
	check.True(t, len(started) > 0, "the node published no applications: %v", started)
}

func factoryCoreWatcher() gen.ProcessBehavior { return &coreWatcher{} }

type coreWatcher struct {
	act.Actor

	started []gen.Atom
	seen    bool
}

func (w *coreWatcher) Init(args ...any) error {
	// a subscription hands back what the bus still holds, and dropping it is
	// what makes a late subscriber miss what it came for
	buffered, err := w.MonitorEvent(gen.Event{Name: gen.CoreEvent})
	if err != nil {
		return err
	}
	for _, message := range buffered {
		w.HandleEvent(message)
	}
	return nil
}

func (w *coreWatcher) HandleEvent(message gen.MessageEvent) error {
	if started, ok := message.Message.(gen.MessageCoreNodeStarted); ok {
		w.started = started.Applications
		w.seen = true
	}
	return nil
}

func (w *coreWatcher) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if w.seen == false {
		return nil, nil
	}
	return w.started, nil
}
