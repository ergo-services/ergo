package local

import (
	"errors"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// testRouter routes by name carried in the message/request: a gen.Atom names the
// target route (or registered process); anything else is discarded. Normal
// priority is routed; High/Max is handled here (admin) and reported to the
// collector, as is any MessageRouteFailed self-delivered on a failed route.
type testRouter struct {
	act.Router
	collector gen.PID
}

func factoryTestRouter() gen.ProcessBehavior { return &testRouter{} }

func (r *testRouter) Init(args ...any) (act.RouterOptions, error) {
	r.collector = args[0].(gen.PID)
	return act.RouterOptions{
		Routes: []act.Route{
			{Name: "a", Factory: factoryPoolWorker},
			{Name: "b", Factory: factoryPoolWorker},
			{Name: "c", Factory: factoryPoolWorker},
		},
	}, nil
}

func (r *testRouter) RouteMessage(from gen.PID, message any) gen.Atom {
	if name, ok := message.(gen.Atom); ok {
		return name
	}
	return act.RouteDiscard
}

func (r *testRouter) RouteCall(from gen.PID, ref gen.Ref, request any) gen.Atom {
	if name, ok := request.(gen.Atom); ok {
		return name
	}
	return act.RouteDiscard
}

func (r *testRouter) HandleMessage(from gen.PID, message any) error {
	return r.Send(r.collector, message)
}

func (r *testRouter) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return r.PID(), nil
}

// newRouter starts a node with a testRouter and returns the node, the router pid,
// the collector pid, and the three route workers in route order (a, b, c).
func newRouter(t *testing.T) (*stage.Node, gen.PID, gen.PID, []gen.PID) {
	t.Helper()
	s := stage.New(t)
	n := s.Node("n")
	collector := n.Spawn(factoryEcho, gen.ProcessOptions{})
	mk := n.Mark()
	router := n.Spawn(factoryTestRouter, gen.ProcessOptions{}, collector)
	routes := poolWorkers(n, router, mk, 3)
	return n, router, collector, routes
}

// TestLocalRouter: system-level coverage of act.Router behaviors that are only
// observable end to end on a live node (the in-package act/router_test.go already
// covers the internal lifecycle/pending state machine in depth).
func TestLocalRouter(t *testing.T) {
	high, max := gen.MessagePriorityHigh, gen.MessagePriorityMax

	// F1: Init spawns every route, in order, as distinct workers
	t.Run("InitSpawnsRoutes", func(t *testing.T) {
		_, _, _, routes := newRouter(t)
		check.Equal(t, 3, len(routes))
		check.True(t, sameSet(routes, []gen.PID{routes[0], routes[1], routes[2]}))
		check.True(t, routes[0] != routes[1] && routes[1] != routes[2] && routes[0] != routes[2])
	})

	// F2: a normal-priority Send is routed and forwarded to the named route's
	// worker, preserving the original sender
	t.Run("SendRoutesToWorker", func(t *testing.T) {
		n, router, _, routes := newRouter(t)
		names := []gen.Atom{"a", "b", "c"}
		for i, name := range names {
			mk := n.Mark()
			n.Send(router, name)
			fwd, ok := n.ShouldForward().By(router).To(routes[i]).Since(mk).Once().Within(time.Second).Capture()
			check.True(t, ok)
			check.Equal(t, n.PID(), fwd.From) // original sender preserved
		}
	})

	// F3: a normal-priority Call is routed; the response comes from the route worker
	t.Run("CallRoutesToWorker", func(t *testing.T) {
		n, router, _, routes := newRouter(t)
		names := []gen.Atom{"a", "b", "c"}
		for i, name := range names {
			mk := n.Mark()
			v, err := n.Call(router, name)
			check.NoError(t, err)
			check.Equal(t, routes[i], v)
			n.ShouldForward().By(router).To(routes[i]).Since(mk).Once().Within(time.Second).Must()
		}
	})

	// F4: RouteDiscard drops a Send (no forward) and answers a Call with ErrDiscarded
	t.Run("Discard", func(t *testing.T) {
		n, router, _, routes := newRouter(t)

		// Send discard: the routed "a" that follows is the FIFO barrier proving the
		// discarded send was already processed without a forward (same priority, no
		// inversion), so exactly one forward is observed.
		mk := n.Mark()
		n.Send(router, gen.Atom(""))
		n.Send(router, gen.Atom("a"))
		n.ShouldForward().By(router).To(routes[0]).Since(mk).Once().Within(time.Second).Must()
		n.ShouldForward().By(router).Since(mk).Once().Assert()

		// Call discard: the caller gets ErrDiscarded
		_, err := n.Call(router, gen.Atom(""))
		check.True(t, errors.Is(err, gen.ErrDiscarded))
	})

	// F5: High/Max priority bypasses routing and is handled by the router itself
	t.Run("PriorityHandledByRouter", func(t *testing.T) {
		n, router, _, _ := newRouter(t)
		for _, pr := range []gen.MessagePriority{high, max} {
			// admin call: handled here (returns the router pid), not routed
			mk := n.Mark()
			v, err := n.Native().CallWithPriority(router, gen.Atom("a"), pr)
			check.NoError(t, err)
			check.Equal(t, router, v)
			n.ShouldForward().By(router).Since(mk).None().Assert()

			// admin message: handled here (relayed to collector), not routed
			mk = n.Mark()
			check.NoError(t, n.Native().SendWithPriority(router, gen.Atom("ping"), pr))
			n.ShouldSend().From(router).Message(gen.Atom("ping")).Since(mk).Once().Within(time.Second).Must()
			n.ShouldForward().By(router).Since(mk).None().Assert()
		}
	})

	// F6: a name that is not a route but is a registered process falls back to the
	// node registry and is forwarded there
	t.Run("FallbackToRegistry", func(t *testing.T) {
		n, router, _, _ := newRouter(t)
		ext := n.SpawnRegister("ext_proc", factoryPoolWorker, gen.ProcessOptions{})

		mk := n.Mark()
		n.Send(router, gen.Atom("ext_proc"))
		n.ShouldForward().By(router).To(ext).Since(mk).Once().Within(time.Second).Must()

		v, err := n.Call(router, gen.Atom("ext_proc"))
		check.NoError(t, err)
		check.Equal(t, ext, v)
	})

	// F7: an unknown name fails routing: a Send yields MessageRouteFailed to the
	// admin handler; a Call answers the caller with ErrProcessUnknown
	t.Run("UnknownNameFails", func(t *testing.T) {
		n, router, _, _ := newRouter(t)

		mk := n.Mark()
		n.Send(router, gen.Atom("ghost"))
		n.ShouldSend().From(router).Where(func(s stage.Sent) bool {
			f, ok := s.Message.(act.MessageRouteFailed)
			return ok && f.Name == "ghost" && errors.Is(f.Reason, gen.ErrProcessUnknown)
		}).Since(mk).Once().Within(time.Second).Must()

		_, err := n.Call(router, gen.Atom("ghost"))
		check.True(t, errors.Is(err, gen.ErrProcessUnknown))
	})

	// F8 (negative): a route worker dies. Routes are linked child->parent, so the
	// router is notified and eagerly respawns a replacement; routing to that name
	// then reaches the new worker, and the router never terminates.
	t.Run("RouteWorkerDeathEagerRespawn", func(t *testing.T) {
		n, router, _, routes := newRouter(t)
		victim := routes[0]

		mk := n.Mark()
		check.NoError(t, n.SendExit(victim, gen.TerminateReasonKill))
		respawned := n.ShouldSpawn().From(router).Since(mk).Times(1).Within(time.Second).Collect()
		check.Equal(t, 1, len(respawned))
		newA := respawned[0].Child
		check.True(t, newA != victim)

		// routing to "a" now reaches the respawned worker, never the dead one
		mk2 := n.Mark()
		n.Send(router, gen.Atom("a"))
		n.ShouldForward().By(router).To(newA).Since(mk2).Once().Within(time.Second).Must()
		n.ShouldForward().By(router).To(victim).Since(mk2).None().Assert()
	})
}
