package node

import (
	"runtime"
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
	group    lib.Map[gen.PID, bool]
	mode     gen.ApplicationMode

	started int64
	parent  gen.Atom
	state   int32
	stopped chan struct{}
	reason  error

	log *log

	// dynamic fields, mutex-protected. Mutators push updates to the registrar.
	mu     sync.RWMutex
	tags   []gen.Atom
	weight int
}

// gen.Application implementation

func (a *application) Name() gen.Atom                    { return a.spec.Name }
func (a *application) Node() gen.Node                    { return a.node }
func (a *application) Log() gen.Log                      { return a.log }
func (a *application) Behavior() gen.ApplicationBehavior { return a.behavior }
func (a *application) Mode() gen.ApplicationMode         { return a.mode }
func (a *application) State() gen.ApplicationState {
	return gen.ApplicationState(atomic.LoadInt32(&a.state))
}

func (a *application) Env(key gen.Env) (any, bool) {
	v, ok := a.spec.Env[key]
	return v, ok
}

func (a *application) EnvList() map[gen.Env]any {
	out := make(map[gen.Env]any, len(a.spec.Env))
	for k, v := range a.spec.Env {
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
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				a.log.Panic("Init panic: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)
				done <- gen.TerminateReasonPanic
			}
		}()
		done <- a.behavior.Init(ref, mode)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		a.log.Warning("Init callback exceeded deadline %v", timeout)
		return gen.ErrTimeout
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
	if atomic.CompareAndSwapInt32(&a.state,
		int32(gen.ApplicationStateLoaded), int32(gen.ApplicationStateInitializing)) == false {
		if atomic.LoadInt32(&a.state) == int32(gen.ApplicationStateRunning) {
			return gen.ErrApplicationRunning
		}
		return gen.ErrApplicationState
	}

	a.mode = mode
	a.parent = options.CorePID.Node
	a.stopped = make(chan struct{})

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

	initTimeout := pickAppTimeout(a.spec.InitTimeout, options.InitTimeout, gen.DefaultApplicationInitTimeout)
	if err := a.runInitCallback(mode, initTimeout); err != nil {
		atomic.StoreInt32(&a.state, int32(gen.ApplicationStateLoaded))
		return err
	}

	for _, item := range a.spec.Group {
		timeout := item.Options.InitTimeout
		if timeout == 0 {
			timeout = gen.DefaultRequestTimeout
		}
		if timeout > gen.DefaultRequestTimeout*3 {
			a.spawnFailCleanup(gen.ErrNotAllowed)
			return gen.ErrNotAllowed
		}
		deadline := time.Now().Unix() + int64(timeout)
		ref, err := a.node.MakeRefWithDeadline(deadline)
		if err != nil {
			a.spawnFailCleanup(err)
			return err
		}

		opts := gen.ProcessOptionsExtra{
			Register:       item.Name,
			ProcessOptions: item.Options,
			ParentPID:      options.CorePID,
			ParentLeader:   options.CorePID,
			ParentLogLevel: options.CoreLogLevel,
			ParentEnv:      appEnv,
			Application:    a.spec.Name,
			Ref:            ref,
		}
		opts.Args = item.Args

		pid, err := a.node.spawn(item.Factory, opts)
		if err != nil {
			a.spawnFailCleanup(err)
			return err
		}
		a.group.Store(pid, true)
	}

	atomic.StoreInt32(&a.state, int32(gen.ApplicationStateRunning))
	a.started = time.Now().Unix()
	a.log.Info("started")

	startTimeout := pickAppTimeout(a.spec.StartTimeout, options.StartTimeout, gen.DefaultApplicationStartTimeout)
	a.runStartCallback(mode, startTimeout)
	a.registerAppRoute()
	return nil
}

// spawnFailCleanup handles Init-OK-but-spawn-failed: Initializing → Stopping,
// drain any spawned members, ensure Terminate fires.
func (a *application) spawnFailCleanup(reason error) {
	atomic.StoreInt32(&a.state, int32(gen.ApplicationStateStopping))
	a.mode = gen.ApplicationModeTemporary
	a.reason = reason

	if a.group.Len() == 0 {
		atomic.StoreInt32(&a.state, int32(gen.ApplicationStateLoaded))
		close(a.stopped)
		a.runTerminateCallback(reason)
		return
	}

	a.killMembers()
	<-a.stopped
}

func (a *application) stop(force bool, timeout time.Duration) error {
	if swapped := atomic.CompareAndSwapInt32(&a.state,
		int32(gen.ApplicationStateRunning),
		int32(gen.ApplicationStateStopping)); swapped == false {
		state := atomic.LoadInt32(&a.state)
		if state == int32(gen.ApplicationStateLoaded) {
			return nil // already stopped
		}

		if force == false {
			if state == int32(gen.ApplicationStateStopping) {
				return gen.ErrApplicationStopping
			}
			return gen.ErrApplicationState
		}
	}

	a.registerAppRoute()
	a.mode = gen.ApplicationModeTemporary

	if force {
		a.reason = gen.TerminateReasonKill
	} else {
		a.reason = gen.TerminateReasonShutdown
		stopTimeout := pickAppTimeout(a.spec.StopTimeout, 0, gen.DefaultApplicationStopTimeout)
		a.runStopCallback(a.reason, stopTimeout)
	}

	pids := a.collectMemberPIDs()
	for _, pid := range pids {
		if force {
			a.node.Kill(pid)
		} else {
			a.node.SendExit(pid, gen.TerminateReasonShutdown)
		}
	}

	select {
	case <-a.stopped:
		return nil
	case <-time.After(timeout):
		return gen.ErrApplicationStopping
	}
}

func (a *application) terminate(pid gen.PID, reason error) {
	if _, exist := a.group.LoadAndDelete(pid); exist == false {
		// child process from deeper in the supervision tree
		return
	}

	switch a.mode {
	case gen.ApplicationModePermanent:
		if atomic.CompareAndSwapInt32(&a.state,
			int32(gen.ApplicationStateRunning), int32(gen.ApplicationStateStopping)) {
			a.log.Info("stopping due to termination of %s with reason: %s", pid, reason)
			a.reason = reason
			go a.coordinateAutoStop(reason)
		}
	case gen.ApplicationModeTransient:
		if reason == gen.TerminateReasonNormal || reason == gen.TerminateReasonShutdown {
			break
		}
		if atomic.CompareAndSwapInt32(&a.state,
			int32(gen.ApplicationStateRunning), int32(gen.ApplicationStateStopping)) {
			a.log.Info("stopping due to termination of %s with reason: %s", pid, reason)
			a.reason = reason
			go a.coordinateAutoStop(reason)
		}
	}

	if a.group.Len() > 0 {
		return
	}

	if a.reason == nil {
		a.reason = gen.TerminateReasonNormal
	}

	old := atomic.SwapInt32(&a.state, int32(gen.ApplicationStateLoaded))
	if old == int32(gen.ApplicationStateLoaded) {
		return
	}
	if a.stopped != nil {
		close(a.stopped)
	}

	a.started = 0
	a.parent = ""

	a.log.Info("stopped with reason %s", a.reason)
	a.runTerminateCallback(a.reason)

	if a.node.Network().Mode() != gen.NetworkModeEnabled {
		return
	}
	a.registerAppRoute()
}

// coordinateAutoStop runs Stop callback then exits Group members. Spawned as
// a goroutine from terminate() so we don't block the process termination path.
func (a *application) coordinateAutoStop(reason error) {
	stopTimeout := pickAppTimeout(a.spec.StopTimeout, 0, gen.DefaultApplicationStopTimeout)
	a.runStopCallback(reason, stopTimeout)
	a.exitMembers(gen.TerminateReasonShutdown)
}

func (a *application) info() gen.ApplicationInfo {
	var info gen.ApplicationInfo
	info.Name = a.spec.Name
	info.Weight = a.spec.Weight

	// copy tags slice
	if len(a.spec.Tags) > 0 {
		info.Tags = make([]gen.Atom, len(a.spec.Tags))
		copy(info.Tags, a.spec.Tags)
	}

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
	info.Mode = a.mode
	info.Parent = a.parent
	info.Uptime = time.Now().Unix() - a.started
	info.Group = []gen.PID{}
	a.group.Range(func(pid gen.PID, _ bool) bool {
		info.Group = append(info.Group, pid)
		return true
	})

	info.Env = make(map[gen.Env]any)
	if a.node.security.ExposeEnvInfo {
		for k, v := range a.spec.Env {
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

func (a *application) collectMemberPIDs() []gen.PID {
	var pids []gen.PID
	a.group.Range(func(pid gen.PID, _ bool) bool {
		pids = append(pids, pid)
		return true
	})
	return pids
}

func (a *application) killMembers() {
	for _, pid := range a.collectMemberPIDs() {
		a.node.Kill(pid)
	}
}

func (a *application) exitMembers(reason error) {
	for _, pid := range a.collectMemberPIDs() {
		a.node.SendExit(pid, reason)
	}
}

func (a *application) registerAppRoute() {
	a.mu.RLock()
	tags := append([]gen.Atom(nil), a.tags...)
	weight := a.weight
	a.mu.RUnlock()
	appRoute := gen.ApplicationRoute{
		Node:   a.node.name,
		Name:   a.spec.Name,
		Weight: weight,
		Tags:   tags,
		Mode:   a.mode,
		State:  gen.ApplicationState(atomic.LoadInt32(&a.state)),
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
