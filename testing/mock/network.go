package mock

import (
	"reflect"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// Network is a standalone gen.Network mock. Its methods are configuration and query
// operations with no egress of their own; every method has an On<Method> override and
// unset returns a safe default. Registrar() hands back an internal *Registrar that
// shares the same recorder, and Node/GetNode/GetNodeWithRoute hand back a *RemoteNode
// mock so consumer chains do not nil-panic.
type Network struct {
	recorder
	registrar *Registrar
	ov        networkOverrides
}

type networkOverrides struct {
	registrar               func() (gen.Registrar, error)
	cookie                  func() string
	setCookie               func(cookie string) error
	maxMessageSize          func() int
	setMaxMessageSize       func(size int)
	networkFlags            func() gen.NetworkFlags
	setNetworkFlags         func(flags gen.NetworkFlags)
	acceptors               func() ([]gen.Acceptor, error)
	node                    func(name gen.Atom) (gen.RemoteNode, error)
	getNode                 func(name gen.Atom) (gen.RemoteNode, error)
	getNodeWithRoute        func(name gen.Atom, route gen.NetworkRoute) (gen.RemoteNode, error)
	nodes                   func() []gen.Atom
	addRoute                func(match string, route gen.NetworkRoute, weight int) error
	removeRoute             func(match string) error
	route                   func(name gen.Atom) ([]gen.NetworkRoute, error)
	addProxyRoute           func(match string, proxy gen.NetworkProxyRoute, weight int) error
	removeProxyRoute        func(match string) error
	proxyRoute              func(name gen.Atom) ([]gen.NetworkProxyRoute, error)
	registerProto           func(proto gen.NetworkProto)
	registerHandshake       func(handshake gen.NetworkHandshake)
	enableSpawn             func(name gen.Atom, factory gen.ProcessFactory, nodes ...gen.Atom) error
	disableSpawn            func(name gen.Atom, nodes ...gen.Atom) error
	enableApplicationStart  func(name gen.Atom, nodes ...gen.Atom) error
	disableApplicationStart func(name gen.Atom, nodes ...gen.Atom) error
	info                    func() (gen.NetworkInfo, error)
	mode                    func() gen.NetworkMode
	protos                  func() []gen.NetworkProto
	registerType            func(v any) error
	registerTypes           func(types []any) error
	registerError           func(e error) error
	registerErrors          func(errs []error) error
	registerAtom            func(a gen.Atom) error
	registerAtoms           func(atoms []gen.Atom) error
	registeredTypes         func() []gen.RegisteredTypeInfo
	lookupType              func(name string) (reflect.Type, bool)
}

var _ gen.Network = (*Network)(nil)

// NewNetwork returns a dumb gen.Network mock (no recording; use NewNetworkT for Should*).
func NewNetwork() *Network { return newNetwork(recorder{}) }

// NewNetworkT returns a gen.Network mock that shares the recorder t with its sub-mocks.
func NewNetworkT(t check.T) *Network { return newNetwork(newRecorder(t)) }

func newNetwork(r recorder) *Network {
	n := &Network{recorder: r}
	n.registrar = newRegistrar(r)
	return n
}

// On<Method> overrides

func (n *Network) OnRegistrar(fn func() (gen.Registrar, error)) { n.ov.registrar = fn }
func (n *Network) OnCookie(fn func() string)                    { n.ov.cookie = fn }
func (n *Network) OnSetCookie(fn func(cookie string) error)     { n.ov.setCookie = fn }
func (n *Network) OnMaxMessageSize(fn func() int)               { n.ov.maxMessageSize = fn }
func (n *Network) OnSetMaxMessageSize(fn func(size int))        { n.ov.setMaxMessageSize = fn }
func (n *Network) OnNetworkFlags(fn func() gen.NetworkFlags)    { n.ov.networkFlags = fn }
func (n *Network) OnSetNetworkFlags(fn func(flags gen.NetworkFlags)) {
	n.ov.setNetworkFlags = fn
}
func (n *Network) OnAcceptors(fn func() ([]gen.Acceptor, error)) { n.ov.acceptors = fn }
func (n *Network) OnNode(fn func(name gen.Atom) (gen.RemoteNode, error)) {
	n.ov.node = fn
}
func (n *Network) OnGetNode(fn func(name gen.Atom) (gen.RemoteNode, error)) {
	n.ov.getNode = fn
}
func (n *Network) OnGetNodeWithRoute(fn func(name gen.Atom, route gen.NetworkRoute) (gen.RemoteNode, error)) {
	n.ov.getNodeWithRoute = fn
}
func (n *Network) OnNodes(fn func() []gen.Atom) { n.ov.nodes = fn }
func (n *Network) OnAddRoute(fn func(match string, route gen.NetworkRoute, weight int) error) {
	n.ov.addRoute = fn
}
func (n *Network) OnRemoveRoute(fn func(match string) error) { n.ov.removeRoute = fn }
func (n *Network) OnRoute(fn func(name gen.Atom) ([]gen.NetworkRoute, error)) {
	n.ov.route = fn
}
func (n *Network) OnAddProxyRoute(fn func(match string, proxy gen.NetworkProxyRoute, weight int) error) {
	n.ov.addProxyRoute = fn
}
func (n *Network) OnRemoveProxyRoute(fn func(match string) error) { n.ov.removeProxyRoute = fn }
func (n *Network) OnProxyRoute(fn func(name gen.Atom) ([]gen.NetworkProxyRoute, error)) {
	n.ov.proxyRoute = fn
}
func (n *Network) OnRegisterProto(fn func(proto gen.NetworkProto)) { n.ov.registerProto = fn }
func (n *Network) OnRegisterHandshake(fn func(handshake gen.NetworkHandshake)) {
	n.ov.registerHandshake = fn
}
func (n *Network) OnEnableSpawn(fn func(name gen.Atom, factory gen.ProcessFactory, nodes ...gen.Atom) error) {
	n.ov.enableSpawn = fn
}
func (n *Network) OnDisableSpawn(fn func(name gen.Atom, nodes ...gen.Atom) error) {
	n.ov.disableSpawn = fn
}
func (n *Network) OnEnableApplicationStart(fn func(name gen.Atom, nodes ...gen.Atom) error) {
	n.ov.enableApplicationStart = fn
}
func (n *Network) OnDisableApplicationStart(fn func(name gen.Atom, nodes ...gen.Atom) error) {
	n.ov.disableApplicationStart = fn
}
func (n *Network) OnInfo(fn func() (gen.NetworkInfo, error)) { n.ov.info = fn }
func (n *Network) OnMode(fn func() gen.NetworkMode)          { n.ov.mode = fn }
func (n *Network) OnProtos(fn func() []gen.NetworkProto)     { n.ov.protos = fn }
func (n *Network) OnRegisterType(fn func(v any) error)       { n.ov.registerType = fn }
func (n *Network) OnRegisterTypes(fn func(types []any) error) {
	n.ov.registerTypes = fn
}
func (n *Network) OnRegisterError(fn func(e error) error)       { n.ov.registerError = fn }
func (n *Network) OnRegisterErrors(fn func(errs []error) error) { n.ov.registerErrors = fn }
func (n *Network) OnRegisterAtom(fn func(a gen.Atom) error)     { n.ov.registerAtom = fn }
func (n *Network) OnRegisterAtoms(fn func(atoms []gen.Atom) error) {
	n.ov.registerAtoms = fn
}
func (n *Network) OnRegisteredTypes(fn func() []gen.RegisteredTypeInfo) {
	n.ov.registeredTypes = fn
}
func (n *Network) OnLookupType(fn func(name string) (reflect.Type, bool)) {
	n.ov.lookupType = fn
}

// gen.Network

func (n *Network) Registrar() (gen.Registrar, error) {
	if n.ov.registrar != nil {
		return n.ov.registrar()
	}
	return n.registrar, nil
}

func (n *Network) ResolveApplication(name gen.Atom) (gen.ApplicationRoutes, error) {
	reg, err := n.Registrar()
	if err != nil {
		return nil, err
	}
	return reg.Resolver().ResolveApplication(name)
}

func (n *Network) Cookie() string {
	if n.ov.cookie != nil {
		return n.ov.cookie()
	}
	return ""
}

func (n *Network) SetCookie(cookie string) error {
	if n.ov.setCookie != nil {
		return n.ov.setCookie(cookie)
	}
	return nil
}

func (n *Network) MaxMessageSize() int {
	if n.ov.maxMessageSize != nil {
		return n.ov.maxMessageSize()
	}
	return 0
}

func (n *Network) SetMaxMessageSize(size int) {
	if n.ov.setMaxMessageSize != nil {
		n.ov.setMaxMessageSize(size)
	}
}

func (n *Network) NetworkFlags() gen.NetworkFlags {
	if n.ov.networkFlags != nil {
		return n.ov.networkFlags()
	}
	return gen.NetworkFlags{}
}

func (n *Network) SetNetworkFlags(flags gen.NetworkFlags) {
	if n.ov.setNetworkFlags != nil {
		n.ov.setNetworkFlags(flags)
	}
}

func (n *Network) Acceptors() ([]gen.Acceptor, error) {
	if n.ov.acceptors != nil {
		return n.ov.acceptors()
	}
	return nil, nil
}

func (n *Network) Node(name gen.Atom) (gen.RemoteNode, error) {
	if n.ov.node != nil {
		return n.ov.node(name)
	}
	return newRemoteNode(n.recorder), nil
}

func (n *Network) GetNode(name gen.Atom) (gen.RemoteNode, error) {
	if n.ov.getNode != nil {
		return n.ov.getNode(name)
	}
	return newRemoteNode(n.recorder), nil
}

func (n *Network) GetNodeWithRoute(name gen.Atom, route gen.NetworkRoute) (gen.RemoteNode, error) {
	if n.ov.getNodeWithRoute != nil {
		return n.ov.getNodeWithRoute(name, route)
	}
	return newRemoteNode(n.recorder), nil
}

func (n *Network) Nodes() []gen.Atom {
	if n.ov.nodes != nil {
		return n.ov.nodes()
	}
	return nil
}

func (n *Network) AddRoute(match string, route gen.NetworkRoute, weight int) error {
	if n.ov.addRoute != nil {
		return n.ov.addRoute(match, route, weight)
	}
	return nil
}

func (n *Network) RemoveRoute(match string) error {
	if n.ov.removeRoute != nil {
		return n.ov.removeRoute(match)
	}
	return nil
}

func (n *Network) Route(name gen.Atom) ([]gen.NetworkRoute, error) {
	if n.ov.route != nil {
		return n.ov.route(name)
	}
	return nil, nil
}

func (n *Network) AddProxyRoute(match string, proxy gen.NetworkProxyRoute, weight int) error {
	if n.ov.addProxyRoute != nil {
		return n.ov.addProxyRoute(match, proxy, weight)
	}
	return nil
}

func (n *Network) RemoveProxyRoute(match string) error {
	if n.ov.removeProxyRoute != nil {
		return n.ov.removeProxyRoute(match)
	}
	return nil
}

func (n *Network) ProxyRoute(name gen.Atom) ([]gen.NetworkProxyRoute, error) {
	if n.ov.proxyRoute != nil {
		return n.ov.proxyRoute(name)
	}
	return nil, nil
}

func (n *Network) RegisterProto(proto gen.NetworkProto) {
	if n.ov.registerProto != nil {
		n.ov.registerProto(proto)
	}
}

func (n *Network) RegisterHandshake(handshake gen.NetworkHandshake) {
	if n.ov.registerHandshake != nil {
		n.ov.registerHandshake(handshake)
	}
}

func (n *Network) EnableSpawn(name gen.Atom, factory gen.ProcessFactory, nodes ...gen.Atom) error {
	if n.ov.enableSpawn != nil {
		return n.ov.enableSpawn(name, factory, nodes...)
	}
	return nil
}

func (n *Network) DisableSpawn(name gen.Atom, nodes ...gen.Atom) error {
	if n.ov.disableSpawn != nil {
		return n.ov.disableSpawn(name, nodes...)
	}
	return nil
}

func (n *Network) EnableApplicationStart(name gen.Atom, nodes ...gen.Atom) error {
	if n.ov.enableApplicationStart != nil {
		return n.ov.enableApplicationStart(name, nodes...)
	}
	return nil
}

func (n *Network) DisableApplicationStart(name gen.Atom, nodes ...gen.Atom) error {
	if n.ov.disableApplicationStart != nil {
		return n.ov.disableApplicationStart(name, nodes...)
	}
	return nil
}

func (n *Network) Info() (gen.NetworkInfo, error) {
	if n.ov.info != nil {
		return n.ov.info()
	}
	return gen.NetworkInfo{}, nil
}

func (n *Network) Mode() gen.NetworkMode {
	if n.ov.mode != nil {
		return n.ov.mode()
	}
	return gen.NetworkModeEnabled
}

func (n *Network) Protos() []gen.NetworkProto {
	if n.ov.protos != nil {
		return n.ov.protos()
	}
	return nil
}

func (n *Network) RegisterType(v any) error {
	if n.ov.registerType != nil {
		return n.ov.registerType(v)
	}
	return nil
}

func (n *Network) RegisterTypes(types []any) error {
	if n.ov.registerTypes != nil {
		return n.ov.registerTypes(types)
	}
	return nil
}

func (n *Network) RegisterError(e error) error {
	if n.ov.registerError != nil {
		return n.ov.registerError(e)
	}
	return nil
}

func (n *Network) RegisterErrors(errs []error) error {
	if n.ov.registerErrors != nil {
		return n.ov.registerErrors(errs)
	}
	return nil
}

func (n *Network) RegisterAtom(a gen.Atom) error {
	if n.ov.registerAtom != nil {
		return n.ov.registerAtom(a)
	}
	return nil
}

func (n *Network) RegisterAtoms(atoms []gen.Atom) error {
	if n.ov.registerAtoms != nil {
		return n.ov.registerAtoms(atoms)
	}
	return nil
}

func (n *Network) RegisteredTypes() []gen.RegisteredTypeInfo {
	if n.ov.registeredTypes != nil {
		return n.ov.registeredTypes()
	}
	return nil
}

func (n *Network) LookupType(name string) (reflect.Type, bool) {
	if n.ov.lookupType != nil {
		return n.ov.lookupType(name)
	}
	return nil, false
}

// Registrar is a gen.Registrar mock. A Network mints one internally (reach it by
// type-asserting Network().Registrar() to *Registrar to set its overrides), or build
// a standalone one with NewRegistrar/NewRegistrarT. Resolver() hands back a *Resolver
// sharing the same recorder.
type Registrar struct {
	recorder
	resolver *Resolver
	ov       registrarOverrides
}

type registrarOverrides struct {
	register                   func(node gen.NodeRegistrar, routes gen.RegisterRoutes) (gen.StaticRoutes, error)
	resolver                   func() gen.Resolver
	registerProxy              func(to gen.Atom) error
	unregisterProxy            func(to gen.Atom) error
	registerApplicationRoute   func(route gen.ApplicationRoute) error
	unregisterApplicationRoute func(name gen.Atom) error
	nodes                      func() ([]gen.Atom, error)
	config                     func(items ...string) (map[string]any, error)
	configItem                 func(item string) (any, error)
	event                      func() (gen.Event, error)
	info                       func() gen.RegistrarInfo
	terminate                  func()
	version                    func() gen.Version
}

var _ gen.Registrar = (*Registrar)(nil)

// NewRegistrar returns a dumb gen.Registrar mock (no recording; use NewRegistrarT).
func NewRegistrar() *Registrar { return newRegistrar(recorder{}) }

// NewRegistrarT returns a gen.Registrar mock that records and asserts through t.
func NewRegistrarT(t check.T) *Registrar { return newRegistrar(newRecorder(t)) }

func newRegistrar(r recorder) *Registrar {
	m := &Registrar{recorder: r}
	m.resolver = newResolver(r)
	return m
}

// On<Method> overrides

func (m *Registrar) OnRegister(fn func(node gen.NodeRegistrar, routes gen.RegisterRoutes) (gen.StaticRoutes, error)) {
	m.ov.register = fn
}
func (m *Registrar) OnResolver(fn func() gen.Resolver)            { m.ov.resolver = fn }
func (m *Registrar) OnRegisterProxy(fn func(to gen.Atom) error)   { m.ov.registerProxy = fn }
func (m *Registrar) OnUnregisterProxy(fn func(to gen.Atom) error) { m.ov.unregisterProxy = fn }
func (m *Registrar) OnRegisterApplicationRoute(fn func(route gen.ApplicationRoute) error) {
	m.ov.registerApplicationRoute = fn
}
func (m *Registrar) OnUnregisterApplicationRoute(fn func(name gen.Atom) error) {
	m.ov.unregisterApplicationRoute = fn
}
func (m *Registrar) OnNodes(fn func() ([]gen.Atom, error)) { m.ov.nodes = fn }
func (m *Registrar) OnConfig(fn func(items ...string) (map[string]any, error)) {
	m.ov.config = fn
}
func (m *Registrar) OnConfigItem(fn func(item string) (any, error)) { m.ov.configItem = fn }
func (m *Registrar) OnEvent(fn func() (gen.Event, error))           { m.ov.event = fn }
func (m *Registrar) OnInfo(fn func() gen.RegistrarInfo)             { m.ov.info = fn }
func (m *Registrar) OnTerminate(fn func())                          { m.ov.terminate = fn }
func (m *Registrar) OnVersion(fn func() gen.Version)                { m.ov.version = fn }

// gen.Registrar

func (m *Registrar) Register(node gen.NodeRegistrar, routes gen.RegisterRoutes) (gen.StaticRoutes, error) {
	if m.ov.register != nil {
		return m.ov.register(node, routes)
	}
	return gen.StaticRoutes{}, nil
}

func (m *Registrar) Resolver() gen.Resolver {
	if m.ov.resolver != nil {
		return m.ov.resolver()
	}
	return m.resolver
}

func (m *Registrar) RegisterProxy(to gen.Atom) error {
	if m.ov.registerProxy != nil {
		return m.ov.registerProxy(to)
	}
	return nil
}

func (m *Registrar) UnregisterProxy(to gen.Atom) error {
	if m.ov.unregisterProxy != nil {
		return m.ov.unregisterProxy(to)
	}
	return nil
}

func (m *Registrar) RegisterApplicationRoute(route gen.ApplicationRoute) error {
	if m.ov.registerApplicationRoute != nil {
		return m.ov.registerApplicationRoute(route)
	}
	return nil
}

func (m *Registrar) UnregisterApplicationRoute(name gen.Atom) error {
	if m.ov.unregisterApplicationRoute != nil {
		return m.ov.unregisterApplicationRoute(name)
	}
	return nil
}

func (m *Registrar) Nodes() ([]gen.Atom, error) {
	if m.ov.nodes != nil {
		return m.ov.nodes()
	}
	return nil, nil
}

func (m *Registrar) Config(items ...string) (map[string]any, error) {
	if m.ov.config != nil {
		return m.ov.config(items...)
	}
	return nil, nil
}

func (m *Registrar) ConfigItem(item string) (any, error) {
	if m.ov.configItem != nil {
		return m.ov.configItem(item)
	}
	return nil, nil
}

func (m *Registrar) Event() (gen.Event, error) {
	if m.ov.event != nil {
		return m.ov.event()
	}
	return gen.Event{}, nil
}

func (m *Registrar) Info() gen.RegistrarInfo {
	if m.ov.info != nil {
		return m.ov.info()
	}
	return gen.RegistrarInfo{}
}

func (m *Registrar) Terminate() {
	if m.ov.terminate != nil {
		m.ov.terminate()
	}
}

func (m *Registrar) Version() gen.Version {
	if m.ov.version != nil {
		return m.ov.version()
	}
	return gen.Version{}
}

// Resolver is a gen.Resolver mock. A Registrar mints one internally (reach it by
// type-asserting Registrar().Resolver() to *Resolver to set its overrides), or build
// a standalone one with NewResolver/NewResolverT.
type Resolver struct {
	recorder
	ov resolverOverrides
}

type resolverOverrides struct {
	resolve            func(node gen.Atom) ([]gen.Route, error)
	resolveProxy       func(node gen.Atom) ([]gen.ProxyRoute, error)
	resolveApplication func(name gen.Atom) (gen.ApplicationRoutes, error)
}

var _ gen.Resolver = (*Resolver)(nil)

// NewResolver returns a dumb gen.Resolver mock (no recording; use NewResolverT).
func NewResolver() *Resolver { return newResolver(recorder{}) }

// NewResolverT returns a gen.Resolver mock that records and asserts through t.
func NewResolverT(t check.T) *Resolver { return newResolver(newRecorder(t)) }

func newResolver(r recorder) *Resolver { return &Resolver{recorder: r} }

// On<Method> overrides

func (m *Resolver) OnResolve(fn func(node gen.Atom) ([]gen.Route, error)) { m.ov.resolve = fn }
func (m *Resolver) OnResolveProxy(fn func(node gen.Atom) ([]gen.ProxyRoute, error)) {
	m.ov.resolveProxy = fn
}
func (m *Resolver) OnResolveApplication(fn func(name gen.Atom) (gen.ApplicationRoutes, error)) {
	m.ov.resolveApplication = fn
}

// gen.Resolver

func (m *Resolver) Resolve(node gen.Atom) ([]gen.Route, error) {
	if m.ov.resolve != nil {
		return m.ov.resolve(node)
	}
	return nil, nil
}

func (m *Resolver) ResolveProxy(node gen.Atom) ([]gen.ProxyRoute, error) {
	if m.ov.resolveProxy != nil {
		return m.ov.resolveProxy(node)
	}
	return nil, nil
}

func (m *Resolver) ResolveApplication(name gen.Atom) (gen.ApplicationRoutes, error) {
	if m.ov.resolveApplication != nil {
		return m.ov.resolveApplication(name)
	}
	return nil, nil
}
