package unit

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// MetaSubject drives a meta-process behavior under test. It is spawned from an
// actor Subject (sub.SpawnMeta), inheriting the parent PID and environment, and
// shares the parent's record stream: a meta's egress (Send, SendResponse, child
// SpawnMeta, Log) is observed as originating from the parent PID, exactly as the
// real runtime routes it. Scope an assertion to the meta phase with Since(mark)
// when the parent and the meta both send.
//
// The drivers mirror the meta runtime: DeliverMessage runs HandleMessage, Request
// runs HandleCall (auto-responding with a non-nil result), Inspect runs
// HandleInspect, Terminate runs Terminate. The blocking Start() integration loop
// is out of scope for the unit harness (it belongs in the stage harness); the thin
// Start() driver is provided only for non-blocking Start() implementations.
type MetaSubject struct {
	*check.Asserter
	t        testing.TB
	behavior gen.MetaBehavior
	meta     *mockMeta
	stubs    *stubs // the meta's own egress stubs, isolated from the parent actor
	inited   bool   // Run has been called (meta Init ran); gates drivers and double-run
}

// SpawnMeta instantiates a meta-process behavior under the parent actor, runs its
// Init, and returns a MetaSubject to drive it. Equivalent to PrepareMeta followed
// by Run. Init runs in the pre-running state (Send and Spawn are allowed,
// SendResponse is not, matching the runtime). If Init fails the meta is left
// terminated and the error is returned.
func (s *Subject) SpawnMeta(behavior gen.MetaBehavior, options gen.MetaOptions) (*MetaSubject, error) {
	s.t.Helper()
	ms := s.PrepareMeta(behavior, options)
	err := ms.Run()
	return ms, err
}

// PrepareMeta instantiates a meta-process behavior under the parent actor and
// returns its MetaSubject WITHOUT running Init, so the meta's Init-time egress can
// be stubbed first on its own scope (ms.OnSend, ms.OnSpawnMeta). Call ms.Run() to
// run Init. The meta's stubs are isolated from the parent actor; its records still
// land in the shared journal.
func (s *Subject) PrepareMeta(behavior gen.MetaBehavior, options gen.MetaOptions) *MetaSubject {
	s.t.Helper()
	level := options.LogLevel
	if level == gen.LogLevelDefault {
		level = s.node.logLevel
	}
	st := newStubs()
	m := &mockMeta{
		node:        s.node,
		proc:        s.process,
		parent:      s.process.pid,
		id:          s.node.synthAlias(),
		state:       gen.MetaStateSleep,
		priority:    options.SendPriority,
		compression: options.Compression,
		log:         newMockLog(s.node, s.process.pid, level),
		stubs:       st,
	}
	return &MetaSubject{
		Asserter: check.NewAsserter(s.t, s.node.rec),
		t:        s.t,
		behavior: behavior,
		meta:     m,
		stubs:    st,
	}
}

// Run runs the prepared meta's Init. SpawnMeta calls this for you; after PrepareMeta
// call it explicitly. Calling Run twice fails loudly. If Init fails the meta is left
// terminated and the error is returned.
func (m *MetaSubject) Run() error {
	m.t.Helper()
	if m.inited {
		m.t.Fatalf("unit: meta Run called twice (the meta is already initialized)")
		return nil
	}
	m.inited = true
	err := m.behavior.Init(m.meta)
	if err != nil {
		m.meta.state = gen.MetaStateTerminated
	}
	return err
}

// requireInited fails loudly when a driver runs before the meta's Init (PrepareMeta
// without Run).
func (m *MetaSubject) requireInited(op string) {
	m.t.Helper()
	if m.inited == false {
		m.t.Fatalf("unit: %s before the meta is initialized; call Run() after PrepareMeta (or use SpawnMeta)", op)
	}
}

// Behavior returns the meta behavior under test for white-box assertions.
func (m *MetaSubject) Behavior() gen.MetaBehavior { return m.behavior }

// ID returns the alias identifying this meta process.
func (m *MetaSubject) ID() gen.Alias { return m.meta.id }

// Parent returns the PID of the parent actor that spawned this meta.
func (m *MetaSubject) Parent() gen.PID { return m.meta.parent }

// State returns the current meta state (Sleep, Running, or Terminated).
func (m *MetaSubject) State() gen.MetaState { return m.meta.state }

// DeliverMessage delivers an async message to the meta, running HandleMessage in
// the Running state. A non-nil return terminates the meta.
func (m *MetaSubject) DeliverMessage(from gen.PID, message any) *MetaSubject {
	m.t.Helper()
	m.requireInited("DeliverMessage")
	m.requireAlive("DeliverMessage")
	m.meta.state = gen.MetaStateRunning
	reason := m.behavior.HandleMessage(from, message)
	if reason != nil {
		m.terminate(reason)
		return m
	}
	m.meta.state = gen.MetaStateSleep
	return m
}

// Request delivers a synchronous request to the meta, running HandleCall in the
// Running state, and returns the result and error. A non-nil result with no error
// (or TerminateReasonNormal) is auto-recorded as a SendResponse, mirroring the
// runtime. A non-normal error terminates the meta.
func (m *MetaSubject) Request(from gen.PID, request any) (any, error) {
	m.t.Helper()
	m.requireInited("Request")
	m.requireAlive("Request")
	ref := m.meta.node.synthRef()
	m.meta.state = gen.MetaStateRunning
	result, reason := m.behavior.HandleCall(from, ref, request)
	switch reason {
	case nil:
		if result != nil {
			m.meta.node.routeSendResponse(m.meta.parent, from, ref, result, m.meta.options())
		}
		m.meta.state = gen.MetaStateSleep
		return result, nil
	case gen.TerminateReasonNormal:
		if result != nil {
			m.meta.node.routeSendResponse(m.meta.parent, from, ref, result, m.meta.options())
		}
		m.terminate(reason)
		return result, nil
	default:
		m.terminate(reason)
		return nil, reason
	}
}

// Inspect runs HandleInspect in the Running state and returns its map.
func (m *MetaSubject) Inspect(from gen.PID, items ...string) map[string]string {
	m.t.Helper()
	m.requireInited("Inspect")
	m.requireAlive("Inspect")
	m.meta.state = gen.MetaStateRunning
	result := m.behavior.HandleInspect(from, items...)
	m.meta.state = gen.MetaStateSleep
	return result
}

// FireTimers delivers the scheduled (and not cancelled) timers this meta addressed
// to its own alias into HandleMessage, in scheduling order, and returns how many
// fired. Timers addressed elsewhere belong to the parent Subject's FireTimers.
func (m *MetaSubject) FireTimers() int {
	m.t.Helper()
	m.requireInited("FireTimers")
	fired := 0
	for _, tm := range m.meta.node.timers {
		if tm.fired || tm.cancelled {
			continue
		}
		alias, ok := tm.to.(gen.Alias)
		if ok == false || alias != m.meta.id {
			continue
		}
		if m.meta.state == gen.MetaStateTerminated {
			break
		}
		tm.fired = true
		fired++
		m.DeliverMessage(tm.from, tm.message)
	}
	return fired
}

// Start runs the behavior's Start() synchronously and then terminates the meta
// with its result (TerminateReasonNormal if nil), mirroring the runtime where a
// returning Start() ends the meta. Only for non-blocking Start() implementations;
// a Start() that blocks on I/O belongs in the stage harness.
func (m *MetaSubject) Start() error {
	m.t.Helper()
	m.requireInited("Start")
	m.requireAlive("Start")
	m.meta.state = gen.MetaStateSleep
	err := m.behavior.Start()
	reason := err
	if reason == nil {
		reason = gen.TerminateReasonNormal
	}
	m.terminate(reason)
	return err
}

// Terminate runs the behavior's Terminate(reason) and marks the meta terminated.
func (m *MetaSubject) Terminate(reason error) *MetaSubject {
	m.t.Helper()
	m.requireInited("Terminate")
	if m.meta.state == gen.MetaStateTerminated {
		m.t.Fatalf("unit: Terminate called on an already terminated meta")
	}
	m.terminate(reason)
	return m
}

func (m *MetaSubject) terminate(reason error) {
	m.meta.state = gen.MetaStateTerminated
	m.behavior.Terminate(reason)
}

func (m *MetaSubject) requireAlive(op string) {
	m.t.Helper()
	if m.meta.state == gen.MetaStateTerminated {
		m.t.Fatalf("unit: %s called on a terminated meta", op)
	}
}

// mockMeta implements gen.MetaProcess. Egress is routed through the parent node's
// record helpers with the parent PID, so it lands in the shared record stream.
type mockMeta struct {
	node        *mockNode
	proc        *mockProcess
	log         *mockLog
	parent      gen.PID
	id          gen.Alias
	priority    gen.MessagePriority
	state       gen.MetaState
	compression bool
	stubs       *stubs // the meta's own egress stubs (shared with its MetaSubject)
}

var _ gen.MetaProcess = (*mockMeta)(nil)

func (m *mockMeta) options() gen.MessageOptions {
	return gen.MessageOptions{
		Priority:    m.priority,
		Compression: gen.Compression{Enable: m.compression},
	}
}

func (m *mockMeta) ID() gen.Alias   { return m.id }
func (m *mockMeta) Parent() gen.PID { return m.parent }

func (m *mockMeta) Send(to any, message any) error {
	return m.node.routeSend(m.stubs, m.parent, to, message, m.options())
}

func (m *mockMeta) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	opts := m.options()
	opts.Priority = priority
	return m.node.routeSend(m.stubs, m.parent, to, message, opts)
}

func (m *mockMeta) SendAfter(to any, message any, after time.Duration) (gen.CancelFunc, error) {
	if m.state == gen.MetaStateTerminated {
		return nil, gen.ErrNotAllowed
	}
	return m.node.schedule(m.parent, to, message, after, m.options()), nil
}

func (m *mockMeta) SendWithPriorityAfter(to any, message any, priority gen.MessagePriority, after time.Duration) (gen.CancelFunc, error) {
	if m.state == gen.MetaStateTerminated {
		return nil, gen.ErrNotAllowed
	}
	opts := m.options()
	opts.Priority = priority
	return m.node.schedule(m.parent, to, message, after, opts), nil
}

func (m *mockMeta) SendEvery(to any, message any, period time.Duration) (gen.CancelFunc, error) {
	if m.state == gen.MetaStateTerminated {
		return nil, gen.ErrNotAllowed
	}
	if period <= 0 {
		return nil, gen.ErrIncorrect
	}
	return m.node.scheduleEvery(m.parent, to, message, period, m.options()), nil
}

func (m *mockMeta) SendWithPriorityEvery(to any, message any, priority gen.MessagePriority, period time.Duration) (gen.CancelFunc, error) {
	if m.state == gen.MetaStateTerminated {
		return nil, gen.ErrNotAllowed
	}
	if period <= 0 {
		return nil, gen.ErrIncorrect
	}
	opts := m.options()
	opts.Priority = priority
	return m.node.scheduleEvery(m.parent, to, message, period, opts), nil
}

func (m *mockMeta) SendResponse(to gen.PID, ref gen.Ref, message any) error {
	if m.state != gen.MetaStateRunning {
		return gen.ErrNotAllowed
	}
	return m.node.routeSendResponse(m.parent, to, ref, message, m.options())
}

func (m *mockMeta) SendResponseError(to gen.PID, ref gen.Ref, err error) error {
	if m.state != gen.MetaStateRunning {
		return gen.ErrNotAllowed
	}
	return m.node.routeSendResponse(m.parent, to, ref, err, m.options())
}

func (m *mockMeta) Spawn(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	if m.state == gen.MetaStateTerminated {
		return gen.Alias{}, gen.ErrNotAllowed
	}
	return m.node.routeSpawnMeta(m.stubs, m.parent, behavior)
}

func (m *mockMeta) SendPriority() gen.MessagePriority { return m.priority }

func (m *mockMeta) SetSendPriority(priority gen.MessagePriority) error {
	// the real meta.SetSendPriority (node/meta.go) has no state gate: it just stores.
	m.priority = priority
	return nil
}

func (m *mockMeta) Env(name gen.Env) (any, bool)         { return m.proc.Env(name) }
func (m *mockMeta) EnvList() map[gen.Env]any             { return m.proc.EnvList() }
func (m *mockMeta) EnvDefault(name gen.Env, def any) any { return m.proc.EnvDefault(name, def) }

func (m *mockMeta) Log() gen.Log      { return m.log }
func (m *mockMeta) Compression() bool { return m.compression }

func (m *mockMeta) SetCompression(enabled bool) error {
	if m.state != gen.MetaStateRunning {
		return gen.ErrNotAllowed
	}
	m.compression = enabled
	return nil
}
