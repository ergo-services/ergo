package unit

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// TestProcess implements gen.Process for testing
type TestProcess struct {
	t        testing.TB
	events   lib.QueueMPSC
	node     *TestNode
	log      *TestLog
	options  TestOptions
	pid      gen.PID
	nextID   uint64
	env      map[gen.Env]any
	name     gen.Atom
	state    gen.ProcessState
	behavior gen.ProcessBehavior
	mailbox  gen.ProcessMailbox
	Failures
}

// NewTestProcess creates a new test process instance
func NewTestProcess(t testing.TB, events lib.QueueMPSC, node *TestNode, options TestOptions) *TestProcess {
	pid := gen.PID{
		Node:     options.NodeName,
		ID:       1000,
		Creation: options.NodeCreation,
	}

	if options.Env == nil {
		options.Env = make(map[gen.Env]any)
	}

	tp := &TestProcess{
		t:        t,
		events:   events,
		node:     node,
		options:  options,
		pid:      pid,
		nextID:   1001,
		env:      options.Env,
		name:     options.Register,
		state:    gen.ProcessStateRunning,
		Failures: newFailures(events, "process"),
		mailbox: gen.ProcessMailbox{
			Main:   lib.NewQueueMPSC(),
			System: lib.NewQueueMPSC(),
			Urgent: lib.NewQueueMPSC(),
			Log:    lib.NewQueueMPSC(),
		},
	}

	tp.log = NewTestLog(t, events, options.LogLevel)
	return tp
}

// gen.Process interface implementation

func (tp *TestProcess) PID() gen.PID {
	return tp.pid
}

func (tp *TestProcess) Name() gen.Atom {
	return tp.name
}

func (tp *TestProcess) RegisterName(name gen.Atom) error {
	tp.events.Push(RegisterNameEvent{Name: name, PID: tp.pid})
	tp.name = name
	return nil
}

func (tp *TestProcess) UnregisterName() error {
	if tp.name != "" {
		tp.events.Push(UnregisterNameEvent{Name: tp.name})
		tp.name = ""
	}
	return nil
}

func (tp *TestProcess) Parent() gen.PID {
	return tp.options.Parent
}

func (tp *TestProcess) Leader() gen.PID {
	return tp.options.Leader
}

func (tp *TestProcess) Node() gen.Node {
	return tp.node
}

func (tp *TestProcess) Behavior() gen.ProcessBehavior {
	return tp.behavior
}

func (tp *TestProcess) State() gen.ProcessState {
	return tp.state
}

func (tp *TestProcess) Uptime() int64 {
	return 0
}

func (tp *TestProcess) Send(to any, message any) error {
	// Check for failure injection
	if err := tp.CheckMethodFailure("Send", to, message); err != nil {
		return err
	}

	tp.events.Push(SendEvent{
		From:     tp.pid,
		To:       to,
		Message:  message,
		Priority: tp.options.Priority,
	})
	return nil
}

func (tp *TestProcess) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	tp.events.Push(SendEvent{
		From:     tp.pid,
		To:       to,
		Message:  message,
		Priority: priority,
	})
	return nil
}

func (tp *TestProcess) SendImportant(to any, message any) error {
	tp.events.Push(SendEvent{
		From:      tp.pid,
		To:        to,
		Message:   message,
		Priority:  tp.options.Priority,
		Important: true,
	})
	return nil
}

func (tp *TestProcess) SendAfter(to any, message any, after time.Duration) (gen.CancelFunc, error) {
	tp.events.Push(SendEvent{
		From:     tp.pid,
		To:       to,
		Message:  message,
		Priority: tp.options.Priority,
		After:    after,
	})
	return gen.CancelFunc(func() bool { return true }), nil
}

func (tp *TestProcess) SendResponse(to gen.PID, ref gen.Ref, response any) error {
	tp.events.Push(SendResponseEvent{
		From:     tp.pid,
		To:       to,
		Response: response,
		Ref:      ref,
		Priority: tp.options.Priority,
	})
	return nil
}

func (tp *TestProcess) SendResponseError(to gen.PID, ref gen.Ref, err error) error {
	tp.events.Push(SendResponseErrorEvent{
		From:     tp.pid,
		To:       to,
		Error:    err,
		Ref:      ref,
		Priority: tp.options.Priority,
	})
	return nil
}

func (tp *TestProcess) Call(to any, request any) (any, error) {
	event := CallEvent{
		From:    tp.pid,
		To:      to,
		Request: request,
	}
	tp.events.Push(event)
	return nil, nil
}

func (tp *TestProcess) CallWithTimeout(to any, request any, timeout int) (any, error) {
	event := CallEvent{
		From:    tp.pid,
		To:      to,
		Request: request,
		Timeout: timeout,
	}
	tp.events.Push(event)
	return nil, nil
}

func (tp *TestProcess) CallWithPriority(to any, request any, priority gen.MessagePriority) (any, error) {
	event := CallEvent{
		From:    tp.pid,
		To:      to,
		Request: request,
	}
	tp.events.Push(event)
	return nil, nil
}

func (tp *TestProcess) CallImportant(to any, request any) (any, error) {
	event := CallEvent{
		From:    tp.pid,
		To:      to,
		Request: request,
	}
	tp.events.Push(event)
	return nil, nil
}

func (tp *TestProcess) CallPID(to gen.PID, request any, timeout int) (any, error) {
	event := CallEvent{
		From:    tp.pid,
		To:      to,
		Request: request,
		Timeout: timeout,
	}
	tp.events.Push(event)
	return nil, nil
}

func (tp *TestProcess) CallProcessID(to gen.ProcessID, request any, timeout int) (any, error) {
	event := CallEvent{
		From:    tp.pid,
		To:      to,
		Request: request,
		Timeout: timeout,
	}
	tp.events.Push(event)
	return nil, nil
}

func (tp *TestProcess) CallAlias(to gen.Alias, request any, timeout int) (any, error) {
	event := CallEvent{
		From:    tp.pid,
		To:      to,
		Request: request,
		Timeout: timeout,
	}
	tp.events.Push(event)
	return nil, nil
}

func (tp *TestProcess) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	// Check for failure injection
	if err := tp.CheckMethodFailure("Spawn", factory, options, args); err != nil {
		return gen.PID{}, err
	}

	tp.nextID++
	result := gen.PID{
		Node:     tp.pid.Node,
		ID:       tp.nextID,
		Creation: tp.pid.Creation,
	}

	tp.events.Push(SpawnEvent{
		Factory: factory,
		Options: options,
		Args:    args,
		Result:  result,
	})

	return result, nil
}

func (tp *TestProcess) SpawnMeta(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	tp.nextID++
	result := gen.Alias{
		Node:     tp.pid.Node,
		ID:       [3]uint64{tp.nextID, 0, 0},
		Creation: tp.pid.Creation,
	}

	tp.events.Push(SpawnMetaEvent{
		Behavior: behavior,
		Options:  options,
		Result:   result,
	})

	return result, nil
}

func (tp *TestProcess) SendExit(to gen.PID, reason error) error {
	tp.events.Push(ExitEvent{
		To:     to,
		Reason: reason,
	})
	return nil
}

func (tp *TestProcess) SendExitMeta(to gen.Alias, reason error) error {
	tp.events.Push(ExitMetaEvent{
		Meta:   to,
		Reason: reason,
	})
	return nil
}

func (tp *TestProcess) CreateAlias() (gen.Alias, error) {
	tp.nextID++
	result := gen.Alias{
		Node:     tp.pid.Node,
		ID:       [3]uint64{tp.nextID, 0, 0},
		Creation: tp.pid.Creation,
	}

	tp.events.Push(AliasEvent{
		Result: result,
	})

	return result, nil
}

func (tp *TestProcess) DeleteAlias(alias gen.Alias) error {
	// No event needed for delete
	return nil
}

func (tp *TestProcess) Aliases() []gen.Alias {
	return []gen.Alias{}
}

// Linking methods
func (tp *TestProcess) Link(target any) error {
	tp.events.Push(LinkEvent{Target: target})
	return nil
}

func (tp *TestProcess) Unlink(target any) error {
	tp.events.Push(UnlinkEvent{Target: target})
	return nil
}

func (tp *TestProcess) LinkPID(target gen.PID) error {
	return tp.Link(target)
}

func (tp *TestProcess) UnlinkPID(target gen.PID) error {
	return tp.Unlink(target)
}

func (tp *TestProcess) LinkProcessID(target gen.ProcessID) error {
	return tp.Link(target)
}

func (tp *TestProcess) UnlinkProcessID(target gen.ProcessID) error {
	return tp.Unlink(target)
}

func (tp *TestProcess) LinkAlias(target gen.Alias) error {
	return tp.Link(target)
}

func (tp *TestProcess) UnlinkAlias(target gen.Alias) error {
	return tp.Unlink(target)
}

func (tp *TestProcess) LinkEvent(target gen.Event) ([]gen.MessageEvent, error) {
	tp.Link(target)
	return []gen.MessageEvent{}, nil
}

func (tp *TestProcess) UnlinkEvent(target gen.Event) error {
	return tp.Unlink(target)
}

func (tp *TestProcess) LinkNode(target gen.Atom) error {
	return tp.Link(target)
}

func (tp *TestProcess) UnlinkNode(target gen.Atom) error {
	return tp.Unlink(target)
}

// Monitoring methods
func (tp *TestProcess) Monitor(target any) error {
	tp.events.Push(MonitorEvent{Target: target})
	return nil
}

func (tp *TestProcess) Demonitor(target any) error {
	tp.events.Push(DemonitorEvent{Target: target})
	return nil
}

func (tp *TestProcess) MonitorPID(pid gen.PID) error {
	return tp.Monitor(pid)
}

func (tp *TestProcess) DemonitorPID(pid gen.PID) error {
	return tp.Demonitor(pid)
}

func (tp *TestProcess) MonitorProcessID(process gen.ProcessID) error {
	return tp.Monitor(process)
}

func (tp *TestProcess) DemonitorProcessID(process gen.ProcessID) error {
	return tp.Demonitor(process)
}

func (tp *TestProcess) MonitorAlias(alias gen.Alias) error {
	return tp.Monitor(alias)
}

func (tp *TestProcess) DemonitorAlias(alias gen.Alias) error {
	return tp.Demonitor(alias)
}

func (tp *TestProcess) MonitorEvent(event gen.Event) ([]gen.MessageEvent, error) {
	tp.Monitor(event)
	return []gen.MessageEvent{}, nil
}

func (tp *TestProcess) DemonitorEvent(event gen.Event) error {
	return tp.Demonitor(event)
}

func (tp *TestProcess) MonitorNode(node gen.Atom) error {
	return tp.Monitor(node)
}

func (tp *TestProcess) DemonitorNode(node gen.Atom) error {
	return tp.Demonitor(node)
}

// Event methods
func (tp *TestProcess) RegisterEvent(name gen.Atom, options gen.EventOptions) (gen.Ref, error) {
	ref := makeTestRefWithCreation(tp.pid.Node, tp.pid.Creation)
	tp.events.Push(RegisterEvent{
		Name:    name,
		Options: options,
		Result:  ref,
	})
	return ref, nil
}

func (tp *TestProcess) UnregisterEvent(name gen.Atom) error {
	tp.events.Push(UnregisterNameEvent{Name: name})
	return nil
}

func (tp *TestProcess) SendEvent(name gen.Atom, token gen.Ref, message any) error {
	tp.events.Push(SendEventEvent{
		Name:    name,
		Token:   token,
		Message: message,
	})
	return nil
}

func (tp *TestProcess) SendEventWithOptions(
	name gen.Atom,
	token gen.Ref,
	options gen.MessageOptions,
	message any,
) error {
	tp.events.Push(SendEventEvent{
		Name:    name,
		Token:   token,
		Message: message,
		Options: options,
	})
	return nil
}

// Environment methods
func (tp *TestProcess) Env(name gen.Env) (any, bool) {
	value, found := tp.env[name]
	return value, found
}

func (tp *TestProcess) EnvDefault(name gen.Env, def any) any {
	if value, found := tp.env[name]; found {
		return value
	}
	return def
}

func (tp *TestProcess) EnvList() map[gen.Env]any {
	result := make(map[gen.Env]any)
	for k, v := range tp.env {
		result[k] = v
	}
	return result
}

func (tp *TestProcess) SetEnv(name gen.Env, value any) {
	if tp.env == nil {
		tp.env = make(map[gen.Env]any)
	}
	tp.env[name] = value
}

// Log method
func (tp *TestProcess) Log() gen.Log {
	return tp.log
}

// Priority methods
func (tp *TestProcess) SetSendPriority(priority gen.MessagePriority) error {
	tp.options.Priority = priority
	return nil
}

func (tp *TestProcess) SendPriority() gen.MessagePriority {
	return tp.options.Priority
}

func (tp *TestProcess) SetImportantDelivery(important bool) error {
	tp.options.ImportantDelivery = important
	return nil
}

func (tp *TestProcess) ImportantDelivery() bool {
	return tp.options.ImportantDelivery
}

// Compression methods
func (tp *TestProcess) Compression() bool {
	return false // Default to false for testing
}

func (tp *TestProcess) SetCompression(enabled bool) error {
	// No-op for testing
	return nil
}

func (tp *TestProcess) CompressionType() gen.CompressionType {
	return gen.CompressionTypeGZIP // Default
}

func (tp *TestProcess) SetCompressionType(ctype gen.CompressionType) error {
	// No-op for testing
	return nil
}

func (tp *TestProcess) CompressionLevel() gen.CompressionLevel {
	return gen.CompressionLevel(0) // Default
}

func (tp *TestProcess) SetCompressionLevel(level gen.CompressionLevel) error {
	// No-op for testing
	return nil
}

func (tp *TestProcess) CompressionThreshold() int {
	return 1024 // Default threshold
}

func (tp *TestProcess) SetCompressionThreshold(threshold int) error {
	// No-op for testing
	return nil
}

func (tp *TestProcess) SetKeepNetworkOrder(order bool) error {
	// No-op for testing
	return nil
}

func (tp *TestProcess) KeepNetworkOrder() bool {
	return true // Default to true
}

// Missing PID methods
func (tp *TestProcess) SendPID(to gen.PID, message any) error {
	return tp.Send(to, message)
}

func (tp *TestProcess) SendProcessID(to gen.ProcessID, message any) error {
	return tp.Send(to, message)
}

func (tp *TestProcess) SendAlias(to gen.Alias, message any) error {
	return tp.Send(to, message)
}

// Missing spawn methods
func (tp *TestProcess) SpawnRegister(
	register gen.Atom,
	factory gen.ProcessFactory,
	options gen.ProcessOptions,
	args ...any,
) (gen.PID, error) {
	pid, err := tp.Spawn(factory, options, args...)
	if err != nil {
		return pid, err
	}
	// In testing, we'll just track the registration
	tp.events.Push(RegisterNameEvent{Name: register, PID: pid})
	return pid, nil
}

func (tp *TestProcess) RemoteSpawn(
	node gen.Atom,
	name gen.Atom,
	options gen.ProcessOptions,
	args ...any,
) (gen.PID, error) {
	tp.nextID++
	result := gen.PID{
		Node:     node,
		ID:       tp.nextID,
		Creation: tp.pid.Creation,
	}

	tp.events.Push(RemoteSpawnEvent{
		Node:    node,
		Name:    name,
		Options: options,
		Args:    args,
		Result:  result,
	})

	return result, nil
}

func (tp *TestProcess) RemoteSpawnRegister(
	node gen.Atom,
	name gen.Atom,
	register gen.Atom,
	options gen.ProcessOptions,
	args ...any,
) (gen.PID, error) {
	tp.nextID++
	result := gen.PID{
		Node:     node,
		ID:       tp.nextID,
		Creation: tp.pid.Creation,
	}

	tp.events.Push(RemoteSpawnEvent{
		Node:     node,
		Name:     name,
		Register: register,
		Options:  options,
		Args:     args,
		Result:   result,
	})

	return result, nil
}

// Missing inspect method
func (tp *TestProcess) Inspect(target gen.PID, item ...string) (map[string]string, error) {
	// For testing, return empty result
	return map[string]string{}, nil
}

func (tp *TestProcess) InspectMeta(meta gen.Alias, item ...string) (map[string]string, error) {
	// For testing, return empty result
	return map[string]string{}, nil
}

// Missing Events method
func (tp *TestProcess) Events() []gen.Atom {
	return []gen.Atom{} // Return empty for testing
}

// Mailbox and process info methods
func (tp *TestProcess) Mailbox() gen.ProcessMailbox {
	return tp.mailbox
}

func (tp *TestProcess) Info() (gen.ProcessInfo, error) {
	return gen.ProcessInfo{
		PID:      tp.pid,
		Name:     tp.name,
		State:    tp.state,
		Behavior: reflect.TypeOf(tp).String(),
	}, nil
}

func (tp *TestProcess) MetaInfo(meta gen.Alias) (gen.MetaInfo, error) {
	return gen.MetaInfo{
		ID: meta,
	}, nil
}

func (tp *TestProcess) Forward(to gen.PID, message *gen.MailboxMessage, priority gen.MessagePriority) error {
	tp.events.Push(SendEvent{
		From:     tp.pid,
		To:       to,
		Message:  message,
		Priority: priority,
	})
	return nil
}

func (tp *TestProcess) ProcessInit(process gen.Process, args ...any) error {
	return fmt.Errorf("ProcessInit should not be called on TestProcess")
}

func (tp *TestProcess) ProcessTerminate(reason error) {
	tp.t.Fatalf("ProcessTerminate should not be called on TestProcess: %v", reason)
}
