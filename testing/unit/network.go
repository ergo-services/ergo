package unit

import (
	"fmt"
	"regexp"
	"sort"

	"ergo.services/ergo/gen"
)

// TestNetwork implements gen.Network interface for testing
type TestNetwork struct {
	test        *TestActor
	registrar   *TestRegistrar
	cookie      string
	flags       gen.NetworkFlags
	maxMsgSize  int
	mode        gen.NetworkMode
	acceptors   []TestAcceptor
	nodes       map[gen.Atom]*TestRemoteNode
	routes      map[string]routeEntry
	proxyRoutes map[string]proxyRouteEntry
	handshakes  map[string]gen.NetworkHandshake
	protos      map[string]gen.NetworkProto
	enabled     map[gen.Atom]enabledSpawn
	enabledApps map[gen.Atom]enabledApp
	Failures
}

type routeEntry struct {
	route  gen.NetworkRoute
	weight int
}

type proxyRouteEntry struct {
	route  gen.NetworkProxyRoute
	weight int
}

type enabledSpawn struct {
	factory gen.ProcessFactory
	nodes   []gen.Atom
}

type enabledApp struct {
	nodes []gen.Atom
}

// TestRemoteNode implements gen.RemoteNode interface for testing
type TestRemoteNode struct {
	name             gen.Atom
	uptime           int64
	connectionUptime int64
	version          gen.Version
	info             gen.RemoteNodeInfo
	creation         int64
	connected        bool
}

// TestAcceptor implements gen.Acceptor interface for testing
type TestAcceptor struct {
	cookie     string
	flags      gen.NetworkFlags
	maxMsgSize int
	info       gen.AcceptorInfo
}

func newTestNetwork(test *TestActor) *TestNetwork {
	return &TestNetwork{
		test:        test,
		registrar:   newTestRegistrar(test),
		cookie:      "test-cookie",
		flags:       gen.DefaultNetworkFlags,
		maxMsgSize:  1024 * 1024, // 1MB default
		mode:        gen.NetworkModeEnabled,
		acceptors:   []TestAcceptor{},
		nodes:       make(map[gen.Atom]*TestRemoteNode),
		routes:      make(map[string]routeEntry),
		proxyRoutes: make(map[string]proxyRouteEntry),
		handshakes:  make(map[string]gen.NetworkHandshake),
		protos:      make(map[string]gen.NetworkProto),
		enabled:     make(map[gen.Atom]enabledSpawn),
		enabledApps: make(map[gen.Atom]enabledApp),
		Failures:    newFailures(test.events, "network"),
	}
}

// Network interface implementation

func (n *TestNetwork) Registrar() (gen.Registrar, error) {
	return n.registrar, nil
}

func (n *TestNetwork) Cookie() string {
	return n.cookie
}

func (n *TestNetwork) SetCookie(cookie string) error {
	n.cookie = cookie
	return nil
}

func (n *TestNetwork) MaxMessageSize() int {
	return n.maxMsgSize
}

func (n *TestNetwork) SetMaxMessageSize(size int) {
	n.maxMsgSize = size
}

func (n *TestNetwork) NetworkFlags() gen.NetworkFlags {
	return n.flags
}

func (n *TestNetwork) SetNetworkFlags(flags gen.NetworkFlags) {
	n.flags = flags
}

func (n *TestNetwork) Acceptors() ([]gen.Acceptor, error) {
	acceptors := make([]gen.Acceptor, len(n.acceptors))
	for i, acc := range n.acceptors {
		acceptors[i] = &acc
	}
	return acceptors, nil
}

func (n *TestNetwork) Node(name gen.Atom) (gen.RemoteNode, error) {
	if node, exists := n.nodes[name]; exists && node.connected {
		return node, nil
	}
	return nil, gen.ErrNoConnection
}

func (n *TestNetwork) GetNode(name gen.Atom) (gen.RemoteNode, error) {
	// Check for failure injection
	if err := n.CheckMethodFailure("GetNode", name); err != nil {
		return nil, err
	}

	if node, exists := n.nodes[name]; exists {
		node.connected = true
		return node, nil
	}

	// Create new remote node
	node := &TestRemoteNode{
		name:             name,
		uptime:           1000,
		connectionUptime: 100,
		version:          gen.Version{Name: "test", Release: "1.0"},
		creation:         1234567890,
		connected:        true,
	}
	n.nodes[name] = node
	return node, nil
}

func (n *TestNetwork) GetNodeWithRoute(name gen.Atom, route gen.NetworkRoute) (gen.RemoteNode, error) {
	// For testing, we ignore the route and just return a node
	return n.GetNode(name)
}

func (n *TestNetwork) Nodes() []gen.Atom {
	var nodes []gen.Atom
	for name, node := range n.nodes {
		if node.connected {
			nodes = append(nodes, name)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		return string(nodes[i]) < string(nodes[j])
	})
	return nodes
}

func (n *TestNetwork) AddRoute(match string, route gen.NetworkRoute, weight int) error {
	n.routes[match] = routeEntry{route: route, weight: weight}
	return nil
}

func (n *TestNetwork) RemoveRoute(match string) error {
	delete(n.routes, match)
	return nil
}

func (n *TestNetwork) Route(name gen.Atom) ([]gen.NetworkRoute, error) {
	var routes []gen.NetworkRoute

	for pattern, entry := range n.routes {
		matched, err := regexp.MatchString(pattern, string(name))
		if err != nil {
			continue
		}
		if matched {
			routes = append(routes, entry.route)
		}
	}

	// Sort by weight descending
	sort.Slice(routes, func(i, j int) bool {
		return n.getRouteWeight(routes[i]) > n.getRouteWeight(routes[j])
	})

	return routes, nil
}

func (n *TestNetwork) getRouteWeight(route gen.NetworkRoute) int {
	for _, entry := range n.routes {
		if entry.route.Route.Host == route.Route.Host && entry.route.Route.Port == route.Route.Port {
			return entry.weight
		}
	}
	return 0
}

func (n *TestNetwork) AddProxyRoute(match string, proxy gen.NetworkProxyRoute, weight int) error {
	n.proxyRoutes[match] = proxyRouteEntry{route: proxy, weight: weight}
	return nil
}

func (n *TestNetwork) RemoveProxyRoute(match string) error {
	delete(n.proxyRoutes, match)
	return nil
}

func (n *TestNetwork) ProxyRoute(name gen.Atom) ([]gen.NetworkProxyRoute, error) {
	var routes []gen.NetworkProxyRoute

	for pattern, entry := range n.proxyRoutes {
		matched, err := regexp.MatchString(pattern, string(name))
		if err != nil {
			continue
		}
		if matched {
			routes = append(routes, entry.route)
		}
	}

	return routes, nil
}

func (n *TestNetwork) RegisterProto(proto gen.NetworkProto) {
	n.protos[proto.Version().String()] = proto
}

func (n *TestNetwork) RegisterHandshake(handshake gen.NetworkHandshake) {
	n.handshakes[handshake.Version().String()] = handshake
}

func (n *TestNetwork) EnableSpawn(name gen.Atom, factory gen.ProcessFactory, nodes ...gen.Atom) error {
	n.enabled[name] = enabledSpawn{factory: factory, nodes: nodes}
	return nil
}

func (n *TestNetwork) DisableSpawn(name gen.Atom, nodes ...gen.Atom) error {
	if len(nodes) == 0 {
		delete(n.enabled, name)
	} else {
		// Remove specific nodes
		if entry, exists := n.enabled[name]; exists {
			var remaining []gen.Atom
			for _, existing := range entry.nodes {
				keep := true
				for _, toRemove := range nodes {
					if existing == toRemove {
						keep = false
						break
					}
				}
				if keep {
					remaining = append(remaining, existing)
				}
			}
			if len(remaining) == 0 {
				delete(n.enabled, name)
			} else {
				entry.nodes = remaining
				n.enabled[name] = entry
			}
		}
	}
	return nil
}

func (n *TestNetwork) EnableApplicationStart(name gen.Atom, nodes ...gen.Atom) error {
	n.enabledApps[name] = enabledApp{nodes: nodes}
	return nil
}

func (n *TestNetwork) DisableApplicationStart(name gen.Atom, nodes ...gen.Atom) error {
	if len(nodes) == 0 {
		delete(n.enabledApps, name)
	} else {
		// Remove specific nodes
		if entry, exists := n.enabledApps[name]; exists {
			var remaining []gen.Atom
			for _, existing := range entry.nodes {
				keep := true
				for _, toRemove := range nodes {
					if existing == toRemove {
						keep = false
						break
					}
				}
				if keep {
					remaining = append(remaining, existing)
				}
			}
			if len(remaining) == 0 {
				delete(n.enabledApps, name)
			} else {
				entry.nodes = remaining
				n.enabledApps[name] = entry
			}
		}
	}
	return nil
}

func (n *TestNetwork) Info() (gen.NetworkInfo, error) {
	info := gen.NetworkInfo{
		Mode:             n.mode,
		MaxMessageSize:   n.maxMsgSize,
		HandshakeVersion: gen.Version{Name: "test-handshake", Release: "1.0"},
		ProtoVersion:     gen.Version{Name: "test-proto", Release: "1.0"},
		Nodes:            n.Nodes(),
		Flags:            n.flags,
	}

	info.Registrar = n.registrar.Info()

	// Add acceptor info
	for _, acc := range n.acceptors {
		info.Acceptors = append(info.Acceptors, acc.info)
	}

	// Add enabled spawn info
	for name, entry := range n.enabled {
		info.EnabledSpawn = append(info.EnabledSpawn, gen.NetworkSpawnInfo{
			Name:     name,
			Behavior: fmt.Sprintf("%T", entry.factory),
			Nodes:    entry.nodes,
		})
	}

	// Add enabled app start info
	for name, entry := range n.enabledApps {
		info.EnabledApplicationStart = append(info.EnabledApplicationStart, gen.NetworkApplicationStartInfo{
			Name:  name,
			Nodes: entry.nodes,
		})
	}

	return info, nil
}

func (n *TestNetwork) Mode() gen.NetworkMode {
	return n.mode
}

// Helper method to set network mode (for testing)
func (n *TestNetwork) SetMode(mode gen.NetworkMode) {
	n.mode = mode
}

// Helper method to add a remote node (for testing)
func (n *TestNetwork) AddRemoteNode(name gen.Atom, connected bool) *TestRemoteNode {
	node := &TestRemoteNode{
		name:             name,
		uptime:           1000,
		connectionUptime: 100,
		version:          gen.Version{Name: "test", Release: "1.0"},
		creation:         1234567890,
		connected:        connected,
	}
	n.nodes[name] = node
	return node
}

// RemoteNode interface implementation

func (rn *TestRemoteNode) Name() gen.Atom {
	return rn.name
}

func (rn *TestRemoteNode) Uptime() int64 {
	return rn.uptime
}

func (rn *TestRemoteNode) ConnectionUptime() int64 {
	return rn.connectionUptime
}

func (rn *TestRemoteNode) Version() gen.Version {
	return rn.version
}

func (rn *TestRemoteNode) Info() gen.RemoteNodeInfo {
	return rn.info
}

func (rn *TestRemoteNode) Spawn(name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	// For testing, create a mock PID
	return gen.PID{Node: rn.name, ID: 12345, Creation: rn.creation}, nil
}

func (rn *TestRemoteNode) SpawnRegister(
	register gen.Atom,
	name gen.Atom,
	options gen.ProcessOptions,
	args ...any,
) (gen.PID, error) {
	return gen.PID{Node: rn.name, ID: 12346, Creation: rn.creation}, nil
}

func (rn *TestRemoteNode) ApplicationStart(name gen.Atom, options gen.ApplicationOptions) error {
	return nil // Mock success
}

func (rn *TestRemoteNode) ApplicationStartTemporary(name gen.Atom, options gen.ApplicationOptions) error {
	return nil // Mock success
}

func (rn *TestRemoteNode) ApplicationStartTransient(name gen.Atom, options gen.ApplicationOptions) error {
	return nil // Mock success
}

func (rn *TestRemoteNode) ApplicationStartPermanent(name gen.Atom, options gen.ApplicationOptions) error {
	return nil // Mock success
}

func (rn *TestRemoteNode) Creation() int64 {
	return rn.creation
}

func (rn *TestRemoteNode) Disconnect() {
	rn.connected = false
}

// Helper methods for testing

func (rn *TestRemoteNode) SetUptime(uptime int64) {
	rn.uptime = uptime
}

func (rn *TestRemoteNode) SetConnectionUptime(uptime int64) {
	rn.connectionUptime = uptime
}

func (rn *TestRemoteNode) SetVersion(version gen.Version) {
	rn.version = version
}

func (rn *TestRemoteNode) SetInfo(info gen.RemoteNodeInfo) {
	rn.info = info
}

func (rn *TestRemoteNode) SetConnected(connected bool) {
	rn.connected = connected
}

// Acceptor interface implementation

func (a *TestAcceptor) Cookie() string {
	return a.cookie
}

func (a *TestAcceptor) SetCookie(cookie string) {
	a.cookie = cookie
}

func (a *TestAcceptor) NetworkFlags() gen.NetworkFlags {
	return a.flags
}

func (a *TestAcceptor) SetNetworkFlags(flags gen.NetworkFlags) {
	a.flags = flags
}

func (a *TestAcceptor) MaxMessageSize() int {
	return a.maxMsgSize
}

func (a *TestAcceptor) SetMaxMessageSize(size int) {
	a.maxMsgSize = size
}

func (a *TestAcceptor) Info() gen.AcceptorInfo {
	return a.info
}
