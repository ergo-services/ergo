package gen

// Registrar interface
type Registrar interface {
	// Register invokes on network start
	Register(node NodeRegistrar, routes RegisterRoutes) (StaticRoutes, error)

	// Resolver returns the gen.Resolver interface
	Resolver() Resolver

	// RegisterProxy allows to register this node as a proxy for the given node on the registrar
	// (if the registrar does support this feature)
	RegisterProxy(to Atom) error
	// UnregisterProxy unregisters this node as a proxy to the given node on the registrar
	// (if the registrar does support this feature)
	UnregisterProxy(to Atom) error

	// RegisterApplicationRoute registers the application on the registrar
	// (if the registrar does support this feature).
	RegisterApplicationRoute(route ApplicationRoute) error
	// UnregisterApplication unregisters the given application.
	// (if the registrar does support this feature).
	UnregisterApplicationRoute(name Atom) error

	// Nodes returns a list of the nodes registered on the registrar
	Nodes() ([]Atom, error)
	// Config returns config received from the registrar
	// (if the registrar does support this feature)
	Config(items ...string) (map[string]any, error)
	// ConfigItem returns the value from the config for the given name
	// (if the registrar does support this feature)
	ConfigItem(item string) (any, error)
	// Event returns the event you may want to Link/Monitor
	// (if the registrar does support this feature)
	Event() (Event, error)
	// Info return short information about the registrar
	Info() RegistrarInfo

	// Terminate invokes on network stop.
	Terminate()

	Version() Version
}

// Resolver interface
type Resolver interface {
	// Resolve resolves the routes for the given node name
	Resolve(node Atom) ([]Route, error)
	// Resolve resolves the proxy routes for the given node name
	ResolveProxy(node Atom) ([]ProxyRoute, error)
	// Resolve resolves the applications routes for the given application.
	// This information allows you to know where the given application is running
	// or loaded.
	ResolveApplication(name Atom) ([]ApplicationRoute, error)
}

type RegistrarInfo struct {
	Server                     string
	EmbeddedServer             bool
	SupportRegisterProxy       bool
	SupportRegisterApplication bool
	SupportConfig              bool
	SupportEvent               bool
	Version                    Version
}

type AcceptorInfo struct {
	Interface        string
	MaxMessageSize   int
	Flags            NetworkFlags
	TLS              bool
	CustomRegistrar  bool
	RegistrarServer  string
	RegistrarVersion Version
	HandshakeVersion Version
	ProtoVersion     Version
}

type RegisterRoutes struct {
	Routes            []Route
	ApplicationRoutes []ApplicationRoute
	ProxyRoutes       []ProxyRoute // if Proxy was enabled
}

type RegistrarConfig struct {
	LastUpdate int64 // timestamp
	Config     map[string]any
}

type Route struct {
	Host             string
	Port             uint16
	TLS              bool
	HandshakeVersion Version
	ProtoVersion     Version
}

type ProxyRoute struct {
	To    Atom // to
	Proxy Atom // via
}

type ApplicationRoute struct {
	Node   Atom
	Name   Atom
	Weight int
	Mode   ApplicationMode
	State  ApplicationState
}

type StaticRoutes struct {
	Routes  map[string]NetworkRoute      // match string => network route
	Proxies map[string]NetworkProxyRoute // match string => proxy route
}

type RouteInfo struct {
	Match            string
	Weight           int
	UseResolver      bool
	UseCustomCookie  bool
	UseCustomCert    bool
	Flags            NetworkFlags
	HandshakeVersion Version
	ProtoVersion     Version
	Host             string
	Port             uint16
}

type ProxyRouteInfo struct {
	Match           string
	Weight          int
	UseResolver     bool
	UseCustomCookie bool
	Flags           NetworkProxyFlags
	MaxHop          int
	Proxy           Atom
}
