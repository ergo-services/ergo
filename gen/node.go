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
	SpawnRegister(
		register Atom,
		factory ProcessFactory,
		options ProcessOptions,
		args ...any,
	) (PID, error)

	// RegisterName associates a name with the given PID.
	// The process can then be addressed using ProcessID{name, nodename}.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrTaken if name already taken.
	RegisterName(name Atom, pid PID) error

	// UnregisterName removes the name association.
	// Returns the PID that was associated with this name.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states, ErrProcessUnknown if name not found.
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
	// More efficient than ProcessList + ProcessInfo for each.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ProcessListShortInfo(start, limit int) ([]ProcessShortInfo, error)

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

	// ApplicationProcessList returns all processes belonging to the application.
	// Includes all processes started by the application and its children (recursive).
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ApplicationProcessList(name Atom, limit int) ([]PID, error)

	// ApplicationProcessListShortInfo returns process list with essential information.
	// Includes all processes from application and its children.
	// More efficient than ApplicationProcessList + ProcessInfo for each.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	ApplicationProcessListShortInfo(name Atom, limit int) ([]ProcessShortInfo, error)

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
	// Waits for all processes and applications to terminate.
	// Can be called from any state (idempotent).
	Stop()

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
	// Important delivery ensures request reaches the target process mailbox.
	// Provides network transparency for error detection:
	// - Without Important: timeout if remote process doesn't exist (ambiguous - slow or missing?)
	// - With Important: immediate ErrProcessUnknown if remote process doesn't exist
	// Aligns remote error handling with local delivery (local always returns immediate error).
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

	// Log returns the node's logger interface.
	// Available in all states.
	Log() Log

	// LogLevelProcess returns the logging level for the given process.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	LogLevelProcess(pid PID) (LogLevel, error)

	// SetLogLevelProcess sets the logging level for the given process.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	SetLogLevelProcess(pid PID, level LogLevel) error

	// LogLevelMeta returns the logging level for the given meta process.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	LogLevelMeta(meta Alias) (LogLevel, error)

	// SetLogLevelMeta sets the logging level for the given meta process.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	SetLogLevelMeta(meta Alias, level LogLevel) error

	// Loggers returns a list of registered logger names.
	// Available in all states.
	Loggers() []string

	// LoggerAddPID registers a process as a logger.
	// The process will receive MessageLogNode and MessageLogProcess messages.
	// Optional filter specifies which log levels to receive.
	// Available in: Running state only.
	// Returns ErrNodeTerminated in other states.
	LoggerAddPID(pid PID, name string, filter ...LogLevel) error

	// LoggerAdd registers a custom logger implementation.
	// Optional filter specifies which log levels to send to this logger.
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
	SetEnv(name Env, value any)
	RegisterEvent(name Atom, options EventOptions) (Ref, error)
	UnregisterEvent(name Atom) error
	SendEvent(name Atom, token Ref, options MessageOptions, message any) error
	Log() Log
	Stop()
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

	// Cron contains cron scheduler information (jobs, schedule, next run).
	Cron CronInfo

	// ProcessesTotal is the total number of processes on this node.
	ProcessesTotal int64

	// ProcessesRunning is the number of processes currently in Running state.
	ProcessesRunning int64

	// ProcessesZombee is the number of killed processes (Zombee state).
	ProcessesZombee int64

	// RegisteredAliases is the total number of registered aliases.
	RegisteredAliases int64

	// RegisteredNames is the total number of registered process names.
	RegisteredNames int64

	// RegisteredEvents is the total number of registered events.
	RegisteredEvents int64

	// ApplicationsTotal is the total number of loaded applications.
	ApplicationsTotal int64

	// ApplicationsRunning is the number of currently running applications.
	ApplicationsRunning int64

	// MemoryUsed is the current memory usage in bytes (from runtime.MemStats.Alloc).
	MemoryUsed uint64

	// MemoryAlloc is the cumulative bytes allocated (from runtime.MemStats.TotalAlloc).
	MemoryAlloc uint64

	// UserTime is the user CPU time in nanoseconds.
	UserTime int64

	// SystemTime is the system CPU time in nanoseconds.
	SystemTime int64
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
