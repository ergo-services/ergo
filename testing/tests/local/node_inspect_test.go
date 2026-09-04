package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// inspectSup is a One-For-One supervisor with two children.
type inspectSup struct{ act.Supervisor }

func factoryInspectSup() gen.ProcessBehavior { return &inspectSup{} }

func (s *inspectSup) Init(args ...any) (act.SupervisorSpec, error) {
	return act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "child1", Factory: factoryEcho},
			{Name: "child2", Factory: factoryEcho},
		},
		Restart: act.SupervisorRestart{Strategy: act.SupervisorStrategyTransient},
	}, nil
}

// metaActor spawns an inspectable meta process on request and returns its alias.
type metaActor struct{ act.Actor }

func factoryMetaActor() gen.ProcessBehavior { return &metaActor{} }

// A caller that hands over a channel gets it closed once the meta's goroutine is parked,
// which SpawnMeta returning does not say anything about.
func (a *metaActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	parked, _ := request.(chan struct{})
	return a.SpawnMeta(factoryInspectMeta(parked), gen.MetaOptions{})
}

// inspectMeta is a meta process exposing data via HandleInspect.
type inspectMeta struct {
	gen.MetaProcess
	stop   chan struct{}
	parked chan struct{}
}

func factoryInspectMeta(parked chan struct{}) gen.MetaBehavior {
	return &inspectMeta{stop: make(chan struct{}), parked: parked}
}

func (m *inspectMeta) Init(meta gen.MetaProcess) error { m.MetaProcess = meta; return nil }

// Start parks this meta's own goroutine until the meta is terminated. The channel is
// closed from inside it, so whoever waits on it knows the goroutine stands in this frame
// rather than having merely been launched.
func (m *inspectMeta) Start() error {
	if m.parked != nil {
		close(m.parked)
	}
	<-m.stop
	return nil
}
func (m *inspectMeta) HandleMessage(from gen.PID, message any) error {
	return nil
}
func (m *inspectMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}
func (m *inspectMeta) Terminate(reason error) { close(m.stop) }
func (m *inspectMeta) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{"test_meta": "ok"}
}

// TestLocalNodeInspect: Inspect returns a process's internal state (a supervisor
// reports its type and child count); inspecting an unknown, remote or terminated
// process is rejected with the matching error.
func TestLocalNodeInspect(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()

	sup := n.Spawn(factoryInspectSup, gen.ProcessOptions{})
	info, err := nd.Inspect(sup)
	check.NoError(t, err)
	check.Equal(t, "One For One", info["ergo:type"])
	check.Equal(t, "2", info["ergo:children_total"])

	// unknown process
	_, err = nd.Inspect(gen.PID{Node: n.Name(), ID: 99999, Creation: 1})
	check.ErrorIs(t, err, gen.ErrProcessUnknown)

	// remote process (local-only operation)
	_, err = nd.Inspect(gen.PID{Node: "remote@host", ID: 1, Creation: 1})
	check.ErrorIs(t, err, gen.ErrNotAllowed)

	// terminated process: monitor it, kill it, wait for the down, then inspect
	w := n.Spawn(factoryWatcher, gen.ProcessOptions{})
	victim := n.Spawn(factoryEcho, gen.ProcessOptions{})
	n.Send(w, monitorCmd{Target: victim})
	n.ShouldMonitor().From(w).Target(victim).Once().Within(time.Second).Must()
	n.Kill(victim)
	n.ShouldReceiveDown().To(w).About(victim).Once().Within(time.Second).Must()
	if _, err := nd.Inspect(victim); err != gen.ErrProcessTerminated {
		check.ErrorIs(t, err, gen.ErrProcessUnknown)
	}
}

// TestLocalNodeInspectMeta: InspectMeta returns a meta process's data; inspecting
// an unknown or remote meta is rejected.
func TestLocalNodeInspectMeta(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()

	owner := n.Spawn(factoryMetaActor, gen.ProcessOptions{})
	aliasAny, err := n.Call(owner, "spawnmeta")
	check.NoError(t, err)
	alias := aliasAny.(gen.Alias)

	info, err := nd.InspectMeta(alias)
	check.NoError(t, err)
	check.Equal(t, "ok", info["test_meta"])

	// unknown meta
	fake := gen.Alias(gen.Ref{Node: n.Name(), ID: [3]uint64{99999, 0, 0}, Creation: 1})
	_, err = nd.InspectMeta(fake)
	check.ErrorIs(t, err, gen.ErrMetaUnknown)

	// remote meta (local-only operation)
	remote := gen.Alias(gen.Ref{Node: "remote@host", ID: [3]uint64{1, 0, 0}, Creation: 1})
	_, err = nd.InspectMeta(remote)
	check.ErrorIs(t, err, gen.ErrNotAllowed)
}
