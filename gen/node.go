package gen

import (
	"time"
)

type Node interface {
	// Name returns node name
	Name() Atom
	// IsAlive returns true if node is still alive
	IsAlive() bool
	// Uptime returns node uptime in seconds. Returns 0 if node is stopped.
	Uptime() int64
	// Version returns node version
	Version() Version
	// FrameworkVersion returns framework version
	FrameworkVersion() Version
	// Info returns summary information about this node
	Info() (NodeInfo, error)
	// EnvList returns a map of configured Node environment variables.
	EnvList() map[Env]any
	// SetEnv set node environment variable with given name. Use nil value to remove variable with given name.
	SetEnv(name Env, value any)
	// Env returns value associated with given environment name.
	Env(name Env) (any, bool)

	// Spawn spawns a new process
	Spawn(factory ProcessFactory, options ProcessOptions, args ...any) (PID, error)
	// SpawnRegister spawns a new process and register associated name with it
	SpawnRegister(register Atom, factory ProcessFactory, options ProcessOptions, args ...any) (PID, error)

	// RegisterName register associates the name with the given PID so you can address messages
	// to this process using gen.ProcessID{<name>, <nodename>}. Returns error if this process
	// is already has registered name.
	RegisterName(name Atom, pid PID) error

	// UnregisterName unregister associated name. Returns PID that this name belonged to.
	UnregisterName(name Atom) (PID, error)

	// MetaInfo returns summary information about given meta process
	MetaInfo(meta Alias) (MetaInfo, error)

	// ProcessInfo returns short summary information about given process
	ProcessInfo(pid PID) (ProcessInfo, error)

	// ProcessList returns the list of the processes
	ProcessList() ([]PID, error)

	// ProcessListShortInfo returns the list of the processes with short information
	// for the given range of process identifiers (gen.PID.ID)
	ProcessListShortInfo(start, limit int) ([]ProcessShortInfo, error)

	// ProcessState returns state of the given process:
	// - ProcessStateSleep (process has no messages)
	// - ProcessStateRunning (process is handling its mailbox)
	// - ProcessStateTerminated (final state of the process lifespan before it will be removed
	//   removed from the node)
	// - ProcessStateZombee (process was killed by node being in the running state).
	ProcessState(pid PID) (ProcessState, error)

	// ApplicationLoad loads application to the node. To start it use ApplicationStart method.
	// Returns the name of loaded application. Returns error gen.ErrTaken if application name
	// is already registered in the node.
	ApplicationLoad(app ApplicationBehavior, args ...any) (Atom, error)

	// ApplcationInfo returns the short information about the given application.
	// Returns error gen.ErrApplicationUnknown if it does not exist in the node
	ApplicationInfo(name Atom) (ApplicationInfo, error)

	// ApplicationUnload unloads application from the node. Returns gen.ErrApplicationRunning
	// if given application is already started (must be stopped before the unloading).
	// Or returns error gen.ErrApplicationUnknown if it does not exist in the node.
	ApplicationUnload(name Atom) error

	// ApplicationStart starts application with its children processes. Starting mode is according
	// to the defined in the gen.ApplicationSpec.Mode
	ApplicationStart(name Atom, options ApplicationOptions) error

	// ApplicationStartTemporary starts application in temporary mode overriding the value
	// of gen.ApplicationSpec.Mode
	ApplicationStartTemporary(name Atom, options ApplicationOptions) error

	// ApplicationStartTransient starts application in transient mode overriding the value
	// of gen.ApplicationSpec.Mode
	ApplicationStartTransient(name Atom, options ApplicationOptions) error

	// ApplicationStartPermanent starts application in permanent mode overriding the value
	// of gen.ApplicationSpec.Mode
	ApplicationStartPermanent(name Atom, options ApplicationOptions) error

	// ApplicationStop stops the given applications, awaiting all children to be stopped.
	// The default waiting time is 5 seconds. Returns error gen.ErrApplicationStopping
	// if application is still stopping. Once the application is stopped it can be unloaded
	// from the node using ApplicationUnload.
	ApplicationStop(name Atom) error

	// ApplicationStopForce force to kill all children, no awaiting the termination of children processes.
	ApplicationStopForce(name Atom) error

	// ApplicationStopWithTimeout stops the given applications, awaiting all children to be stopped
	// during the given period of time. Returns gen.ErrApplicationStopping on timeout.
	ApplicationStopWithTimeout(name Atom, timeout time.Duration) error

	// Applications return list of all applications (loaded and started).
	Applications() []Atom
	// ApplicationsRunning return list of all running applications
	ApplicationsRunning() []Atom

	NetworkStart(options NetworkOptions) error
	NetworkStop() error
	Network() Network

	CertManager() CertManager

	Security() SecurityOptions

	Stop()
	StopForce()

	// Wait waits node termination
	Wait()
	// WaitWithTimeout waits node termination with the given period of time
	WaitWithTimeout(timeout time.Duration) error

	// Kill terminates the given process. Can be used for the local process only.
	Kill(pid PID) error

	// Send sends a message to the given process.
	Send(to any, message any) error

	// SendEvent sends event message to the subscribers (to the processes that made link/monitor
	// on this event). Event must be registered with RegisterEvent method.
	SendEvent(name Atom, token Ref, options MessageOptions, message any) error

	// RegisterEvent registers a new event. Returns a reference as the token
	// for sending events. Unregistering this event is allowed to the node only.
	// Sending an event can be done using SendEvent method with the provided token.
	RegisterEvent(name Atom, options EventOptions) (Ref, error)

	// UnregisterEvent unregisters an event. Can be used for the registered
	// events by the node.
	UnregisterEvent(name Atom) error

	// SendExit sends graceful termination request to the process.
	SendExit(pid PID, reason error) error

	// Log returns gen.Log interface
	Log() Log

	// LogLevelProcess returns logging level for the given process
	LogLevelProcess(pid PID) (LogLevel, error)

	// SetLogLevelProcess allows set logging level for the given process
	SetLogLevelProcess(pid PID, level LogLevel) error

	// LogLevelMeta returns logging level for the given meta process
	LogLevelMeta(meta Alias) (LogLevel, error)

	// SetLogLevelMeta allows set logging level for the given meta process
	SetLogLevelMeta(meta Alias, level LogLevel) error

	// Loggers returns list of loggers
	Loggers() []string

	// LoggerAdd makes process to receive log messages gen.MessageLogNode, gen.MessageLogProcess
	LoggerAddPID(pid PID, name string, filter ...LogLevel) error

	// LoggerAdd allows to add a custom logger
	LoggerAdd(name string, logger LoggerBehavior, filter ...LogLevel) error

	// LoggerDelete removes process from the loggers list
	LoggerDeletePID(pid PID)

	// LoggerDeleteCustom removes custom logger from the logger list
	LoggerDelete(name string)

	// LoggerLevels returns list of log levels for the given logger
	LoggerLevels(name string) []LogLevel

	// MakeRef creates an unique reference within this node
	MakeRef() Ref

	// Commercial returns list of component versions with a commercial license (gen.LicenseBSL1)
	Commercial() []Version

	// PID returns virtual PID of the core. This PID is using as a source
	// for the messages sent by node using methods Send, SendExit or SendEvent
	// and as a parent PID for the spawned process by the node
	PID() PID
	Creation() int64

	// SetCTRLC allows you to catch Ctrl+C to enable/disable debug level for the node
	// Twice Ctrl+C - to stop node gracefully
	SetCTRLC(enable bool)
}

// NodeRegistrar bridge interface from Node to the Registrar
type NodeRegistrar interface {
	Name() Atom
	RegisterEvent(name Atom, options EventOptions) (Ref, error)
	UnregisterEvent(name Atom) error
	SendEvent(name Atom, token Ref, options MessageOptions, message any) error
	Log() Log
}

// NodeHandshake bridge interface from Node to the Handshake
type NodeHandshake interface {
	Name() Atom
	Creation() int64
	Version() Version
}

// There is no NodeProto bridge interface. gen.Core is used for that.

// NodeOptions defines bootstrapping options for the node
type NodeOptions struct {
	// Applications application list that must be started
	Applications []ApplicationBehavior
	// Env node environment
	Env map[Env]any
	// Network
	Network NetworkOptions
	// CertManager
	CertManager CertManager
	// Security options
	Security SecurityOptions
	// Log options for the defaulf logger
	Log LogOptions
	// Version sets the version details for your node
	Version Version
}

type SecurityOptions struct {
	ExposeEnvInfo bool
	// ExposeEnvRemoteSpawn makes remote spawned process inherit env from the parent process/node
	ExposeEnvRemoteSpawn            bool
	ExposeEnvRemoteApplicationStart bool
}

// LogOptions
type LogOptions struct {
	// Level default logging level for node
	Level LogLevel

	// loggers options

	// DefaultLogger options
	DefaultLogger DefaultLoggerOptions

	// Loggers add extra loggers on start
	Loggers []Logger
}

type Logger struct {
	Name   string
	Logger LoggerBehavior
	Filter []LogLevel
}

type Compression struct {
	// Enable enables compression for all outgoing messages having size
	// greater than the defined threshold.
	Enable bool
	// Type defines type of compression. Use gen.CompressionTypeZLIB or gen.CompressionTypeLZW. By default is using gen.CompressionTypeGZIP
	Type CompressionType
	// Level defines compression level. Use gen.CompressionBestSize or gen.CompressionBestSpeed. By default is using gen.CompressionDefault
	Level CompressionLevel
	// Threshold defines the minimal message size for the compression.
	// Messages less of this threshold will not be compressed.
	Threshold int
}

type CompressionLevel int
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
	CompressionDefault   CompressionLevel = 0
	CompressionBestSpeed CompressionLevel = 1
	CompressionBestSize  CompressionLevel = 2

	CompressionTypeGZIP CompressionType = "gzip"
	CompressionTypeLZW  CompressionType = "lzw"
	CompressionTypeZLIB CompressionType = "zlib"
)

type NodeInfo struct {
	Name       Atom
	Uptime     int64
	Version    Version
	Framework  Version
	Commercial []Version

	Env      map[Env]any // gen.NodeOptions.Security.ExposeEnvInfo must be enabled to reveal this data
	LogLevel LogLevel
	Loggers  []LoggerInfo

	ProcessesTotal   int64
	ProcessesRunning int64
	ProcessesZombee  int64

	RegisteredAliases int64
	RegisteredNames   int64
	RegisteredEvents  int64

	ApplicationsTotal   int64
	ApplicationsRunning int64

	MemoryUsed  uint64
	MemoryAlloc uint64

	UserTime   int64
	SystemTime int64
}

type LoggerInfo struct {
	Name     string
	Behavior string
	Levels   []LogLevel
}
