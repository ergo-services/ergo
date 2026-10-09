package gen

import (
	"fmt"
	"time"
)

type MetaState int32

const (
	MetaStateSleep      MetaState = 1
	MetaStateRunning    MetaState = 2
	MetaStateTerminated MetaState = 4
)

func (p MetaState) String() string {
	switch p {
	case MetaStateSleep:
		return "sleep"
	case MetaStateRunning:
		return "running"
	case MetaStateTerminated:
		return "terminated"
	}
	return fmt.Sprintf("state#%d", int32(p))
}

type MetaBehavior interface {
	Init(process MetaProcess) error
	Start() error
	HandleMessage(from PID, message any) error
	HandleCall(from PID, ref Ref, request any) (any, error)
	// HandleResponse is invoked on the answer to a request made with SendRequest.
	// Non-nil value of the returning error terminates the meta process.
	HandleResponse(response MessageResponse) error
	Terminate(reason error)

	HandleInspect(from PID, item ...string) map[string]string
}

// MetaProcess interface provides methods for meta process operations.
//
// Meta processes bridge the actor model with the synchronous world by using two goroutines:
//  1. Forever-running goroutine: Executes Start() method, typically blocked on sync operations
//     (HTTP server, TCP accept loop, blocking I/O, etc.)
//  2. Message-handling goroutine: Handles mailbox messages, created only when messages arrive
//     (same as regular process - sequential message handling)
//
// This design allows:
// - One goroutine blocked on sync operations (Start() method)
// - Another goroutine handles async actor messages when needed
// - Bridging sync/blocking APIs with async actor model
//
// State-based access control:
// - Sleep state: Message handler idle, but Start() goroutine still running
// - Running state: Message handler active in HandleMessage()/HandleCall() callbacks
// - Terminated state: Both goroutines finished or finishing
//
// Send/Spawn can be called from Start() goroutine while meta in Sleep state,
// enabling integration with blocking I/O and external event sources.
type MetaProcess interface {
	// ID returns the alias identifying this meta process.
	// Available in all states.
	ID() Alias

	// Parent returns the PID of the parent process that spawned this meta.
	// Available in all states.
	Parent() PID

	// Send sends an asynchronous message to the target.
	// Target can be: PID, ProcessID, Alias, Atom (local registered name).
	// Any other type returns ErrUnsupported.
	// Available in: Sleep, Running, Terminated states.
	// Sleep allowed because external code (HTTP/TCP handlers) calls from non-actor goroutines.
	Send(to any, message any) error

	// SendWithPriority sends an asynchronous message with the specified priority.
	// Available in: Sleep, Running, Terminated states.
	// Sleep allowed for external code integration.
	SendWithPriority(to any, message any, priority MessagePriority) error

	// SendAfter sends the message to the target once the delay elapses. The cancel
	// function discards the scheduled send and reports false if it already fired.
	// Dropped if the meta process or its parent terminates before the timer expires.
	// Available in: Sleep, Running states. Returns ErrNotAllowed in Terminated state.
	SendAfter(to any, message any, after time.Duration) (CancelFunc, error)

	// SendWithPriorityAfter is SendAfter with the specified priority.
	SendWithPriorityAfter(to any, message any, priority MessagePriority, after time.Duration) (CancelFunc, error)

	// SendEvery sends the message to the target on every period, until the cancel
	// function is called or the meta process or its parent terminates.
	// Fixed rate: tick k is due at the scheduling moment plus k periods, so a late
	// tick does not shift the ones after it. Ticks delayed by more than a period
	// are dropped rather than delivered in a burst, as time.Ticker drops them.
	// Available in: Sleep, Running states. Returns ErrNotAllowed in Terminated state,
	// ErrIncorrect on a non-positive period.
	SendEvery(to any, message any, period time.Duration) (CancelFunc, error)

	// SendWithPriorityEvery is SendEvery with the specified priority. Fixed rate
	// too: the phase does not drift, and ticks missed by more than a period are
	// dropped.
	SendWithPriorityEvery(to any, message any, priority MessagePriority, period time.Duration) (CancelFunc, error)

	// SendRequest makes a request without blocking. The answer arrives in
	// HandleResponse, matched by the returned ref. One answer per request: the
	// result, the callee's error, or ErrTimeout - exactly one, unless the mailbox
	// is still full at the deadline.
	// An answer that finds the mailbox full is dropped and logged as an error,
	// and the request ends with ErrTimeout. If the mailbox is still full at the
	// deadline, ErrTimeout is dropped and logged the same way and nothing arrives.
	// The callee sees an ordinary Call made by the parent process.
	// Target can be: PID, ProcessID, Alias, Atom (local registered name).
	// A request to the parent process or to this meta process itself returns
	// ErrNotAllowed: the meta process speaks with the voice of its parent.
	// Available in: Sleep, Running states (Init too).
	// Returns ErrNotAllowed in Terminated state, ErrUnsupported on an unknown target type.
	SendRequest(to any, request any) (Ref, error)

	// SendRequestImportant is SendRequest with the important delivery flag, so an
	// undeliverable request answers with the delivery error at once instead of
	// waiting out the timeout.
	SendRequestImportant(to any, request any) (Ref, error)

	// SendRequestWithTimeout is SendRequest with the answer deadline in seconds.
	// Zero means DefaultRequestTimeout.
	SendRequestWithTimeout(to any, request any, timeout int) (Ref, error)

	// SendRequestWithLabel is SendRequest with a value of yours attached. The label
	// comes back in MessageResponse, so the answer carries its own context.
	SendRequestWithLabel(to any, request any, label any) (Ref, error)

	// SendRequestWithOptions is SendRequest with every knob at once.
	SendRequestWithOptions(to any, request any, options RequestOptions) (Ref, error)

	// CancelRequest drops a request made with SendRequest: no answer is delivered
	// for it any more, and a late one is discarded.
	// Available in all states.
	// Returns ErrUnknown if there is no such request.
	CancelRequest(ref Ref) error

	// SendResponse sends a response to a Call request.
	// Used in HandleCall() to respond to synchronous requests.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	SendResponse(to PID, ref Ref, message any) error

	// SendResponseError sends an error response to a Call request.
	// Used in HandleCall() to respond with an error.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	SendResponseError(to PID, ref Ref, err error) error

	// Spawn creates a child meta process.
	// Available in: Sleep, Running states.
	// Sleep allowed because external code (TCP accept) spawns connections.
	// Returns ErrNotAllowed in Terminated state.
	Spawn(behavior MetaBehavior, options MetaOptions) (Alias, error)

	// SendPriority returns the default priority for sending messages.
	// Available in all states.
	SendPriority() MessagePriority

	// SetSendPriority sets the default priority for sending messages.
	// Use MessagePriorityNormal (default), MessagePriorityHigh, or MessagePriorityMax.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	SetSendPriority(priority MessagePriority) error

	// Env returns the value associated with the given environment variable name.
	// Returns (value, true) if found, (nil, false) if not found.
	// Inherits from parent process environment.
	// Available in all states.
	Env(name Env) (any, bool)

	// EnvList returns a map of environment variables.
	// Includes variables from parent process.
	// Available in all states.
	EnvList() map[Env]any

	// EnvDefault returns the value associated with the given environment variable name,
	// or the default value if the variable is not set.
	// Available in all states.
	EnvDefault(name Env, def any) any

	// Log returns the logger interface for this meta process.
	// Available in all states.
	Log() Log

	// Compression returns true if compression is enabled for this meta process.
	// Available in all states.
	Compression() bool

	// SetCompression enables or disables compression for messages sent by this meta.
	// Available in: Running state only.
	// Returns ErrNotAllowed in other states.
	SetCompression(enabled bool) error
}

type MetaOptions struct {
	MailboxSize  int64
	SendPriority MessagePriority
	Compression  bool
	LogLevel     LogLevel
}

// MetaInfo
type MetaInfo struct {
	ID              Alias
	Parent          PID
	Application     Atom
	Behavior        string
	MailboxSize     int64
	MailboxQueues   MailboxQueues
	MessagePriority MessagePriority
	MessagesIn      uint64
	MessagesOut     uint64
	LogLevel        LogLevel
	Uptime          int64
	State           MetaState
}
