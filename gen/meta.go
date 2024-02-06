package gen

import "errors"

type MetaHandleState int32

const (
	MetaHandleStateSleep      MetaHandleState = 1
	MetaHandleStateRunning    MetaHandleState = 2
	MetaHandleStateTerminated MetaHandleState = 4
)

var (
	TerminateMetaNormal error = errors.New("normal")
	TerminateMetaPanic  error = errors.New("meta panic")
)

type MetaBehavior interface {
	Init(process MetaProcess) error
	Start() error
	HandleMessage(from PID, message any) error
	HandleCall(from PID, ref Ref, request any) (any, error)
	Terminate(reason error)

	HandleInspect(from PID) map[string]string
}

type MetaProcess interface {
	ID() Alias
	Parent() PID
	Send(to any, message any) error
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
