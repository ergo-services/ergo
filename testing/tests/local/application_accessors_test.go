package local

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type appQuery struct {
	Kind  string
	Tag   gen.Atom
	Tags  []gen.Atom
	Value int
}

type appResult struct{ Err error }

type appReporter struct {
	act.Actor
	handle gen.Application
}

func factoryAppReporter() gen.ProcessBehavior { return &appReporter{} }

func (r *appReporter) Init(args ...any) error {
	r.handle = args[0].(gen.Application)
	return nil
}

func (r *appReporter) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	q, ok := request.(appQuery)
	if ok == false {
		return nil, nil
	}
	switch q.Kind {
	case "name":
		return r.handle.Name(), nil
	case "mode":
		return r.handle.Mode(), nil
	case "state":
		return r.handle.State(), nil
	case "env":
		v, _ := r.handle.Env("TEST")
		return v, nil
	case "envmissing":
		_, found := r.handle.Env("NOPE")
		return found, nil
	case "envlist":
		return r.handle.EnvList(), nil
	case "tags":
		return r.handle.Tags(), nil
	case "addtag":
		return appResult{Err: r.handle.AddTag(q.Tag)}, nil
	case "removetag":
		return appResult{Err: r.handle.RemoveTag(q.Tag)}, nil
	case "settags":
		return appResult{Err: r.handle.SetTags(q.Tags)}, nil
	case "weight":
		return r.handle.Weight(), nil
	case "setweight":
		return appResult{Err: r.handle.SetWeight(q.Value)}, nil
	case "logger":
		return r.handle.Log().Level(), nil
	case "node":
		return r.handle.Node().Name(), nil
	case "behavior":
		return r.handle.Behavior() != nil, nil
	}
	return nil, nil
}

type appAccessors struct{ app.Application }

func createAppAccessors() gen.ApplicationBehavior { return &appAccessors{} }

func (a *appAccessors) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name: "accessor_app",
		Group: []gen.ApplicationMemberSpec{
			{Name: "accessor_member", Factory: factoryAppReporter, Args: []any{gen.Application(a)}},
		},
		Env:    map[gen.Env]any{"TEST": 12345},
		Mode:   gen.ApplicationModeTransient,
		Weight: 7,
		Tags:   []gen.Atom{"alpha"},
	}, nil
}

func TestApplicationAccessors(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{Applications: []gen.ApplicationBehavior{createAppAccessors()}})
	member := n.ProcessID("accessor_member")

	ask := func(q appQuery) any {
		t.Helper()
		result, err := n.Call(member, q)
		check.NoError(t, err)
		if r, ok := result.(appResult); ok {
			check.NoError(t, r.Err)
		}
		return result
	}

	check.Equal(t, gen.Atom("accessor_app"), ask(appQuery{Kind: "name"}))
	check.Equal(t, gen.ApplicationModeTransient, ask(appQuery{Kind: "mode"}))
	check.Equal(t, gen.ApplicationStateRunning, ask(appQuery{Kind: "state"}))
	check.Equal(t, n.Name(), ask(appQuery{Kind: "node"}))
	check.Equal(t, true, ask(appQuery{Kind: "behavior"}))
	check.Equal(t, 12345, ask(appQuery{Kind: "env"}))
	check.Equal(t, false, ask(appQuery{Kind: "envmissing"}))
	check.Equal(t, map[gen.Env]any{"TEST": 12345}, ask(appQuery{Kind: "envlist"}))

	check.Equal(t, []gen.Atom{"alpha"}, ask(appQuery{Kind: "tags"}))
	check.Equal(t, 7, ask(appQuery{Kind: "weight"}))
	check.Equal(t, gen.LogLevelInfo, ask(appQuery{Kind: "logger"}))

	ask(appQuery{Kind: "addtag", Tag: "beta"})
	ask(appQuery{Kind: "addtag", Tag: "beta"})
	check.Equal(t, []gen.Atom{"alpha", "beta"}, ask(appQuery{Kind: "tags"}))

	ask(appQuery{Kind: "removetag", Tag: "alpha"})
	ask(appQuery{Kind: "removetag", Tag: "gamma"})
	check.Equal(t, []gen.Atom{"beta"}, ask(appQuery{Kind: "tags"}))

	ask(appQuery{Kind: "settags", Tags: []gen.Atom{"one", "two"}})
	check.Equal(t, []gen.Atom{"one", "two"}, ask(appQuery{Kind: "tags"}))

	ask(appQuery{Kind: "setweight", Value: 42})
	check.Equal(t, 42, ask(appQuery{Kind: "weight"}))

	info, err := n.Native().ApplicationInfo("accessor_app")
	check.NoError(t, err)
	check.Equal(t, 42, info.Weight)
	check.Equal(t, []gen.Atom{"one", "two"}, info.Tags)
}
