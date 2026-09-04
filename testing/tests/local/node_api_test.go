package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type echoAny struct {
	act.Actor
	alias gen.Alias
}

func factoryEchoAny() gen.ProcessBehavior { return &echoAny{} }

func (e *echoAny) Init(args ...any) error {
	alias, err := e.CreateAlias()
	if err != nil {
		return err
	}
	e.alias = alias
	return nil
}

func (e *echoAny) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "alias" {
		return e.alias, nil
	}
	return request, nil
}

func TestNodeCallVariants(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()
	pid := n.SpawnRegister("echoany", factoryEchoAny, gen.ProcessOptions{})

	aliasAny, err := nd.CallPID(pid, "alias", 0)
	check.NoError(t, err)
	alias := aliasAny.(gen.Alias)

	response, err := nd.CallPID(pid, "by pid", 5)
	check.NoError(t, err)
	check.Equal(t, "by pid", response)

	response, err = nd.CallProcessID(gen.ProcessID{Name: "echoany", Node: n.Name()}, "by name", 5)
	check.NoError(t, err)
	check.Equal(t, "by name", response)

	response, err = nd.CallAlias(alias, "by alias", 5)
	check.NoError(t, err)
	check.Equal(t, "by alias", response)

	_, err = nd.CallPID(gen.PID{Node: n.Name(), ID: 999999}, "nobody", 1)
	check.ErrorIs(t, err, gen.ErrProcessUnknown)

	_, err = nd.CallProcessID(gen.ProcessID{Name: "nobody", Node: n.Name()}, "x", 1)
	check.ErrorIs(t, err, gen.ErrProcessUnknown)

	_, err = nd.CallAlias(gen.Alias{Node: n.Name(), ID: [3]uint64{9, 9, 9}}, "x", 1)
	check.ErrorIs(t, err, gen.ErrProcessUnknown)

	nd.Stop()
	nd.Wait()

	_, err = nd.CallPID(pid, "x", 1)
	check.ErrorIs(t, err, gen.ErrNodeTerminated)
	_, err = nd.CallProcessID(gen.ProcessID{Name: "echoany", Node: n.Name()}, "x", 1)
	check.ErrorIs(t, err, gen.ErrNodeTerminated)
	_, err = nd.CallAlias(alias, "x", 1)
	check.ErrorIs(t, err, gen.ErrNodeTerminated)
}

func TestNodeEnvDefault(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{Env: map[gen.Env]any{"SET": "yes"}})
	nd := n.Native()

	check.Equal(t, "yes", nd.EnvDefault("SET", "fallback"))
	check.Equal(t, "fallback", nd.EnvDefault("UNSET", "fallback"))
}

func TestNodeEventInfoListing(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()
	pid := n.Spawn(factoryTarget, gen.ProcessOptions{})

	result, err := n.Call(pid, "info")
	check.NoError(t, err)
	event := result.(targetInfo).Event

	list, err := nd.EventListInfo(0, 100)
	check.NoError(t, err)
	found := false
	for _, e := range list {
		if e.Event.Name == event.Name {
			found = true
		}
	}
	check.True(t, found)

	filtered, err := nd.EventListInfo(0, 100, func(info gen.EventInfo) bool { return info.Event.Name == event.Name })
	check.NoError(t, err)
	check.Equal(t, 1, len(filtered))

	seen := 0
	check.NoError(t, nd.EventRangeInfo(func(info gen.EventInfo) bool {
		seen++
		return true
	}))
	check.True(t, seen > 0)

	nd.Stop()
	nd.Wait()

	_, err = nd.EventListInfo(0, 100)
	check.ErrorIs(t, err, gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.EventRangeInfo(func(gen.EventInfo) bool { return true }), gen.ErrNodeTerminated)
}

type loggerActor struct{ act.Actor }

func factoryLoggerActor() gen.ProcessBehavior { return &loggerActor{} }

func (l *loggerActor) HandleLog(message gen.MessageLog) error { return nil }

func TestNodeLoggerPlane(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()
	pid := n.Spawn(factoryLoggerActor, gen.ProcessOptions{})
	unknown := gen.PID{Node: n.Name(), ID: 999999}

	check.ErrorIs(t, nd.LoggerAddPID(unknown, "bypid"), gen.ErrProcessUnknown)
	check.ErrorIs(t, nd.LoggerAddPID(pid, ""), gen.ErrIncorrect)

	check.NoError(t, nd.LoggerAddPID(pid, "bypid", gen.LogLevelError, gen.LogLevelPanic))
	check.ErrorIs(t, nd.LoggerAddPID(pid, "again"), gen.ErrNotAllowed)

	check.True(t, contains(nd.Loggers(), "bypid"))

	levels := nd.LoggerLevels("bypid")
	check.Equal(t, 2, len(levels))
	check.True(t, contains(levels, gen.LogLevelError))
	check.True(t, contains(levels, gen.LogLevelPanic))
	check.Equal(t, 0, len(nd.LoggerLevels("missing")))

	nd.LoggerDeletePID(pid)
	check.True(t, contains(nd.Loggers(), "bypid") == false)

	nd.Stop()
	nd.Wait()
	check.ErrorIs(t, nd.LoggerAddPID(pid, "late"), gen.ErrNodeTerminated)
}

func TestNodeApplicationStopWithTimeout(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{Applications: []gen.ApplicationBehavior{createAppAccessors()}})
	nd := n.Native()

	check.ErrorIs(t, nd.ApplicationStopWithTimeout("nosuchapp", time.Second), gen.ErrApplicationUnknown)
	check.NoError(t, nd.ApplicationStopWithTimeout("accessor_app", 5*time.Second))

	info, err := nd.ApplicationInfo("accessor_app")
	check.NoError(t, err)
	check.Equal(t, gen.ApplicationStateLoaded, info.State)
}
