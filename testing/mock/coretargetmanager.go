package mock

import (
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// CoreTargetManager is a standalone gen.CoreTargetManager mock. Every method has an
// On<Method> override; the route send/exit/event methods record an ingress check
// record (mirroring testing/stage's recordBridge mapping), the rest return safe
// defaults.
type CoreTargetManager struct {
	recorder
	log *Log
	ov  coreTargetManagerOverrides
}

type coreTargetManagerOverrides struct {
	name                   func() gen.Atom
	pid                    func() gen.PID
	logFn                  func() gen.Log
	makeRef                func() gen.Ref
	routeSendPID           func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error
	routeSendExitMessages  func(from gen.PID, to []gen.PID, message any) error
	routeSendEventMessages func(from gen.PID, to []gen.PID, options gen.MessageOptions, message gen.MessageEvent) error
	getConnection          func(node gen.Atom) (gen.Connection, error)
}

var _ gen.CoreTargetManager = (*CoreTargetManager)(nil)

// NewCoreTargetManager returns a dumb gen.CoreTargetManager mock (no recording; use
// NewCoreTargetManagerT for Should*).
func NewCoreTargetManager() *CoreTargetManager { return newCoreTargetManager(recorder{}) }

// NewCoreTargetManagerT returns a gen.CoreTargetManager mock that records the route
// send/exit/event operations and asserts through t.
func NewCoreTargetManagerT(t check.T) *CoreTargetManager {
	return newCoreTargetManager(newRecorder(t))
}

func newCoreTargetManager(r recorder) *CoreTargetManager {
	return &CoreTargetManager{recorder: r, log: newLog(r)}
}

// On<Method> overrides

func (c *CoreTargetManager) OnName(fn func() gen.Atom)   { c.ov.name = fn }
func (c *CoreTargetManager) OnPID(fn func() gen.PID)     { c.ov.pid = fn }
func (c *CoreTargetManager) OnLog(fn func() gen.Log)     { c.ov.logFn = fn }
func (c *CoreTargetManager) OnMakeRef(fn func() gen.Ref) { c.ov.makeRef = fn }
func (c *CoreTargetManager) OnRouteSendPID(fn func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error) {
	c.ov.routeSendPID = fn
}
func (c *CoreTargetManager) OnRouteSendExitMessages(fn func(from gen.PID, to []gen.PID, message any) error) {
	c.ov.routeSendExitMessages = fn
}
func (c *CoreTargetManager) OnRouteSendEventMessages(fn func(from gen.PID, to []gen.PID, options gen.MessageOptions, message gen.MessageEvent) error) {
	c.ov.routeSendEventMessages = fn
}
func (c *CoreTargetManager) OnGetConnection(fn func(node gen.Atom) (gen.Connection, error)) {
	c.ov.getConnection = fn
}

// gen.CoreTargetManager

func (c *CoreTargetManager) Name() gen.Atom {
	if c.ov.name != nil {
		return c.ov.name()
	}
	return mockNode
}

func (c *CoreTargetManager) PID() gen.PID {
	if c.ov.pid != nil {
		return c.ov.pid()
	}
	return synthPID(1)
}

func (c *CoreTargetManager) Log() gen.Log {
	if c.ov.logFn != nil {
		return c.ov.logFn()
	}
	return c.log
}

func (c *CoreTargetManager) MakeRef() gen.Ref {
	if c.ov.makeRef != nil {
		return c.ov.makeRef()
	}
	return gen.Ref{}
}

func (c *CoreTargetManager) RouteSendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.routeSendPID != nil {
		err = c.ov.routeSendPID(from, to, options, message)
	}
	switch message.(type) {
	case gen.MessageDownPID, gen.MessageDownProcessID, gen.MessageDownAlias, gen.MessageDownNode, gen.MessageDownEvent:
		c.put(check.Down{To: to, Message: message})
	case gen.MessageEventStart, gen.MessageEventStop:
		c.put(check.Delivered{From: from, To: to, Message: message})
	default:
		c.put(check.Delivered{From: from, To: to, Message: message})
	}
	return err
}

func (c *CoreTargetManager) RouteSendExitMessages(from gen.PID, to []gen.PID, message any) error {
	var err error
	if c.ov.routeSendExitMessages != nil {
		err = c.ov.routeSendExitMessages(from, to, message)
	}
	for _, pid := range to {
		c.put(check.Exit{To: pid, Message: message})
	}
	return err
}

func (c *CoreTargetManager) RouteSendEventMessages(from gen.PID, to []gen.PID, options gen.MessageOptions, message gen.MessageEvent) error {
	var err error
	if c.ov.routeSendEventMessages != nil {
		err = c.ov.routeSendEventMessages(from, to, options, message)
	}
	for _, pid := range to {
		c.put(check.Event{To: pid, Event: message.Event, Timestamp: message.Timestamp, Message: message.Message})
	}
	return err
}

func (c *CoreTargetManager) GetConnection(node gen.Atom) (gen.Connection, error) {
	if c.ov.getConnection != nil {
		return c.ov.getConnection(node)
	}
	return nil, nil
}
