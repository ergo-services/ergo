package act

import (
	"fmt"
	"reflect"
	"runtime"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// RouteDiscard is the sentinel returned from RouteMessage and RouteCall to
// drop the incoming message. Equal to the empty gen.Atom.
const RouteDiscard gen.Atom = ""

// Route is a named routing destination configured at Router initialization.
type Route struct {
	Name    gen.Atom
	Factory gen.ProcessFactory
	Args    []any
}

// RouterOptions configures a Router process.
type RouterOptions struct {
	Routes      []Route
	MailboxSize int64
}

// RouterBehavior is the interface a Router implementation must satisfy.
//
// Routing callbacks (RouteMessage, RouteCall) receive traffic delivered with
// MessagePriorityNormal and return the target name. Admin callbacks
// (HandleMessage, HandleCall) receive traffic delivered with
// MessagePriorityHigh or Max and are not routed.
type RouterBehavior interface {
	gen.ProcessBehavior

	// Init is invoked on a Router spawn for initialization.
	Init(args ...any) (RouterOptions, error)

	// Terminate is invoked on Router termination.
	Terminate(reason error)

	// RouteMessage decides destination for Send arriving with
	// MessagePriorityNormal. Returns the target route Name (or a name of a
	// process registered on this node, used as a fallback). Returns
	// RouteDiscard to drop the message; discarded counter is incremented and
	// the sender receives no notification.
	RouteMessage(from gen.PID, message any) gen.Atom

	// RouteCall decides destination for Call arriving with
	// MessagePriorityNormal. Returns the target route Name. Returns
	// RouteDiscard to respond gen.ErrDiscarded to the caller.
	RouteCall(from gen.PID, ref gen.Ref, request any) gen.Atom

	// HandleMessage is invoked for Send arriving with MessagePriorityHigh or
	// Max, and for self-delivered MessageRouteFailed sentinels when a routed
	// async message could not be forwarded.
	HandleMessage(from gen.PID, message any) error

	// HandleCall is invoked for Call arriving with MessagePriorityHigh or
	// Max. Return non-nil result to respond synchronously, nil to defer the
	// response via SendResponse. Non-nil reason terminates the Router; if
	// reason is gen.TerminateReasonNormal and result is non-nil, the result
	// is delivered to the caller before the Router shuts down.
	HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)

	// HandleEvent is invoked on a subscribed event.
	HandleEvent(message gen.MessageEvent) error

	// HandleInspect is invoked on Inspect requests. Default returns routing
	// statistics; override to add fields.
	HandleInspect(from gen.PID, item ...string) map[string]string
}

// MessageRouteFailed is delivered to HandleMessage when a routed async
// message could not be forwarded. Reason carries the cause:
//   - gen.ErrProcessUnknown: name resolved to nothing (no route, no registry
//     entry, or respawn of an owned route failed).
//   - gen.ErrProcessMailboxFull: target's mailbox is full.
//   - gen.ErrDisabled: target route is disabled or mid-disable.
//   - gen.ErrNoRoute: target route is being removed.
//   - gen.ErrBusy: target route is mid-replace with no current worker.
type MessageRouteFailed struct {
	Name    gen.Atom
	From    gen.PID
	Message any
	Reason  error
}

// RoutePending names the async operation in flight on a route.
// When non-None, all public route-management methods return gen.ErrBusy.
type RoutePending int

const (
	RoutePendingNone    RoutePending = 0
	RoutePendingDisable RoutePending = 1
	RoutePendingReplace RoutePending = 2
	RoutePendingRemove  RoutePending = 3
)

func (p RoutePending) String() string {
	switch p {
	case RoutePendingNone:
		return "none"
	case RoutePendingDisable:
		return "disable"
	case RoutePendingReplace:
		return "replace"
	case RoutePendingRemove:
		return "remove"
	}
	return fmt.Sprintf("pending#%d", int(p))
}

// RouterRouteInfo is a snapshot of a route returned from Routes() and
// Route(). PID is empty when the route is not currently running.
type RouterRouteInfo struct {
	Name     gen.Atom
	PID      gen.PID
	Disabled bool
	Pending  RoutePending
}

type routeEntry struct {
	Route
	pid         gen.PID
	disabled    bool
	pending     RoutePending
	pendingSpec *Route // set only when pending == RoutePendingReplace
}

// Router is the base type embedded in user RouterBehavior implementations.
type Router struct {
	gen.Process

	behavior RouterBehavior
	mailbox  gen.ProcessMailbox

	options RouterOptions

	routes       []*routeEntry
	routesByName map[gen.Atom]*routeEntry

	// pendingExit holds worker PIDs Router stopped tracking before consuming
	// their link-EXIT. handleExit drops matching EXITs silently. Required for
	// the race between cleanupProcess removing the worker from the node
	// registry and the link-EXIT landing in our Urgent queue. PIDs not in
	// this set follow normal link semantics: Router terminates.
	pendingExit map[gen.PID]struct{}

	forwarded uint64
	discarded uint64
	failed    uint64
	restarts  uint64
}

// Routes returns snapshots of all routes in initialization order.
func (r *Router) Routes() []RouterRouteInfo {
	out := make([]RouterRouteInfo, 0, len(r.routes))
	for _, re := range r.routes {
		out = append(out, snapshotRoute(re))
	}
	return out
}

// Route returns a snapshot of the named route. ok is false when the name is
// not a known route.
func (r *Router) Route(name gen.Atom) (RouterRouteInfo, bool) {
	re, ok := r.routesByName[name]
	if ok == false {
		return RouterRouteInfo{}, false
	}
	return snapshotRoute(re), true
}

func snapshotRoute(re *routeEntry) RouterRouteInfo {
	return RouterRouteInfo{
		Name:     re.Name,
		PID:      re.pid,
		Disabled: re.disabled,
		Pending:  re.pending,
	}
}

// AddRoute appends a new named route and spawns its worker.
func (r *Router) AddRoute(route Route) error {
	if r.State() != gen.ProcessStateRunning {
		return gen.ErrNotAllowed
	}
	if route.Name == "" {
		return fmt.Errorf("route name can not be empty")
	}
	if route.Factory == nil {
		return fmt.Errorf("route %q has nil Factory", route.Name)
	}
	if _, dup := r.routesByName[route.Name]; dup {
		return ErrRouteDuplicate
	}
	re := &routeEntry{Route: route}
	pid, err := r.spawnRoute(re)
	if err != nil {
		return err
	}
	re.pid = pid
	r.routes = append(r.routes, re)
	r.routesByName[route.Name] = re
	return nil
}

// RemoveRoute terminates the route's worker (if running) and removes the
// route from the Router. Idempotent for unknown names.
func (r *Router) RemoveRoute(name gen.Atom) error {
	if r.State() != gen.ProcessStateRunning {
		return gen.ErrNotAllowed
	}
	re, ok := r.routesByName[name]
	if ok == false {
		return nil
	}
	if re.pending != RoutePendingNone {
		return gen.ErrBusy
	}
	var empty gen.PID
	if re.pid == empty {
		r.dropRoute(re)
		return nil
	}
	err := r.SendExit(re.pid, gen.TerminateReasonShutdown)
	if err == nil {
		re.pending = RoutePendingRemove
		return nil
	}
	if err == gen.ErrProcessUnknown || err == gen.ErrProcessTerminated {
		r.pendingExit[re.pid] = struct{}{}
		re.pid = empty
		r.dropRoute(re)
		return nil
	}
	return err
}

// DisableRoute terminates the route's worker (if running) and marks the
// route as admin-disabled. Idempotent for already-disabled routes.
func (r *Router) DisableRoute(name gen.Atom) error {
	if r.State() != gen.ProcessStateRunning {
		return gen.ErrNotAllowed
	}
	re, ok := r.routesByName[name]
	if ok == false {
		return gen.ErrNoRoute
	}
	if re.pending != RoutePendingNone {
		return gen.ErrBusy
	}
	if re.disabled {
		return nil
	}
	var empty gen.PID
	if re.pid == empty {
		re.disabled = true
		return nil
	}
	err := r.SendExit(re.pid, gen.TerminateReasonShutdown)
	if err == nil {
		re.pending = RoutePendingDisable
		return nil
	}
	if err == gen.ErrProcessUnknown || err == gen.ErrProcessTerminated {
		r.pendingExit[re.pid] = struct{}{}
		re.pid = empty
		re.disabled = true
		return nil
	}
	return err
}

// EnableRoute clears the admin-disabled flag and spawns a fresh worker.
// Idempotent for already-enabled routes.
func (r *Router) EnableRoute(name gen.Atom) error {
	if r.State() != gen.ProcessStateRunning {
		return gen.ErrNotAllowed
	}
	re, ok := r.routesByName[name]
	if ok == false {
		return gen.ErrNoRoute
	}
	if re.pending != RoutePendingNone {
		return gen.ErrBusy
	}
	if re.disabled == false {
		return nil
	}
	re.disabled = false
	var empty gen.PID
	if re.pid == empty {
		pid, err := r.spawnRoute(re)
		if err != nil {
			return err
		}
		re.pid = pid
	}
	return nil
}

// ReplaceRoute swaps the spec for an existing route and (re)spawns the
// worker with the new spec. The route Name in the provided spec must match
// `name` or be empty.
func (r *Router) ReplaceRoute(name gen.Atom, route Route) error {
	if r.State() != gen.ProcessStateRunning {
		return gen.ErrNotAllowed
	}
	if route.Factory == nil {
		return fmt.Errorf("route %q has nil Factory", name)
	}
	if route.Name != "" && route.Name != name {
		return fmt.Errorf("ReplaceRoute name mismatch: spec Name %q vs key %q", route.Name, name)
	}
	re, ok := r.routesByName[name]
	if ok == false {
		return gen.ErrNoRoute
	}
	if re.pending != RoutePendingNone {
		return gen.ErrBusy
	}
	route.Name = name

	var empty gen.PID
	if re.pid == empty {
		re.Route = route
		if re.disabled {
			return nil
		}
		pid, err := r.spawnRoute(re)
		if err != nil {
			return err
		}
		re.pid = pid
		return nil
	}

	err := r.SendExit(re.pid, gen.TerminateReasonShutdown)
	if err == nil {
		spec := route
		re.pending = RoutePendingReplace
		re.pendingSpec = &spec
		return nil
	}
	if err == gen.ErrProcessUnknown || err == gen.ErrProcessTerminated {
		r.pendingExit[re.pid] = struct{}{}
		re.pid = empty
		re.Route = route
		if re.disabled {
			return nil
		}
		pid, sperr := r.spawnRoute(re)
		if sperr != nil {
			return sperr
		}
		re.pid = pid
		return nil
	}
	return err
}

// RespawnRoute spawns a worker for a route that is currently not running.
// Returns ErrRouteRunning if the worker is alive, gen.ErrDisabled if the
// route is admin-disabled, gen.ErrBusy if a pending operation is in
// flight.
func (r *Router) RespawnRoute(name gen.Atom) error {
	if r.State() != gen.ProcessStateRunning {
		return gen.ErrNotAllowed
	}
	re, ok := r.routesByName[name]
	if ok == false {
		return gen.ErrNoRoute
	}
	if re.pending != RoutePendingNone {
		return gen.ErrBusy
	}
	if re.disabled {
		return gen.ErrDisabled
	}
	var empty gen.PID
	if re.pid != empty {
		return ErrRouteRunning
	}
	pid, err := r.spawnRoute(re)
	if err != nil {
		return err
	}
	re.pid = pid
	return nil
}

func (r *Router) dropRoute(re *routeEntry) {
	delete(r.routesByName, re.Name)
	for i, e := range r.routes {
		if e == re {
			r.routes = append(r.routes[:i], r.routes[i+1:]...)
			break
		}
	}
}

//
// ProcessBehavior implementation
//

func (r *Router) ProcessInit(process gen.Process, args ...any) (rr error) {
	behavior, ok := process.Behavior().(RouterBehavior)
	if ok == false {
		return fmt.Errorf("ProcessInit: not a RouterBehavior %s", process.BehaviorName())
	}
	r.behavior = behavior
	r.Process = process
	r.mailbox = process.Mailbox()

	if lib.Recover() {
		defer func() {
			if rec := recover(); rec != nil {
				pc, fn, line, _ := runtime.Caller(2)
				r.Log().Panic("Router initialization failed. Panic reason: %#v at %s[%s:%d]",
					rec, runtime.FuncForPC(pc).Name(), fn, line)
				rr = gen.TerminateReasonPanic
			}
		}()
	}

	options, err := behavior.Init(args...)
	if err != nil {
		return err
	}

	r.options = options
	r.routes = make([]*routeEntry, 0, len(options.Routes))
	r.routesByName = make(map[gen.Atom]*routeEntry, len(options.Routes))
	r.pendingExit = make(map[gen.PID]struct{})

	for _, route := range options.Routes {
		if route.Name == "" {
			return fmt.Errorf("route name can not be empty")
		}
		if route.Factory == nil {
			return fmt.Errorf("route %q has nil Factory", route.Name)
		}
		if _, dup := r.routesByName[route.Name]; dup {
			return fmt.Errorf("duplicate route name %q", route.Name)
		}
		re := &routeEntry{Route: route}
		pid, err := r.spawnRoute(re)
		if err != nil {
			return fmt.Errorf("spawn route %q: %w", route.Name, err)
		}
		re.pid = pid
		r.routes = append(r.routes, re)
		r.routesByName[route.Name] = re
	}

	return nil
}

func (r *Router) ProcessRun() (rr error) {
	var message *gen.MailboxMessage

	if lib.Recover() {
		defer func() {
			if rec := recover(); rec != nil {
				pc, fn, line, _ := runtime.Caller(2)
				r.Log().Panic("Router terminated. Panic reason: %#v at %s[%s:%d]",
					rec, runtime.FuncForPC(pc).Name(), fn, line)
				rr = gen.TerminateReasonPanic
			}
		}()
	}

	for {
		if r.State() != gen.ProcessStateRunning {
			return gen.TerminateReasonKill
		}

		if message != nil {
			gen.ReleaseMailboxMessage(message)
			message = nil
		}

		var fromMain bool

		// pop next message: Urgent -> System -> Main
		for {
			msg, ok := r.mailbox.Urgent.Pop()
			if ok {
				message = msg.(*gen.MailboxMessage)
				break
			}

			msg, ok = r.mailbox.System.Pop()
			if ok {
				message = msg.(*gen.MailboxMessage)
				break
			}

			msg, ok = r.mailbox.Main.Pop()
			if ok {
				message = msg.(*gen.MailboxMessage)
				fromMain = true
				break
			}

			if _, ok := r.mailbox.Log.Pop(); ok {
				panic("router process can not be a logger")
			}
			return nil
		}

		switch message.Type {
		case gen.MailboxMessageTypeRegular:
			messageHasTracing := message.Tracing.ID != [2]uint64{}
			if messageHasTracing {
				r.SetPropagatingTrace(message.Tracing)
			}

			if fromMain {
				name := r.behavior.RouteMessage(message.From, message.Message)
				var terminate error
				for {
					if name == RouteDiscard {
						r.discarded++
						break
					}
					ferr := r.forward(name, message)
					if ferr == nil {
						message = nil // ownership transferred to forward target
						break
					}
					terminate = r.behavior.HandleMessage(r.PID(), MessageRouteFailed{
						Name:    name,
						From:    message.From,
						Message: message.Message,
						Reason:  ferr,
					})
					break
				}
				if messageHasTracing {
					r.SetPropagatingTrace(gen.Tracing{})
				}
				if terminate != nil {
					return terminate
				}
				break
			}

			if reason := r.behavior.HandleMessage(message.From, message.Message); reason != nil {
				r.sendSpanProcessed(message, gen.TracingKindSend, reason.Error())
				return reason
			}
			r.sendSpanProcessed(message, gen.TracingKindSend, "")
			if messageHasTracing {
				r.SetPropagatingTrace(gen.Tracing{})
			}

		case gen.MailboxMessageTypeRequest:
			messageHasTracing := message.Tracing.ID != [2]uint64{}
			if messageHasTracing {
				r.SetPropagatingTrace(message.Tracing)
			}

			if fromMain {
				name := r.behavior.RouteCall(message.From, message.Ref, message.Message)
				for {
					if name == RouteDiscard {
						r.discarded++
						r.SendResponseError(message.From, message.Ref, gen.ErrDiscarded)
						break
					}
					ferr := r.forward(name, message)
					if ferr == nil {
						message = nil // ownership transferred to forward target
						break
					}
					r.SendResponseError(message.From, message.Ref, ferr)
					break
				}
				if messageHasTracing {
					r.SetPropagatingTrace(gen.Tracing{})
				}
				break
			}

			result, reason := r.behavior.HandleCall(message.From, message.Ref, message.Message)
			if reason != nil {
				if reason == gen.TerminateReasonNormal && result != nil {
					r.sendSpanProcessed(message, gen.TracingKindRequest, "")
					r.SendResponse(message.From, message.Ref, result)
				} else {
					r.sendSpanProcessed(message, gen.TracingKindRequest, reason.Error())
				}
				return reason
			}
			r.sendSpanProcessed(message, gen.TracingKindRequest, "")
			if result != nil {
				r.SendResponse(message.From, message.Ref, result)
			}
			if messageHasTracing {
				r.SetPropagatingTrace(gen.Tracing{})
			}

		case gen.MailboxMessageTypeEvent:
			if reason := r.behavior.HandleEvent(message.Message.(gen.MessageEvent)); reason != nil {
				return reason
			}

		case gen.MailboxMessageTypeExit:
			if reason := r.handleExit(message); reason != nil {
				return reason
			}

		case gen.MailboxMessageTypeInspect:
			result := r.behavior.HandleInspect(message.From, message.Message.([]string)...)
			r.SendResponse(message.From, message.Ref, result)

		case gen.MailboxMessageTypeSpan:
			panic("router process can not be a tracing exporter")
		}
	}
}

func (r *Router) ProcessTerminate(reason error) {
	r.behavior.Terminate(reason)
}

//
// internals
//

func (r *Router) spawnRoute(re *routeEntry) (gen.PID, error) {
	opts := gen.ProcessOptions{
		LinkChild:  true,
		LinkParent: true,
	}
	return r.Spawn(re.Factory, opts, re.Args...)
}

// resolveTarget returns the pid for `name`. isRoute is true when name belongs
// to one of our routes. Falls back to local-registry lookup otherwise.
func (r *Router) resolveTarget(name gen.Atom) (gen.PID, bool, error) {
	var empty gen.PID
	if re, ok := r.routesByName[name]; ok {
		if re.disabled || re.pending == RoutePendingDisable {
			return empty, true, gen.ErrDisabled
		}
		if re.pending == RoutePendingRemove {
			return empty, true, gen.ErrNoRoute
		}
		if re.pid == empty {
			if re.pending == RoutePendingReplace {
				return empty, true, gen.ErrBusy
			}
			newPid, err := r.spawnRoute(re)
			if err != nil {
				return empty, true, err
			}
			re.pid = newPid
			r.restarts++
		}
		return re.pid, true, nil
	}
	pid, err := r.Node().ProcessPID(name)
	if err != nil {
		return empty, false, err
	}
	return pid, false, nil
}

// forward resolves the target and forwards the message. Returns the failure
// reason if forwarding ultimately failed (after respawn-and-retry for our
// own routes). Returns nil on success.
func (r *Router) forward(name gen.Atom, message *gen.MailboxMessage) error {
	pid, isRoute, err := r.resolveTarget(name)
	if err != nil {
		r.failed++
		return err
	}

	ferr := r.Forward(pid, message, gen.MessagePriorityNormal)
	if ferr == nil {
		r.forwarded++
		return nil
	}

	if isRoute && (ferr == gen.ErrProcessUnknown || ferr == gen.ErrProcessTerminated) {
		re := r.routesByName[name]
		if re.pending == RoutePendingNone {
			r.pendingExit[re.pid] = struct{}{}
			re.pid = gen.PID{}
			newPid, sperr := r.spawnRoute(re)
			if sperr == nil {
				re.pid = newPid
				r.restarts++
				if ferr2 := r.Forward(newPid, message, gen.MessagePriorityNormal); ferr2 == nil {
					r.forwarded++
					return nil
				} else {
					ferr = ferr2
				}
			} else {
				r.Log().Error("respawn route %q failed: %s", name, sperr)
				ferr = sperr
			}
		}
	}

	r.failed++
	return ferr
}

func (r *Router) handleExit(message *gen.MailboxMessage) error {
	switch exit := message.Message.(type) {
	case gen.MessageExitPID:
		var found *routeEntry
		for _, re := range r.routes {
			if re.pid == exit.PID {
				found = re
				break
			}
		}
		if found == nil {
			if _, ok := r.pendingExit[exit.PID]; ok {
				delete(r.pendingExit, exit.PID)
				return nil
			}
			return fmt.Errorf("%s: %w", exit.PID, exit.Reason)
		}

		found.pid = gen.PID{}

		switch found.pending {
		case RoutePendingNone:
			if found.disabled {
				return nil
			}
			pid, err := r.spawnRoute(found)
			if err != nil {
				r.Log().Error("eager respawn route %q failed: %s", found.Name, err)
				return nil
			}
			found.pid = pid
			r.restarts++

		case RoutePendingDisable:
			found.pending = RoutePendingNone
			found.disabled = true

		case RoutePendingReplace:
			spec := *found.pendingSpec
			found.pending = RoutePendingNone
			found.pendingSpec = nil
			found.Route = spec
			if found.disabled == false {
				pid, err := r.spawnRoute(found)
				if err != nil {
					r.Log().Error("respawn after replace route %q failed: %s", found.Name, err)
					return nil
				}
				found.pid = pid
			}

		case RoutePendingRemove:
			r.dropRoute(found)
		}
		return nil

	case gen.MessageExitProcessID:
		return fmt.Errorf("%s: %w", exit.ProcessID, exit.Reason)

	case gen.MessageExitAlias:
		return fmt.Errorf("%s: %w", exit.Alias, exit.Reason)

	case gen.MessageExitEvent:
		return fmt.Errorf("%s: %w", exit.Event, exit.Reason)

	case gen.MessageExitNode:
		return fmt.Errorf("%s: %w", exit.Name, gen.ErrNoConnection)

	default:
		panic(fmt.Sprintf("unknown exit message: %#v", exit))
	}
}

//
// default RouterBehavior implementations
//
// RouteMessage and RouteCall are intentionally NOT defined on *Router so that
// every user behavior must provide both. Return RouteDiscard from either to
// disable that path.
//

func (r *Router) HandleMessage(from gen.PID, message any) error {
	r.Log().Warning("Router.HandleMessage: unhandled message from %s", from)
	return nil
}

func (r *Router) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	r.Log().Warning("Router.HandleCall: unhandled request from %s", from)
	return nil, nil
}

func (r *Router) HandleEvent(message gen.MessageEvent) error {
	r.Log().Warning("Router.HandleEvent: unhandled event %#v", message)
	return nil
}

func (r *Router) Terminate(reason error) {}

func (r *Router) sendSpanProcessed(message *gen.MailboxMessage, kind gen.TracingKind, errStr string) {
	if message.Tracing.ID == [2]uint64{} {
		return
	}
	var msgType string
	if message.Message != nil {
		msgType = reflect.TypeOf(message.Message).String()
	}
	r.SendTracingSpan(gen.TracingSpan{
		TraceID:    message.Tracing.ID,
		SpanID:     message.Tracing.SpanID,
		Point:      gen.TracingPointProcessed,
		Kind:       kind,
		Timestamp:  time.Now().UnixNano(),
		Node:       r.Node().Name(),
		From:       message.From,
		To:         r.PID(),
		Ref:        message.Ref,
		Behavior:   r.BehaviorName(),
		Message:    msgType,
		Error:      errStr,
		Attributes: r.TracingAttributes(),
	})
	r.ClearTracingSpanAttributes()
}

func (r *Router) HandleInspect(from gen.PID, item ...string) map[string]string {
	var empty gen.PID
	result := map[string]string{
		"type":         "Router",
		"routes_total": fmt.Sprintf("%d", len(r.routes)),
		"mailbox_size": fmt.Sprintf("%d", r.options.MailboxSize),
		"forwarded":    fmt.Sprintf("%d", r.forwarded),
		"discarded":    fmt.Sprintf("%d", r.discarded),
		"failed":       fmt.Sprintf("%d", r.failed),
		"restarts":     fmt.Sprintf("%d", r.restarts),
	}
	active, disabled, pending := 0, 0, 0
	for _, re := range r.routes {
		base := "route:" + string(re.Name)
		if re.pid == empty {
			result[base+":pid"] = ""
		} else {
			result[base+":pid"] = re.pid.String()
			active++
		}
		result[base+":disabled"] = fmt.Sprintf("%t", re.disabled)
		if re.disabled {
			disabled++
		}
		if re.pending != RoutePendingNone {
			result[base+":pending"] = re.pending.String()
			pending++
		}
	}
	result["routes_active"] = fmt.Sprintf("%d", active)
	result["routes_disabled"] = fmt.Sprintf("%d", disabled)
	result["routes_pending"] = fmt.Sprintf("%d", pending)
	return result
}
