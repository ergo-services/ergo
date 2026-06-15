package unit

import (
	"testing"

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
}

// SpawnMeta instantiates a meta-process behavior under the parent actor, runs its
// Init, and returns a MetaSubject to drive it. Init runs in the pre-running state
// (Send and Spawn are allowed, SendResponse is not, matching the runtime). If Init
// fails the meta is left terminated and the error is returned.
func (s *Subject) SpawnMeta(behavior gen.MetaBehavior, options gen.MetaOptions) (*MetaSubject, error) {
	s.t.Helper()
	level := options.LogLevel
	if level == gen.LogLevelDefault {
		level = s.node.logLevel
	}
	m := &mockMeta{
		node:        s.node,
		proc:        s.process,
		parent:      s.process.pid,
		id:          s.node.synthAlias(),
		state:       gen.MetaStateSleep,
		priority:    options.SendPriority,
		compression: options.Compression,
		log:         newMockLog(s.node, s.process.pid, level),
	}
	ms := &MetaSubject{
		Asserter: check.NewAsserter(s.t, s.node.rec),
		t:        s.t,
		behavior: behavior,
		meta:     m,
	}
	err := behavior.Init(m)
	if err != nil {
		m.state = gen.MetaStateTerminated
	}
	return ms, err
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
	m.requireAlive("Inspect")
	m.meta.state = gen.MetaStateRunning
	result := m.behavior.HandleInspect(from, items...)
	m.meta.state = gen.MetaStateSleep
	return result
}

// Start runs the behavior's Start() synchronously and then terminates the meta
// with its result (TerminateReasonNormal if nil), mirroring the runtime where a
// returning Start() ends the meta. Only for non-blocking Start() implementations;
// a Start() that blocks on I/O belongs in the stage harness.
func (m *MetaSubject) Start() error {
	m.t.Helper()
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
	return m.node.routeSend(m.parent, to, message, m.options())
}

func (m *mockMeta) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	opts := m.options()
	opts.Priority = priority
	return m.node.routeSend(m.parent, to, message, opts)
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
	return m.node.routeSpawnMeta(m.parent, behavior)
}

func (m *mockMeta) SendPriority() gen.MessagePriority { return m.priority }

func (m *mockMeta) SetSendPriority(priority gen.MessagePriority) error {
	if m.state != gen.MetaStateRunning {
		return gen.ErrNotAllowed
	}
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
