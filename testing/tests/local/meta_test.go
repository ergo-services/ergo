package local

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// stageMeta is a meta process: it forwards "forward" to any pid it is sent,
// echoes a call request, spawns a child meta on a bool request, and answers
// inspect with a map keyed by the requester. Start blocks until termination (the
// meta run-loop model), which is not an actor callback so blocking is correct.
type stageMeta struct {
	gen.MetaProcess
	stop chan struct{}
}

func createStageMeta() gen.MetaBehavior { return &stageMeta{stop: make(chan struct{})} }

func (m *stageMeta) Init(meta gen.MetaProcess) error {
	m.MetaProcess = meta
	return nil
}

func (m *stageMeta) Start() error {
	<-m.stop
	return nil
}

func (m *stageMeta) HandleMessage(from gen.PID, message any) error {
	if pid, ok := message.(gen.PID); ok {
		return m.Send(pid, "forward")
	}
	return nil
}

func (m *stageMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if _, ok := request.(bool); ok {
		id, err := m.Spawn(createStageMeta(), gen.MetaOptions{})
		if err != nil {
			return err, nil
		}
		return id, nil
	}
	return request, nil
}

func (m *stageMeta) Terminate(reason error) { close(m.stop) }

func (m *stageMeta) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{from.String(): "ok"}
}

// metaHost owns meta processes (only a process can SpawnMeta) and performs meta
// operations on command. trap is set so a link-death of a meta is delivered as a
// message instead of terminating the host.
type metaHost struct {
	act.Actor
}

func factoryMetaHost() gen.ProcessBehavior { return &metaHost{} }

func (h *metaHost) Init(args ...any) error {
	if len(args) > 0 {
		h.SetTrapExit(args[0].(bool))
	}
	return nil
}

type spawnMetaCmd struct{}
type exitMetaCmd struct {
	Alias  gen.Alias
	Reason error
}
type linkMetaCmd struct{ Alias gen.Alias }
type monitorMetaCmd struct{ Alias gen.Alias }

func (h *metaHost) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch c := request.(type) {
	case spawnMetaCmd:
		id, err := h.SpawnMeta(createStageMeta(), gen.MetaOptions{})
		if err != nil {
			return err, nil
		}
		return id, nil
	case exitMetaCmd:
		return errText(h.SendExitMeta(c.Alias, c.Reason)), nil
	case linkMetaCmd:
		return errText(h.Link(c.Alias)), nil
	case monitorMetaCmd:
		return errText(h.Monitor(c.Alias)), nil
	}
	return "ok", nil
}

// TestLocalMeta: meta-process lifecycle and addressing on a live node. A process
// spawns a meta; the meta is reachable by its alias for call/send/inspect; it can
// spawn a child meta; SendExitMeta terminates it with the exact reason; a process
// can link or monitor a meta and receive MessageExitAlias / MessageDownAlias on
// its termination; and a meta dies with its parent's reason when the parent does.
func TestLocalMeta(t *testing.T) {
	// Basic: spawn, call, send-forward, inspect, child meta, SendExitMeta reason
	t.Run("Basic", func(t *testing.T) {
		s := stage.New(t)
		n := s.StartNode("n")
		collector := n.Spawn(factoryEcho, gen.ProcessOptions{})
		host := n.Spawn(factoryMetaHost, gen.ProcessOptions{}, false)
		watcher := n.Spawn(factoryWatcher, gen.ProcessOptions{})

		aliasAny, err := n.Call(host, spawnMetaCmd{})
		check.NoError(t, err)
		alias, ok := aliasAny.(gen.Alias)
		check.True(t, ok)

		// call the meta directly by alias; it echoes the request
		resp, err := n.Call(alias, "ping-pong")
		check.NoError(t, err)
		check.Equal(t, "ping-pong", resp)

		// send the meta a pid; it forwards "forward" to that pid
		mk := n.Mark()
		n.Send(alias, collector)
		n.ShouldDeliver().To(collector).Message("forward").Since(mk).Once().Within(time.Second).Must()

		// inspect returns a map keyed by the requester (the node)
		insp, err := n.Native().InspectMeta(alias)
		check.NoError(t, err)
		check.True(t, reflect.DeepEqual(insp, map[string]string{n.PID().String(): "ok"}))

		// the meta spawns a child meta, returning its alias
		childAny, err := n.Call(alias, true)
		check.NoError(t, err)
		_, ok = childAny.(gen.Alias)
		check.True(t, ok)

		// monitor the meta, then SendExitMeta: it terminates with the exact reason
		n.Send(watcher, monitorCmd{Target: alias})
		n.ShouldMonitor().From(watcher).Target(alias).Once().Within(time.Second).Must()
		xterm := errors.New("test meta exit")
		mk2 := n.Mark()
		res, err := n.Call(host, exitMetaCmd{Alias: alias, Reason: xterm})
		check.NoError(t, err)
		check.Equal(t, "", res)
		n.ShouldReceiveDown().To(watcher).AboutAlias(alias).Reason(xterm).Since(mk2).Once().Within(time.Second).Must()
	})

	// Link: a process linked to a meta receives MessageExitAlias on its termination
	t.Run("Link", func(t *testing.T) {
		s := stage.New(t)
		n := s.StartNode("n")
		host := n.Spawn(factoryMetaHost, gen.ProcessOptions{}, true) // trap so the link-death is a message

		aliasAny, err := n.Call(host, spawnMetaCmd{})
		check.NoError(t, err)
		alias := aliasAny.(gen.Alias)

		res, err := n.Call(host, linkMetaCmd{Alias: alias})
		check.NoError(t, err)
		check.Equal(t, "", res)

		idterm := errors.New("term meta")
		mk := n.Mark()
		_, err = n.Call(host, exitMetaCmd{Alias: alias, Reason: idterm})
		check.NoError(t, err)
		n.ShouldReceiveExit().To(host).AboutAlias(alias).Reason(idterm).Since(mk).Once().Within(time.Second).Must()
	})

	// Monitor: a process monitoring a meta receives MessageDownAlias on its termination
	t.Run("Monitor", func(t *testing.T) {
		s := stage.New(t)
		n := s.StartNode("n")
		host := n.Spawn(factoryMetaHost, gen.ProcessOptions{}, false)

		aliasAny, err := n.Call(host, spawnMetaCmd{})
		check.NoError(t, err)
		alias := aliasAny.(gen.Alias)

		res, err := n.Call(host, monitorMetaCmd{Alias: alias})
		check.NoError(t, err)
		check.Equal(t, "", res)

		idterm := errors.New("term meta")
		mk := n.Mark()
		_, err = n.Call(host, exitMetaCmd{Alias: alias, Reason: idterm})
		check.NoError(t, err)
		n.ShouldReceiveDown().To(host).AboutAlias(alias).Reason(idterm).Since(mk).Once().Within(time.Second).Must()
	})

	// ParentDeath: a meta terminates with its parent's reason when the parent dies
	t.Run("ParentDeath", func(t *testing.T) {
		s := stage.New(t)
		n := s.StartNode("n")
		host := n.Spawn(factoryMetaHost, gen.ProcessOptions{}, false) // non-trap: SendExit terminates it
		watcher := n.Spawn(factoryWatcher, gen.ProcessOptions{})

		aliasAny, err := n.Call(host, spawnMetaCmd{})
		check.NoError(t, err)
		alias := aliasAny.(gen.Alias)

		n.Send(watcher, monitorCmd{Target: alias})
		n.ShouldMonitor().From(watcher).Target(alias).Once().Within(time.Second).Must()

		pidterm := errors.New("blabla")
		mk := n.Mark()
		check.NoError(t, n.SendExit(host, pidterm))
		n.ShouldReceiveDown().To(watcher).AboutAlias(alias).Reason(pidterm).Since(mk).Once().Within(time.Second).Must()
	})
}
