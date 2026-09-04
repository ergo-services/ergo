package tm

import (
	"sync"
	"sync/atomic"

	"ergo.services/ergo/gen"
)

type wireKey struct {
	target any
	kind   Kind
}

const (
	wireInitial   uint32 = 0
	wirePiggyback uint32 = 1 // subsequent subscribers piggyback, no wire-call
	wireBuffered  uint32 = 2 // every subscriber does its own wire-call (buffered event)
)

// localCount is the authoritative "any local consumer left?" check for
// ensureRemoteUnlink. sync.Map.Range over the storage index is
// best-effort and can miss a recently-added entry; an atomic counter
// cannot.
type wirePresence struct {
	mu         sync.Mutex
	state      atomic.Uint32
	localCount atomic.Int32
}

func (m *Manager) wireFor(target any, kind Kind) *wirePresence {
	key := wireKey{target: target, kind: kind}
	if v, ok := m.wires.Load(key); ok {
		return v.(*wirePresence)
	}
	wp := &wirePresence{}
	actual, _ := m.wires.LoadOrStore(key, wp)
	return actual.(*wirePresence)
}

// wireForExisting returns nil if the slot was never created (or was
// cleared by resetWire on target termination). Used by unsubscribe
// paths to avoid leaking a fresh wp for a dead target.
func (m *Manager) wireForExisting(target any, kind Kind) *wirePresence {
	v, ok := m.wires.Load(wireKey{target: target, kind: kind})
	if ok == false {
		return nil
	}
	return v.(*wirePresence)
}

func (m *Manager) ensureRemoteLink(wp *wirePresence, node gen.Atom, doLink func(conn gen.Connection) error) error {
	if wp.state.Load() == wirePiggyback {
		return nil
	}
	wp.mu.Lock()
	defer wp.mu.Unlock()
	if wp.state.Load() == wirePiggyback {
		return nil
	}
	conn, err := m.core.GetConnection(node)
	if err != nil {
		return err
	}
	if err := doLink(conn); err != nil {
		return err
	}
	wp.state.Store(wirePiggyback)
	return nil
}

// Buffered events always make their own wire-call (each subscriber needs
// a fresh buffer snapshot). Non-buffered piggyback after the first.
func (m *Manager) ensureRemoteLinkEvent(wp *wirePresence, event gen.Event, kind Kind) ([]gen.MessageEvent, error) {
	switch wp.state.Load() {
	case wirePiggyback:
		return nil, nil
	case wireBuffered:
		return m.doWireLinkEvent(event, kind)
	}

	wp.mu.Lock()
	defer wp.mu.Unlock()
	switch wp.state.Load() {
	case wirePiggyback:
		return nil, nil
	case wireBuffered:
		return m.doWireLinkEvent(event, kind)
	}

	buffer, err := m.doWireLinkEvent(event, kind)
	if err != nil {
		return nil, err
	}
	if buffer == nil {
		wp.state.Store(wirePiggyback)
		return nil, nil
	}
	wp.state.Store(wireBuffered)
	return buffer, nil
}

func (m *Manager) doWireLinkEvent(event gen.Event, kind Kind) ([]gen.MessageEvent, error) {
	conn, err := m.core.GetConnection(event.Node)
	if err != nil {
		return nil, err
	}
	if kind == KindLink {
		return conn.LinkEvent(m.core.PID(), event)
	}
	return conn.MonitorEvent(m.core.PID(), event)
}

// Caller must Unregister and decrement wp.localCount before calling.
// state.Store(wireInitial) happens BEFORE the wire-call so a fast-path
// Link reader that sees piggyback knows the wire-link is still alive.
// The second localCount check rolls state back if a concurrent Register
// raced in, so the new consumer can piggyback on the surviving link.
func (m *Manager) ensureRemoteUnlink(wp *wirePresence, node gen.Atom, doUnlink func(conn gen.Connection)) {
	if wp == nil {
		return
	}
	wp.mu.Lock()
	defer wp.mu.Unlock()
	prev := wp.state.Load()
	if prev == wireInitial {
		return
	}
	if wp.localCount.Load() > 0 {
		return
	}
	wp.state.Store(wireInitial)
	if wp.localCount.Load() > 0 {
		wp.state.Store(prev)
		return
	}
	conn, err := m.core.GetConnection(node)
	if err == nil {
		doUnlink(conn)
	}
}

// Drops wire-state on target termination so a re-registration starts fresh.
func (m *Manager) resetWire(target any) {
	m.wires.Delete(wireKey{target: target, kind: KindLink})
	m.wires.Delete(wireKey{target: target, kind: KindMonitor})
}
