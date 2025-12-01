package tm

import (
	"net"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

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
	eventBuffers    map[gen.Event][]gen.MessageEvent
	connectionError error
	linkError       error
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

type exitRequest struct {
	from    gen.PID
	to      gen.PID
	message any // actual exit message (gen.MessageExitPID, gen.MessageExitProcessID, etc.)
}

type downRequest struct {
	from    gen.PID
	to      gen.PID
	message any // actual down message (gen.MessageDownPID, gen.MessageDownProcessID, etc.)
}

type linkRequest struct {
	from gen.PID
	to   gen.PID
}

type monitorRequest struct {
	from gen.PID
	to   gen.PID
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
	}
}

func (m *mockCore) Name() gen.Atom {
	return m.name
}

func (m *mockCore) PID() gen.PID {
	return m.pid
}

func (m *mockCore) Log() gen.Log {
	return nil
}

func (m *mockCore) RouteSendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	// Lock-free Push to MPSC queues!
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

	return &mockConnection{
		core:      m,
		linkError: m.linkError,
	}, nil
}

type mockConnection struct {
	core      *mockCore
	linkError error
}

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

	// eventBuffers still needs mutex (map access)
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

	var buffer []gen.MessageEvent
	if c.core.eventBuffers != nil {
		buffer = c.core.eventBuffers[event]
	}

	return buffer, nil
}

func (c *mockConnection) DemonitorEvent(from gen.PID, event gen.Event) error {
	return nil
}

// Stubs for all other Connection methods (minimal implementation)
func (c *mockConnection) Node() gen.RemoteNode { return nil }
func (c *mockConnection) SendPID(gen.PID, gen.PID, gen.MessageOptions, any) error { return nil }
func (c *mockConnection) SendProcessID(gen.PID, gen.ProcessID, gen.MessageOptions, any) error {
	return nil
}
func (c *mockConnection) SendAlias(gen.PID, gen.Alias, gen.MessageOptions, any) error { return nil }
func (c *mockConnection) SendEvent(gen.PID, gen.MessageOptions, gen.MessageEvent) error {
	return nil
}
func (c *mockConnection) CallPID(gen.PID, gen.PID, gen.MessageOptions, any) error    { return nil }
func (c *mockConnection) CallProcessID(gen.PID, gen.ProcessID, gen.MessageOptions, any) error {
	return nil
}
func (c *mockConnection) CallAlias(gen.PID, gen.Alias, gen.MessageOptions, any) error { return nil }
func (c *mockConnection) SendExit(gen.PID, gen.PID, error) error                    { return nil }
func (c *mockConnection) SendResponse(gen.PID, gen.PID, gen.MessageOptions, any) error {
	return nil
}
func (c *mockConnection) SendResponseError(gen.PID, gen.PID, gen.MessageOptions, error) error {
	return nil
}
func (c *mockConnection) SendTerminatePID(gen.PID, error) error                       { return nil }
func (c *mockConnection) SendTerminateProcessID(gen.ProcessID, error) error           { return nil }
func (c *mockConnection) SendTerminateAlias(gen.Alias, error) error                   { return nil }
func (c *mockConnection) SendTerminateEvent(gen.Event, error) error                   { return nil }
func (c *mockConnection) Spawn(gen.Atom, gen.ProcessOptions, ...any) (gen.PID, error) {
	return gen.PID{}, nil
}
func (c *mockConnection) SpawnRegister(gen.Atom, gen.Atom, gen.ProcessOptions, ...any) (gen.PID, error) {
	return gen.PID{}, nil
}
func (c *mockConnection) ApplicationStart(gen.Atom, gen.ApplicationOptions) error { return nil }
func (c *mockConnection) ApplicationStartTemporary(gen.Atom, gen.ApplicationOptions) error {
	return nil
}
func (c *mockConnection) ApplicationStartTransient(gen.Atom, gen.ApplicationOptions) error {
	return nil
}
func (c *mockConnection) ApplicationStartPermanent(gen.Atom, gen.ApplicationOptions) error {
	return nil
}
func (c *mockConnection) ApplicationInfo(gen.Atom) (gen.ApplicationInfo, error) {
	return gen.ApplicationInfo{}, nil
}
func (c *mockConnection) SendEventMessage(gen.PID, gen.Event, gen.MessageOptions, gen.MessageEvent) error {
	return nil
}
func (c *mockConnection) Join(net.Conn, string, gen.NetworkDial, []byte) error { return nil }
func (c *mockConnection) RemoteSpawn(gen.Atom, gen.ProcessOptionsExtra) (gen.PID, error) {
	return gen.PID{}, nil
}
func (c *mockConnection) Terminate(error) {}

// Helper: count items in queue
func countQueue(q lib.QueueMPSC) int {
	count := 0
	item := q.Item()
	for item != nil {
		count++
		item = item.Next()
	}
	return count
}

// Helpers for tests
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

func (m *mockCore) resetSentLinks()      { m.sentLinks = lib.NewQueueMPSC() }
func (m *mockCore) resetSentUnlinks()    { m.sentUnlinks = lib.NewQueueMPSC() }
func (m *mockCore) resetSentMonitors()   { m.sentMonitors = lib.NewQueueMPSC() }
func (m *mockCore) resetSentDemonitors() { m.sentDemonitors = lib.NewQueueMPSC() }
func (m *mockCore) resetSentExits()      { m.sentExits = lib.NewQueueMPSC() }
func (m *mockCore) resetSentDowns()      { m.sentDowns = lib.NewQueueMPSC() }
func (m *mockCore) resetSentEvents()     { m.sentEvents = lib.NewQueueMPSC() }

func (m *mockCore) getFirstSentLink() (linkRequest, bool) {
	item := m.sentLinks.Item()
	if item == nil {
		return linkRequest{}, false
	}
	return item.Value().(linkRequest), true
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

func (m *mockCore) getFirstSentMonitor() (monitorRequest, bool) {
	item := m.sentMonitors.Item()
	if item == nil {
		return monitorRequest{}, false
	}
	return item.Value().(monitorRequest), true
}

func (m *mockCore) getAllSentExits() []exitRequest {
	var exits []exitRequest
	item := m.sentExits.Item()
	for item != nil {
		exits = append(exits, item.Value().(exitRequest))
		item = item.Next()
	}
	return exits
}

func (m *mockCore) getAllSentDowns() []downRequest {
	var downs []downRequest
	item := m.sentDowns.Item()
	for item != nil {
		downs = append(downs, item.Value().(downRequest))
		item = item.Next()
	}
	return downs
}

func (m *mockCore) resetSentEventStarts() { m.sentEventStarts = lib.NewQueueMPSC() }
func (m *mockCore) resetSentEventStops()  { m.sentEventStops = lib.NewQueueMPSC() }
func (m *mockCore) resetSentEventLinks()  { m.sentEventLinks = lib.NewQueueMPSC() }
