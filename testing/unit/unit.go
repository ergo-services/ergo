// Package unit is the in-process single-process test harness. It spawns exactly
// one gen.ProcessBehavior (act.Actor, act.Supervisor and the rest are just sugar
// over it) with a mocked gen.Process. That mock process's Node() returns a mock
// gen.Node, and both delegate every outbound operation to that mock node, which
// records the egress as check.Records and lets the test stub what those operations
// return (including errors) to exercise negative paths. Assertions use the shared
// testing/check grammar, identical to testing/stage; the differences are the mock
// environment and synchronous (snapshot) assertions.
package unit

import (
	"fmt"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// Subject is the spawned process under test. It embeds *check.Asserter, so the
// whole Should* grammar is available directly (subject.ShouldSend(), etc.).
type Subject struct {
	*check.Asserter
	t          testing.TB
	mock       *MockNode
	behavior   gen.ProcessBehavior
	process    *mockProcess
	node       *mockNode
	stubs      *stubs
	terminated bool
	reason     error
}

// MockNode is the test-facing mock node. It embeds the mocked gen.Node, so every
// node method (ProcessInfo, ProcessList, ...) and every On<Method> override setter
// is available directly on it; its own Spawn / SpawnRegister shadow gen.Node.Spawn
// to return the Subject under test (unit exercises exactly that one process). It is
// returned both by unit.Node(...) (configure before spawn) and by Subject.Node()
// (configure / inspect after spawn).
//
// gen.NodeOptions carries no mock gen.Network / gen.Cron (only their real config),
// so inject those with WithNetwork / WithCron before spawning; without them, a
// Node().Network() / Node().Cron() call from the process fails the test.
type MockNode struct {
	*mockNode
	t testing.TB
}

// Node creates a mock node. Mirrors stage's s.Node(name, NodeOptions): spawn the
// process under test on the returned node with Spawn / SpawnRegister. Env is taken
// from options.Env; the rest of gen.NodeOptions is accepted for parity with a real
// node but unused by the mock.
func Node(t testing.TB, name gen.Atom, options gen.NodeOptions) *MockNode {
	t.Helper()
	if name == "" {
		name = "unit@localhost"
	}
	return &MockNode{mockNode: newMockNode(t, name, options), t: t}
}

// WithNetwork injects the gen.Network returned by Node().Network(). Chainable
// before Spawn. Without it, any Node().Network() call from the process fails the test.
func (n *MockNode) WithNetwork(network gen.Network) *MockNode { n.mockNode.network = network; return n }

// WithCron injects the gen.Cron returned by Node().Cron(). Chainable before Spawn.
// Without it, any Node().Cron() call from the process fails the test.
func (n *MockNode) WithCron(cron gen.Cron) *MockNode { n.mockNode.cron = cron; return n }

// Spawn creates the process under test on this node and runs its ProcessInit.
// Mirrors gen.Node.Spawn (factory, options, args...); pass gen.ProcessOptions{}
// for defaults. Shadows the embedded gen.Node.Spawn.
func (n *MockNode) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (*Subject, error) {
	n.t.Helper()
	return n.spawn(factory, "", options, args...)
}

// SpawnRegister creates the process under test with a registered name and runs its
// ProcessInit. Mirrors gen.Node.SpawnRegister (register, factory, options, args...).
func (n *MockNode) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (*Subject, error) {
	n.t.Helper()
	return n.spawn(factory, register, options, args...)
}

func (n *MockNode) spawn(factory gen.ProcessFactory, register gen.Atom, options gen.ProcessOptions, args ...any) (*Subject, error) {
	process := newMockProcess(n.mockNode, register, options)
	// the process under test is itself a process the node knows about
	n.mockNode.registerProc(&procEntry{pid: process.pid, name: process.name, parent: process.parent, leader: process.leader, factory: factory, options: options})

	s := &Subject{
		Asserter: check.NewAsserter(n.t, n.mockNode.rec),
		t:        n.t,
		mock:     n,
		node:     n.mockNode,
		process:  process,
		stubs:    n.mockNode.stubs,
	}

	behavior := factory()
	s.behavior = behavior
	process.behavior = behavior

	if err := behavior.ProcessInit(process, args...); err != nil {
		return nil, fmt.Errorf("unit: ProcessInit: %w", err)
	}
	return s, nil
}

// Spawn creates the process under test on a default mock node and runs its
// ProcessInit. Shortcut for Node(t).Spawn(...); use Node(t, NodeOptions{...}) when
// you need to inject Network/Cron, set the node name, or seed env.
func Spawn(t testing.TB, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (*Subject, error) {
	t.Helper()
	return Node(t, "unit@localhost", gen.NodeOptions{}).Spawn(factory, options, args...)
}

// SpawnRegister creates the process under test with a registered name on a default
// mock node and runs its ProcessInit. Shortcut for Node(t, ...).SpawnRegister(...).
func SpawnRegister(t testing.TB, register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (*Subject, error) {
	t.Helper()
	return Node(t, "unit@localhost", gen.NodeOptions{}).SpawnRegister(register, factory, options, args...)
}

// Behavior returns the process's behavior for state inspection.
func (s *Subject) Behavior() gen.ProcessBehavior { return s.behavior }

// Node returns the mock node backing the process under test. Use it to override node
// methods (sub.Node().OnProcessInfo(...)) or to inspect node state directly
// (sub.Node().ProcessList()). It wraps the same node the process sees via its Node().
func (s *Subject) Node() *MockNode { return s.mock }

// PID returns the process's PID.
func (s *Subject) PID() gen.PID { return s.process.pid }

// Terminated reports whether the process has terminated.
func (s *Subject) Terminated() bool { return s.terminated }

// Reason returns the termination reason (nil while running).
func (s *Subject) Reason() error { return s.reason }

// run drives one ProcessRun and records a Terminated on abnormal return.
func (s *Subject) run() {
	if s.terminated {
		return
	}
	if err := s.behavior.ProcessRun(); err != nil {
		s.terminated = true
		s.reason = err
		s.behavior.ProcessTerminate(err)
		s.node.rec.Put(check.Terminated{PID: s.process.pid, Reason: err})
	}
}

// deliver pushes one mailbox message addressed to the process itself (Target = PID).
func (s *Subject) deliver(from gen.PID, message any, mtype gen.MailboxMessageType, ref gen.Ref) {
	s.deliverTo(from, s.process.pid, message, mtype, ref)
}

// deliverTo pushes one mailbox message with an explicit Target (how the message was
// addressed: PID, registered name (gen.Atom), or gen.Alias) and runs the process. The
// Target drives an actor's split-handler dispatch.
func (s *Subject) deliverTo(from gen.PID, target any, message any, mtype gen.MailboxMessageType, ref gen.Ref) {
	s.t.Helper()
	if s.terminated {
		s.t.Logf("unit: message to terminated process %s ignored (reason: %v)", s.process.pid, s.reason)
		return
	}
	mm := gen.TakeMailboxMessage()
	defer gen.ReleaseMailboxMessage(mm)
	mm.From = from
	mm.Type = mtype
	mm.Target = target
	mm.Message = message
	mm.Ref = ref
	// exit signals and inspect requests arrive on the urgent queue (mirrors the runtime)
	if mtype == gen.MailboxMessageTypeExit || mtype == gen.MailboxMessageTypeInspect {
		s.process.mailbox.Urgent.Push(mm)
	} else {
		s.process.mailbox.Main.Push(mm)
	}
	s.run()
}

// SendMessage delivers a regular message to the process (addressed by PID).
func (s *Subject) SendMessage(from gen.PID, message any) *Subject {
	s.deliver(from, message, gen.MailboxMessageTypeRegular, gen.Ref{})
	return s
}

// SendMessageName delivers a regular message addressed by registered name. With an
// actor in split-handler mode this dispatches to HandleMessageName.
func (s *Subject) SendMessageName(name gen.Atom, from gen.PID, message any) *Subject {
	s.deliverTo(from, name, message, gen.MailboxMessageTypeRegular, gen.Ref{})
	return s
}

// SendMessageAlias delivers a regular message addressed by alias. With an actor in
// split-handler mode this dispatches to HandleMessageAlias.
func (s *Subject) SendMessageAlias(alias gen.Alias, from gen.PID, message any) *Subject {
	s.deliverTo(from, alias, message, gen.MailboxMessageTypeRegular, gen.Ref{})
	return s
}

// SendMessageWithPriority delivers a message into the queue for the given priority
// (Max -> Urgent, High -> System, Normal -> Main).
func (s *Subject) SendMessageWithPriority(from gen.PID, message any, priority gen.MessagePriority) *Subject {
	s.t.Helper()
	if s.terminated {
		return s
	}
	mm := gen.TakeMailboxMessage()
	defer gen.ReleaseMailboxMessage(mm)
	mm.From = from
	mm.Type = gen.MailboxMessageTypeRegular
	mm.Target = s.process.pid
	mm.Message = message
	switch priority {
	case gen.MessagePriorityMax:
		s.process.mailbox.Urgent.Push(mm)
	case gen.MessagePriorityHigh:
		s.process.mailbox.System.Push(mm)
	default:
		s.process.mailbox.Main.Push(mm)
	}
	s.run()
	return s
}

// DeliverExit delivers a MessageExitPID into the urgent queue.
func (s *Subject) DeliverExit(pid gen.PID, reason error) *Subject {
	s.deliver(gen.PID{}, gen.MessageExitPID{PID: pid, Reason: reason}, gen.MailboxMessageTypeExit, gen.Ref{})
	return s
}

// DeliverExitMessage delivers an arbitrary exit message (ExitPID/ProcessID/Alias/
// Event/Node) into the urgent queue.
func (s *Subject) DeliverExitMessage(exit any) *Subject {
	s.deliver(gen.PID{}, exit, gen.MailboxMessageTypeExit, gen.Ref{})
	return s
}

// DeliverDown delivers a MessageDownPID as a regular message (a monitored target
// died); use DeliverDownMessage for the other Down kinds.
func (s *Subject) DeliverDown(pid gen.PID, reason error) *Subject {
	s.deliver(gen.PID{}, gen.MessageDownPID{PID: pid, Reason: reason}, gen.MailboxMessageTypeRegular, gen.Ref{})
	return s
}

// DeliverDownMessage delivers an arbitrary down message.
func (s *Subject) DeliverDownMessage(down any) *Subject {
	s.deliver(gen.PID{}, down, gen.MailboxMessageTypeRegular, gen.Ref{})
	return s
}

// DeliverEvent delivers a pub/sub event to the process's HandleEvent.
func (s *Subject) DeliverEvent(event gen.Event, message any) *Subject {
	s.deliver(gen.PID{}, gen.MessageEvent{Event: event, Message: message}, gen.MailboxMessageTypeEvent, gen.Ref{})
	return s
}

// DeliverLog delivers a log message to the process's HandleLog (the process must be
// registered as a logger). Arrives on the dedicated log queue.
func (s *Subject) DeliverLog(message gen.MessageLog) *Subject {
	s.t.Helper()
	if s.terminated {
		return s
	}
	s.process.mailbox.Log.Push(message)
	s.run()
	return s
}

// DeliverSpan delivers a tracing span to the process's HandleSpan (the process must
// be registered as a tracing exporter).
func (s *Subject) DeliverSpan(span gen.TracingSpan) *Subject {
	s.deliver(gen.PID{}, span, gen.MailboxMessageTypeSpan, gen.Ref{})
	return s
}

// Call makes a synchronous request to the process (addressed by PID) and resolves
// the response from the process's reply: an immediate or deferred SendResponse, or an
// error (SendResponseError or an abnormal HandleCall return).
func (s *Subject) Call(from gen.PID, request any) (any, error) {
	s.t.Helper()
	return s.callTo(from, s.process.pid, request)
}

// CallName makes a synchronous request addressed by registered name. With an actor
// in split-handler mode this dispatches to HandleCallName.
func (s *Subject) CallName(name gen.Atom, from gen.PID, request any) (any, error) {
	s.t.Helper()
	return s.callTo(from, name, request)
}

// CallAlias makes a synchronous request addressed by alias. With an actor in
// split-handler mode this dispatches to HandleCallAlias.
func (s *Subject) CallAlias(alias gen.Alias, from gen.PID, request any) (any, error) {
	s.t.Helper()
	return s.callTo(from, alias, request)
}

// CallWithPriority makes a synchronous request delivered at the given priority
// (Max -> Urgent, High -> System, Normal -> Main). Routers and pools dispatch a
// high-priority request to the admin HandleCall instead of routing/forwarding it.
func (s *Subject) CallWithPriority(from gen.PID, request any, priority gen.MessagePriority) (any, error) {
	s.t.Helper()
	if s.terminated {
		return nil, s.reason
	}
	if priority == gen.MessagePriorityNormal {
		return s.callTo(from, s.process.pid, request)
	}
	ref := s.node.synthRef()
	mk := s.node.rec.Mark()
	mm := gen.TakeMailboxMessage()
	defer gen.ReleaseMailboxMessage(mm)
	mm.From = from
	mm.Type = gen.MailboxMessageTypeRequest
	mm.Target = s.process.pid
	mm.Message = request
	mm.Ref = ref
	if priority == gen.MessagePriorityMax {
		s.process.mailbox.Urgent.Push(mm)
	} else {
		s.process.mailbox.System.Push(mm)
	}
	s.run()
	return s.resolveResponse(mk, ref)
}

func (s *Subject) callTo(from gen.PID, target any, request any) (any, error) {
	if s.terminated {
		return nil, s.reason
	}
	ref := s.node.synthRef()
	mk := s.node.rec.Mark()
	s.deliverTo(from, target, request, gen.MailboxMessageTypeRequest, ref)
	return s.resolveResponse(mk, ref)
}

// resolveResponse extracts the call response (or termination reason) recorded after
// mark for the given ref.
func (s *Subject) resolveResponse(mk int, ref gen.Ref) (any, error) {
	// a response may have been sent even if the process then terminated (e.g. a
	// HandleCall returning a result together with TerminateReasonNormal), so look for
	// the response first and fall back to the termination reason only if none was sent.
	for _, r := range s.node.rec.Records()[mk:] {
		sr, ok := r.(check.SentResponse)
		if ok == false || sr.Ref != ref {
			continue
		}
		if e, ok := sr.Message.(error); ok {
			return nil, e
		}
		return sr.Message, nil
	}
	if s.terminated && s.reason != nil {
		return nil, s.reason
	}
	return nil, nil
}

// Inspect sends an inspection request to the process and resolves the map the
// process replies with via HandleInspect. from is the inspecting PID (the real
// Inspect sets the message From to the caller's PID); pass gen.PID{} when the actor
// ignores it. Mirrors the runtime: the request arrives on the urgent queue and the
// reply is a SendResponse carrying the result map.
func (s *Subject) Inspect(from gen.PID, items ...string) (map[string]string, error) {
	s.t.Helper()
	if s.terminated {
		return nil, s.reason
	}
	ref := s.node.synthRef()
	mk := s.node.rec.Mark()
	s.deliver(from, items, gen.MailboxMessageTypeInspect, ref)
	if s.terminated && s.reason != nil {
		return nil, s.reason
	}
	for _, r := range s.node.rec.Records()[mk:] {
		sr, ok := r.(check.SentResponse)
		if ok == false || sr.Ref != ref {
			continue
		}
		if e, ok := sr.Message.(error); ok {
			return nil, e
		}
		m, _ := sr.Message.(map[string]string)
		return m, nil
	}
	return nil, nil
}

// FireTimers delivers every scheduled (and not cancelled) SendAfter message whose
// target is the process under test into its mailbox, in scheduling order, and runs
// the process. Timers targeting other processes are marked fired but not delivered
// (the message would leave the process, which the harness only records). Returns the
// number of timers fired.
func (s *Subject) FireTimers() int {
	s.t.Helper()
	fired := 0
	for _, tm := range s.node.timers {
		if tm.fired || tm.cancelled {
			continue
		}
		tm.fired = true
		fired++
		if toSelf(tm.to, s.process.pid, s.process.name) {
			s.deliver(tm.from, tm.message, gen.MailboxMessageTypeRegular, gen.Ref{})
		}
	}
	return fired
}

// toSelf reports whether a SendAfter target addresses the process under test.
func toSelf(to any, pid gen.PID, name gen.Atom) bool {
	switch t := to.(type) {
	case gen.PID:
		return t == pid
	case gen.Atom:
		return name != "" && t == name
	case gen.ProcessID:
		return name != "" && t.Name == name
	}
	return false
}
