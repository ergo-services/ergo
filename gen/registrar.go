package gen

// Registrar interface provides service registry and discovery functionality.
// Enables dynamic node discovery, routing, configuration, and application deployment.
//
// Default Registrar Behavior (if not configured in NetworkOptions):
// - Uses minimal built-in registrar (no external service required)
// - Tries to start embedded registrar server on localhost:41000
// - If port taken, connects to existing registrar on localhost (TCP client)
// - Same host: nodes discover each other via local registrar server
// - Different hosts: queries remote host's registrar via UDP (host:41000)
//   • Remote host must have registrar server running
//   • Remote host must be reachable and port open
//   • No persistent connection - query on-demand only
// - Limited features: basic route resolution only
// - No centralized config, proxy routes, application routes, or events
//
// For distributed clusters across multiple hosts, configure external registrar:
// - etcd registrar: uses etcd for distributed discovery and configuration
//   • Nodes on any host discover each other automatically
//   • Centralized configuration via etcd key-value store
//   • Application route advertisement and discovery
//   • Full feature support
//
// - Saturn registrar: embedded Raft-based distributed registry
//   • One node runs Saturn server, others connect as clients
//   • Automatic discovery across hosts
//   • No external dependencies (self-contained)
//
// Features marked "if registrar supports" are optional capabilities.
// Check RegistrarInfo to see which features are available.
type Registrar interface {
	// Register is called when the node network starts.
	// Registers this node with the service registry and publishes routes.
	// Returns static routes received from registrar for initial connectivity.
	Register(node NodeRegistrar, routes RegisterRoutes) (StaticRoutes, error)

	// Resolver returns the Resolver interface for dynamic route discovery.
	// Used to find routes to other nodes, proxy routes, and application locations.
	Resolver() Resolver

	// RegisterProxy registers this node as a proxy to the target node.
	// Other nodes can connect to target through this node.
	// Optional feature - check RegistrarInfo.SupportRegisterProxy.
	// Returns ErrUnsupported if registrar doesn't support proxying.
	RegisterProxy(to Atom) error

	// UnregisterProxy removes this node as a proxy to the target node.
	// Optional feature - check RegistrarInfo.SupportRegisterProxy.
	UnregisterProxy(to Atom) error

	// RegisterApplicationRoute registers application deployment information.
	// Advertises that this application is loaded/running on this node.
	// Optional feature - check RegistrarInfo.SupportRegisterApplication.
	// Returns ErrUnsupported if registrar doesn't support application routes.
	RegisterApplicationRoute(route ApplicationRoute) error

	// UnregisterApplicationRoute removes application deployment information.
	// Called when application is unloaded or stopped.
	// Optional feature - check RegistrarInfo.SupportRegisterApplication.
	UnregisterApplicationRoute(name Atom) error

	// Nodes returns a list of all nodes registered in the service registry.
	// Enables discovering available nodes dynamically.
	Nodes() ([]Atom, error)

	// Config returns configuration values from the registrar.
	// Centralized configuration storage (like etcd key-value).
	// Optional feature - check RegistrarInfo.SupportConfig.
	// Returns ErrUnsupported if not supported.
	Config(items ...string) (map[string]any, error)

	// ConfigItem returns a single configuration value by name.
	// Optional feature - check RegistrarInfo.SupportConfig.
	// Returns ErrUnsupported if not supported.
	ConfigItem(item string) (any, error)

	// Event returns an event for monitoring registrar changes.
	// Link/Monitor this event to receive notifications when registry changes.
	// Optional feature - check RegistrarInfo.SupportEvent.
	// Returns ErrUnsupported if not supported.
	Event() (Event, error)

	// Info returns information about the registrar capabilities and connection.
	Info() RegistrarInfo

	// Terminate is called when the node network stops.
	// Unregisters the node and cleans up resources.
	Terminate()

	// Version returns the registrar implementation version.
	Version() Version
}

// Resolver interface provides dynamic route discovery.
// Part of Registrar. Resolves routes for nodes and applications.
type Resolver interface {
	// Resolve resolves connection routes for the given node name.
	// Returns available routes (host:port combinations) to connect to the node.
	// Routes are discovered from the service registry.
	Resolve(node Atom) ([]Route, error)

	// ResolveProxy resolves proxy routes for the given node name.
	// Returns proxy paths (node A → proxy B → target node C).
	// Enables connecting through intermediate proxy nodes.
	ResolveProxy(node Atom) ([]ProxyRoute, error)

	// ResolveApplication resolves deployment locations for the given application.
	// Returns which nodes have this application loaded or running.
	// Enables finding where applications are deployed in the cluster.
	ResolveApplication(name Atom) ([]ApplicationRoute, error)
}

// RegistrarInfo describes registrar capabilities and connection details.
// Retrieved via registrar.Info(). Shows which features are supported.
type RegistrarInfo struct {
	// Server is the registrar server address (e.g., "localhost:2379" for etcd).
	Server string

	// EmbeddedServer indicates if registrar runs embedded in this node.
	// True for Saturn registrar, false for external etcd.
	EmbeddedServer bool

	// SupportRegisterProxy indicates if registrar supports proxy registration.
	// Allows nodes to act as proxies for other nodes.
	SupportRegisterProxy bool

	// SupportRegisterApplication indicates if registrar supports application routes.
	// Enables advertising where applications are deployed.
	SupportRegisterApplication bool

	// SupportConfig indicates if registrar provides centralized configuration.
	// Enables Config() and ConfigItem() methods.
	SupportConfig bool

	// SupportEvent indicates if registrar provides change notification events.
	// Enables Event() method for monitoring registry changes.
	SupportEvent bool

	// Version is the registrar implementation version.
	Version Version
}

// AcceptorInfo describes a network acceptor (listener) and its configuration.
// Part of NetworkInfo. Shows acceptor status and settings.
type AcceptorInfo struct {
	// Interface is the listening address (e.g., "0.0.0.0:15000").
	Interface string

	// MaxMessageSize is the message size limit for this acceptor (bytes).
	MaxMessageSize int

	// Flags shows network capabilities for this acceptor.
	Flags NetworkFlags

	// TLS indicates if this acceptor uses TLS encryption.
	TLS bool

	// CustomRegistrar indicates if this acceptor uses a custom registrar.
	CustomRegistrar bool

	// RegistrarServer is the registrar address used by this acceptor.
	RegistrarServer string

	// RegistrarVersion is the registrar version.
	RegistrarVersion Version

	// HandshakeVersion is the handshake protocol version.
	HandshakeVersion Version

	// ProtoVersion is the network protocol version (EDF or Erlang).
	ProtoVersion Version
}

// RegisterRoutes contains routes to publish when registering with the service registry.
// Used in Registrar.Register() to advertise this node's connectivity.
type RegisterRoutes struct {
	// Routes lists this node's network routes (host:port combinations).
	// Published so other nodes can discover how to connect.
	Routes []Route

	// ApplicationRoutes lists applications deployed on this node.
	// Published so other nodes can discover where applications are running.
	ApplicationRoutes []ApplicationRoute

	// ProxyRoutes lists proxy routes if this node acts as a proxy.
	// Only included if proxy feature is enabled.
	ProxyRoutes []ProxyRoute
}

// RegistrarConfig contains centralized configuration from the service registry.
// Retrieved via registrar.Config(). Enables configuration management.
type RegistrarConfig struct {
	// LastUpdate is the unix timestamp of last configuration update.
	LastUpdate int64

	// Config is the key-value configuration map.
	Config map[string]any
}

// Route describes a network route to connect to a node.
// Contains host, port, and protocol information.
type Route struct {
	// Host is the hostname or IP address.
	Host string

	// Port is the TCP port number.
	Port uint16

	// TLS indicates if this route requires TLS encryption.
	TLS bool

	// HandshakeVersion is the handshake protocol version for this route.
	HandshakeVersion Version

	// ProtoVersion is the network protocol version (EDF or Erlang).
	ProtoVersion Version
}

// ProxyRoute describes a route through a proxy node.
// Enables connecting to a target node via an intermediate proxy.
type ProxyRoute struct {
	// To is the target node name (final destination).
	To Atom

	// Proxy is the intermediate proxy node name.
	// Connect to Proxy, then Proxy forwards to To.
	Proxy Atom
}

// ApplicationRoute describes where an application is deployed.
// Retrieved via resolver.ResolveApplication(). Shows application location and state.
type ApplicationRoute struct {
	// Node is the node name where application is deployed.
	Node Atom

	// Name is the application name.
	Name Atom

	// Weight is the routing weight for load balancing.
	// Higher weight = preferred for remote operations.
	Weight int

	// Mode is the application starting mode (Temporary, Transient, Permanent).
	Mode ApplicationMode

	// State is the current application state (Loaded, Running, Stopped).
	State ApplicationState
}

// StaticRoutes contains static route configuration received from registrar.
// Returned by Registrar.Register(). Applied as initial routes.
type StaticRoutes struct {
	// Routes maps route patterns to network routes.
	// Key is match pattern (e.g., "prod-*"), value is route configuration.
	Routes map[string]NetworkRoute

	// Proxies maps proxy patterns to proxy routes.
	// Key is match pattern, value is proxy configuration.
	Proxies map[string]NetworkProxyRoute
}

// RouteInfo describes a configured route with its settings.
// Part of NetworkInfo. Shows route configuration and capabilities.
type RouteInfo struct {
	// Match is the node name pattern this route applies to.
	// Supports wildcards (e.g., "prod-*@example.com").
	Match string

	// Weight is the route priority (higher = preferred).
	Weight int

	// UseResolver indicates if this route uses dynamic resolution.
	// True = resolver-based, false = static configuration.
	UseResolver bool

	// UseCustomCookie indicates if this route uses a custom cookie.
	// True = custom, false = uses node's default cookie.
	UseCustomCookie bool

	// UseCustomCert indicates if this route uses custom TLS certificates.
	// True = custom CertManager, false = uses node's default.
	UseCustomCert bool

	// Flags shows network capabilities for this route.
	Flags NetworkFlags

	// HandshakeVersion is the handshake protocol version.
	HandshakeVersion Version

	// ProtoVersion is the network protocol version.
	ProtoVersion Version

	// Host is the target hostname or IP address.
	Host string

	// Port is the target TCP port number.
	Port uint16
}

// ProxyRouteInfo describes a configured proxy route.
// Part of NetworkInfo. Shows proxy configuration.
type ProxyRouteInfo struct {
	// Match is the node name pattern this proxy route applies to.
	Match string

	// Weight is the route priority.
	Weight int

	// UseResolver indicates if proxy uses dynamic resolution.
	UseResolver bool

	// UseCustomCookie indicates if proxy uses custom cookie.
	UseCustomCookie bool

	// Flags shows proxy capabilities.
	Flags NetworkProxyFlags

	// MaxHop is the maximum number of proxy hops allowed.
	// Prevents infinite proxy loops. Default: 8.
	MaxHop int

	// Proxy is the intermediate proxy node name.
	Proxy Atom
}
