package unit

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// TestActor provides a test harness for actor testing
type TestActor struct {
	t                 testing.TB
	behavior          gen.ProcessBehavior
	process           *TestProcess
	node              *TestNode
	events            lib.QueueMPSC
	captures          map[string]any
	options           TestOptions
	storedEvents      []Event // Store events separately to avoid consuming them
	terminated        bool    // Track if actor is terminated
	terminationReason error   // Store termination reason
}

// TestOptions configures the test environment
type TestOptions struct {
	Args              []any
	LogLevel          gen.LogLevel
	Env               map[gen.Env]any
	Parent            gen.PID
	Leader            gen.PID
	Priority          gen.MessagePriority
	ImportantDelivery bool
	Register          gen.Atom
	NodeName          gen.Atom
	NodeCreation      int64
}

// Option is a function for configuring TestOptions
type Option func(*TestOptions)

// WithArgs sets the arguments for the test actor
func WithArgs(args ...any) Option {
	return func(opts *TestOptions) {
		opts.Args = args
	}
}

// WithLogLevel sets the log level for the test actor
func WithLogLevel(level gen.LogLevel) Option {
	return func(opts *TestOptions) {
		opts.LogLevel = level
	}
}

// WithEnv sets environment variables for the test actor
func WithEnv(env map[gen.Env]any) Option {
	return func(opts *TestOptions) {
		opts.Env = env
	}
}

// WithParent sets the parent PID for the test actor
func WithParent(parent gen.PID) Option {
	return func(opts *TestOptions) {
		opts.Parent = parent
	}
}

// WithRegister sets the registered name for the test actor
func WithRegister(name gen.Atom) Option {
	return func(opts *TestOptions) {
		opts.Register = name
	}
}

// WithNodeName sets the node name for the test environment
func WithNodeName(name gen.Atom) Option {
	return func(opts *TestOptions) {
		opts.NodeName = name
	}
}

// Spawn creates a new test actor instance
func Spawn(t testing.TB, factory gen.ProcessFactory, options ...Option) (*TestActor, error) {
	t.Helper()

	// Default options
	opts := TestOptions{
		LogLevel:     gen.LogLevelError,
		NodeName:     "test@localhost",
		NodeCreation: time.Now().Unix(),
	}

	// Apply options
	for _, opt := range options {
		opt(&opts)
	}

	// Create test environment
	events := lib.NewQueueMPSC()
	node := NewTestNode(t, events, opts)
	process := NewTestProcess(t, events, node, opts)

	ta := &TestActor{
		t:        t,
		process:  process,
		node:     node,
		events:   events,
		captures: make(map[string]any),
		options:  opts,
	}

	// Create the behavior and initialize it
	behavior := factory()
	ta.behavior = behavior

	// Set the behavior on the process so it can be retrieved by the actor
	ta.process.behavior = behavior

	// Set the process to point to our test process (important for act.Actor)
	if actorBehavior, ok := behavior.(interface {
		ProcessInit(gen.Process, ...any) error
	}); ok {
		if err := actorBehavior.ProcessInit(ta.process, opts.Args...); err != nil {
			return nil, fmt.Errorf("failed to initialize actor: %v", err)
		}
	}

	return ta, nil
}

// SendMessage sends a message to the actor and returns the test actor for chaining
func (ta *TestActor) SendMessage(from gen.PID, message any) *TestActor {
	ta.t.Helper()

	// Don't process messages if actor is already terminated
	if ta.terminated {
		ta.t.Logf("Attempted to send message %v to terminated actor %s (reason: %v)", message, ta.PID(), ta.terminationReason)
		return ta
	}

	// Create a mailbox message for proper message routing
	mailboxMessage := gen.TakeMailboxMessage()
	defer gen.ReleaseMailboxMessage(mailboxMessage)

	mailboxMessage.From = from
	mailboxMessage.Type = gen.MailboxMessageTypeRegular
	mailboxMessage.Target = ta.process.PID() // Default target
	mailboxMessage.Message = message

	// Put the message into the main mailbox queue
	mailbox := ta.process.Mailbox()
	mailbox.Main.Push(mailboxMessage)

	// Let the behavior process its mailbox by calling ProcessRun
	if err := ta.behavior.ProcessRun(); err != nil {
		// Actor returned an error - it should terminate
		ta.terminated = true
		ta.terminationReason = err

		// Emit termination event
		ta.events.Push(TerminateEvent{
			PID:    ta.PID(),
			Reason: err,
		})

		// Only log non-normal termination for debugging
		if err != gen.TerminateReasonNormal && err != gen.TerminateReasonShutdown {
			ta.t.Logf("Actor %s terminated with error: %v", ta.PID(), err)
		}
	}

	return ta
}

// Call makes a synchronous call to the actor
func (ta *TestActor) Call(from gen.PID, request any) *CallResult {
	ta.t.Helper()

	// Don't process calls if actor is already terminated
	if ta.terminated {
		ta.t.Logf("Attempted to call terminated actor %s with request %v (reason: %v)", ta.PID(), request, ta.terminationReason)
		return &CallResult{
			Request:  request,
			Response: nil,
			Error:    ta.terminationReason,
			Ref:      gen.Ref{},
			From:     from,
			actor:    ta,
		}
	}

	ref := makeTestRefWithCreation(ta.node.Name(), ta.node.Creation())

	// Create a mailbox message for proper call routing
	mailboxMessage := gen.TakeMailboxMessage()
	defer gen.ReleaseMailboxMessage(mailboxMessage)

	mailboxMessage.From = from
	mailboxMessage.Type = gen.MailboxMessageTypeRequest
	mailboxMessage.Target = ta.process.PID()
	mailboxMessage.Message = request
	mailboxMessage.Ref = ref

	// Put the request into the main mailbox queue
	mailbox := ta.process.Mailbox()
	mailbox.Main.Push(mailboxMessage)

	// Let the behavior process its mailbox by calling ProcessRun
	var callError error
	if err := ta.behavior.ProcessRun(); err != nil {
		// Actor returned an error - it should terminate
		ta.terminated = true
		ta.terminationReason = err
		callError = err

		// Emit termination event
		ta.events.Push(TerminateEvent{
			PID:    ta.PID(),
			Reason: err,
		})

		// Only log non-normal termination for debugging
		if err != gen.TerminateReasonNormal && err != gen.TerminateReasonShutdown {
			ta.t.Logf("Actor %s terminated during call with error: %v", ta.PID(), err)
		}
	}

	// Check if we got an async response via SendResponse events
	response, responseError := ta.findResponseForRef(ref)

	// If we have a process error, use that; otherwise use response error
	finalError := callError
	if finalError == nil {
		finalError = responseError
	}

	return &CallResult{
		Request:  request,
		Response: response,
		Error:    finalError,
		Ref:      ref,
		From:     from,
		actor:    ta,
	}
}

// findResponseForRef looks for SendResponse or SendResponseError events matching the ref
func (ta *TestActor) findResponseForRef(ref gen.Ref) (any, error) {
	events := ta.Events()
	for _, event := range events {
		if responseEvent, ok := event.(SendResponseEvent); ok {
			if responseEvent.Ref == ref {
				return responseEvent.Response, nil
			}
		}
		if errorEvent, ok := event.(SendResponseErrorEvent); ok {
			if errorEvent.Ref == ref {
				return nil, errorEvent.Error
			}
		}
	}
	return nil, nil // No response found yet (async response may come later)
}

// CallResult wraps the result of a call operation
type CallResult struct {
	Request  any
	Response any
	Error    error
	Ref      gen.Ref
	From     gen.PID
	actor    *TestActor
}

// GetAsyncResponse waits for and returns the async response from SendResponse events
func (cr *CallResult) GetAsyncResponse() (any, error) {
	return cr.actor.findResponseForRef(cr.Ref)
}

// Behavior returns the underlying actor behavior for direct access
func (ta *TestActor) Behavior() gen.ProcessBehavior {
	return ta.behavior
}

// Process returns the test process for accessing process methods
func (ta *TestActor) Process() *TestProcess {
	return ta.process
}

// Node returns the test node for accessing node methods
func (ta *TestActor) Node() gen.Node {
	return ta.node
}

// PID returns the PID of the test actor
func (ta *TestActor) PID() gen.PID {
	return ta.process.PID()
}

// Capture stores a value with the given name for later retrieval
func (ta *TestActor) Capture(name string, value any) *TestActor {
	ta.captures[name] = value
	return ta
}

// Retrieved gets a previously captured value
func (ta *TestActor) Retrieved(name string) any {
	return ta.captures[name]
}

// Events returns all captured events for manual inspection
func (ta *TestActor) Events() []Event {
	// First, collect any new events from the queue
	for {
		event, ok := ta.events.Pop()
		if !ok {
			break
		}
		ta.storedEvents = append(ta.storedEvents, event.(Event))
	}

	// Return a copy of stored events
	result := make([]Event, len(ta.storedEvents))
	copy(result, ta.storedEvents)
	return result
}

// ClearEvents removes all captured events
func (ta *TestActor) ClearEvents() *TestActor {
	// Clear the queue
	for {
		_, ok := ta.events.Pop()
		if !ok {
			break
		}
	}
	// Clear stored events
	ta.storedEvents = ta.storedEvents[:0]
	return ta
}

// EventCount returns the number of captured events
func (ta *TestActor) EventCount() int {
	// Make sure we've collected all events first
	ta.Events()
	return len(ta.storedEvents)
}

// LastEvent returns the most recent event
func (ta *TestActor) LastEvent() Event {
	events := ta.Events()
	if len(events) == 0 {
		return nil
	}
	return events[len(events)-1]
}

// Fluent assertion methods
func (ta *TestActor) ShouldSend() *SendAssertion {
	return &SendAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

func (ta *TestActor) ShouldNotSend() *SendAssertion {
	return &SendAssertion{
		actor:    ta,
		expected: false,
	}
}

func (ta *TestActor) ShouldSpawn() *SpawnAssertion {
	return &SpawnAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

func (ta *TestActor) ShouldSpawnMeta() *SpawnMetaAssertion {
	return &SpawnMetaAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

func (ta *TestActor) ShouldLog() *LogAssertion {
	return &LogAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

func (ta *TestActor) ShouldCall() *CallAssertion {
	return &CallAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

// Cron assertion methods

// ShouldAddCronJob starts a cron job add assertion
func (ta *TestActor) ShouldAddCronJob() *CronJobAssertion {
	return &CronJobAssertion{
		actor:    ta,
		expected: true,
		count:    1,
		action:   "add",
	}
}

// ShouldRemoveCronJob starts a cron job remove assertion
func (ta *TestActor) ShouldRemoveCronJob() *CronJobAssertion {
	return &CronJobAssertion{
		actor:    ta,
		expected: true,
		count:    1,
		action:   "remove",
	}
}

// ShouldEnableCronJob starts a cron job enable assertion
func (ta *TestActor) ShouldEnableCronJob() *CronJobAssertion {
	return &CronJobAssertion{
		actor:    ta,
		expected: true,
		count:    1,
		action:   "enable",
	}
}

// ShouldDisableCronJob starts a cron job disable assertion
func (ta *TestActor) ShouldDisableCronJob() *CronJobAssertion {
	return &CronJobAssertion{
		actor:    ta,
		expected: true,
		count:    1,
		action:   "disable",
	}
}

// ShouldExecuteCronJob starts a cron job execution assertion
func (ta *TestActor) ShouldExecuteCronJob() *CronJobExecutionAssertion {
	return &CronJobExecutionAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

// Cron helper methods

// TriggerCronJob manually triggers a cron job for testing
func (ta *TestActor) TriggerCronJob(name gen.Atom) error {
	testCron := ta.node.cron
	return testCron.TriggerJob(name)
}

// SetCronMockTime sets mock time for cron testing
func (ta *TestActor) SetCronMockTime(t time.Time) {
	testCron := ta.node.cron
	testCron.SetMockTime(t)
}

// Built-in assertion functions (zero dependencies)
func Equal(t testing.TB, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		msg := ""
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
		t.Errorf("Expected %v, got %v. %s", expected, actual, msg)
	}
}

func NotEqual(t testing.TB, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	if reflect.DeepEqual(expected, actual) {
		msg := ""
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
		t.Errorf("Expected %v to not equal %v. %s", expected, actual, msg)
	}
}

func True(t testing.TB, condition bool, msgAndArgs ...any) {
	t.Helper()
	if !condition {
		msg := "Expected condition to be true"
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
		t.Error(msg)
	}
}

func False(t testing.TB, condition bool, msgAndArgs ...any) {
	t.Helper()
	if condition {
		msg := "Expected condition to be false"
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
		t.Error(msg)
	}
}

func Nil(t testing.TB, value any, msgAndArgs ...any) {
	t.Helper()
	if value != nil {
		msg := ""
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
		t.Errorf("Expected nil, got %v. %s", value, msg)
	}
}

func NotNil(t testing.TB, value any, msgAndArgs ...any) {
	t.Helper()
	if value == nil {
		msg := "Expected value to not be nil"
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
		t.Error(msg)
	}
}

func Contains(t testing.TB, haystack, needle string, msgAndArgs ...any) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		msg := ""
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
		t.Errorf("Expected %q to contain %q. %s", haystack, needle, msg)
	}
}

func IsType(t testing.TB, expectedType, value any, msgAndArgs ...any) {
	t.Helper()
	if reflect.TypeOf(value) != reflect.TypeOf(expectedType) {
		msg := ""
		if len(msgAndArgs) > 0 {
			msg = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
		t.Errorf("Expected type %T, got %T. %s", expectedType, value, msg)
	}
}

// RemoteNode returns the TestNetwork to access remote node functionality
func (ta *TestActor) RemoteNode() *TestNetwork {
	return ta.node.network
}

// CreateRemoteNode creates a new remote node for testing
func (ta *TestActor) CreateRemoteNode(name gen.Atom, connected bool) *TestRemoteNode {
	return ta.node.network.AddRemoteNode(name, connected)
}

// GetRemoteNode gets an existing remote node or creates it if it doesn't exist
func (ta *TestActor) GetRemoteNode(name gen.Atom) (gen.RemoteNode, error) {
	return ta.node.network.GetNode(name)
}

// ConnectRemoteNode connects to a remote node (creates connection if needed)
func (ta *TestActor) ConnectRemoteNode(name gen.Atom) gen.RemoteNode {
	node, _ := ta.node.network.GetNode(name)
	return node
}

// DisconnectRemoteNode disconnects from a remote node
func (ta *TestActor) DisconnectRemoteNode(name gen.Atom) error {
	node, err := ta.node.network.Node(name)
	if err != nil {
		return err
	}
	node.Disconnect()
	return nil
}

// ListConnectedNodes returns all connected remote nodes
func (ta *TestActor) ListConnectedNodes() []gen.Atom {
	return ta.node.network.Nodes()
}

// ShouldSendResponse starts a response assertion
func (ta *TestActor) ShouldSendResponse() *SendResponseAssertion {
	return &SendResponseAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

// ShouldNotSendResponse starts a negative response assertion
func (ta *TestActor) ShouldNotSendResponse() *SendResponseAssertion {
	return &SendResponseAssertion{
		actor:    ta,
		expected: false,
	}
}

// ShouldSendResponseError starts a response error assertion
func (ta *TestActor) ShouldSendResponseError() *SendResponseErrorAssertion {
	return &SendResponseErrorAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

// ShouldNotSendResponseError starts a negative response error assertion
func (ta *TestActor) ShouldNotSendResponseError() *SendResponseErrorAssertion {
	return &SendResponseErrorAssertion{
		actor:    ta,
		expected: false,
	}
}

// Termination-related methods

// IsTerminated returns true if the actor has been terminated
func (ta *TestActor) IsTerminated() bool {
	return ta.terminated
}

// TerminationReason returns the reason for termination, or nil if not terminated
func (ta *TestActor) TerminationReason() error {
	return ta.terminationReason
}

// ShouldTerminate starts a termination assertion
func (ta *TestActor) ShouldTerminate() *TerminateAssertion {
	return &TerminateAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

// ShouldNotTerminate starts a negative termination assertion
func (ta *TestActor) ShouldNotTerminate() *TerminateAssertion {
	return &TerminateAssertion{
		actor:    ta,
		expected: false,
	}
}

// Exit assertion methods

// ShouldSendExit starts an exit assertion
func (ta *TestActor) ShouldSendExit() *ExitAssertion {
	return &ExitAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

// ShouldNotSendExit starts a negative exit assertion
func (ta *TestActor) ShouldNotSendExit() *ExitAssertion {
	return &ExitAssertion{
		actor:    ta,
		expected: false,
	}
}

// ShouldSendExitMeta starts an exit meta assertion
func (ta *TestActor) ShouldSendExitMeta() *ExitMetaAssertion {
	return &ExitMetaAssertion{
		actor:    ta,
		expected: true,
		count:    1,
	}
}

// ShouldNotSendExitMeta starts a negative exit meta assertion
func (ta *TestActor) ShouldNotSendExitMeta() *ExitMetaAssertion {
	return &ExitMetaAssertion{
		actor:    ta,
		expected: false,
	}
}
