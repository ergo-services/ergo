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

type MetaProcess interface {
	ID() Alias
	Parent() PID
	Send(to any, message any) error
	SendImportant(to any, message any) error
	SendWithPriority(to any, message any, priority MessagePriority) error
	Spawn(behavior MetaBehavior, options MetaOptions) (Alias, error)
	Env(name Env) (any, bool)
	EnvList() map[Env]any
	Log() Log
}

type MetaOptions struct {
	MailboxSize  int64
	SendPriority MessagePriority
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
