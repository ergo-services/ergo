package gen

import (
	"errors"
	"fmt"
	"time"

	"ergo.services/ergo/lib"
)

// ProcessBehavior interface contains methods you should implement to make your own process behavior
type ProcessBehavior interface {
	ProcessInit(process Process, args ...any) error
	ProcessRun() error
	ProcessTerminate(reason error)
}

type ProcessFactory func() ProcessBehavior
type CancelFunc func() bool

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
	ProcessStateInit         ProcessState = 1
	ProcessStateSleep        ProcessState = 2
	ProcessStateRunning      ProcessState = 4
	ProcessStateWaitResponse ProcessState = 8
	ProcessStateTerminated   ProcessState = 16
	ProcessStateZombee       ProcessState = 32
)

var (
	TerminateReasonNormal   error = errors.New("normal")
	TerminateReasonKill     error = errors.New("kill")
	TerminateReasonPanic    error = errors.New("panic")
	TerminateReasonShutdown error = errors.New("shutdown")
)

// Process
type Process interface {
	// Node returns Node interface
	Node() Node

	// Name returns registered name associated with this process
	Name() Atom

	// PID returns identificator belonging to the process
	PID() PID

	// Leader returns group leader process. Usually it points to the application (or supervisor) process. Otherwise, it has the same value as parent.
	Leader() PID

	// Parent returns parent process.
	Parent() PID

	// Uptime returns process uptime in seconds
	Uptime() int64

	// Spawn creates a child process. Terminating the parent process
	// doesn't cause terminating this process.
	Spawn(factory ProcessFactory, options ProcessOptions, args ...any) (PID, error)

	// Spawn creates a child process with associated name.
	SpawnRegister(
		register Atom,
		factory ProcessFactory,
		options ProcessOptions,
		args ...any,
	) (PID, error)

	// SpawnMeta creates a meta process. Returned alias is associated with this process and other
	// processes can send messages (using Send method) or make the requests (with Call method)
	// to this meta process.
	SpawnMeta(behavior MetaBehavior, options MetaOptions) (Alias, error)

	// RemoteSpawn makes request to the remote node to spawn a new process. See also ProvideSpawn method.
	RemoteSpawn(node Atom, name Atom, options ProcessOptions, args ...any) (PID, error)
	// RemoteSpawnRegister makes request to the remote node to spawn a new process and register it there
	// with the given rigistered name.
	RemoteSpawnRegister(
		node Atom,
		name Atom,
		register Atom,
		options ProcessOptions,
		args ...any,
	) (PID, error)

	// State returns current process state. Usually, it returns gen.ProcessStateRunning.
	// But, If node has killed this process during the handling of its mailbox,
	// it returns gen.ProcessStateZombee, which means this process won't receive any new messages,
	// and most of the gen.Process methods won't be working returning gen.ErrNotAllowed.
	State() ProcessState

	// RegisterName register associates the name with PID so you can address messages to this
	// process using gen.ProcessID{<name>, <nodename>}. Returns error if this process is already
	// has registered name.
	RegisterName(name Atom) error

	// UnregisterName unregister associated name.
	UnregisterName() error

	// EnvList returns a map of configured environment variables.
	// It also includes environment variables from the GroupLeader, Parent and Node.
	// which are overlapped by priority: Process(Parent(GroupLeader(Node)))
	EnvList() map[Env]any

	// SetEnv set environment variable with given name. Use nil value to remove variable with given name.
	SetEnv(name Env, value any)

	// Env returns value associated with given environment name.
	Env(name Env) (any, bool)

	// EnvDefault returns value associated with given environment name or default value if it is not set.
	EnvDefault(name Env, def any) any

	// Compression returns true if compression is enabled for this process.
	Compression() bool

	// SetCompression enables/disables compression for the messages sent over the network
	SetCompression(enabled bool) error

	// CompressionType returns type of compression
	CompressionType() CompressionType
	// SetCompressionType defines the compression type. Use gen.CompressionTypeZLIB or gen.CompressionTypeLZW. Be default is using gen.CompressionTypeGZIP
	SetCompressionType(ctype CompressionType) error

	// CompressionLevel returns comression level for the process
	CompressionLevel() CompressionLevel

	// SetCompressionLevel defines compression level. Use gen.CompressionBestSize or gen.CompressionBestSpeed. By default is using gen.CompressionDefault
	SetCompressionLevel(level CompressionLevel) error

	// CompressionThreshold returns compression threshold for the process
	CompressionThreshold() int

	// SetCompressionThreshold defines the minimal size for the message that must be compressed
	// Value must be greater than DefaultCompressionThreshold (1024)
	SetCompressionThreshold(threshold int) error

	// SendPriority returns priority for the sending messages
	SendPriority() MessagePriority

	// SetSendPriority defines priority for the sending messages
	SetSendPriority(priority MessagePriority) error

	// SetKeepNetworkOrder enables/disables to keep delivery order over the network. In some cases disabling this options allows improve network performance.
	SetKeepNetworkOrder(order bool) error

	// KeepNetworkOrder returns true if it was enabled, otherwise - false. Enabled by default.
	KeepNetworkOrder() bool

	// SetImportantDelivery enables/disables important flag for sending messages. This flag makes remote node to send confirmation that message was delivered into the process mailbox
	SetImportantDelivery(important bool) error
	// ImportantDelivery returns true if flag ImportantDelivery was set for this process
	ImportantDelivery() bool

	// CreateAlias creates a new alias associated with this process
	CreateAlias() (Alias, error)

	// DeleteAlias deletes the given alias
	DeleteAlias(alias Alias) error

	// Aliases lists of aliases associated with this process
	Aliases() []Alias

	// Events lists of registered events by this process
	Events() []Atom

	// Send sends a message
	Send(to any, message any) error
	SendPID(to PID, message any) error
	SendProcessID(to ProcessID, message any) error
	SendAlias(to Alias, message any) error
	SendWithPriority(to any, message any, priority MessagePriority) error
	SendImportant(to any, message any) error

	// SendAfter starts a timer. When the timer expires, the message sends to the process
	// identified by 'to'. Returns cancel function in order to discard
	// sending a message. CancelFunc returns bool value. If it returns false, than the timer has
	// already expired and the message has been sent.
	SendAfter(to any, message any, after time.Duration) (CancelFunc, error)

	// SendEvent sends event message to the subscribers (to the processes that made link/monitor
	// on this event). Event must be registered with RegisterEvent method.
	SendEvent(name Atom, token Ref, message any) error

	// SendExit sends graceful termination request to the process.
	SendExit(to PID, reason error) error

	// SendExitMeta sends graceful termination request to the meta process.
	SendExitMeta(meta Alias, reason error) error

	SendResponse(to PID, ref Ref, message any) error
	SendResponseError(to PID, ref Ref, err error) error

	// Call makes a sync request
	Call(to any, message any) (any, error)
	CallWithTimeout(to any, message any, timeout int) (any, error)
	CallWithPriority(to any, message any, priority MessagePriority) (any, error)
	CallImportant(to any, message any) (any, error)
	CallPID(to PID, message any, timeout int) (any, error)
	CallProcessID(to ProcessID, message any, timeout int) (any, error)
	CallAlias(to Alias, message any, timeout int) (any, error)

	// Inspect sends inspect request to the process.
	Inspect(target PID, item ...string) (map[string]string, error)
	// Inspect sends inspect request to the meta process.
	InspectMeta(meta Alias, item ...string) (map[string]string, error)

	// RegisterEvent registers a new event. Returns a reference as the token
	// for sending events. Unregistering the event is allowed to the process
	// that registered it. Sending an event can be done by any other process
	// using the registered event name with the provided token (delegation of
	// event sending feature).
	RegisterEvent(name Atom, options EventOptions) (Ref, error)

	// UnregisterEvent unregisters an event. It can be done by the process owner
	// of this event only.
	UnregisterEvent(name Atom) error

	// links
	Link(target any) error
	Unlink(target any) error

	LinkPID(target PID) error
	UnlinkPID(target PID) error

	LinkProcessID(target ProcessID) error
	UnlinkProcessID(target ProcessID) error

	LinkAlias(target Alias) error
	UnlinkAlias(target Alias) error

	LinkEvent(target Event) ([]MessageEvent, error)
	UnlinkEvent(target Event) error

	LinkNode(target Atom) error
	UnlinkNode(target Atom) error

	// monitors
	Monitor(target any) error
	Demonitor(target any) error

	MonitorPID(pid PID) error
	DemonitorPID(pid PID) error

	MonitorProcessID(process ProcessID) error
	DemonitorProcessID(process ProcessID) error

	MonitorAlias(alias Alias) error
	DemonitorAlias(alias Alias) error

	MonitorEvent(event Event) ([]MessageEvent, error)
	DemonitorEvent(event Event) error

	MonitorNode(node Atom) error
	DemonitorNode(node Atom) error

	// Log returns gen.Log interface
	Log() Log

	// Info returns summary information about this process
	Info() (ProcessInfo, error)

	// MetaInfo returns summary information about given meta process
	MetaInfo(meta Alias) (MetaInfo, error)

	// low level api (for gen.ProcessBehavior implementaions)

	Mailbox() ProcessMailbox
	Behavior() ProcessBehavior
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

// ProcessOptions
type ProcessOptions struct {
	// MailboxSize defines the length of message queue for the process. Default is zero - unlimited
	MailboxSize int64
	// Leader
	Leader PID
	// Env set the process environment variables
	Env map[Env]any

	Compression Compression

	// SendPriority defines the priority of the sending messages.
	// Actor-receiver handles its Mailbox with the next order:
	//  - Urgent
	//  - System
	//  - Main
	// Setting this option to MessagePriorityHigh makes the node deliver messages
	// to the "Mailbox.System" of the receving Process
	// With MessagePriorityMax - makes delivering to the Mailbox.Urgent
	// By default, messages are delivering to the Mailbox.Main.
	SendPriority MessagePriority

	// ImportantDelivery enables important flag for sending messages. This flag makes remote node to send confirmation that message was delivered into the process mailbox
	ImportantDelivery bool

	// Fallback defines the process to where messages will be forwarded
	// if the mailbox is overflowed. The tag value could be used to
	// differentiate the source processes. Forwarded messages are wrapped
	// into the MessageFallback struct with the given tag value.
	// This option is ignored for the unlimited mailbox size
	Fallback ProcessFallback

	// LinkParent creates a link with the parent process on start.
	// It will make this process terminate on the parent process termination.
	// This option is ignored if this process starts by the node.
	LinkParent bool

	// LinkChild makes the node create a link with the spawning process.
	// This feature allows you to link the parent process with the child even
	// being in the init state. This option is ignored if this process starts by the node.
	LinkChild bool

	// LogLevel defines logging level. Default is gen.LogLevelInfo
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

// ProcessInfo
type ProcessInfo struct {
	// PID process ID
	PID PID
	// Name registered associated name with this process
	Name Atom
	// Application application name if this process started under application umbrella
	Application Atom
	// Behavior
	Behavior string
	// MailboxSize
	MailboxSize int64
	// MailboxQueues
	MailboxQueues MailboxQueues
	// MessagesIn total number of messages this process received
	MessagesIn uint64
	// MessagesOut total number of messages this process sent
	MessagesOut uint64
	// RunningTime how long this process was in 'running' state in ns
	RunningTime uint64
	// Compression
	Compression Compression
	// MessagePriority priority for the sending messages
	MessagePriority MessagePriority
	// Uptime of the process in seconds
	Uptime int64
	// State shows current state of the process
	State ProcessState
	// Parent points to the parent process that spawned this process as a child
	Parent PID
	// Leader usually points to the Supervisor or Application process
	Leader PID
	// Fallback
	Fallback ProcessFallback
	// Env process environment. gen.NodeOptions.Security.ExposeEnvInfo must be enabled to reveal this data
	Env map[Env]any
	// Aliases list of the aliases belonging to this process
	Aliases []Alias
	// Events list of the events this process is the owner of
	Events []Atom

	// Metas list of meta processes
	Metas []Alias

	// MonitorsPID list of processes monitored by this process by the PID
	MonitorsPID []PID
	// MonitorsProcessID list of processes monitored by this process by the name
	MonitorsProcessID []ProcessID
	// MonitorsAlias list of aliases monitored by this process
	MonitorsAlias []Alias
	// MonitorsEvent list of events monitored by this process
	MonitorsEvent []Event
	// MonitorsNode list of remote nodes monitored by this process
	MonitorsNode []Atom

	// LinksPID list of the processes this process is linked with
	LinksPID []PID
	// LinksProcessID list of the processes this process is linked with by the name.
	LinksProcessID []ProcessID
	// LinksAlias list of the aliases this process is linked with
	LinksAlias []Alias
	// LinksEvent list of the events this process is linked with
	LinksEvent []Event
	//LinksNode list of the remote nodes this process is linked with
	LinksNode []Atom

	// LogLevel current logging level
	LogLevel LogLevel
	// KeepNetworkOrder
	KeepNetworkOrder bool
	// ImportantDelivery
	ImportantDelivery bool
}

// ProcessShortInfo
type ProcessShortInfo struct {
	// PID process ID
	PID PID
	// Name registered associated name with this process
	Name Atom
	// Application application name if this process started under application umbrella
	Application Atom
	// Behavior
	Behavior string
	// MessagesIn total number of messages this process received
	MessagesIn uint64
	// MessagesOut total number of messages this process sent
	MessagesOut uint64
	// MessagesMailbox total number of messages in mailbox queues
	MessagesMailbox uint64
	// RunningTime how long this process was in 'running' state in ns
	RunningTime uint64
	// Uptime of the process in seconds
	Uptime int64
	// State shows current state of the process
	State ProcessState
	// Parent points to the parent process that spawned this process as a child
	Parent PID
	// Leader usually points to the Supervisor or Application process
	Leader PID
	// LogLevel current logging level
	LogLevel LogLevel
}

// ProcessFallback
type ProcessFallback struct {
	Enable bool
	Name   Atom
	Tag    string
}

type ProcessMailbox struct {
	Main   lib.QueueMPSC
	System lib.QueueMPSC
	Urgent lib.QueueMPSC
	Log    lib.QueueMPSC
}

type MailboxQueues struct {
	Main   int64
	System int64
	Urgent int64
	Log    int64
}
