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

// MessageFallback
type MessageFallback struct {
	PID     PID
	Tag     string
	Target  any
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
type MessageEvent struct {
	Event     Event
	Timestamp int64
	Message   any
}

// MessageLog
type MessageLog struct {
	Time   time.Time
	Level  LogLevel
	Source any // MessageLogProcess, MessageLogNode, MessageLogNetwork, MessageLogMeta
	Format string
	Args   []any
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
