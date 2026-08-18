package gen

import (
	"time"
)

// Node interface provides node-level operations for managing processes, applications, and networking.
//
// Unlike Process interface, Node methods can be called from any goroutine at any time.
// Node is not an actor - it's a container and runtime for actor-based processes.
//
// Most operations require the node to be in Running state (operational).
// Read-only operations and cleanup methods work in all states.
type Node interface {
	// Name returns the node name.
	// Format: "name@hostname"
	// Available in all states.
	Name() Atom

	// IsAlive returns true if the node is running.
	// Returns false if node is stopped or stopping.
	// Available in all states.
	IsAlive() bool

	// Uptime returns node uptime in seconds since start.
	// Returns 0 if node is stopped.
	// Available in all states.
	Uptime() int64

	// Version returns the node version information.
	// Includes Name, Release, License, and Commit.
	// Available in all states.
	Version() Version

	// FrameworkVersion returns the Ergo framework version.
	// Available in all states.
	FrameworkVersion() Version

	// Info returns comprehensive node information.
	// Includes uptime, processes, applications, memory, environment, etc.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	Info() (NodeInfo, error)

	// ShortInfo returns essential node information.
	// Includes uptime, process and application counters, memory, runtime and peers.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ShortInfo() (NodeShortInfo, error)

	// EnvList returns a map of configured node environment variables.
	// Available in all states.
	EnvList() map[Env]any

	// SetEnv sets a node environment variable.
	// Use nil value to remove the variable.
	// Available in: Running state only.
	SetEnv(name Env, value any)

	// Env returns the value associated with the given environment variable name.
	// Returns (value, true) if found, (nil, false) if not found.
	// Available in all states.
	Env(name Env) (any, bool)

	// EnvDefault returns the value associated with the given environment variable name,
	// or the default value if not set.
	// Available in all states.
	EnvDefault(name Env, def any) any

	// Spawn creates a new process on this node.
	// The process parent and leader are set to the node's core PID.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	Spawn(factory ProcessFactory, options ProcessOptions, args ...any) (PID, error)

	// SpawnRegister creates a new process and registers it with the given name.
	// The process will be addressable via ProcessID{register, nodename}.
	// Parent and leader are set to the node's core PID.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTaken if name already registered.
	SpawnRegister(register Atom, factory ProcessFactory, options ProcessOptions, args ...any) (PID, error)

	// RegisterName associates a name with the given PID.
	// The process can then be addressed using ProcessID{name, nodename}.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTaken if name already taken.
	RegisterName(name Atom, pid PID) error

	// UnregisterName removes the name association.
	// Returns the PID that was associated with this name.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrNameUnknown if name not found.
	UnregisterName(name Atom) (PID, error)

	// MetaInfo returns detailed information about the given meta process.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	MetaInfo(meta Alias) (MetaInfo, error)

	// ProcessInfo returns detailed information about the given process.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrProcessUnknown if process not found.
	ProcessInfo(pid PID) (ProcessInfo, error)

	// ProcessList returns a list of all process PIDs on this node.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ProcessList() ([]PID, error)

	// ProcessListShortInfo returns a list of processes with essential information.
	// The start and limit parameters filter by process ID range.
	// A limit of 0 applies the default of 100; a negative limit returns ErrIncorrect.
	// More efficient than ProcessList + ProcessInfo for each.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ProcessListShortInfo(start, limit int, filter ...func(ProcessShortInfo) bool) ([]ProcessShortInfo, error)

	// ProcessRangeShortInfo iterates over all processes calling fn for each.
	// The callback receives ProcessShortInfo and returns true to continue
	// or false to stop iteration.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ProcessRangeShortInfo(fn func(ProcessShortInfo) bool) error

	// ProcessName returns the registered name for the given PID.
	// Returns empty Atom if the process has no registered name.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrProcessUnknown if process not found.
	ProcessName(pid PID) (Atom, error)

	// ProcessPID returns the PID for the given registered name.
	// Reverse of ProcessName - looks up PID by name.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrProcessUnknown if name not registered.
	ProcessPID(name Atom) (PID, error)

	// ProcessState returns the current state of the given process.
	// Returns ProcessStateSleep (idle), ProcessStateRunning (handling messages),
	// ProcessStateWaitResponse (blocked in Call), ProcessStateTerminated (terminating),
	// or ProcessStateZombee (killed).
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ProcessState(pid PID) (ProcessState, error)

	// ApplicationLoad loads an application into the node.
	// The application is loaded but not started. Use ApplicationStart to run it.
	// Returns the application name.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTaken if name already registered.
	ApplicationLoad(app ApplicationBehavior, args ...any) (Atom, error)

	// ApplicationInfo returns information about the given application.
	// Includes name, state, mode, and children count.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrApplicationUnknown if not found.
	ApplicationInfo(name Atom) (ApplicationInfo, error)

	// ApplicationProcessList returns PIDs of the application's processes, including
	// those spawned by its members (recursive), in ascending id order.
	// A limit of 0 returns all of them; a positive limit caps the result; a
	// negative limit returns ErrIncorrect.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ApplicationProcessList(name Atom, limit int) ([]PID, error)

	// ApplicationProcessListShortInfo returns process list with essential information.
	// Includes all processes from application and its children, in ascending id order.
	// A limit of 0 applies the default of 100; a negative limit returns ErrIncorrect.
	// The second return value is the number of matching processes omitted because
	// the limit was reached (0 means the whole list was returned).
	// More efficient than ApplicationProcessList + ProcessInfo for each.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ApplicationProcessListShortInfo(name Atom, limit int) ([]ProcessShortInfo, int, error)

	// ApplicationUnload unloads an application from the node.
	// Application must be stopped before unloading.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states,
	// ErrApplicationRunning if still running, ErrApplicationUnknown if not found.
	ApplicationUnload(name Atom) error

	// ApplicationStart starts the application with its supervision tree.
	// Uses the starting mode defined in ApplicationSpec.Mode.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ApplicationStart(name Atom, options ApplicationOptions) error

	// ApplicationStartTemporary starts the application in temporary mode.
	// Overrides ApplicationSpec.Mode. Temporary: stops when any child terminates abnormally.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ApplicationStartTemporary(name Atom, options ApplicationOptions) error

	// ApplicationStartTransient starts the application in transient mode.
	// Overrides ApplicationSpec.Mode. Transient: stops only on abnormal child termination.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ApplicationStartTransient(name Atom, options ApplicationOptions) error

	// ApplicationStartPermanent starts the application in permanent mode.
	// Overrides ApplicationSpec.Mode. Permanent: never stops on child termination.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ApplicationStartPermanent(name Atom, options ApplicationOptions) error

	// ApplicationStop stops the application gracefully.
	// Waits for all children to terminate (default timeout: 5 seconds).
	// Application can be unloaded after stopping.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrApplicationStopping if still stopping.
	ApplicationStop(name Atom) error

	// ApplicationStopForce forcefully kills all application children.
	// Does not wait for graceful termination.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ApplicationStopForce(name Atom) error

	// ApplicationStopWithTimeout stops the application with custom timeout.
	// Waits for all children to terminate within the specified duration.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrApplicationStopping on timeout.
	ApplicationStopWithTimeout(name Atom, timeout time.Duration) error

	// Applications returns a list of all application names (loaded and started).
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	Applications() []Atom

	// ApplicationsRunning returns a list of currently running application names.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ApplicationsRunning() []Atom

	// NetworkStart starts the network stack with the given options.
	// Enables networking if it was disabled.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	NetworkStart(options NetworkOptions) error

	// NetworkStop stops the network stack.
	// Closes all connections and acceptors.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	NetworkStop() error

	// Network returns the Network interface for managing connections and routing.
	// Available in all states.
	Network() Network

	// Cron returns the Cron interface for scheduling tasks.
	// Available in all states.
	Cron() Cron

	// CertManager returns the certificate manager for TLS operations.
	// Available in all states.
	CertManager() CertManager

	// Security returns the security configuration options.
	// Available in all states.
	Security() SecurityOptions

	// Stop initiates graceful node shutdown.
	// Waits for all processes and applications to terminate, up to
	// NodeOptions.ShutdownTimeout. On timeout remaining processes are
	// force-killed.
	// Can be called from any state (idempotent).
	Stop()

	// StopWithTimeout initiates graceful node shutdown with a caller-provided
	// shutdown deadline, overriding NodeOptions.ShutdownTimeout. On timeout
	// remaining processes are force-killed.
	// Can be called from any state (idempotent).
	StopWithTimeout(timeout time.Duration)

	// StopForce forcefully kills all processes and stops the node immediately.
	// No graceful shutdown.
	// Can be called from any state (idempotent).
	StopForce()

	// Wait blocks until the node terminates.
	// Returns immediately if node is already stopped.
	// Can be called from any state.
	Wait()

	// WaitWithTimeout blocks until the node terminates or timeout expires.
	// Returns ErrTimeout if timeout occurs before termination.
	// Can be called from any state.
	WaitWithTimeout(timeout time.Duration) error

	// Kill forcefully terminates the given process.
	// Only works for local processes on this node.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrProcessUnknown if process not found.
	Kill(pid PID) error

	// Send sends an asynchronous message to the target.
	// Sender is the node's core PID. Target can be: PID, ProcessID, Alias, Atom, or string.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	Send(to any, message any) error

	// SendWithPriority sends an asynchronous message with the specified priority.
	// Sender is the node's core PID.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	SendWithPriority(to any, message any, priority MessagePriority) error

	// SendEvent sends an event message to all subscribers of the event.
	// Event must be registered first using RegisterEvent.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrEventUnknown if event not registered.
	SendEvent(name Atom, token Ref, options MessageOptions, message any) error

	// RegisterEvent registers a new event with the node as producer.
	// Returns a reference token for sending events.
	// Other processes can subscribe via LinkEvent/MonitorEvent.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTaken if event already registered.
	RegisterEvent(name Atom, options EventOptions) (Ref, error)

	// UnregisterEvent unregisters an event registered by the node.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrEventUnknown if not found.
	UnregisterEvent(name Atom) error

	// EventInfo returns information about the given event.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrEventUnknown if not found.
	EventInfo(event Event) (EventInfo, error)

	// EventRangeInfo iterates over all registered events calling fn for each.
	// The callback receives EventInfo and returns true to continue
	// or false to stop iteration.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	EventRangeInfo(fn func(EventInfo) bool) error

	// EventListInfo returns a paginated list of events in registration order.
	// timestamp: 0 = from oldest, -1 = from newest, >0 = from events created at or after this time (unix nanos).
	// limit: >0 = forward (oldest first), <0 = backward (newest first), abs(limit) results max.
	// filter: optional function to include only matching events.
	// Available in: Running state only.
	EventListInfo(timestamp int64, limit int, filter ...func(EventInfo) bool) ([]EventInfo, error)

	// SendExit sends a graceful termination request to the process.
	// Sender is the node's core PID.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	SendExit(pid PID, reason error) error

	// Call makes a synchronous request with default timeout (5 seconds).
	// Blocks until response arrives or timeout occurs. Sender is node's core PID.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTimeout on timeout.
	Call(to any, request any) (any, error)

	// CallWithTimeout makes a synchronous request with custom timeout (in seconds).
	// Blocks until response arrives or timeout occurs.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTimeout on timeout.
	CallWithTimeout(to any, request any, timeout int) (any, error)

	// CallWithPriority makes a synchronous request with custom priority.
	// Uses default timeout (5 seconds). Blocks until response.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTimeout on timeout.
	CallWithPriority(to any, request any, priority MessagePriority) (any, error)

	// CallImportant makes a synchronous request with important delivery flag.
	// Uses default timeout (5 seconds). Blocks until response.
	//
	// Important delivery provides network transparency for error detection:
	// - Without Important: timeout if remote process doesn't exist (ambiguous - slow or missing?)
	// - With Important: immediate ErrProcessUnknown if remote process doesn't exist
	// Aligns remote error handling with local delivery (local always returns immediate error).
	//
	// When the responder uses SendResponseImportant or SendResponseErrorImportant, creates
	// Fully-Reliable Two-Phase Commit (FR-2PC) with guaranteed delivery in both directions.
	//
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTimeout on timeout,
	// ErrProcessUnknown if target doesn't exist (with Important flag).
	CallImportant(to any, request any) (any, error)

	// CallPID makes a synchronous request to the process identified by PID.
	// Timeout in seconds. Blocks until response.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTimeout on timeout.
	CallPID(to PID, request any, timeout int) (any, error)

	// CallProcessID makes a synchronous request to the named process.
	// Timeout in seconds. Blocks until response.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTimeout on timeout.
	CallProcessID(to ProcessID, request any, timeout int) (any, error)

	// CallAlias makes a synchronous request to the process via alias.
	// Timeout in seconds. Blocks until response.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTimeout on timeout.
	CallAlias(to Alias, request any, timeout int) (any, error)

	// Inspect sends an inspection request to a local process.
	// Returns a map of inspection items. Synchronous operation.
	// Only works for local processes (same node).
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrNotAllowed for remote processes,
	// ErrProcessUnknown if target doesn't exist, ErrProcessTerminated if terminated,
	// ErrProcessMailboxFull if mailbox is full, ErrTimeout on timeout.
	Inspect(target PID, item ...string) (map[string]string, error)

	// InspectMeta sends an inspection request to a local meta process.
	// Returns a map of inspection items. Synchronous operation.
	// Only works for local meta processes (same node).
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrNotAllowed for remote meta,
	// ErrMetaUnknown if target doesn't exist, ErrProcessTerminated if terminated,
	// ErrProcessMailboxFull if mailbox is full, ErrTimeout on timeout.
	InspectMeta(alias Alias, item ...string) (map[string]string, error)

	// Log returns the node's logger interface.
	// Available in all states.
	Log() Log

	// SetProcessLogLevel sets the logging level for the given process.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	SetProcessLogLevel(pid PID, level LogLevel) error

	// SetProcessSendPriority sets the default message sending priority for the given process.
	// Available in: Running state only.
	SetProcessSendPriority(pid PID, priority MessagePriority) error

	// SetProcessCompression enables or disables compression for the given process.
	// Available in: Running state only.
	SetProcessCompression(pid PID, enabled bool) error

	// SetProcessCompressionType sets the compression type for the given process.
	// Available in: Running state only.
	SetProcessCompressionType(pid PID, ctype CompressionType) error

	// SetProcessCompressionLevel sets the compression level for the given process.
	// Available in: Running state only.
	SetProcessCompressionLevel(pid PID, level CompressionLevel) error

	// SetProcessCompressionThreshold sets the minimum message size that triggers compression for the given process.
	// Available in: Running state only.
	SetProcessCompressionThreshold(pid PID, threshold int) error

	// SetProcessKeepNetworkOrder enables or disables maintaining delivery order over the network for the given process.
	// Available in: Running state only.
	SetProcessKeepNetworkOrder(pid PID, order bool) error

	// SetProcessImportantDelivery enables or disables the important delivery flag for the given process.
	// Available in: Running state only.
	SetProcessImportantDelivery(pid PID, important bool) error

	// SetMetaLogLevel sets the logging level for the given meta process.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	SetMetaLogLevel(meta Alias, level LogLevel) error

	// SetMetaSendPriority sets the default message sending priority for the given meta process.
	// Available in: Running state only.
	SetMetaSendPriority(meta Alias, priority MessagePriority) error

	// Loggers returns a list of registered logger names.
	// Available in all states.
	Loggers() []string

	// LoggerAddPID registers a process as a logger.
	// The process will receive MessageLogNode and MessageLogProcess messages.
	// Optional filter specifies which log levels to receive.
	//
	// Hidden loggers: Prefix the name with "." to create a hidden logger.
	// Hidden loggers are excluded from fan-out distribution - they only receive logs
	// from processes that explicitly call SetLogger(name) to use them.
	// Use hidden loggers to create separate logging streams for specific processes
	// without mixing their logs with general system logs.
	//
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	LoggerAddPID(pid PID, name string, filter ...LogLevel) error

	// LoggerAdd registers a custom logger implementation.
	// Optional filter specifies which log levels to send to this logger.
	//
	// Hidden loggers: Prefix the name with "." to create a hidden logger.
	// Hidden loggers are excluded from fan-out distribution - they only receive logs
	// from processes that explicitly call SetLogger(name) to use them.
	// Use hidden loggers to create separate logging streams for specific processes
	// without mixing their logs with general system logs.
	//
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTaken if name already used.
	LoggerAdd(name string, logger LoggerBehavior, filter ...LogLevel) error

	// LoggerDeletePID removes a process from the loggers list.
	// Safe cleanup operation.
	// Available in all states.
	LoggerDeletePID(pid PID)

	// LoggerDelete removes a custom logger from the loggers list.
	// Calls logger.Terminate() if logger exists.
	// Safe cleanup operation.
	// Available in all states.
	LoggerDelete(name string)

	// LoggerLevels returns the list of log levels for the given logger.
	// Shows which levels are being captured by this logger.
	// Available in all states.
	LoggerLevels(name string) []LogLevel

	// TracingExporterAddPID registers a process as a tracing exporter.
	// The process will receive TracingSpan messages.
	// Available in: Running state only.
	// Returns ErrTaken if name already registered.
	TracingExporterAddPID(pid PID, name string, flags TracingFlags) error

	// TracingExporterAdd registers a custom tracing exporter implementation.
	// Available in: Running state only.
	// Returns ErrTaken if name already registered.
	TracingExporterAdd(name string, exporter TracingBehavior, flags TracingFlags) error

	// TracingExporterDeletePID removes a process-based tracing exporter.
	// Available in all states.
	TracingExporterDeletePID(pid PID)

	// TracingExporterDelete removes a tracing exporter.
	// Calls exporter.Terminate() if exporter exists.
	// Available in all states.
	TracingExporterDelete(name string)

	// TracingExporters returns a list of registered tracing exporter names.
	// Available in all states.
	TracingExporters() []string

	// TracingExporterFlags returns the flags for the given tracing exporter.
	// Available in all states.
	TracingExporterFlags(name string) TracingFlags

	// SetTracingSampler sets the tracing sampler for node-level Send/Call.
	// Use TracingSamplerDisable to turn off.
	// Available in: Running state only.
	SetTracingSampler(sampler TracingSampler) error

	// SetTracingAttribute sets a permanent tracing attribute on the node.
	SetTracingAttribute(key, value string)

	// RemoveTracingAttribute removes a permanent tracing attribute from the node.
	RemoveTracingAttribute(key string)

	// TracingSampler returns the current tracing sampler for the node.
	// Available in all states.
	TracingSampler() TracingSampler

	// SetProcessTracingSampler sets the tracing sampler for the given process.
	// Available in: Running state only.
	SetProcessTracingSampler(pid PID, sampler TracingSampler) error

	// MakeRef creates a unique reference within this node.
	// Used for Call requests, event tokens, and correlation.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	MakeRef() Ref

	// MakeRefWithDeadline creates a unique reference with a deadline.
	// The deadline is a unix timestamp in seconds and must be in the future.
	// Stored in Ref.ID[2]. Recipient can check validity using Ref.IsAlive().
	// Used for Call operations to embed timeout information.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrIncorrect if deadline invalid.
	MakeRefWithDeadline(deadline int64) (Ref, error)

	// Commercial returns a list of components with commercial licenses (LicenseBSL1).
	// Used for license compliance reporting.
	// Available in all states.
	Commercial() []Version

	// PID returns the node's core virtual PID.
	// Used as sender for node-level operations (Send, SendExit, SendEvent).
	// Used as parent PID for node-spawned processes.
	// Available in all states.
	PID() PID

	// Creation returns the node creation timestamp (unix seconds).
	// Changes on node restart. Used to detect incarnation differences.
	// Available in all states.
	Creation() int64

	// SetCTRLC enables or disables SIGTERM signal handling.
	// When enabled, SIGTERM triggers graceful node shutdown.
	// Available in: Running state only.
	SetCTRLC(enable bool)
}

// NodeRegistrar bridge interface from Node to the Registrar
type NodeRegistrar interface {
	Name() Atom
	Creation() int64

	// Peers returns the remote nodes this node has a connection with.
	Peers() []Atom
	SetEnv(name Env, value any)
	RegisterEvent(name Atom, options EventOptions) (Ref, error)
	UnregisterEvent(name Atom) error
	SendEvent(name Atom, token Ref, options MessageOptions, message any) error
	Log() Log
	Stop()
	StopWithTimeout(timeout time.Duration)
	StopForce()
}

// NodeHandshake bridge interface from Node to the Handshake
type NodeHandshake interface {
	Name() Atom
	Creation() int64
	Version() Version
}

// NodeOptions defines configuration options for node initialization.
// Used in node.Start() to configure the node before it becomes operational.
type NodeOptions struct {
	// ShutdownTimeout sets the maximum time to wait for processes to terminate
	// during graceful shutdown. Default is 3 minutes (gen.DefaultShutdownTimeout).
	// After timeout expires, the node force exits with error code 1.
	ShutdownTimeout time.Duration

	// Applications is the list of applications to load and start automatically.
	// Applications are started after the node becomes operational.
	// Empty means no applications auto-started.
	Applications []ApplicationBehavior

	// Env configures node-level environment variables.
	// These are inherited by all processes (lowest priority in inheritance chain).
	// Can be overridden by application, process, or parent environment.
	Env map[Env]any

	// Network configures distributed communication settings.
	// Includes mode, cookie, acceptors, registrar, protocol settings.
	// See NetworkOptions for details.
	Network NetworkOptions

	// Cron configures the cron scheduler for time-based task execution.
	// See CronOptions for details.
	Cron CronOptions

	// CertManager provides TLS certificate management for secure connections.
	// If set, enables TLS for network connections.
	CertManager CertManager

	// TargetManager provides custom link/monitor tracking implementation.
	// If nil, uses default implementation.
	// Advanced option for custom relationship management.
	TargetManager TargetManager

	// Security configures security and information exposure policies.
	// Controls what information is exposed in Info queries and to remote nodes.
	Security SecurityOptions

	// Log configures logging settings for the node and default logger.
	// Includes log level, default logger options, and custom loggers.
	Log LogOptions

	// Version sets the version information for this node.
	// Reported in node.Version() and during network handshakes.
	// Includes Name, Release, License, Commit details.
	Version Version

	// Tracing configures tracing exporters at node startup.
	Tracing TracingOptions

	// Events lists node-level events to register before starting any application.
	// All events declared here are registered with the node as producer
	// (Open is forced to true) and live until the node stops. Use this to
	// establish node-wide event buses that application processes can subscribe
	// to from Init() without the race of waiting for a producer process to
	// register the event first.
	Events []NodeEventSpec
}

// NodeEventSpec declares a node-level event to be registered during node
// startup, before any application starts.
type NodeEventSpec struct {
	// Name is the event name. Must be unique within the node event namespace.
	Name Atom

	// Buffer is the ring buffer size for recent MessageEvent values.
	// Zero means no buffer.
	Buffer int
}

// SecurityOptions controls information exposure and security policies.
type SecurityOptions struct {
	// ExposeEnvInfo includes environment variables in Info() responses.
	// When false, ProcessInfo.Env and NodeInfo.Env are empty.
	// Enable for debugging, disable for production security.
	ExposeEnvInfo bool

	// ExposeEnvRemoteSpawn allows remote-spawned processes to inherit environment.
	// When enabled, processes spawned via RemoteSpawn inherit parent/node environment.
	// When disabled, remote-spawned processes get empty environment (more secure).
	ExposeEnvRemoteSpawn bool

	// ExposeEnvRemoteApplicationStart allows remotely-started applications to access environment.
	// When enabled, applications started via RemoteApplicationStart can access node environment.
	// When disabled, remote applications get empty environment (more secure).
	ExposeEnvRemoteApplicationStart bool
}

// LogOptions configures logging settings for the node.
type LogOptions struct {
	// Level sets the default logging level for the node.
	// Default: LogLevelInfo. Processes inherit this unless overridden.
	Level LogLevel

	// DefaultLogger configures the built-in logger (console/JSON output).
	// Can be disabled to use only custom loggers.
	DefaultLogger DefaultLoggerOptions

	// Loggers is a list of custom loggers to register on node start.
	// Each logger receives log messages based on its filter.
	Loggers []Logger
}

// Logger defines a custom logger to register on node start.
type Logger struct {
	// Name is the unique identifier for this logger.
	Name string

	// Logger is the LoggerBehavior implementation.
	Logger LoggerBehavior

	// Filter specifies which log levels this logger receives.
	// Empty means receive all log levels.
	Filter []LogLevel
}

// Compression configures message compression settings for processes.
// Used in ProcessOptions to reduce network traffic for messages sent by the process.
// Compression is applied automatically when message size exceeds threshold.
type Compression struct {
	// Enable activates compression for outgoing messages.
	// Messages exceeding Threshold will be compressed before sending.
	Enable bool

	// Type specifies the compression algorithm.
	// CompressionTypeGZIP (default) - good balance of speed and size
	// CompressionTypeZLIB - similar to GZIP, slightly different format
	// CompressionTypeLZW - faster but lower compression ratio
	Type CompressionType

	// Level specifies the compression level (speed vs size trade-off).
	// CompressionDefault (0) - balanced (default)
	// CompressionBestSpeed (1) - faster compression, larger size
	// CompressionBestSize (2) - slower compression, smaller size
	Level CompressionLevel

	// Threshold is the minimum message size (in bytes) to trigger compression.
	// Messages smaller than this are sent uncompressed.
	// Default: DefaultCompressionThreshold (1024 bytes).
	// Set higher to avoid compressing small messages (compression overhead not worth it).
	Threshold int
}

// CompressionLevel represents compression speed vs size trade-off.
type CompressionLevel int

// CompressionType represents the compression algorithm.
type CompressionType string

func (cl CompressionLevel) String() string {
	switch cl {
	case CompressionBestSize:
		return "best size"
	case CompressionBestSpeed:
		return "best speed"
	case CompressionDefault:
		return "default"
	default:
		return "unknown compression level"
	}
}

func (cl CompressionLevel) MarshalJSON() ([]byte, error) {
	return []byte("\"" + cl.String() + "\""), nil
}

func (ct CompressionType) ID() uint8 {
	switch ct {
	case CompressionTypeLZW:
		return 100
	case CompressionTypeZLIB:
		return 101
	case CompressionTypeGZIP:
		return 102
	default:
		return 0
	}
}

const (
	// CompressionDefault provides balanced compression (speed vs size).
	// Recommended for most use cases.
	CompressionDefault CompressionLevel = 0

	// CompressionBestSpeed prioritizes compression speed over size.
	// Use for high-throughput scenarios where CPU is more important than bandwidth.
	CompressionBestSpeed CompressionLevel = 1

	// CompressionBestSize prioritizes smaller compressed size over speed.
	// Use for bandwidth-constrained networks where compression time is acceptable.
	CompressionBestSize CompressionLevel = 2

	// CompressionTypeGZIP uses GZIP compression algorithm (default).
	// Good balance of compression ratio and speed. Widely supported.
	CompressionTypeGZIP CompressionType = "gzip"

	// CompressionTypeLZW uses Lempel-Ziv-Welch compression algorithm.
	// Faster than GZIP but lower compression ratio. Good for high-throughput.
	CompressionTypeLZW CompressionType = "lzw"

	// CompressionTypeZLIB uses ZLIB compression algorithm.
	// Similar to GZIP with slightly different format. Good compression ratio.
	CompressionTypeZLIB CompressionType = "zlib"
)

// NodeInfo contains comprehensive information about the node state and statistics.
// Retrieved via node.Info(). Provides a complete snapshot for monitoring and debugging.
type NodeInfo struct {
	// Name is the node name.
	Name Atom

	// Uptime is the node uptime in seconds since start.
	Uptime int64

	// Version is the node version information.
	Version Version

	// Framework is the Ergo framework version.
	Framework Version

	// Commercial lists components with commercial licenses (LicenseBSL1).
	Commercial []Version

	// Env contains node environment variables.
	// Only populated if SecurityOptions.ExposeEnvInfo is enabled.
	Env map[Env]any

	// LogLevel is the default logging level for the node.
	LogLevel LogLevel

	// Loggers lists all registered loggers with their configuration.
	Loggers []LoggerInfo

	// Tracing contains node-level tracing configuration.
	Tracing TracingInfo

	// TracingExporters lists all registered tracing exporters.
	TracingExporters []TracingExporterInfo

	// LogMessages contains cumulative log message counts by level.
	// Indexed as: [0]=Trace, [1]=Debug, [2]=Info, [3]=Warning, [4]=Error, [5]=Panic
	LogMessages [6]uint64

	// TracingSpans contains cumulative tracing span counts by kind.
	// Indexed as: [0]=Send, [1]=Request, [2]=Response, [3]=Spawn, [4]=Terminate
	TracingSpans [5]uint64

	// Cron contains cron scheduler information (jobs, schedule, next run).
	Cron CronInfo

	// ProcessesTotal is the total number of processes on this node.
	ProcessesTotal int64

	// ProcessesRunning is the number of processes currently in Running state.
	ProcessesRunning int64

	// ProcessesWaitResponse is the number of processes blocked in a synchronous Call.
	ProcessesWaitResponse int64

	// ProcessesZombee is the number of killed processes (Zombee state).
	ProcessesZombee int64

	// ProcessesSpawned is the cumulative number of successfully spawned processes.
	ProcessesSpawned uint64

	// ProcessesSpawnFailed is the cumulative number of failed spawn attempts.
	ProcessesSpawnFailed uint64

	// ProcessesTerminated is the cumulative number of terminated processes.
	ProcessesTerminated uint64

	// SendErrorsLocal is the cumulative number of local send delivery errors.
	SendErrorsLocal uint64

	// SendErrorsRemote is the cumulative number of remote send delivery errors.
	SendErrorsRemote uint64

	// CallErrorsLocal is the cumulative number of local call delivery errors.
	CallErrorsLocal uint64

	// CallErrorsRemote is the cumulative number of remote call delivery errors.
	CallErrorsRemote uint64

	// RegisteredAliases is the total number of registered aliases.
	RegisteredAliases int64

	// RegisteredNames is the total number of registered process names.
	RegisteredNames int64

	// RegisteredEvents is the total number of registered events.
	RegisteredEvents int64

	// EventsPublished is the cumulative number of events published by local producers.
	EventsPublished int64

	// EventsReceived is the cumulative number of events received from remote nodes.
	EventsReceived int64

	// EventsLocalSent is the cumulative number of event messages sent to local subscribers.
	EventsLocalSent int64

	// EventsRemoteSent is the cumulative number of event messages sent to remote subscribers.
	EventsRemoteSent int64

	// ApplicationsTotal is the total number of loaded applications.
	ApplicationsTotal int64

	// ApplicationsRunning is the number of currently running applications.
	ApplicationsRunning int64

	// MemoryUsed is the total memory obtained from the OS, in bytes.
	MemoryUsed uint64

	// MemoryAlloc is the memory occupied by live heap objects, in bytes.
	MemoryAlloc uint64

	// MemoryLimit is the soft memory limit set via GOMEMLIMIT, in bytes.
	// MaxInt64 means no limit is set.
	MemoryLimit uint64

	// HeapLive is the heap memory occupied as of the last garbage collection, in bytes.
	HeapLive uint64

	// HeapGoal is the heap size that triggers the next garbage collection, in bytes.
	HeapGoal uint64

	// Goroutines is the current number of goroutines.
	Goroutines int64

	// GCCycles is the cumulative number of completed garbage collection cycles.
	GCCycles uint64

	// CPUTimeGC is the cumulative CPU time spent in garbage collection, in seconds.
	CPUTimeGC float64

	// CPUTimeTotal is the cumulative CPU time available to the process, in seconds,
	// as defined by GOMAXPROCS. Includes idle time.
	CPUTimeTotal float64

	// UserTime is the user CPU time in nanoseconds.
	UserTime int64

	// SystemTime is the system CPU time in nanoseconds.
	SystemTime int64

	// ServerTime is the current server time with timezone.
	// Useful in Observer and MCP for correlating logs across nodes in different timezones.
	ServerTime time.Time
}

// NodeShortInfo contains essential information about a node.
// Retrieved via node.ShortInfo().
type NodeShortInfo struct {
	// Name is the node name.
	Name Atom

	// Creation is the node incarnation identifier. A different value means
	// the node has been restarted.
	Creation int64

	// Uptime is the node uptime in seconds since start.
	Uptime int64

	// Version is the node version information.
	Version Version

	// Framework is the Ergo framework version.
	Framework Version

	// Mode is the current network mode (Enabled, Hidden, or Disabled).
	Mode NetworkMode

	// LogLevel is the default logging level for the node.
	LogLevel LogLevel

	// ProcessesTotal is the total number of processes on this node.
	ProcessesTotal int64

	// ProcessesRunning is the number of processes currently in Running state.
	ProcessesRunning int64

	// ProcessesWaitResponse is the number of processes blocked in a synchronous Call.
	ProcessesWaitResponse int64

	// ProcessesZombee is the number of killed processes (Zombee state).
	ProcessesZombee int64

	// ProcessesSpawned is the cumulative number of successfully spawned processes.
	ProcessesSpawned uint64

	// ProcessesSpawnFailed is the cumulative number of failed spawn attempts.
	ProcessesSpawnFailed uint64

	// ProcessesTerminated is the cumulative number of terminated processes.
	ProcessesTerminated uint64

	// ApplicationsTotal is the total number of loaded applications.
	ApplicationsTotal int64

	// ApplicationsRunning is the number of currently running applications.
	ApplicationsRunning int64

	// SendErrorsLocal is the cumulative number of local send delivery errors.
	SendErrorsLocal uint64

	// SendErrorsRemote is the cumulative number of remote send delivery errors.
	SendErrorsRemote uint64

	// CallErrorsLocal is the cumulative number of local call delivery errors.
	CallErrorsLocal uint64

	// CallErrorsRemote is the cumulative number of remote call delivery errors.
	CallErrorsRemote uint64

	// LogMessages contains cumulative log message counts by level.
	// Indexed as: [0]=Trace, [1]=Debug, [2]=Info, [3]=Warning, [4]=Error, [5]=Panic
	LogMessages [6]uint64

	// MemoryUsed is the total memory obtained from the OS, in bytes.
	MemoryUsed uint64

	// MemoryAlloc is the memory occupied by live heap objects, in bytes.
	MemoryAlloc uint64

	// MemoryLimit is the soft memory limit set via GOMEMLIMIT, in bytes.
	// MaxInt64 means no limit is set.
	MemoryLimit uint64

	// HeapLive is the heap memory occupied as of the last garbage collection, in bytes.
	HeapLive uint64

	// HeapGoal is the heap size that triggers the next garbage collection, in bytes.
	HeapGoal uint64

	// Goroutines is the current number of goroutines.
	Goroutines int64

	// GCCycles is the cumulative number of completed garbage collection cycles.
	GCCycles uint64

	// CPUTimeGC is the cumulative CPU time spent in garbage collection, in seconds.
	CPUTimeGC float64

	// CPUTimeTotal is the cumulative CPU time available to the process, in seconds,
	// as defined by GOMAXPROCS. Includes idle time.
	CPUTimeTotal float64

	// UserTime is the user CPU time in nanoseconds.
	UserTime int64

	// SystemTime is the system CPU time in nanoseconds.
	SystemTime int64

	// Applications lists the applications loaded on this node. The set of
	// applications is what makes a node's role, so it groups nodes the way
	// naming conventions cannot.
	Applications []Atom

	// Peers describes the connections this node currently has.
	Peers []RemoteNodeShortInfo

	// ServerTime is the current server time with timezone.
	ServerTime time.Time
}

// LoggerInfo describes a registered logger.
// Part of NodeInfo. Shows which loggers are active and their configuration.
type LoggerInfo struct {
	// Name is the unique logger identifier.
	Name string

	// Behavior is the logger type name (e.g., "ColoredLogger", "RotateLogger").
	// Empty for process-based loggers.
	Behavior string

	// Levels lists the log levels this logger is filtering.
	// Empty means logger receives all log levels.
	Levels []LogLevel
}

// TracingExporterInfo contains information about a registered tracing exporter.
type TracingExporterInfo struct {
	// Name is the unique exporter identifier.
	Name string

	// Behavior is the exporter type name.
	Behavior string

	// Flags is the tracing granularity for this exporter.
	Flags TracingFlags

	// DroppedSpans counts spans dropped because the exporter's queue was full
	// (a slow or blocked object exporter). Always zero for process-based exporters.
	DroppedSpans uint64
}
