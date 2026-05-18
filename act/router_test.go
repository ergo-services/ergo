package act

import (
	"errors"
	"strings"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

//
// Worker factories used by router tests
//

type routerTestWorker struct{ Actor }

func factoryRouterWorkerA() gen.ProcessBehavior { return &routerTestWorker{} }
func factoryRouterWorkerB() gen.ProcessBehavior { return &routerTestWorker{} }
func factoryRouterWorkerC() gen.ProcessBehavior { return &routerTestWorker{} }

//
// Test router with hooks for per-test routing/handling logic.
//

type testRouter struct {
	Router

	initRoutes  []Route
	routeTarget gen.Atom // returned from RouteMessage (empty = RouteDiscard)
	callTarget  gen.Atom // returned from RouteCall (empty = RouteDiscard)

	// Captured for verification.
	routeFailed []MessageRouteFailed
}

func (r *testRouter) Init(args ...any) (RouterOptions, error) {
	return RouterOptions{Routes: r.initRoutes}, nil
}

func (r *testRouter) RouteMessage(from gen.PID, msg any) gen.Atom {
	return r.routeTarget
}

func (r *testRouter) RouteCall(from gen.PID, ref gen.Ref, req any) gen.Atom {
	return r.callTarget
}

func (r *testRouter) HandleMessage(from gen.PID, msg any) error {
	if mrf, ok := msg.(MessageRouteFailed); ok {
		r.routeFailed = append(r.routeFailed, mrf)
	}
	return nil
}

// makeRouterFactory wraps a testRouter instance in a ProcessFactory closure.
func makeRouterFactory(r *testRouter) gen.ProcessFactory {
	return func() gen.ProcessBehavior { return r }
}

// forwardedPayload extracts the user-level message from a SendEvent.Message
// emitted by Forward (which wraps the user message in *gen.MailboxMessage).
func forwardedPayload(m any) any {
	if mbm, ok := m.(*gen.MailboxMessage); ok {
		return mbm.Message
	}
	return m
}

// pidFromRouterRoute returns the PID of a named route from a testRouter.
func pidFromRouterRoute(t *testing.T, behavior gen.ProcessBehavior, name gen.Atom) gen.PID {
	t.Helper()
	tr, ok := behavior.(*testRouter)
	if ok == false {
		t.Fatalf("behavior is not *testRouter: %T", behavior)
	}
	info, found := tr.Route(name)
	if found == false {
		t.Fatalf("route %q not found", name)
	}
	return info.PID
}

//
// T1.x: initialization
//

func TestRouterInitSpawnsAllRoutes(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
			{Name: "b", Factory: factoryRouterWorkerB},
			{Name: "c", Factory: factoryRouterWorkerC},
		},
	}

	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatalf("spawn router: %v", err)
	}

	spawnCount := 0
	for _, ev := range actor.Events() {
		se, ok := ev.(unit.SpawnEvent)
		if ok == false {
			continue
		}
		spawnCount++
		if se.Options.LinkChild == false {
			t.Errorf("spawn #%d: LinkChild=false, want true", spawnCount)
		}
		if se.Options.LinkParent == false {
			t.Errorf("spawn #%d: LinkParent=false, want true", spawnCount)
		}
	}
	unit.Equal(t, 3, spawnCount, "expected 3 spawns at init")

	infos := r.Routes()
	unit.Equal(t, 3, len(infos), "Routes() len")
	unit.Equal(t, gen.Atom("a"), infos[0].Name)
	unit.Equal(t, gen.Atom("b"), infos[1].Name)
	unit.Equal(t, gen.Atom("c"), infos[2].Name)
	for i, info := range infos {
		if info.PID == (gen.PID{}) {
			t.Errorf("info[%d].PID is empty", i)
		}
		if info.Disabled {
			t.Errorf("info[%d].Disabled = true, want false", i)
		}
		if info.Pending != RoutePendingNone {
			t.Errorf("info[%d].Pending = %s, want None", i, info.Pending)
		}
	}
}

func TestRouterInitDuplicateNameRejected(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "x", Factory: factoryRouterWorkerA},
			{Name: "x", Factory: factoryRouterWorkerB},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err == nil {
		t.Fatal("expected init error, got nil")
	}
	if strings.Contains(err.Error(), "duplicate route name") == false {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouterInitEmptyNameRejected(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err == nil {
		t.Fatal("expected init error, got nil")
	}
	if strings.Contains(err.Error(), "can not be empty") == false {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouterInitNilFactoryRejected(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: nil},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err == nil {
		t.Fatal("expected init error, got nil")
	}
	if strings.Contains(err.Error(), "nil Factory") == false {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouterInitEmptyRoutesIsFreeRouter(t *testing.T) {
	r := &testRouter{initRoutes: nil}

	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatalf("spawn free router: %v", err)
	}

	actor.ShouldNotSpawn().Assert()
	unit.Equal(t, 0, len(r.Routes()), "Routes() should be empty for free router")
}

//
// T2.x: Routes() / Route() / HandleInspect
//

func TestRouterRoutesSnapshotOrder(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "first", Factory: factoryRouterWorkerA},
			{Name: "second", Factory: factoryRouterWorkerB},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	infos := r.Routes()
	unit.Equal(t, 2, len(infos))
	unit.Equal(t, gen.Atom("first"), infos[0].Name)
	unit.Equal(t, gen.Atom("second"), infos[1].Name)
}

func TestRouterRouteKnownReturnsInfo(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "alpha", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	info, ok := r.Route("alpha")
	unit.True(t, ok, "Route(alpha) ok")
	unit.Equal(t, gen.Atom("alpha"), info.Name)
	unit.NotNil(t, info.PID, "PID must be non-empty")
	unit.False(t, info.Disabled)
	unit.Equal(t, RoutePendingNone, info.Pending)
}

func TestRouterRouteUnknownReturnsFalse(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "alpha", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	info, ok := r.Route("missing")
	unit.False(t, ok, "Route(missing) ok=false expected")
	unit.Equal(t, RouterRouteInfo{}, info, "info should be zero value")
}

func TestRouterHandleInspectFields(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "x", Factory: factoryRouterWorkerA},
			{Name: "y", Factory: factoryRouterWorkerB},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	result := r.HandleInspect(gen.PID{})
	unit.Equal(t, "Router", result["type"])
	unit.Equal(t, "2", result["routes_total"])
	unit.Equal(t, "2", result["routes_active"])
	unit.Equal(t, "0", result["routes_disabled"])
	unit.Equal(t, "0", result["routes_pending"])
	unit.Equal(t, "0", result["forwarded"])
	unit.Equal(t, "0", result["discarded"])
	unit.Equal(t, "0", result["failed"])
	unit.Equal(t, "0", result["restarts"])

	if _, ok := result["route:x:pid"]; ok == false {
		t.Errorf("missing route:x:pid. keys: %v", keysOf(result))
	}
	if _, ok := result["route:y:pid"]; ok == false {
		t.Errorf("missing route:y:pid. keys: %v", keysOf(result))
	}
	unit.Equal(t, "false", result["route:x:disabled"])
	unit.Equal(t, "false", result["route:y:disabled"])
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

//
// T3.x: RouteMessage / async forwarding
//

func TestRouterCustomRouteMessageForwards(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
			{Name: "b", Factory: factoryRouterWorkerB},
		},
		routeTarget: "b",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	slotBPID := pidFromRouterRoute(t, actor.Behavior(), "b")
	actor.ClearEvents()

	sender := gen.PID{Node: "test@localhost", ID: 9001, Creation: 1}
	actor.SendMessage(sender, "ping")

	actor.ShouldSend().
		To(slotBPID).
		MessageMatching(func(m any) bool {
			return forwardedPayload(m) == "ping"
		}).
		Once().
		Assert()

	result := r.HandleInspect(gen.PID{})
	unit.Equal(t, "1", result["forwarded"])
}

func TestRouterRouteDiscardDropsMessage(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		routeTarget: RouteDiscard,
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()

	actor.SendMessage(gen.PID{Node: "test@localhost", ID: 9002, Creation: 1}, "drop-me")

	actor.ShouldNotSend().Assert()

	result := r.HandleInspect(gen.PID{})
	unit.Equal(t, "1", result["discarded"])
}

func TestRouterUnknownNameDeliversMessageRouteFailed(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		routeTarget: "ghost",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()

	sender := gen.PID{Node: "test@localhost", ID: 9003, Creation: 1}
	actor.SendMessage(sender, "to-ghost")

	unit.Equal(t, 1, len(r.routeFailed), "expected one MessageRouteFailed")
	mrf := r.routeFailed[0]
	unit.Equal(t, gen.Atom("ghost"), mrf.Name)
	unit.Equal(t, sender, mrf.From)
	unit.Equal(t, "to-ghost", mrf.Message)
	unit.True(t, errors.Is(mrf.Reason, gen.ErrProcessUnknown))

	result := r.HandleInspect(gen.PID{})
	unit.Equal(t, "1", result["failed"])
}

func TestRouterForwardPreservesOriginalFrom(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		routeTarget: "a",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()

	sender := gen.PID{Node: "test@localhost", ID: 12345, Creation: 1}
	actor.SendMessage(sender, "msg")

	events := actor.Events()
	found := false
	for _, ev := range events {
		se, ok := ev.(unit.SendEvent)
		if ok == false {
			continue
		}
		mbm, ok := se.Message.(*gen.MailboxMessage)
		if ok == false {
			continue
		}
		unit.Equal(t, sender, mbm.From, "forwarded message must carry original From")
		found = true
	}
	unit.True(t, found, "expected at least one SendEvent from Forward")
}

//
// T4.x: RouteCall / sync forwarding
//

func TestRouterCustomRouteCallForwards(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "calc", Factory: factoryRouterWorkerA},
		},
		callTarget: "calc",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	calcPID := pidFromRouterRoute(t, actor.Behavior(), "calc")
	actor.ClearEvents()

	caller := gen.PID{Node: "test@localhost", ID: 9101, Creation: 1}
	result := actor.Call(caller, "compute")

	actor.ShouldSend().
		To(calcPID).
		MessageMatching(func(m any) bool {
			mbm, ok := m.(*gen.MailboxMessage)
			if ok == false {
				return false
			}
			if mbm.Type != gen.MailboxMessageTypeRequest {
				return false
			}
			return mbm.Message == "compute" && mbm.Ref == result.Ref
		}).
		Once().
		Assert()

	stats := r.HandleInspect(gen.PID{})
	unit.Equal(t, "1", stats["forwarded"])
}

func TestRouterRouteCallDiscardRespondsWithErrDiscarded(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		callTarget: RouteDiscard,
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()

	caller := gen.PID{Node: "test@localhost", ID: 9102, Creation: 1}
	result := actor.Call(caller, "req")
	unit.True(t, errors.Is(result.Error, gen.ErrDiscarded))
}

func TestRouterRouteCallUnknownNameRespondsErrProcessUnknown(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		callTarget: "ghost",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()

	caller := gen.PID{Node: "test@localhost", ID: 9103, Creation: 1}
	result := actor.Call(caller, "req")
	unit.True(t, errors.Is(result.Error, gen.ErrProcessUnknown))
}

//
// T5.x: lazy respawn on Forward failure
//

func TestRouterForwardErrProcessUnknownTriggersLazyRespawn(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		routeTarget: "a",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	actor.ClearEvents()
	actor.Process().SetMethodFailureOnce("Forward", gen.ErrProcessUnknown)

	sender := gen.PID{Node: "test@localhost", ID: 5001, Creation: 1}
	actor.SendMessage(sender, "ping")

	// one new Spawn after init (the respawn)
	spawnCount := 0
	sendCount := 0
	for _, ev := range actor.Events() {
		switch ev.(type) {
		case unit.SpawnEvent:
			spawnCount++
		case unit.SendEvent:
			sendCount++
		}
	}
	unit.Equal(t, 1, spawnCount, "expected one respawn Spawn event")
	unit.Equal(t, 1, sendCount, "expected one successful Forward (failed Forward emits no SendEvent)")

	info, ok := r.Route("a")
	unit.True(t, ok)
	if info.PID == oldPID {
		t.Errorf("route pid did not change after respawn: %s", info.PID)
	}

	stats := r.HandleInspect(gen.PID{})
	unit.Equal(t, "1", stats["restarts"])
	unit.Equal(t, "1", stats["forwarded"])
	unit.Equal(t, "0", stats["failed"])
}

func TestRouterForwardErrProcessUnknownRespawnFails(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		routeTarget: "a",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()

	spawnErr := errors.New("spawn injected failure")
	actor.Process().SetMethodFailureOnce("Forward", gen.ErrProcessUnknown)
	actor.Process().SetMethodFailureOnce("Spawn", spawnErr)

	sender := gen.PID{Node: "test@localhost", ID: 5002, Creation: 1}
	actor.SendMessage(sender, "ping")

	unit.Equal(t, 1, len(r.routeFailed), "expected one MessageRouteFailed")
	mrf := r.routeFailed[0]
	unit.Equal(t, gen.Atom("a"), mrf.Name)
	unit.True(t, errors.Is(mrf.Reason, spawnErr), "Reason should wrap injected spawn error")

	stats := r.HandleInspect(gen.PID{})
	unit.Equal(t, "1", stats["failed"])
	unit.Equal(t, "0", stats["forwarded"])
}

func TestRouterForwardErrProcessMailboxFullNoRespawn(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		routeTarget: "a",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	actor.ClearEvents()
	actor.Process().SetMethodFailureOnce("Forward", gen.ErrProcessMailboxFull)

	sender := gen.PID{Node: "test@localhost", ID: 5003, Creation: 1}
	actor.SendMessage(sender, "ping")

	// no respawn for non-unknown/terminated errors
	actor.ShouldNotSpawn().Assert()

	unit.Equal(t, 1, len(r.routeFailed))
	unit.True(t, errors.Is(r.routeFailed[0].Reason, gen.ErrProcessMailboxFull))

	info, _ := r.Route("a")
	unit.Equal(t, oldPID, info.PID, "pid unchanged when respawn skipped")

	stats := r.HandleInspect(gen.PID{})
	unit.Equal(t, "1", stats["failed"])
	unit.Equal(t, "0", stats["restarts"])
}

//
// T6.x: eager respawn on MessageExitPID
//

func TestRouterMessageExitPIDTriggersEagerRespawn(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	actor.ClearEvents()

	actor.DeliverExit(oldPID, errors.New("crash"))

	actor.ShouldSpawn().Times(1).Assert()

	info, ok := r.Route("a")
	unit.True(t, ok)
	if info.PID == oldPID || info.PID == (gen.PID{}) {
		t.Errorf("route pid after respawn: got %s, expected new non-empty pid", info.PID)
	}

	stats := r.HandleInspect(gen.PID{})
	unit.Equal(t, "1", stats["restarts"])
}

func TestRouterEagerRespawnFailureLeavesSlotEmpty(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r), unit.WithLogLevel(gen.LogLevelError))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	actor.ClearEvents()

	spawnErr := errors.New("eager spawn injected failure")
	actor.Process().SetMethodFailureOnce("Spawn", spawnErr)
	actor.DeliverExit(oldPID, errors.New("crash"))

	actor.ShouldLog().
		Level(gen.LogLevelError).
		Containing("eager respawn route").
		Once().
		Assert()

	info, ok := r.Route("a")
	unit.True(t, ok)
	unit.Equal(t, gen.PID{}, info.PID, "slot should be empty after eager respawn failure")
	unit.False(t, info.Disabled)
	unit.Equal(t, RoutePendingNone, info.Pending)
}

//
// T7.x: AddRoute
//

func TestRouterAddRouteAppendsAndSpawns(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()

	if err := r.AddRoute(Route{Name: "b", Factory: factoryRouterWorkerB}); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}

	actor.ShouldSpawn().Times(1).Assert()
	infos := r.Routes()
	unit.Equal(t, 2, len(infos))
	unit.Equal(t, gen.Atom("b"), infos[1].Name)
	info, ok := r.Route("b")
	unit.True(t, ok)
	if info.PID == (gen.PID{}) {
		t.Error("new route pid empty after AddRoute")
	}
}

func TestRouterAddRouteDuplicateRejected(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	err = r.AddRoute(Route{Name: "a", Factory: factoryRouterWorkerB})
	unit.True(t, errors.Is(err, ErrRouteDuplicate), "expected ErrRouteDuplicate")
}

func TestRouterAddRouteEmptyNameRejected(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	err = r.AddRoute(Route{Name: "", Factory: factoryRouterWorkerB})
	if err == nil || strings.Contains(err.Error(), "can not be empty") == false {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouterAddRouteNilFactoryRejected(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	err = r.AddRoute(Route{Name: "b", Factory: nil})
	if err == nil || strings.Contains(err.Error(), "nil Factory") == false {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouterAddRouteSpawnFailureNotAdded(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	spawnErr := errors.New("add route spawn failed")
	actor.Process().SetMethodFailureOnce("Spawn", spawnErr)

	err = r.AddRoute(Route{Name: "b", Factory: factoryRouterWorkerB})
	unit.True(t, errors.Is(err, spawnErr))

	_, ok := r.Route("b")
	unit.False(t, ok, "route 'b' must not be present after spawn failure")
	unit.Equal(t, 1, len(r.Routes()))
}

//
// T8.x: RemoveRoute
//

func TestRouterRemoveRouteOnRunningSendsExitAndSetsPending(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	actor.ClearEvents()

	if err := r.RemoveRoute("a"); err != nil {
		t.Fatalf("RemoveRoute: %v", err)
	}

	actor.ShouldSendExit().To(oldPID).WithReason(gen.TerminateReasonShutdown).Once().Assert()
	info, ok := r.Route("a")
	unit.True(t, ok)
	unit.Equal(t, RoutePendingRemove, info.Pending)
}

func TestRouterRemoveRouteAfterExitDropsEntry(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	if err := r.RemoveRoute("a"); err != nil {
		t.Fatal(err)
	}
	actor.DeliverExit(oldPID, gen.TerminateReasonShutdown)

	_, ok := r.Route("a")
	unit.False(t, ok, "route should be dropped after exit")
	unit.Equal(t, 0, len(r.Routes()))
}

func TestRouterRemoveRouteOnDeadIsSync(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r), unit.WithLogLevel(gen.LogLevelError))
	if err != nil {
		t.Fatal(err)
	}

	// Put route in dead state via failed eager respawn.
	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	actor.Process().SetMethodFailureOnce("Spawn", errors.New("respawn fail"))
	actor.DeliverExit(oldPID, errors.New("crash"))

	info, _ := r.Route("a")
	unit.Equal(t, gen.PID{}, info.PID, "precondition: route dead")
	actor.ClearEvents()

	if err := r.RemoveRoute("a"); err != nil {
		t.Fatal(err)
	}

	actor.ShouldNotSendExit().Assert()
	_, ok := r.Route("a")
	unit.False(t, ok)
}

func TestRouterRemoveRouteUnknownIsIdempotent(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveRoute("ghost"); err != nil {
		t.Errorf("RemoveRoute(ghost) = %v, want nil", err)
	}
}

func TestRouterRemoveRouteDuringPendingReturnsBusy(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}
	err = r.RemoveRoute("a")
	unit.True(t, errors.Is(err, gen.ErrBusy))
}

func TestRouterForwardDuringPendingRemoveReturnsRouteUnknown(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		routeTarget: "a",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	if err := r.RemoveRoute("a"); err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()

	sender := gen.PID{Node: "test@localhost", ID: 8001, Creation: 1}
	actor.SendMessage(sender, "to-removing")

	unit.Equal(t, 1, len(r.routeFailed))
	unit.True(t, errors.Is(r.routeFailed[0].Reason, gen.ErrNoRoute))
}

//
// T9.x: DisableRoute / EnableRoute
//

func TestRouterDisableRouteOnRunningSendsExitAndSetsPending(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	actor.ClearEvents()

	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}

	actor.ShouldSendExit().To(oldPID).WithReason(gen.TerminateReasonShutdown).Once().Assert()
	info, _ := r.Route("a")
	unit.Equal(t, RoutePendingDisable, info.Pending)
}

func TestRouterDisableRouteAfterExitMarksDisabled(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()
	actor.DeliverExit(oldPID, gen.TerminateReasonShutdown)

	// No respawn on intentional disable.
	actor.ShouldNotSpawn().Assert()

	info, _ := r.Route("a")
	unit.True(t, info.Disabled)
	unit.Equal(t, gen.PID{}, info.PID)
	unit.Equal(t, RoutePendingNone, info.Pending)
}

func TestRouterDisableRouteOnDeadIsSync(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r), unit.WithLogLevel(gen.LogLevelError))
	if err != nil {
		t.Fatal(err)
	}

	// Put in dead state.
	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	actor.Process().SetMethodFailureOnce("Spawn", errors.New("respawn fail"))
	actor.DeliverExit(oldPID, errors.New("crash"))

	info, _ := r.Route("a")
	unit.Equal(t, gen.PID{}, info.PID)
	actor.ClearEvents()

	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}

	actor.ShouldNotSendExit().Assert()
	info, _ = r.Route("a")
	unit.True(t, info.Disabled)
}

func TestRouterDisableRouteAlreadyDisabledIsIdempotent(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}
	actor.DeliverExit(oldPID, gen.TerminateReasonShutdown)
	actor.ClearEvents()

	if err := r.DisableRoute("a"); err != nil {
		t.Errorf("DisableRoute idempotent: %v", err)
	}
	actor.ShouldNotSendExit().Assert()
}

func TestRouterForwardToDisabledRouteReturnsRouteDisabled(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		routeTarget: "a",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}
	actor.DeliverExit(oldPID, gen.TerminateReasonShutdown)
	actor.ClearEvents()

	sender := gen.PID{Node: "test@localhost", ID: 9201, Creation: 1}
	actor.SendMessage(sender, "msg")

	unit.Equal(t, 1, len(r.routeFailed))
	unit.True(t, errors.Is(r.routeFailed[0].Reason, gen.ErrDisabled))
}

func TestRouterCallToDisabledRouteRespondsRouteDisabled(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		callTarget: "a",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}
	actor.DeliverExit(oldPID, gen.TerminateReasonShutdown)
	actor.ClearEvents()

	caller := gen.PID{Node: "test@localhost", ID: 9202, Creation: 1}
	result := actor.Call(caller, "req")
	unit.True(t, errors.Is(result.Error, gen.ErrDisabled))
}

func TestRouterEnableRouteOnDisabledSpawns(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}
	actor.DeliverExit(oldPID, gen.TerminateReasonShutdown)
	actor.ClearEvents()

	if err := r.EnableRoute("a"); err != nil {
		t.Fatal(err)
	}

	actor.ShouldSpawn().Times(1).Assert()
	info, _ := r.Route("a")
	unit.False(t, info.Disabled)
	if info.PID == (gen.PID{}) {
		t.Error("expected new pid after EnableRoute")
	}
}

func TestRouterEnableRouteOnEnabledIsIdempotent(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()

	if err := r.EnableRoute("a"); err != nil {
		t.Errorf("EnableRoute idempotent: %v", err)
	}
	actor.ShouldNotSpawn().Assert()
}

func TestRouterEnableRouteSpawnFailureLeavesDeadState(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}
	actor.DeliverExit(oldPID, gen.TerminateReasonShutdown)
	actor.ClearEvents()

	spawnErr := errors.New("enable spawn failed")
	actor.Process().SetMethodFailureOnce("Spawn", spawnErr)

	err = r.EnableRoute("a")
	unit.True(t, errors.Is(err, spawnErr))

	info, _ := r.Route("a")
	unit.False(t, info.Disabled, "disabled flag cleared")
	unit.Equal(t, gen.PID{}, info.PID, "pid empty after failed spawn")
}

func TestRouterDisableEnableUnknownReturnsRouteUnknown(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	unit.True(t, errors.Is(r.DisableRoute("ghost"), gen.ErrNoRoute))
	unit.True(t, errors.Is(r.EnableRoute("ghost"), gen.ErrNoRoute))
}

func TestRouterDisableEnableDuringPendingReturnsBusy(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}
	unit.True(t, errors.Is(r.DisableRoute("a"), gen.ErrBusy))
	unit.True(t, errors.Is(r.EnableRoute("a"), gen.ErrBusy))
}

//
// T10.x: ReplaceRoute
//

func TestRouterReplaceRouteOnRunningSendsExitAndSetsPending(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	actor.ClearEvents()

	if err := r.ReplaceRoute("a", Route{Factory: factoryRouterWorkerB}); err != nil {
		t.Fatalf("ReplaceRoute: %v", err)
	}

	actor.ShouldSendExit().To(oldPID).WithReason(gen.TerminateReasonShutdown).Once().Assert()
	info, _ := r.Route("a")
	unit.Equal(t, RoutePendingReplace, info.Pending)
}

func TestRouterReplaceRouteAfterExitSpawnsWithNewFactory(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	if err := r.ReplaceRoute("a", Route{Factory: factoryRouterWorkerB}); err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()
	actor.DeliverExit(oldPID, gen.TerminateReasonShutdown)

	spawnResult := actor.ShouldSpawn().Factory(factoryRouterWorkerB).Once().Capture()
	unit.NotNil(t, spawnResult, "expected spawn with new factory")

	info, _ := r.Route("a")
	if info.PID == (gen.PID{}) || info.PID == oldPID {
		t.Errorf("expected new pid after replace, got %s", info.PID)
	}
	unit.Equal(t, RoutePendingNone, info.Pending)
}

func TestRouterReplaceRouteOnDeadIsSync(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r), unit.WithLogLevel(gen.LogLevelError))
	if err != nil {
		t.Fatal(err)
	}

	// Put in dead state.
	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	actor.Process().SetMethodFailureOnce("Spawn", errors.New("respawn fail"))
	actor.DeliverExit(oldPID, errors.New("crash"))

	actor.ClearEvents()

	if err := r.ReplaceRoute("a", Route{Factory: factoryRouterWorkerB}); err != nil {
		t.Fatal(err)
	}

	actor.ShouldSpawn().Factory(factoryRouterWorkerB).Once().Assert()
	actor.ShouldNotSendExit().Assert()
}

func TestRouterReplaceRouteOnDisabledSwapsSpecNoSpawn(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}
	actor.DeliverExit(oldPID, gen.TerminateReasonShutdown)
	actor.ClearEvents()

	if err := r.ReplaceRoute("a", Route{Factory: factoryRouterWorkerB}); err != nil {
		t.Fatal(err)
	}

	actor.ShouldNotSpawn().Assert()
	info, _ := r.Route("a")
	unit.True(t, info.Disabled, "should stay disabled after replace")
	unit.Equal(t, gen.PID{}, info.PID)

	// EnableRoute now should spawn with the new factory.
	actor.ClearEvents()
	if err := r.EnableRoute("a"); err != nil {
		t.Fatal(err)
	}
	actor.ShouldSpawn().Factory(factoryRouterWorkerB).Once().Assert()
}

func TestRouterReplaceRouteNameMismatchRejected(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	err = r.ReplaceRoute("a", Route{Name: "other", Factory: factoryRouterWorkerB})
	if err == nil || strings.Contains(err.Error(), "name mismatch") == false {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouterReplaceRouteNilFactoryRejected(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	err = r.ReplaceRoute("a", Route{Factory: nil})
	if err == nil || strings.Contains(err.Error(), "nil Factory") == false {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouterReplaceRouteDuringPendingReturnsBusy(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}
	err = r.ReplaceRoute("a", Route{Factory: factoryRouterWorkerB})
	unit.True(t, errors.Is(err, gen.ErrBusy))
}

func TestRouterReplaceRouteUnknownReturnsRouteUnknown(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	err = r.ReplaceRoute("ghost", Route{Factory: factoryRouterWorkerB})
	unit.True(t, errors.Is(err, gen.ErrNoRoute))
}

func TestRouterForwardDuringPendingReplaceUsesCurrentPID(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		routeTarget: "a",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	if err := r.ReplaceRoute("a", Route{Factory: factoryRouterWorkerB}); err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()

	sender := gen.PID{Node: "test@localhost", ID: 10001, Creation: 1}
	actor.SendMessage(sender, "in-flight")

	actor.ShouldSend().To(oldPID).Once().Assert()
}

func TestRouterForwardDuringPendingReplaceErrProcessUnknownNoLazyRespawn(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		routeTarget: "a",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}

	if err := r.ReplaceRoute("a", Route{Factory: factoryRouterWorkerB}); err != nil {
		t.Fatal(err)
	}
	actor.ClearEvents()
	actor.Process().SetMethodFailureOnce("Forward", gen.ErrProcessUnknown)

	sender := gen.PID{Node: "test@localhost", ID: 10002, Creation: 1}
	actor.SendMessage(sender, "in-flight")

	// No respawn during pending=Replace; handleExit will spawn with new spec.
	actor.ShouldNotSpawn().Assert()
	unit.Equal(t, 1, len(r.routeFailed))
	unit.True(t, errors.Is(r.routeFailed[0].Reason, gen.ErrProcessUnknown))
}

//
// T11.x: RespawnRoute
//

func TestRouterRespawnRouteOnDeadSpawns(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r), unit.WithLogLevel(gen.LogLevelError))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	actor.Process().SetMethodFailureOnce("Spawn", errors.New("respawn fail"))
	actor.DeliverExit(oldPID, errors.New("crash"))

	info, _ := r.Route("a")
	unit.Equal(t, gen.PID{}, info.PID)
	actor.ClearEvents()

	if err := r.RespawnRoute("a"); err != nil {
		t.Fatal(err)
	}
	actor.ShouldSpawn().Times(1).Assert()
	info, _ = r.Route("a")
	if info.PID == (gen.PID{}) {
		t.Error("expected new pid after RespawnRoute")
	}
}

func TestRouterRespawnRouteRunningReturnsRouteRunning(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	err = r.RespawnRoute("a")
	unit.True(t, errors.Is(err, ErrRouteRunning))
}

func TestRouterRespawnRouteDisabledReturnsRouteDisabled(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}
	actor.DeliverExit(oldPID, gen.TerminateReasonShutdown)

	err = r.RespawnRoute("a")
	unit.True(t, errors.Is(err, gen.ErrDisabled))
}

func TestRouterRespawnRouteDuringPendingReturnsBusy(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.DisableRoute("a"); err != nil {
		t.Fatal(err)
	}
	err = r.RespawnRoute("a")
	unit.True(t, errors.Is(err, gen.ErrBusy))
}

func TestRouterRespawnRouteUnknownReturnsRouteUnknown(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	_, err := unit.Spawn(t, makeRouterFactory(r))
	if err != nil {
		t.Fatal(err)
	}
	err = r.RespawnRoute("ghost")
	unit.True(t, errors.Is(err, gen.ErrNoRoute))
}

func TestRouterRespawnRouteSpawnFailureReturnsError(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r), unit.WithLogLevel(gen.LogLevelError))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")
	actor.Process().SetMethodFailureOnce("Spawn", errors.New("first spawn fail"))
	actor.DeliverExit(oldPID, errors.New("crash"))

	spawnErr := errors.New("respawn fail")
	actor.Process().SetMethodFailureOnce("Spawn", spawnErr)
	err = r.RespawnRoute("a")
	unit.True(t, errors.Is(err, spawnErr))
}

func TestRouterAfterEagerFailureNextMessageTriggersLazyRespawn(t *testing.T) {
	r := &testRouter{
		initRoutes: []Route{
			{Name: "a", Factory: factoryRouterWorkerA},
		},
		routeTarget: "a",
	}
	actor, err := unit.Spawn(t, makeRouterFactory(r), unit.WithLogLevel(gen.LogLevelError))
	if err != nil {
		t.Fatal(err)
	}

	oldPID := pidFromRouterRoute(t, actor.Behavior(), "a")

	// Eager respawn fails, slot stays empty.
	actor.Process().SetMethodFailureOnce("Spawn", errors.New("eager spawn failed"))
	actor.DeliverExit(oldPID, errors.New("crash"))

	info, _ := r.Route("a")
	unit.Equal(t, gen.PID{}, info.PID, "slot empty after failed eager respawn")

	actor.ClearEvents()

	// Next message triggers lazy respawn (failure rule already consumed).
	sender := gen.PID{Node: "test@localhost", ID: 6001, Creation: 1}
	actor.SendMessage(sender, "wake")

	actor.ShouldSpawn().Times(1).Assert()

	info, _ = r.Route("a")
	if info.PID == (gen.PID{}) {
		t.Errorf("expected new pid after lazy respawn, got empty")
	}

	stats := r.HandleInspect(gen.PID{})
	unit.Equal(t, "1", stats["forwarded"])
}
