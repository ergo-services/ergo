package gen

import (
	"context"
	"fmt"
	"time"

	"github.com/halturin/ergo/etf"
)

var (
	ErrUnsupportedRequest = fmt.Errorf("Unsupported request")
	ErrStop               = fmt.Errorf("stop")
)

type Process interface {
	Registrar
	Spawn(name string, opts ProcessOptions, object ProcessBehavior, args ...etf.Term) (Process, error)
	RemoteSpawn(node string, object string, opts RemoteSpawnOptions, args ...etf.Term) (etf.Pid, error)
	Name() string
	Info() ProcessInfo
	Self() etf.Pid
	Call(to interface{}, message etf.Term) (etf.Term, error)
	CallWithTimeout(to interface{}, message etf.Term, timeout int) (etf.Term, error)
	CallRPC(node, module, function string, args ...etf.Term) (etf.Term, error)
	CallRPCWithTimeout(timeout int, node, module, function string, args ...etf.Term) (etf.Term, error)
	Direct(request interface{}) (interface{}, error)
	DirectWithTimeout(request interface{}, timeout int) (interface{}, error)
	CastRPC(node, module, function string, args ...etf.Term)
	Send(to interface{}, message etf.Term) error
	SendAfter(to interface{}, message etf.Term, after time.Duration) context.CancelFunc
	CastAfter(to interface{}, message etf.Term, after time.Duration) context.CancelFunc
	Cast(to interface{}, message etf.Term) error
	// Exit initiate a graceful stopping process
	Exit(reason string) error
	// Kill immidiately stops process
	Kill()
	CreateAlias() (etf.Alias, error)
	DeleteAlias(alias etf.Alias) error
	ListEnv() map[string]interface{}
	SetEnv(name string, value interface{})
	Env(name string) interface{}
	Wait()
	WaitWithTimeout(d time.Duration) error
	Link(with etf.Pid)
	Unlink(with etf.Pid)
	IsAlive() bool
	SetTrapExit(trap bool)
	TrapExit() bool
	MonitorNode(name string) etf.Ref
	DemonitorNode(ref etf.Ref) bool
	MonitorProcess(process interface{}) etf.Ref
	DemonitorProcess(ref etf.Ref) bool
	Behavior() ProcessBehavior
	GroupLeader() Process
	Parent() Process
	Context() context.Context
	Children() []etf.Pid

	// Methods below are intended to be used for the ProcessBehavior implementation

	SendSyncRequestRaw(ref etf.Ref, node etf.Atom, messages ...etf.Term)
	PutSyncReply(ref etf.Ref, term etf.Term)
	SendSyncRequest(ref etf.Ref, to interface{}, message etf.Term)
	WaitSyncReply(ref etf.Ref, timeout int) (etf.Term, error)
	ProcessChannels() ProcessChannels
}

// ProcessInfo struct with process details
type ProcessInfo struct {
	PID             etf.Pid
	Name            string
	CurrentFunction string
	Status          string
	MessageQueueLen int
	Links           []etf.Pid
	Monitors        []etf.Pid
	MonitoredBy     []etf.Pid
	Aliases         []etf.Alias
	Dictionary      etf.Map
	TrapExit        bool
	GroupLeader     etf.Pid
	Reductions      uint64
}

type ProcessOptions struct {
	MailboxSize uint16
	GroupLeader Process
	Env         map[string]interface{}
}

// RemoteSpawnOptions defines options for RemoteSpawn method
type RemoteSpawnOptions struct {
	// RegisterName
	RegisterName string
	// Monitor enables monitor on the spawned process using provided reference
	Monitor etf.Ref
	// Link enables link between the calling and spawned processes
	Link bool
	// Function in order to support {M,F,A} request to the Erlang node
	Function string
	// Timeout
	Timeout int
}

type ProcessChannels struct {
	Mailbox      <-chan ProcessMailboxMessage
	Direct       <-chan ProcessDirectMessage
	GracefulExit <-chan ProcessGracefulExitRequest
}

type ProcessMailboxMessage struct {
	From    etf.Pid
	Message interface{}
}

type ProcessDirectMessage struct {
	Message interface{}
	Err     error
	Reply   chan ProcessDirectMessage
}

type ProcessGracefulExitRequest struct {
	From   etf.Pid
	Reason string
}

type ProcessState struct {
	Process
	State interface{}
}

// ProcessBehavior interface contains methods you should implement to make own process behaviour
type ProcessBehavior interface {
	ProcessInit(Process, ...etf.Term) (ProcessState, error)
	ProcessLoop(ProcessState, chan<- bool) string // method which implements control flow of process
}
type Registrar interface {
	Monitor
	NodeName() string
	NodeStop()
	ProcessByName(name string) Process
	ProcessByPid(pid etf.Pid) Process
	ProcessByAlias(alias etf.Alias) Process
	ProcessInfo(pid etf.Pid) (ProcessInfo, error)
	ProcessList() []Process
	MakeRef() etf.Ref
	RegisterName(name string, pid etf.Pid) error
	UnregisterName(name string)
	IsProcessAlive(process Process) bool

	RegisterBehavior(group, name string, behavior ProcessBehavior, data interface{}) error
	RegisteredBehavior(group, name string) (RegisteredBehavior, error)
	RegisteredBehaviorGroup(group string) []RegisteredBehavior
	UnregisterBehavior(group, name string) error

	Route(from etf.Pid, to etf.Term, message etf.Term)
	RouteRaw(nodename etf.Atom, messages ...etf.Term) error
}

type Monitor interface {
	Links(process etf.Pid) []etf.Pid
	Monitors(process etf.Pid) []etf.Pid
	MonitoredBy(process etf.Pid) []etf.Pid
}

type RegisteredBehavior struct {
	Behavior ProcessBehavior
	Data     interface{}
}

type DownMessage struct {
	Down   etf.Atom // = etf.Atom("DOWN")
	Ref    etf.Ref  // a monitor reference
	Type   etf.Atom // = etf.Atom("process")
	From   etf.Term // Pid or Name. Depends on how MonitorProcess was called - by name or by pid
	Reason string
}

// MessageExit delievers to Server's HandleInfo callback on enabled trap exit using SetTrapExit(true)
type MessageExit struct {
	From   etf.Pid
	Reason string
}

// RPC defines rpc function type
type RPC func(...etf.Term) etf.Term

// MessageManageRPC is using to manage RPC feature provides by "rex" process
type MessageManageRPC struct {
	Provide  bool
	Module   string
	Function string
	Fun      RPC
}

type MessageDirectChildren struct{}

func IsDownMessage(message etf.Term) (isTrue bool, d DownMessage) {
	// {DOWN, Ref, process, PidOrName, Reason}
	err := etf.TermIntoStruct(message, &d)
	if err != nil {
		return
	}
	if d.Down != etf.Atom("DOWN") {
		return
	}
	if d.Type != etf.Atom("process") {
		return
	}
	isTrue = true
	return
}
