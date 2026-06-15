package tm

import (
	"net"
	"sync/atomic"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// mockCore mirrors tm's mockCore with lock-free queues for recorded
// operations, so ported tests can read them via Item()/Next() and the
// count/get helpers below.
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

func (m *mockCore) Name() gen.Atom { return m.name }
func (m *mockCore) PID() gen.PID   { return m.pid }
func (m *mockCore) Log() gen.Log   { return nil }
func (m *mockCore) MakeRef() gen.Ref {
	return gen.Ref{Node: m.name, Creation: m.pid.Creation, ID: [3]uint64{m.refSeq.Add(1), 0, 0}}
}

func (m *mockCore) RouteSendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
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
}

func (m *mockCore) RouteSendExitMessages(from gen.PID, to []gen.PID, message any) error {
	for _, pid := range to {
		m.sentExits.Push(exitRequest{from: from, to: pid, message: message})
	}
	return nil
}

func (m *mockCore) RouteSendEventMessages(from gen.PID, to []gen.PID, options gen.MessageOptions, message gen.MessageEvent) error {
	for _, pid := range to {
		m.sentEvents.Push(eventDelivery{from: from, to: pid})
	}
	return nil
}

func (m *mockCore) GetConnection(node gen.Atom) (gen.Connection, error) {
	if m.connectionError != nil {
		return nil, m.connectionError
	}
	return &mockConnection{core: m, linkError: m.linkError}, nil
}

type mockConnection struct {
	core      *mockCore
	linkError error
}

func (c *mockConnection) Node() gen.RemoteNode { return nil }

func (c *mockConnection) LinkPID(from gen.PID, to gen.PID) error {
	if c.linkError != nil {
		return c.linkError
	}
	c.core.sentLinks.Push(linkRequest{from: from, to: to})
	return nil
}

func (c *mockConnection) UnlinkPID(from gen.PID, to gen.PID) error {
	c.core.sentUnlinks.Push(linkRequest{from: from, to: to})
	return nil
}

func (c *mockConnection) MonitorPID(from gen.PID, to gen.PID) error {
	if c.linkError != nil {
		return c.linkError
	}
	c.core.sentMonitors.Push(monitorRequest{from: from, to: to})
	return nil
}

func (c *mockConnection) DemonitorPID(from gen.PID, to gen.PID) error {
	c.core.sentDemonitors.Push(monitorRequest{from: from, to: to})
	return nil
}

func (c *mockConnection) LinkProcessID(from gen.PID, to gen.ProcessID) error {
	if c.linkError != nil {
		return c.linkError
	}
	c.core.sentLinks.Push(linkRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
	return nil
}

func (c *mockConnection) UnlinkProcessID(from gen.PID, to gen.ProcessID) error {
	c.core.sentUnlinks.Push(linkRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
	return nil
}

func (c *mockConnection) MonitorProcessID(from gen.PID, to gen.ProcessID) error {
	if c.linkError != nil {
		return c.linkError
	}
	c.core.sentMonitors.Push(monitorRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
	return nil
}

func (c *mockConnection) DemonitorProcessID(from gen.PID, to gen.ProcessID) error {
	c.core.sentDemonitors.Push(monitorRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
	return nil
}

func (c *mockConnection) LinkAlias(from gen.PID, to gen.Alias) error {
	if c.linkError != nil {
		return c.linkError
	}
	c.core.sentLinks.Push(linkRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
	return nil
}

func (c *mockConnection) UnlinkAlias(from gen.PID, to gen.Alias) error {
	c.core.sentUnlinks.Push(linkRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
	return nil
}

func (c *mockConnection) MonitorAlias(from gen.PID, to gen.Alias) error {
	if c.linkError != nil {
		return c.linkError
	}
	c.core.sentMonitors.Push(monitorRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
	return nil
}

func (c *mockConnection) DemonitorAlias(from gen.PID, to gen.Alias) error {
	c.core.sentDemonitors.Push(monitorRequest{from: from, to: gen.PID{Node: to.Node, ID: 0}})
	return nil
}

func (c *mockConnection) LinkEvent(from gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	if c.linkError != nil {
		return nil, c.linkError
	}
	c.core.sentEventLinks.Push(eventLinkRequest{from: from, event: event})
	var buffer []gen.MessageEvent
	if c.core.eventBuffers != nil {
		buffer = c.core.eventBuffers[event]
	}
	return buffer, nil
}

func (c *mockConnection) UnlinkEvent(from gen.PID, event gen.Event) error {
	return nil
}

func (c *mockConnection) MonitorEvent(from gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	if c.linkError != nil {
		return nil, c.linkError
	}
	c.core.sentEventLinks.Push(eventLinkRequest{from: from, event: event})
	var buffer []gen.MessageEvent
	if c.core.eventBuffers != nil {
		buffer = c.core.eventBuffers[event]
	}
	return buffer, nil
}

func (c *mockConnection) DemonitorEvent(from gen.PID, event gen.Event) error {
	return nil
}

func (c *mockConnection) SendTerminatePID(target gen.PID, reason error) error {
	c.core.sentTermPIDs.Push(termRequest{target: target, reason: reason})
	return nil
}

func (c *mockConnection) SendTerminateProcessID(target gen.ProcessID, reason error) error {
	c.core.sentTermProcIDs.Push(termRequest{target: target, reason: reason})
	return nil
}

func (c *mockConnection) SendTerminateAlias(target gen.Alias, reason error) error {
	c.core.sentTermAliases.Push(termRequest{target: target, reason: reason})
	return nil
}

func (c *mockConnection) SendTerminateEvent(target gen.Event, reason error) error {
	c.core.sentTermEvents.Push(termRequest{target: target, reason: reason})
	return nil
}

// Stubs for the rest of gen.Connection.

func (c *mockConnection) SendPID(gen.PID, gen.PID, gen.MessageOptions, any) error {
	return nil
}
func (c *mockConnection) SendProcessID(gen.PID, gen.ProcessID, gen.MessageOptions, any) error {
	return nil
}
func (c *mockConnection) SendAlias(gen.PID, gen.Alias, gen.MessageOptions, any) error { return nil }
func (c *mockConnection) SendEvent(gen.PID, gen.MessageOptions, gen.MessageEvent) error {
	return nil
}
func (c *mockConnection) CallPID(gen.PID, gen.PID, gen.MessageOptions, any) error { return nil }
func (c *mockConnection) CallProcessID(gen.PID, gen.ProcessID, gen.MessageOptions, any) error {
	return nil
}
func (c *mockConnection) CallAlias(gen.PID, gen.Alias, gen.MessageOptions, any) error { return nil }
func (c *mockConnection) SendExit(gen.PID, gen.PID, error) error                      { return nil }
func (c *mockConnection) SendResponse(gen.PID, gen.PID, gen.MessageOptions, any) error {
	return nil
}
func (c *mockConnection) SendResponseError(gen.PID, gen.PID, gen.MessageOptions, error) error {
	return nil
}
func (c *mockConnection) RemoteSpawn(gen.Atom, gen.ProcessOptionsExtra) (gen.PID, error) {
	return gen.PID{}, nil
}
func (c *mockConnection) Join(net.Conn, string, gen.NetworkDial, []byte) error { return nil }
func (c *mockConnection) Terminate(error)                                      {}

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
	return Create(core, Options{}).(*Manager), core
}
