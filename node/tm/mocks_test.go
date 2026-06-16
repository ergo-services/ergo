package tm

import (
	"sync/atomic"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/testing/mock"
)

// mockCore is the tm test recorder: it owns lock-free queues of recorded
// operations (read via the count/get helpers below) and builds the gen
// CoreTargetManager / Connection that the Manager talks to, wiring each routed
// operation into those queues through testing/mock overrides.
type mockCore struct {
	name            gen.Atom
	pid             gen.PID
	sentLinks       lib.QueueMPSC
	sentUnlinks     lib.QueueMPSC
	sentMonitors    lib.QueueMPSC
	sentDemonitors  lib.QueueMPSC
	sentExits       lib.QueueMPSC
	sentDowns       lib.QueueMPSC
	sentEventLinks  lib.QueueMPSC
	sentEventStarts lib.QueueMPSC
	sentEventStops  lib.QueueMPSC
	sentEvents      lib.QueueMPSC
	sentTermPIDs    lib.QueueMPSC
	sentTermProcIDs lib.QueueMPSC
	sentTermAliases lib.QueueMPSC
	sentTermEvents  lib.QueueMPSC
	eventBuffers    map[gen.Event][]gen.MessageEvent
	connectionError error
	linkError       error
	refSeq          atomic.Uint64
}

type linkRequest struct {
	from gen.PID
	to   gen.PID
}

type monitorRequest struct {
	from gen.PID
	to   gen.PID
}

type exitRequest struct {
	from    gen.PID
	to      gen.PID
	message any
}

type downRequest struct {
	from    gen.PID
	to      gen.PID
	message any
}

type eventLinkRequest struct {
	from  gen.PID
	event gen.Event
}

type eventNotification struct {
	producer gen.PID
}

type eventDelivery struct {
	from gen.PID
	to   gen.PID
}

type termRequest struct {
	target any
	reason error
}

func newMockCore(nodeName string) *mockCore {
	return &mockCore{
		name:            gen.Atom(nodeName),
		pid:             gen.PID{Node: gen.Atom(nodeName), ID: 1, Creation: 100},
		sentLinks:       lib.NewQueueMPSC(),
		sentUnlinks:     lib.NewQueueMPSC(),
		sentMonitors:    lib.NewQueueMPSC(),
		sentDemonitors:  lib.NewQueueMPSC(),
		sentExits:       lib.NewQueueMPSC(),
		sentDowns:       lib.NewQueueMPSC(),
		sentEventLinks:  lib.NewQueueMPSC(),
		sentEventStarts: lib.NewQueueMPSC(),
		sentEventStops:  lib.NewQueueMPSC(),
		sentEvents:      lib.NewQueueMPSC(),
		sentTermPIDs:    lib.NewQueueMPSC(),
		sentTermProcIDs: lib.NewQueueMPSC(),
		sentTermAliases: lib.NewQueueMPSC(),
		sentTermEvents:  lib.NewQueueMPSC(),
	}
}

// ctm builds the gen.CoreTargetManager the Manager is created with: identity and
// MakeRef come from this mockCore, and each routed delivery is recorded into the
// matching queue. GetConnection hands back a recording connection (or the
// configured connection error).
func (m *mockCore) ctm() gen.CoreTargetManager {
	c := mock.NewCoreTargetManager()
	c.OnName(func() gen.Atom { return m.name })
	c.OnPID(func() gen.PID { return m.pid })
	c.OnMakeRef(func() gen.Ref {
		return gen.Ref{Node: m.name, Creation: m.pid.Creation, ID: [3]uint64{m.refSeq.Add(1), 0, 0}}
	})
	c.OnRouteSendPID(func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
		switch message.(type) {
		case gen.MessageDownPID, gen.MessageDownProcessID, gen.MessageDownAlias, gen.MessageDownEvent, gen.MessageDownNode:
			m.sentDowns.Push(downRequest{from: from, to: to, message: message})
		case gen.MessageEventStart:
			m.sentEventStarts.Push(eventNotification{producer: to})
		case gen.MessageEventStop:
			m.sentEventStops.Push(eventNotification{producer: to})
		case gen.MessageEvent:
			m.sentEvents.Push(eventDelivery{from: from, to: to})
		}
		return nil
	})
	c.OnRouteSendExitMessages(func(from gen.PID, to []gen.PID, message any) error {
		for _, pid := range to {
			m.sentExits.Push(exitRequest{from: from, to: pid, message: message})
		}
		return nil
	})
	c.OnRouteSendEventMessages(func(from gen.PID, to []gen.PID, options gen.MessageOptions, message gen.MessageEvent) error {
		for _, pid := range to {
			m.sentEvents.Push(eventDelivery{from: from, to: pid})
		}
		return nil
	})
	c.OnGetConnection(func(node gen.Atom) (gen.Connection, error) {
		if m.connectionError != nil {
			return nil, m.connectionError
		}
		return m.conn(), nil
	})
	return c
}

// conn builds a recording gen.Connection. It snapshots linkError at build time
// (mirroring the per-connection capture the manager sees) and records every
// link/monitor/event/terminate into the parent mockCore's queues.
func (m *mockCore) conn() gen.Connection {
	linkErr := m.linkError
	c := mock.NewConnection()

	c.OnLinkPID(func(from gen.PID, to gen.PID) error {
		if linkErr != nil {
			return linkErr
		}
		m.sentLinks.Push(linkRequest{from: from, to: to})
		return nil
	})
	c.OnUnlinkPID(func(from gen.PID, to gen.PID) error {
		m.sentUnlinks.Push(linkRequest{from: from, to: to})
		return nil
	})
	c.OnMonitorPID(func(from gen.PID, to gen.PID) error {
		if linkErr != nil {
			return linkErr
		}
		m.sentMonitors.Push(monitorRequest{from: from, to: to})
		return nil
	})
	c.OnDemonitorPID(func(from gen.PID, to gen.PID) error {
		m.sentDemonitors.Push(monitorRequest{from: from, to: to})
		return nil
	})

	c.OnLinkProcessID(func(from gen.PID, to gen.ProcessID) error {
		if linkErr != nil {
			return linkErr
		}
		m.sentLinks.Push(linkRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
		return nil
	})
	c.OnUnlinkProcessID(func(from gen.PID, to gen.ProcessID) error {
		m.sentUnlinks.Push(linkRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
		return nil
	})
	c.OnMonitorProcessID(func(from gen.PID, to gen.ProcessID) error {
		if linkErr != nil {
			return linkErr
		}
		m.sentMonitors.Push(monitorRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
		return nil
	})
	c.OnDemonitorProcessID(func(from gen.PID, to gen.ProcessID) error {
		m.sentDemonitors.Push(monitorRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
		return nil
	})

	c.OnLinkAlias(func(from gen.PID, to gen.Alias) error {
		if linkErr != nil {
			return linkErr
		}
		m.sentLinks.Push(linkRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
		return nil
	})
	c.OnUnlinkAlias(func(from gen.PID, to gen.Alias) error {
		m.sentUnlinks.Push(linkRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
		return nil
	})
	c.OnMonitorAlias(func(from gen.PID, to gen.Alias) error {
		if linkErr != nil {
			return linkErr
		}
		m.sentMonitors.Push(monitorRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
		return nil
	})
	c.OnDemonitorAlias(func(from gen.PID, to gen.Alias) error {
		m.sentDemonitors.Push(monitorRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
		return nil
	})

	c.OnLinkEvent(func(from gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
		if linkErr != nil {
			return nil, linkErr
		}
		m.sentEventLinks.Push(eventLinkRequest{from: from, event: event})
		if m.eventBuffers != nil {
			return m.eventBuffers[event], nil
		}
		return nil, nil
	})
	c.OnMonitorEvent(func(from gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
		if linkErr != nil {
			return nil, linkErr
		}
		m.sentEventLinks.Push(eventLinkRequest{from: from, event: event})
		if m.eventBuffers != nil {
			return m.eventBuffers[event], nil
		}
		return nil, nil
	})

	c.OnSendTerminatePID(func(target gen.PID, reason error) error {
		m.sentTermPIDs.Push(termRequest{target: target, reason: reason})
		return nil
	})
	c.OnSendTerminateProcessID(func(target gen.ProcessID, reason error) error {
		m.sentTermProcIDs.Push(termRequest{target: target, reason: reason})
		return nil
	})
	c.OnSendTerminateAlias(func(target gen.Alias, reason error) error {
		m.sentTermAliases.Push(termRequest{target: target, reason: reason})
		return nil
	})
	c.OnSendTerminateEvent(func(target gen.Event, reason error) error {
		m.sentTermEvents.Push(termRequest{target: target, reason: reason})
		return nil
	})

	return c
}

// Queue helpers

func countQueue(q lib.QueueMPSC) int {
	n := 0
	for it := q.Item(); it != nil; it = it.Next() {
		n++
	}
	return n
}

func (m *mockCore) countSentLinks() int       { return countQueue(m.sentLinks) }
func (m *mockCore) countSentUnlinks() int     { return countQueue(m.sentUnlinks) }
func (m *mockCore) countSentMonitors() int    { return countQueue(m.sentMonitors) }
func (m *mockCore) countSentDemonitors() int  { return countQueue(m.sentDemonitors) }
func (m *mockCore) countSentExits() int       { return countQueue(m.sentExits) }
func (m *mockCore) countSentDowns() int       { return countQueue(m.sentDowns) }
func (m *mockCore) countSentEventLinks() int  { return countQueue(m.sentEventLinks) }
func (m *mockCore) countSentEventStarts() int { return countQueue(m.sentEventStarts) }
func (m *mockCore) countSentEventStops() int  { return countQueue(m.sentEventStops) }
func (m *mockCore) countSentEvents() int      { return countQueue(m.sentEvents) }
func (m *mockCore) countSentTermPIDs() int    { return countQueue(m.sentTermPIDs) }
func (m *mockCore) countSentTermEvents() int  { return countQueue(m.sentTermEvents) }

func (m *mockCore) resetSentLinks()       { m.sentLinks = lib.NewQueueMPSC() }
func (m *mockCore) resetSentUnlinks()     { m.sentUnlinks = lib.NewQueueMPSC() }
func (m *mockCore) resetSentMonitors()    { m.sentMonitors = lib.NewQueueMPSC() }
func (m *mockCore) resetSentDemonitors()  { m.sentDemonitors = lib.NewQueueMPSC() }
func (m *mockCore) resetSentExits()       { m.sentExits = lib.NewQueueMPSC() }
func (m *mockCore) resetSentDowns()       { m.sentDowns = lib.NewQueueMPSC() }
func (m *mockCore) resetSentEvents()      { m.sentEvents = lib.NewQueueMPSC() }
func (m *mockCore) resetSentEventStarts() { m.sentEventStarts = lib.NewQueueMPSC() }
func (m *mockCore) resetSentEventStops()  { m.sentEventStops = lib.NewQueueMPSC() }
func (m *mockCore) resetSentEventLinks()  { m.sentEventLinks = lib.NewQueueMPSC() }

func (m *mockCore) getFirstSentLink() (linkRequest, bool) {
	item := m.sentLinks.Item()
	if item == nil {
		return linkRequest{}, false
	}
	return item.Value().(linkRequest), true
}

func (m *mockCore) getFirstSentUnlink() (linkRequest, bool) {
	item := m.sentUnlinks.Item()
	if item == nil {
		return linkRequest{}, false
	}
	return item.Value().(linkRequest), true
}

func (m *mockCore) getFirstSentMonitor() (monitorRequest, bool) {
	item := m.sentMonitors.Item()
	if item == nil {
		return monitorRequest{}, false
	}
	return item.Value().(monitorRequest), true
}

func (m *mockCore) getFirstSentDemonitor() (monitorRequest, bool) {
	item := m.sentDemonitors.Item()
	if item == nil {
		return monitorRequest{}, false
	}
	return item.Value().(monitorRequest), true
}

func (m *mockCore) getFirstSentExit() (exitRequest, bool) {
	item := m.sentExits.Item()
	if item == nil {
		return exitRequest{}, false
	}
	return item.Value().(exitRequest), true
}

func (m *mockCore) getFirstSentDown() (downRequest, bool) {
	item := m.sentDowns.Item()
	if item == nil {
		return downRequest{}, false
	}
	return item.Value().(downRequest), true
}

func (m *mockCore) getAllSentExits() []exitRequest {
	var out []exitRequest
	for it := m.sentExits.Item(); it != nil; it = it.Next() {
		out = append(out, it.Value().(exitRequest))
	}
	return out
}

func (m *mockCore) getAllSentDowns() []downRequest {
	var out []downRequest
	for it := m.sentDowns.Item(); it != nil; it = it.Next() {
		out = append(out, it.Value().(downRequest))
	}
	return out
}

func (m *mockCore) getAllSentEvents() []eventDelivery {
	var out []eventDelivery
	for it := m.sentEvents.Item(); it != nil; it = it.Next() {
		out = append(out, it.Value().(eventDelivery))
	}
	return out
}

// newManagerWithMock constructs a Manager backed by a fresh mockCore.
func newManagerWithMock(nodeName string) (*Manager, *mockCore) {
	core := newMockCore(nodeName)
	return Create(core.ctm(), Options{}).(*Manager), core
}
