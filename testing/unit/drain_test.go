package unit_test

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

type sessionReady struct{}
type sessionWarm struct{}

// selfStarter is the idiomatic actor whose Init only posts to itself, because Call,
// Link, Monitor and RegisterName are not available in the Init state.
type selfStarter struct {
	act.Actor
	state string
	hops  int
}

func factorySelfStarter() gen.ProcessBehavior { return &selfStarter{} }

func (s *selfStarter) Init(args ...any) error {
	return s.Send(s.PID(), sessionReady{})
}

func (s *selfStarter) HandleMessage(from gen.PID, message any) error {
	s.hops++
	switch message.(type) {
	case sessionReady:
		s.state = "ready"
		return s.Send(s.PID(), sessionWarm{})
	case sessionWarm:
		s.state = "warm"
		return s.Send("collector", "up")
	}
	return nil
}

// Drain runs the whole Init chain, as a live node does on its own.
func TestDrainRunsInitChain(t *testing.T) {
	a, err := unit.Spawn(t, factorySelfStarter, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b := a.Behavior().(*selfStarter)
	check.Equal(t, "", b.state)

	check.Equal(t, 2, a.Drain())
	check.Equal(t, "warm", b.state)
	check.Equal(t, 2, b.hops)

	// the self-sends stay observable, and the outbound one went out
	a.ShouldSend().To(a.PID()).Message(sessionReady{}).Once().Assert()
	a.ShouldSend().To(a.PID()).Message(sessionWarm{}).Once().Assert()
	a.ShouldSend().To("collector").Message("up").Once().Assert()

	// nothing left to deliver
	check.Equal(t, 0, a.Drain())
}

// Step advances the chain one hop at a time.
func TestStepOneHop(t *testing.T) {
	a, _ := unit.Spawn(t, factorySelfStarter, gen.ProcessOptions{})
	b := a.Behavior().(*selfStarter)

	check.True(t, a.Step())
	check.Equal(t, "ready", b.state)

	check.True(t, a.Step())
	check.Equal(t, "warm", b.state)

	check.False(t, a.Step())
}

// A message the actor sent elsewhere is recorded, never delivered back to it.
func TestDrainLeavesOutboundAlone(t *testing.T) {
	a, _ := unit.Spawn(t, factorySelfStarter, gen.ProcessOptions{})
	a.Drain()
	before := a.Behavior().(*selfStarter).hops

	a.ShouldSend().To("collector").Once().Assert()
	check.Equal(t, 0, a.Drain())
	check.Equal(t, before, a.Behavior().(*selfStarter).hops)
}

// prioritySelf posts to itself at a priority and records the arrival order.
type prioritySelf struct {
	act.Actor
	order []string
}

func factoryPrioritySelf() gen.ProcessBehavior { return &prioritySelf{} }

func (p *prioritySelf) Init(args ...any) error {
	if err := p.SendWithPriority(p.PID(), "normal", gen.MessagePriorityNormal); err != nil {
		return err
	}
	return p.SendWithPriority(p.PID(), "urgent", gen.MessagePriorityMax)
}

func (p *prioritySelf) HandleMessage(from gen.PID, message any) error {
	p.order = append(p.order, message.(string))
	return nil
}

// A self-send lands in the queue for its priority, so the urgent one is handled
// first even though it was sent last.
func TestDrainHonorsPriority(t *testing.T) {
	a, _ := unit.Spawn(t, factoryPrioritySelf, gen.ProcessOptions{})
	check.Equal(t, 2, a.Drain())
	check.Equal(t, []string{"urgent", "normal"}, a.Behavior().(*prioritySelf).order)
}

// aliasSelf posts to its own alias, which the split handler receives.
type aliasSelf struct {
	act.Actor
	viaAlias int
	viaPID   int
}

func factoryAliasSelf() gen.ProcessBehavior { return &aliasSelf{} }

func (a *aliasSelf) Init(args ...any) error {
	a.SetSplitHandle(true)
	alias, err := a.CreateAlias()
	if err != nil {
		return err
	}
	return a.Send(alias, "via-alias")
}

func (a *aliasSelf) HandleMessage(from gen.PID, message any) error {
	a.viaPID++
	return nil
}

func (a *aliasSelf) HandleMessageAlias(alias gen.Alias, from gen.PID, message any) error {
	a.viaAlias++
	return nil
}

// A send to the actor's own alias is delivered as an alias-addressed message.
func TestDrainDeliversToOwnAlias(t *testing.T) {
	a, _ := unit.Spawn(t, factoryAliasSelf, gen.ProcessOptions{})
	check.Equal(t, 1, a.Drain())

	b := a.Behavior().(*aliasSelf)
	check.Equal(t, 1, b.viaAlias)
	check.Equal(t, 0, b.viaPID)
}

// namedSelf posts to its own registered name.
type namedSelf struct {
	act.Actor
	got int
}

func factoryNamedSelf() gen.ProcessBehavior { return &namedSelf{} }

func (n *namedSelf) Init(args ...any) error { return n.Send(gen.Atom("worker"), "by-name") }

func (n *namedSelf) HandleMessage(from gen.PID, message any) error {
	n.got++
	return nil
}

// A send addressed to the actor's own registered name is a self-send too.
func TestDrainDeliversToOwnName(t *testing.T) {
	a, err := unit.SpawnRegister(t, "worker", factoryNamedSelf, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	check.Equal(t, 1, a.Drain())
	check.Equal(t, 1, a.Behavior().(*namedSelf).got)
}

// terminatingSelf stops itself on the second hop.
type terminatingSelf struct {
	act.Actor
	hops int
}

func factoryTerminatingSelf() gen.ProcessBehavior { return &terminatingSelf{} }

func (s *terminatingSelf) Init(args ...any) error { return s.Send(s.PID(), sessionReady{}) }

func (s *terminatingSelf) HandleMessage(from gen.PID, message any) error {
	s.hops++
	switch message.(type) {
	case sessionReady:
		return s.Send(s.PID(), sessionWarm{})
	case sessionWarm:
		return gen.TerminateReasonNormal
	}
	return nil
}

// Drain stops when the actor terminates mid-chain.
func TestDrainStopsOnTermination(t *testing.T) {
	a, _ := unit.Spawn(t, factoryTerminatingSelf, gen.ProcessOptions{})
	a.Drain()

	check.Equal(t, 2, a.Behavior().(*terminatingSelf).hops)
	a.ShouldTerminate().Normally().Once().Assert()
	check.Equal(t, 0, a.Drain())
}

// metaHeartbeat posts to its parent actor from Init.
type metaHeartbeat struct{ mp gen.MetaProcess }

func (m *metaHeartbeat) Init(process gen.MetaProcess) error {
	m.mp = process
	return m.mp.Send(m.mp.Parent(), "meta-up")
}

func (m *metaHeartbeat) Start() error                                  { return nil }
func (m *metaHeartbeat) HandleMessage(from gen.PID, message any) error { return nil }
func (m *metaHeartbeat) Terminate(reason error)                        {}
func (m *metaHeartbeat) HandleInspect(from gen.PID, item ...string) map[string]string {
	return nil
}

func (m *metaHeartbeat) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

// counterParent counts what its meta sends it.
type counterParent struct {
	act.Actor
	got []any
}

func factoryCounterParent() gen.ProcessBehavior { return &counterParent{} }

func (c *counterParent) HandleMessage(from gen.PID, message any) error {
	c.got = append(c.got, message)
	return nil
}

// A message a meta sends to its parent is addressed to the process under test, so
// Drain delivers it, as the runtime does.
func TestDrainDeliversMetaToParent(t *testing.T) {
	sub, err := unit.Spawn(t, factoryCounterParent, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sub.SpawnMeta(&metaHeartbeat{}, gen.MetaOptions{}); err != nil {
		t.Fatal(err)
	}

	check.Equal(t, 1, sub.Drain())
	check.Equal(t, []any{"meta-up"}, sub.Behavior().(*counterParent).got)
}
