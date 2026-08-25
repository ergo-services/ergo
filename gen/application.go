package gen

import (
	"fmt"
	"time"
)

type ApplicationMode int
type ApplicationState int32

const (
	ApplicationModeTemporary ApplicationMode = 1
	ApplicationModeTransient ApplicationMode = 2
	ApplicationModePermanent ApplicationMode = 3

	ApplicationStateLoaded   ApplicationState = 1
	ApplicationStateRunning  ApplicationState = 2
	ApplicationStateStopping ApplicationState = 3

	// internal state transition from "load" to "running"; not exposed to the registrar
	ApplicationStateInitializing ApplicationState = 10
)

func (am ApplicationMode) String() string {
	switch am {
	case ApplicationModePermanent:
		return "permanent"
	case ApplicationModeTransient:
		return "transient"
	default:
		return "temporary"
	}
}

func (am ApplicationMode) MarshalJSON() ([]byte, error) {
	return []byte("\"" + am.String() + "\""), nil
}

func (am *ApplicationMode) UnmarshalJSON(data []byte) error {
	s, err := unmarshalName(data)
	if err != nil {
		return err
	}
	switch s {
	case "permanent":
		*am = ApplicationModePermanent
	case "transient":
		*am = ApplicationModeTransient
	case "temporary":
		*am = ApplicationModeTemporary
	default:
		return fmt.Errorf("unknown application mode %q", s)
	}
	return nil
}

func (as ApplicationState) String() string {
	switch as {
	case ApplicationStateInitializing:
		return "initializing"
	case ApplicationStateRunning:
		return "running"
	case ApplicationStateStopping:
		return "stopping"
	default:
		return "loaded"
	}
}

func (as ApplicationState) MarshalJSON() ([]byte, error) {
	return []byte("\"" + as.String() + "\""), nil
}

func (as *ApplicationState) UnmarshalJSON(data []byte) error {
	s, err := unmarshalName(data)
	if err != nil {
		return err
	}
	switch s {
	case "loaded":
		*as = ApplicationStateLoaded
	case "initializing":
		*as = ApplicationStateInitializing
	case "running":
		*as = ApplicationStateRunning
	case "stopping":
		*as = ApplicationStateStopping
	default:
		return fmt.Errorf("unknown application state %q", s)
	}
	return nil
}

// Application is the runtime view of a loaded application on a node.
// Returned by Process.Application() so processes can introspect the
// containing application and mutate dynamic fields (tags, weight) that
// propagate to the registrar. Also passed to ApplicationBehavior.PreLoad
// for the embedded base to bind.
type Application interface {
	Name() Atom
	Mode() ApplicationMode
	State() ApplicationState
	Node() Node
	Log() Log

	// Env returns a value from the application's effective environment. While the
	// application is running this is the node core env, ApplicationSpec.Env, and the
	// per-start ApplicationOptions.Env merged in that order (later layers win); while
	// not running it is ApplicationSpec.Env.
	Env(key Env) (any, bool)
	// EnvList returns a copy of the application's effective environment (see Env).
	EnvList() map[Env]any

	Tags() []Atom
	AddTag(tag Atom) error
	RemoveTag(tag Atom) error
	SetTags(tags []Atom) error

	Weight() int
	// SetWeight updates the route weight. A negative weight takes the
	// application out of resolve results (out of rotation); any non-negative
	// weight restores it.
	SetWeight(w int) error

	// Behavior returns the application behavior implementation.
	// Used by app.Application.PreLoad to dispatch the user's Load callback.
	Behavior() ApplicationBehavior
}

// ApplicationBehavior is the application lifecycle interface.
// User types must embed app.Application to inherit PreLoad and default
// no-op implementations of Init/Start/Stop/Terminate, then implement Load.
type ApplicationBehavior interface {
	// PreLoad is the framework entry point. Implemented by app.Application
	// via embed: binds the runtime application and dispatches to Load.
	// DO NOT OVERRIDE.
	PreLoad(app Application, args ...any) (ApplicationSpec, error)

	// Load returns the application spec. User must implement.
	// The runtime Application is bound when Load is called; use a.Log(),
	// a.Node(), etc. via the app.Application embed.
	Load(args ...any) (ApplicationSpec, error)

	// Init runs pre-start: open resources needed by Group processes.
	// ref.IsAlive() == false means InitTimeout exceeded; unwind and return
	// gen.ErrTimeout. Non-nil error aborts start; Terminate is NOT called.
	Init(ref Ref, mode ApplicationMode) error

	// Start runs post-start: register health checks, export metrics.
	// ref deadline is StartTimeout.
	Start(ref Ref, mode ApplicationMode)

	// Stop runs pre-stop: drain, deregister, flush. Blocks subsequent
	// Group exit until return or StopTimeout expiry.
	Stop(ref Ref, reason error)

	// Terminate runs post-stop: close resources opened in Init.
	Terminate(reason error)
}

type ApplicationOptions struct {
	// Env is a per-start set of environment variables merged on top of the node core
	// env and ApplicationSpec.Env for this start. Inherited by the application's
	// processes and reflected by Application.Env()/EnvList() while running. Each start
	// supplies its own Env; values do not carry over between starts.
	Env      map[Env]any
	LogLevel LogLevel

	// InitTimeout overrides ApplicationSpec.InitTimeout if non-zero.
	InitTimeout time.Duration

	// StartTimeout overrides ApplicationSpec.StartTimeout if non-zero.
	StartTimeout time.Duration

	// StopTimeout overrides ApplicationSpec.StopTimeout if non-zero.
	StopTimeout time.Duration
}

type ApplicationOptionsExtra struct {
	ApplicationOptions
	CorePID      PID
	CoreEnv      map[Env]any
	CoreLogLevel LogLevel
}

// ApplicationSpec defines the configuration for an application.
// Used in ApplicationBehavior.Load() to specify application structure and behavior.
type ApplicationSpec struct {
	// Name is the unique application identifier.
	// Application names exist in a separate namespace from process names.
	// An application and a process can have the same name without conflict.
	Name Atom

	// Description provides human-readable information about the application.
	Description string

	// Version specifies the application version information.
	Version Version

	// Depends lists application dependencies (other applications or network).
	Depends ApplicationDepends

	// Network declares wire-format values this application contributes
	// to the node's network. Processed during ApplicationLoad, before
	// any process in the application is spawned. Entries are silently
	// ignored if the node's network mode is NetworkModeDisabled.
	Network ApplicationNetwork

	// Env contains application-level environment variables.
	// Inherited by all processes within the application.
	// With the node env (merged at load) it forms the base of the effective
	// environment reported by Application.Env()/EnvList().
	Env map[Env]any

	// Group lists the processes that belong to this application.
	// Started as part of the application lifecycle.
	Group []ApplicationMemberSpec

	// Mode defines the application starting mode (Temporary, Transient, Permanent).
	Mode ApplicationMode

	// Weight is used for load balancing across multiple application instances.
	// Available via resolver (in ApplicationRoute) or ApplicationInfo.
	// Higher weight indicates this instance should be preferred when selecting among instances.
	// Clients use weight to make routing decisions and distribute load.
	Weight int

	// Tags is a list of labels for categorizing this application instance.
	// Published to registrar for service discovery and instance selection.
	// Used for deployment strategies (blue/green, canary) or operational states (maintenance).
	// Examples: "blue", "green", "canary", "stable", "maintenance".
	Tags []Atom

	// Map is a key-value mapping of logical roles to process names within the application.
	// Allows looking up process names by role, then using the name to communicate with the process.
	// Example: map["api"] = "api_server" lets you find the name, then Send/Call to "api_server".
	Map map[string]Atom

	// LogLevel sets the default logging level for application processes.
	LogLevel LogLevel

	// InitTimeout limits the Init callback. 0 -> DefaultApplicationInitTimeout.
	InitTimeout time.Duration

	// StartTimeout limits the Start callback. 0 -> DefaultApplicationStartTimeout.
	StartTimeout time.Duration

	// StopTimeout limits the Stop callback. 0 -> DefaultApplicationStopTimeout.
	StopTimeout time.Duration
}

// ApplicationMemberSpec defines a process that belongs to an application.
// Part of ApplicationSpec.Group. Specifies how to spawn the process.
type ApplicationMemberSpec struct {
	// Factory creates the process behavior instance.
	Factory ProcessFactory

	// Options configures process spawn settings.
	Options ProcessOptions

	// Name is the registered process name.
	Name Atom

	// Args are passed to the process Init callback.
	Args []any
}

// ApplicationDepends specifies application dependencies.
// Part of ApplicationSpec. Controls when application can start.
type ApplicationDepends struct {
	// Applications lists other applications that must be running before this one starts.
	Applications []Atom

	// Network indicates if network connectivity is required before starting.
	Network bool
}

// ApplicationNetwork groups the network-scoped declarations of an
// application. All entries are processed during ApplicationLoad,
// before any process in the application is spawned, and only when
// the node's network mode is not NetworkModeDisabled.
type ApplicationNetwork struct {
	// RegisterTypes are Go types registered with every TypeRegistry-capable
	// proto on the node. Order is irrelevant; protos resolve inter-type
	// dependencies internally.
	RegisterTypes []any

	// RegisterErrors are sentinel errors registered for wire transport.
	RegisterErrors []error

	// RegisterAtoms are atoms registered in the wire-format atom cache.
	RegisterAtoms []Atom
}

// ApplicationInfo contains runtime information about an application.
// Retrieved via Node.ApplicationInfo() or RemoteNode.ApplicationInfo().
type ApplicationInfo struct {
	// Name is the application identifier.
	// Application names exist in a separate namespace from process names.
	Name Atom

	// Weight is used for load balancing across multiple application instances.
	// Available via resolver (in ApplicationRoute) or ApplicationInfo.
	// Higher weight indicates this instance should be preferred when selecting among instances.
	Weight int

	// Tags is a list of labels for filtering and selecting application instances.
	// Used for deployment strategies like blue/green, canary, or marking maintenance mode.
	// Allows choosing specific application instances based on tags when multiple are available.
	// Examples: "blue", "green", "canary", "stable", "maintenance".
	Tags []Atom

	// Map is a key-value mapping of logical roles to process names within the application.
	// Allows looking up process names by role, then using the name to communicate with the process.
	// Example: map["api"] = "api_server" lets you find the name, then Send/Call to "api_server".
	Map map[string]Atom

	// Description provides human-readable information about the application.
	Description string

	// Version specifies the application version.
	Version Version

	// Env is the application's effective environment (see Application.Env).
	// Only populated if NodeOptions.Security.ExposeEnvInfo is enabled.
	Env map[Env]any `sentinel:"empty unless NodeOptions.Security.ExposeEnvInfo is enabled"`

	// Depends lists application dependencies.
	Depends ApplicationDepends

	// Mode is the application starting mode.
	Mode ApplicationMode

	// State is the current application state (Loaded, Running, Stopped).
	State ApplicationState

	// Parent is the node name that started this application.
	// For local starts, this is the local node name.
	// For remote starts via RemoteNode.ApplicationStart(), this is the requesting node name.
	Parent Atom

	// Uptime is the number of seconds since application started.
	Uptime int64

	// Group lists all process PIDs belonging to this application.
	Group []PID
}
