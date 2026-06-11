package local

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// targetInfo exposes a target's alias and event for addressing.
type targetInfo struct {
	Alias gen.Alias
	Event gen.Event
}

// target is monitorable/linkable by PID/ProcessID/Alias/Event and panics on demand.
type target struct {
	act.Actor
	alias gen.Alias
	event gen.Event
}

func factoryTarget() gen.ProcessBehavior { return &target{} }

func (tg *target) Init(args ...any) error {
	alias, err := tg.CreateAlias()
	if err != nil {
		return err
	}
	tg.alias = alias
	name := gen.Atom(fmt.Sprintf("ev-%d", tg.PID().ID))
	if _, err := tg.RegisterEvent(name, gen.EventOptions{}); err != nil {
		return err
	}
	tg.event = gen.Event{Name: name, Node: tg.Node().Name()}
	return nil
}

func (tg *target) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return targetInfo{Alias: tg.alias, Event: tg.event}, nil
}

// unregisterCmd tells the target to unregister its name / alias / event (which
// breaks monitors and links by that identity with reason ErrUnregistered, while
// the process itself stays alive).
type unregisterCmd struct{ Kind string }

func (tg *target) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "panic" {
			panic("boom")
		}
	case unregisterCmd:
		switch m.Kind {
		case "name":
			return tg.UnregisterName()
		case "alias":
			return tg.DeleteAlias(tg.alias)
		case "event":
			return tg.UnregisterEvent(tg.event.Name)
		}
	}
	return nil
}

// unregisterFor sends the target the unregister command matching how it is
// addressed (name/alias/event); used to break a monitor/link with ErrUnregistered.
func unregisterFor(n *stage.Node, bPID gen.PID, addr any) {
	switch addr.(type) {
	case gen.ProcessID:
		n.Send(bPID, unregisterCmd{Kind: "name"})
	case gen.Alias:
		n.Send(bPID, unregisterCmd{Kind: "alias"})
	case gen.Event:
		n.Send(bPID, unregisterCmd{Kind: "event"})
	}
}

// monitorCmd tells the watcher to monitor Target (PID/ProcessID/Alias/Event).
type monitorCmd struct{ Target any }

// monWatcher monitors on command and survives all incoming downs.
type monWatcher struct{ act.Actor }

func factoryMonWatcher() gen.ProcessBehavior { return &monWatcher{} }

func (w *monWatcher) HandleMessage(from gen.PID, message any) error {
	if c, ok := message.(monitorCmd); ok {
		switch tgt := c.Target.(type) {
		case gen.PID:
			_ = w.MonitorPID(tgt)
		case gen.ProcessID:
			_ = w.MonitorProcessID(tgt)
		case gen.Alias:
			_ = w.MonitorAlias(tgt)
		case gen.Event:
			_, _ = w.MonitorEvent(tgt)
		}
	}
	return nil
}

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

func assertDownAbout(a *stage.DownAssert, target any) *stage.DownAssert {
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
	w := n.Spawn(factoryMonWatcher)

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
	assertDownAbout(n.ShouldReceiveDown().To(w), monTarget).Reason(reason).
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
			b := n.Spawn(factoryTarget)
			runMonitor(t, s, n, reason, b, b)
		}
	})

	t.Run("ProcessID", func(t *testing.T) {
		for i, reason := range withUnreg {
			name := gen.Atom("mon-pid-" + string(rune('a'+i)))
			b := n.SpawnRegister(name, factoryTarget)
			runMonitor(t, s, n, reason, b, gen.ProcessID{Name: name, Node: n.Name()})
		}
	})

	t.Run("Alias", func(t *testing.T) {
		for _, reason := range withUnreg {
			b := n.Spawn(factoryTarget)
			info, err := n.Call(b, "info")
			check.NoError(t, err)
			runMonitor(t, s, n, reason, b, info.(targetInfo).Alias)
		}
	})

	t.Run("Event", func(t *testing.T) {
		for _, reason := range withUnreg {
			b := n.Spawn(factoryTarget)
			info, err := n.Call(b, "info")
			check.NoError(t, err)
			runMonitor(t, s, n, reason, b, info.(targetInfo).Event)
		}
	})

	t.Run("Unknown", func(t *testing.T) {
		w := n.Spawn(factoryMonWatcher)

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
