package gen

type ApplicationMode int
type ApplicationState int32

const (
	ApplicationModeTemporary ApplicationMode = 1
	ApplicationModeTransient ApplicationMode = 2
	ApplicationModePermanent ApplicationMode = 3

	ApplicationStateLoaded   ApplicationState = 1
	ApplicationStateRunning  ApplicationState = 2
	ApplicationStateStopping ApplicationState = 3
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

func (as ApplicationState) String() string {
	switch as {
	case ApplicationStateStopping:
		return "stopping"
	case ApplicationStateRunning:
		return "running"
	default:
		return "loaded"
	}
}

func (as ApplicationState) MarshalJSON() ([]byte, error) {
	return []byte("\"" + as.String() + "\""), nil
}

type ApplicationBehavior interface {
	// Load invoked on loading application using method ApplicationLoad of gen.Node interface.
	Load(node Node, args ...any) (ApplicationSpec, error)
	// Start invoked once the application started
	Start(mode ApplicationMode)
	// Terminate invoked once the application stopped
	Terminate(reason error)
}

type ApplicationOptions struct {
	Env      map[Env]any
	LogLevel LogLevel
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

	// Env contains application-level environment variables.
	// Inherited by all processes within the application.
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

	// Env contains application-level environment variables.
	// Only populated if NodeOptions.Security.ExposeEnvInfo is enabled.
	Env map[Env]any

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
