package gen

type Core interface {
	// sending message
	RouteSendPID(from PID, to PID, options MessageOptions, message any) error
	RouteSendProcessID(from PID, to ProcessID, options MessageOptions, message any) error
	RouteSendAlias(from PID, to Alias, options MessageOptions, message any) error

	RouteSendEvent(from PID, token Ref, options MessageOptions, message MessageEvent) error
	RouteSendExit(from PID, to PID, reason error) error
	RouteSendResponse(from PID, to PID, ref Ref, options MessageOptions, message any) error

	// call requests
	RouteCallPID(ref Ref, from PID, to PID, options MessageOptions, message any) error
	RouteCallProcessID(ref Ref, from PID, to ProcessID, options MessageOptions, message any) error
	RouteCallAlias(ref Ref, from PID, to Alias, options MessageOptions, message any) error

	// linking requests
	RouteLinkPID(pid PID, target PID) error
	RouteUnlinkPID(pid PID, target PID) error

	RouteLinkProcessID(pid PID, target ProcessID) error
	RouteUnlinkProcessID(pid PID, target ProcessID) error

	RouteLinkAlias(pid PID, target Alias) error
	RouteUnlinkAlias(pid PID, target Alias) error

	RouteLinkEvent(pid PID, target Event) ([]MessageEvent, error)
	RouteUnlinkEvent(pid PID, target Event) error

	// monitoring requests
	RouteMonitorPID(pid PID, target PID) error
	RouteDemonitorPID(pid PID, target PID) error

	RouteMonitorProcessID(pid PID, target ProcessID) error
	RouteDemonitorProcessID(pid PID, target ProcessID) error

	RouteMonitorAlias(pid PID, target Alias) error
	RouteDemonitorAlias(pid PID, target Alias) error

	RouteMonitorEvent(pid PID, target Event) ([]MessageEvent, error)
	RouteDemonitorEvent(pid PID, target Event) error

	// target termination
	RouteTerminatePID(target PID, reason error) error
	RouteTerminateProcessID(target ProcessID, reason error) error
	RouteTerminateEvent(target Event, reason error) error
	RouteTerminateAlias(terget Alias, reason error) error

	RouteSpawn(node Atom, name Atom, options ProcessOptionsExtra, source Atom) (PID, error)
	RouteApplicationStart(name Atom, mode ApplicationMode, options ApplicationOptionsExtra, source Atom) error

	RouteNodeDown(node Atom, reason error)

	MakeRef() Ref
	Name() Atom
	Creation() int64

	PID() PID
	LogLevel() LogLevel
	Security() Security
	EnvList() map[Env]any
}