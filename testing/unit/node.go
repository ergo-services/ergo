package unit

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

var refCounter uint64

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
	cron      *TestCron
	Failures
}

// TestCron implements gen.Cron interface for testing
type TestCron struct {
	t      testing.TB
	events lib.QueueMPSC
	jobs   map[gen.Atom]gen.CronJob

	// Mock time for testing cron schedules
	mockTime    time.Time
	useMockTime bool
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
		cron:      NewTestCron(t, events),
		Failures:  newFailures(events, "node"),
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
	if err := tn.CheckMethodFailure("Spawn", factory, options, args); err != nil {
		return gen.PID{}, err
	}

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

func (tn *TestNode) SpawnRegister(
	register gen.Atom,
	factory gen.ProcessFactory,
	options gen.ProcessOptions,
	args ...any,
) (gen.PID, error) {
	if err := tn.CheckMethodFailure("SpawnRegister", factory, options, args); err != nil {
		return gen.PID{}, err
	}

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
	// Check for failure injection
	if err := tn.CheckMethodFailure("RegisterName", name, pid); err != nil {
		return err
	}

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

func (tn *TestNode) ProcessName(pid gen.PID) (gen.Atom, error) {
	// Simple stub - returns empty name
	return "", nil
}

func (tn *TestNode) ProcessPID(name gen.Atom) (gen.PID, error) {
	// Simple stub - returns error
	return gen.PID{}, gen.ErrProcessUnknown
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
	// Check for failure injection
	if err := tn.CheckMethodFailure("ApplicationUnload", name); err != nil {
		return err
	}

	tn.events.Push(UnregisterNameEvent{Name: name})
	return nil
}

func (tn *TestNode) ApplicationStart(name gen.Atom, options gen.ApplicationOptions) error {
	// Check for failure injection
	if err := tn.CheckMethodFailure("ApplicationStart", name, options); err != nil {
		return err
	}

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

// ApplicationProcessList returns the list of processes that belongs to the given application.
func (tn *TestNode) ApplicationProcessList(name gen.Atom, limit int) ([]gen.PID, error) {
	tn.events.Push(SpawnEvent{
		Factory: nil,
		Options: gen.ProcessOptions{},
		Args:    []any{name, limit},
		Result:  gen.PID{},
	})

	// Return empty list for testing - can be customized per test case
	return []gen.PID{}, nil
}

// ApplicationProcessListShortInfo returns the list of processes that belongs to the given application.
func (tn *TestNode) ApplicationProcessListShortInfo(name gen.Atom, limit int) ([]gen.ProcessShortInfo, error) {
	tn.events.Push(SpawnEvent{
		Factory: nil,
		Options: gen.ProcessOptions{},
		Args:    []any{name, limit},
		Result:  gen.PID{},
	})

	// Return empty list for testing - can be customized per test case
	return []gen.ProcessShortInfo{}, nil
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

// Cron returns the test cron interface
func (tn *TestNode) Cron() gen.Cron {
	return tn.cron
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
	// Check for failure injection
	if err := tn.CheckMethodFailure("Kill", pid); err != nil {
		return err
	}

	tn.events.Push(ExitEvent{
		To:     pid,
		Reason: gen.TerminateReasonKill,
	})
	return nil
}

func (tn *TestNode) Send(to any, message any) error {
	// Check for failure injection
	if err := tn.CheckMethodFailure("Send", to, message); err != nil {
		return err
	}

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
		From:     tn.PID(),
		To:       to,
		Request:  request,
		Timeout:  gen.DefaultRequestTimeout,
		Priority: gen.MessagePriorityNormal,
	})
	return nil, nil
}

func (tn *TestNode) CallWithTimeout(to any, request any, timeout int) (any, error) {
	tn.events.Push(CallEvent{
		From:     tn.PID(),
		To:       to,
		Request:  request,
		Timeout:  timeout,
		Priority: gen.MessagePriorityNormal,
	})
	return nil, nil
}

func (tn *TestNode) CallWithPriority(to any, request any, priority gen.MessagePriority) (any, error) {
	tn.events.Push(CallEvent{
		From:     tn.PID(),
		To:       to,
		Request:  request,
		Timeout:  gen.DefaultRequestTimeout,
		Priority: priority,
	})
	return nil, nil
}

func (tn *TestNode) CallImportant(to any, request any) (any, error) {
	tn.events.Push(CallEvent{
		From:      tn.PID(),
		To:        to,
		Request:   request,
		Timeout:   gen.DefaultRequestTimeout,
		Priority:  gen.MessagePriorityNormal,
		Important: true,
	})
	return nil, nil
}

func (tn *TestNode) CallPID(to gen.PID, request any, timeout int) (any, error) {
	tn.events.Push(CallEvent{
		From:     tn.PID(),
		To:       to,
		Request:  request,
		Timeout:  timeout,
		Priority: gen.MessagePriorityNormal,
	})
	return nil, nil
}

func (tn *TestNode) CallProcessID(to gen.ProcessID, request any, timeout int) (any, error) {
	tn.events.Push(CallEvent{
		From:     tn.PID(),
		To:       to,
		Request:  request,
		Timeout:  timeout,
		Priority: gen.MessagePriorityNormal,
	})
	return nil, nil
}

func (tn *TestNode) CallAlias(to gen.Alias, request any, timeout int) (any, error) {
	tn.events.Push(CallEvent{
		From:     tn.PID(),
		To:       to,
		Request:  request,
		Timeout:  timeout,
		Priority: gen.MessagePriorityNormal,
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
	ref := makeTestRefWithCreation(tn.options.NodeName, tn.options.NodeCreation)
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
	// Check for failure injection
	if err := tn.CheckMethodFailure("SendExit", pid, reason); err != nil {
		return err
	}

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
	return makeTestRefWithCreation(tn.options.NodeName, tn.options.NodeCreation)
}

func (tn *TestNode) MakeRefWithDeadline(deadline int64) (gen.Ref, error) {
	if deadline < 1 {
		return gen.Ref{}, gen.ErrIncorrect
	}

	now := time.Now().Unix()
	if deadline <= now {
		return gen.Ref{}, gen.ErrIncorrect
	}

	ref := makeTestRefWithCreation(tn.options.NodeName, tn.options.NodeCreation)
	ref.ID[2] = uint64(deadline)
	return ref, nil
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

// Note: Failures interface methods are now embedded and automatically available

// Additional test-specific methods

func (tn *TestNode) AddProcess(process *TestProcess) {
	tn.processes[process.PID()] = process
}

func (tn *TestNode) RemoveProcess(pid gen.PID) {
	delete(tn.processes, pid)
}

// Helper function to create test references with consistent creation time
func makeTestRefWithCreation(nodeName gen.Atom, creation int64) gen.Ref {
	// Get unique ID using atomic operation
	id := atomic.AddUint64(&refCounter, 1)

	return gen.Ref{
		Node:     nodeName,
		Creation: creation,
		ID:       [3]uint64{id, 0, 0},
	}
}

// NewTestCron creates a new test cron instance
func NewTestCron(t testing.TB, events lib.QueueMPSC) *TestCron {
	return &TestCron{
		t:      t,
		events: events,
		jobs:   make(map[gen.Atom]gen.CronJob),
	}
}

// AddJob adds new job
func (tc *TestCron) AddJob(job gen.CronJob) error {
	tc.t.Helper()

	if job.Name == "" {
		return fmt.Errorf("empty job name")
	}

	if job.Action == nil {
		return fmt.Errorf("empty action")
	}

	if _, exists := tc.jobs[job.Name]; exists {
		return gen.ErrTaken
	}

	tc.jobs[job.Name] = job

	// Record event
	event := CronJobAddEvent{Job: job}
	tc.events.Push(event)

	return nil
}

// RemoveJob removes job
func (tc *TestCron) RemoveJob(name gen.Atom) error {
	tc.t.Helper()

	if _, exists := tc.jobs[name]; !exists {
		return gen.ErrUnknown
	}

	delete(tc.jobs, name)

	// Record event
	event := CronJobRemoveEvent{Name: name}
	tc.events.Push(event)

	return nil
}

// EnableJob enables previously disabled job
func (tc *TestCron) EnableJob(name gen.Atom) error {
	tc.t.Helper()

	if _, exists := tc.jobs[name]; !exists {
		return gen.ErrUnknown
	}

	// Record event
	event := CronJobEnableEvent{Name: name}
	tc.events.Push(event)

	return nil
}

// DisableJob disables job
func (tc *TestCron) DisableJob(name gen.Atom) error {
	tc.t.Helper()

	if _, exists := tc.jobs[name]; !exists {
		return gen.ErrUnknown
	}

	// Record event
	event := CronJobDisableEvent{Name: name}
	tc.events.Push(event)

	return nil
}

// Info returns information about the jobs
func (tc *TestCron) Info() gen.CronInfo {
	var info gen.CronInfo

	// Use mock time if set, otherwise use current time
	now := time.Now()
	if tc.useMockTime {
		now = tc.mockTime
	}

	info.Next = now.Add(time.Minute).Truncate(time.Minute)
	info.Spool = []gen.Atom{}
	info.Jobs = []gen.CronJobInfo{}

	for name, job := range tc.jobs {
		jobInfo := gen.CronJobInfo{
			Name:       name,
			Spec:       job.Spec,
			Location:   job.Location.String(),
			ActionInfo: job.Action.Info(),
			Disabled:   false, // In test mode, jobs are always enabled unless explicitly disabled
			Fallback:   job.Fallback,
		}
		info.Jobs = append(info.Jobs, jobInfo)
	}

	return info
}

// JobInfo returns information for the given job
func (tc *TestCron) JobInfo(name gen.Atom) (gen.CronJobInfo, error) {
	var jobInfo gen.CronJobInfo

	job, exists := tc.jobs[name]
	if !exists {
		return jobInfo, gen.ErrUnknown
	}

	jobInfo = gen.CronJobInfo{
		Name:       name,
		Spec:       job.Spec,
		Location:   job.Location.String(),
		ActionInfo: job.Action.Info(),
		Disabled:   false,
		Fallback:   job.Fallback,
	}

	return jobInfo, nil
}

// Schedule returns a list of jobs planned to be run for the given period
func (tc *TestCron) Schedule(since time.Time, duration time.Duration) []gen.CronSchedule {
	var schedule []gen.CronSchedule

	// For testing purposes, return a simplified schedule
	// In real testing, you would parse the cron spec and calculate actual schedules
	start := since.Truncate(time.Minute)
	end := start.Add(duration)

	for now := start; now.Before(end); now = now.Add(time.Minute) {
		cronSchedule := gen.CronSchedule{
			Time: now,
			Jobs: []gen.Atom{},
		}

		// For simplicity in testing, we'll say all jobs run every minute
		// Real implementation would parse cron specs
		for name := range tc.jobs {
			cronSchedule.Jobs = append(cronSchedule.Jobs, name)
		}

		if len(cronSchedule.Jobs) > 0 {
			schedule = append(schedule, cronSchedule)
		}
	}

	return schedule
}

// JobSchedule returns a list of scheduled run times for the given job and period
func (tc *TestCron) JobSchedule(job gen.Atom, since time.Time, duration time.Duration) ([]time.Time, error) {
	if _, exists := tc.jobs[job]; !exists {
		return nil, gen.ErrUnknown
	}

	var schedule []time.Time
	start := since.Truncate(time.Minute)
	end := start.Add(duration)

	// For testing purposes, return every minute (simplified)
	for now := start; now.Before(end); now = now.Add(time.Minute) {
		schedule = append(schedule, now)
	}

	return schedule, nil
}

// SetMockTime allows setting a mock time for testing time-dependent cron functionality
func (tc *TestCron) SetMockTime(t time.Time) {
	tc.mockTime = t
	tc.useMockTime = true
}

// UseMockTime enables or disables mock time usage
func (tc *TestCron) UseMockTime(use bool) {
	tc.useMockTime = use
}

// TriggerJob manually triggers a cron job for testing purposes
func (tc *TestCron) TriggerJob(name gen.Atom) error {
	tc.t.Helper()

	job, exists := tc.jobs[name]
	if !exists {
		return gen.ErrUnknown
	}

	// Use mock time if set, otherwise use current time
	actionTime := time.Now()
	if tc.useMockTime {
		actionTime = tc.mockTime
	}

	// Record execution event
	event := CronJobExecutionEvent{
		Name:       name,
		Time:       actionTime,
		ActionInfo: job.Action.Info(),
	}
	tc.events.Push(event)

	return nil
}
