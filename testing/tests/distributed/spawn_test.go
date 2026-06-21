package distributed

import (
	"reflect"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// spawnable is the factory exposed for remote spawning; it reports its own env
// (so env inheritance can be checked without relying on ProcessInfo exposure).
type spawnable struct{ act.Actor }

func factorySpawnable() gen.ProcessBehavior { return &spawnable{} }

func (s *spawnable) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return s.EnvList(), nil
}

// rspawner performs process-level remote spawns on command (a process is needed
// so the spawned child inherits the spawner as parent).
type rspawner struct{ act.Actor }

func factoryRSpawner() gen.ProcessBehavior { return &rspawner{} }

type rspawnCmd struct{ Node, Name gen.Atom }
type rspawnRegCmd struct{ Node, Name, Reg gen.Atom }

func (s *rspawner) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch c := request.(type) {
	case rspawnCmd:
		pid, err := s.RemoteSpawn(c.Node, c.Name, gen.ProcessOptions{})
		if err != nil {
			return err.Error(), nil
		}
		return pid, nil
	case rspawnRegCmd:
		pid, err := s.RemoteSpawnRegister(c.Node, c.Name, c.Reg, gen.ProcessOptions{})
		if err != nil {
			return err.Error(), nil
		}
		return pid, nil
	}
	return "ok", nil
}

// TestDistRemoteSpawn: a node spawns a process on a peer via the RemoteNode handle
// (parent/leader become the requesting node, env is inherited when exposed) and
// via process.RemoteSpawn (parent is the spawning process); unknown factory names
// and duplicate registered names are rejected.
func TestDistRemoteSpawn(t *testing.T) {
	s := stage.New(t)
	n1 := s.StartNode("n1", stage.NodeOptions{
		Env:      map[gen.Env]any{"K": "V"},
		Security: gen.SecurityOptions{ExposeEnvRemoteSpawn: true},
	})
	n2 := s.StartNode("n2")
	n2.EnableSpawn("tst", factorySpawnable)
	remote := s.Connect(n1, n2)

	t.Run("RemoteNode", func(t *testing.T) {
		pid, err := remote.Spawn("tst", gen.ProcessOptions{})
		check.NoError(t, err)
		info, err := n2.Native().ProcessInfo(pid)
		check.NoError(t, err)
		check.Equal(t, n1.PID(), info.Parent)
		check.Equal(t, n1.PID(), info.Leader)
		// the process inherited the requesting node's env (checked via the process
		// itself, since ProcessInfo.Env is gated by the peer's ExposeEnvInfo)
		ev, err := n2.Call(pid, "env")
		check.NoError(t, err)
		check.True(t, reflect.DeepEqual(ev, n1.Native().EnvList()))

		reg, err := remote.SpawnRegister("regtst", "tst", gen.ProcessOptions{})
		check.NoError(t, err)
		rinfo, err := n2.Native().ProcessInfo(reg)
		check.NoError(t, err)
		check.Equal(t, gen.Atom("regtst"), rinfo.Name)
		check.Equal(t, n1.PID(), rinfo.Parent)
	})

	t.Run("Process", func(t *testing.T) {
		spawner := n1.Spawn(factoryRSpawner, gen.ProcessOptions{})

		// unknown factory name on the peer
		res, err := n1.Call(spawner, rspawnCmd{Node: n2.Name(), Name: "unknown"})
		check.NoError(t, err)
		check.Equal(t, gen.ErrNameUnknown.Error(), res)

		// spawn: the child's parent is the spawning process
		v, err := n1.Call(spawner, rspawnCmd{Node: n2.Name(), Name: "tst"})
		check.NoError(t, err)
		pid := v.(gen.PID)
		info, err := n2.Native().ProcessInfo(pid)
		check.NoError(t, err)
		check.Equal(t, spawner, info.Parent)

		// register, then a duplicate registered name is rejected
		v2, err := n1.Call(spawner, rspawnRegCmd{Node: n2.Name(), Name: "tst", Reg: "proc_reg"})
		check.NoError(t, err)
		_, ok := v2.(gen.PID)
		check.True(t, ok)
		res3, err := n1.Call(spawner, rspawnRegCmd{Node: n2.Name(), Name: "tst", Reg: "proc_reg"})
		check.NoError(t, err)
		check.Equal(t, gen.ErrTaken.Error(), res3)
	})
}

// TestDistRemoteSpawnAllowList: EnableSpawn with no nodes is open to all; with
// nodes it restricts to them. Regression for the allow-list inversion: disabling a
// different node must not deny an allowed one, and disabling the last allowed node
// must not re-open spawning to all.
func TestDistRemoteSpawnAllowList(t *testing.T) {
	s := stage.New(t)
	n1 := s.StartNode("n1")
	n2 := s.StartNode("n2")
	remote := s.Connect(n1, n2)

	other := gen.Atom("other@localhost")

	// open to all -> n1 allowed
	check.NoError(t, n2.Native().Network().EnableSpawn("tst", factorySpawnable))
	_, err := remote.Spawn("tst", gen.ProcessOptions{})
	check.NoError(t, err)

	// disabling a different node must not deny n1
	check.NoError(t, n2.Native().Network().DisableSpawn("tst", other))
	_, err = remote.Spawn("tst", gen.ProcessOptions{})
	check.NoError(t, err)

	// restrict to "other" only -> n1 not allowed
	check.NoError(t, n2.Native().Network().EnableSpawn("tst", factorySpawnable, other))
	_, err = remote.Spawn("tst", gen.ProcessOptions{})
	check.True(t, err == gen.ErrNotAllowed)

	// disabling the only allowed node must not re-open to all
	check.NoError(t, n2.Native().Network().DisableSpawn("tst", other))
	_, err = remote.Spawn("tst", gen.ProcessOptions{})
	check.True(t, err == gen.ErrNotAllowed)

	// re-open to all -> n1 allowed again
	check.NoError(t, n2.Native().Network().EnableSpawn("tst", factorySpawnable))
	_, err = remote.Spawn("tst", gen.ProcessOptions{})
	check.NoError(t, err)
}
