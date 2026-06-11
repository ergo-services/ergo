package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// spawnTrapper asks an exiter to spawn a trapper child (so the exiter is its parent).
type spawnTrapper struct{ Trap bool }

// doExit asks an exiter to send an exit signal to Target.
type doExit struct {
	Target gen.PID
	Reason error
}

// exiter spawns trapper children and sends exit signals on command.
type exiter struct{ act.Actor }

func factoryExiter() gen.ProcessBehavior { return &exiter{} }

func (e *exiter) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch c := request.(type) {
	case spawnTrapper:
		return e.Spawn(factoryTrapper, gen.ProcessOptions{}, c.Trap)
	case doExit:
		if err := e.SendExit(c.Target, c.Reason); err != nil {
			return nil, err
		}
		return "ok", nil
	}
	return nil, nil
}

// setTrap toggles the trapper's trap-exit flag and returns the new value.
type setTrap struct{ V bool }

// trapper toggles/reports its trap flag and survives trapped exit signals.
type trapper struct{ act.Actor }

func factoryTrapper() gen.ProcessBehavior { return &trapper{} }

func (tr *trapper) Init(args ...any) error {
	tr.SetTrapExit(args[0].(bool))
	return nil
}

func (tr *trapper) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch c := request.(type) {
	case setTrap:
		tr.SetTrapExit(c.V)
		return tr.TrapExit(), nil
	case string:
		if c == "gettrap" {
			return tr.TrapExit(), nil
		}
	}
	return nil, nil
}

func (tr *trapper) HandleMessage(from gen.PID, message any) error { return nil }

// TestLocalTrapExit: TrapExit is a togglable flag; a trapping process receives a
// non-parent exit signal as a message (and survives), while an exit signal from
// its parent terminates it regardless of the flag.
func TestLocalTrapExit(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	host := n.Spawn(factoryExiter)
	alien := n.Spawn(factoryExiter)
	w := n.Spawn(factoryMonWatcher)

	t.Run("Toggle", func(t *testing.T) {
		tpAny, err := n.Call(host, spawnTrapper{Trap: false})
		check.NoError(t, err)
		tp := tpAny.(gen.PID)

		v, err := n.Call(tp, "gettrap")
		check.NoError(t, err)
		check.Equal(t, false, v)

		v, err = n.Call(tp, setTrap{V: true})
		check.NoError(t, err)
		check.Equal(t, true, v)

		v, err = n.Call(tp, setTrap{V: false})
		check.NoError(t, err)
		check.Equal(t, false, v)
	})

	t.Run("NonParentTrapped", func(t *testing.T) {
		tpAny, err := n.Call(host, spawnTrapper{Trap: true})
		check.NoError(t, err)
		tp := tpAny.(gen.PID)

		mk := n.Mark()
		_, err = n.Call(alien, doExit{Target: tp, Reason: gen.TerminateReasonShutdown})
		check.NoError(t, err)

		// the trap turns the non-parent exit into a message carrying the sender
		n.ShouldReceiveExit().To(tp).About(alien).Reason(gen.TerminateReasonShutdown).
			Since(mk).Once().Within(time.Second).Must()

		// and the process survives it
		_, err = n.Native().ProcessInfo(tp)
		check.NoError(t, err)
	})

	t.Run("ParentTerminates", func(t *testing.T) {
		tpAny, err := n.Call(host, spawnTrapper{Trap: true})
		check.NoError(t, err)
		tp := tpAny.(gen.PID)

		n.Send(w, monitorCmd{Target: tp})
		n.ShouldMonitor().From(w).Target(tp).Once().Within(time.Second).Must()

		mk := n.Mark()
		_, err = n.Call(host, doExit{Target: tp, Reason: gen.TerminateReasonShutdown})
		check.NoError(t, err)

		// an exit from the parent terminates the process despite trap exit
		n.ShouldReceiveDown().To(w).About(tp).ReasonIs(gen.TerminateReasonShutdown).
			Since(mk).Once().Within(time.Second).Must()
	})
}
