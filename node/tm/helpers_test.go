package tm

import "ergo.services/ergo/gen"

// Test-only accessors into Manager internals (sync.Map storage,
// reverse index, wirePresence).

func (m *Manager) totalLinks() int    { l, _ := m.storage.Counts(); return int(l) }
func (m *Manager) totalMonitors() int { _, mn := m.storage.Counts(); return int(mn) }

func (m *Manager) totalEvents() int {
	count := 0
	m.events.Range(func(_, _ any) bool { count++; return true })
	return count
}

func (m *Manager) totalTargets() int {
	count := 0
	m.storage.targets.Range(func(_, _ any) bool { count++; return true })
	return count
}

func (m *Manager) hasLinkRelation(consumer gen.PID, target any) bool {
	return m.storage.Has(target, consumer, KindLink)
}

func (m *Manager) hasMonitorRelation(consumer gen.PID, target any) bool {
	return m.storage.Has(target, consumer, KindMonitor)
}

func (m *Manager) getTargetEntry(target any) *targetEntry {
	v, ok := m.storage.targets.Load(target)
	if ok == false {
		return nil
	}
	return v.(*targetEntry)
}

func (m *Manager) getEventEntry(event gen.Event) *eventEntry {
	v, ok := m.events.Load(event)
	if ok == false {
		return nil
	}
	return v.(*eventEntry)
}

// consumerCount reports the number of distinct (consumer, kind) pairs in
// the target's index. Used by ported tests that previously inspected
// entry.consumers map length.
func (m *Manager) consumerCount(target any) int {
	v, ok := m.storage.targets.Load(target)
	if ok == false {
		return 0
	}
	entry := v.(*targetEntry)
	n := 0
	entry.index.Range(func(_, _ any) bool { n++; return true })
	return n
}

// wireEstablished reports whether the wire-link for (target, kind) has
// transitioned out of the initial state. Mirrors tm's
// entry.allowAlwaysFirst == false check.
func (m *Manager) wireEstablished(target any, kind Kind) bool {
	v, ok := m.wires.Load(wireKey{target: target, kind: kind})
	if ok == false {
		return false
	}
	return v.(*wirePresence).state.Load() != wireInitial
}

// producerEvents returns the set of events registered with this producer.
// Mirrors tm's helper.
func (m *Manager) producerEvents(producer gen.PID) map[gen.Event]struct{} {
	out := map[gen.Event]struct{}{}
	m.events.Range(func(_, v any) bool {
		e := v.(*eventEntry)
		if e.producer == producer {
			out[e.event] = struct{}{}
		}
		return true
	})
	if len(out) == 0 {
		return nil
	}
	return out
}
