package unit

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// TestNode implements gen.Node for testing
type TestNode struct {
	t         testing.TB
	events    lib.QueueMPSC
	options   TestOptions
	log       *TestLog
	env       map[gen.Env]any
	loggers   map[string]gen.LoggerBehavior
	processes map[gen.PID]*TestProcess
	network   *TestNetwork
	nextID    uint64
	uptime    time.Time
}

// NewTestNode creates a new test node instance
func NewTestNode(t testing.TB, events lib.QueueMPSC, options TestOptions) *TestNode {
	tn := &TestNode{
		t:         t,
		events:    events,
		options:   options,
		env:       make(map[gen.Env]any),
		loggers:   make(map[string]gen.LoggerBehavior),
		processes: make(map[gen.PID]*TestProcess),
		nextID:    2000,
		uptime:    time.Now(),
	}

	tn.log = NewTestLog(t, events, options.LogLevel)

	// Copy environment
	if options.Env != nil {
		for k, v := range options.Env {
			tn.env[k] = v
		}
	}

	// Create a test actor for the network
	testActor := &TestActor{
		t:        t,
		events:   events,
		options:  options,
		node:     tn,
		process:  NewTestProcess(t, events, tn, options),
		captures: make(map[string]any),
	}
	tn.network = newTestNetwork(testActor)

	return tn
}

// gen.Node interface implementation

func (tn *TestNode) Name() gen.Atom {
	return tn.options.NodeName
}

func (tn *TestNode) IsAlive() bool {
	return true
}

func (tn *TestNode) Uptime() int64 {
	return int64(time.Since(tn.uptime).Seconds())
}

func (tn *TestNode) Version() gen.Version {
	return gen.Version{
		Name:    "test-node",
		Release: "1.0.0",
		License: "MIT",
	}
}

func (tn *TestNode) FrameworkVersion() gen.Version {
	return gen.Version{
		Name:    "ergo-testing",
		Release: "1.0.0",
		License: "MIT",
	}
}

func (tn *TestNode) Info() (gen.NodeInfo, error) {
	return gen.NodeInfo{
		Name:      tn.options.NodeName,
		Uptime:    tn.Uptime(),
		Version:   tn.Version(),
		Framework: tn.FrameworkVersion(),
		Env:       tn.EnvList(),
		LogLevel:  tn.log.Level(),
	}, nil
}

func (tn *TestNode) EnvList() map[gen.Env]any {
	result := make(map[gen.Env]any)
	for k, v := range tn.env {
		result[k] = v
	}
	return result
}

func (tn *TestNode) SetEnv(name gen.Env, value any) {
	if value == nil {
		delete(tn.env, name)
	} else {
		tn.env[name] = value
	}
}

func (tn *TestNode) Env(name gen.Env) (any, bool) {
	value, found := tn.env[name]
	return value, found
}

func (tn *TestNode) EnvDefault(name gen.Env, def any) any {
	if value, found := tn.env[name]; found {
		return value
	}
	return def
}

func (tn *TestNode) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	tn.nextID++
	pid := gen.PID{
		Node:     tn.options.NodeName,
		ID:       tn.nextID,
		Creation: tn.options.NodeCreation,
	}

	tn.events.Push(SpawnEvent{
		Factory: factory,
		Options: options,
		Args:    args,
		Result:  pid,
	})

	return pid, nil
}

func (tn *TestNode) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	pid, err := tn.Spawn(factory, options, args...)
	if err != nil {
		return pid, err
	}

	tn.events.Push(RegisterNameEvent{
		Name: register,
		PID:  pid,
	})

	return pid, nil
}

func (tn *TestNode) RegisterName(name gen.Atom, pid gen.PID) error {
	tn.events.Push(RegisterNameEvent{
		Name: name,
		PID:  pid,
	})
	return nil
}

func (tn *TestNode) UnregisterName(name gen.Atom) (gen.PID, error) {
	tn.events.Push(UnregisterNameEvent{Name: name})
	return gen.PID{}, nil
}

func (tn *TestNode) MetaInfo(meta gen.Alias) (gen.MetaInfo, error) {
	return gen.MetaInfo{
		ID: meta,
	}, nil
}

func (tn *TestNode) ProcessInfo(pid gen.PID) (gen.ProcessInfo, error) {
	return gen.ProcessInfo{
		PID:   pid,
		State: gen.ProcessStateRunning,
	}, nil
}

func (tn *TestNode) ProcessList() ([]gen.PID, error) {
	var pids []gen.PID
	for pid := range tn.processes {
		pids = append(pids, pid)
	}
	return pids, nil
}

func (tn *TestNode) ProcessListShortInfo(start, limit int) ([]gen.ProcessShortInfo, error) {
	var infos []gen.ProcessShortInfo
	for pid := range tn.processes {
		infos = append(infos, gen.ProcessShortInfo{
			PID:   pid,
			State: gen.ProcessStateRunning,
		})
	}
	return infos, nil
}

func (tn *TestNode) ProcessState(pid gen.PID) (gen.ProcessState, error) {
	return gen.ProcessStateRunning, nil
}

// Application methods
func (tn *TestNode) ApplicationLoad(app gen.ApplicationBehavior, args ...any) (gen.Atom, error) {
	spec, err := app.Load(tn, args...)
	if err != nil {
		return "", err
	}

	// Record the load event
	tn.events.Push(SpawnEvent{
		Factory: nil, // ApplicationBehavior is not ProcessFactory
		Options: gen.ProcessOptions{},
		Args:    args,
		Result:  gen.PID{Node: tn.options.NodeName, ID: 0, Creation: tn.options.NodeCreation},
	})

	return spec.Name, nil
}

func (tn *TestNode) ApplicationInfo(name gen.Atom) (gen.ApplicationInfo, error) {
	return gen.ApplicationInfo{
		Name:  name,
		State: gen.ApplicationStateRunning,
	}, nil
}

func (tn *TestNode) ApplicationUnload(name gen.Atom) error {
	tn.events.Push(UnregisterNameEvent{Name: name})
	return nil
}

func (tn *TestNode) ApplicationStart(name gen.Atom, options gen.ApplicationOptions) error {
	tn.events.Push(SpawnEvent{
		Factory: nil,
		Options: gen.ProcessOptions{},
		Args:    []any{name, options},
		Result:  gen.PID{},
	})
	return nil
}

func (tn *TestNode) ApplicationStartTemporary(name gen.Atom, options gen.ApplicationOptions) error {
	return tn.ApplicationStart(name, options)
}

func (tn *TestNode) ApplicationStartTransient(name gen.Atom, options gen.ApplicationOptions) error {
	return tn.ApplicationStart(name, options)
}

func (tn *TestNode) ApplicationStartPermanent(name gen.Atom, options gen.ApplicationOptions) error {
	return tn.ApplicationStart(name, options)
}

func (tn *TestNode) ApplicationStop(name gen.Atom) error {
	tn.events.Push(ExitEvent{
		To:     gen.PID{},
		Reason: gen.TerminateReasonShutdown,
	})
	return nil
}

func (tn *TestNode) ApplicationStopForce(name gen.Atom) error {
	tn.events.Push(ExitEvent{
		To:     gen.PID{},
		Reason: gen.TerminateReasonKill,
	})
	return nil
}

func (tn *TestNode) ApplicationStopWithTimeout(name gen.Atom, timeout time.Duration) error {
	return tn.ApplicationStop(name)
}

func (tn *TestNode) Applications() []gen.Atom {
	return []gen.Atom{}
}

func (tn *TestNode) ApplicationsRunning() []gen.Atom {
	return []gen.Atom{}
}

// Network methods
func (tn *TestNode) NetworkStart(options gen.NetworkOptions) error {
	return nil
}

func (tn *TestNode) NetworkStop() error {
	return nil
}

func (tn *TestNode) Network() gen.Network {
	return tn.network
}

func (tn *TestNode) Cron() gen.Cron {
	return nil // TestCron could be implemented later
}

func (tn *TestNode) CertManager() gen.CertManager {
	return nil
}

func (tn *TestNode) Security() gen.SecurityOptions {
	return gen.SecurityOptions{}
}

func (tn *TestNode) Stop() {
	// No-op for testing
}

func (tn *TestNode) StopForce() {
	// No-op for testing
}

func (tn *TestNode) Wait() {
	// No-op for testing
}

func (tn *TestNode) WaitWithTimeout(timeout time.Duration) error {
	return nil
}

func (tn *TestNode) Kill(pid gen.PID) error {
	tn.events.Push(ExitEvent{
		To:     pid,
		Reason: gen.TerminateReasonKill,
	})
	return nil
}

func (tn *TestNode) Send(to any, message any) error {
	tn.events.Push(SendEvent{
		From:    tn.PID(),
		To:      to,
		Message: message,
	})
	return nil
}

func (tn *TestNode) SendImportant(to any, message any) error {
	tn.events.Push(SendEvent{
		From:      tn.PID(),
		To:        to,
		Message:   message,
		Important: true,
	})
	return nil
}

func (tn *TestNode) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	tn.events.Push(SendEvent{
		From:     tn.PID(),
		To:       to,
		Message:  message,
		Priority: priority,
	})
	return nil
}

func (tn *TestNode) SendWithOptions(to any, options gen.MessageOptions, message any) error {
	tn.events.Push(SendEvent{
		From:     tn.PID(),
		To:       to,
		Message:  message,
		Priority: options.Priority,
		Ref:      options.Ref,
	})
	return nil
}

func (tn *TestNode) Call(to any, request any) (any, error) {
	tn.events.Push(CallEvent{
		From:    tn.PID(),
		To:      to,
		Request: request,
	})
	return nil, nil
}

func (tn *TestNode) CallWithTimeout(to any, request any, timeout int) (any, error) {
	tn.events.Push(CallEvent{
		From:    tn.PID(),
		To:      to,
		Request: request,
		Timeout: timeout,
	})
	return nil, nil
}

func (tn *TestNode) SendEvent(name gen.Atom, token gen.Ref, options gen.MessageOptions, message any) error {
	tn.events.Push(SendEventEvent{
		Name:    name,
		Token:   token,
		Message: message,
		Options: options,
	})
	return nil
}

func (tn *TestNode) RegisterEvent(name gen.Atom, options gen.EventOptions) (gen.Ref, error) {
	ref := makeTestRef()
	tn.events.Push(RegisterEvent{
		Name:    name,
		Options: options,
		Result:  ref,
	})
	return ref, nil
}

func (tn *TestNode) UnregisterEvent(name gen.Atom) error {
	tn.events.Push(UnregisterNameEvent{Name: name})
	return nil
}

func (tn *TestNode) SendExit(pid gen.PID, reason error) error {
	tn.events.Push(ExitEvent{
		To:     pid,
		Reason: reason,
	})
	return nil
}

func (tn *TestNode) Log() gen.Log {
	return tn.log
}

func (tn *TestNode) LogLevelProcess(pid gen.PID) (gen.LogLevel, error) {
	return gen.LogLevelInfo, nil
}

func (tn *TestNode) SetLogLevelProcess(pid gen.PID, level gen.LogLevel) error {
	return nil
}

func (tn *TestNode) LogLevelMeta(meta gen.Alias) (gen.LogLevel, error) {
	return gen.LogLevelInfo, nil
}

func (tn *TestNode) SetLogLevelMeta(meta gen.Alias, level gen.LogLevel) error {
	return nil
}

func (tn *TestNode) Loggers() []string {
	var names []string
	for name := range tn.loggers {
		names = append(names, name)
	}
	return names
}

func (tn *TestNode) LoggerAddPID(pid gen.PID, name string, filter ...gen.LogLevel) error {
	return nil
}

func (tn *TestNode) LoggerAdd(name string, logger gen.LoggerBehavior, filter ...gen.LogLevel) error {
	tn.loggers[name] = logger
	return nil
}

func (tn *TestNode) LoggerDeletePID(pid gen.PID) {
	// No-op for testing
}

func (tn *TestNode) LoggerDelete(name string) {
	delete(tn.loggers, name)
}

func (tn *TestNode) LoggerLevels(name string) []gen.LogLevel {
	return []gen.LogLevel{gen.LogLevelInfo}
}

func (tn *TestNode) MakeRef() gen.Ref {
	return makeTestRef()
}

func (tn *TestNode) Commercial() []gen.Version {
	return []gen.Version{}
}

func (tn *TestNode) PID() gen.PID {
	return gen.PID{
		Node:     tn.options.NodeName,
		ID:       1,
		Creation: tn.options.NodeCreation,
	}
}

func (tn *TestNode) Creation() int64 {
	return tn.options.NodeCreation
}

func (tn *TestNode) SetCTRLC(enable bool) {
	// No-op for testing
}

// Additional test-specific methods

func (tn *TestNode) AddProcess(process *TestProcess) {
	tn.processes[process.PID()] = process
}

func (tn *TestNode) RemoveProcess(pid gen.PID) {
	delete(tn.processes, pid)
}

// Helper function to create test references
func makeTestRef() gen.Ref {
	return gen.Ref{
		Node:     "test@localhost",
		Creation: time.Now().Unix(),
		ID:       [3]uint64{uint64(time.Now().UnixNano()), 0, 0},
	}
}
