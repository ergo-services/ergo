package stage_test

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// two minimal applications, each exposing one registered service
type app1 struct{ app.Application }

func createApp1() gen.ApplicationBehavior { return &app1{} }

func (a *app1) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:  "app1",
		Group: []gen.ApplicationMemberSpec{{Factory: factoryPinger, Name: "service1"}},
		Mode:  gen.ApplicationModePermanent,
	}, nil
}

type app2 struct{ app.Application }

func createApp2() gen.ApplicationBehavior { return &app2{} }

func (a *app2) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:  "app2",
		Group: []gen.ApplicationMemberSpec{{Factory: factoryPonger, Name: "service2"}},
		Mode:  gen.ApplicationModePermanent,
	}, nil
}

// wire messages (registered for cross-node transport)
type ping struct{ Seq int }
type pong struct{ Seq int }
type pingRequest struct{ Seq int }
type sendPing struct {
	To  gen.PID
	Seq int
}
type sendPingHigh struct {
	To  gen.PID
	Seq int
}

// workers

// pinger, on a sendPing trigger, sends a ping to the given target.
type pinger struct{ act.Actor }

func factoryPinger() gen.ProcessBehavior { return &pinger{} }

func (p *pinger) HandleMessage(from gen.PID, message any) error {
	switch t := message.(type) {
	case sendPing:
		return p.Send(t.To, ping{Seq: t.Seq})
	case sendPingHigh:
		return p.SendWithPriority(t.To, ping{Seq: t.Seq}, gen.MessagePriorityHigh)
	}
	return nil
}

// ponger answers requests and accepts pings.
type ponger struct{ act.Actor }

func factoryPonger() gen.ProcessBehavior { return &ponger{} }

func (p *ponger) HandleMessage(from gen.PID, message any) error { return nil }

func (p *ponger) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if r, ok := request.(pingRequest); ok {
		return pong{Seq: r.Seq}, nil
	}
	return nil, nil
}

// watcher monitors the given target from its Init.
type watcher struct{ act.Actor }

func factoryWatcher() gen.ProcessBehavior { return &watcher{} }

func (w *watcher) Init(args ...any) error { return w.MonitorPID(args[0].(gen.PID)) }

func (w *watcher) HandleMessage(from gen.PID, message any) error { return nil }

func registerWire(nodes ...*stage.Node) {
	for _, n := range nodes {
		net := n.Native().Network()
		net.RegisterType(ping{})
		net.RegisterType(pong{})
		net.RegisterType(pingRequest{})
	}
}

// TestStageTwoNodes: two nodes, a worker on each, cross-node messaging and a
// cross-node request. Verifies egress (sender side) and ingress (receiver side)
// observation plus an active request via the node API.
func TestStageTwoNodes(t *testing.T) {
	s := stage.New(t)
	a := s.Node("a")
	b := s.Node("b")
	s.Connect(a, b)
	registerWire(a, b)

	pongerPID := b.Spawn(factoryPonger, gen.ProcessOptions{})
	pingerPID := a.Spawn(factoryPinger, gen.ProcessOptions{})

	// active: a cross-node request to ponger@b returns its response
	resp, err := a.Call(pongerPID, pingRequest{Seq: 7})
	check.NoError(t, err)
	check.Equal(t, pong{Seq: 7}, resp)

	// messaging: trigger pinger to send a ping to ponger@b
	a.Send(pingerPID, sendPing{To: pongerPID, Seq: 1})

	// egress observed on a (sender side)
	a.ShouldSend().From(pingerPID).Message(ping{Seq: 1}).Once().Within(time.Second).Must()

	// ingress observed on b (receiver side): the ping landed in ponger's mailbox
	b.ShouldDeliver().To(pongerPID).Message(ping{Seq: 1}).Once().Within(time.Second).Must()

	// negative: ponger never received a different ping
	b.ShouldDeliver().To(pongerPID).Message(ping{Seq: 2}).None().Within(150 * time.Millisecond).Assert()
}

// TestStageMonitorDown: a process monitors a target; killing the target yields a
// Down at the monitor. Verifies egress (monitor setup) and ingress via the
// target-manager bridge (Down delivery, which bypasses gen.Core).
func TestStageMonitorDown(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")

	target := n.Spawn(factoryPonger, gen.ProcessOptions{})
	w := n.Spawn(factoryWatcher, gen.ProcessOptions{}, target)

	// egress: the watcher set up a monitor on the target
	n.ShouldMonitor().From(w).Target(target).Once().Within(time.Second).Must()

	// ingress: killing the target delivers a Down to the watcher
	s.Kill(n, target)
	n.ShouldReceiveDown().To(w).About(target).Reason(gen.TerminateReasonKill).
		Once().Within(time.Second).Must()

	// quiescence: after the Down, the monitor does not re-fire (the correct
	// "no second Down" negative, scoped past the legit one via Since).
	m := n.Mark()
	n.ShouldReceiveDown().To(w).About(target).Since(m).None().Within(150 * time.Millisecond).Assert()
}

// TestStageSince: a repeated identical action is counted per-phase via Since,
// while the cumulative view sees both.
func TestStageSince(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	ponger := n.Spawn(factoryPonger, gen.ProcessOptions{})

	m1 := n.Mark()
	n.Send(ponger, ping{Seq: 1})
	n.ShouldDeliver().To(ponger).Message(ping{Seq: 1}).Since(m1).Once().Within(time.Second).Must()

	// second phase: Since scopes to this phase, so still exactly one
	m2 := n.Mark()
	n.Send(ponger, ping{Seq: 1})
	n.ShouldDeliver().To(ponger).Message(ping{Seq: 1}).Since(m2).Once().Within(time.Second).Must()

	// cumulative (no Since) sees both deliveries
	n.ShouldDeliver().To(ponger).Message(ping{Seq: 1}).Times(2).Within(time.Second).Must()
}

// TestStageSendPriority: the effective send priority is observable on the egress
// Sent record in stage exactly as in unit (same ShouldSend().Priority() assertion).
func TestStageSendPriority(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	target := n.Spawn(factoryPonger, gen.ProcessOptions{})
	sender := n.Spawn(factoryPinger, gen.ProcessOptions{})

	n.Send(sender, sendPingHigh{To: target, Seq: 1})
	n.ShouldSend().From(sender).Message(ping{Seq: 1}).
		Priority(gen.MessagePriorityHigh).Once().Within(time.Second).Must()
}

// TestStageTwoApps: two nodes, each running its own application; the apps'
// processes communicate. Verifies the real-app path (framework-spawned,
// supervised, name-registered processes are observed) end to end.
func TestStageTwoApps(t *testing.T) {
	s := stage.New(t)
	a := s.Node("a", stage.NodeOptions{Applications: []gen.ApplicationBehavior{createApp1()}})
	b := s.Node("b", stage.NodeOptions{Applications: []gen.ApplicationBehavior{createApp2()}})
	s.Connect(a, b)
	registerWire(a, b)

	service1, err := a.ProcessPID("service1")
	check.NoError(t, err)
	service2, err := b.ProcessPID("service2")
	check.NoError(t, err)

	// active: cross-node request to app2's service returns its response
	resp, err := a.Call(service2, pingRequest{Seq: 9})
	check.NoError(t, err)
	check.Equal(t, pong{Seq: 9}, resp)

	// app-to-app messaging: trigger service1 to message service2@b
	m := a.Mark()
	a.Send(service1, sendPing{To: service2, Seq: 5})
	a.ShouldSend().From(service1).Message(ping{Seq: 5}).Since(m).Once().Within(time.Second).Must()
	b.ShouldDeliver().To(service2).Message(ping{Seq: 5}).Once().Within(time.Second).Must()
}

// regSub subscribes to the registrar event stream in Init.
type regSub struct{ act.Actor }

func factoryRegSub() gen.ProcessBehavior { return &regSub{} }

func (s *regSub) Init(args ...any) error {
	reg, err := s.Node().Network().Registrar()
	if err != nil {
		return err
	}
	ev, err := reg.Event()
	if err != nil {
		return err
	}
	_, err = s.MonitorEvent(ev)
	return err
}

// TestStageRegistrarFull: with RegistrarFull, the in-memory registrar serves
// ResolveApplication and produces the canonical gen.MessageRegistrar* event stream;
// a subscriber receives ApplicationStarted when an application route is registered.
func TestStageRegistrarFull(t *testing.T) {
	s := stage.New(t, stage.StageOptions{RegistrarFull: true})
	n := s.Node("n")
	sub := n.Spawn(factoryRegSub, gen.ProcessOptions{})
	mk := n.Mark()

	reg, err := n.Native().Network().Registrar()
	check.NoError(t, err)
	route := gen.ApplicationRoute{Name: "myapp", Node: n.Name(), State: gen.ApplicationStateRunning}
	check.NoError(t, reg.RegisterApplicationRoute(route))

	n.ShouldReceiveEvent().To(sub).Where(func(e check.Event) bool {
		m, ok := e.Message.(gen.MessageRegistrarApplicationStarted)
		return ok && m.Route.Name == "myapp" && m.Route.Node == n.Name()
	}).Since(mk).Once().Within(time.Second).Must()

	routes, err := reg.Resolver().ResolveApplication("myapp")
	check.NoError(t, err)
	check.Equal(t, 1, len(routes))
	check.Equal(t, gen.ApplicationStateRunning, routes[0].State)
}
