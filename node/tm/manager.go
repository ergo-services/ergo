package tm

import (
	"sync"
	"sync/atomic"

	"ergo.services/ergo/gen"
)

type Options struct{}

type Manager struct {
	core    gen.CoreTargetManager
	storage *Storage

	events      sync.Map // gen.Event -> *eventEntry
	eventsByID  sync.Map // uint64 -> *eventEntry
	eventSeq    atomic.Uint64
	eventsCount atomic.Int64

	exitsProduced    atomic.Int64
	exitsDelivered   atomic.Int64
	downsProduced    atomic.Int64
	downsDelivered   atomic.Int64
	eventsPublished  atomic.Int64
	eventsReceived   atomic.Int64
	eventsLocalSent  atomic.Int64
	eventsRemoteSent atomic.Int64

	wires sync.Map // wireKey -> *wirePresence
}

func Create(core gen.CoreTargetManager, options Options) gen.TargetManager {
	return &Manager{
		core:    core,
		storage: NewStorage(),
	}
}

func (m *Manager) HasLink(consumer gen.PID, target any) bool {
	return m.storage.Has(target, consumer, KindLink)
}

func (m *Manager) HasMonitor(consumer gen.PID, target any) bool {
	return m.storage.Has(target, consumer, KindMonitor)
}

func (m *Manager) LinkPID(consumer gen.PID, target gen.PID) error {
	if m.storage.Register(target, consumer, KindLink) == false {
		if consumer.Node != m.core.Name() {
			return nil
		}
		return gen.ErrTargetExist
	}
	if target.Node == m.core.Name() {
		return nil
	}
	wp := m.wireFor(target, KindLink)
	if consumer.Node == m.core.Name() {
		wp.localCount.Add(1)
	}
	err := m.ensureRemoteLink(wp, target.Node, func(conn gen.Connection) error {
		return conn.LinkPID(m.core.PID(), target)
	})
	if err != nil {
		if consumer.Node == m.core.Name() {
			wp.localCount.Add(-1)
		}
		m.storage.Unregister(target, consumer, KindLink)
		return err
	}
	return nil
}

func (m *Manager) UnlinkPID(consumer gen.PID, target gen.PID) error {
	if m.storage.Unregister(target, consumer, KindLink) == false {
		return nil
	}
	if target.Node == m.core.Name() {
		return nil
	}
	wp := m.wireForExisting(target, KindLink)
	if wp != nil && consumer.Node == m.core.Name() {
		wp.localCount.Add(-1)
	}
	m.ensureRemoteUnlink(wp, target.Node, func(conn gen.Connection) {
		conn.UnlinkPID(m.core.PID(), target)
	})
	return nil
}

func (m *Manager) MonitorPID(consumer gen.PID, target gen.PID) error {
	if m.storage.Register(target, consumer, KindMonitor) == false {
		if consumer.Node != m.core.Name() {
			return nil
		}
		return gen.ErrTargetExist
	}
	if target.Node == m.core.Name() {
		return nil
	}
	wp := m.wireFor(target, KindMonitor)
	if consumer.Node == m.core.Name() {
		wp.localCount.Add(1)
	}
	err := m.ensureRemoteLink(wp, target.Node, func(conn gen.Connection) error {
		return conn.MonitorPID(m.core.PID(), target)
	})
	if err != nil {
		if consumer.Node == m.core.Name() {
			wp.localCount.Add(-1)
		}
		m.storage.Unregister(target, consumer, KindMonitor)
		return err
	}
	return nil
}

func (m *Manager) DemonitorPID(consumer gen.PID, target gen.PID) error {
	if m.storage.Unregister(target, consumer, KindMonitor) == false {
		return nil
	}
	if target.Node == m.core.Name() {
		return nil
	}
	wp := m.wireForExisting(target, KindMonitor)
	if wp != nil && consumer.Node == m.core.Name() {
		wp.localCount.Add(-1)
	}
	m.ensureRemoteUnlink(wp, target.Node, func(conn gen.Connection) {
		conn.DemonitorPID(m.core.PID(), target)
	})
	return nil
}

func (m *Manager) LinkProcessID(consumer gen.PID, target gen.ProcessID) error {
	if m.storage.Register(target, consumer, KindLink) == false {
		if consumer.Node != m.core.Name() {
			return nil
		}
		return gen.ErrTargetExist
	}
	node := processIDNode(target, m.core.Name())
	if node == m.core.Name() {
		return nil
	}
	wp := m.wireFor(target, KindLink)
	if consumer.Node == m.core.Name() {
		wp.localCount.Add(1)
	}
	err := m.ensureRemoteLink(wp, node, func(conn gen.Connection) error {
		return conn.LinkProcessID(m.core.PID(), target)
	})
	if err != nil {
		if consumer.Node == m.core.Name() {
			wp.localCount.Add(-1)
		}
		m.storage.Unregister(target, consumer, KindLink)
		return err
	}
	return nil
}

func (m *Manager) UnlinkProcessID(consumer gen.PID, target gen.ProcessID) error {
	if m.storage.Unregister(target, consumer, KindLink) == false {
		return nil
	}
	node := processIDNode(target, m.core.Name())
	if node == m.core.Name() {
		return nil
	}
	wp := m.wireForExisting(target, KindLink)
	if wp != nil && consumer.Node == m.core.Name() {
		wp.localCount.Add(-1)
	}
	m.ensureRemoteUnlink(wp, node, func(conn gen.Connection) {
		conn.UnlinkProcessID(m.core.PID(), target)
	})
	return nil
}

func (m *Manager) MonitorProcessID(consumer gen.PID, target gen.ProcessID) error {
	if m.storage.Register(target, consumer, KindMonitor) == false {
		if consumer.Node != m.core.Name() {
			return nil
		}
		return gen.ErrTargetExist
	}
	node := processIDNode(target, m.core.Name())
	if node == m.core.Name() {
		return nil
	}
	wp := m.wireFor(target, KindMonitor)
	if consumer.Node == m.core.Name() {
		wp.localCount.Add(1)
	}
	err := m.ensureRemoteLink(wp, node, func(conn gen.Connection) error {
		return conn.MonitorProcessID(m.core.PID(), target)
	})
	if err != nil {
		if consumer.Node == m.core.Name() {
			wp.localCount.Add(-1)
		}
		m.storage.Unregister(target, consumer, KindMonitor)
		return err
	}
	return nil
}

func (m *Manager) DemonitorProcessID(consumer gen.PID, target gen.ProcessID) error {
	if m.storage.Unregister(target, consumer, KindMonitor) == false {
		return nil
	}
	node := processIDNode(target, m.core.Name())
	if node == m.core.Name() {
		return nil
	}
	wp := m.wireForExisting(target, KindMonitor)
	if wp != nil && consumer.Node == m.core.Name() {
		wp.localCount.Add(-1)
	}
	m.ensureRemoteUnlink(wp, node, func(conn gen.Connection) {
		conn.DemonitorProcessID(m.core.PID(), target)
	})
	return nil
}

func processIDNode(t gen.ProcessID, self gen.Atom) gen.Atom {
	if t.Node == "" {
		return self
	}
	return t.Node
}

func (m *Manager) LinkAlias(consumer gen.PID, target gen.Alias) error {
	if m.storage.Register(target, consumer, KindLink) == false {
		if consumer.Node != m.core.Name() {
			return nil
		}
		return gen.ErrTargetExist
	}
	if target.Node == m.core.Name() {
		return nil
	}
	wp := m.wireFor(target, KindLink)
	if consumer.Node == m.core.Name() {
		wp.localCount.Add(1)
	}
	err := m.ensureRemoteLink(wp, target.Node, func(conn gen.Connection) error {
		return conn.LinkAlias(m.core.PID(), target)
	})
	if err != nil {
		if consumer.Node == m.core.Name() {
			wp.localCount.Add(-1)
		}
		m.storage.Unregister(target, consumer, KindLink)
		return err
	}
	return nil
}

func (m *Manager) UnlinkAlias(consumer gen.PID, target gen.Alias) error {
	if m.storage.Unregister(target, consumer, KindLink) == false {
		return nil
	}
	if target.Node == m.core.Name() {
		return nil
	}
	wp := m.wireForExisting(target, KindLink)
	if wp != nil && consumer.Node == m.core.Name() {
		wp.localCount.Add(-1)
	}
	m.ensureRemoteUnlink(wp, target.Node, func(conn gen.Connection) {
		conn.UnlinkAlias(m.core.PID(), target)
	})
	return nil
}

func (m *Manager) MonitorAlias(consumer gen.PID, target gen.Alias) error {
	if m.storage.Register(target, consumer, KindMonitor) == false {
		if consumer.Node != m.core.Name() {
			return nil
		}
		return gen.ErrTargetExist
	}
	if target.Node == m.core.Name() {
		return nil
	}
	wp := m.wireFor(target, KindMonitor)
	if consumer.Node == m.core.Name() {
		wp.localCount.Add(1)
	}
	err := m.ensureRemoteLink(wp, target.Node, func(conn gen.Connection) error {
		return conn.MonitorAlias(m.core.PID(), target)
	})
	if err != nil {
		if consumer.Node == m.core.Name() {
			wp.localCount.Add(-1)
		}
		m.storage.Unregister(target, consumer, KindMonitor)
		return err
	}
	return nil
}

func (m *Manager) DemonitorAlias(consumer gen.PID, target gen.Alias) error {
	if m.storage.Unregister(target, consumer, KindMonitor) == false {
		return nil
	}
	if target.Node == m.core.Name() {
		return nil
	}
	wp := m.wireForExisting(target, KindMonitor)
	if wp != nil && consumer.Node == m.core.Name() {
		wp.localCount.Add(-1)
	}
	m.ensureRemoteUnlink(wp, target.Node, func(conn gen.Connection) {
		conn.DemonitorAlias(m.core.PID(), target)
	})
	return nil
}

// Node targets are always local; no wire propagation.
func (m *Manager) LinkNode(consumer gen.PID, target gen.Atom) error {
	if m.storage.Register(target, consumer, KindLink) {
		return nil
	}
	return gen.ErrTargetExist
}

func (m *Manager) UnlinkNode(consumer gen.PID, target gen.Atom) error {
	m.storage.Unregister(target, consumer, KindLink)
	return nil
}

func (m *Manager) MonitorNode(consumer gen.PID, target gen.Atom) error {
	if m.storage.Register(target, consumer, KindMonitor) {
		return nil
	}
	return gen.ErrTargetExist
}

func (m *Manager) DemonitorNode(consumer gen.PID, target gen.Atom) error {
	m.storage.Unregister(target, consumer, KindMonitor)
	return nil
}

func (m *Manager) LinksFor(consumer gen.PID) []any {
	return m.storage.LinksFor(consumer)
}

func (m *Manager) MonitorsFor(consumer gen.PID) []any {
	return m.storage.MonitorsFor(consumer)
}

func (m *Manager) EventsFor(producer gen.PID) []gen.Event {
	var events []gen.Event
	m.events.Range(func(_, v any) bool {
		e := v.(*eventEntry)
		if e.producer == producer {
			events = append(events, e.event)
		}
		return true
	})
	return events
}

func (m *Manager) Info() gen.TargetManagerInfo {
	links, monitors := m.storage.Counts()
	return gen.TargetManagerInfo{
		Links:                 links,
		Monitors:              monitors,
		Events:                m.eventsCount.Load(),
		ExitSignalsProduced:   m.exitsProduced.Load(),
		ExitSignalsDelivered:  m.exitsDelivered.Load(),
		DownMessagesProduced:  m.downsProduced.Load(),
		DownMessagesDelivered: m.downsDelivered.Load(),
		EventsPublished:       m.eventsPublished.Load(),
		EventsReceived:        m.eventsReceived.Load(),
		EventsLocalSent:       m.eventsLocalSent.Load(),
		EventsRemoteSent:      m.eventsRemoteSent.Load(),
	}
}
