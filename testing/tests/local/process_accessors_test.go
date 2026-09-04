package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type probeQuery struct {
	Kind   string
	PID    gen.PID
	Alias  gen.Alias
	Key    string
	Value  string
	Reason error
}

type probeResult struct {
	Err   error
	Value any
}

type probe struct {
	act.Actor
	meta gen.Alias
}

func factoryProbe() gen.ProcessBehavior { return &probe{} }

func (p *probe) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	q, ok := request.(probeQuery)
	if ok == false {
		return probeResult{}, nil
	}

	switch q.Kind {
	case "info":
		info, err := p.Info()
		return probeResult{Err: err, Value: info.PID}, nil

	case "envdefault":
		return probeResult{Value: p.EnvDefault(gen.Env(q.Key), q.Value)}, nil

	case "compressiontype":
		if err := p.SetCompressionType(gen.CompressionType(q.Value)); err != nil {
			return probeResult{Err: err}, nil
		}
		return probeResult{Value: p.CompressionType()}, nil

	case "important":
		if err := p.SetImportantDelivery(true); err != nil {
			return probeResult{Err: err}, nil
		}
		return probeResult{Value: p.ImportantDelivery()}, nil

	case "kind":
		return probeResult{Err: p.SetProcessKind(gen.ProcessKind(q.Value))}, nil

	case "spawnmeta":
		alias, err := p.SpawnMeta(createStageMeta(), gen.MetaOptions{})
		p.meta = alias
		return probeResult{Err: err, Value: alias}, nil

	case "metainfo":
		info, err := p.MetaInfo(q.Alias)
		return probeResult{Err: err, Value: info.Parent}, nil

	case "inspect":
		result, err := p.Inspect(q.PID)
		return probeResult{Err: err, Value: result}, nil

	case "exitafter":
		cancel, err := p.SendExitAfter(q.PID, q.Reason, time.Hour)
		if err == nil {
			cancel()
		}
		return probeResult{Err: err}, nil

	case "exitafternow":
		_, err := p.SendExitAfter(q.PID, q.Reason, 10*time.Millisecond)
		return probeResult{Err: err}, nil

	case "exitmetaafter":
		cancel, err := p.SendExitMetaAfter(q.Alias, q.Reason, time.Hour)
		if err == nil {
			cancel()
		}
		return probeResult{Err: err}, nil

	case "tracingattrs":
		p.SetTracingAttribute("a", "1")
		p.SetTracingAttribute("b", "2")
		p.SetTracingAttribute("a", "3")
		p.SetTracingAttribute("ergo.node", "refused")
		p.SetTracingSpanAttribute("span", "s1")
		p.SetTracingSpanAttribute("span", "s2")
		p.SetTracingSpanAttribute("ergo.span", "refused")
		return probeResult{Value: p.TracingAttributes()}, nil

	case "removetracingattr":
		p.RemoveTracingAttribute("a")
		p.RemoveTracingAttribute("missing")
		return probeResult{Value: p.TracingAttributes()}, nil
	}

	return probeResult{}, nil
}

func (p *probe) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{"probe": "yes"}
}

func askProbe(t *testing.T, n *stage.Node, pid gen.PID, q probeQuery) probeResult {
	t.Helper()
	result, err := n.Call(pid, q)
	check.NoError(t, err)
	return result.(probeResult)
}

func TestProcessAccessors(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{Env: map[gen.Env]any{"SET": "yes"}})
	pid := n.Spawn(factoryProbe, gen.ProcessOptions{})

	r := askProbe(t, n, pid, probeQuery{Kind: "info"})
	check.NoError(t, r.Err)
	check.Equal(t, pid, r.Value)

	r = askProbe(t, n, pid, probeQuery{Kind: "envdefault", Key: "SET", Value: "fallback"})
	check.Equal(t, "yes", r.Value)

	r = askProbe(t, n, pid, probeQuery{Kind: "envdefault", Key: "UNSET", Value: "fallback"})
	check.Equal(t, "fallback", r.Value)

	r = askProbe(t, n, pid, probeQuery{Kind: "compressiontype", Value: string(gen.CompressionTypeLZW)})
	check.NoError(t, r.Err)
	check.Equal(t, gen.CompressionTypeLZW, r.Value)

	r = askProbe(t, n, pid, probeQuery{Kind: "compressiontype", Value: "brotli"})
	check.ErrorIs(t, r.Err, gen.ErrIncorrect)

	r = askProbe(t, n, pid, probeQuery{Kind: "important"})
	check.NoError(t, r.Err)
	check.Equal(t, true, r.Value)

	r = askProbe(t, n, pid, probeQuery{Kind: "kind", Value: "custom"})
	check.NoError(t, r.Err)
}

func TestProcessMetaAndInspect(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	pid := n.Spawn(factoryProbe, gen.ProcessOptions{})
	other := n.Spawn(factoryProbe, gen.ProcessOptions{})

	r := askProbe(t, n, pid, probeQuery{Kind: "spawnmeta"})
	check.NoError(t, r.Err)
	alias := r.Value.(gen.Alias)

	r = askProbe(t, n, pid, probeQuery{Kind: "metainfo", Alias: alias})
	check.NoError(t, r.Err)
	check.Equal(t, pid, r.Value)

	r = askProbe(t, n, pid, probeQuery{Kind: "metainfo", Alias: gen.Alias{Node: n.Name(), ID: [3]uint64{9, 9, 9}}})
	check.ErrorIs(t, r.Err, gen.ErrProcessUnknown)

	r = askProbe(t, n, pid, probeQuery{Kind: "inspect", PID: other})
	check.NoError(t, r.Err)
	check.Equal(t, map[string]string{"probe": "yes"}, r.Value)

	r = askProbe(t, n, pid, probeQuery{Kind: "inspect", PID: gen.PID{Node: n.Name(), ID: 999999}})
	check.ErrorIs(t, r.Err, gen.ErrProcessUnknown)

	r = askProbe(t, n, pid, probeQuery{Kind: "inspect", PID: gen.PID{Node: "elsewhere@localhost", ID: 1}})
	check.ErrorIs(t, r.Err, gen.ErrNotAllowed)
}

func TestProcessSendExitAfter(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	pid := n.Spawn(factoryProbe, gen.ProcessOptions{})
	victim := n.Spawn(factoryTarget, gen.ProcessOptions{})

	r := askProbe(t, n, pid, probeQuery{Kind: "exitafter", PID: victim, Reason: gen.TerminateReasonShutdown})
	check.NoError(t, r.Err)

	r = askProbe(t, n, pid, probeQuery{Kind: "exitafter", PID: victim})
	check.ErrorIs(t, r.Err, gen.ErrIncorrect)

	r = askProbe(t, n, pid, probeQuery{Kind: "exitafter", PID: pid, Reason: gen.TerminateReasonShutdown})
	check.ErrorIs(t, r.Err, gen.ErrNotAllowed)

	r = askProbe(t, n, pid, probeQuery{Kind: "spawnmeta"})
	check.NoError(t, r.Err)
	alias := r.Value.(gen.Alias)

	r = askProbe(t, n, pid, probeQuery{Kind: "exitmetaafter", Alias: alias, Reason: gen.TerminateReasonShutdown})
	check.NoError(t, r.Err)

	r = askProbe(t, n, pid, probeQuery{Kind: "exitmetaafter", Alias: alias})
	check.ErrorIs(t, r.Err, gen.ErrIncorrect)

	unknown := gen.Alias{Node: n.Name(), ID: [3]uint64{9, 9, 9}}
	r = askProbe(t, n, pid, probeQuery{Kind: "exitmetaafter", Alias: unknown, Reason: gen.TerminateReasonShutdown})
	check.ErrorIs(t, r.Err, gen.ErrAliasUnknown)

	w := n.Spawn(factoryWatcher, gen.ProcessOptions{})
	n.Send(w, monitorCmd{Target: victim})
	n.ShouldMonitor().From(w).Target(victim).Once().Within(time.Second).Must()

	mk := n.Mark()
	r = askProbe(t, n, pid, probeQuery{Kind: "exitafternow", PID: victim, Reason: gen.TerminateReasonShutdown})
	check.NoError(t, r.Err)
	n.ShouldReceiveDown().To(w).About(victim).Since(mk).Once().Within(5 * time.Second).Must()
}

func TestProcessTracingAttributes(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	pid := n.Spawn(factoryProbe, gen.ProcessOptions{})

	r := askProbe(t, n, pid, probeQuery{Kind: "tracingattrs"})
	attrs := r.Value.([]gen.TracingAttribute)
	got := map[string]string{}
	for _, a := range attrs {
		got[a.Key] = a.Value
	}
	check.Equal(t, "3", got["a"])
	check.Equal(t, "2", got["b"])
	check.Equal(t, "s2", got["span"])
	check.Equal(t, 3, len(got))

	r = askProbe(t, n, pid, probeQuery{Kind: "removetracingattr"})
	attrs = r.Value.([]gen.TracingAttribute)
	for _, a := range attrs {
		if a.Key == "a" {
			t.Fatal("the removed attribute is still reported")
		}
	}
}
