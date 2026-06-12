package local

import (
	"errors"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

func triggerExit(t *testing.T, s *stage.Stage, n *stage.Node, target gen.PID, reason error) {
	t.Helper()
	switch reason {
	case gen.TerminateReasonKill:
		s.Kill(n, target)
	case gen.TerminateReasonPanic:
		n.Send(target, "panic")
	default:
		if err := n.SendExit(target, reason); err != nil {
			t.Fatalf("send exit: %s", err)
		}
	}
}

// monitorsOf returns the count and presence of target in the matching Monitors* list.
func monitorsOf(info gen.ProcessInfo, target any) (int, bool) {
	switch t := target.(type) {
	case gen.PID:
		return len(info.MonitorsPID), contains(info.MonitorsPID, t)
	case gen.ProcessID:
		return len(info.MonitorsProcessID), contains(info.MonitorsProcessID, t)
	case gen.Alias:
		return len(info.MonitorsAlias), contains(info.MonitorsAlias, t)
	case gen.Event:
		return len(info.MonitorsEvent), contains(info.MonitorsEvent, t)
	}
	return -1, false
}

func downAbout(a *stage.DownAssert, target any) *stage.DownAssert {
	switch t := target.(type) {
	case gen.PID:
		return a.About(t)
	case gen.ProcessID:
		return a.AboutProcessID(t)
	case gen.Alias:
		return a.AboutAlias(t)
	case gen.Event:
		return a.AboutEvent(t)
	}
	return a
}

// runMonitor: a fresh watcher monitors monTarget (verifying ProcessInfo goes from
// empty to exactly one entry), the target then terminates for the given reason,
// and the watcher receives a Down carrying the target identity and reason.
func runMonitor(t *testing.T, s *stage.Stage, n *stage.Node, reason error, bPID gen.PID, monTarget any) {
	t.Helper()
	w := n.Spawn(factoryWatcher, gen.ProcessOptions{})

	info, err := n.Native().ProcessInfo(w)
	check.NoError(t, err)
	cnt, has := monitorsOf(info, monTarget)
	check.Equal(t, 0, cnt)
	check.True(t, has == false)

	mk := n.Mark()
	n.Send(w, monitorCmd{Target: monTarget})
	n.ShouldMonitor().From(w).Target(monTarget).Since(mk).Once().Within(time.Second).Must()

	info, err = n.Native().ProcessInfo(w)
	check.NoError(t, err)
	cnt, has = monitorsOf(info, monTarget)
	check.Equal(t, 1, cnt)
	check.True(t, has)

	if reason == gen.ErrUnregistered {
		// break the monitor by unregistering the monitored name/alias/event;
		// the target process itself stays alive
		unregisterFor(n, bPID, monTarget)
	} else {
		triggerExit(t, s, n, bPID, reason)
	}
	downAbout(n.ShouldReceiveDown().To(w), monTarget).Reason(reason).
		Since(mk).Once().Within(time.Second).Must()
	if reason == gen.ErrUnregistered {
		// the down was the identity being unregistered, not a termination
		_, err := n.Native().ProcessInfo(bPID)
		check.NoError(t, err)
	}
}

// TestLocalMonitor: monitoring a process by PID, registered name, alias or event
// registers exactly one monitor (visible in ProcessInfo) and, when the target
// terminates for any reason, delivers the matching Down notification carrying the
// target's identity and the termination reason. Monitoring an unknown target fails.
func TestLocalMonitor(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")

	custom := errors.New("custom")
	reasons := []error{gen.TerminateReasonKill, custom, gen.TerminateReasonShutdown, gen.TerminateReasonPanic}
	withUnreg := append(append([]error{}, reasons...), gen.ErrUnregistered)

	t.Run("PID", func(t *testing.T) {
		for _, reason := range reasons {
			b := n.Spawn(factoryTarget, gen.ProcessOptions{})
			runMonitor(t, s, n, reason, b, b)
		}
	})

	t.Run("ProcessID", func(t *testing.T) {
		for i, reason := range withUnreg {
			name := gen.Atom("mon-pid-" + string(rune('a'+i)))
			b := n.SpawnRegister(name, factoryTarget, gen.ProcessOptions{})
			runMonitor(t, s, n, reason, b, gen.ProcessID{Name: name, Node: n.Name()})
		}
	})

	t.Run("Alias", func(t *testing.T) {
		for _, reason := range withUnreg {
			b := n.Spawn(factoryTarget, gen.ProcessOptions{})
			info, err := n.Call(b, "info")
			check.NoError(t, err)
			runMonitor(t, s, n, reason, b, info.(targetInfo).Alias)
		}
	})

	t.Run("Event", func(t *testing.T) {
		for _, reason := range withUnreg {
			b := n.Spawn(factoryTarget, gen.ProcessOptions{})
			info, err := n.Call(b, "info")
			check.NoError(t, err)
			runMonitor(t, s, n, reason, b, info.(targetInfo).Event)
		}
	})

	t.Run("Unknown", func(t *testing.T) {
		w := n.Spawn(factoryWatcher, gen.ProcessOptions{})

		ghost := gen.PID{Node: n.Name(), ID: 999999}
		mk := n.Mark()
		n.Send(w, monitorCmd{Target: ghost})
		n.ShouldMonitor().From(w).Target(ghost).Error(gen.ErrProcessUnknown).Since(mk).Once().Within(time.Second).Must()

		unknownName := gen.ProcessID{Name: "no_such", Node: n.Name()}
		mk = n.Mark()
		n.Send(w, monitorCmd{Target: unknownName})
		n.ShouldMonitor().From(w).Target(unknownName).Error(gen.ErrProcessUnknown).Since(mk).Once().Within(time.Second).Must()

		unknownAlias := gen.Alias{Node: n.Name()}
		mk = n.Mark()
		n.Send(w, monitorCmd{Target: unknownAlias})
		n.ShouldMonitor().From(w).Target(unknownAlias).Error(gen.ErrAliasUnknown).Since(mk).Once().Within(time.Second).Must()

		unknownEvent := gen.Event{Name: "no_such", Node: n.Name()}
		mk = n.Mark()
		n.Send(w, monitorCmd{Target: unknownEvent})
		n.ShouldMonitor().From(w).Target(unknownEvent).Error(gen.ErrEventUnknown).Since(mk).Once().Within(time.Second).Must()
	})
}
