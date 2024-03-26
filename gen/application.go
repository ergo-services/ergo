package gen

type ApplicationMode int

const (
	// TODO add details here
	// ApplicationModeTemporary
	ApplicationModeTemporary ApplicationMode = 1

	// TODO
	// ApplicationModeTransient
	ApplicationModeTransient ApplicationMode = 2

	// TODO
	// ApplicationModePermanent
	ApplicationModePermanent ApplicationMode = 3
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
	State       string
	Uptime      int64
	Group       []PID
}
