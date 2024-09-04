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

// ApplicationSpec
type ApplicationSpec struct {
	Name        Atom
	Description string
	Version     Version
	Depends     ApplicationDepends
	Env         map[Env]any
	Group       []ApplicationMemberSpec
	Mode        ApplicationMode
	Weight      int
	LogLevel    LogLevel
}

// ApplicationMemberSpec
type ApplicationMemberSpec struct {
	Factory ProcessFactory
	Options ProcessOptions
	Name    Atom
	Args    []any
}

type ApplicationDepends struct {
	Applications []Atom
	Network      bool
}

type ApplicationInfo struct {
	Name        Atom
	Weight      int
	Description string
	Version     Version
	Env         map[Env]any
	Depends     ApplicationDepends
	Mode        ApplicationMode
	State       ApplicationState
	Parent      Atom
	Uptime      int64
	Group       []PID
}
