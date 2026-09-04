package local

import (
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/stage"
)

// Shared helpers used across the local suite: an addressable/terminable target, a
// monitor-on-command watcher, and small generic utilities.

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

// watcher monitors on command and survives all incoming downs.
type watcher struct{ act.Actor }

func factoryWatcher() gen.ProcessBehavior { return &watcher{} }

func (w *watcher) HandleMessage(from gen.PID, message any) error {
	c, ok := message.(monitorCmd)
	if ok == false {
		return nil
	}
	// ignore the monitor error (the failure is captured in the Monitor record);
	// the watcher must survive a failed monitor so the test can observe it.
	if ev, isEvent := c.Target.(gen.Event); isEvent {
		_, _ = w.MonitorEvent(ev)
		return nil
	}
	_ = w.Monitor(c.Target)
	return nil
}

// contains reports whether s holds v.
func contains[T comparable](s []T, v T) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
