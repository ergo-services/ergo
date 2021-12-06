package gen

import (
	"context"
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
)

var (
	ErrUnsupportedRequest = fmt.Errorf("Unsupported request")
	ErrServerTerminated   = fmt.Errorf("Server terminated")
)

// EnvKey
type EnvKey string

// Process
type Process interface {
	Core

	// Spawn create a new process with parent
	Spawn(name string, opts ProcessOptions, object ProcessBehavior, args ...etf.Term) (Process, error)

	// RemoteSpawn creates a new process at a remote node. The object name is a regitered
	// behavior on a remote name using RegisterBehavior(...). The given options will stored
	// in the process environment using node.EnvKeyRemoteSpawn as a key
	RemoteSpawn(node string, object string, opts RemoteSpawnOptions, args ...etf.Term) (etf.Pid, error)
	RemoteSpawnWithTimeout(timeout int, node string, object string, opts RemoteSpawnOptions, args ...etf.Term) (etf.Pid, error)

	// Name returns process name used on starting.
	Name() string

	// RegisterName register associates the name with pid (not overrides registered name on starting)
	RegisterName(name string) error

	// UnregisterName unregister named process. Unregistering name is allowed to the owner only
	UnregisterName(name string) error

	// NodeName returns node name
	NodeName() string

	// NodeStop stops the node
	NodeStop()

	// NodeUptime returns node lifespan
	NodeUptime() int64

	// Info returns process details
	Info() ProcessInfo

	// Self returns registered process identificator belongs to the process
	Self() etf.Pid

	// Direct make a direct request to the actor (gen.Application, gen.Supervisor, gen.Server or
	// inherited from gen.Server actor) with default timeout 5 seconds
	Direct(request interface{}) (interface{}, error)

	// DirectWithTimeout make a direct request to the actor with the given timeout (in seconds)
	DirectWithTimeout(request interface{}, timeout int) (interface{}, error)

	// Send sends a message in fashion of 'erlang:send'. The value of 'to' can be a Pid, registered local name
	// or gen.ProcessID{RegisteredName, NodeName}
	Send(to interface{}, message etf.Term) error

	// SendAfter starts a timer. When the timer expires, the message sends to the process
	// identified by 'to'.  'to' can be a Pid, registered local name or
	// gen.ProcessID{RegisteredName, NodeName}. Returns cancel function in order to discard
	// sending a message
	SendAfter(to interface{}, message etf.Term, after time.Duration) context.CancelFunc

	// Exit initiate a graceful stopping process
	Exit(reason string) error

	// Kill immediately stops process
	Kill()

	// CreateAlias creates a new alias for the Process
	CreateAlias() (etf.Alias, error)

	// DeleteAlias deletes the given alias
	DeleteAlias(alias etf.Alias) error

	// ListEnv returns a map of configured environment variables.
	// It also includes environment variables from the GroupLeader, Parent and Node.
	// which are overlapped by priority: Process(Parent(GroupLeader(Node)))
	ListEnv() map[EnvKey]interface{}

	// SetEnv set environment variable with given name. Use nil value to remove variable with given name.
	SetEnv(name EnvKey, value interface{})

	// Env returns value associated with given environment name.
	Env(name EnvKey) interface{}

	// Wait waits until process stopped
	Wait()

	// WaitWithTimeout waits until process stopped. Return ErrTimeout
	// if given timeout is exceeded
	WaitWithTimeout(d time.Duration) error

	// Link creates a link between the calling process and another process.
	// Links are bidirectional and there can only be one link between two processes.
	// Repeated calls to Process.Link(Pid) have no effect. If one of the participants
	// of a link terminates, it will send an exit signal to the other participant and caused
	// termination of the last one. If process set a trap using Process.SetTrapExit(true) the exit signal transorms into the MessageExit and delivers as a regular message.
	Link(with etf.Pid)

	// Unlink removes the link, if there is one, between the calling process and
	// the process referred to by Pid.
	Unlink(with etf.Pid)

	// IsAlive returns whether the process is alive
	IsAlive() bool

	// SetTrapExit enables/disables the trap on terminate process. When a process is trapping exits,
	// it will not terminate when an exit signal is received. Instead, the signal is transformed
	// into a 'gen.MessageExit' which is put into the mailbox of the process just like a regular message.
	SetTrapExit(trap bool)

	// TrapExit returns whether the trap was enabled on this process
	TrapExit() bool

	// SetCompression enables/disables compression for the messages sent outside of this node
	SetCompression(enabled bool)

	// Compression returns true if compression is enabled for this process
	Compression() bool

	// MonitorNode creates monitor between the current process and node. If Node fails or does not exist,
	// the message MessageNodeDown is delivered to the process.
	MonitorNode(name string) etf.Ref

	// DemonitorNode removes monitor. Returns false if the given reference wasn't found
	DemonitorNode(ref etf.Ref) bool

	// MonitorProcess creates monitor between the processes.
	// Allowed types for the 'process' value: etf.Pid, gen.ProcessID
	// When a process monitor is triggered, a MessageDown sends to the caller.
	// Note: The monitor request is an asynchronous signal. That is, it takes
	// time before the signal reaches its destination.
	MonitorProcess(process interface{}) etf.Ref

	// DemonitorProcess removes monitor. Returns false if the given reference wasn't found
	DemonitorProcess(ref etf.Ref) bool

	// Behavior returns the object this process runs on.
	Behavior() ProcessBehavior
	// GroupLeader returns group leader process. Usually it points to the application process.
	GroupLeader() Process
	// Parent returns parent process. It returns nil if this process was spawned using Node.Spawn.
	Parent() Process
	// Context returns process context.
	Context() context.Context

	// Children returns list of children pid (Application, Supervisor)
	Children() ([]etf.Pid, error)

	// Links returns list of the process pids this process has linked to.
	Links() []etf.Pid
	// Monitors returns list of monitors created this process by pid.
	Monitors() []etf.Pid
	// Monitors returns list of monitors created this process by name.
	MonitorsByName() []ProcessID
	// MonitoredBy returns list of process pids monitored this process.
	MonitoredBy() []etf.Pid
	// Aliases returns list of aliases of this process.
	Aliases() []etf.Alias

	PutSyncRequest(ref etf.Ref)
	CancelSyncRequest(ref etf.Ref)
	WaitSyncReply(ref etf.Ref, timeout int) (etf.Term, error)
	PutSyncReply(ref etf.Ref, term etf.Term) error
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
	MonitorsByName  []ProcessID
	MonitoredBy     []etf.Pid
	Aliases         []etf.Alias
	Dictionary      etf.Map
	TrapExit        bool
	GroupLeader     etf.Pid
	Reductions      uint64
	Compression     bool
}

// ProcessOptions
type ProcessOptions struct {
	// Context allows mix the system context with the custom one. E.g. to limit
	// the lifespan using context.WithTimeout
	Context context.Context
	// MailboxSize defines the length of message queue for the process
	MailboxSize uint16
	// GroupLeader
	GroupLeader Process
	// Env set the process environment variables
	Env map[EnvKey]interface{}
}

// RemoteSpawnRequest
type RemoteSpawnRequest struct {
	From    etf.Pid
	Ref     etf.Ref
	Options RemoteSpawnOptions
}

// RemoteSpawnOptions defines options for RemoteSpawn method
type RemoteSpawnOptions struct {
	// Name register associated name with spawned process
	Name string
	// Monitor enables monitor on the spawned process using provided reference
	Monitor etf.Ref
	// Link enables link between the calling and spawned processes
	Link bool
	// Function in order to support {M,F,A} request to the Erlang node
	Function string
}

// ProcessChannels
type ProcessChannels struct {
	Mailbox      <-chan ProcessMailboxMessage
	Direct       <-chan ProcessDirectMessage
	GracefulExit <-chan ProcessGracefulExitRequest
}

// ProcessMailboxMessage
type ProcessMailboxMessage struct {
	From    etf.Pid
	Message interface{}
}

// ProcessDirectMessage
type ProcessDirectMessage struct {
	Message interface{}
	Err     error
	Reply   chan ProcessDirectMessage
}

// ProcessGracefulExitRequest
type ProcessGracefulExitRequest struct {
	From   etf.Pid
	Reason string
}

// ProcessState
type ProcessState struct {
	Process
	State interface{}
}

// ProcessBehavior interface contains methods you should implement to make own process behaviour
type ProcessBehavior interface {
	ProcessInit(Process, ...etf.Term) (ProcessState, error)
	ProcessLoop(ProcessState, chan<- bool) string // method which implements control flow of process
}

// Core the common set of methods provided by Process and node.Node interfaces
type Core interface {

	// ProcessByName returns Process for the given name.
	// Returns nil if it doesn't exist (not found) or terminated.
	ProcessByName(name string) Process

	// ProcessByPid returns Process for the given Pid.
	// Returns nil if it doesn't exist (not found) or terminated.
	ProcessByPid(pid etf.Pid) Process

	// ProcessByAlias returns Process for the given alias.
	// Returns nil if it doesn't exist (not found) or terminated
	ProcessByAlias(alias etf.Alias) Process

	// ProcessInfo returns the details about given Pid
	ProcessInfo(pid etf.Pid) (ProcessInfo, error)

	// ProcessList returns the list of running processes
	ProcessList() []Process

	// MakeRef creates an unique reference within this node
	MakeRef() etf.Ref

	// IsAlias checks whether the given alias is belongs to the alive process on this node.
	// If the process died all aliases are cleaned up and this function returns
	// false for the given alias. For alias from the remote node always returns false.
	IsAlias(etf.Alias) bool

	// IsMonitor returns true if the given references is a monitor
	IsMonitor(ref etf.Ref) bool

	// RegisterBehavior
	RegisterBehavior(group, name string, behavior ProcessBehavior, data interface{}) error
	// RegisteredBehavior
	RegisteredBehavior(group, name string) (RegisteredBehavior, error)
	// RegisteredBehaviorGroup
	RegisteredBehaviorGroup(group string) []RegisteredBehavior
	// UnregisterBehavior
	UnregisterBehavior(group, name string) error
}

// RegisteredBehavior
type RegisteredBehavior struct {
	Behavior ProcessBehavior
	Data     interface{}
}

// ProcessID long notation of registered process {process_name, node_name}
type ProcessID struct {
	Name string
	Node string
}

// String string representaion of ProcessID value
func (p ProcessID) String() string {
	return fmt.Sprintf("<%s:%s>", p.Name, p.Node)
}

// MessageDown delivers as a message to Server's HandleInfo callback of the process
// that created monitor using MonitorProcess.
// Reason values:
//  - the exit reason of the process
//  - 'noproc' (process did not exist at the time of monitor creation)
//  - 'noconnection' (no connection to the node where the monitored process resides)
type MessageDown struct {
	Ref       etf.Ref   // a monitor reference
	ProcessID ProcessID // if monitor was created by name
	Pid       etf.Pid
	Reason    string
}

// MessageNodeDown delivers as a message to Server's HandleInfo callback of the process
// that created monitor using MonitorNode
type MessageNodeDown struct {
	Name string
}

// MessageExit delievers to Server's HandleInfo callback on enabled trap exit using SetTrapExit(true)
type MessageExit struct {
	Pid    etf.Pid
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

// MessageDirectChildren type intended to be used in Process.Children which returns []etf.Pid
// You can handle this type of message in your HandleDirect callback to enable Process.Children
// support for your gen.Server actor.
type MessageDirectChildren struct{}

// IsMessageDown
func IsMessageDown(message etf.Term) (MessageDown, bool) {
	var md MessageDown
	switch m := message.(type) {
	case MessageDown:
		return m, true
	}
	return md, false
}

// IsMessageExit
func IsMessageExit(message etf.Term) (MessageExit, bool) {
	var me MessageExit
	switch m := message.(type) {
	case MessageExit:
		return m, true
	}
	return me, false
}
