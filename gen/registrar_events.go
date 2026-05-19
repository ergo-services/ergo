package gen

// Standard message types published through Registrar.Event().
//
// All registrar implementations that report SupportEvent: true MUST publish
// these message types for the corresponding state transitions. A registrar
// MAY additionally publish vendor-specific message types declared in its
// own package; consult the registrar's documentation.
//
// Subscribe via process.LinkEvent / process.MonitorEvent on the token
// returned by registrar.Event().

// MessageRegistrarNodeJoined is published when a node has been observed
// joining the cluster (registered with the service registry).
type MessageRegistrarNodeJoined struct {
	Name Atom
}

// MessageRegistrarNodeLeft is published when a node has been observed
// leaving the cluster (unregistered, lease expired, or membership removed).
type MessageRegistrarNodeLeft struct {
	Name Atom
}

// MessageRegistrarConfigUpdate is published when a centralized configuration
// value changes. Only registrars that report SupportConfig: true emit this.
type MessageRegistrarConfigUpdate struct {
	Item  string
	Value any
}

// MessageRegistrarApplicationLoaded is published when an application has
// been advertised as loaded on some node. Route.State == ApplicationStateLoaded.
type MessageRegistrarApplicationLoaded struct {
	Route ApplicationRoute
}

// MessageRegistrarApplicationInitializing is published when an application
// is starting its initialization phase on some node.
// Route.State == ApplicationStateInitializing.
type MessageRegistrarApplicationInitializing struct {
	Route ApplicationRoute
}

// MessageRegistrarApplicationStarted is published when an application has
// transitioned to running state on some node. Route.State == ApplicationStateRunning.
type MessageRegistrarApplicationStarted struct {
	Route ApplicationRoute
}

// MessageRegistrarApplicationStopping is published when an application is
// shutting down on some node. Route.State == ApplicationStateStopping.
type MessageRegistrarApplicationStopping struct {
	Route ApplicationRoute
}

// MessageRegistrarApplicationStopped is published when an application has
// stopped on some node but remains loaded. Route carries a frozen snapshot
// of the route as it was immediately before the stop.
type MessageRegistrarApplicationStopped struct {
	Route ApplicationRoute
}

// MessageRegistrarApplicationUnloaded is published when an application has
// been removed from a node entirely. Route carries a frozen snapshot of
// the route as it was immediately before the removal.
type MessageRegistrarApplicationUnloaded struct {
	Route ApplicationRoute
}

// MessageRegistrarProxyRegistered is published when a node has been advertised
// as a proxy to a target node, enabling connections to flow through the proxy.
type MessageRegistrarProxyRegistered struct {
	Route ProxyRoute
}

// MessageRegistrarProxyUnregistered is published when a previously advertised
// proxy route is removed.
type MessageRegistrarProxyUnregistered struct {
	Route ProxyRoute
}
