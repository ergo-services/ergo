package tm

import (
	"ergo.services/ergo/gen"
)

func (m *Manager) TerminatedTargetPID(pid gen.PID, reason error) {
	relations := m.storage.RemoveTarget(pid)
	m.dispatchTargetTerminate(pid, relations, reason)
	m.resetWire(pid)
}

func (m *Manager) TerminatedTargetProcessID(processID gen.ProcessID, reason error) {
	relations := m.storage.RemoveTarget(processID)
	m.dispatchTargetTerminate(processID, relations, reason)
	m.resetWire(processID)
}

func (m *Manager) TerminatedTargetAlias(alias gen.Alias, reason error) {
	relations := m.storage.RemoveTarget(alias)
	m.dispatchTargetTerminate(alias, relations, reason)
	m.resetWire(alias)
}

func (m *Manager) TerminatedTargetEvent(event gen.Event, reason error) {
	if v, ok := m.events.LoadAndDelete(event); ok {
		m.eventsByID.Delete(v.(*eventEntry).id)
		m.eventsCount.Add(-1)
	}
	relations := m.storage.RemoveTarget(event)
	m.dispatchTargetTerminate(event, relations, reason)
	m.resetWire(event)
}

// Two passes: kill targets hosted on the dead node (always ErrNoConnection
// regardless of `reason`), then drop consumers that lived on the dead node
// from surviving targets.
func (m *Manager) TerminatedTargetNode(node gen.Atom, reason error) {
	var toKill []any
	m.storage.RangeTargets(func(t any) bool {
		if targetLivesOnNode(t, node) {
			toKill = append(toKill, t)
		}
		return true
	})

	for _, t := range toKill {
		if ev, ok := t.(gen.Event); ok {
			if v, ok := m.events.LoadAndDelete(ev); ok {
				m.eventsByID.Delete(v.(*eventEntry).id)
				m.eventsCount.Add(-1)
			}
		}
		relations := m.storage.RemoveTarget(t)
		m.dispatchTargetTerminate(t, relations, gen.ErrNoConnection)
		m.resetWire(t)
	}

	m.storage.RangeTargets(func(t any) bool {
		removed := m.storage.RemoveConsumersOnNode(t, node)
		if removed == 0 {
			return true
		}
		m.eventSubscribersDropped(t, removed)
		return true
	})
}

// Caller must serialize TerminatedProcess against this process's own
// Link/Monitor/RegisterEvent operations; concurrent ones may leave a
// phantom relation/event that the reverse-index sweep doesn't reach.
// The runtime guarantees this via process lifecycle.
func (m *Manager) TerminatedProcess(pid gen.PID, reason error) {
	for _, t := range m.storage.LinksFor(pid) {
		if m.storage.Unregister(t, pid, KindLink) {
			m.eventSubscribersDropped(t, 1)
			m.afterRemoteConsumerGone(t, KindLink)
		}
	}
	for _, t := range m.storage.MonitorsFor(pid) {
		if m.storage.Unregister(t, pid, KindMonitor) {
			m.eventSubscribersDropped(t, 1)
			m.afterRemoteConsumerGone(t, KindMonitor)
		}
	}
	m.storage.ClearConsumer(pid)

	m.events.Range(func(_, v any) bool {
		e := v.(*eventEntry)
		if e.producer != pid {
			return true
		}
		if m.events.CompareAndDelete(e.event, e) == false {
			return true
		}
		m.eventsByID.Delete(e.id)
		m.eventsCount.Add(-1)
		relations := m.storage.RemoveTarget(e.event)
		m.dispatchTargetTerminate(e.event, relations, reason)
		return true
	})
}

// Called by TerminatedProcess: consumer is always local, so always decrement.
func (m *Manager) afterRemoteConsumerGone(target any, kind Kind) {
	switch t := target.(type) {
	case gen.PID:
		if t.Node == m.core.Name() {
			return
		}
		wp := m.wireForExisting(t, kind)
		if wp != nil {
			wp.localCount.Add(-1)
		}
		m.ensureRemoteUnlink(wp, t.Node, func(conn gen.Connection) {
			if kind == KindLink {
				conn.UnlinkPID(m.core.PID(), t)
				return
			}
			conn.DemonitorPID(m.core.PID(), t)
		})
	case gen.ProcessID:
		node := processIDNode(t, m.core.Name())
		if node == m.core.Name() {
			return
		}
		wp := m.wireForExisting(t, kind)
		if wp != nil {
			wp.localCount.Add(-1)
		}
		m.ensureRemoteUnlink(wp, node, func(conn gen.Connection) {
			if kind == KindLink {
				conn.UnlinkProcessID(m.core.PID(), t)
				return
			}
			conn.DemonitorProcessID(m.core.PID(), t)
		})
	case gen.Alias:
		if t.Node == m.core.Name() {
			return
		}
		wp := m.wireForExisting(t, kind)
		if wp != nil {
			wp.localCount.Add(-1)
		}
		m.ensureRemoteUnlink(wp, t.Node, func(conn gen.Connection) {
			if kind == KindLink {
				conn.UnlinkAlias(m.core.PID(), t)
				return
			}
			conn.DemonitorAlias(m.core.PID(), t)
		})
	case gen.Event:
		if t.Node == m.core.Name() {
			return
		}
		wp := m.wireForExisting(t, kind)
		if wp != nil {
			wp.localCount.Add(-1)
		}
		m.ensureRemoteUnlink(wp, t.Node, func(conn gen.Connection) {
			if kind == KindLink {
				conn.UnlinkEvent(m.core.PID(), t)
				return
			}
			conn.DemonitorEvent(m.core.PID(), t)
		})
	}
}

// Decrements subscriberCount and fires MessageEventStop at zero, for bulk
// removals outside the LinkEvent/UnlinkEvent path.
func (m *Manager) eventSubscribersDropped(target any, removed int) {
	ev, ok := target.(gen.Event)
	if ok == false {
		return
	}
	v, ok := m.events.Load(ev)
	if ok == false {
		return
	}
	entry := v.(*eventEntry)
	count := entry.subscriberCount.Add(-int64(removed))
	if count != 0 || entry.notify == false {
		return
	}
	m.core.RouteSendPID(
		m.core.PID(),
		entry.producer,
		gen.MessageOptions{Priority: gen.MessagePriorityHigh},
		gen.MessageEventStop{Name: ev.Name},
	)
}

func (m *Manager) dispatchTargetTerminate(target any, relations []Relation, reason error) {
	var localExits []gen.PID
	var localDowns []gen.PID
	remoteNodes := make(map[gen.Atom]struct{})

	for _, r := range relations {
		if r.Consumer.Node != m.core.Name() {
			remoteNodes[r.Consumer.Node] = struct{}{}
			continue
		}
		if r.Kind == KindLink {
			localExits = append(localExits, r.Consumer)
			continue
		}
		localDowns = append(localDowns, r.Consumer)
	}

	if len(localExits) > 0 {
		m.core.RouteSendExitMessages(m.core.PID(), localExits, exitMessageFor(target, reason))
		m.exitsProduced.Add(1)
		m.exitsDelivered.Add(int64(len(localExits)))
	}
	if len(localDowns) > 0 {
		downMsg := downMessageFor(target, reason)
		for _, c := range localDowns {
			m.core.RouteSendPID(
				m.core.PID(),
				c,
				gen.MessageOptions{Priority: gen.MessagePriorityHigh},
				downMsg,
			)
		}
		m.downsProduced.Add(1)
		m.downsDelivered.Add(int64(len(localDowns)))
	}
	for node := range remoteNodes {
		conn, err := m.core.GetConnection(node)
		if err != nil {
			continue
		}
		sendRemoteTerminate(conn, target, reason)
	}
}

func targetLivesOnNode(target any, node gen.Atom) bool {
	switch t := target.(type) {
	case gen.PID:
		return t.Node == node
	case gen.ProcessID:
		return t.Node == node
	case gen.Alias:
		return t.Node == node
	case gen.Event:
		return t.Node == node
	case gen.Atom:
		return t == node
	}
	return false
}

func exitMessageFor(target any, reason error) any {
	switch t := target.(type) {
	case gen.PID:
		return gen.MessageExitPID{PID: t, Reason: reason}
	case gen.ProcessID:
		return gen.MessageExitProcessID{ProcessID: t, Reason: reason}
	case gen.Alias:
		return gen.MessageExitAlias{Alias: t, Reason: reason}
	case gen.Event:
		return gen.MessageExitEvent{Event: t, Reason: reason}
	case gen.Atom:
		return gen.MessageExitNode{Name: t}
	}
	return nil
}

func downMessageFor(target any, reason error) any {
	switch t := target.(type) {
	case gen.PID:
		return gen.MessageDownPID{PID: t, Reason: reason}
	case gen.ProcessID:
		return gen.MessageDownProcessID{ProcessID: t, Reason: reason}
	case gen.Alias:
		return gen.MessageDownAlias{Alias: t, Reason: reason}
	case gen.Event:
		return gen.MessageDownEvent{Event: t, Reason: reason}
	case gen.Atom:
		return gen.MessageDownNode{Name: t}
	}
	return nil
}

func sendRemoteTerminate(conn gen.Connection, target any, reason error) {
	switch t := target.(type) {
	case gen.PID:
		conn.SendTerminatePID(t, reason)
	case gen.ProcessID:
		conn.SendTerminateProcessID(t, reason)
	case gen.Alias:
		conn.SendTerminateAlias(t, reason)
	case gen.Event:
		conn.SendTerminateEvent(t, reason)
	case gen.Atom:
		// Node targets are local-only.
	}
}
