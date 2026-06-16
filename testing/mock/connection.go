package mock

import (
	"net"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// Connection is a standalone gen.Connection mock. Every method has an On<Method>
// override; the send/call and link/monitor methods record an egress check record,
// the rest return safe defaults.
type Connection struct {
	recorder
	ov connectionOverrides
}

type connectionOverrides struct {
	node                   func() gen.RemoteNode
	sendPID                func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error
	sendProcessID          func(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error
	sendAlias              func(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error
	sendEvent              func(from gen.PID, options gen.MessageOptions, message gen.MessageEvent) error
	sendExit               func(from gen.PID, to gen.PID, reason error) error
	sendResponse           func(from gen.PID, to gen.PID, options gen.MessageOptions, response any) error
	sendResponseError      func(from gen.PID, to gen.PID, options gen.MessageOptions, err error) error
	sendTerminatePID       func(target gen.PID, reason error) error
	sendTerminateProcessID func(target gen.ProcessID, reason error) error
	sendTerminateAlias     func(target gen.Alias, reason error) error
	sendTerminateEvent     func(target gen.Event, reason error) error
	callPID                func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error
	callProcessID          func(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error
	callAlias              func(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error
	linkPID                func(pid gen.PID, target gen.PID) error
	unlinkPID              func(pid gen.PID, target gen.PID) error
	linkProcessID          func(pid gen.PID, target gen.ProcessID) error
	unlinkProcessID        func(pid gen.PID, target gen.ProcessID) error
	linkAlias              func(pid gen.PID, target gen.Alias) error
	unlinkAlias            func(pid gen.PID, target gen.Alias) error
	linkEvent              func(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error)
	unlinkEvent            func(pid gen.PID, targer gen.Event) error
	monitorPID             func(pid gen.PID, target gen.PID) error
	demonitorPID           func(pid gen.PID, target gen.PID) error
	monitorProcessID       func(pid gen.PID, target gen.ProcessID) error
	demonitorProcessID     func(pid gen.PID, target gen.ProcessID) error
	monitorAlias           func(pid gen.PID, target gen.Alias) error
	demonitorAlias         func(pid gen.PID, target gen.Alias) error
	monitorEvent           func(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error)
	demonitorEvent         func(pid gen.PID, targer gen.Event) error
	remoteSpawn            func(name gen.Atom, options gen.ProcessOptionsExtra) (gen.PID, error)
	join                   func(c net.Conn, id string, dial gen.NetworkDial, tail []byte) error
	terminate              func(reason error)
}

var _ gen.Connection = (*Connection)(nil)

// NewConnection returns a dumb gen.Connection mock (no recording; use NewConnectionT
// for Should*).
func NewConnection() *Connection { return newConnection(recorder{}) }

// NewConnectionT returns a gen.Connection mock that records the send/call and
// link/monitor operations and asserts through t.
func NewConnectionT(t check.T) *Connection { return newConnection(newRecorder(t)) }

func newConnection(r recorder) *Connection { return &Connection{recorder: r} }

// On<Method> overrides

func (c *Connection) OnNode(fn func() gen.RemoteNode) { c.ov.node = fn }
func (c *Connection) OnSendPID(fn func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error) {
	c.ov.sendPID = fn
}
func (c *Connection) OnSendProcessID(fn func(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error) {
	c.ov.sendProcessID = fn
}
func (c *Connection) OnSendAlias(fn func(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error) {
	c.ov.sendAlias = fn
}
func (c *Connection) OnSendEvent(fn func(from gen.PID, options gen.MessageOptions, message gen.MessageEvent) error) {
	c.ov.sendEvent = fn
}
func (c *Connection) OnSendExit(fn func(from gen.PID, to gen.PID, reason error) error) {
	c.ov.sendExit = fn
}
func (c *Connection) OnSendResponse(fn func(from gen.PID, to gen.PID, options gen.MessageOptions, response any) error) {
	c.ov.sendResponse = fn
}
func (c *Connection) OnSendResponseError(fn func(from gen.PID, to gen.PID, options gen.MessageOptions, err error) error) {
	c.ov.sendResponseError = fn
}
func (c *Connection) OnSendTerminatePID(fn func(target gen.PID, reason error) error) {
	c.ov.sendTerminatePID = fn
}
func (c *Connection) OnSendTerminateProcessID(fn func(target gen.ProcessID, reason error) error) {
	c.ov.sendTerminateProcessID = fn
}
func (c *Connection) OnSendTerminateAlias(fn func(target gen.Alias, reason error) error) {
	c.ov.sendTerminateAlias = fn
}
func (c *Connection) OnSendTerminateEvent(fn func(target gen.Event, reason error) error) {
	c.ov.sendTerminateEvent = fn
}
func (c *Connection) OnCallPID(fn func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error) {
	c.ov.callPID = fn
}
func (c *Connection) OnCallProcessID(fn func(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error) {
	c.ov.callProcessID = fn
}
func (c *Connection) OnCallAlias(fn func(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error) {
	c.ov.callAlias = fn
}
func (c *Connection) OnLinkPID(fn func(pid gen.PID, target gen.PID) error)   { c.ov.linkPID = fn }
func (c *Connection) OnUnlinkPID(fn func(pid gen.PID, target gen.PID) error) { c.ov.unlinkPID = fn }
func (c *Connection) OnLinkProcessID(fn func(pid gen.PID, target gen.ProcessID) error) {
	c.ov.linkProcessID = fn
}
func (c *Connection) OnUnlinkProcessID(fn func(pid gen.PID, target gen.ProcessID) error) {
	c.ov.unlinkProcessID = fn
}
func (c *Connection) OnLinkAlias(fn func(pid gen.PID, target gen.Alias) error) {
	c.ov.linkAlias = fn
}
func (c *Connection) OnUnlinkAlias(fn func(pid gen.PID, target gen.Alias) error) {
	c.ov.unlinkAlias = fn
}
func (c *Connection) OnLinkEvent(fn func(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error)) {
	c.ov.linkEvent = fn
}
func (c *Connection) OnUnlinkEvent(fn func(pid gen.PID, targer gen.Event) error) {
	c.ov.unlinkEvent = fn
}
func (c *Connection) OnMonitorPID(fn func(pid gen.PID, target gen.PID) error) {
	c.ov.monitorPID = fn
}
func (c *Connection) OnDemonitorPID(fn func(pid gen.PID, target gen.PID) error) {
	c.ov.demonitorPID = fn
}
func (c *Connection) OnMonitorProcessID(fn func(pid gen.PID, target gen.ProcessID) error) {
	c.ov.monitorProcessID = fn
}
func (c *Connection) OnDemonitorProcessID(fn func(pid gen.PID, target gen.ProcessID) error) {
	c.ov.demonitorProcessID = fn
}
func (c *Connection) OnMonitorAlias(fn func(pid gen.PID, target gen.Alias) error) {
	c.ov.monitorAlias = fn
}
func (c *Connection) OnDemonitorAlias(fn func(pid gen.PID, target gen.Alias) error) {
	c.ov.demonitorAlias = fn
}
func (c *Connection) OnMonitorEvent(fn func(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error)) {
	c.ov.monitorEvent = fn
}
func (c *Connection) OnDemonitorEvent(fn func(pid gen.PID, targer gen.Event) error) {
	c.ov.demonitorEvent = fn
}
func (c *Connection) OnRemoteSpawn(fn func(name gen.Atom, options gen.ProcessOptionsExtra) (gen.PID, error)) {
	c.ov.remoteSpawn = fn
}
func (c *Connection) OnJoin(fn func(c net.Conn, id string, dial gen.NetworkDial, tail []byte) error) {
	c.ov.join = fn
}
func (c *Connection) OnTerminate(fn func(reason error)) { c.ov.terminate = fn }

// gen.Connection

func (c *Connection) Node() gen.RemoteNode {
	if c.ov.node != nil {
		return c.ov.node()
	}
	return nil
}

func (c *Connection) SendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.sendPID != nil {
		err = c.ov.sendPID(from, to, options, message)
	}
	c.put(check.Send{From: from, To: to, Message: message, Options: options})
	return err
}

func (c *Connection) SendProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.sendProcessID != nil {
		err = c.ov.sendProcessID(from, to, options, message)
	}
	c.put(check.Send{From: from, To: to, Message: message, Options: options})
	return err
}

func (c *Connection) SendAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.sendAlias != nil {
		err = c.ov.sendAlias(from, to, options, message)
	}
	c.put(check.Send{From: from, To: to, Message: message, Options: options})
	return err
}

func (c *Connection) SendEvent(from gen.PID, options gen.MessageOptions, message gen.MessageEvent) error {
	var err error
	if c.ov.sendEvent != nil {
		err = c.ov.sendEvent(from, options, message)
	}
	c.put(check.SendEvent{From: from, Name: message.Event.Name, Message: message.Message, Options: options})
	return err
}

func (c *Connection) SendExit(from gen.PID, to gen.PID, reason error) error {
	var err error
	if c.ov.sendExit != nil {
		err = c.ov.sendExit(from, to, reason)
	}
	c.put(check.SendExit{From: from, To: to, Reason: reason})
	return err
}

func (c *Connection) SendResponse(from gen.PID, to gen.PID, options gen.MessageOptions, response any) error {
	var err error
	if c.ov.sendResponse != nil {
		err = c.ov.sendResponse(from, to, options, response)
	}
	c.put(check.SendResponse{From: from, To: to, Message: response, Options: options})
	return err
}

func (c *Connection) SendResponseError(from gen.PID, to gen.PID, options gen.MessageOptions, err error) error {
	var rerr error
	if c.ov.sendResponseError != nil {
		rerr = c.ov.sendResponseError(from, to, options, err)
	}
	c.put(check.SendResponse{From: from, To: to, Message: err, Options: options})
	return rerr
}

func (c *Connection) SendTerminatePID(target gen.PID, reason error) error {
	if c.ov.sendTerminatePID != nil {
		return c.ov.sendTerminatePID(target, reason)
	}
	return nil
}

func (c *Connection) SendTerminateProcessID(target gen.ProcessID, reason error) error {
	if c.ov.sendTerminateProcessID != nil {
		return c.ov.sendTerminateProcessID(target, reason)
	}
	return nil
}

func (c *Connection) SendTerminateAlias(target gen.Alias, reason error) error {
	if c.ov.sendTerminateAlias != nil {
		return c.ov.sendTerminateAlias(target, reason)
	}
	return nil
}

func (c *Connection) SendTerminateEvent(target gen.Event, reason error) error {
	if c.ov.sendTerminateEvent != nil {
		return c.ov.sendTerminateEvent(target, reason)
	}
	return nil
}

func (c *Connection) CallPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.callPID != nil {
		err = c.ov.callPID(from, to, options, message)
	}
	c.put(check.Call{From: from, To: to, Request: message})
	return err
}

func (c *Connection) CallProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.callProcessID != nil {
		err = c.ov.callProcessID(from, to, options, message)
	}
	c.put(check.Call{From: from, To: to, Request: message})
	return err
}

func (c *Connection) CallAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.callAlias != nil {
		err = c.ov.callAlias(from, to, options, message)
	}
	c.put(check.Call{From: from, To: to, Request: message})
	return err
}

func (c *Connection) LinkPID(pid gen.PID, target gen.PID) error {
	var err error
	if c.ov.linkPID != nil {
		err = c.ov.linkPID(pid, target)
	}
	c.put(check.Link{From: pid, Target: target})
	return err
}

func (c *Connection) UnlinkPID(pid gen.PID, target gen.PID) error {
	var err error
	if c.ov.unlinkPID != nil {
		err = c.ov.unlinkPID(pid, target)
	}
	c.put(check.Unlink{From: pid, Target: target})
	return err
}

func (c *Connection) LinkProcessID(pid gen.PID, target gen.ProcessID) error {
	var err error
	if c.ov.linkProcessID != nil {
		err = c.ov.linkProcessID(pid, target)
	}
	c.put(check.Link{From: pid, Target: target})
	return err
}

func (c *Connection) UnlinkProcessID(pid gen.PID, target gen.ProcessID) error {
	var err error
	if c.ov.unlinkProcessID != nil {
		err = c.ov.unlinkProcessID(pid, target)
	}
	c.put(check.Unlink{From: pid, Target: target})
	return err
}

func (c *Connection) LinkAlias(pid gen.PID, target gen.Alias) error {
	var err error
	if c.ov.linkAlias != nil {
		err = c.ov.linkAlias(pid, target)
	}
	c.put(check.Link{From: pid, Target: target})
	return err
}

func (c *Connection) UnlinkAlias(pid gen.PID, target gen.Alias) error {
	var err error
	if c.ov.unlinkAlias != nil {
		err = c.ov.unlinkAlias(pid, target)
	}
	c.put(check.Unlink{From: pid, Target: target})
	return err
}

func (c *Connection) LinkEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {
	var (
		events []gen.MessageEvent
		err    error
	)
	if c.ov.linkEvent != nil {
		events, err = c.ov.linkEvent(pid, target)
	}
	c.put(check.Link{From: pid, Target: target})
	return events, err
}

func (c *Connection) UnlinkEvent(pid gen.PID, targer gen.Event) error {
	var err error
	if c.ov.unlinkEvent != nil {
		err = c.ov.unlinkEvent(pid, targer)
	}
	c.put(check.Unlink{From: pid, Target: targer})
	return err
}

func (c *Connection) MonitorPID(pid gen.PID, target gen.PID) error {
	var err error
	if c.ov.monitorPID != nil {
		err = c.ov.monitorPID(pid, target)
	}
	c.put(check.Monitor{From: pid, Target: target})
	return err
}

func (c *Connection) DemonitorPID(pid gen.PID, target gen.PID) error {
	var err error
	if c.ov.demonitorPID != nil {
		err = c.ov.demonitorPID(pid, target)
	}
	c.put(check.Demonitor{From: pid, Target: target})
	return err
}

func (c *Connection) MonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	var err error
	if c.ov.monitorProcessID != nil {
		err = c.ov.monitorProcessID(pid, target)
	}
	c.put(check.Monitor{From: pid, Target: target})
	return err
}

func (c *Connection) DemonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	var err error
	if c.ov.demonitorProcessID != nil {
		err = c.ov.demonitorProcessID(pid, target)
	}
	c.put(check.Demonitor{From: pid, Target: target})
	return err
}

func (c *Connection) MonitorAlias(pid gen.PID, target gen.Alias) error {
	var err error
	if c.ov.monitorAlias != nil {
		err = c.ov.monitorAlias(pid, target)
	}
	c.put(check.Monitor{From: pid, Target: target})
	return err
}

func (c *Connection) DemonitorAlias(pid gen.PID, target gen.Alias) error {
	var err error
	if c.ov.demonitorAlias != nil {
		err = c.ov.demonitorAlias(pid, target)
	}
	c.put(check.Demonitor{From: pid, Target: target})
	return err
}

func (c *Connection) MonitorEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {
	var (
		events []gen.MessageEvent
		err    error
	)
	if c.ov.monitorEvent != nil {
		events, err = c.ov.monitorEvent(pid, target)
	}
	c.put(check.Monitor{From: pid, Target: target})
	return events, err
}

func (c *Connection) DemonitorEvent(pid gen.PID, targer gen.Event) error {
	var err error
	if c.ov.demonitorEvent != nil {
		err = c.ov.demonitorEvent(pid, targer)
	}
	c.put(check.Demonitor{From: pid, Target: targer})
	return err
}

func (c *Connection) RemoteSpawn(name gen.Atom, options gen.ProcessOptionsExtra) (gen.PID, error) {
	if c.ov.remoteSpawn != nil {
		return c.ov.remoteSpawn(name, options)
	}
	return gen.PID{}, nil
}

func (c *Connection) Join(conn net.Conn, id string, dial gen.NetworkDial, tail []byte) error {
	if c.ov.join != nil {
		return c.ov.join(conn, id, dial, tail)
	}
	return nil
}

func (c *Connection) Terminate(reason error) {
	if c.ov.terminate != nil {
		c.ov.terminate(reason)
	}
}
