package mock_test

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

// every mock satisfies its gen interface (compile-time, in addition to the var _ guards).
var (
	_ gen.Node        = mock.NewNode()
	_ gen.Process     = mock.NewProcess()
	_ gen.MetaProcess = mock.NewMeta()
	_ gen.Log         = mock.NewLog()
	_ gen.Cron        = mock.NewCron()
	_ gen.Network     = mock.NewNetwork()
	_ gen.RemoteNode  = mock.NewRemoteNode()
	_ gen.Registrar   = mock.NewRegistrar()
	_ gen.Resolver    = mock.NewResolver()
)

// dumb mock: override a couple of methods, inject into code that consumes the
// interface; everything else returns a safe default.
func TestDumbProcessOverride(t *testing.T) {
	p := mock.NewProcess()
	p.OnCall(func(to any, request any) (any, error) { return "pong", nil })

	resp, err := p.Call(gen.Atom("svc"), "ping")
	check.NoError(t, err)
	check.Equal(t, "pong", resp)

	// an unconfigured egress just returns its safe default (no panic, no fatal)
	check.NoError(t, p.Send(gen.Atom("x"), "hi"))
}

// dumb mock as a gen.Node dependency, one method overridden.
func TestInjectGenNode(t *testing.T) {
	n := mock.NewNode()
	n.OnName(func() gen.Atom { return "overridden@host" })

	var node gen.Node = n
	check.Equal(t, gen.Atom("overridden@host"), node.Name())
}

// NewXT records egress; Should* asserts over it.
func TestProcessTRecords(t *testing.T) {
	p := mock.NewProcessT(t)
	p.Send(gen.Atom("worker"), "job")
	p.Call(gen.Atom("db"), "query")

	p.ShouldSend().To(gen.Atom("worker")).Message("job").Once().Assert()
	p.ShouldCall().To(gen.Atom("db")).Request("query").Once().Assert()
}

// a NewNodeT node hands its sub-mocks the same recorder, so node egress and the
// node's logger collate into one stream the node asserts over.
func TestNodeCompositionOneStream(t *testing.T) {
	n := mock.NewNodeT(t)
	n.Send(gen.Atom("peer"), "ping")
	n.Log().Info("started %d workers", 3)

	n.ShouldSend().To(gen.Atom("peer")).Message("ping").Once().Assert()
	n.ShouldLog().Containing("started 3 workers").Once().Assert()
}

// dumb Log: override one level, the rest are safe no-ops.
func TestDumbLogOverride(t *testing.T) {
	l := mock.NewLog()
	got := ""
	l.OnError(func(format string, args ...any) { got = format })
	l.Info("ignored")
	l.Error("boom")
	check.Equal(t, "boom", got)
}

func TestCronTRecords(t *testing.T) {
	c := mock.NewCronT(t)
	check.NoError(t, c.AddJob(gen.CronJob{Name: "nightly", Spec: "0 0 * * *"}))
	c.ShouldAddCronJob().Name(gen.Atom("nightly")).Once().Assert()
}

func TestRemoteNodeTRecords(t *testing.T) {
	rn := mock.NewRemoteNodeT(t)
	rn.Spawn("worker", gen.ProcessOptions{})
	rn.ShouldRemoteSpawn().Name(gen.Atom("worker")).Once().Assert()
}

// the override shapes the result, but on a recording mock the egress is STILL
// recorded (matches testing/unit: a stub sets the return, the action is recorded).
func TestProcessTOverrideStillRecords(t *testing.T) {
	p := mock.NewProcessT(t)
	p.OnSend(func(to any, message any) error { return gen.ErrProcessUnknown })
	p.OnCall(func(to any, request any) (any, error) { return "resp", nil })

	check.ErrorIs(t, p.Send(gen.Atom("dead"), "hi"), gen.ErrProcessUnknown)
	r, _ := p.Call(gen.Atom("svc"), "q")
	check.Equal(t, "resp", r)

	p.ShouldSend().To(gen.Atom("dead")).Once().Assert() // recorded despite override
	p.ShouldCall().To(gen.Atom("svc")).Request("q").Once().Assert()
}

// the per-line Log override runs first (it is the behavior) and the line is still
// recorded afterwards.
func TestLogTOverrideStillRecords(t *testing.T) {
	l := mock.NewLogT(t)
	ran := false
	l.OnInfo(func(format string, args ...any) { ran = true })
	l.Info("hello %d", 1)
	check.True(t, ran)
	l.ShouldLog().Containing("hello 1").Once().Assert()
}

// SendExit records the operation error (C1) distinct from the exit reason, and
// SendExitMeta records the alias destination as a SendExitMeta (C2).
func TestProcessTSendExit(t *testing.T) {
	p := mock.NewProcessT(t)
	p.OnSendExit(func(to gen.PID, reason error) error { return gen.ErrProcessUnknown })

	check.ErrorIs(t, p.SendExit(gen.PID{Node: "n@h", ID: 5, Creation: 1}, gen.TerminateReasonShutdown), gen.ErrProcessUnknown)
	p.ShouldSendExit().ErrorIs(gen.ErrProcessUnknown).ReasonIs(gen.TerminateReasonShutdown).Once().Assert()

	alias := gen.Alias{Node: "n@h", Creation: 1, ID: [3]uint64{9, 0, 0}}
	p.SendExitMeta(alias, gen.TerminateReasonNormal)
	p.ShouldSendExitMeta().Meta(alias).ReasonIs(gen.TerminateReasonNormal).Once().Assert()
}

// the Network sub-mocks (Registrar/Resolver) are exported, so a consumer can reach
// them by type assertion and override their methods (B).
func TestNetworkResolverOverrideReachable(t *testing.T) {
	net := mock.NewNetworkT(t)
	reg, _ := net.Registrar()
	reg.(*mock.Registrar).Resolver().(*mock.Resolver).
		OnResolve(func(node gen.Atom) ([]gen.Route, error) { return nil, gen.ErrNoRoute })

	r, _ := net.Registrar()
	_, err := r.Resolver().Resolve(gen.Atom("svc"))
	check.ErrorIs(t, err, gen.ErrNoRoute)
}
