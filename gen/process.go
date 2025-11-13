package gen

import (
	"errors"
	"fmt"
	"time"

	"ergo.services/ergo/lib"
)

// ProcessBehavior interface defines the lifecycle callbacks for actor-based processes.
//
// Implementation guidelines:
// - All callbacks execute in a single goroutine (actor model - sequential message handling)
// - NEVER spawn goroutines in callbacks (violates actor model, causes race conditions)
// - NEVER use blocking primitives (mutexes, channels, sync.WaitGroup) in callbacks
// - Use async message passing (Send) instead of synchronous calls between actors
//
// Lifecycle:
// 1. ProcessInit() - called once during process initialization (Init state)
// 2. ProcessRun() - called repeatedly to handle messages (Running state)
// 3. ProcessTerminate() - called once during shutdown (Terminated state)
type ProcessBehavior interface {
	// ProcessInit initializes the process.
	// Called once when process is created, before it's registered in the node.
	// Use this to set up initial state, spawn children, configure properties.
	// Process is in Init state - some operations restricted (Call, Link, Monitor).
	// Returning error prevents process registration and triggers immediate termination.
	ProcessInit(process Process, args ...any) error

	// ProcessRun handles the message processing loop.
	// Called by the framework when messages arrive. Should not be called directly.
	// Typically implemented by act.Actor or custom behavior that calls
	// process.Mailbox() and handles messages manually.
	// Process is in Running state during message handling.
	// Returning error terminates the process with that error as reason.
	ProcessRun() error

	// ProcessTerminate is called during process shutdown.
	// Use this for cleanup: closing resources, sending final messages, logging.
	// Process is in Terminated state - limited operations available (async Send only).
	// Can send cleanup messages but cannot Call, Link, or create resources.
	// This method should not block or panic.
	ProcessTerminate(reason error)
}

// ProcessFactory is a function that creates a new ProcessBehavior instance.
// Must return a new instance on each call (behaviors are not reusable).
// Used by Spawn() to create process instances.
type ProcessFactory func() ProcessBehavior

// CancelFunc is returned by timer-based operations (SendAfter, SendExitAfter).
// Call it to cancel the scheduled operation.
// Returns true if successfully cancelled, false if timer already expired.
type CancelFunc func() bool

// ProcessState represents the current state of a process in its lifecycle.
// States determine which Process interface methods are available.
type ProcessState int32

func (p ProcessState) String() string {
	switch p {
	case ProcessStateInit:
		return "init"
	case ProcessStateSleep:
		return "sleep"
	case ProcessStateRunning:
		return "running"
	case ProcessStateWaitResponse:
		return "wait response"
	case ProcessStateTerminated:
		return "terminated"
	case ProcessStateZombee:
		return "zombee"
	}
	return fmt.Sprintf("state#%d", int32(p))
}
func (p ProcessState) MarshalJSON() ([]byte, error) {
	return []byte("\"" + p.String() + "\""), nil
}

const (
	// ProcessStateInit indicates process is initializing in ProcessInit() callback.
	// Process is NOT yet registered in node - cannot be found by PID lookup.
	// Operations allowed: Spawn, Send (async), property setters.
	// Operations restricted: Call (sync), Link, Monitor, RegisterName.
	ProcessStateInit ProcessState = 1

	// ProcessStateSleep indicates process is idle, waiting for messages.
	// No user callback is running. Internal state - user code never executes in Sleep.
	// Framework uses this state between message handling cycles.
	ProcessStateSleep ProcessState = 2

	// ProcessStateRunning indicates process is handling messages.
	// User code executes in HandleMessage() or HandleCall() callbacks.
	// All Process interface methods are available in this state.
	ProcessStateRunning ProcessState = 4

	// ProcessStateWaitResponse indicates process is blocked in Call() waiting for response.
	// Actor goroutine is blocked in select{}. Internal state - user cannot call methods
	// while blocked (would require spawning goroutine, which violates actor model).
	ProcessStateWaitResponse ProcessState = 8

	// ProcessStateTerminated indicates process is terminating or terminated.
	// User code executes in ProcessTerminate() callback.
	// Operations allowed: Send (async), SendExit (cleanup messages).
	// Operations restricted: Call, Link, Monitor, Spawn, property setters.
	ProcessStateTerminated ProcessState = 16

	// ProcessStateZombee indicates process was killed while running.
	// Process is dead - all operations return ErrNotAllowed.
	// This is a terminal state with no recovery.
	ProcessStateZombee ProcessState = 32
)

var (
	// TerminateReasonNormal indicates normal process termination.
	// Return this error from HandleMessage()/HandleCall() to gracefully stop the process.
	// Returning nil keeps the process running, returning TerminateReasonNormal stops it.
	// This is the standard way to terminate a process from within its own logic.
	// Does not trigger error logging.
	TerminateReasonNormal error = errors.New("normal")

	// TerminateReasonKill indicates the process was forcefully killed.
	// Set when node.Kill(pid) is called or process receives a kill signal.
	// Process terminates immediately without graceful shutdown opportunity.
	// Triggers error logging.
	TerminateReasonKill error = errors.New("kill")

	// TerminateReasonPanic indicates the process terminated due to a panic.
	// Set when panic occurs in ProcessInit(), ProcessRun(), or message handlers.
	// Framework recovers the panic and terminates the process with this reason.
	// Triggers panic logging with stack trace.
	TerminateReasonPanic error = errors.New("panic")

	// TerminateReasonShutdown indicates the process terminated due to node shutdown.
	// Set when node.Stop() is called and all processes are gracefully terminated.
	// Process should clean up resources in ProcessTerminate() callback.
	// Does not trigger error logging (expected termination).
	TerminateReasonShutdown error = errors.New("shutdown")
)

// Process interface provides methods for actor-based process operations.
//
// State-based access control:
// - Init state: Process initializing in ProcessInit() callback, NOT yet registered in node
// - Running state: Process handling messages in HandleMessage()/HandleCall() callbacks
// - Terminated state: Process in ProcessTerminate() callback or finished
// - Sleep/WaitResponse/Zombee: Internal states, user code never executes in these states
//
// Methods have different availability based on process state to enforce actor model
// constraints and prevent operations on unregistered processes.
type Process interface {
	// Node returns the Node interface this process belongs to.
	// Available in all states.
	Node() Node

	// Name returns the registered name associated with this process.
	// Returns empty string if process has no registered name.
	// Available in all states.
	Name() Atom

	// PID returns the process identifier (PID) belonging to this process.
	// Available in all states.
	PID() PID

	// Leader returns the group leader process PID.
	// Usually points to the application or supervisor process, otherwise equals parent.
	// Available in all states.
	Leader() PID

	// Parent returns the parent process PID.
	// Available in all states.
	Parent() PID

	// Uptime returns process uptime in seconds since creation.
	// Available in all states.
	Uptime() int64

	// Spawn creates a child process. Terminating the parent process
	// doesn't cause terminating the child process unless ProcessOptions.LinkChild is enabled.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states.
	Spawn(factory ProcessFactory, options ProcessOptions, args ...any) (PID, error)

	// SpawnRegister creates a child process and registers it with the given name.
	// The child will be addressable via ProcessID{register, nodename}.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states.
	SpawnRegister(
		register Atom,
		factory ProcessFactory,
		options ProcessOptions,
		args ...any,
	) (PID, error)

	// SpawnMeta creates a meta process. The returned alias is associated with this process.
	// Other processes can send messages (using Send) or make requests (using Call) to this meta process.
	// Meta processes are lightweight and handle messages in their own goroutine.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states.
	SpawnMeta(behavior MetaBehavior, options MetaOptions) (Alias, error)

	// RemoteSpawn makes a request to a remote node to spawn a new process.
	// The process will be created on the remote node independently.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states.
	RemoteSpawn(node Atom, name Atom, options ProcessOptions, args ...any) (PID, error)
	// RemoteSpawnRegister makes a request to a remote node to spawn a new process
	// and register it there with the given registered name.
	// The spawned process will be addressable via ProcessID{register, remote_node}.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states.
	RemoteSpawnRegister(
		node Atom,
		name Atom,
		register Atom,
		options ProcessOptions,
		args ...any,
	) (PID, error)

	// State returns the current process state.
	// Returns ProcessStateInit during ProcessInit() callback,
	// ProcessStateRunning during HandleMessage()/HandleCall() callbacks,
	// ProcessStateTerminated during/after ProcessTerminate() callback,
	// or ProcessStateZombee if the process was killed.
	// Sleep and WaitResponse are internal states not directly observable by user code.
	// Available in all states.
	State() ProcessState

	// RegisterName associates a name with this process's PID.
	// After registration, this process can be addressed using ProcessID{name, nodename}.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (process must be registered in node first).
	// Returns ErrTaken if this process already has a registered name.
	RegisterName(name Atom) error

	// UnregisterName removes the name association from this process.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	UnregisterName() error

	// EnvList returns a map of configured environment variables.
	// Includes environment variables from GroupLeader, Parent, and Node,
	// overlapped by priority: Process > Parent > GroupLeader > Node.
	// Available in all states.
	EnvList() map[Env]any

	// SetEnv sets an environment variable with the given name.
	// Use nil value to remove the variable.
	// Available in: Init, Running states.
	SetEnv(name Env, value any)

	// Env returns the value associated with the given environment variable name.
	// Returns (value, true) if found, (nil, false) if not found.
	// Available in all states.
	Env(name Env) (any, bool)

	// EnvDefault returns the value associated with the given environment variable name,
	// or the default value if the variable is not set.
	// Available in all states.
	EnvDefault(name Env, def any) any

	// Compression returns true if compression is enabled for this process.
	// Available in all states.
	Compression() bool

	// SetCompression enables or disables compression for messages sent over the network.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states.
	SetCompression(enabled bool) error

	// CompressionType returns the compression type for this process.
	// Available in all states.
	CompressionType() CompressionType

	// SetCompressionType sets the compression type.
	// Use CompressionTypeGZIP (default), CompressionTypeZLIB, or CompressionTypeLZW.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states, ErrIncorrect for invalid type.
	SetCompressionType(ctype CompressionType) error

	// CompressionLevel returns the compression level for this process.
	// Available in all states.
	CompressionLevel() CompressionLevel

	// SetCompressionLevel sets the compression level.
	// Use CompressionBestSize, CompressionBestSpeed, or CompressionDefault.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states, ErrIncorrect for invalid level.
	SetCompressionLevel(level CompressionLevel) error

	// CompressionThreshold returns the minimum message size for compression.
	// Available in all states.
	CompressionThreshold() int

	// SetCompressionThreshold sets the minimum message size that triggers compression.
	// Value must be greater than or equal to DefaultCompressionThreshold (1024).
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states, ErrIncorrect if threshold too small.
	SetCompressionThreshold(threshold int) error

	// SendPriority returns the default priority for sending messages.
	// Available in all states.
	SendPriority() MessagePriority

	// SetSendPriority sets the default priority for sending messages.
	// Use MessagePriorityNormal (default), MessagePriorityHigh, or MessagePriorityMax.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states, ErrIncorrect for invalid priority.
	SetSendPriority(priority MessagePriority) error

	// SetKeepNetworkOrder enables or disables maintaining delivery order over the network.
	// Disabling this option can improve network performance in some cases.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states.
	SetKeepNetworkOrder(order bool) error

	// KeepNetworkOrder returns true if network message ordering is enabled.
	// Enabled by default.
	// Available in all states.
	KeepNetworkOrder() bool

	// SetImportantDelivery enables or disables the important delivery flag for all messages.
	// When enabled, remote nodes send delivery confirmation for Send operations.
	//
	// Provides network transparency:
	// - Local: immediate error if process missing or mailbox full
	// - Remote without Important: fire-and-forget, no error feedback
	// - Remote with Important: confirmation or error (ErrProcessUnknown, ErrProcessMailboxFull)
	//
	// Use Important to make remote message delivery behave like local delivery,
	// detecting missing processes and full mailboxes immediately instead of silently failing.
	//
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states.
	SetImportantDelivery(important bool) error

	// ImportantDelivery returns true if the important delivery flag is enabled.
	// Available in all states.
	ImportantDelivery() bool

	// CreateAlias creates a new alias associated with this process.
	// Other processes can send messages or make calls using this alias.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (process must be registered in node first).
	CreateAlias() (Alias, error)

	// DeleteAlias deletes the given alias.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	DeleteAlias(alias Alias) error

	// Aliases returns a list of aliases associated with this process.
	// Available in all states.
	Aliases() []Alias

	// Events returns a list of event names registered by this process.
	// Available in all states.
	Events() []Atom

	// Send sends an asynchronous message to the target.
	// Target can be: PID, ProcessID, Alias, Atom (process name), or string (process name).
	// Available in: Init, Running, Terminated states.
	// Returns ErrNotAllowed in other states.
	Send(to any, message any) error

	// SendPID sends an asynchronous message to the process identified by PID.
	// Available in: Init, Running, Terminated states.
	// Returns ErrNotAllowed in other states.
	SendPID(to PID, message any) error

	// SendProcessID sends an asynchronous message to the named process.
	// Available in: Init, Running, Terminated states.
	// Returns ErrNotAllowed in other states.
	SendProcessID(to ProcessID, message any) error

	// SendAlias sends an asynchronous message to the process via alias.
	// Available in: Init, Running, Terminated states.
	// Returns ErrNotAllowed in other states.
	SendAlias(to Alias, message any) error

	// SendWithPriority sends an asynchronous message with the specified priority.
	// Available in: Init, Running, Terminated states.
	// Returns ErrNotAllowed in other states.
	SendWithPriority(to any, message any, priority MessagePriority) error

	// SendImportant sends a message with important delivery flag.
	// The remote node sends confirmation when the message is delivered to the mailbox.
	//
	// Important delivery provides network transparency for error detection:
	// - Local delivery: immediate error if process doesn't exist or mailbox full
	// - Remote without Important: message sent, no confirmation (fire-and-forget)
	// - Remote with Important: confirmation or error (ErrProcessUnknown, ErrProcessMailboxFull)
	// Aligns remote behavior with local delivery semantics.
	//
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for confirmation routing),
	// ErrProcessUnknown if target doesn't exist, ErrProcessMailboxFull if mailbox full.
	SendImportant(to any, message any) error

	// SendAfter starts a timer. When the timer expires, sends the message to the target.
	// Returns a cancel function to discard the scheduled send.
	// CancelFunc returns false if the timer already expired and the message was sent.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states (creates background timer task).
	SendAfter(to any, message any, after time.Duration) (CancelFunc, error)

	// SendWithPriorityAfter starts a timer. When the timer expires, sends the message
	// to the target with the specified priority.
	// Returns a cancel function to discard the scheduled send.
	// CancelFunc returns false if the timer already expired and the message was sent.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states (creates background timer task).
	SendWithPriorityAfter(to any, message any, priority MessagePriority, after time.Duration) (CancelFunc, error)

	// SendEvent sends an event message to all subscribers (processes that linked or monitored this event).
	// The event must be registered first using RegisterEvent.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrEventUnknown if event not registered.
	SendEvent(name Atom, token Ref, message any) error

	// SendExit sends a graceful termination request to the target process.
	// The target process will receive the exit signal and terminate.
	// Available in: Init, Running, Terminated states.
	// Returns ErrNotAllowed in other states.
	SendExit(to PID, reason error) error

	// SendExitAfter starts a timer. When the timer expires, sends a graceful termination
	// request to the target process.
	// Returns a cancel function to discard the scheduled exit signal.
	// CancelFunc returns false if the timer already expired and the signal was sent.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states (creates background timer task).
	SendExitAfter(to PID, reason error, after time.Duration) (CancelFunc, error)

	// SendExitMeta sends a graceful termination request to the target meta process.
	// Available in: Init, Running, Terminated states.
	// Returns ErrNotAllowed in other states.
	SendExitMeta(meta Alias, reason error) error

	// SendExitMetaAfter starts a timer. When the timer expires, sends a graceful termination
	// request to the target meta process.
	// Returns a cancel function to discard the scheduled exit signal.
	// CancelFunc returns false if the timer already expired and the signal was sent.
	// Available in: Init, Running states.
	// Returns ErrNotAllowed in other states (creates background timer task).
	SendExitMetaAfter(meta Alias, reason error, after time.Duration) (CancelFunc, error)

	// SendResponse sends a response to a Call request.
	// Used in HandleCall() to respond to synchronous requests.
	//
	// If the process's ImportantDelivery flag is enabled (via SetImportantDelivery(true)),
	// this method behaves like SendResponseImportant - blocks until requester confirms receipt.
	// Otherwise, fire-and-forget delivery without waiting for confirmation.
	//
	// For explicit important delivery regardless of process flag, use SendResponseImportant.
	//
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrResponseIgnored if requester not waiting (with important),
	// ErrTimeout if confirmation not received (with important).
	SendResponse(to PID, ref Ref, message any) error

	// SendResponseImportant sends a response with delivery confirmation (2-phase commit).
	// Blocks until the requester confirms receipt or timeout occurs.
	//
	// Phase 1: Sends response with ImportantDelivery flag to requester
	// Phase 2: Waits for confirmation from requester (automatic, sent after receiving response)
	//
	// When used with CallImportant, creates 3-phase commit for the complete request-response cycle:
	// request delivery confirmation + response delivery + response confirmation.
	//
	// If the requester is not waiting (timed out or terminated), returns ErrResponseIgnored immediately.
	// If the requester receives the response but crashes before sending confirmation, returns ErrTimeout.
	// Uses DefaultRequestTimeout (5 seconds) for waiting for confirmation.
	//
	// Use this when the response represents critical state and you need guarantee the requester
	// actually received it. The responder can safely proceed knowing the requester has the response.
	//
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrResponseIgnored if requester not waiting,
	// ErrTimeout if confirmation not received within timeout.
	SendResponseImportant(to PID, ref Ref, message any) error

	// SendResponseError sends an error response to a Call request.
	// Used in HandleCall() to respond with an error.
	//
	// If the process's ImportantDelivery flag is enabled (via SetImportantDelivery(true)),
	// this method behaves like SendResponseErrorImportant - blocks until requester confirms receipt.
	// Otherwise, fire-and-forget delivery without waiting for confirmation.
	//
	// For explicit important delivery regardless of process flag, use SendResponseErrorImportant.
	//
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrResponseIgnored if requester not waiting (with important),
	// ErrTimeout if confirmation not received (with important).
	SendResponseError(to PID, ref Ref, err error) error

	// SendResponseErrorImportant sends an error response with delivery confirmation (2-phase commit).
	// Blocks until the requester confirms receipt or timeout occurs.
	//
	// Phase 1: Sends error response with ImportantDelivery flag to requester
	// Phase 2: Waits for confirmation from requester (automatic, sent after receiving response)
	//
	// When used with CallImportant, creates 3-phase commit for the complete request-response cycle:
	// request delivery confirmation + error response delivery + error response confirmation.
	//
	// If the requester is not waiting (timed out or terminated), returns ErrResponseIgnored immediately.
	// If the requester receives the error but crashes before sending confirmation, returns ErrTimeout.
	// Uses DefaultRequestTimeout (5 seconds) for waiting for confirmation.
	//
	// Use this when the error represents critical information (operation failed, rollback needed)
	// and you need guarantee the requester knows about the failure.
	//
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrResponseIgnored if requester not waiting,
	// ErrTimeout if confirmation not received within timeout.
	SendResponseErrorImportant(to PID, ref Ref, err error) error

	// Call makes a synchronous request with default timeout (5 seconds).
	// Blocks the actor goroutine until response arrives or timeout occurs.
	// Target can be: PID, ProcessID, Alias, Atom (process name), or string (process name).
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrTimeout on timeout.
	Call(to any, message any) (any, error)

	// CallWithTimeout makes a synchronous request with the specified timeout (in seconds).
	// Blocks the actor goroutine until response arrives or timeout occurs.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrTimeout on timeout.
	CallWithTimeout(to any, message any, timeout int) (any, error)

	// CallWithPriority makes a synchronous request with the specified priority.
	// Uses default timeout (5 seconds). Blocks the actor goroutine.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrTimeout on timeout.
	CallWithPriority(to any, message any, priority MessagePriority) (any, error)

	// CallImportant makes a synchronous request with important delivery flag.
	// Uses default timeout (5 seconds). Blocks the actor goroutine.
	//
	// Important delivery ensures request reaches the target process mailbox.
	// For remote processes, provides network transparency:
	// - Without Important: timeout if remote process doesn't exist (can't distinguish from slow response)
	// - With Important: immediate ErrProcessUnknown if remote process doesn't exist
	// Aligns remote behavior with local delivery (local always returns immediate error if process missing).
	//
	// When combined with SendResponseImportant, creates 3-phase commit:
	// Phase 1: Request delivery + confirmation
	// Phase 2: Response delivery (from SendResponseImportant)
	// Phase 3: Response confirmation (from SendResponseImportant)
	// Both request and response have guaranteed delivery.
	//
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrTimeout on timeout,
	// ErrProcessUnknown if target doesn't exist (with Important flag).
	CallImportant(to any, message any) (any, error)

	// CallPID makes a synchronous request to the process identified by PID.
	// Timeout specified in seconds. Blocks the actor goroutine.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrTimeout on timeout.
	CallPID(to PID, message any, timeout int) (any, error)

	// CallProcessID makes a synchronous request to the named process.
	// Timeout specified in seconds. Blocks the actor goroutine.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrTimeout on timeout.
	CallProcessID(to ProcessID, message any, timeout int) (any, error)

	// CallAlias makes a synchronous request to the process via alias.
	// Timeout specified in seconds. Blocks the actor goroutine.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrTimeout on timeout.
	CallAlias(to Alias, message any, timeout int) (any, error)

	// Inspect sends an inspection request to the target process.
	// Returns a map of inspection items. Synchronous operation.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	Inspect(target PID, item ...string) (map[string]string, error)

	// InspectMeta sends an inspection request to the target meta process.
	// Returns a map of inspection items. Synchronous operation.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	InspectMeta(meta Alias, item ...string) (map[string]string, error)

	// RegisterEvent registers a new event with this process as the producer.
	// Returns a reference token for sending events.
	// Other processes can subscribe via LinkEvent/MonitorEvent.
	// Only the producer can unregister the event.
	// Other processes can send events using the token (delegation feature).
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrTaken if event name already registered.
	RegisterEvent(name Atom, options EventOptions) (Ref, error)

	// UnregisterEvent unregisters an event.
	// Can only be called by the process that registered the event (the producer).
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states, ErrEventUnknown if event not found.
	UnregisterEvent(name Atom) error

	// Link creates a bidirectional link to the target.
	// If either process terminates, the other receives an exit message and terminates too.
	// Target can be: PID, ProcessID, Alias, Event, or Atom (node name).
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for exit message routing).
	Link(target any) error

	// Unlink removes a bidirectional link to the target.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	Unlink(target any) error

	// LinkPID creates a bidirectional link to the process identified by PID.
	// If either process terminates, the other receives an exit message.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for exit routing).
	LinkPID(target PID) error

	// UnlinkPID removes a bidirectional link to the process identified by PID.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	UnlinkPID(target PID) error

	// LinkProcessID creates a bidirectional link to the named process.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for exit routing).
	LinkProcessID(target ProcessID) error

	// UnlinkProcessID removes a bidirectional link to the named process.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	UnlinkProcessID(target ProcessID) error

	// LinkAlias creates a bidirectional link to the process via alias.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for exit routing).
	LinkAlias(target Alias) error

	// UnlinkAlias removes a bidirectional link to the process via alias.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	UnlinkAlias(target Alias) error

	// LinkEvent creates a bidirectional link to an event.
	// This process will receive event messages and exit if the event is unregistered.
	// Returns the last N event messages if buffering is enabled.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for exit routing).
	LinkEvent(target Event) ([]MessageEvent, error)

	// UnlinkEvent removes a bidirectional link to an event.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	UnlinkEvent(target Event) error

	// LinkNode creates a bidirectional link to a node.
	// If the node disconnects, this process receives an exit message.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for exit routing).
	LinkNode(target Atom) error

	// UnlinkNode removes a bidirectional link to a node.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	UnlinkNode(target Atom) error

	// Monitor creates a unidirectional monitor to the target.
	// If the target terminates, this process receives a down message (non-fatal).
	// Target can be: PID, ProcessID, Alias, Event, or Atom (node name).
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for down routing).
	Monitor(target any) error

	// Demonitor removes a unidirectional monitor to the target.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	Demonitor(target any) error

	// MonitorPID creates a unidirectional monitor to the process identified by PID.
	// If the target terminates, this process receives a down message.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for down routing).
	MonitorPID(pid PID) error

	// DemonitorPID removes a unidirectional monitor to the process identified by PID.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	DemonitorPID(pid PID) error

	// MonitorProcessID creates a unidirectional monitor to the named process.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for down routing).
	MonitorProcessID(process ProcessID) error

	// DemonitorProcessID removes a unidirectional monitor to the named process.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	DemonitorProcessID(process ProcessID) error

	// MonitorAlias creates a unidirectional monitor to the process via alias.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for down routing).
	MonitorAlias(alias Alias) error

	// DemonitorAlias removes a unidirectional monitor to the process via alias.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	DemonitorAlias(alias Alias) error

	// MonitorEvent creates a unidirectional monitor to an event.
	// This process will receive event messages and down notification if event is unregistered.
	// Returns the last N event messages if buffering is enabled.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for down routing).
	MonitorEvent(event Event) ([]MessageEvent, error)

	// DemonitorEvent removes a unidirectional monitor to an event.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	DemonitorEvent(event Event) error

	// MonitorNode creates a unidirectional monitor to a node.
	// If the node disconnects, this process receives a down message.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states (requires process registered for down routing).
	MonitorNode(node Atom) error

	// DemonitorNode removes a unidirectional monitor to a node.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	DemonitorNode(node Atom) error

	// Log returns the logger interface for this process.
	// Available in all states.
	Log() Log

	// Info returns summary information about this process.
	// Includes PID, name, state, behavior type, and other process details.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	Info() (ProcessInfo, error)

	// MetaInfo returns summary information about the given meta process.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	MetaInfo(meta Alias) (MetaInfo, error)

	// Low-level API (for gen.ProcessBehavior implementations)

	// Mailbox returns the process mailbox queues (Main, System, Urgent, Log).
	// Available in all states.
	Mailbox() ProcessMailbox

	// Behavior returns the ProcessBehavior implementation for this process.
	// Available in all states.
	Behavior() ProcessBehavior

	// Forward forwards a mailbox message to another process with the specified priority.
	// This is a low-level operation for custom message routing.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	Forward(to PID, message *MailboxMessage, priority MessagePriority) error
}

type MessagePriority int

func (mp MessagePriority) String() string {
	switch mp {
	case 0:
		return "normal"
	case 1:
		return "high"
	case 2:
		return "max"
	}
	return "undefined"
}

func (mp MessagePriority) MarshalJSON() ([]byte, error) {
	return []byte("\"" + mp.String() + "\""), nil
}

const MessagePriorityNormal MessagePriority = 0 // default
const MessagePriorityHigh MessagePriority = 1
const MessagePriorityMax MessagePriority = 2

type MessageOptions struct {
	Ref               Ref
	Priority          MessagePriority
	Compression       Compression
	KeepNetworkOrder  bool
	ImportantDelivery bool
}

// ProcessOptions defines configuration options for spawning a process.
type ProcessOptions struct {
	// MailboxSize defines the maximum length of the message queue.
	// Zero (default) means unlimited mailbox size.
	// When mailbox is full, senders receive ErrProcessMailboxFull or messages
	// are forwarded to Fallback process if configured.
	MailboxSize int64

	// Leader sets the group leader process PID.
	// If not specified, inherits from parent or defaults to parent PID.
	// The leader is typically the application or supervisor process.
	Leader PID

	// Env sets process-specific environment variables.
	// These variables are accessible via process.Env() and have highest priority,
	// overriding parent, leader, and node environment variables.
	Env map[Env]any

	// Compression configures compression settings for messages sent by this process.
	// Includes Enable flag, Type (GZIP/ZLIB/LZW), Level, and Threshold.
	Compression Compression

	// SendPriority sets the default priority for messages sent by this process.
	// Messages are delivered to receiver's mailbox queues in this order:
	//  - MessagePriorityMax → Mailbox.Urgent (highest priority)
	//  - MessagePriorityHigh → Mailbox.System
	//  - MessagePriorityNormal (default) → Mailbox.Main
	// Receiver processes mailbox in order: Urgent → System → Main.
	SendPriority MessagePriority

	// ImportantDelivery enables delivery confirmation for messages sent to remote nodes.
	// When enabled, remote node sends confirmation when message is delivered to mailbox.
	// Process will receive delivery confirmation or error response.
	ImportantDelivery bool

	// Fallback defines a process to receive forwarded messages when mailbox overflows.
	// Forwarded messages are wrapped in MessageFallback struct with the specified tag.
	// Only applies when MailboxSize is limited (non-zero).
	// Allows building backpressure handling and overflow protection.
	Fallback ProcessFallback

	// LinkParent creates a bidirectional link with the parent process on start.
	// If parent terminates, this process receives exit signal and terminates.
	// Ignored if process is spawned by node (not by another process).
	LinkParent bool

	// LinkChild creates a bidirectional link from parent to this child process.
	// If child terminates, parent receives exit signal and terminates.
	// Works even when parent is in Init state (uses manual targetManager.AddLink).
	// Ignored if process is spawned by node (not by another process).
	LinkChild bool

	// LogLevel sets the initial logging level for this process.
	// Default is LogLevelInfo. Can be changed later via process.Log().SetLevel().
	LogLevel LogLevel
}

type ProcessOptionsExtra struct {
	ProcessOptions

	ParentPID      PID
	ParentLeader   PID
	ParentEnv      map[Env]any
	ParentLogLevel LogLevel

	Register    Atom
	Application Atom

	Args []any
}

// ProcessInfo contains comprehensive information about a process.
// Retrieved via process.Info() or node.ProcessInfo(pid).
type ProcessInfo struct {
	// PID is the unique process identifier.
	PID PID

	// Name is the registered name associated with this process.
	// Empty if process has no registered name.
	Name Atom

	// Application is the application name if this process runs under an application.
	// Empty for standalone processes.
	Application Atom

	// Behavior is the type name of the ProcessBehavior implementation.
	Behavior string

	// MailboxSize is the maximum mailbox queue length.
	// Zero means unlimited.
	MailboxSize int64

	// MailboxQueues shows current message counts in each mailbox queue.
	MailboxQueues MailboxQueues

	// MessagesIn is the total number of messages this process received.
	MessagesIn uint64

	// MessagesOut is the total number of messages this process sent.
	MessagesOut uint64

	// RunningTime is the cumulative time spent in Running state (nanoseconds).
	RunningTime uint64

	// Compression contains the compression configuration for this process.
	Compression Compression

	// MessagePriority is the default priority for sending messages.
	MessagePriority MessagePriority

	// Uptime is the process uptime in seconds since creation.
	Uptime int64

	// State is the current process state (Init, Sleep, Running, WaitResponse, Terminated, Zombee).
	State ProcessState

	// Parent is the PID of the parent process that spawned this process.
	Parent PID

	// Leader is the group leader PID (typically supervisor or application).
	Leader PID

	// Fallback contains mailbox overflow handling configuration.
	Fallback ProcessFallback

	// Env contains process environment variables.
	// Only populated if NodeOptions.Security.ExposeEnvInfo is enabled.
	Env map[Env]any

	// Aliases is the list of aliases associated with this process.
	Aliases []Alias

	// Events is the list of events this process registered (as producer).
	Events []Atom

	// Metas is the list of meta processes spawned by this process.
	Metas []Alias

	// MonitorsPID is the list of processes monitored by PID.
	MonitorsPID []PID

	// MonitorsProcessID is the list of named processes being monitored.
	MonitorsProcessID []ProcessID

	// MonitorsAlias is the list of aliases being monitored.
	MonitorsAlias []Alias

	// MonitorsEvent is the list of events being monitored.
	MonitorsEvent []Event

	// MonitorsNode is the list of remote nodes being monitored.
	MonitorsNode []Atom

	// LinksPID is the list of processes linked by PID.
	LinksPID []PID

	// LinksProcessID is the list of named processes linked with.
	LinksProcessID []ProcessID

	// LinksAlias is the list of aliases linked with.
	LinksAlias []Alias

	// LinksEvent is the list of events linked with.
	LinksEvent []Event

	// LinksNode is the list of remote nodes linked with.
	LinksNode []Atom

	// LogLevel is the current logging level for this process.
	LogLevel LogLevel

	// KeepNetworkOrder indicates if network message ordering is enabled.
	KeepNetworkOrder bool

	// ImportantDelivery indicates if delivery confirmation is enabled.
	ImportantDelivery bool
}

// ProcessShortInfo contains essential information about a process.
// Lighter weight version of ProcessInfo without links/monitors/aliases.
// Retrieved via node.ProcessListShortInfo() or node.ApplicationProcessListShortInfo().
// Useful for listing many processes efficiently.
type ProcessShortInfo struct {
	// PID is the unique process identifier.
	PID PID

	// Name is the registered name associated with this process.
	// Empty if process has no registered name.
	Name Atom

	// Application is the application name if this process runs under an application.
	// Empty for standalone processes.
	Application Atom

	// Behavior is the type name of the ProcessBehavior implementation.
	Behavior string

	// MessagesIn is the total number of messages this process received.
	MessagesIn uint64

	// MessagesOut is the total number of messages this process sent.
	MessagesOut uint64

	// MessagesMailbox is the total number of messages currently in mailbox queues.
	MessagesMailbox uint64

	// RunningTime is the cumulative time spent in Running state (nanoseconds).
	RunningTime uint64

	// Uptime is the process uptime in seconds since creation.
	Uptime int64

	// State is the current process state (Init, Sleep, Running, WaitResponse, Terminated, Zombee).
	State ProcessState

	// Parent is the PID of the parent process that spawned this process.
	Parent PID

	// Leader is the group leader PID (typically supervisor or application).
	Leader PID

	// LogLevel is the current logging level for this process.
	LogLevel LogLevel
}

// ProcessFallback defines mailbox overflow handling configuration.
// When mailbox is full and Fallback is enabled, messages are forwarded
// to the fallback process instead of rejecting them with ErrProcessMailboxFull.
// Forwarded messages are wrapped in MessageFallback with the specified Tag.
type ProcessFallback struct {
	// Enable activates fallback message forwarding.
	Enable bool

	// Name is the registered process name to forward overflow messages to.
	Name Atom

	// Tag is a string identifier included in MessageFallback.
	// Allows the fallback process to identify the source of forwarded messages.
	Tag string
}

// ProcessMailbox contains the message queues for a process.
// Retrieved via process.Mailbox(). Low-level API for custom message handling.
//
// Process handles mailbox queues in priority order:
// 1. Urgent - highest priority messages (MessagePriorityMax)
// 2. System - high priority messages (MessagePriorityHigh)
// 3. Main - normal priority messages (MessagePriorityNormal, default)
// 4. Log - logging messages (MessageLogNode, MessageLogProcess)
//
// Each queue is MPSC (Multi-Producer Single-Consumer) - thread-safe for
// multiple senders, single reader (the process's actor goroutine).
type ProcessMailbox struct {
	// Main queue for normal priority messages (MessagePriorityNormal).
	// Default queue for most messages.
	Main lib.QueueMPSC

	// System queue for high priority messages (MessagePriorityHigh).
	// Processed before Main queue.
	System lib.QueueMPSC

	// Urgent queue for maximum priority messages (MessagePriorityMax).
	// Processed before System and Main queues.
	Urgent lib.QueueMPSC

	// Log queue for logging messages (MessageLogNode, MessageLogProcess).
	// Processed after all other queues.
	Log lib.QueueMPSC
}

// MailboxQueues contains message counts for each mailbox queue.
// Part of ProcessInfo and ProcessShortInfo.
// Represents a snapshot of mailbox load at the time of query.
// Use to monitor process load and detect potential bottlenecks.
type MailboxQueues struct {
	// Main is the number of normal priority messages in the Main queue.
	Main int64

	// System is the number of high priority messages in the System queue.
	System int64

	// Urgent is the number of maximum priority messages in the Urgent queue.
	Urgent int64

	// Log is the number of logging messages in the Log queue.
	Log int64
}
