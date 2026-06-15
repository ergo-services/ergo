package mock

import (
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// Core is a standalone gen.Core mock. Every method has an On<Method> override; the
// route send/call/exit/response and link/monitor methods record an ingress/wire
// check record (mirroring testing/stage's recordCore mapping), the rest return safe
// defaults.
type Core struct {
	recorder
	ov coreOverrides
}

type coreOverrides struct {
	routeSendPID            func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error
	routeSendProcessID      func(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error
	routeSendAlias          func(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error
	routeSendEvent          func(from gen.PID, token gen.Ref, options gen.MessageOptions, message gen.MessageEvent) error
	routeSendExit           func(from gen.PID, to gen.PID, reason error) error
	routeSendResponse       func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error
	routeSendResponseError  func(from gen.PID, to gen.PID, options gen.MessageOptions, err error) error
	routeCallPID            func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error
	routeCallProcessID      func(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error
	routeCallAlias          func(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error
	routeLinkPID            func(pid gen.PID, target gen.PID) error
	routeUnlinkPID          func(pid gen.PID, target gen.PID) error
	routeLinkProcessID      func(pid gen.PID, target gen.ProcessID) error
	routeUnlinkProcessID    func(pid gen.PID, target gen.ProcessID) error
	routeLinkAlias          func(pid gen.PID, target gen.Alias) error
	routeUnlinkAlias        func(pid gen.PID, target gen.Alias) error
	routeLinkEvent          func(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error)
	routeUnlinkEvent        func(pid gen.PID, target gen.Event) error
	routeMonitorPID         func(pid gen.PID, target gen.PID) error
	routeDemonitorPID       func(pid gen.PID, target gen.PID) error
	routeMonitorProcessID   func(pid gen.PID, target gen.ProcessID) error
	routeDemonitorProcessID func(pid gen.PID, target gen.ProcessID) error
	routeMonitorAlias       func(pid gen.PID, target gen.Alias) error
	routeDemonitorAlias     func(pid gen.PID, target gen.Alias) error
	routeMonitorEvent       func(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error)
	routeDemonitorEvent     func(pid gen.PID, target gen.Event) error
	routeTerminatePID       func(target gen.PID, reason error) error
	routeTerminateProcessID func(target gen.ProcessID, reason error) error
	routeTerminateEvent     func(target gen.Event, reason error) error
	routeTerminateAlias     func(terget gen.Alias, reason error) error
	routeSpawn              func(node gen.Atom, name gen.Atom, options gen.ProcessOptionsExtra, source gen.Atom) (gen.PID, error)
	routeApplicationStart   func(name gen.Atom, mode gen.ApplicationMode, options gen.ApplicationOptionsExtra, source gen.Atom) error
	routeApplicationInfo    func(name gen.Atom) (gen.ApplicationInfo, error)
	routeNodeDown           func(node gen.Atom, reason error)
	makeRef                 func() gen.Ref
	makeRefWithDeadline     func(deadline int64) (gen.Ref, error)
	name                    func() gen.Atom
	creation                func() int64
	pid                     func() gen.PID
	logLevel                func() gen.LogLevel
	security                func() gen.SecurityOptions
	envList                 func() map[gen.Env]any
}

var _ gen.Core = (*Core)(nil)

// NewCore returns a dumb gen.Core mock (no recording; use NewCoreT for Should*).
func NewCore() *Core { return newCore(recorder{}) }

// NewCoreT returns a gen.Core mock that records the route send/call/exit/response and
// link/monitor operations and asserts through t.
func NewCoreT(t check.T) *Core { return newCore(newRecorder(t)) }

func newCore(r recorder) *Core { return &Core{recorder: r} }

// On<Method> overrides

func (c *Core) OnRouteSendPID(fn func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error) {
	c.ov.routeSendPID = fn
}
func (c *Core) OnRouteSendProcessID(fn func(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error) {
	c.ov.routeSendProcessID = fn
}
func (c *Core) OnRouteSendAlias(fn func(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error) {
	c.ov.routeSendAlias = fn
}
func (c *Core) OnRouteSendEvent(fn func(from gen.PID, token gen.Ref, options gen.MessageOptions, message gen.MessageEvent) error) {
	c.ov.routeSendEvent = fn
}
func (c *Core) OnRouteSendExit(fn func(from gen.PID, to gen.PID, reason error) error) {
	c.ov.routeSendExit = fn
}
func (c *Core) OnRouteSendResponse(fn func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error) {
	c.ov.routeSendResponse = fn
}
func (c *Core) OnRouteSendResponseError(fn func(from gen.PID, to gen.PID, options gen.MessageOptions, err error) error) {
	c.ov.routeSendResponseError = fn
}
func (c *Core) OnRouteCallPID(fn func(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error) {
	c.ov.routeCallPID = fn
}
func (c *Core) OnRouteCallProcessID(fn func(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error) {
	c.ov.routeCallProcessID = fn
}
func (c *Core) OnRouteCallAlias(fn func(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error) {
	c.ov.routeCallAlias = fn
}
func (c *Core) OnRouteLinkPID(fn func(pid gen.PID, target gen.PID) error)   { c.ov.routeLinkPID = fn }
func (c *Core) OnRouteUnlinkPID(fn func(pid gen.PID, target gen.PID) error) { c.ov.routeUnlinkPID = fn }
func (c *Core) OnRouteLinkProcessID(fn func(pid gen.PID, target gen.ProcessID) error) {
	c.ov.routeLinkProcessID = fn
}
func (c *Core) OnRouteUnlinkProcessID(fn func(pid gen.PID, target gen.ProcessID) error) {
	c.ov.routeUnlinkProcessID = fn
}
func (c *Core) OnRouteLinkAlias(fn func(pid gen.PID, target gen.Alias) error) {
	c.ov.routeLinkAlias = fn
}
func (c *Core) OnRouteUnlinkAlias(fn func(pid gen.PID, target gen.Alias) error) {
	c.ov.routeUnlinkAlias = fn
}
func (c *Core) OnRouteLinkEvent(fn func(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error)) {
	c.ov.routeLinkEvent = fn
}
func (c *Core) OnRouteUnlinkEvent(fn func(pid gen.PID, target gen.Event) error) {
	c.ov.routeUnlinkEvent = fn
}
func (c *Core) OnRouteMonitorPID(fn func(pid gen.PID, target gen.PID) error) {
	c.ov.routeMonitorPID = fn
}
func (c *Core) OnRouteDemonitorPID(fn func(pid gen.PID, target gen.PID) error) {
	c.ov.routeDemonitorPID = fn
}
func (c *Core) OnRouteMonitorProcessID(fn func(pid gen.PID, target gen.ProcessID) error) {
	c.ov.routeMonitorProcessID = fn
}
func (c *Core) OnRouteDemonitorProcessID(fn func(pid gen.PID, target gen.ProcessID) error) {
	c.ov.routeDemonitorProcessID = fn
}
func (c *Core) OnRouteMonitorAlias(fn func(pid gen.PID, target gen.Alias) error) {
	c.ov.routeMonitorAlias = fn
}
func (c *Core) OnRouteDemonitorAlias(fn func(pid gen.PID, target gen.Alias) error) {
	c.ov.routeDemonitorAlias = fn
}
func (c *Core) OnRouteMonitorEvent(fn func(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error)) {
	c.ov.routeMonitorEvent = fn
}
func (c *Core) OnRouteDemonitorEvent(fn func(pid gen.PID, target gen.Event) error) {
	c.ov.routeDemonitorEvent = fn
}
func (c *Core) OnRouteTerminatePID(fn func(target gen.PID, reason error) error) {
	c.ov.routeTerminatePID = fn
}
func (c *Core) OnRouteTerminateProcessID(fn func(target gen.ProcessID, reason error) error) {
	c.ov.routeTerminateProcessID = fn
}
func (c *Core) OnRouteTerminateEvent(fn func(target gen.Event, reason error) error) {
	c.ov.routeTerminateEvent = fn
}
func (c *Core) OnRouteTerminateAlias(fn func(terget gen.Alias, reason error) error) {
	c.ov.routeTerminateAlias = fn
}
func (c *Core) OnRouteSpawn(fn func(node gen.Atom, name gen.Atom, options gen.ProcessOptionsExtra, source gen.Atom) (gen.PID, error)) {
	c.ov.routeSpawn = fn
}
func (c *Core) OnRouteApplicationStart(fn func(name gen.Atom, mode gen.ApplicationMode, options gen.ApplicationOptionsExtra, source gen.Atom) error) {
	c.ov.routeApplicationStart = fn
}
func (c *Core) OnRouteApplicationInfo(fn func(name gen.Atom) (gen.ApplicationInfo, error)) {
	c.ov.routeApplicationInfo = fn
}
func (c *Core) OnRouteNodeDown(fn func(node gen.Atom, reason error)) { c.ov.routeNodeDown = fn }
func (c *Core) OnMakeRef(fn func() gen.Ref)                          { c.ov.makeRef = fn }
func (c *Core) OnMakeRefWithDeadline(fn func(deadline int64) (gen.Ref, error)) {
	c.ov.makeRefWithDeadline = fn
}
func (c *Core) OnName(fn func() gen.Atom)                { c.ov.name = fn }
func (c *Core) OnCreation(fn func() int64)               { c.ov.creation = fn }
func (c *Core) OnPID(fn func() gen.PID)                  { c.ov.pid = fn }
func (c *Core) OnLogLevel(fn func() gen.LogLevel)        { c.ov.logLevel = fn }
func (c *Core) OnSecurity(fn func() gen.SecurityOptions) { c.ov.security = fn }
func (c *Core) OnEnvList(fn func() map[gen.Env]any)      { c.ov.envList = fn }

// gen.Core - sending message

func (c *Core) RouteSendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.routeSendPID != nil {
		err = c.ov.routeSendPID(from, to, options, message)
	}
	c.put(check.Delivered{From: from, To: to, Message: message})
	return err
}

func (c *Core) RouteSendProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.routeSendProcessID != nil {
		err = c.ov.routeSendProcessID(from, to, options, message)
	}
	c.put(check.Delivered{From: from, To: to, Message: message})
	return err
}

func (c *Core) RouteSendAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.routeSendAlias != nil {
		err = c.ov.routeSendAlias(from, to, options, message)
	}
	c.put(check.Delivered{From: from, To: to, Message: message})
	return err
}

func (c *Core) RouteSendEvent(from gen.PID, token gen.Ref, options gen.MessageOptions, message gen.MessageEvent) error {
	if c.ov.routeSendEvent != nil {
		return c.ov.routeSendEvent(from, token, options, message)
	}
	return nil
}

func (c *Core) RouteSendExit(from gen.PID, to gen.PID, reason error) error {
	var err error
	if c.ov.routeSendExit != nil {
		err = c.ov.routeSendExit(from, to, reason)
	}
	c.put(check.Exit{To: to, Message: gen.MessageExitPID{PID: from, Reason: reason}})
	return err
}

func (c *Core) RouteSendResponse(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.routeSendResponse != nil {
		err = c.ov.routeSendResponse(from, to, options, message)
	}
	c.put(check.SendResponse{From: from, To: to, Message: message})
	return err
}

func (c *Core) RouteSendResponseError(from gen.PID, to gen.PID, options gen.MessageOptions, err error) error {
	var rerr error
	if c.ov.routeSendResponseError != nil {
		rerr = c.ov.routeSendResponseError(from, to, options, err)
	}
	c.put(check.SendResponse{From: from, To: to, Message: err})
	return rerr
}

// gen.Core - call requests

func (c *Core) RouteCallPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.routeCallPID != nil {
		err = c.ov.routeCallPID(from, to, options, message)
	}
	c.put(check.Delivered{From: from, To: to, Message: message})
	return err
}

func (c *Core) RouteCallProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.routeCallProcessID != nil {
		err = c.ov.routeCallProcessID(from, to, options, message)
	}
	c.put(check.Delivered{From: from, To: to, Message: message})
	return err
}

func (c *Core) RouteCallAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	var err error
	if c.ov.routeCallAlias != nil {
		err = c.ov.routeCallAlias(from, to, options, message)
	}
	c.put(check.Delivered{From: from, To: to, Message: message})
	return err
}

// gen.Core - linking requests

func (c *Core) RouteLinkPID(pid gen.PID, target gen.PID) error {
	var err error
	if c.ov.routeLinkPID != nil {
		err = c.ov.routeLinkPID(pid, target)
	}
	c.put(check.WireLink{From: pid, Target: target})
	return err
}

func (c *Core) RouteUnlinkPID(pid gen.PID, target gen.PID) error {
	var err error
	if c.ov.routeUnlinkPID != nil {
		err = c.ov.routeUnlinkPID(pid, target)
	}
	c.put(check.WireUnlink{From: pid, Target: target})
	return err
}

func (c *Core) RouteLinkProcessID(pid gen.PID, target gen.ProcessID) error {
	var err error
	if c.ov.routeLinkProcessID != nil {
		err = c.ov.routeLinkProcessID(pid, target)
	}
	c.put(check.WireLink{From: pid, Target: target})
	return err
}

func (c *Core) RouteUnlinkProcessID(pid gen.PID, target gen.ProcessID) error {
	var err error
	if c.ov.routeUnlinkProcessID != nil {
		err = c.ov.routeUnlinkProcessID(pid, target)
	}
	c.put(check.WireUnlink{From: pid, Target: target})
	return err
}

func (c *Core) RouteLinkAlias(pid gen.PID, target gen.Alias) error {
	var err error
	if c.ov.routeLinkAlias != nil {
		err = c.ov.routeLinkAlias(pid, target)
	}
	c.put(check.WireLink{From: pid, Target: target})
	return err
}

func (c *Core) RouteUnlinkAlias(pid gen.PID, target gen.Alias) error {
	var err error
	if c.ov.routeUnlinkAlias != nil {
		err = c.ov.routeUnlinkAlias(pid, target)
	}
	c.put(check.WireUnlink{From: pid, Target: target})
	return err
}

func (c *Core) RouteLinkEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {
	var (
		events []gen.MessageEvent
		err    error
	)
	if c.ov.routeLinkEvent != nil {
		events, err = c.ov.routeLinkEvent(pid, target)
	}
	c.put(check.WireLink{From: pid, Target: target})
	return events, err
}

func (c *Core) RouteUnlinkEvent(pid gen.PID, target gen.Event) error {
	var err error
	if c.ov.routeUnlinkEvent != nil {
		err = c.ov.routeUnlinkEvent(pid, target)
	}
	c.put(check.WireUnlink{From: pid, Target: target})
	return err
}

// gen.Core - monitoring requests

func (c *Core) RouteMonitorPID(pid gen.PID, target gen.PID) error {
	var err error
	if c.ov.routeMonitorPID != nil {
		err = c.ov.routeMonitorPID(pid, target)
	}
	c.put(check.WireMonitor{From: pid, Target: target})
	return err
}

func (c *Core) RouteDemonitorPID(pid gen.PID, target gen.PID) error {
	var err error
	if c.ov.routeDemonitorPID != nil {
		err = c.ov.routeDemonitorPID(pid, target)
	}
	c.put(check.WireDemonitor{From: pid, Target: target})
	return err
}

func (c *Core) RouteMonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	var err error
	if c.ov.routeMonitorProcessID != nil {
		err = c.ov.routeMonitorProcessID(pid, target)
	}
	c.put(check.WireMonitor{From: pid, Target: target})
	return err
}

func (c *Core) RouteDemonitorProcessID(pid gen.PID, target gen.ProcessID) error {
	var err error
	if c.ov.routeDemonitorProcessID != nil {
		err = c.ov.routeDemonitorProcessID(pid, target)
	}
	c.put(check.WireDemonitor{From: pid, Target: target})
	return err
}

func (c *Core) RouteMonitorAlias(pid gen.PID, target gen.Alias) error {
	var err error
	if c.ov.routeMonitorAlias != nil {
		err = c.ov.routeMonitorAlias(pid, target)
	}
	c.put(check.WireMonitor{From: pid, Target: target})
	return err
}

func (c *Core) RouteDemonitorAlias(pid gen.PID, target gen.Alias) error {
	var err error
	if c.ov.routeDemonitorAlias != nil {
		err = c.ov.routeDemonitorAlias(pid, target)
	}
	c.put(check.WireDemonitor{From: pid, Target: target})
	return err
}

func (c *Core) RouteMonitorEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {
	var (
		events []gen.MessageEvent
		err    error
	)
	if c.ov.routeMonitorEvent != nil {
		events, err = c.ov.routeMonitorEvent(pid, target)
	}
	c.put(check.WireMonitor{From: pid, Target: target})
	return events, err
}

func (c *Core) RouteDemonitorEvent(pid gen.PID, target gen.Event) error {
	var err error
	if c.ov.routeDemonitorEvent != nil {
		err = c.ov.routeDemonitorEvent(pid, target)
	}
	c.put(check.WireDemonitor{From: pid, Target: target})
	return err
}

// gen.Core - target termination

func (c *Core) RouteTerminatePID(target gen.PID, reason error) error {
	if c.ov.routeTerminatePID != nil {
		return c.ov.routeTerminatePID(target, reason)
	}
	return nil
}

func (c *Core) RouteTerminateProcessID(target gen.ProcessID, reason error) error {
	if c.ov.routeTerminateProcessID != nil {
		return c.ov.routeTerminateProcessID(target, reason)
	}
	return nil
}

func (c *Core) RouteTerminateEvent(target gen.Event, reason error) error {
	if c.ov.routeTerminateEvent != nil {
		return c.ov.routeTerminateEvent(target, reason)
	}
	return nil
}

func (c *Core) RouteTerminateAlias(terget gen.Alias, reason error) error {
	if c.ov.routeTerminateAlias != nil {
		return c.ov.routeTerminateAlias(terget, reason)
	}
	return nil
}

func (c *Core) RouteSpawn(node gen.Atom, name gen.Atom, options gen.ProcessOptionsExtra, source gen.Atom) (gen.PID, error) {
	if c.ov.routeSpawn != nil {
		return c.ov.routeSpawn(node, name, options, source)
	}
	return gen.PID{}, nil
}

func (c *Core) RouteApplicationStart(name gen.Atom, mode gen.ApplicationMode, options gen.ApplicationOptionsExtra, source gen.Atom) error {
	if c.ov.routeApplicationStart != nil {
		return c.ov.routeApplicationStart(name, mode, options, source)
	}
	return nil
}

func (c *Core) RouteApplicationInfo(name gen.Atom) (gen.ApplicationInfo, error) {
	if c.ov.routeApplicationInfo != nil {
		return c.ov.routeApplicationInfo(name)
	}
	return gen.ApplicationInfo{}, nil
}

func (c *Core) RouteNodeDown(node gen.Atom, reason error) {
	if c.ov.routeNodeDown != nil {
		c.ov.routeNodeDown(node, reason)
	}
}

func (c *Core) MakeRef() gen.Ref {
	if c.ov.makeRef != nil {
		return c.ov.makeRef()
	}
	return gen.Ref{}
}

func (c *Core) MakeRefWithDeadline(deadline int64) (gen.Ref, error) {
	if c.ov.makeRefWithDeadline != nil {
		return c.ov.makeRefWithDeadline(deadline)
	}
	return gen.Ref{}, nil
}

func (c *Core) Name() gen.Atom {
	if c.ov.name != nil {
		return c.ov.name()
	}
	return ""
}

func (c *Core) Creation() int64 {
	if c.ov.creation != nil {
		return c.ov.creation()
	}
	return 0
}

func (c *Core) PID() gen.PID {
	if c.ov.pid != nil {
		return c.ov.pid()
	}
	return gen.PID{}
}

func (c *Core) LogLevel() gen.LogLevel {
	if c.ov.logLevel != nil {
		return c.ov.logLevel()
	}
	return gen.LogLevel(0)
}

func (c *Core) Security() gen.SecurityOptions {
	if c.ov.security != nil {
		return c.ov.security()
	}
	return gen.SecurityOptions{}
}

func (c *Core) EnvList() map[gen.Env]any {
	if c.ov.envList != nil {
		return c.ov.envList()
	}
	return nil
}
