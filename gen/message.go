package gen

import "time"

// MessageDownPID
type MessageDownPID struct {
	PID    PID
	Reason error
}

// MessageDownProcessID
type MessageDownProcessID struct {
	ProcessID ProcessID
	Reason    error
}

// MessageDownAlias
type MessageDownAlias struct {
	Alias  Alias
	Reason error
}

// MessageDownEvent
type MessageDownEvent struct {
	Event  Event
	Reason error
}

// MessageDownNode
type MessageDownNode struct {
	Name Atom
}

// MessageDownProxy
type MessageDownProxy struct {
	Node   Atom
	Proxy  Atom
	Reason error
}

// MessageExitPID
type MessageExitPID struct {
	PID    PID
	Reason error
}

// MessageExitProcessID
type MessageExitProcessID struct {
	ProcessID ProcessID
	Reason    error
}

// MessageExitAlias
type MessageExitAlias struct {
	Alias  Alias
	Reason error
}

// MessageExitEvent
type MessageExitEvent struct {
	Event  Event
	Reason error
}

// MessageExitNode
type MessageExitNode struct {
	Name Atom
}

// MessageFallback is delivered to the fallback process instead of the message that
// did not fit the target mailbox.
type MessageFallback struct {
	// PID of the process whose mailbox is full
	PID PID
	// Tag set in the fallback options of that process
	Tag string
	// Target the sender addressed: PID, ProcessID or Alias
	Target any
	// Message that has not been delivered
	Message any
}

// MessageEventStart
type MessageEventStart struct {
	Name Atom
}

// MessageEventStop
type MessageEventStop struct {
	Name Atom
}

// MessageEvent
// MessageResponse is the answer to a request made with SendRequest: the result,
// the callee's error, or ErrTimeout. Label is what SendRequestWithLabel attached.
type MessageResponse struct {
	From   PID
	Ref    Ref
	Label  any
	Result any
	Error  error
}

type MessageEvent struct {
	Event     Event
	Timestamp int64
	Message   any
}

// MessageLog. No stack trace here: a logger captures it itself with
// runtime.Callers inside Log, which runs in the recover defer. See logger/sentry.
type MessageLog struct {
	Time   time.Time
	Level  LogLevel
	Source any // MessageLogProcess, MessageLogNode, MessageLogNetwork, MessageLogMeta
	Format string
	Args   []any
	Fields []LogField
}

// MessageLogProcess
type MessageLogProcess struct {
	Node     Atom
	PID      PID
	Name     Atom
	Behavior string
}

// MessageLogMeta
type MessageLogMeta struct {
	Node     Atom
	Parent   PID
	Meta     Alias
	Behavior string
}

// MessageLogNode
type MessageLogNode struct {
	Node     Atom
	Creation int64
}

type MessageLogNetwork struct {
	Node     Atom
	Peer     Atom
	Creation int64
}

// MessageLogApplication
type MessageLogApplication struct {
	Node     Atom
	Name     Atom
	Mode     ApplicationMode
	Behavior string
}

type MessageCron struct {
	Node Atom
	Job  Atom
	Time time.Time
}

type MessageCronFallback struct {
	Job  Atom
	Tag  string
	Time time.Time
	Err  error
}
