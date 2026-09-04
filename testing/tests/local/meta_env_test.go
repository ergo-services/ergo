package local

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type envMeta struct {
	gen.MetaProcess
	stop chan struct{}
}

func createEnvMeta() gen.MetaBehavior { return &envMeta{stop: make(chan struct{})} }

func (m *envMeta) Init(meta gen.MetaProcess) error {
	m.MetaProcess = meta
	return nil
}

func (m *envMeta) Start() error {
	<-m.stop
	return nil
}

func (m *envMeta) HandleMessage(from gen.PID, message any) error { return nil }

func (m *envMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "env":
		v, found := m.Env("SET")
		return probeResult{Value: v, Err: boolError(found)}, nil
	case "envmissing":
		_, found := m.Env("UNSET")
		return probeResult{Err: boolError(found)}, nil
	case "envlist":
		return probeResult{Value: m.EnvList()}, nil
	case "envdefault":
		return probeResult{Value: m.EnvDefault("UNSET", "fallback")}, nil
	case "priority":
		return probeResult{Value: m.SendPriority()}, nil
	case "setpriority":
		if err := m.SetSendPriority(gen.MessagePriorityMax); err != nil {
			return probeResult{Err: err}, nil
		}
		return probeResult{Value: m.SendPriority()}, nil
	}
	return probeResult{}, nil
}

func (m *envMeta) Terminate(reason error) { close(m.stop) }

func (m *envMeta) HandleInspect(from gen.PID, item ...string) map[string]string { return nil }

func boolError(found bool) error {
	if found {
		return nil
	}
	return gen.ErrUnknown
}

type envMetaHost struct{ act.Actor }

func factoryEnvMetaHost() gen.ProcessBehavior { return &envMetaHost{} }

func (h *envMetaHost) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	alias, err := h.SpawnMeta(createEnvMeta(), gen.MetaOptions{})
	if err != nil {
		return probeResult{Err: err}, nil
	}
	return probeResult{Value: alias}, nil
}

func TestMetaEnvAndPriority(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{Env: map[gen.Env]any{"SET": "yes"}})
	host := n.Spawn(factoryEnvMetaHost, gen.ProcessOptions{})

	result, err := n.Call(host, "spawn")
	check.NoError(t, err)
	alias := result.(probeResult).Value.(gen.Alias)

	ask := func(request any) probeResult {
		t.Helper()
		r, err := n.Call(alias, request)
		check.NoError(t, err)
		return r.(probeResult)
	}

	r := ask("env")
	check.NoError(t, r.Err)
	check.Equal(t, "yes", r.Value)

	check.ErrorIs(t, ask("envmissing").Err, gen.ErrUnknown)

	list := ask("envlist").Value.(map[gen.Env]any)
	check.Equal(t, "yes", list["SET"])

	check.Equal(t, "fallback", ask("envdefault").Value)

	check.Equal(t, gen.MessagePriorityNormal, ask("priority").Value)

	r = ask("setpriority")
	check.NoError(t, r.Err)
	check.Equal(t, gen.MessagePriorityMax, r.Value)
}
