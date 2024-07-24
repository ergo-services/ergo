package node

import (
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
)

type application struct {
	spec     gen.ApplicationSpec
	node     *node
	behavior gen.ApplicationBehavior
	group    sync.Map
	mode     gen.ApplicationMode

	started int64
	parent  gen.Atom
	state   int32
	stopped chan struct{}
	reason  error
}

func (a *application) start(mode gen.ApplicationMode, options gen.ApplicationOptionsExtra) error {
	if swapped := atomic.CompareAndSwapInt32(&a.state,
		int32(gen.ApplicationStateLoaded), int32(gen.ApplicationStateRunning)); swapped == false {
		if atomic.LoadInt32(&a.state) == int32(gen.ApplicationStateRunning) {
			return gen.ErrApplicationRunning
		}
		return gen.ErrApplicationState
	}

	// build app env
	appEnv := make(map[gen.Env]any)
	// 1. from core env
	for k, v := range options.CoreEnv {
		appEnv[k] = v
	}
	// 2. from app spec env
	for k, v := range a.spec.Env {
		appEnv[k] = v
	}
	// 3. from options.Env (gen.ApplicationOptions.Env override spec.Env)
	for k, v := range options.Env {
		appEnv[k] = v
	}

	// start items
	for _, item := range a.spec.Group {
		opts := gen.ProcessOptionsExtra{
			Register:       item.Name,
			ProcessOptions: item.Options,
			ParentPID:      options.CorePID,
			ParentLeader:   options.CorePID,
			ParentLogLevel: options.CoreLogLevel,
			ParentEnv:      appEnv,
			Application:    a.spec.Name,
		}

		opts.Args = item.Args

		pid, err := a.node.spawn(item.Factory, opts)
		if err != nil {
			a.group.Range(func(k, _ any) bool {
				pid := k.(gen.PID)
				a.node.Kill(pid)
				return true
			})
			return err
		}

		a.group.Store(pid, true)
	}

	a.stopped = make(chan struct{})
	a.node.log.Info("application %s (%s) started", a.spec.Name, a.mode)
	a.mode = mode
	a.parent = options.CorePID.Node

	a.started = time.Now().Unix()
	a.behavior.Start(mode)
	a.registerAppRoute()

	return nil
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

	a.registerAppRoute() // new state of the app

	// update mode to prevent triggering 'permantent' mode
	a.mode = gen.ApplicationModeTemporary

	a.group.Range(func(k, _ any) bool {
		pid := k.(gen.PID)
		if force {
			a.node.Kill(pid)
		} else {
			a.node.SendExit(pid, gen.TerminateReasonShutdown)
		}
		return true
	})

	if force {
		a.reason = gen.TerminateReasonKill
	} else {
		a.reason = gen.TerminateReasonShutdown
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
		// it was started as a child process somewhere deep in the supervision tree
		// do nothing.
		return
	}

	switch a.mode {
	case gen.ApplicationModePermanent:
		state := atomic.SwapInt32(&a.state, int32(gen.ApplicationStateStopping))
		if state == int32(gen.ApplicationStateStopping) {
			// already in stopping
			break
		}
		a.node.Log().Info("application %s (%s) will be stopped due to termination of %s with reason: %s", a.spec.Name, a.mode, pid, reason)
		a.reason = reason
		a.group.Range(func(k, _ any) bool {
			pid := k.(gen.PID)
			a.node.SendExit(pid, gen.TerminateReasonShutdown)
			return true
		})
	case gen.ApplicationModeTransient:
		if reason == gen.TerminateReasonNormal || reason == gen.TerminateReasonShutdown {
			// do nothing
			break
		}
		a.node.Log().Info("application %s (%s) will be stopped due to termination of %s with reason: %s", a.spec.Name, a.mode, pid, reason)

		state := atomic.SwapInt32(&a.state, int32(gen.ApplicationStateStopping))
		if state == int32(gen.ApplicationStateStopping) {
			// already in stopping
			break
		}
		a.reason = reason
		a.group.Range(func(k, _ any) bool {
			pid := k.(gen.PID)
			a.node.SendExit(pid, gen.TerminateReasonShutdown)
			return true
		})
	default:
		// do nothing
	}

	// check if it was the last item
	empty := true
	a.group.Range(func(_, _ any) bool {
		empty = false
		return false
	})

	if empty == false {
		// do nothing
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

	a.node.log.Info("application %s (%s) stopped with reason %s", a.spec.Name, a.mode, a.reason)
	a.behavior.Terminate(a.reason)

	network := a.node.Network()
	if network.Mode() != gen.NetworkModeEnabled {
		return
	}
	a.registerAppRoute() // new state for the app
	return
}

func (a *application) info() gen.ApplicationInfo {
	var info gen.ApplicationInfo
	info.Name = a.spec.Name
	info.Weight = a.spec.Weight
	info.Description = a.spec.Description
	info.Version = a.spec.Version
	info.Depends = a.spec.Depends
	info.Mode = a.mode
	info.Uptime = time.Now().Unix() - a.started
	info.Group = []gen.PID{}
	a.group.Range(func(k, _ any) bool {
		pid := k.(gen.PID)
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

func (a *application) registerAppRoute() {
	appRoute := gen.ApplicationRoute{
		Node:   a.node.name,
		Name:   a.spec.Name,
		Weight: a.spec.Weight,
		Mode:   a.mode,
		State:  gen.ApplicationState(a.state),
	}
	network := a.node.Network()
	if network.Mode() != gen.NetworkModeEnabled {
		return
	}
	if reg, err := network.Registrar(); err == nil {
		reg.RegisterApplication(appRoute)
	}
}

func (a *application) unregisterAppRoute() {
	network := a.node.Network()
	if network.Mode() != gen.NetworkModeEnabled {
		return
	}
	if reg, err := network.Registrar(); err == nil {
		reg.UnregisterApplication(a.spec.Name)
	}
}
