package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// responder echoes each request and then terminates normally (HandleCall returns
// the result together with gen.TerminateReasonNormal: reply, then stop).
type responder struct{ act.Actor }

func factoryResponder() gen.ProcessBehavior { return &responder{} }

func (r *responder) Init(args ...any) error {
	_, err := r.CreateAlias()
	return err
}

func (r *responder) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return request, gen.TerminateReasonNormal
}

// fwd carries a call to be answered by another process.
type fwd struct {
	From    gen.PID
	Ref     gen.Ref
	Request any
}

// forwarder defers a call, forwards it, and terminates normally.
type forwarder struct {
	act.Actor
	to gen.PID
}

func factoryForwarder() gen.ProcessBehavior { return &forwarder{} }

func (f *forwarder) Init(args ...any) error {
	f.to = args[0].(gen.PID)
	return nil
}

func (f *forwarder) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if err := f.Send(f.to, fwd{From: from, Ref: ref, Request: request}); err != nil {
		return nil, err
	}
	return nil, gen.TerminateReasonNormal // forward, then stop
}

// fwdTarget answers a forwarded call directly to the original caller.
type fwdTarget struct{ act.Actor }

func factoryFwdTarget() gen.ProcessBehavior { return &fwdTarget{} }

func (ft *fwdTarget) HandleMessage(from gen.PID, message any) error {
	if m, ok := message.(fwd); ok {
		return ft.SendResponse(m.From, m.Ref, m.Request)
	}
	return nil
}

// callAndCheck makes the request, verifies the echoed response and the reply
// egress, and that the responder terminated normally after replying.
func callAndCheck(t *testing.T, n *stage.Node, w, svc gen.PID, to any) {
	t.Helper()
	n.Send(w, monitorCmd{Target: svc})
	n.ShouldMonitor().From(w).Target(svc).Once().Within(time.Second).Must()

	mk := n.Mark()
	out, err := n.Call(to, "ping")
	check.NoError(t, err)
	check.Equal(t, "ping", out)
	n.ShouldReply().From(svc).Message("ping").Since(mk).Once().Within(time.Second).Must()
	n.ShouldReceiveDown().To(w).About(svc).Reason(gen.TerminateReasonNormal).
		Since(mk).Once().Within(time.Second).Must()
}

// TestLocalCall: a synchronous request returns the handler's response (observed
// as the responder's reply); the handler replies and then terminates normally.
// The responder is reached by PID, registered name and alias. A deferred call is
// forwarded to and answered by another process, and the forwarder then stops.
// Calling an unknown target fails.
func TestLocalCall(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	w := n.Spawn(factoryMonWatcher)

	t.Run("PID", func(t *testing.T) {
		svc := n.Spawn(factoryResponder)
		callAndCheck(t, n, w, svc, svc)
	})

	t.Run("ProcessID", func(t *testing.T) {
		svc := n.SpawnRegister("svc", factoryResponder)
		callAndCheck(t, n, w, svc, gen.Atom("svc"))
	})

	t.Run("Alias", func(t *testing.T) {
		svc := n.Spawn(factoryResponder)
		info, err := n.Native().ProcessInfo(svc)
		check.NoError(t, err)
		check.True(t, len(info.Aliases) == 1)
		callAndCheck(t, n, w, svc, info.Aliases[0])
	})

	t.Run("Forward", func(t *testing.T) {
		target := n.Spawn(factoryFwdTarget)
		fwder := n.Spawn(factoryForwarder, target)
		n.Send(w, monitorCmd{Target: fwder})
		n.ShouldMonitor().From(w).Target(fwder).Once().Within(time.Second).Must()

		mk := n.Mark()
		out, err := n.Call(fwder, "fwd-me")
		check.NoError(t, err)
		check.Equal(t, "fwd-me", out)
		// the target (not the forwarder) answers the original caller
		n.ShouldReply().From(target).Message("fwd-me").Since(mk).Once().Within(time.Second).Must()
		// the forwarder stops (terminated normally) after forwarding
		n.ShouldReceiveDown().To(w).About(fwder).Reason(gen.TerminateReasonNormal).
			Since(mk).Once().Within(time.Second).Must()
	})

	t.Run("Unknown", func(t *testing.T) {
		_, err := n.Call(gen.Atom("no_such_process"), 1)
		check.ErrorIs(t, err, gen.ErrProcessUnknown)
	})
}
