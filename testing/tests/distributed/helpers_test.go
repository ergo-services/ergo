package distributed

import (
	"fmt"
	"sync/atomic"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// errText returns "" for a nil error, otherwise err.Error(). Returning a non-nil
// value avoids the actor treating a (nil,nil) HandleCall result as a deferred reply.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// monitorCmd asks a watcher to monitor the target.
type monitorCmd struct{ Target any }

// unregisterCmd tells a target to unregister its name / alias / event (breaking
// monitors and links by that identity with ErrUnregistered, the process surviving).
type unregisterCmd struct{ Kind string }

// watcher monitors a target on command and survives any incoming Down.
type watcher struct{ act.Actor }

func factoryWatcher() gen.ProcessBehavior { return &watcher{} }

func (w *watcher) HandleMessage(from gen.PID, message any) error {
	c, ok := message.(monitorCmd)
	if ok == false {
		return nil
	}
	// ignore the monitor error (the failure is captured in the Monitored record);
	// the watcher must survive a failed monitor so the test can observe it.
	if ev, isEvent := c.Target.(gen.Event); isEvent {
		_, _ = w.MonitorEvent(ev)
		return nil
	}
	_ = w.Monitor(c.Target)
	return nil
}

// unregisteredValue is a type deliberately NOT registered in EDF, so it cannot be
// serialized for the wire (used to test cross-node encode failure).
type unregisteredValue struct{ X int }

// remote link/monitor toolkit (shared by link, monitor and optimization)

var rtargetSeq atomic.Uint64

// rtarget is the remote link/monitor target: addressable by PID, registered name,
// alias and event; it can unregister its identities and panic on command.
type rtarget struct {
	act.Actor
	alias gen.Alias
	event gen.Event
}

func factoryRTarget() gen.ProcessBehavior { return &rtarget{} }

func (t *rtarget) Init(args ...any) error {
	a, err := t.CreateAlias()
	if err != nil {
		return err
	}
	t.alias = a
	name := gen.Atom(fmt.Sprintf("rev-%d", t.PID().ID))
	if _, err := t.RegisterEvent(name, gen.EventOptions{}); err != nil {
		return err
	}
	t.event = gen.Event{Name: name, Node: t.Node().Name()}
	return nil
}

type rinfo struct {
	Alias gen.Alias
	Event gen.Event
}

func (t *rtarget) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return rinfo{Alias: t.alias, Event: t.event}, nil
}

func (t *rtarget) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case string:
		if m == "panic" {
			panic("boom")
		}
	case unregisterCmd:
		switch m.Kind {
		case "name":
			return t.UnregisterName()
		case "alias":
			return t.DeleteAlias(t.alias)
		case "event":
			return t.UnregisterEvent(t.event.Name)
		}
	}
	return nil
}

// rlinker links a (possibly remote) target on command; trap is set so it can
// receive the exit as a message instead of cascading.
type rlinker struct{ act.Actor }

func factoryRLinker() gen.ProcessBehavior { return &rlinker{} }

func (l *rlinker) Init(args ...any) error {
	l.SetTrapExit(args[0].(bool))
	return nil
}

type linkTarget struct{ Target any }
type unlinkTarget struct{ Target any }
type linkNodeCmd struct{ Node gen.Atom }

func (l *rlinker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch c := request.(type) {
	case linkTarget:
		if ev, ok := c.Target.(gen.Event); ok {
			_, err := l.LinkEvent(ev)
			return errText(err), nil
		}
		return errText(l.Link(c.Target)), nil
	case unlinkTarget:
		if ev, ok := c.Target.(gen.Event); ok {
			return errText(l.UnlinkEvent(ev)), nil
		}
		return errText(l.Unlink(c.Target)), nil
	case monitorTarget:
		if ev, ok := c.Target.(gen.Event); ok {
			_, err := l.MonitorEvent(ev)
			return errText(err), nil
		}
		return errText(l.Monitor(c.Target)), nil
	case linkNodeCmd:
		return errText(l.LinkNode(c.Node)), nil
	}
	return "ok", nil
}

func (l *rlinker) HandleMessage(from gen.PID, message any) error { return nil }

// rmonitor monitors a (possibly remote) target on command. It does not trap and
// never cascades; incoming Down notifications are observed via the node recorder.
type rmonitor struct{ act.Actor }

func factoryRMonitor() gen.ProcessBehavior { return &rmonitor{} }

type monitorTarget struct{ Target any }
type demonitorTarget struct{ Target any }
type monitorNodeCmd struct{ Node gen.Atom }

func (m *rmonitor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch c := request.(type) {
	case monitorTarget:
		if ev, ok := c.Target.(gen.Event); ok {
			_, err := m.MonitorEvent(ev)
			return errText(err), nil
		}
		return errText(m.Monitor(c.Target)), nil
	case demonitorTarget:
		if ev, ok := c.Target.(gen.Event); ok {
			return errText(m.DemonitorEvent(ev)), nil
		}
		return errText(m.Demonitor(c.Target)), nil
	case monitorNodeCmd:
		return errText(m.MonitorNode(c.Node)), nil
	}
	return "ok", nil
}

func (m *rmonitor) HandleMessage(from gen.PID, message any) error { return nil }

// exitAbout / downAbout dispatch the addressing-mode filter for an exit or
// down assertion (PID/ProcessID/Alias/Event).
func exitAbout(a *check.ExitAssert, target any) *check.ExitAssert {
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

func downAbout(a *check.DownAssert, target any) *check.DownAssert {
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

// newRTarget spawns a fresh remote target on n2 and returns it addressed by addr
// plus its pid. ProcessID targets get a unique registered name.
func newRTarget(t *testing.T, n2 *stage.Node, addr string) (target any, pid gen.PID) {
	t.Helper()
	if addr == "processid" {
		name := gen.Atom(fmt.Sprintf("rt-%d", rtargetSeq.Add(1)))
		pid = n2.SpawnRegister(name, factoryRTarget, gen.ProcessOptions{})
		return n2.ProcessID(name), pid
	}
	pid = n2.Spawn(factoryRTarget, gen.ProcessOptions{})
	i, err := n2.Call(pid, nil)
	check.NoError(t, err)
	info := i.(rinfo)
	switch addr {
	case "alias":
		return info.Alias, pid
	case "event":
		return info.Event, pid
	}
	return pid, pid
}
