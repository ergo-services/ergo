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

var errPMAbnormal = errors.New("pm: abnormal return")

// pmActor terminates abnormally on command: panic, or an abnormal callback return.
// (Kill is driven externally.) Spawn with PreserveMailbox so each path captures
// the mailbox into a *gen.Error.
type pmActor struct{ act.Actor }

func factoryPMActor() gen.ProcessBehavior { return &pmActor{} }

func (a *pmActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "panic":
		panic("boom")
	case "fail":
		return errPMAbnormal
	}
	return nil
}

// TestLocalPreserveMailbox: a process spawned with PreserveMailbox that terminates
// abnormally has its reason captured into a *gen.Error (carrying the mailbox). That
// wrapped reason flows to both linkers (exit) and monitors (down), so ReasonIs
// matches the underlying reason while an exact Reason does not. Covers all three
// triggers: panic, abnormal callback return, and kill (the kill of an idle process
// goes through node.Kill's direct path, which must wrap just like the run loop).
func TestLocalPreserveMailbox(t *testing.T) {
	cases := []struct {
		name    string
		trigger func(s *stage.Stage, n *stage.Node, tgt gen.PID)
		reason  error
	}{
		{"Panic", func(s *stage.Stage, n *stage.Node, tgt gen.PID) { n.Send(tgt, "panic") }, gen.TerminateReasonPanic},
		{"AbnormalReturn", func(s *stage.Stage, n *stage.Node, tgt gen.PID) { n.Send(tgt, "fail") }, errPMAbnormal},
		{"Kill", func(s *stage.Stage, n *stage.Node, tgt gen.PID) { s.Kill(n, tgt) }, gen.TerminateReasonKill},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := stage.New(t)
			n := s.Node("n")

			tgt := n.Spawn(factoryPMActor, gen.ProcessOptions{PreserveMailbox: true})
			linker := n.Spawn(factoryLinkerC, gen.ProcessOptions{}, true) // trap: observes the exit as a message
			w := n.Spawn(factoryWatcher, gen.ProcessOptions{})

			n.Send(linker, linkCmd{Target: tgt})
			n.ShouldLink().From(linker).Target(tgt).Once().Within(time.Second).Must()
			n.Send(w, monitorCmd{Target: tgt})
			n.ShouldMonitor().From(w).Target(tgt).Once().Within(time.Second).Must()

			mk := n.Mark()
			tc.trigger(s, n, tgt)

			// the wrapped reason matches the underlying reason via errors.Is, on both
			// the link (exit) and the monitor (down) path
			n.ShouldReceiveExit().To(linker).About(tgt).ReasonIs(tc.reason).
				Since(mk).Once().Within(time.Second).Must()
			n.ShouldReceiveDown().To(w).About(tgt).ReasonIs(tc.reason).
				Since(mk).Once().Within(time.Second).Must()
			// but it is not the bare reason: an exact match finds nothing (proves the wrap)
			n.ShouldReceiveExit().To(linker).About(tgt).Reason(tc.reason).Since(mk).None().Assert()

			// the reason is a *gen.Error carrying the captured mailbox
			rec, ok := n.ShouldReceiveExit().To(linker).About(tgt).Since(mk).Within(time.Second).Capture()
			check.True(t, ok)
			m := rec.Message.(gen.MessageExitPID)
			ge, ok := m.Reason.(*gen.Error)
			check.True(t, ok)
			check.True(t, ge.Mailbox != nil)
		})
	}
}
