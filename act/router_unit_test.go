package act_test

import (
	"errors"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

// route worker process
type routeWorker struct{ act.Actor }

func factoryRouteWorker() gen.ProcessBehavior { return &routeWorker{} }

// test router: RouteMessage/RouteCall route to the gen.Atom carried in the message
// (RouteDiscard for a non-atom or the empty atom). MessageRouteFailed is captured.
type rtr struct {
	act.Router
	failed []act.MessageRouteFailed
}

func factoryRtr() gen.ProcessBehavior { return &rtr{} }

func (r *rtr) Init(args ...any) (act.RouterOptions, error) {
	return args[0].(act.RouterOptions), nil
}
func (r *rtr) RouteMessage(from gen.PID, message any) gen.Atom {
	if a, ok := message.(gen.Atom); ok {
		return a
	}
	return act.RouteDiscard
}
func (r *rtr) RouteCall(from gen.PID, ref gen.Ref, request any) gen.Atom {
	if a, ok := request.(gen.Atom); ok {
		return a
	}
	return act.RouteDiscard
}
func (r *rtr) HandleMessage(from gen.PID, message any) error {
	if f, ok := message.(act.MessageRouteFailed); ok {
		r.failed = append(r.failed, f)
	}
	return nil
}

func twoRoutes() act.RouterOptions {
	return act.RouterOptions{Routes: []act.Route{
		{Name: "a", Factory: factoryRouteWorker},
		{Name: "b", Factory: factoryRouteWorker},
	}}
}

// spawnRouter spawns the router under test and returns it plus the route PIDs by name.
func spawnRouter(t *testing.T, opts act.RouterOptions) (*unit.Subject, map[gen.Atom]gen.PID) {
	t.Helper()
	s, err := unit.Spawn(t, factoryRtr, gen.ProcessOptions{}, opts)
	check.NoError(t, err)
	spawns := s.ShouldSpawn().Collect()
	pids := make(map[gen.Atom]gen.PID, len(spawns))
	for i, sp := range spawns {
		pids[opts.Routes[i].Name] = sp.Child
	}
	return s, pids
}

//
// init
//

// Init spawns every route (anonymously, linked both ways).
func TestRouterUnitInitSpawnsRoutes(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	check.Equal(t, 2, len(pids))
	for _, sp := range s.ShouldSpawn().Collect() {
		check.Equal(t, gen.Atom(""), sp.Register) // routes are spawned anonymously
		check.True(t, sp.Options.LinkChild)
		check.True(t, sp.Options.LinkParent)
	}
	check.Equal(t, 2, len(s.Behavior().(*rtr).Routes()))
}

func TestRouterUnitInitEmptyNameFails(t *testing.T) {
	opts := act.RouterOptions{Routes: []act.Route{{Name: "", Factory: factoryRouteWorker}}}
	_, err := unit.Spawn(t, factoryRtr, gen.ProcessOptions{}, opts)
	check.Error(t, err)
}

func TestRouterUnitInitNilFactoryFails(t *testing.T) {
	opts := act.RouterOptions{Routes: []act.Route{{Name: "a", Factory: nil}}}
	_, err := unit.Spawn(t, factoryRtr, gen.ProcessOptions{}, opts)
	check.Error(t, err)
}

func TestRouterUnitInitDuplicateNameFails(t *testing.T) {
	opts := act.RouterOptions{Routes: []act.Route{
		{Name: "a", Factory: factoryRouteWorker},
		{Name: "a", Factory: factoryRouteWorker},
	}}
	_, err := unit.Spawn(t, factoryRtr, gen.ProcessOptions{}, opts)
	check.Error(t, err)
}

//
// routing (normal priority -> RouteMessage / RouteCall)
//

// a normal-priority Send is routed (forwarded) to the named route.
func TestRouterUnitRouteMessageForwards(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	sender := gen.PID{Node: "test@localhost", ID: 7, Creation: 1}

	mark := s.Mark()
	s.SendMessage(sender, gen.Atom("a")) // route to "a"
	s.ShouldForward().To(pids["a"]).Since(mark).Once().Assert()
}

// RouteDiscard drops the message: nothing is forwarded.
func TestRouterUnitRouteMessageDiscard(t *testing.T) {
	s, _ := spawnRouter(t, twoRoutes())
	mark := s.Mark()
	s.SendMessage(gen.PID{}, "not-an-atom") // -> RouteDiscard
	s.ShouldForward().Since(mark).None().Assert()
}

// a normal-priority Call is routed (forwarded) to the named route.
func TestRouterUnitRouteCallForwards(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	mark := s.Mark()
	s.Call(gen.PID{}, gen.Atom("b")) // route to "b"
	s.ShouldForward().To(pids["b"]).Since(mark).Once().Assert()
}

// a discarded Call responds with ErrDiscarded.
func TestRouterUnitRouteCallDiscard(t *testing.T) {
	s, _ := spawnRouter(t, twoRoutes())
	_, err := s.Call(gen.PID{}, "not-an-atom")
	check.ErrorIs(t, err, gen.ErrDiscarded)
}

// routing to an unknown name fails the forward and notifies via MessageRouteFailed.
func TestRouterUnitRouteFailedUnknown(t *testing.T) {
	s, _ := spawnRouter(t, twoRoutes())
	s.SendMessage(gen.PID{}, gen.Atom("nonexistent"))

	b := s.Behavior().(*rtr)
	check.Equal(t, 1, len(b.failed))
	check.ErrorIs(t, b.failed[0].Reason, gen.ErrProcessUnknown)
}

//
// admin path (high priority -> HandleMessage)
//

func TestRouterUnitAdminHandleMessage(t *testing.T) {
	s, _ := spawnRouter(t, twoRoutes())
	mark := s.Mark()
	// high-priority send goes to HandleMessage, not routing -> not forwarded
	s.SendMessageWithPriority(gen.PID{}, gen.Atom("a"), gen.MessagePriorityHigh)
	s.ShouldForward().Since(mark).None().Assert()
}

//
// inspect
//

func TestRouterUnitInspect(t *testing.T) {
	s, _ := spawnRouter(t, twoRoutes())
	m, err := s.Inspect(gen.PID{})
	check.NoError(t, err)
	check.NotNil(t, m)
}

//
// route worker lifecycle
//

// a route worker's exit triggers an eager respawn.
func TestRouterUnitRouteExitRespawns(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	mark := s.Mark()
	s.DeliverExit(pids["a"], errors.New("crash"))
	s.ShouldSpawn().Since(mark).Once().Assert() // route "a" respawned
}

// an exit from a non-route process terminates the router.
func TestRouterUnitForeignExitTerminates(t *testing.T) {
	s, _ := spawnRouter(t, twoRoutes())
	stranger := gen.PID{Node: "test@localhost", ID: 999, Creation: 1}
	s.DeliverExit(stranger, errors.New("boom"))
	check.True(t, s.Terminated())
}

func rctl(s *unit.Subject) *rtr { return s.Behavior().(*rtr) }

//
// route management
//

func TestRouterUnitAddRoute(t *testing.T) {
	s, _ := spawnRouter(t, twoRoutes())
	mark := s.Mark()
	check.NoError(t, rctl(s).AddRoute(act.Route{Name: "c", Factory: factoryRouteWorker}))
	s.ShouldSpawn().Since(mark).Once().Assert()
	_, ok := rctl(s).Route("c")
	check.True(t, ok)

	check.ErrorIs(t, rctl(s).AddRoute(act.Route{Name: "c", Factory: factoryRouteWorker}), act.ErrRouteDuplicate)
	check.Error(t, rctl(s).AddRoute(act.Route{Name: "", Factory: factoryRouteWorker}))
	check.Error(t, rctl(s).AddRoute(act.Route{Name: "d", Factory: nil}))
}

func TestRouterUnitRemoveRoute(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	mark := s.Mark()
	check.NoError(t, rctl(s).RemoveRoute("a")) // sends exit to worker, pending=Remove
	s.ShouldSendExit().To(pids["a"]).Since(mark).Once().Assert()
	s.DeliverExit(pids["a"], gen.TerminateReasonShutdown) // completes removal
	_, ok := rctl(s).Route("a")
	check.False(t, ok)

	check.NoError(t, rctl(s).RemoveRoute("unknown")) // idempotent
}

func TestRouterUnitDisableEnableRoute(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	mark := s.Mark()
	check.NoError(t, rctl(s).DisableRoute("a")) // sends exit, pending=Disable
	s.ShouldSendExit().To(pids["a"]).Since(mark).Once().Assert()
	s.DeliverExit(pids["a"], gen.TerminateReasonShutdown) // -> disabled
	info, _ := rctl(s).Route("a")
	check.True(t, info.Disabled)

	check.NoError(t, rctl(s).DisableRoute("a")) // idempotent
	check.ErrorIs(t, rctl(s).DisableRoute("nope"), gen.ErrNoRoute)

	enableMark := s.Mark()
	check.NoError(t, rctl(s).EnableRoute("a")) // re-enable -> respawn
	s.ShouldSpawn().Since(enableMark).Once().Assert()
	check.NoError(t, rctl(s).EnableRoute("a")) // idempotent
	check.ErrorIs(t, rctl(s).EnableRoute("nope"), gen.ErrNoRoute)
}

func TestRouterUnitReplaceRoute(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	mark := s.Mark()
	check.NoError(t, rctl(s).ReplaceRoute("a", act.Route{Factory: factoryRouteWorker})) // exit + pending=Replace
	s.ShouldSendExit().To(pids["a"]).Since(mark).Once().Assert()
	respawnMark := s.Mark()
	s.DeliverExit(pids["a"], gen.TerminateReasonShutdown) // -> respawn with new spec
	s.ShouldSpawn().Since(respawnMark).Once().Assert()

	check.Error(t, rctl(s).ReplaceRoute("b", act.Route{Factory: nil}))                           // nil factory
	check.Error(t, rctl(s).ReplaceRoute("b", act.Route{Name: "x", Factory: factoryRouteWorker})) // name mismatch
	check.ErrorIs(t, rctl(s).ReplaceRoute("nope", act.Route{Factory: factoryRouteWorker}), gen.ErrNoRoute)
}

// forwarding to a disabled route fails with ErrDisabled (resolveTarget disabled path).
func TestRouterUnitForwardDisabledRoute(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	check.NoError(t, rctl(s).DisableRoute("a"))
	s.DeliverExit(pids["a"], gen.TerminateReasonShutdown) // a now disabled

	s.SendMessage(gen.PID{}, gen.Atom("a"))
	b := rctl(s)
	check.Equal(t, 1, len(b.failed))
	check.ErrorIs(t, b.failed[0].Reason, gen.ErrDisabled)
}

// RespawnRoute error branches.
func TestRouterUnitRespawnRouteErrors(t *testing.T) {
	s, _ := spawnRouter(t, twoRoutes())
	check.ErrorIs(t, rctl(s).RespawnRoute("a"), act.ErrRouteRunning) // worker alive
	check.ErrorIs(t, rctl(s).RespawnRoute("nope"), gen.ErrNoRoute)
}

// makeWorkerless drives route "a" to a worker-less state: its EXIT triggers an eager
// respawn that fails, leaving pid empty. Returns the PID a later spawn will yield.
func makeWorkerless(t *testing.T, s *unit.Subject, aPID gen.PID) gen.PID {
	t.Helper()
	newPID := gen.PID{Node: "unit@localhost", ID: 6000, Creation: 1}
	n := 0
	s.OnSpawn(factoryRouteWorker).ReturnFunc(func() (gen.PID, error) {
		n++
		if n == 1 {
			return gen.PID{}, gen.ErrProcessTerminated // eager respawn fails
		}
		return newPID, nil
	})
	s.DeliverExit(aPID, errors.New("crash"))
	info, _ := rctl(s).Route("a")
	check.Equal(t, gen.PID{}, info.PID) // worker-less
	return newPID
}

// RespawnRoute recovers a worker-less route.
func TestRouterUnitRespawnWorkerlessRoute(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	makeWorkerless(t, s, pids["a"])
	mark := s.Mark()
	check.NoError(t, rctl(s).RespawnRoute("a"))
	s.ShouldSpawn().Since(mark).Once().Assert()
}

// ReplaceRoute on a worker-less route spawns the new spec directly.
func TestRouterUnitReplaceWorkerlessRoute(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	makeWorkerless(t, s, pids["a"])
	mark := s.Mark()
	check.NoError(t, rctl(s).ReplaceRoute("a", act.Route{Factory: factoryRouteWorker}))
	s.ShouldSpawn().Since(mark).Once().Assert()
}

//
// default callbacks (a router that does not override them)
//

type rtrPlain struct{ act.Router }

func factoryRtrPlain() gen.ProcessBehavior { return &rtrPlain{} }

func (r *rtrPlain) Init(args ...any) (act.RouterOptions, error) {
	return args[0].(act.RouterOptions), nil
}
func (r *rtrPlain) RouteMessage(from gen.PID, message any) gen.Atom { return act.RouteDiscard }
func (r *rtrPlain) RouteCall(from gen.PID, ref gen.Ref, request any) gen.Atom {
	return act.RouteDiscard
}

func TestRouterUnitDefaultCallbacks(t *testing.T) {
	s, err := unit.Spawn(t, factoryRtrPlain, gen.ProcessOptions{}, twoRoutes())
	check.NoError(t, err)
	s.ShouldSpawn().Times(2).Assert()

	s.SendMessageWithPriority(gen.PID{}, "admin", gen.MessagePriorityHigh) // default HandleMessage
	s.DeliverEvent(gen.Event{Name: "ev"}, "m")                             // default HandleEvent
	m, err := s.Inspect(gen.PID{})                                         // default HandleInspect
	check.NoError(t, err)
	check.NotNil(t, m)
	s.ShouldTerminate().None().Assert()
}

// router with its own HandleInspect
type rtrInspect struct{ act.Router }

func factoryRtrInspect() gen.ProcessBehavior { return &rtrInspect{} }

func (r *rtrInspect) Init(args ...any) (act.RouterOptions, error) {
	return args[0].(act.RouterOptions), nil
}
func (r *rtrInspect) RouteMessage(from gen.PID, message any) gen.Atom { return act.RouteDiscard }
func (r *rtrInspect) RouteCall(from gen.PID, ref gen.Ref, request any) gen.Atom {
	return act.RouteDiscard
}
func (r *rtrInspect) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{"custom": "value", "type": "MyRouter"}
}

// a custom HandleInspect adds/overrides fields, the routing stats are kept.
func TestRouterUnitInspectCustom(t *testing.T) {
	s, err := unit.Spawn(t, factoryRtrInspect, gen.ProcessOptions{}, twoRoutes())
	check.NoError(t, err)
	s.ShouldSpawn().Times(2).Assert()

	m, err := s.Inspect(gen.PID{})
	check.NoError(t, err)
	check.Equal(t, "value", m["custom"])
	check.Equal(t, "2", m["routes_total"]) // routing stats are not lost
	check.Equal(t, "MyRouter", m["type"])  // the behavior wins on the same key
}

// forwarding to a route whose worker is gone respawns it and retries the forward.
func TestRouterUnitForwardRespawnRetry(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	s.OnForward(pids["a"]).Fail(gen.ErrProcessUnknown) // forward to the current worker fails

	mark := s.Mark()
	s.SendMessage(gen.PID{}, gen.Atom("a")) // -> respawn "a", retry forward to the new worker
	s.ShouldSpawn().Since(mark).Once().Assert()
}

// when SendExit fails synchronously (already terminated), RemoveRoute completes inline.
func TestRouterUnitRemoveRouteSendExitFails(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	s.OnSendExit(pids["a"]).Fail(gen.ErrProcessTerminated)
	check.NoError(t, rctl(s).RemoveRoute("a")) // inline drop, worker pid -> pendingExit
	_, ok := rctl(s).Route("a")
	check.False(t, ok)

	// the late EXIT of that worker is dropped silently (pendingExit path)
	s.DeliverExit(pids["a"], gen.TerminateReasonShutdown)
	check.False(t, s.Terminated())
}

// ReplaceRoute completes inline when SendExit reports the worker already gone.
func TestRouterUnitReplaceRouteSendExitFails(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	s.OnSendExit(pids["a"]).Fail(gen.ErrProcessTerminated)
	mark := s.Mark()
	check.NoError(t, rctl(s).ReplaceRoute("a", act.Route{Factory: factoryRouteWorker}))
	s.ShouldSpawn().Since(mark).Once().Assert() // respawned inline with new spec
}

// a failed forward-respawn leaves the route worker-less; a later route recovers it.
func TestRouterUnitForwardRespawnFailsThenRecovers(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	s.OnForward(pids["a"]).Fail(gen.ErrProcessUnknown)
	newPID := gen.PID{Node: "unit@localhost", ID: 5000, Creation: 1}
	n := 0
	s.OnSpawn(factoryRouteWorker).ReturnFunc(func() (gen.PID, error) {
		n++
		if n == 1 {
			return gen.PID{}, gen.ErrProcessTerminated // the forward-triggered respawn fails
		}
		return newPID, nil
	})

	// forward fails -> respawn fails -> failed + MessageRouteFailed; route "a" now worker-less
	s.SendMessage(gen.PID{}, gen.Atom("a"))
	check.Equal(t, 1, len(rctl(s).failed))

	// next route to "a": resolveTarget respawns the worker and forwards to it
	mark := s.Mark()
	s.SendMessage(gen.PID{}, gen.Atom("a"))
	s.ShouldForward().To(newPID).Since(mark).Once().Assert()
}

func TestRouterUnitDisableRouteSendExitFails(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	s.OnSendExit(pids["a"]).Fail(gen.ErrProcessTerminated)
	check.NoError(t, rctl(s).DisableRoute("a")) // inline disable
	info, _ := rctl(s).Route("a")
	check.True(t, info.Disabled)
}

// routing to a route mid-removal yields ErrNoRoute (resolveTarget pending path).
func TestRouterUnitForwardPendingRemove(t *testing.T) {
	s, pids := spawnRouter(t, twoRoutes())
	check.NoError(t, rctl(s).RemoveRoute("a")) // pending=Remove (worker exit not delivered yet)

	s.SendMessage(gen.PID{}, gen.Atom("a"))
	b := rctl(s)
	check.Equal(t, 1, len(b.failed))
	check.ErrorIs(t, b.failed[0].Reason, gen.ErrNoRoute)
	_ = pids
}

// a busy route (pending op) rejects management calls with ErrBusy.
func TestRouterUnitRouteBusy(t *testing.T) {
	s, _ := spawnRouter(t, twoRoutes())
	check.NoError(t, rctl(s).RemoveRoute("a")) // pending=Remove
	check.ErrorIs(t, rctl(s).DisableRoute("a"), gen.ErrBusy)
	check.ErrorIs(t, rctl(s).RemoveRoute("a"), gen.ErrBusy)
	check.ErrorIs(t, rctl(s).RespawnRoute("a"), gen.ErrBusy)
}

// trivial coverage of ProcessKind and RoutePending.String.
func TestRouterUnitKindAndStrings(t *testing.T) {
	_ = act.RoutePendingNone.String()
	_ = act.RoutePendingDisable.String()
	_ = act.RoutePendingReplace.String()
	_ = act.RoutePendingRemove.String()
	_ = act.RoutePending(99).String()

	s, _ := spawnRouter(t, twoRoutes())
	kind := s.Behavior().(interface{ ProcessKind() gen.ProcessKind }).ProcessKind()
	check.Equal(t, gen.ProcessKindRouter, kind)
}
