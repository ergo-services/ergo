package unit_test

import (
	"errors"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

var errBoom = errors.New("boom")

type sample struct{ act.Actor }

func factorySample() gen.ProcessBehavior { return &sample{} }

func (s *sample) Init(args ...any) error {
	s.Send(s.PID(), "init-sent")
	return nil
}

func (s *sample) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "ping":
		resp, err := s.Call(gen.Atom("backend"), "q")
		if err != nil {
			s.Send(gen.Atom("logger"), "backend failed") // negative reaction
			return nil
		}
		s.Send(gen.Atom("client"), resp)
		return nil
	case "spawn":
		pid, _ := s.Spawn(factorySample, gen.ProcessOptions{})
		s.Send(gen.Atom("mgr"), pid) // forward the generated artifact
		return nil
	case "die":
		return errBoom
	}
	return nil
}

func (s *sample) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return "pong", nil
}

// positive: stubbed Call responds, actor forwards the response
func TestSmokePositive(t *testing.T) {
	a, err := unit.Spawn(t, factorySample, gen.ProcessOptions{})
	check.NoError(t, err)
	a.ShouldSend().To(a.PID()).Message("init-sent").Once().Assert()

	a.OnCall(gen.Atom("backend")).Respond("OK")
	a.SendMessage(gen.PID{}, "ping")
	a.ShouldCall().To(gen.Atom("backend")).Once().Assert()
	a.ShouldSend().To(gen.Atom("client")).Message("OK").Once().Assert()
	a.ShouldTerminate().None().Assert()
}

// negative: stubbed Call fails, actor takes the error branch and does NOT forward
func TestSmokeNegative(t *testing.T) {
	a, _ := unit.Spawn(t, factorySample, gen.ProcessOptions{})
	a.OnCall(gen.Atom("backend")).Fail(gen.ErrTimeout)
	a.SendMessage(gen.PID{}, "ping")
	a.ShouldSend().To(gen.Atom("logger")).Message("backend failed").Once().Assert()
	a.ShouldSend().To(gen.Atom("client")).None().Assert()
}

// OnCall(...).Where(matcher) selects the responding stub by request content. The
// non-matching stub is skipped even though it is registered last, proving Where is
// evaluated against the actual request.
func TestSmokeCallWhereMatcher(t *testing.T) {
	a, err := unit.Spawn(t, factorySample, gen.ProcessOptions{})
	check.NoError(t, err)
	a.OnCall(gen.Atom("backend")).Where(check.Equals("q")).Respond("OK")
	a.OnCall(gen.Atom("backend")).Where(check.MatchedBy(func(r any) bool { return r == "nomatch" })).Respond("WRONG")
	a.SendMessage(gen.PID{}, "ping")
	a.ShouldSend().To(gen.Atom("client")).Message("OK").Once().Assert()
	a.ShouldSend().To(gen.Atom("client")).Message("WRONG").None().Assert()
}

// discoverer resolves an application via the registrar and acts on a running route.
type discoverer struct{ act.Actor }

func factoryDiscoverer() gen.ProcessBehavior { return &discoverer{} }

func (d *discoverer) HandleMessage(from gen.PID, message any) error {
	reg, err := d.Node().Network().Registrar()
	if err != nil {
		d.Send(gen.Atom("logger"), "no-registrar")
		return nil
	}
	routes, err := reg.Resolver().ResolveApplication(gen.Atom("worker_app"))
	if err != nil {
		return nil
	}
	for _, r := range routes {
		if r.State == gen.ApplicationStateRunning {
			d.Send(gen.ProcessID{Name: "manager", Node: r.Node}, "spawn")
			return nil
		}
	}
	return nil
}

// the built-in mock network stubs registrar/resolver discovery per name
func TestSmokeMockNetworkResolveApplication(t *testing.T) {
	a, err := unit.Spawn(t, factoryDiscoverer, gen.ProcessOptions{})
	check.NoError(t, err)
	a.Node().Network().Registrar().Resolver().OnResolveApplication(gen.Atom("worker_app")).Return(
		gen.ApplicationRoute{Node: "node2@localhost", State: gen.ApplicationStateLoaded},
		gen.ApplicationRoute{Node: "node1@localhost", State: gen.ApplicationStateRunning},
	)
	a.SendMessage(gen.PID{}, "go")
	a.ShouldSend().To(gen.ProcessID{Name: "manager", Node: "node1@localhost"}).Message("spawn").Once().Assert()
}

// FailRegistrar drives the no-registrar branch
func TestSmokeMockNetworkFailRegistrar(t *testing.T) {
	a, _ := unit.Spawn(t, factoryDiscoverer, gen.ProcessOptions{})
	a.Node().Network().FailRegistrar(gen.ErrUnsupported)
	a.SendMessage(gen.PID{}, "go")
	a.ShouldSend().To(gen.Atom("logger")).Message("no-registrar").Once().Assert()
}

// regWatcher subscribes to the registrar event in Init and reacts to a canonical
// application-started message by forwarding the app name.
type regWatcher struct{ act.Actor }

func factoryRegWatcher() gen.ProcessBehavior { return &regWatcher{} }

func (w *regWatcher) Init(args ...any) error {
	reg, err := w.Node().Network().Registrar()
	if err != nil {
		return err
	}
	ev, err := reg.Event()
	if err != nil {
		return err
	}
	_, err = w.MonitorEvent(ev)
	return err
}

func (w *regWatcher) HandleEvent(message gen.MessageEvent) error {
	if m, ok := message.Message.(gen.MessageRegistrarApplicationStarted); ok {
		w.Send(gen.Atom("logger"), m.Route.Name)
	}
	return nil
}

// DeliverRegistrarEvent feeds a canonical registrar message through the subscribed event
func TestSmokeDeliverRegistrarEvent(t *testing.T) {
	a, err := unit.Spawn(t, factoryRegWatcher, gen.ProcessOptions{})
	check.NoError(t, err)
	a.DeliverRegistrarEvent(gen.MessageRegistrarApplicationStarted{
		Route: gen.ApplicationRoute{Name: "worker_app", Node: "node1@localhost", State: gen.ApplicationStateRunning},
	})
	a.ShouldSend().To(gen.Atom("logger")).Message(gen.Atom("worker_app")).Once().Assert()
}

// remoteUser spawns on / starts an app on a remote node obtained via GetNode.
type remoteUser struct{ act.Actor }

func factoryRemoteUser() gen.ProcessBehavior { return &remoteUser{} }

func (u *remoteUser) HandleMessage(from gen.PID, message any) error {
	rn, err := u.Node().Network().GetNode("peer@localhost")
	if err != nil {
		u.Send(gen.Atom("logger"), "no-node")
		return nil
	}
	switch message {
	case "spawn":
		rn.Spawn("svc", gen.ProcessOptions{})
	case "start-app":
		if e := rn.ApplicationStartPermanent("app", gen.ApplicationOptions{}); e != nil {
			u.Send(gen.Atom("logger"), "start-failed")
		}
	}
	return nil
}

// remote Spawn / ApplicationStart via RemoteNode are recorded as egress
func TestSmokeRemoteNode(t *testing.T) {
	a, err := unit.Spawn(t, factoryRemoteUser, gen.ProcessOptions{})
	check.NoError(t, err)
	rn := a.Node().Network().OnGetNode("peer@localhost")
	rn.OnSpawn("svc").Return(gen.PID{Node: "peer@localhost", ID: 42})
	rn.OnApplicationStart("app").Fail(gen.ErrNameUnknown)

	a.SendMessage(gen.PID{}, "spawn")
	a.ShouldRemoteSpawn().From(a.PID()).To("peer@localhost").Name("svc").
		Child(gen.PID{Node: "peer@localhost", ID: 42}).Once().Assert()

	a.SendMessage(gen.PID{}, "start-app")
	a.ShouldRemoteApplicationStart().From(a.PID()).To("peer@localhost").Name("app").
		Mode(gen.ApplicationModePermanent).ErrorIs(gen.ErrNameUnknown).Once().Assert()
	a.ShouldSend().To(gen.Atom("logger")).Message("start-failed").Once().Assert()
}

// unstubbed GetNode returns ErrNoConnection
func TestSmokeRemoteNodeNoConnection(t *testing.T) {
	a, _ := unit.Spawn(t, factoryRemoteUser, gen.ProcessOptions{})
	a.SendMessage(gen.PID{}, "spawn")
	a.ShouldSend().To(gen.Atom("logger")).Message("no-node").Once().Assert()
}

// the call into the actor resolves the actor's reply
func TestSmokeCall(t *testing.T) {
	a, _ := unit.Spawn(t, factorySample, gen.ProcessOptions{})
	resp, err := a.Call(gen.PID{}, "hello")
	check.NoError(t, err)
	check.Equal(t, "pong", resp)
}

// generated artifact (spawned pid) is captured and matched in the forwarded message
func TestSmokeSpawnArtifact(t *testing.T) {
	a, _ := unit.Spawn(t, factorySample, gen.ProcessOptions{})
	a.SendMessage(gen.PID{}, "spawn")
	rec, ok := a.ShouldSpawn().Capture()
	check.True(t, ok)
	a.ShouldSend().To(gen.Atom("mgr")).Message(rec.Child).Once().Assert()
}

// artifacts: CreateAlias, RegisterEvent, RemoteSpawn, SpawnMeta now produce records
type artifactor struct{ act.Actor }

func factoryArtifactor() gen.ProcessBehavior { return &artifactor{} }

func (s *artifactor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "create-alias":
		s.CreateAlias()
	case "delete-alias":
		s.DeleteAlias(gen.Alias{})
	case "register-event":
		s.RegisterEvent(gen.Atom("evt"), gen.EventOptions{})
	case "unregister-event":
		s.UnregisterEvent(gen.Atom("evt"))
	case "remote-spawn":
		s.RemoteSpawn(gen.Atom("peer@host"), gen.Atom("worker"), gen.ProcessOptions{})
	case "remote-spawn-register":
		s.RemoteSpawnRegister(gen.Atom("peer@host"), gen.Atom("worker"), gen.Atom("w1"), gen.ProcessOptions{})
	}
	return nil
}

func TestSmokeCreateAliasRecord(t *testing.T) {
	a, err := unit.Spawn(t, factoryArtifactor, gen.ProcessOptions{})
	check.NoError(t, err)
	a.SendMessage(gen.PID{}, "create-alias")
	a.ShouldCreateAlias().From(a.PID()).Once().Assert()
}

func TestSmokeDeleteAliasRecord(t *testing.T) {
	a, _ := unit.Spawn(t, factoryArtifactor, gen.ProcessOptions{})
	a.SendMessage(gen.PID{}, "delete-alias")
	a.ShouldDeleteAlias().From(a.PID()).Once().Assert()
}

func TestSmokeRegisterEventRecord(t *testing.T) {
	a, _ := unit.Spawn(t, factoryArtifactor, gen.ProcessOptions{})
	a.SendMessage(gen.PID{}, "register-event")
	a.ShouldRegisterEvent().From(a.PID()).Name(gen.Atom("evt")).Once().Assert()
}

func TestSmokeUnregisterEventRecord(t *testing.T) {
	a, _ := unit.Spawn(t, factoryArtifactor, gen.ProcessOptions{})
	a.SendMessage(gen.PID{}, "unregister-event")
	a.ShouldUnregisterEvent().From(a.PID()).Name(gen.Atom("evt")).Once().Assert()
}

func TestSmokeRemoteSpawnRecord(t *testing.T) {
	a, _ := unit.Spawn(t, factoryArtifactor, gen.ProcessOptions{})
	a.SendMessage(gen.PID{}, "remote-spawn")
	a.ShouldRemoteSpawn().From(a.PID()).To(gen.Atom("peer@host")).Name(gen.Atom("worker")).Once().Assert()
	a.SendMessage(gen.PID{}, "remote-spawn-register")
	a.ShouldRemoteSpawn().To(gen.Atom("peer@host")).Register(gen.Atom("w1")).Once().Assert()
}

// abnormal return terminates the actor with the reason
func TestSmokeTerminate(t *testing.T) {
	a, _ := unit.Spawn(t, factorySample, gen.ProcessOptions{})
	a.SendMessage(gen.PID{}, "die")
	a.ShouldTerminate().Reason(errBoom).Once().Assert()
}

type timed struct {
	act.Actor
}

func factoryTimed() gen.ProcessBehavior { return &timed{} }

func (a *timed) Init(args ...any) error {
	a.SendAfter(a.PID(), "tick", time.Second)
	return nil
}

func (a *timed) HandleMessage(from gen.PID, message any) error {
	if message == "tick" {
		a.Send(gen.Atom("done"), "fired")
	}
	return nil
}

// actor that spawns a child and immediately queries the node about it
type inspector struct {
	act.Actor
	childName gen.Atom
	childErr  error
}

func factoryInspector() gen.ProcessBehavior { return &inspector{} }

func (a *inspector) HandleMessage(from gen.PID, message any) error {
	if message == "spawn-and-inspect" {
		pid, _ := a.Spawn(factorySample, gen.ProcessOptions{})
		info, err := a.Node().ProcessInfo(pid)
		a.childName = info.Name
		a.childErr = err
	}
	return nil
}

// a process can spawn a child and query Node().ProcessInfo about it in the same
// callback: the mock node answers from its registry.
func TestSmokeProcessInfoAfterSpawn(t *testing.T) {
	s, _ := unit.Spawn(t, factoryInspector, gen.ProcessOptions{})
	s.SendMessage(gen.PID{}, "spawn-and-inspect")

	b := s.Behavior().(*inspector)
	check.NoError(t, b.childErr)

	// an unknown pid yields ErrProcessUnknown, just like a real node
	_, err := s.Node().ProcessInfo(gen.PID{Node: "x@y", ID: 999})
	check.ErrorIs(t, err, gen.ErrProcessUnknown)
}

// process-method override: sub.OnEnv supplies a value the behavior reads.
type envReader struct {
	act.Actor
	got any
}

func factoryEnvReader() gen.ProcessBehavior { return &envReader{} }

func (a *envReader) HandleMessage(from gen.PID, message any) error {
	a.got, _ = a.Env(gen.Env("k"))
	return nil
}

func TestSmokeProcessEnvOverride(t *testing.T) {
	s, _ := unit.Spawn(t, factoryEnvReader, gen.ProcessOptions{})
	s.OnEnv(func(name gen.Env) (any, bool) { return "overridden", true })
	s.SendMessage(gen.PID{}, "go")
	check.Equal(t, "overridden", s.Behavior().(*envReader).got)
}

// node-method override: sub.Node().OnIsAlive flips what the behavior observes.
type aliveChecker struct {
	act.Actor
	alive bool
}

func factoryAliveChecker() gen.ProcessBehavior { return &aliveChecker{alive: true} }

func (a *aliveChecker) HandleMessage(from gen.PID, message any) error {
	a.alive = a.Node().IsAlive()
	return nil
}

func TestSmokeNodeIsAliveOverride(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAliveChecker, gen.ProcessOptions{})
	s.Node().OnIsAlive(func() bool { return false })
	s.SendMessage(gen.PID{}, "go")
	check.False(t, s.Behavior().(*aliveChecker).alive)
}

// selective failure: FailFunc fails only the 2nd and 5th send; all six are recorded.
type burster struct {
	act.Actor
	errs []error
}

func factoryBurster() gen.ProcessBehavior { return &burster{} }

func (a *burster) HandleMessage(from gen.PID, message any) error {
	for i := 0; i < 6; i++ {
		a.errs = append(a.errs, a.Send(gen.Atom("svc"), i))
	}
	return nil
}

func TestSmokeSendFailFunc(t *testing.T) {
	s, _ := unit.Spawn(t, factoryBurster, gen.ProcessOptions{})
	i := 0
	s.OnSend(gen.Atom("svc")).FailFunc(func() error {
		i++
		if i == 2 || i == 5 {
			return gen.ErrProcessMailboxFull
		}
		return nil
	})
	s.SendMessage(gen.PID{}, "burst")

	b := s.Behavior().(*burster)
	check.NoError(t, b.errs[0])
	check.ErrorIs(t, b.errs[1], gen.ErrProcessMailboxFull)
	check.NoError(t, b.errs[2])
	check.NoError(t, b.errs[3])
	check.ErrorIs(t, b.errs[4], gen.ErrProcessMailboxFull)
	check.NoError(t, b.errs[5])
	s.ShouldSend().To(gen.Atom("svc")).Times(6).Assert() // all recorded regardless
}

// send priority is stateful: SetSendPriority changes the priority subsequent plain
// Sends carry; SendWithPriority overrides one send via options without mutating the
// stored default; the default is seeded from ProcessOptions; invalid values are
// rejected exactly as the real process does.
type prio struct {
	act.Actor
	setErr       error
	afterDefault gen.MessagePriority
}

func factoryPrio() gen.ProcessBehavior { return &prio{} }

func (a *prio) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "set-high-send":
		a.SetSendPriority(gen.MessagePriorityHigh)
		a.Send(gen.Atom("svc"), "x")
	case "with-max":
		a.SendWithPriority(gen.Atom("svc"), "y", gen.MessagePriorityMax)
		a.afterDefault = a.SendPriority() // must be unchanged by the one-shot override
	case "plain":
		a.Send(gen.Atom("svc"), "z")
	case "set-invalid":
		a.setErr = a.SetSendPriority(gen.MessagePriority(99))
	}
	return nil
}

func TestSmokeSendPriorityStateful(t *testing.T) {
	s, _ := unit.Spawn(t, factoryPrio, gen.ProcessOptions{})
	s.SendMessage(gen.PID{}, "set-high-send")
	s.ShouldSend().To(gen.Atom("svc")).Message("x").Priority(gen.MessagePriorityHigh).Once().Assert()
}

func TestSmokeSendWithPriorityNoStateChange(t *testing.T) {
	s, _ := unit.Spawn(t, factoryPrio, gen.ProcessOptions{})
	s.SendMessage(gen.PID{}, "with-max")
	s.ShouldSend().Message("y").Priority(gen.MessagePriorityMax).Once().Assert()
	check.Equal(t, gen.MessagePriorityNormal, s.Behavior().(*prio).afterDefault)
}

func TestSmokeSendPrioritySeeded(t *testing.T) {
	s, _ := unit.Spawn(t, factoryPrio, gen.ProcessOptions{SendPriority: gen.MessagePriorityHigh})
	s.SendMessage(gen.PID{}, "plain")
	s.ShouldSend().Message("z").Priority(gen.MessagePriorityHigh).Once().Assert()
}

func TestSmokeSetSendPriorityInvalid(t *testing.T) {
	s, _ := unit.Spawn(t, factoryPrio, gen.ProcessOptions{})
	s.SendMessage(gen.PID{}, "set-invalid")
	check.ErrorIs(t, s.Behavior().(*prio).setErr, gen.ErrIncorrect)
}

// the mock logger gates by level like the real one: lines below the configured
// level are not recorded; SetLevel validates its argument.
type logActor struct {
	act.Actor
	setErr error
}

func factoryLogActor() gen.ProcessBehavior { return &logActor{} }

func (a *logActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "log":
		a.Log().Info("info line")
		a.Log().Error("error line")
	case "set-trace":
		a.setErr = a.Log().SetLevel(gen.LogLevelTrace)
	}
	return nil
}

func TestSmokeLogLevelGate(t *testing.T) {
	s, _ := unit.Spawn(t, factoryLogActor, gen.ProcessOptions{LogLevel: gen.LogLevelError})
	s.SendMessage(gen.PID{}, "log")
	s.ShouldLog().Message("info line").None().Assert()  // below Error -> dropped
	s.ShouldLog().Message("error line").Once().Assert() // at Error -> recorded
}

func TestSmokeSetLevelRejectsTrace(t *testing.T) {
	s, _ := unit.Spawn(t, factoryLogActor, gen.ProcessOptions{})
	s.SendMessage(gen.PID{}, "set-trace")
	check.ErrorIs(t, s.Behavior().(*logActor).setErr, gen.ErrIncorrect)
}

// a test can override a node introspection method when the default does not fit.
func TestSmokeProcessInfoOverride(t *testing.T) {
	n := unit.Node(t, "unit@localhost", gen.NodeOptions{})
	n.OnProcessInfo(func(pid gen.PID) (gen.ProcessInfo, error) {
		return gen.ProcessInfo{Name: "overridden"}, nil
	})
	s, _ := n.Spawn(factoryInspector, gen.ProcessOptions{})
	s.SendMessage(gen.PID{}, "spawn-and-inspect")

	b := s.Behavior().(*inspector)
	check.Equal(t, gen.Atom("overridden"), b.childName)
}

// SendAfter is recorded as a SendAfter and not delivered until FireTimers.
func TestSmokeTimer(t *testing.T) {
	a, _ := unit.Spawn(t, factoryTimed, gen.ProcessOptions{})
	a.ShouldSendAfter().To(a.PID()).Message("tick").Once().Assert()
	a.ShouldSend().To(gen.Atom("done")).None().Assert()

	fired := a.FireTimers()
	check.Equal(t, 1, fired)
	a.ShouldSend().To(gen.Atom("done")).Message("fired").Once().Assert()
}

// cron jobs are recorded as AddCronJob/RemoveCronJob and only fire on FireCron.
type cronActor struct {
	act.Actor
	fired int
}

func factoryCron() gen.ProcessBehavior { return &cronActor{} }

func (c *cronActor) Init(args ...any) error {
	return c.Node().Cron().AddJob(gen.CronJob{Name: gen.Atom("nightly"), Spec: "0 0 * * *"})
}

func (c *cronActor) HandleMessage(from gen.PID, message any) error {
	if m, ok := message.(gen.MessageCron); ok {
		c.fired++
		c.Send(gen.Atom("reporter"), m.Job)
	}
	return nil
}

func TestSmokeCron(t *testing.T) {
	a, _ := unit.Spawn(t, factoryCron, gen.ProcessOptions{})
	a.ShouldAddCronJob().Name(gen.Atom("nightly")).Spec("0 0 * * *").Once().Assert()
	a.ShouldSend().To(gen.Atom("reporter")).None().Assert() // not fired yet

	a.FireCron(gen.Atom("nightly"))
	check.Equal(t, 1, a.Behavior().(*cronActor).fired)
	a.ShouldSend().To(gen.Atom("reporter")).Message(gen.Atom("nightly")).Once().Assert()
}

func TestSmokeCronRemove(t *testing.T) {
	a, _ := unit.Spawn(t, factoryCron, gen.ProcessOptions{})
	check.NoError(t, a.Node().Cron().RemoveJob(gen.Atom("nightly")))
	a.ShouldRemoveCronJob().Name(gen.Atom("nightly")).Once().Assert()
	check.ErrorIs(t, a.Node().Cron().RemoveJob(gen.Atom("nightly")), gen.ErrUnknown)
}

// SendExitMeta egress: the stub error is captured in the check.SendExitMeta record
// alongside the exit reason (C: unit parity with SendExit).
type exiterActor struct{ act.Actor }

func factoryExiter() gen.ProcessBehavior { return &exiterActor{} }

func (a *exiterActor) HandleMessage(from gen.PID, message any) error {
	if alias, ok := message.(gen.Alias); ok {
		a.SendExitMeta(alias, gen.TerminateReasonShutdown)
	}
	return nil
}

func TestSmokeSendExitMeta(t *testing.T) {
	s, _ := unit.Spawn(t, factoryExiter, gen.ProcessOptions{})
	alias := gen.Alias{Node: "unit@localhost", Creation: 1, ID: [3]uint64{7, 0, 0}}
	s.OnSendExitMeta(alias).Fail(gen.ErrProcessUnknown)

	s.SendMessage(gen.PID{}, alias)

	s.ShouldSendExitMeta().Meta(alias).
		ErrorIs(gen.ErrProcessUnknown).
		ReasonIs(gen.TerminateReasonShutdown).
		Once().Assert()
}
