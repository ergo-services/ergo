package unit

import (
	"reflect"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// mockNetwork is the default gen.Network behind Node().Network(). Resolver().Resolve and
// ResolveApplication are stubbed per name via OnResolve / OnResolveApplication; an
// unstubbed resolve fails the test (tier-3, mirroring an unstubbed OnCall) so a forgotten
// stub is loud, not a silent zero. Registrar().Event() returns a default registrar event
// (override with OnRegistrarEvent) into which the test feeds canonical gen.MessageRegistrar*
// messages via Subject.DeliverRegistrarEvent. Every other gen.Network method returns a safe
// default; for behavior the mock does not model, override the whole network with
// OnNetwork(func() gen.Network) instead.
type mockNetwork struct {
	node       *mockNode
	reg        *mockRegistrar
	resolve    map[gen.Atom]*resolveResult
	resolveApp map[gen.Atom]*resolveAppResult
	remotes    map[gen.Atom]*mockRemoteNode
	event      *gen.Event
	eventErr   error
	regErr     error // when set, Registrar() returns this error
}

type resolveResult struct {
	routes []gen.Route
	err    error
}

type resolveAppResult struct {
	routes gen.ApplicationRoutes
	err    error
}

// unitRegistrarEvent is the default registrar event the mock produces; consumers get
// it from Registrar().Event() and never hardcode it.
const unitRegistrarEvent = gen.Atom("$unit_registrar")

func newMockNetwork(n *mockNode) *mockNetwork {
	mn := &mockNetwork{
		node:       n,
		resolve:    make(map[gen.Atom]*resolveResult),
		resolveApp: make(map[gen.Atom]*resolveAppResult),
		remotes:    make(map[gen.Atom]*mockRemoteNode),
	}
	ev := gen.Event{Name: unitRegistrarEvent, Node: n.nodeName}
	mn.event = &ev
	mn.reg = &mockRegistrar{net: mn}
	return mn
}

// gen.Network

func (mn *mockNetwork) Registrar() (gen.Registrar, error) {
	if mn.regErr != nil {
		return nil, mn.regErr
	}
	return mn.reg, nil
}
func (mn *mockNetwork) Cookie() string                     { return "" }
func (mn *mockNetwork) SetCookie(cookie string) error      { return nil }
func (mn *mockNetwork) MaxMessageSize() int                { return 0 }
func (mn *mockNetwork) SetMaxMessageSize(size int)         {}
func (mn *mockNetwork) NetworkFlags() gen.NetworkFlags     { return gen.NetworkFlags{} }
func (mn *mockNetwork) SetNetworkFlags(gen.NetworkFlags)   {}
func (mn *mockNetwork) Acceptors() ([]gen.Acceptor, error) { return nil, nil }
func (mn *mockNetwork) Node(name gen.Atom) (gen.RemoteNode, error) {
	if rn, ok := mn.remotes[name]; ok {
		return rn, nil
	}
	return nil, gen.ErrNoConnection
}
func (mn *mockNetwork) GetNode(name gen.Atom) (gen.RemoteNode, error) {
	return mn.Node(name)
}
func (mn *mockNetwork) GetNodeWithRoute(name gen.Atom, route gen.NetworkRoute) (gen.RemoteNode, error) {
	return mn.Node(name)
}
func (mn *mockNetwork) Nodes() []gen.Atom                                      { return nil }
func (mn *mockNetwork) AddRoute(string, gen.NetworkRoute, int) error           { return nil }
func (mn *mockNetwork) RemoveRoute(string) error                               { return nil }
func (mn *mockNetwork) Route(gen.Atom) ([]gen.NetworkRoute, error)             { return nil, gen.ErrNoRoute }
func (mn *mockNetwork) AddProxyRoute(string, gen.NetworkProxyRoute, int) error { return nil }
func (mn *mockNetwork) RemoveProxyRoute(string) error                          { return nil }
func (mn *mockNetwork) ProxyRoute(gen.Atom) ([]gen.NetworkProxyRoute, error) {
	return nil, gen.ErrNoRoute
}
func (mn *mockNetwork) RegisterProto(gen.NetworkProto)                              {}
func (mn *mockNetwork) RegisterHandshake(gen.NetworkHandshake)                      {}
func (mn *mockNetwork) EnableSpawn(gen.Atom, gen.ProcessFactory, ...gen.Atom) error { return nil }
func (mn *mockNetwork) DisableSpawn(gen.Atom, ...gen.Atom) error                    { return nil }
func (mn *mockNetwork) EnableApplicationStart(gen.Atom, ...gen.Atom) error          { return nil }
func (mn *mockNetwork) DisableApplicationStart(gen.Atom, ...gen.Atom) error         { return nil }
func (mn *mockNetwork) Info() (gen.NetworkInfo, error)                              { return gen.NetworkInfo{}, nil }
func (mn *mockNetwork) Mode() gen.NetworkMode                                       { return gen.NetworkModeEnabled }
func (mn *mockNetwork) Protos() []gen.NetworkProto                                  { return nil }
func (mn *mockNetwork) RegisterType(any) error                                      { return nil }
func (mn *mockNetwork) RegisterTypes([]any) error                                   { return nil }
func (mn *mockNetwork) RegisterError(error) error                                   { return nil }
func (mn *mockNetwork) RegisterErrors([]error) error                                { return nil }
func (mn *mockNetwork) RegisterAtom(gen.Atom) error                                 { return nil }
func (mn *mockNetwork) RegisterAtoms([]gen.Atom) error                              { return nil }
func (mn *mockNetwork) RegisteredTypes() []gen.RegisteredTypeInfo                   { return nil }
func (mn *mockNetwork) LookupType(string) (reflect.Type, bool)                      { return nil, false }

// mockRegistrar is the gen.Registrar behind Network().Registrar(). Resolver() returns
// the same discovery stubs; Event() is stubbed via OnRegistrarEvent. The rest is
// unsupported, matching the embedded registrar's optional-feature surface.
type mockRegistrar struct{ net *mockNetwork }

func (r *mockRegistrar) Register(gen.NodeRegistrar, gen.RegisterRoutes) (gen.StaticRoutes, error) {
	return gen.StaticRoutes{}, nil
}
func (r *mockRegistrar) Resolver() gen.Resolver { return &mockResolver{net: r.net} }
func (r *mockRegistrar) Event() (gen.Event, error) {
	if r.net.eventErr != nil {
		return gen.Event{}, r.net.eventErr
	}
	return *r.net.event, nil
}
func (r *mockRegistrar) RegisterProxy(gen.Atom) error   { return gen.ErrUnsupported }
func (r *mockRegistrar) UnregisterProxy(gen.Atom) error { return gen.ErrUnsupported }
func (r *mockRegistrar) RegisterApplicationRoute(gen.ApplicationRoute) error {
	return gen.ErrUnsupported
}
func (r *mockRegistrar) UnregisterApplicationRoute(gen.Atom) error { return gen.ErrUnsupported }
func (r *mockRegistrar) Nodes() ([]gen.Atom, error)                { return nil, gen.ErrUnsupported }
func (r *mockRegistrar) Config(...string) (map[string]any, error)  { return nil, gen.ErrUnsupported }
func (r *mockRegistrar) ConfigItem(string) (any, error)            { return nil, gen.ErrUnsupported }
func (r *mockRegistrar) Info() gen.RegistrarInfo {
	return gen.RegistrarInfo{Server: "(unit mock)", Version: r.Version()}
}
func (r *mockRegistrar) Terminate() {}
func (r *mockRegistrar) Version() gen.Version {
	return gen.Version{Name: "unit-registrar", Release: "mock", License: gen.LicenseMIT}
}

// mockResolver answers the per-name discovery stubs; an unstubbed name fails the test.
type mockResolver struct{ net *mockNetwork }

func (r *mockResolver) Resolve(name gen.Atom) ([]gen.Route, error) {
	if res, ok := r.net.resolve[name]; ok {
		return res.routes, res.err
	}
	r.net.node.t.Helper()
	r.net.node.t.Fatalf("unit: actor resolved node %q but no stub is set; use sub.Node().Network().Registrar().Resolver().OnResolve(%q)", name, name)
	return nil, nil
}
func (r *mockResolver) ResolveApplication(name gen.Atom) (gen.ApplicationRoutes, error) {
	if res, ok := r.net.resolveApp[name]; ok {
		return res.routes, res.err
	}
	r.net.node.t.Helper()
	r.net.node.t.Fatalf("unit: actor resolved application %q but no stub is set; use sub.Node().Network().Registrar().Resolver().OnResolveApplication(%q)", name, name)
	return nil, nil
}
func (r *mockResolver) ResolveProxy(gen.Atom) ([]gen.ProxyRoute, error) {
	return nil, gen.ErrUnsupported
}

var _ gen.Network = (*mockNetwork)(nil)
var _ gen.Registrar = (*mockRegistrar)(nil)
var _ gen.Resolver = (*mockResolver)(nil)

//
// test-config handles: mirror the gen.Network -> Registrar -> Resolver hierarchy that
// the actor traverses, so configuration walks the same path the actor calls. Obtained
// via sub.Node().Network().Registrar().Resolver(), each is a thin handle over the same
// built-in mock network.
//

// MockNetwork is the test-config handle for Node().Network(); it shadows the embedded
// gen.Network (which the actor still sees) and exposes configuration of the built-in
// mock network.
type MockNetwork struct{ net *mockNetwork }

// Network returns the test-config handle for the built-in mock network.
func (n *MockNode) Network() *MockNetwork { return &MockNetwork{net: n.mockNode.netmock} }

// Registrar returns the test-config handle for Network().Registrar().
func (m *MockNetwork) Registrar() *MockRegistrar { return &MockRegistrar{net: m.net} }

// FailRegistrar makes Network().Registrar() return err (no registrar configured).
func (m *MockNetwork) FailRegistrar(err error) { m.net.regErr = err }

// MockRegistrar is the test-config handle for Network().Registrar().
type MockRegistrar struct{ net *mockNetwork }

// Resolver returns the test-config handle for Registrar().Resolver().
func (m *MockRegistrar) Resolver() *MockResolver { return &MockResolver{net: m.net} }

// OnEvent stubs Registrar().Event() to return the given event (overriding the default).
func (m *MockRegistrar) OnEvent(event gen.Event) { m.net.event = &event }

// FailEvent stubs Registrar().Event() to return err.
func (m *MockRegistrar) FailEvent(err error) { m.net.eventErr = err }

// MockResolver is the test-config handle for Registrar().Resolver().
type MockResolver struct{ net *mockNetwork }

// OnResolve stubs Resolver().Resolve(name).
func (m *MockResolver) OnResolve(name gen.Atom) *ResolveStub {
	res := &resolveResult{}
	m.net.resolve[name] = res
	return &ResolveStub{r: res}
}

// OnResolveApplication stubs Resolver().ResolveApplication(name) (the common
// service-discovery seam).
func (m *MockResolver) OnResolveApplication(name gen.Atom) *ResolveAppStub {
	res := &resolveAppResult{}
	m.net.resolveApp[name] = res
	return &ResolveAppStub{r: res}
}

// ResolveStub configures what Resolver().Resolve(name) returns.
type ResolveStub struct{ r *resolveResult }

// Return makes Resolve(name) return these routes.
func (s *ResolveStub) Return(routes ...gen.Route) { s.r.routes = routes }

// Fail makes Resolve(name) return err.
func (s *ResolveStub) Fail(err error) { s.r.err = err }

// ResolveAppStub configures what Resolver().ResolveApplication(name) returns.
type ResolveAppStub struct{ r *resolveAppResult }

// Return makes ResolveApplication(name) return these application routes.
func (s *ResolveAppStub) Return(routes ...gen.ApplicationRoute) { s.r.routes = routes }

// Fail makes ResolveApplication(name) return err.
func (s *ResolveAppStub) Fail(err error) { s.r.err = err }

// DeliverRegistrarEvent delivers a canonical gen.MessageRegistrar* message through the
// registrar event the actor subscribed to via Registrar().Event() (the built-in mock's
// default, or the one set by Network().Registrar().OnEvent). Convenience over
// DeliverEvent that the test does not need to hold the event value for.
func (s *Subject) DeliverRegistrarEvent(message any) *Subject {
	return s.DeliverEvent(*s.node.netmock.event, message)
}

//
// RemoteNode mock: Network().GetNode(name) returns a *mockRemoteNode (gen.RemoteNode);
// configure it via Network().OnGetNode(name). Spawn/SpawnRegister record RemoteSpawned,
// ApplicationStart* record RemoteApplicationStarted; both stub their return. Until
// configured, GetNode(name)/Node(name) return gen.ErrNoConnection.
//

type spawnResult struct {
	pid gen.PID
	err error
}

type appInfoResult struct {
	info gen.ApplicationInfo
	err  error
}

type mockRemoteNode struct {
	net      *mockNetwork
	name     gen.Atom
	spawn    map[gen.Atom]*spawnResult
	appStart map[gen.Atom]error
	appInfo  map[gen.Atom]*appInfoResult
}

func newMockRemoteNode(net *mockNetwork, name gen.Atom) *mockRemoteNode {
	return &mockRemoteNode{
		net:      net,
		name:     name,
		spawn:    make(map[gen.Atom]*spawnResult),
		appStart: make(map[gen.Atom]error),
		appInfo:  make(map[gen.Atom]*appInfoResult),
	}
}

var _ gen.RemoteNode = (*mockRemoteNode)(nil)

func (r *mockRemoteNode) doSpawn(register, name gen.Atom, options gen.ProcessOptions) (gen.PID, error) {
	pid := r.net.node.synthPID()
	var err error
	if res, ok := r.spawn[name]; ok {
		pid, err = res.pid, res.err
	}
	r.net.node.rec.Put(check.RemoteSpawned{
		Parent: r.net.node.subjectPID, Node: r.name, Name: name,
		Register: register, Child: pid, Options: options, Error: err,
	})
	return pid, err
}

func (r *mockRemoteNode) Spawn(name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	return r.doSpawn("", name, options)
}
func (r *mockRemoteNode) SpawnRegister(register gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	return r.doSpawn(register, name, options)
}

func (r *mockRemoteNode) appStartMode(name gen.Atom, mode gen.ApplicationMode) error {
	err := r.appStart[name]
	r.net.node.rec.Put(check.RemoteApplicationStarted{
		From: r.net.node.subjectPID, Node: r.name, Name: name, Mode: mode, Error: err,
	})
	return err
}

func (r *mockRemoteNode) ApplicationStart(name gen.Atom, _ gen.ApplicationOptions) error {
	return r.appStartMode(name, 0)
}
func (r *mockRemoteNode) ApplicationStartTemporary(name gen.Atom, _ gen.ApplicationOptions) error {
	return r.appStartMode(name, gen.ApplicationModeTemporary)
}
func (r *mockRemoteNode) ApplicationStartTransient(name gen.Atom, _ gen.ApplicationOptions) error {
	return r.appStartMode(name, gen.ApplicationModeTransient)
}
func (r *mockRemoteNode) ApplicationStartPermanent(name gen.Atom, _ gen.ApplicationOptions) error {
	return r.appStartMode(name, gen.ApplicationModePermanent)
}
func (r *mockRemoteNode) ApplicationInfo(name gen.Atom) (gen.ApplicationInfo, error) {
	if res, ok := r.appInfo[name]; ok {
		return res.info, res.err
	}
	return gen.ApplicationInfo{}, gen.ErrApplicationUnknown
}

func (r *mockRemoteNode) Name() gen.Atom           { return r.name }
func (r *mockRemoteNode) Uptime() int64            { return 0 }
func (r *mockRemoteNode) ConnectionUptime() int64  { return 0 }
func (r *mockRemoteNode) Version() gen.Version     { return gen.Version{} }
func (r *mockRemoteNode) Info() gen.RemoteNodeInfo { return gen.RemoteNodeInfo{Node: r.name} }
func (r *mockRemoteNode) Creation() int64          { return 0 }
func (r *mockRemoteNode) Disconnect()              {}

// MockRemoteNode is the test-config handle for Network().GetNode(name).
type MockRemoteNode struct{ rn *mockRemoteNode }

// OnGetNode returns the test-config handle for the remote node `name`. Until configured,
// Network().GetNode(name)/Node(name) return gen.ErrNoConnection.
func (m *MockNetwork) OnGetNode(name gen.Atom) *MockRemoteNode {
	rn, ok := m.net.remotes[name]
	if ok == false {
		rn = newMockRemoteNode(m.net, name)
		m.net.remotes[name] = rn
	}
	return &MockRemoteNode{rn: rn}
}

// OnSpawn stubs RemoteNode.Spawn / SpawnRegister of the named remote factory.
func (m *MockRemoteNode) OnSpawn(name gen.Atom) *RemoteSpawnReturn {
	res := &spawnResult{}
	m.rn.spawn[name] = res
	return &RemoteSpawnReturn{res: res}
}

// RemoteSpawnReturn configures what a remote Spawn of the factory returns.
type RemoteSpawnReturn struct{ res *spawnResult }

// Return makes the remote spawn return pid.
func (s *RemoteSpawnReturn) Return(pid gen.PID) { s.res.pid = pid }

// Fail makes the remote spawn return err.
func (s *RemoteSpawnReturn) Fail(err error) { s.res.err = err }

// OnApplicationStart stubs RemoteNode.ApplicationStart* of the named application.
func (m *MockRemoteNode) OnApplicationStart(name gen.Atom) *RemoteAppStartReturn {
	return &RemoteAppStartReturn{rn: m.rn, name: name}
}

// RemoteAppStartReturn configures the outcome of a remote application start.
type RemoteAppStartReturn struct {
	rn   *mockRemoteNode
	name gen.Atom
}

// Fail makes the remote application start return err (default is success).
func (s *RemoteAppStartReturn) Fail(err error) { s.rn.appStart[s.name] = err }

// OnApplicationInfo stubs RemoteNode.ApplicationInfo of the named application.
func (m *MockRemoteNode) OnApplicationInfo(name gen.Atom) *RemoteAppInfoReturn {
	res := &appInfoResult{}
	m.rn.appInfo[name] = res
	return &RemoteAppInfoReturn{res: res}
}

// RemoteAppInfoReturn configures what a remote ApplicationInfo returns.
type RemoteAppInfoReturn struct{ res *appInfoResult }

// Return makes ApplicationInfo return info.
func (s *RemoteAppInfoReturn) Return(info gen.ApplicationInfo) { s.res.info = info }

// Fail makes ApplicationInfo return err.
func (s *RemoteAppInfoReturn) Fail(err error) { s.res.err = err }
