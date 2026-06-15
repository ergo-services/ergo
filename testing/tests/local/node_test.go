package local

import (
	"reflect"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type t0 struct{ act.Actor }

func factoryT0() gen.ProcessBehavior { return &t0{} }

func (a *t0) HandleMessage(from gen.PID, message any) error { return nil }

// TestLocalNode: a bare node exposes name/liveness/info, env get/set/remove,
// process spawning with name registration (ErrTaken / ErrProcessUnknown /
// ErrNameUnknown), ProcessInfo, ProcessList, Kill, node-level Send and SendExit
// (nil reason rejected), and lifecycle (Wait/Stop/terminated state).
func TestLocalNode(t *testing.T) {
	env := map[gen.Env]any{gen.Env("A"): 1, gen.Env("B"): 1.23, gen.Env("C"): "d"}

	s := stage.New(t)
	n := s.Node("t0", stage.NodeOptions{Env: env})
	nd := n.Native()

	check.True(t, nd.IsAlive())

	info, err := nd.Info()
	check.NoError(t, err)
	check.Equal(t, n.Name(), info.Name)
	check.Equal(t, 0, int(info.ProcessesTotal))
	check.Equal(t, 0, int(info.ProcessesRunning))
	check.Equal(t, 0, int(info.ProcessesZombee))
	check.Equal(t, 0, int(info.RegisteredAliases))
	check.Equal(t, 0, int(info.RegisteredNames))

	// env: list / get (case-insensitive) / remove / set
	check.Equal(t, env, nd.EnvList())
	v, exist := nd.Env("a")
	check.True(t, exist)
	check.Equal(t, env["A"], v)
	nd.SetEnv("a", nil)
	_, exist = nd.Env("a")
	check.True(t, exist == false)
	nd.SetEnv("a", "v")
	v, exist = nd.Env("a")
	check.True(t, exist)
	check.Equal(t, "v", v)

	// spawn + name registration
	pid, err := nd.Spawn(factoryT0, gen.ProcessOptions{})
	check.NoError(t, err)
	check.NoError(t, nd.RegisterName("test", pid))
	check.ErrorIs(t, nd.RegisterName("test", pid), gen.ErrTaken)
	check.ErrorIs(t, nd.RegisterName("test", gen.PID{}), gen.ErrProcessUnknown)

	pinfo, err := nd.ProcessInfo(pid)
	check.NoError(t, err)
	check.Equal(t, pid, pinfo.PID)
	check.Equal(t, gen.Atom("test"), pinfo.Name)
	check.True(t, pinfo.State == gen.ProcessStateSleep || pinfo.State == gen.ProcessStateRunning)
	check.Equal(t, nd.PID(), pinfo.Parent)
	check.Equal(t, nd.PID(), pinfo.Leader)

	info, err = nd.Info()
	check.NoError(t, err)
	check.Equal(t, 1, int(info.ProcessesTotal))
	check.True(t, info.ProcessesRunning <= 1)
	check.Equal(t, 1, int(info.RegisteredNames))

	// unregister name (+ unknown)
	p, err := nd.UnregisterName("test")
	check.NoError(t, err)
	check.Equal(t, pid, p)
	_, err = nd.UnregisterName("test")
	check.ErrorIs(t, err, gen.ErrNameUnknown)

	// process list
	l, err := nd.ProcessList()
	check.NoError(t, err)
	check.True(t, reflect.DeepEqual([]gen.PID{pid}, l))

	// kill: the process becomes zombie/terminated or disappears
	check.NoError(t, nd.Kill(pid))
	if _, err := nd.ProcessInfo(pid); err != nil {
		check.ErrorIs(t, err, gen.ErrProcessUnknown)
	}

	// spawn with registered name
	pid, err = nd.SpawnRegister("test2", factoryT0, gen.ProcessOptions{})
	check.NoError(t, err)
	pinfo, err = nd.ProcessInfo(pid)
	check.NoError(t, err)
	check.Equal(t, pid, pinfo.PID)
	check.Equal(t, gen.Atom("test2"), pinfo.Name)

	// node-level send: by PID, by name, unknown name fails
	check.NoError(t, nd.Send(pid, 1))
	check.NoError(t, nd.Send(gen.Atom("test2"), 1))
	check.ErrorIs(t, nd.Send(gen.Atom("unknown"), 1), gen.ErrProcessUnknown)

	// unregister the spawn-registered name
	p, err = nd.UnregisterName("test2")
	check.NoError(t, err)
	check.Equal(t, pid, p)
	pinfo, err = nd.ProcessInfo(pid)
	check.NoError(t, err)
	check.Equal(t, gen.Atom(""), pinfo.Name)

	// exit signal: nil reason rejected, valid reason accepted
	check.ErrorIs(t, nd.SendExit(pid, nil), gen.ErrIncorrect)
	check.NoError(t, nd.SendExit(pid, gen.TerminateReasonNormal))

	// lifecycle
	check.ErrorIs(t, nd.WaitWithTimeout(0), gen.ErrTimeout)
	nd.Stop()
	check.True(t, nd.IsAlive() == false)
	_, err = nd.Info()
	check.ErrorIs(t, err, gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.WaitWithTimeout(0), gen.ErrNodeTerminated)
}
