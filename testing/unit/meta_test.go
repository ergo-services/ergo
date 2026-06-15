package unit_test

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

// metaParent is a do-nothing actor that only serves as the parent of a meta.
type metaParent struct{ act.Actor }

func factoryMetaParent() gen.ProcessBehavior { return &metaParent{} }

// echoMeta exercises the full meta behavior contract: Init wires the process and
// sends a greeting, HandleMessage echoes to a client, HandleCall responds,
// HandleInspect reports state, Terminate records the reason.
type echoMeta struct {
	mp         gen.MetaProcess
	greeted    bool
	terminated error
}

func (e *echoMeta) Init(process gen.MetaProcess) error {
	e.mp = process
	e.greeted = true
	// Init runs in the pre-running state: Send is allowed.
	return e.mp.Send(gen.Atom("supervisor"), "meta-up")
}

func (e *echoMeta) Start() error { return nil }

func (e *echoMeta) HandleMessage(from gen.PID, message any) error {
	if s, ok := message.(string); ok {
		e.mp.Send(gen.Atom("client"), "got:"+s)
	}
	return nil
}

func (e *echoMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return "pong", nil
}

func (e *echoMeta) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{"state": "ok"}
}

func (e *echoMeta) Terminate(reason error) { e.terminated = reason }

func TestSmokeMeta(t *testing.T) {
	sub, err := unit.Spawn(t, factoryMetaParent, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	m, err := sub.SpawnMeta(&echoMeta{}, gen.MetaOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Init egress: greeting sent from the parent PID.
	m.ShouldSend().To(gen.Atom("supervisor")).Message("meta-up").Once().Assert()
	check.Equal(t, gen.MetaStateSleep, m.State())

	m.DeliverMessage(sub.PID(), "hello")
	m.ShouldSend().To(gen.Atom("client")).Message("got:hello").Once().Assert()

	resp, err := m.Request(sub.PID(), "ping")
	check.NoError(t, err)
	check.Equal(t, "pong", resp)
	m.ShouldSendResponse().Message("pong").Once().Assert()

	info := m.Inspect(sub.PID(), "state")
	check.Equal(t, "ok", info["state"])

	m.Terminate(gen.TerminateReasonNormal)
	check.Equal(t, gen.MetaStateTerminated, m.State())
	check.ErrorIs(t, m.Behavior().(*echoMeta).terminated, gen.TerminateReasonNormal)
}

// gateMeta tries SendResponse from HandleMessage (Running, allowed) and exposes a
// hook to call it outside any callback (Sleep, must be rejected).
type gateMeta struct {
	mp     gen.MetaProcess
	inMsg  error
	atRest error
}

func (g *gateMeta) Init(process gen.MetaProcess) error { g.mp = process; return nil }
func (g *gateMeta) Start() error                       { return nil }

func (g *gateMeta) HandleMessage(from gen.PID, message any) error {
	g.inMsg = g.mp.SendResponse(from, gen.Ref{}, "resp")
	return nil
}

func (g *gateMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}
func (g *gateMeta) HandleInspect(from gen.PID, item ...string) map[string]string { return nil }
func (g *gateMeta) Terminate(reason error)                                       {}

func TestSmokeMetaStateGate(t *testing.T) {
	sub, _ := unit.Spawn(t, factoryMetaParent, gen.ProcessOptions{})
	m, err := sub.SpawnMeta(&gateMeta{}, gen.MetaOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b := m.Behavior().(*gateMeta)

	// SendResponse at rest (Sleep) is rejected.
	b.atRest = b.mp.SendResponse(sub.PID(), gen.Ref{}, "x")
	check.ErrorIs(t, b.atRest, gen.ErrNotAllowed)

	// SendResponse inside HandleMessage (Running) is allowed.
	m.DeliverMessage(sub.PID(), "go")
	check.NoError(t, b.inMsg)
	m.ShouldSendResponse().Message("resp").Once().Assert()
}

// childSpawner spawns a child meta in Init.
type childSpawner struct {
	mp    gen.MetaProcess
	child gen.Alias
}

func (c *childSpawner) Init(process gen.MetaProcess) error {
	c.mp = process
	alias, err := c.mp.Spawn(&echoMeta{}, gen.MetaOptions{})
	c.child = alias
	return err
}
func (c *childSpawner) Start() error                                                 { return nil }
func (c *childSpawner) HandleMessage(from gen.PID, message any) error                { return nil }
func (c *childSpawner) HandleCall(from gen.PID, ref gen.Ref, r any) (any, error)     { return nil, nil }
func (c *childSpawner) HandleInspect(from gen.PID, item ...string) map[string]string { return nil }
func (c *childSpawner) Terminate(reason error)                                       {}

func TestSmokeMetaChildSpawn(t *testing.T) {
	sub, _ := unit.Spawn(t, factoryMetaParent, gen.ProcessOptions{})
	m, err := sub.SpawnMeta(&childSpawner{}, gen.MetaOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m.ShouldSpawnMeta().From(sub.PID()).Once().Assert()
	check.NotEqual(t, gen.Alias{}, m.Behavior().(*childSpawner).child)
}
