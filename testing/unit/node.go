package unit

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// mockNode is the gen.Node returned by the mock process's Node(), and at the same
// time the mocked runtime that backs it: it owns the recorder, the stubs, the
// scheduled timers, the env and the node identity. Both the mock process (from =
// its pid) and the node itself (from = the node core pid) delegate every outbound
// operation to the route* helpers here, which record the egress as check.Records
// and consult the stubs for the return value. This mirrors the routing core that
// stage decorates. Network() returns a built-in stubbable mock (configured via
// sub.Node().Network()...); Cron() returns a built-in mock cron (sub.Node().Cron()).
//
// Every non-egress method first consults its override (see nodeOverrides and the
// On* setters in node_overrides.go); when unset it falls back to the default below.
// Egress methods (Send/Call/Spawn/...) keep the typed stub sugar instead.
type mockNode struct {
	t          testing.TB
	rec        *check.Recorder
	stubs      *stubs
	timers     []*timer
	nodeName   gen.Atom
	creation   int64
	logLevel   gen.LogLevel
	nextID     uint64
	env        map[gen.Env]any
	netmock    *mockNetwork // built-in stubbable network (default behind Network())
	cronmock   *mockCron    // built-in cron (default behind Cron())
	subjectPID gen.PID      // the process under test (From for RemoteNode egress records)
	log        *mockLog

	// registry of processes known to this node (the process under test plus every
	// process it spawned), so the introspection queries can answer from real state
	// instead of failing. Populated by routeSpawn and at subject creation.
	procs map[gen.PID]*procEntry
	names map[gen.Atom]gen.PID

	// per-method overrides: when set they take precedence over the default below.
	ov nodeOverrides
}

var _ gen.Node = (*mockNode)(nil)

// procEntry is a process the mock node knows about.
type procEntry struct {
	pid    gen.PID
	name   gen.Atom
	parent gen.PID
	leader gen.PID
	state  gen.ProcessState
}

type timer struct {
	from      gen.PID
	to        any
	message   any
	cancelled bool
	fired     bool
}

func newMockNode(t testing.TB, name gen.Atom, o gen.NodeOptions) *mockNode {
	env := make(map[gen.Env]any, len(o.Env))
	for k, v := range o.Env {
		env[k] = v
	}
	level := o.Log.Level
	if level == gen.LogLevelDefault {
		level = gen.LogLevelInfo
	}
	n := &mockNode{
		t:        t,
		rec:      check.NewRecorder(),
		stubs:    newStubs(),
		nodeName: name,
		creation: 1,
		logLevel: level,
		nextID:   1000,
		env:      env,
		procs:    make(map[gen.PID]*procEntry),
		names:    make(map[gen.Atom]gen.PID),
	}
	n.log = newMockLog(n, n.nodePID(), n.logLevel)
	n.netmock = newMockNetwork(n)
	n.cronmock = newMockCron(n)
	return n
}

func (n *mockNode) nodePID() gen.PID { return gen.PID{Node: n.nodeName, ID: 1, Creation: n.creation} }

func (n *mockNode) synthPID() gen.PID {
	n.nextID++
	return gen.PID{Node: n.nodeName, ID: n.nextID, Creation: n.creation}
}
func (n *mockNode) synthAlias() gen.Alias {
	n.nextID++
	return gen.Alias{Node: n.nodeName, Creation: n.creation, ID: [3]uint64{n.nextID, 0, 0}}
}
func (n *mockNode) synthRef() gen.Ref {
	n.nextID++
	return gen.Ref{Node: n.nodeName, Creation: n.creation, ID: [3]uint64{n.nextID, 0, 0}}
}

// route helpers (record + stub), shared by process and node

func (n *mockNode) routeSend(from gen.PID, to any, message any, options gen.MessageOptions) error {
	err, _ := resolveFail(n.stubs.send, to)
	n.rec.Put(check.Send{From: from, To: to, Message: message, Options: options, Error: err})
	return err
}

// routeCall is tier-3 strict: an unstubbed call has no sensible default (the
// response drives the process's logic), so it fails the test loudly.
func (n *mockNode) routeCall(from gen.PID, to any, request any) (any, error) {
	resp, err, ok := n.stubs.resolveCall(to, request)
	if ok == false {
		n.t.Helper()
		n.t.Fatalf("unit: process under test called Call to %v with %#v, but no response is stubbed; add OnCall(%v).Respond(...) or .Fail(...)", to, request, to)
	}
	n.rec.Put(check.Call{From: from, To: to, Request: request, Response: resp, Error: err})
	return resp, err
}

func (n *mockNode) routeSpawn(from gen.PID, register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions) (gen.PID, error) {
	pid, err, ok := n.stubs.resolveSpawn(factory)
	if ok == false {
		pid = n.synthPID()
	}
	if err == nil {
		leader := options.Leader
		if leader == (gen.PID{}) {
			leader = from
		}
		n.registerProc(&procEntry{pid: pid, name: register, parent: from, leader: leader, state: gen.ProcessStateRunning})
	}
	n.rec.Put(check.Spawn{Parent: from, Child: pid, Register: register, Factory: factory, Options: options, Error: err})
	return pid, err
}

// registerProc adds a process to the node registry (and its name index).
func (n *mockNode) registerProc(e *procEntry) {
	n.procs[e.pid] = e
	if e.name != "" {
		n.names[e.name] = e.pid
	}
}

func (n *mockNode) routeSpawnMeta(from gen.PID, behavior gen.MetaBehavior) (gen.Alias, error) {
	alias, err, ok := n.stubs.resolveSpawnMeta(behavior)
	if ok == false {
		alias = n.synthAlias()
	}
	n.rec.Put(check.SpawnMeta{Parent: from, Alias: alias, Error: err})
	return alias, err
}

func (n *mockNode) routeRemoteSpawn(from gen.PID, node, name, register gen.Atom, options gen.ProcessOptions) (gen.PID, error) {
	pid, err, ok := n.stubs.resolveRemoteSpawn(node, name)
	if ok == false {
		pid = n.synthPID()
	}
	n.rec.Put(check.RemoteSpawn{Parent: from, Node: node, Name: name, Register: register, Child: pid, Options: options, Error: err})
	return pid, err
}

func (n *mockNode) routeSendExit(from, to gen.PID, reason error) error {
	err, _ := resolveFail(n.stubs.exit, to)
	n.rec.Put(check.SendExit{From: from, To: to, Reason: reason, Error: err})
	return err
}
func (n *mockNode) routeSendExitMeta(from gen.PID, meta gen.Alias, reason error) error {
	err, _ := resolveFail(n.stubs.exitMeta, meta)
	n.rec.Put(check.SendExitMeta{From: from, Meta: meta, Reason: reason, Error: err})
	return err
}

func (n *mockNode) routeSendResponse(from, to gen.PID, ref gen.Ref, message any, options gen.MessageOptions) error {
	n.rec.Put(check.SendResponse{From: from, To: to, Ref: ref, Message: message, Options: options})
	return nil
}

func (n *mockNode) routeSendEvent(from gen.PID, name gen.Atom, token gen.Ref, message any, options gen.MessageOptions) error {
	n.rec.Put(check.SendEvent{From: from, Name: name, Token: token, Message: message, Options: options})
	return nil
}

func (n *mockNode) routeLink(from gen.PID, target any) error {
	err, _ := resolveFail(n.stubs.link, target)
	n.rec.Put(check.Link{From: from, Target: target, Error: err})
	return err
}
func (n *mockNode) routeUnlink(from gen.PID, target any) error {
	err, _ := resolveFail(n.stubs.unlink, target)
	n.rec.Put(check.Unlink{From: from, Target: target, Error: err})
	return err
}
func (n *mockNode) routeMonitor(from gen.PID, target any) error {
	err, _ := resolveFail(n.stubs.monitor, target)
	n.rec.Put(check.Monitor{From: from, Target: target, Error: err})
	return err
}
func (n *mockNode) routeDemonitor(from gen.PID, target any) error {
	err, _ := resolveFail(n.stubs.demonitor, target)
	n.rec.Put(check.Demonitor{From: from, Target: target, Error: err})
	return err
}

func (n *mockNode) routeForward(by, to, from gen.PID, message any) error {
	err, _ := resolveFail(n.stubs.forward, to)
	n.rec.Put(check.Forward{By: by, To: to, From: from, Message: message, Error: err})
	return err
}

func (n *mockNode) routeCreateAlias(from gen.PID) (gen.Alias, error) {
	alias, err, ok := n.stubs.resolveCreateAlias()
	if ok == false {
		alias = n.synthAlias()
	}
	n.rec.Put(check.CreateAlias{PID: from, Alias: alias, Error: err})
	return alias, err
}
func (n *mockNode) routeDeleteAlias(from gen.PID, alias gen.Alias, err error) error {
	n.rec.Put(check.DeleteAlias{PID: from, Alias: alias, Error: err})
	return err
}

func (n *mockNode) routeRegisterEvent(from gen.PID, name gen.Atom) (gen.Ref, error) {
	ref, err, ok := n.stubs.resolveRegisterEvent(name)
	if ok == false {
		ref = n.synthRef()
	}
	n.rec.Put(check.RegisterEvent{PID: from, Name: name, Ref: ref, Error: err})
	return ref, err
}
func (n *mockNode) routeUnregisterEvent(from gen.PID, name gen.Atom, err error) error {
	n.rec.Put(check.UnregisterEvent{PID: from, Name: name, Error: err})
	return err
}

// schedule records a delayed send and returns a CancelFunc; the harness delivers
// it when the test fires timers.
func (n *mockNode) schedule(from gen.PID, to any, message any, after time.Duration, options gen.MessageOptions) gen.CancelFunc {
	tm := &timer{from: from, to: to, message: message}
	n.timers = append(n.timers, tm)
	n.rec.Put(check.SendAfter{From: from, To: to, Message: message, After: after, Options: options})
	return func() bool {
		if tm.fired || tm.cancelled {
			return false
		}
		tm.cancelled = true
		return true
	}
}

// unsupported fails the test for a node value-query that has no sensible default
// and no override (the process consumes the result, so a zero would mislead it).
func (n *mockNode) unsupported(method string) {
	n.t.Helper()
	n.t.Fatalf("unit: process under test called Node().%s, which the mock does not provide; override it with sub.Node().On%s(...) or test via stage", method, method)
}

// gen.Node: identity / accessors

func (n *mockNode) Name() gen.Atom {
	if n.ov.name != nil {
		return n.ov.name()
	}
	return n.nodeName
}
func (n *mockNode) IsAlive() bool {
	if n.ov.isAlive != nil {
		return n.ov.isAlive()
	}
	return true
}
func (n *mockNode) Uptime() int64 {
	if n.ov.uptime != nil {
		return n.ov.uptime()
	}
	return 0
}
func (n *mockNode) Version() gen.Version {
	if n.ov.version != nil {
		return n.ov.version()
	}
	return gen.Version{}
}
func (n *mockNode) FrameworkVersion() gen.Version {
	if n.ov.frameworkVersion != nil {
		return n.ov.frameworkVersion()
	}
	return gen.Version{}
}
func (n *mockNode) PID() gen.PID {
	if n.ov.pid != nil {
		return n.ov.pid()
	}
	return n.nodePID()
}
func (n *mockNode) Creation() int64 {
	if n.ov.creation != nil {
		return n.ov.creation()
	}
	return n.creation
}
func (n *mockNode) Log() gen.Log {
	if n.ov.log != nil {
		return n.ov.log()
	}
	return n.log
}
func (n *mockNode) Commercial() []gen.Version {
	if n.ov.commercial != nil {
		return n.ov.commercial()
	}
	return nil
}

func (n *mockNode) EnvList() map[gen.Env]any {
	if n.ov.envList != nil {
		return n.ov.envList()
	}
	return n.env
}
func (n *mockNode) SetEnv(name gen.Env, value any) {
	if n.ov.setEnv != nil {
		n.ov.setEnv(name, value)
		return
	}
	if value == nil {
		delete(n.env, name)
		return
	}
	n.env[name] = value
}
func (n *mockNode) Env(name gen.Env) (any, bool) {
	if n.ov.env != nil {
		return n.ov.env(name)
	}
	v, ok := n.env[name]
	return v, ok
}
func (n *mockNode) EnvDefault(name gen.Env, def any) any {
	if n.ov.envDefault != nil {
		return n.ov.envDefault(name, def)
	}
	if v, ok := n.env[name]; ok {
		return v
	}
	return def
}

// refs

func (n *mockNode) MakeRef() gen.Ref {
	if n.ov.makeRef != nil {
		return n.ov.makeRef()
	}
	return n.synthRef()
}
func (n *mockNode) MakeRefWithDeadline(deadline int64) (gen.Ref, error) {
	if n.ov.makeRefWithDeadline != nil {
		return n.ov.makeRefWithDeadline(deadline)
	}
	return n.synthRef(), nil
}

// network / cron / managers (injection)

func (n *mockNode) Network() gen.Network {
	return n.netmock
}
func (n *mockNode) Cron() gen.Cron {
	return n.cronmock
}
func (n *mockNode) CertManager() gen.CertManager {
	if n.ov.certManager != nil {
		return n.ov.certManager()
	}
	return nil
}
func (n *mockNode) Security() gen.SecurityOptions {
	if n.ov.security != nil {
		return n.ov.security()
	}
	return gen.SecurityOptions{}
}
func (n *mockNode) NetworkStart(options gen.NetworkOptions) error {
	if n.ov.networkStart != nil {
		return n.ov.networkStart(options)
	}
	return nil
}
func (n *mockNode) NetworkStop() error {
	if n.ov.networkStop != nil {
		return n.ov.networkStop()
	}
	return nil
}

// egress (record + stub, sender = node core pid)

func (n *mockNode) Send(to any, message any) error {
	return n.routeSend(n.nodePID(), to, message, gen.MessageOptions{})
}
func (n *mockNode) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	return n.routeSend(n.nodePID(), to, message, gen.MessageOptions{Priority: priority})
}
func (n *mockNode) SendExit(pid gen.PID, reason error) error {
	return n.routeSendExit(n.nodePID(), pid, reason)
}
func (n *mockNode) SendEvent(name gen.Atom, token gen.Ref, options gen.MessageOptions, message any) error {
	return n.routeSendEvent(n.nodePID(), name, token, message, options)
}

func (n *mockNode) Call(to any, request any) (any, error) {
	return n.routeCall(n.nodePID(), to, request)
}
func (n *mockNode) CallWithTimeout(to any, request any, timeout int) (any, error) {
	return n.routeCall(n.nodePID(), to, request)
}
func (n *mockNode) CallWithPriority(to any, request any, priority gen.MessagePriority) (any, error) {
	return n.routeCall(n.nodePID(), to, request)
}
func (n *mockNode) CallImportant(to any, request any) (any, error) {
	return n.routeCall(n.nodePID(), to, request)
}
func (n *mockNode) CallPID(to gen.PID, request any, timeout int) (any, error) {
	return n.routeCall(n.nodePID(), to, request)
}
func (n *mockNode) CallProcessID(to gen.ProcessID, request any, timeout int) (any, error) {
	return n.routeCall(n.nodePID(), to, request)
}
func (n *mockNode) CallAlias(to gen.Alias, request any, timeout int) (any, error) {
	return n.routeCall(n.nodePID(), to, request)
}

func (n *mockNode) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	return n.routeSpawn(n.nodePID(), "", factory, options)
}
func (n *mockNode) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	return n.routeSpawn(n.nodePID(), register, factory, options)
}

func (n *mockNode) Kill(pid gen.PID) error {
	if n.ov.kill != nil {
		return n.ov.kill(pid)
	}
	e, ok := n.procs[pid]
	if ok == false {
		return gen.ErrProcessUnknown
	}
	if e.name != "" {
		delete(n.names, e.name)
	}
	delete(n.procs, pid)
	return nil
}

// name registry

func (n *mockNode) RegisterName(name gen.Atom, pid gen.PID) error {
	if n.ov.registerName != nil {
		return n.ov.registerName(name, pid)
	}
	if len(name) > 255 {
		return gen.ErrAtomTooLong
	}
	e, ok := n.procs[pid]
	if ok == false {
		return gen.ErrProcessUnknown
	}
	if e.state == gen.ProcessStateTerminated {
		return gen.ErrProcessTerminated // mirrors the real runtime: no register for a dead process
	}
	if e.name != "" {
		return gen.ErrTaken // a process may hold only one registered name
	}
	if _, taken := n.names[name]; taken {
		return gen.ErrTaken
	}
	n.names[name] = pid
	e.name = name
	return nil
}
func (n *mockNode) UnregisterName(name gen.Atom) (gen.PID, error) {
	if n.ov.unregisterName != nil {
		return n.ov.unregisterName(name)
	}
	pid, ok := n.names[name]
	if ok == false {
		return gen.PID{}, gen.ErrNameUnknown
	}
	delete(n.names, name)
	if e, ok := n.procs[pid]; ok {
		e.name = ""
	}
	return pid, nil
}

// events

func (n *mockNode) RegisterEvent(name gen.Atom, options gen.EventOptions) (gen.Ref, error) {
	return n.routeRegisterEvent(n.nodePID(), name)
}
func (n *mockNode) UnregisterEvent(name gen.Atom) error {
	var err error
	if n.ov.unregisterEvent != nil {
		err = n.ov.unregisterEvent(name)
	}
	return n.routeUnregisterEvent(n.nodePID(), name, err)
}
func (n *mockNode) EventInfo(event gen.Event) (gen.EventInfo, error) {
	if n.ov.eventInfo != nil {
		return n.ov.eventInfo(event)
	}
	n.unsupported("EventInfo")
	return gen.EventInfo{}, nil
}
func (n *mockNode) EventRangeInfo(fn func(gen.EventInfo) bool) error {
	if n.ov.eventRangeInfo != nil {
		return n.ov.eventRangeInfo(fn)
	}
	return nil
}
func (n *mockNode) EventListInfo(timestamp int64, limit int, filter ...func(gen.EventInfo) bool) ([]gen.EventInfo, error) {
	if n.ov.eventListInfo != nil {
		return n.ov.eventListInfo(timestamp, limit, filter...)
	}
	return nil, nil
}

// process introspection
//
// The Process* queries answer from the node registry (the process under test plus
// every process it spawned), so a process can spawn a child and immediately query it
// within the same callback. An unknown target yields gen.ErrProcessUnknown, exactly
// as a real node would. Override any of them with sub.Node().On<Method>(...) when the
// registry-backed default does not fit.

func (n *mockNode) Info() (gen.NodeInfo, error) {
	if n.ov.info != nil {
		return n.ov.info()
	}
	n.unsupported("Info")
	return gen.NodeInfo{}, nil
}
func (n *mockNode) MetaInfo(meta gen.Alias) (gen.MetaInfo, error) {
	if n.ov.metaInfo != nil {
		return n.ov.metaInfo(meta)
	}
	n.unsupported("MetaInfo")
	return gen.MetaInfo{}, nil
}
func (n *mockNode) ProcessInfo(pid gen.PID) (gen.ProcessInfo, error) {
	if n.ov.processInfo != nil {
		return n.ov.processInfo(pid)
	}
	e, ok := n.procs[pid]
	if ok == false {
		return gen.ProcessInfo{}, gen.ErrProcessUnknown
	}
	return gen.ProcessInfo{PID: e.pid, Name: e.name, Parent: e.parent, Leader: e.leader, State: e.state, Env: n.env}, nil
}
func (n *mockNode) ProcessList() ([]gen.PID, error) {
	if n.ov.processList != nil {
		return n.ov.processList()
	}
	list := make([]gen.PID, 0, len(n.procs))
	for pid := range n.procs {
		list = append(list, pid)
	}
	return list, nil
}
func (n *mockNode) ProcessListShortInfo(start, limit int, filter ...func(gen.ProcessShortInfo) bool) ([]gen.ProcessShortInfo, error) {
	if n.ov.processListShortInfo != nil {
		return n.ov.processListShortInfo(start, limit, filter...)
	}
	n.unsupported("ProcessListShortInfo")
	return nil, nil
}
func (n *mockNode) ProcessRangeShortInfo(fn func(gen.ProcessShortInfo) bool) error {
	if n.ov.processRangeShortInfo != nil {
		return n.ov.processRangeShortInfo(fn)
	}
	return nil
}
func (n *mockNode) ProcessName(pid gen.PID) (gen.Atom, error) {
	if n.ov.processName != nil {
		return n.ov.processName(pid)
	}
	e, ok := n.procs[pid]
	if ok == false {
		return "", gen.ErrProcessUnknown
	}
	return e.name, nil
}
func (n *mockNode) ProcessPID(name gen.Atom) (gen.PID, error) {
	if n.ov.processPID != nil {
		return n.ov.processPID(name)
	}
	pid, ok := n.names[name]
	if ok == false {
		return gen.PID{}, gen.ErrProcessUnknown
	}
	return pid, nil
}
func (n *mockNode) ProcessState(pid gen.PID) (gen.ProcessState, error) {
	if n.ov.processState != nil {
		return n.ov.processState(pid)
	}
	e, ok := n.procs[pid]
	if ok == false {
		return 0, gen.ErrProcessUnknown
	}
	return e.state, nil
}

// applications

func (n *mockNode) ApplicationLoad(app gen.ApplicationBehavior, args ...any) (gen.Atom, error) {
	if n.ov.applicationLoad != nil {
		return n.ov.applicationLoad(app, args...)
	}
	return "", nil
}
func (n *mockNode) ApplicationInfo(name gen.Atom) (gen.ApplicationInfo, error) {
	if n.ov.applicationInfo != nil {
		return n.ov.applicationInfo(name)
	}
	n.unsupported("ApplicationInfo")
	return gen.ApplicationInfo{}, nil
}
func (n *mockNode) ApplicationProcessList(name gen.Atom, limit int) ([]gen.PID, error) {
	if n.ov.applicationProcessList != nil {
		return n.ov.applicationProcessList(name, limit)
	}
	n.unsupported("ApplicationProcessList")
	return nil, nil
}
func (n *mockNode) ApplicationProcessListShortInfo(name gen.Atom, limit int) ([]gen.ProcessShortInfo, error) {
	if n.ov.applicationProcessListShortInfo != nil {
		return n.ov.applicationProcessListShortInfo(name, limit)
	}
	n.unsupported("ApplicationProcessListShortInfo")
	return nil, nil
}
func (n *mockNode) ApplicationUnload(name gen.Atom) error {
	if n.ov.applicationUnload != nil {
		return n.ov.applicationUnload(name)
	}
	return nil
}
func (n *mockNode) ApplicationStart(name gen.Atom, options gen.ApplicationOptions) error {
	if n.ov.applicationStart != nil {
		return n.ov.applicationStart(name, options)
	}
	return nil
}
func (n *mockNode) ApplicationStartTemporary(name gen.Atom, options gen.ApplicationOptions) error {
	if n.ov.applicationStartTemporary != nil {
		return n.ov.applicationStartTemporary(name, options)
	}
	return nil
}
func (n *mockNode) ApplicationStartTransient(name gen.Atom, options gen.ApplicationOptions) error {
	if n.ov.applicationStartTransient != nil {
		return n.ov.applicationStartTransient(name, options)
	}
	return nil
}
func (n *mockNode) ApplicationStartPermanent(name gen.Atom, options gen.ApplicationOptions) error {
	if n.ov.applicationStartPermanent != nil {
		return n.ov.applicationStartPermanent(name, options)
	}
	return nil
}
func (n *mockNode) ApplicationStop(name gen.Atom) error {
	if n.ov.applicationStop != nil {
		return n.ov.applicationStop(name)
	}
	return nil
}
func (n *mockNode) ApplicationStopForce(name gen.Atom) error {
	if n.ov.applicationStopForce != nil {
		return n.ov.applicationStopForce(name)
	}
	return nil
}
func (n *mockNode) ApplicationStopWithTimeout(name gen.Atom, timeout time.Duration) error {
	if n.ov.applicationStopWithTimeout != nil {
		return n.ov.applicationStopWithTimeout(name, timeout)
	}
	return nil
}
func (n *mockNode) Applications() []gen.Atom {
	if n.ov.applications != nil {
		return n.ov.applications()
	}
	return nil
}
func (n *mockNode) ApplicationsRunning() []gen.Atom {
	if n.ov.applicationsRunning != nil {
		return n.ov.applicationsRunning()
	}
	return nil
}

// inspect

func (n *mockNode) Inspect(target gen.PID, item ...string) (map[string]string, error) {
	if n.ov.inspect != nil {
		return n.ov.inspect(target, item...)
	}
	n.unsupported("Inspect")
	return nil, nil
}
func (n *mockNode) InspectMeta(alias gen.Alias, item ...string) (map[string]string, error) {
	if n.ov.inspectMeta != nil {
		return n.ov.inspectMeta(alias, item...)
	}
	n.unsupported("InspectMeta")
	return nil, nil
}

// per-process / per-meta settings

func (n *mockNode) SetProcessLogLevel(pid gen.PID, level gen.LogLevel) error {
	if n.ov.setProcessLogLevel != nil {
		return n.ov.setProcessLogLevel(pid, level)
	}
	return nil
}
func (n *mockNode) SetProcessSendPriority(pid gen.PID, priority gen.MessagePriority) error {
	if n.ov.setProcessSendPriority != nil {
		return n.ov.setProcessSendPriority(pid, priority)
	}
	return nil
}
func (n *mockNode) SetProcessCompression(pid gen.PID, enabled bool) error {
	if n.ov.setProcessCompression != nil {
		return n.ov.setProcessCompression(pid, enabled)
	}
	return nil
}
func (n *mockNode) SetProcessCompressionType(pid gen.PID, ctype gen.CompressionType) error {
	if n.ov.setProcessCompressionType != nil {
		return n.ov.setProcessCompressionType(pid, ctype)
	}
	return nil
}
func (n *mockNode) SetProcessCompressionLevel(pid gen.PID, level gen.CompressionLevel) error {
	if n.ov.setProcessCompressionLevel != nil {
		return n.ov.setProcessCompressionLevel(pid, level)
	}
	return nil
}
func (n *mockNode) SetProcessCompressionThreshold(pid gen.PID, threshold int) error {
	if n.ov.setProcessCompressionThreshold != nil {
		return n.ov.setProcessCompressionThreshold(pid, threshold)
	}
	return nil
}
func (n *mockNode) SetProcessKeepNetworkOrder(pid gen.PID, order bool) error {
	if n.ov.setProcessKeepNetworkOrder != nil {
		return n.ov.setProcessKeepNetworkOrder(pid, order)
	}
	return nil
}
func (n *mockNode) SetProcessImportantDelivery(pid gen.PID, important bool) error {
	if n.ov.setProcessImportantDelivery != nil {
		return n.ov.setProcessImportantDelivery(pid, important)
	}
	return nil
}
func (n *mockNode) SetMetaLogLevel(meta gen.Alias, level gen.LogLevel) error {
	if n.ov.setMetaLogLevel != nil {
		return n.ov.setMetaLogLevel(meta, level)
	}
	return nil
}
func (n *mockNode) SetMetaSendPriority(meta gen.Alias, priority gen.MessagePriority) error {
	if n.ov.setMetaSendPriority != nil {
		return n.ov.setMetaSendPriority(meta, priority)
	}
	return nil
}

// loggers

func (n *mockNode) Loggers() []string {
	if n.ov.loggers != nil {
		return n.ov.loggers()
	}
	return nil
}
func (n *mockNode) LoggerAddPID(pid gen.PID, name string, filter ...gen.LogLevel) error {
	if n.ov.loggerAddPID != nil {
		return n.ov.loggerAddPID(pid, name, filter...)
	}
	return nil
}
func (n *mockNode) LoggerAdd(name string, logger gen.LoggerBehavior, filter ...gen.LogLevel) error {
	if n.ov.loggerAdd != nil {
		return n.ov.loggerAdd(name, logger, filter...)
	}
	return nil
}
func (n *mockNode) LoggerDeletePID(pid gen.PID) {
	if n.ov.loggerDeletePID != nil {
		n.ov.loggerDeletePID(pid)
		return
	}
}
func (n *mockNode) LoggerDelete(name string) {
	if n.ov.loggerDelete != nil {
		n.ov.loggerDelete(name)
		return
	}
}
func (n *mockNode) LoggerLevels(name string) []gen.LogLevel {
	if n.ov.loggerLevels != nil {
		return n.ov.loggerLevels(name)
	}
	return nil
}

// tracing

func (n *mockNode) TracingExporterAddPID(pid gen.PID, name string, flags gen.TracingFlags) error {
	if n.ov.tracingExporterAddPID != nil {
		return n.ov.tracingExporterAddPID(pid, name, flags)
	}
	return nil
}
func (n *mockNode) TracingExporterAdd(name string, exporter gen.TracingBehavior, flags gen.TracingFlags) error {
	if n.ov.tracingExporterAdd != nil {
		return n.ov.tracingExporterAdd(name, exporter, flags)
	}
	return nil
}
func (n *mockNode) TracingExporterDeletePID(pid gen.PID) {
	if n.ov.tracingExporterDeletePID != nil {
		n.ov.tracingExporterDeletePID(pid)
		return
	}
}
func (n *mockNode) TracingExporterDelete(name string) {
	if n.ov.tracingExporterDelete != nil {
		n.ov.tracingExporterDelete(name)
		return
	}
}
func (n *mockNode) TracingExporters() []string {
	if n.ov.tracingExporters != nil {
		return n.ov.tracingExporters()
	}
	return nil
}
func (n *mockNode) TracingExporterFlags(name string) gen.TracingFlags {
	if n.ov.tracingExporterFlags != nil {
		return n.ov.tracingExporterFlags(name)
	}
	return 0
}
func (n *mockNode) SetTracingSampler(sampler gen.TracingSampler) error {
	if n.ov.setTracingSampler != nil {
		return n.ov.setTracingSampler(sampler)
	}
	return nil
}
func (n *mockNode) SetTracingAttribute(key, value string) {
	if n.ov.setTracingAttribute != nil {
		n.ov.setTracingAttribute(key, value)
		return
	}
}
func (n *mockNode) RemoveTracingAttribute(key string) {
	if n.ov.removeTracingAttribute != nil {
		n.ov.removeTracingAttribute(key)
		return
	}
}
func (n *mockNode) TracingSampler() gen.TracingSampler {
	if n.ov.tracingSampler != nil {
		return n.ov.tracingSampler()
	}
	return nil
}
func (n *mockNode) SetProcessTracingSampler(pid gen.PID, sampler gen.TracingSampler) error {
	if n.ov.setProcessTracingSampler != nil {
		return n.ov.setProcessTracingSampler(pid, sampler)
	}
	return nil
}

// lifecycle

func (n *mockNode) Stop() {
	if n.ov.stop != nil {
		n.ov.stop()
		return
	}
}
func (n *mockNode) StopWithTimeout(timeout time.Duration) {
	if n.ov.stopWithTimeout != nil {
		n.ov.stopWithTimeout(timeout)
		return
	}
}
func (n *mockNode) StopForce() {
	if n.ov.stopForce != nil {
		n.ov.stopForce()
		return
	}
}
func (n *mockNode) Wait() {
	if n.ov.wait != nil {
		n.ov.wait()
		return
	}
}
func (n *mockNode) WaitWithTimeout(timeout time.Duration) error {
	if n.ov.waitWithTimeout != nil {
		return n.ov.waitWithTimeout(timeout)
	}
	return nil
}
func (n *mockNode) SetCTRLC(enable bool) {
	if n.ov.setCTRLC != nil {
		n.ov.setCTRLC(enable)
		return
	}
}
