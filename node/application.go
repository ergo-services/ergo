package node

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type application struct {
	spec     gen.ApplicationSpec
	node     *node
	behavior gen.ApplicationBehavior
	mode     gen.ApplicationMode

	// every process of this application. the value tells a direct spec.Group member
	// from a process spawned deeper in the tree
	procs   lib.Map[gen.PID, bool]
	members atomic.Int64

	started int64
	parent  gen.Atom
	state   int32
	reason  error

	// effective env (CoreEnv + spec.Env + per-start Env), replaced each start; nil when not running
	env atomic.Pointer[map[gen.Env]any]

	log *log

	// dynamic fields, mutex-protected. Mutators push updates to the registrar.
	mu     sync.RWMutex
	tags   []gen.Atom
	weight int

	// per-incarnation, created by start, guarded by mu
	initialized bool
	stopTimeout time.Duration
	stopped     chan struct{} // closed after the Terminate callback
	membersGone chan struct{} // closed when the last group member is gone
	drained     chan struct{} // closed when the last process of the application is gone
}

// gen.Application implementation

func (a *application) Name() gen.Atom                    { return a.spec.Name }
func (a *application) Node() gen.Node                    { return a.node }
func (a *application) Log() gen.Log                      { return a.log }
func (a *application) Behavior() gen.ApplicationBehavior { return a.behavior }
func (a *application) Mode() gen.ApplicationMode {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.mode
}
func (a *application) State() gen.ApplicationState {
	return gen.ApplicationState(atomic.LoadInt32(&a.state))
}

func (a *application) Env(key gen.Env) (any, bool) {
	env := a.spec.Env
	if p := a.env.Load(); p != nil {
		env = *p
	}
	v, ok := env[key]
	return v, ok
}

func (a *application) EnvList() map[gen.Env]any {
	env := a.spec.Env
	if p := a.env.Load(); p != nil {
		env = *p
	}
	out := make(map[gen.Env]any, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}

func (a *application) Tags() []gen.Atom {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]gen.Atom, len(a.tags))
	copy(out, a.tags)
	return out
}

func (a *application) AddTag(tag gen.Atom) error {
	a.mu.Lock()
	for _, t := range a.tags {
		if t == tag {
			a.mu.Unlock()
			return nil
		}
	}
	a.tags = append(a.tags, tag)
	a.mu.Unlock()
	a.registerAppRoute()
	return nil
}

func (a *application) RemoveTag(tag gen.Atom) error {
	a.mu.Lock()
	for i, t := range a.tags {
		if t == tag {
			a.tags = append(a.tags[:i], a.tags[i+1:]...)
			a.mu.Unlock()
			a.registerAppRoute()
			return nil
		}
	}
	a.mu.Unlock()
	return nil
}

func (a *application) SetTags(tags []gen.Atom) error {
	a.mu.Lock()
	a.tags = append([]gen.Atom(nil), tags...)
	a.mu.Unlock()
	a.registerAppRoute()
	return nil
}

func (a *application) Weight() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.weight
}

func (a *application) SetWeight(w int) error {
	a.mu.Lock()
	a.weight = w
	a.mu.Unlock()
	a.registerAppRoute()
	return nil
}

func pickAppTimeout(specVal, optsVal, def time.Duration) time.Duration {
	if optsVal > 0 {
		return optsVal
	}
	if specVal > 0 {
		return specVal
	}
	return def
}

func (a *application) runInitCallback(mode gen.ApplicationMode, timeout time.Duration) error {
	deadline := time.Now().Unix() + int64(timeout.Seconds())
	ref, _ := a.node.MakeRefWithDeadline(deadline)

	var completed int32 // 0 pending, 1 callback won, 2 timeout won
	done := make(chan error, 1)
	go func() {
		var err error
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				a.log.Panic("Init panic: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)
				err = gen.TerminateReasonPanic
			}
			if atomic.CompareAndSwapInt32(&completed, 0, 1) {
				done <- err
			}
			// else: timeout won and start() already unwound; nothing to do
		}()
		err = a.behavior.Init(ref, mode)
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		if atomic.CompareAndSwapInt32(&completed, 0, 2) {
			// timeout won: the abandoned goroutine may never return, so we report
			// the failure instead of waiting for it
			a.log.Warning("Init callback exceeded deadline %v", timeout)
			return gen.ErrTimeout
		}
		return <-done
	}
}

func (a *application) runStartCallback(mode gen.ApplicationMode, timeout time.Duration) {
	deadline := time.Now().Unix() + int64(timeout.Seconds())
	ref, _ := a.node.MakeRefWithDeadline(deadline)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				a.log.Panic("Start panic: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)
			}
		}()
		a.behavior.Start(ref, mode)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		a.log.Warning("Start callback exceeded deadline %v", timeout)
	}
}

func (a *application) runStopCallback(reason error, timeout time.Duration) {
	deadline := time.Now().Unix() + int64(timeout.Seconds())
	ref, _ := a.node.MakeRefWithDeadline(deadline)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				a.log.Panic("Stop panic: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)
			}
		}()
		a.behavior.Stop(ref, reason)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		a.log.Warning("Stop callback exceeded deadline %v", timeout)
	}
}

func (a *application) runTerminateCallback(reason error) {
	defer func() {
		if r := recover(); r != nil {
			pc, fn, line, _ := runtime.Caller(2)
			a.log.Panic("Terminate panic: %#v at %s[%s:%d]",
				r, runtime.FuncForPC(pc).Name(), fn, line)
		}
	}()
	a.behavior.Terminate(reason)
}

func (a *application) start(mode gen.ApplicationMode, options gen.ApplicationOptionsExtra) error {
	a.mu.Lock()
	if atomic.CompareAndSwapInt32(&a.state,
		int32(gen.ApplicationStateLoaded), int32(gen.ApplicationStateInitializing)) == false {
		a.mu.Unlock()
		if atomic.LoadInt32(&a.state) == int32(gen.ApplicationStateRunning) {
			return gen.ErrApplicationRunning
		}
		return gen.ErrApplicationState
	}
	a.mode = mode
	a.parent = options.CorePID.Node
	a.initialized = false
	a.stopTimeout = pickAppTimeout(a.spec.StopTimeout, options.StopTimeout, gen.DefaultApplicationStopTimeout)
	a.stopped = make(chan struct{})
	a.membersGone = make(chan struct{})
	a.drained = make(chan struct{})
	stoppedCh := a.stopped
	a.mu.Unlock()

	appEnv := make(map[gen.Env]any)
	for k, v := range options.CoreEnv {
		appEnv[k] = v
	}
	for k, v := range a.spec.Env {
		appEnv[k] = v
	}
	for k, v := range options.Env {
		appEnv[k] = v
	}
	a.env.Store(&appEnv)

	initTimeout := pickAppTimeout(a.spec.InitTimeout, options.InitTimeout, gen.DefaultApplicationInitTimeout)
	if err := a.runInitCallback(mode, initTimeout); err != nil {
		// Init failed, so Terminate must not be called. Give the state back unless a
		// stop has been requested meanwhile, then the stopper owns the unwinding
		if atomic.CompareAndSwapInt32(&a.state,
			int32(gen.ApplicationStateInitializing), int32(gen.ApplicationStateLoaded)) {
			a.env.Store(nil)
			return err
		}
		<-stoppedCh
		return err
	}

	a.mu.Lock()
	a.initialized = true
	a.mu.Unlock()

	for _, item := range a.spec.Group {
		if a.accepting() == false {
			// a stop has been requested while we were starting
			<-stoppedCh
			return gen.ErrApplicationStopping
		}
		timeout := item.Options.InitTimeout
		if timeout == 0 {
			timeout = gen.DefaultRequestTimeout
		}
		if timeout > gen.DefaultRequestTimeout*3 {
			a.requestStop(gen.ErrNotAllowed, false)
			<-stoppedCh
			return gen.ErrNotAllowed
		}
		deadline := time.Now().Unix() + int64(timeout)
		ref, err := a.node.MakeRefWithDeadline(deadline)
		if err != nil {
			a.requestStop(err, false)
			<-stoppedCh
			return err
		}

		opts := gen.ProcessOptionsExtra{
			Register:               item.Name,
			ProcessOptions:         item.Options,
			ParentPID:              options.CorePID,
			ParentLeader:           options.CorePID,
			ParentLogLevel:         options.CoreLogLevel,
			ParentEnv:              appEnv,
			Application:            a.spec.Name,
			ApplicationGroupMember: true,
			Ref:                    ref,
		}
		opts.Args = item.Args

		if _, err := a.node.spawn(item.Factory, opts); err != nil {
			a.requestStop(err, false)
			<-stoppedCh
			return err
		}
	}

	if atomic.CompareAndSwapInt32(&a.state,
		int32(gen.ApplicationStateInitializing),
		int32(gen.ApplicationStateRunning)) == false {
		<-stoppedCh
		return gen.ErrApplicationStopping
	}
	a.mu.Lock()
	a.started = time.Now().Unix()
	a.mu.Unlock()
	a.log.Info("started")
	a.node.publishCoreEvent(gen.MessageCoreApplicationStarted{Name: a.spec.Name, Mode: mode})

	startTimeout := pickAppTimeout(a.spec.StartTimeout, options.StartTimeout, gen.DefaultApplicationStartTimeout)
	a.runStartCallback(mode, startTimeout)
	a.registerAppRoute()

	if a.members.Load() == 0 {
		// every group member finished while the application was still starting
		a.requestStop(gen.TerminateReasonNormal, false)
	}
	return nil
}

func (a *application) stop(force bool, timeout time.Duration) error {
	reason := gen.TerminateReasonShutdown
	if force {
		reason = gen.TerminateReasonKill
	}

	if a.requestStop(reason, force) == false {
		switch gen.ApplicationState(atomic.LoadInt32(&a.state)) {
		case gen.ApplicationStateLoaded:
			return nil // already stopped
		case gen.ApplicationStateStopping:
			if force == false {
				return gen.ErrApplicationStopping
			}
			// a graceful teardown is already running. cut it short
			a.killProcesses(a.procPIDs())
		default:
			return gen.ErrApplicationState
		}
	}

	if timeout <= 0 {
		return nil
	}

	a.mu.RLock()
	stoppedCh := a.stopped
	a.mu.RUnlock()
	if stoppedCh == nil {
		return nil
	}

	select {
	case <-stoppedCh:
		return nil
	case <-time.After(timeout):
		return gen.ErrApplicationStopping
	}
}

// requestStop claims the teardown of the current incarnation. The caller that moves the
// application into Stopping owns it and runs the stopper, everyone else gets false and
// waits on the stopped channel.
func (a *application) requestStop(reason error, force bool) bool {
	a.mu.Lock()
	fromRunning := atomic.CompareAndSwapInt32(&a.state,
		int32(gen.ApplicationStateRunning), int32(gen.ApplicationStateStopping))
	if fromRunning == false {
		if atomic.CompareAndSwapInt32(&a.state,
			int32(gen.ApplicationStateInitializing), int32(gen.ApplicationStateStopping)) == false {
			a.mu.Unlock()
			return false
		}
	}
	a.reason = reason
	timeout := a.stopTimeout
	a.mu.Unlock()

	a.registerAppRoute()
	// the Stop callback pairs with Start: it is skipped if the application never got there
	go a.stopper(reason, force, fromRunning, timeout)
	return true
}

// stopper owns the teardown: the pre-stop callback, the exit of the group members, the
// draining of every process the application owns, and only then the Terminate callback.
func (a *application) stopper(reason error, force bool, stopCallback bool, timeout time.Duration) {
	if force == false && stopCallback {
		a.runStopCallback(reason, timeout)
	}

	members := a.memberPIDs()
	if force {
		a.killProcesses(members)
	} else {
		a.exitProcesses(members)
	}

	a.waitMembers(timeout)
	a.waitProcesses(timeout)
	a.finalize()
}

// waitMembers waits for the direct group members to terminate. Each member is the root
// of its own supervision subtree, so its exit takes that subtree down with it.
func (a *application) waitMembers(timeout time.Duration) {
	a.signal()

	a.mu.RLock()
	ch := a.membersGone
	a.mu.RUnlock()
	if ch == nil {
		return
	}

	select {
	case <-ch:
		return
	case <-time.After(timeout):
	}

	left := a.memberPIDs()
	a.log.Warning("group member(s) not stopped in %v, killing: %s", timeout, a.describe(left))
	a.killProcesses(left)

	select {
	case <-ch:
	case <-time.After(timeout):
		a.log.Error("group member(s) still running: %s", a.describe(a.memberPIDs()))
	}
}

// waitProcesses drains what the application owns beyond its group members. Whatever is
// still here once the members are gone has no living supervisor of this application
// above it, so the application stops it itself.
func (a *application) waitProcesses(timeout time.Duration) {
	a.signal()

	a.mu.RLock()
	ch := a.drained
	a.mu.RUnlock()
	if ch == nil {
		return
	}

	select {
	case <-ch:
		return
	default:
	}

	left := a.procPIDs()
	a.log.Info("stopping %d process(es) left outside the group: %s", len(left), a.describe(left))
	a.exitProcesses(left)

	select {
	case <-ch:
		return
	case <-time.After(timeout):
	}

	left = a.procPIDs()
	a.log.Warning("process(es) not stopped in %v, killing: %s", timeout, a.describe(left))
	a.killProcesses(left)

	select {
	case <-ch:
	case <-time.After(timeout):
		left = a.procPIDs()
		a.log.Error("running Terminate with %d process(es) still alive: %s", len(left), a.describe(left))
	}
}

// finalize releases the incarnation: the Terminate callback runs here, after the last
// process of the application is gone and before the application can be started again.
func (a *application) finalize() {
	a.mu.Lock()
	reason := a.reason
	if reason == nil {
		reason = gen.TerminateReasonNormal
	}
	mode := a.mode
	initialized := a.initialized
	stoppedCh := a.stopped
	a.reason = nil
	a.initialized = false
	a.started = 0
	a.parent = ""
	a.mu.Unlock()

	a.log.Info("stopped with reason %s", reason)
	a.node.publishCoreEvent(gen.MessageCoreApplicationStopped{Name: a.spec.Name, Mode: mode, Reason: reason})

	if initialized {
		a.runTerminateCallback(reason)
	}

	a.env.Store(nil)
	atomic.StoreInt32(&a.state, int32(gen.ApplicationStateLoaded))
	if stoppedCh != nil {
		close(stoppedCh)
	}
	a.registerAppRoute()
}

// registerProcess adds a process to the application. Returns false if the application
// no longer accepts processes, which aborts the spawn.
func (a *application) registerProcess(pid gen.PID, member bool) bool {
	if a.accepting() == false {
		return false
	}
	a.procs.Store(pid, member)
	if member {
		a.members.Add(1)
	}
	// the application could have moved to Stopping while we were storing
	if a.accepting() {
		return true
	}
	a.removeProcess(pid)
	return false
}

// removeProcess drops a process that never made it to the run state.
func (a *application) removeProcess(pid gen.PID) {
	member, exist := a.procs.LoadAndDelete(pid)
	if exist == false {
		return
	}
	if member {
		a.members.Add(-1)
	}
	a.signal()
}

// processTerminated is the accounting tail of a terminated process. Called once the
// process behavior Terminate callback has returned.
func (a *application) processTerminated(pid gen.PID, reason error) {
	member, exist := a.procs.LoadAndDelete(pid)
	if exist == false {
		return
	}
	if member {
		a.memberTerminated(pid, reason, a.members.Add(-1))
	}
	a.signal()
}

// memberTerminated applies the application mode to the termination of a direct group
// member, starting the teardown if the mode calls for it.
func (a *application) memberTerminated(pid gen.PID, reason error, rest int64) {
	a.mu.RLock()
	mode := a.mode
	a.mu.RUnlock()

	switch mode {
	case gen.ApplicationModePermanent:
		// any member gone stops the application, with that member's reason

	case gen.ApplicationModeTransient:
		if reason == gen.TerminateReasonNormal || reason == gen.TerminateReasonShutdown {
			if rest > 0 || a.isRunning() == false {
				return
			}
			// wound down rather than failed
			reason = gen.TerminateReasonNormal
		}

	default:
		// the group is still being spawned, the last member is not the last one yet
		if rest > 0 || a.isRunning() == false {
			return
		}
		reason = gen.TerminateReasonNormal
	}

	if a.requestStop(reason, false) {
		a.log.Info("stopping due to termination of %s with reason: %s", pid, reason)
	}
}

// signal releases the teardown waiters. Only meaningful while stopping: that is the
// state in which the process set can no longer grow.
func (a *application) signal() {
	if atomic.LoadInt32(&a.state) != int32(gen.ApplicationStateStopping) {
		return
	}
	a.mu.Lock()
	if a.membersGone != nil && a.members.Load() == 0 {
		close(a.membersGone)
		a.membersGone = nil
	}
	if a.drained != nil && a.procs.Len() == 0 {
		close(a.drained)
		a.drained = nil
	}
	a.mu.Unlock()
}

func (a *application) accepting() bool {
	switch gen.ApplicationState(atomic.LoadInt32(&a.state)) {
	case gen.ApplicationStateInitializing, gen.ApplicationStateRunning:
		return true
	}
	return false
}

func (a *application) info() gen.ApplicationInfo {
	var info gen.ApplicationInfo
	info.Name = a.spec.Name

	a.mu.RLock()
	info.Weight = a.weight
	if len(a.tags) > 0 {
		info.Tags = make([]gen.Atom, len(a.tags))
		copy(info.Tags, a.tags)
	}
	info.Mode = a.mode
	info.Parent = a.parent
	started := a.started
	a.mu.RUnlock()

	// copy map
	if len(a.spec.Map) > 0 {
		info.Map = make(map[string]gen.Atom, len(a.spec.Map))
		for k, v := range a.spec.Map {
			info.Map[k] = v
		}
	}

	info.Description = a.spec.Description
	info.Version = a.spec.Version
	info.Depends = a.spec.Depends
	if started > 0 {
		info.Uptime = time.Now().Unix() - started
	}
	info.Group = a.memberPIDs()
	info.ProcessesTotal = a.procs.Len()

	info.Env = make(map[gen.Env]any)
	if a.node.security.ExposeEnvInfo {
		env := a.spec.Env
		if p := a.env.Load(); p != nil {
			env = *p
		}
		for k, v := range env {
			info.Env[k] = v
		}
	}

	info.State = gen.ApplicationState(atomic.LoadInt32(&a.state))
	return info
}

func (a *application) tryUnload() bool {
	return atomic.CompareAndSwapInt32(&a.state, int32(gen.ApplicationStateLoaded), 0)
}

func (a *application) isRunning() bool {
	return atomic.LoadInt32(&a.state) == int32(gen.ApplicationStateRunning)
}

// memberPIDs returns the direct spec.Group members that are still alive.
func (a *application) memberPIDs() []gen.PID {
	pids := []gen.PID{}
	a.procs.Range(func(pid gen.PID, member bool) bool {
		if member {
			pids = append(pids, pid)
		}
		return true
	})
	return pids
}

// procPIDs returns every process of the application that is still alive.
func (a *application) procPIDs() []gen.PID {
	pids := []gen.PID{}
	a.procs.Range(func(pid gen.PID, _ bool) bool {
		pids = append(pids, pid)
		return true
	})
	return pids
}

func (a *application) exitProcesses(pids []gen.PID) {
	for _, pid := range pids {
		a.node.SendExit(pid, gen.TerminateReasonShutdown)
	}
}

func (a *application) killProcesses(pids []gen.PID) {
	for _, pid := range pids {
		a.node.Kill(pid)
	}
}

// describe renders the processes for a teardown log line.
func (a *application) describe(pids []gen.PID) string {
	var sb strings.Builder
	for i, pid := range pids {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(pid.String())
		value, found := a.node.processes.Load(pid)
		if found == false {
			continue
		}
		p := value.(*process)
		if p.name != "" {
			sb.WriteString(" ")
			sb.WriteString(string(p.name))
		}
		sb.WriteString(" (")
		sb.WriteString(p.sbehavior)
		sb.WriteString(")")
	}
	return sb.String()
}

func (a *application) registerAppRoute() {
	a.mu.RLock()
	tags := append([]gen.Atom(nil), a.tags...)
	weight := a.weight
	mode := a.mode
	a.mu.RUnlock()
	appRoute := gen.ApplicationRoute{
		Node:    a.node.name,
		Name:    a.spec.Name,
		Weight:  weight,
		Tags:    tags,
		Mode:    mode,
		State:   gen.ApplicationState(atomic.LoadInt32(&a.state)),
		Version: a.spec.Version,
	}
	network := a.node.Network()
	if network.Mode() != gen.NetworkModeEnabled {
		return
	}
	if reg, err := network.Registrar(); err == nil {
		reg.RegisterApplicationRoute(appRoute)
	}
}

func (a *application) unregisterAppRoute() {
	network := a.node.Network()
	if network.Mode() != gen.NetworkModeEnabled {
		return
	}
	if reg, err := network.Registrar(); err == nil {
		reg.UnregisterApplicationRoute(a.spec.Name)
	}
}
