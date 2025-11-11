package gen

import (
	"fmt"
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
func (p MetaState) MarshalJSON() ([]byte, error) {
	return []byte("\"" + p.String() + "\""), nil
}

type MetaBehavior interface {
	Init(process MetaProcess) error
	Start() error
	HandleMessage(from PID, message any) error
	HandleCall(from PID, ref Ref, request any) (any, error)
	Terminate(reason error)

	HandleInspect(from PID, item ...string) map[string]string
}

// MetaProcess interface provides methods for meta process operations.
//
// Meta processes bridge the actor model with the synchronous world by using two goroutines:
// 1. Forever-running goroutine: Executes Start() method, typically blocked on sync operations
//    (HTTP server, TCP accept loop, blocking I/O, etc.)
// 2. Message-handling goroutine: Handles mailbox messages, created only when messages arrive
//    (same as regular process - sequential message handling)
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
	// Target can be: PID, ProcessID, Alias, Atom (process name), or string (process name).
	// Available in: Sleep, Running, Terminated states.
	// Sleep allowed because external code (HTTP/TCP handlers) calls from non-actor goroutines.
	Send(to any, message any) error

	// SendWithPriority sends an asynchronous message with the specified priority.
	// Available in: Sleep, Running, Terminated states.
	// Sleep allowed for external code integration.
	SendWithPriority(to any, message any, priority MessagePriority) error

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
