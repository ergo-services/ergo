package node

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/handshake"
	"ergo.services/ergo/net/proto"
	"ergo.services/ergo/net/registrar"
)

func createNetwork(node *node) *network {
	n := &network{
		node:             node,
		staticRoutes:     &staticRoutes{},
		staticProxies:    &staticProxies{},
		defaultHandshake: handshake.Create(handshake.Options{}),
		defaultProto:     proto.Create(),
	}
	// register standard handshake and proto
	n.RegisterHandshake(n.defaultHandshake)
	n.RegisterProto(n.defaultProto)
	return n
}

type network struct {
	running atomic.Bool

	mode       gen.NetworkMode
	flags      gen.NetworkFlags
	skipverify bool

	node      *node
	registrar gen.Registrar

	acceptors []*acceptor

	defaultHandshake gen.NetworkHandshake
	defaultProto     gen.NetworkProto

	handshakes sync.Map // .Version().String() -> handshake
	protos     sync.Map // .Version().String() -> proto

	cookie                  string
	maxmessagesize          int
	handshakeTimeoutDefault time.Duration
	softwareKeepAliveMisses int
	fragmentSize            int
	fragmentTimeout         int
	maxFragmentAssemblies   int

	staticRoutes  *staticRoutes
	staticProxies *staticProxies

	enableSpawn    sync.Map
	enableAppStart sync.Map

	connections     sync.Map // gen.Atom (peer name) => gen.Connection (routing index)
	connectionsByID sync.Map // string (ConnectionID) => gen.Connection (authoritative dedup + pool-join attach)
	pending         sync.Map // gen.Atom (peer name) => *pendingEntry
	// mergeMu serializes the connection-merge decision (register/take-over/drop), like
	// OTP's net_kernel: handshakes run concurrently, the decision is one at a time.
	mergeMu sync.Mutex

	connectionsEstablished atomic.Uint64
	connectionsLost        atomic.Uint64
}

type pendingEntry struct {
	ready chan struct{} // closed when connect finishes (success or failure)
}

func (n *network) Registrar() (gen.Registrar, error) {
	if n.running.Load() == false {
		return nil, gen.ErrNetworkStopped
	}
	return n.registrar, nil
}

func (n *network) ResolveApplication(name gen.Atom) (gen.ApplicationRoutes, error) {
	reg, err := n.Registrar()
	if err != nil {
		return nil, err
	}
	return reg.Resolver().ResolveApplication(name)
}

func (n *network) Cookie() string {
	return n.cookie
}
func (n *network) SetCookie(cookie string) error {
	n.cookie = cookie
	if lib.Verbose() {
		n.node.Log().Trace("updated cookie")
	}
	return nil
}

func (n *network) NetworkFlags() gen.NetworkFlags {
	return n.flags
}

func (n *network) SetNetworkFlags(flags gen.NetworkFlags) {
	if flags.Enable == false {
		flags = gen.DefaultNetworkFlags
	}
	n.flags = flags
}

func (n *network) MaxMessageSize() int {
	return n.maxmessagesize
}

func (n *network) SetMaxMessageSize(size int) {
	if size < 0 {
		size = 0
	}
	n.maxmessagesize = size
}

func (n *network) Acceptors() ([]gen.Acceptor, error) {
	var acceptors []gen.Acceptor
	if n.running.Load() == false {
		return nil, gen.ErrNetworkStopped
	}
	for _, acceptor := range n.acceptors {
		acceptors = append(acceptors, acceptor)
	}
	return acceptors, nil
}

func (n *network) Node(name gen.Atom) (gen.RemoteNode, error) {
	c, err := n.Connection(name)
	if err != nil {
		return nil, err
	}
	return c.Node(), nil
}

func (n *network) handshakeTimeout(v time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	if n.handshakeTimeoutDefault > 0 {
		return n.handshakeTimeoutDefault
	}
	return gen.DefaultHandshakeTimeout
}

func (n *network) GetNode(name gen.Atom) (gen.RemoteNode, error) {
	c, err := n.GetConnection(name)
	if err != nil {
		return nil, err
	}
	return c.Node(), nil
}

func (n *network) GetNodeWithRoute(name gen.Atom, route gen.NetworkRoute) (gen.RemoteNode, error) {
	var emptyVersion gen.Version

	route.InsecureSkipVerify = n.skipverify

	if route.Resolver != nil {
		resolved, err := route.Resolver.Resolve(name)
		if err != nil {
			return nil, err
		}
		if len(resolved) == 0 {
			return nil, gen.ErrNoRoute
		}
		route.Route.Port = resolved[0].Port
		route.Route.TLS = resolved[0].TLS
		if route.Route.HandshakeVersion == emptyVersion {
			route.Route.HandshakeVersion = resolved[0].HandshakeVersion
		}
		if route.Route.ProtoVersion == emptyVersion {
			route.Route.ProtoVersion = resolved[0].ProtoVersion
		}
		if route.Route.Host == "" {
			route.Route.Host = resolved[0].Host
		}
	}

	if route.Route.Port == 0 {
		return nil, gen.ErrNoRoute
	}

	if route.Route.HandshakeVersion == emptyVersion {
		route.Route.HandshakeVersion = n.defaultHandshake.Version()
	}

	if route.Route.ProtoVersion == emptyVersion {
		route.Route.ProtoVersion = n.defaultProto.Version()
	}

	c, err := n.connect(name, route)
	if err != nil {
		return nil, err
	}
	return c.Node(), nil
}

func (n *network) AddRoute(match string, route gen.NetworkRoute, weight int) error {
	var emptyVersion gen.Version
	if route.Route.HandshakeVersion == emptyVersion {
		route.Route.HandshakeVersion = n.defaultHandshake.Version()
	}
	if route.Route.ProtoVersion == emptyVersion {
		route.Route.ProtoVersion = n.defaultProto.Version()
	}
	if err := n.staticRoutes.add(match, route, weight); err != nil {
		return err
	}
	if lib.Verbose() {
		n.node.Log().Trace("added static route %s with weight %d", match, weight)
	}
	return nil
}

func (n *network) RemoveRoute(match string) error {
	if err := n.staticRoutes.remove(match); err != nil {
		return err
	}
	if lib.Verbose() {
		n.node.Log().Trace("removed static route %s", match)
	}
	return nil
}

func (n *network) Route(name gen.Atom) ([]gen.NetworkRoute, error) {
	if routes, found := n.staticRoutes.lookup(string(name)); found {
		return routes, nil
	}
	return nil, gen.ErrNoRoute
}

func (n *network) AddProxyRoute(match string, route gen.NetworkProxyRoute, weight int) error {
	if err := n.staticProxies.add(match, route, weight); err != nil {
		return err
	}

	if lib.Verbose() {
		n.node.Log().Trace("added static proxy route %s with weight %d", match, weight)
	}
	return nil
}

func (n *network) RemoveProxyRoute(match string) error {
	if err := n.staticProxies.remove(match); err != nil {
		return err
	}
	if lib.Verbose() {
		n.node.Log().Trace("removed static proxy route %s", match)
	}
	return nil
}

func (n *network) ProxyRoute(name gen.Atom) ([]gen.NetworkProxyRoute, error) {

	if routes, found := n.staticProxies.lookup(string(name)); found {
		return routes, nil
	}
	return nil, gen.ErrNoRoute
}

type enableSpawn struct {
	sync.RWMutex
	factory  gen.ProcessFactory
	behavior string
	all      bool
	nodes    map[gen.Atom]bool
}

func (n *network) EnableSpawn(name gen.Atom, factory gen.ProcessFactory, nodes ...gen.Atom) error {

	if factory == nil {
		return gen.ErrIncorrect
	}

	enable := &enableSpawn{
		factory:  factory,
		nodes:    make(map[gen.Atom]bool),
		behavior: strings.TrimPrefix(reflect.TypeOf(factory()).String(), "*"),
	}

	v, exist := n.enableSpawn.LoadOrStore(name, enable)
	if exist {
		enable = v.(*enableSpawn)
		if reflect.TypeOf(enable.factory()) != reflect.TypeOf(factory()) {
			return fmt.Errorf("%s associated with another process factory", name)
		}
	}
	enable.Lock()
	if len(nodes) == 0 {
		// allow any node to spawn this process
		enable.all = true
		enable.nodes = make(map[gen.Atom]bool)
	} else {
		enable.all = false
		for _, nn := range nodes {
			enable.nodes[nn] = true
		}
	}
	enable.Unlock()

	return nil
}

func (n *network) getEnabledSpawn(name gen.Atom, source gen.Atom) (gen.ProcessFactory, error) {
	v, found := n.enableSpawn.Load(name)
	if found == false {
		return nil, gen.ErrNameUnknown
	}
	enable := v.(*enableSpawn)
	enable.RLock()
	allowed, ok := enable.nodes[source]
	if ok == false {
		allowed = enable.all
	}
	enable.RUnlock()
	if allowed == false {
		return nil, gen.ErrNotAllowed
	}
	return enable.factory, nil
}

func (n *network) listEnabledSpawn() []gen.NetworkSpawnInfo {
	info := []gen.NetworkSpawnInfo{}

	n.enableSpawn.Range(func(k, v any) bool {
		enable := v.(*enableSpawn)
		nsi := gen.NetworkSpawnInfo{
			Name:     k.(gen.Atom),
			Behavior: enable.behavior,
		}
		enable.RLock()
		for peer, en := range enable.nodes {
			if en == false {
				continue
			}
			nsi.Nodes = append(nsi.Nodes, peer)
		}
		enable.RUnlock()
		info = append(info, nsi)
		return true
	})
	return info
}

func (n *network) DisableSpawn(name gen.Atom, nodes ...gen.Atom) error {
	if len(nodes) == 0 {
		if _, exist := n.enableSpawn.LoadAndDelete(name); exist == false {
			return gen.ErrUnknown
		}
		return nil
	}
	v, exist := n.enableSpawn.Load(name)
	if exist == false {
		return gen.ErrUnknown
	}
	enable := v.(*enableSpawn)
	enable.Lock()
	for _, nn := range nodes {
		enable.nodes[nn] = false
	}
	enable.Unlock()
	return nil
}

type enableAppStart struct {
	sync.RWMutex
	all   bool
	nodes map[gen.Atom]bool
}

func (n *network) EnableApplicationStart(name gen.Atom, nodes ...gen.Atom) error {
	enable := &enableAppStart{
		nodes: make(map[gen.Atom]bool),
	}

	v, exist := n.enableAppStart.LoadOrStore(name, enable)
	if exist {
		enable = v.(*enableAppStart)
	}
	enable.Lock()
	if len(nodes) == 0 {
		// allow any node to start this app
		enable.all = true
		enable.nodes = make(map[gen.Atom]bool)
	} else {
		enable.all = false
		for _, nn := range nodes {
			enable.nodes[nn] = true
		}
	}
	enable.Unlock()

	return nil
}

func (n *network) isEnabledApplicationStart(name gen.Atom, source gen.Atom) error {
	v, found := n.enableAppStart.Load(name)
	if found == false {
		return gen.ErrNameUnknown
	}
	enable := v.(*enableAppStart)
	enable.RLock()
	allowed, ok := enable.nodes[source]
	if ok == false {
		allowed = enable.all
	}
	enable.RUnlock()
	if allowed == false {
		return gen.ErrNotAllowed
	}
	return nil
}

func (n *network) listEnabledApplicationStart() []gen.NetworkApplicationStartInfo {
	info := []gen.NetworkApplicationStartInfo{}

	n.enableAppStart.Range(func(k, v any) bool {
		nas := gen.NetworkApplicationStartInfo{
			Name: k.(gen.Atom),
		}
		enable := v.(*enableAppStart)
		enable.RLock()
		for peer, en := range enable.nodes {
			if en == false {
				continue
			}
			nas.Nodes = append(nas.Nodes, peer)
		}
		enable.RUnlock()
		info = append(info, nas)
		return true
	})
	return info
}

func (n *network) DisableApplicationStart(name gen.Atom, nodes ...gen.Atom) error {
	if len(nodes) == 0 {
		if _, exist := n.enableAppStart.LoadAndDelete(name); exist == false {
			return gen.ErrUnknown
		}
		return nil
	}
	v, exist := n.enableAppStart.Load(name)
	if exist == false {
		return gen.ErrUnknown
	}
	enable := v.(*enableAppStart)
	enable.Lock()
	for _, nn := range nodes {
		enable.nodes[nn] = false
	}
	enable.Unlock()
	return nil
}

func (n *network) RegisterHandshake(handshake gen.NetworkHandshake) {
	if handshake == nil {
		n.node.Log().Error("unable to register nil value as a handshake")
		return
	}
	_, exist := n.handshakes.LoadOrStore(handshake.Version().Str(), handshake)
	if exist == false {
		if lib.Verbose() {
			n.node.Log().Trace("registered handshake %s", handshake.Version())
		}
	}
}

func (n *network) RegisterProto(proto gen.NetworkProto) {
	if proto == nil {
		n.node.Log().Error("unable to register nil value as a proto ")
		return
	}
	_, exist := n.protos.LoadOrStore(proto.Version().Str(), proto)
	if exist == false {
		if lib.Verbose() {
			n.node.Log().Trace("registered proto %s", proto.Version())
		}
	}
}

func (n *network) Nodes() []gen.Atom {
	var nodes []gen.Atom

	n.connections.Range(func(k, _ any) bool {
		node := k.(gen.Atom)
		nodes = append(nodes, node)
		return true
	})

	return nodes
}

func (n *network) Info() (gen.NetworkInfo, error) {
	var info gen.NetworkInfo

	if n.running.Load() == false {
		return info, gen.ErrNetworkStopped
	}

	info.Mode = n.mode
	info.Registrar = n.registrar.Info()

	for _, acceptor := range n.acceptors {
		info.Acceptors = append(info.Acceptors, acceptor.Info())
	}
	info.MaxMessageSize = n.maxmessagesize
	info.HandshakeVersion = n.defaultHandshake.Version()
	info.ProtoVersion = n.defaultProto.Version()

	n.connections.Range(func(k, _ any) bool {
		node := k.(gen.Atom)
		info.Nodes = append(info.Nodes, node)
		return true
	})

	info.Routes = n.staticRoutes.info()
	info.ProxyRoutes = n.staticProxies.info()

	info.Flags = n.flags

	info.ConnectionsEstablished = n.connectionsEstablished.Load()
	info.ConnectionsLost = n.connectionsLost.Load()

	info.EnabledSpawn = n.listEnabledSpawn()
	info.EnabledApplicationStart = n.listEnabledApplicationStart()

	return info, nil
}

func (n *network) Mode() gen.NetworkMode {
	return n.mode
}

func (n *network) Protos() []gen.NetworkProto {
	var list []gen.NetworkProto
	n.protos.Range(func(_, v any) bool {
		list = append(list, v.(gen.NetworkProto))
		return true
	})
	return list
}

// typeRegistryEntry pairs a proto with its TypeRegistry capability.
type typeRegistryEntry struct {
	proto    gen.NetworkProto
	registry gen.TypeRegistry
}

func (n *network) typeRegistries() []typeRegistryEntry {
	var list []typeRegistryEntry
	n.protos.Range(func(_, v any) bool {
		p := v.(gen.NetworkProto)
		if r, ok := p.(gen.TypeRegistry); ok {
			list = append(list, typeRegistryEntry{p, r})
		}
		return true
	})
	return list
}

func (n *network) RegisterType(v any) error {
	regs := n.typeRegistries()
	if len(regs) == 0 {
		return gen.ErrUnsupported
	}
	var errs []string
	for _, r := range regs {
		err := r.registry.RegisterType(v)
		if err == nil || err == gen.ErrTaken {
			continue
		}
		errs = append(errs, fmt.Sprintf("%s: %s", r.proto.Version(), err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("RegisterType: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (n *network) RegisterTypes(types []any) error {
	regs := n.typeRegistries()
	if len(regs) == 0 {
		return gen.ErrUnsupported
	}
	var errs []string
	for _, r := range regs {
		err := r.registry.RegisterTypes(types)
		if err == nil || err == gen.ErrTaken {
			continue
		}
		errs = append(errs, fmt.Sprintf("%s: %s", r.proto.Version(), err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("RegisterTypes: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (n *network) RegisterError(e error) error {
	regs := n.typeRegistries()
	if len(regs) == 0 {
		return gen.ErrUnsupported
	}
	var errs []string
	for _, r := range regs {
		err := r.registry.RegisterError(e)
		if err == nil || err == gen.ErrTaken {
			continue
		}
		errs = append(errs, fmt.Sprintf("%s: %s", r.proto.Version(), err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("RegisterError: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (n *network) RegisterErrors(list []error) error {
	regs := n.typeRegistries()
	if len(regs) == 0 {
		return gen.ErrUnsupported
	}
	var errs []string
	for _, r := range regs {
		err := r.registry.RegisterErrors(list)
		if err == nil || err == gen.ErrTaken {
			continue
		}
		errs = append(errs, fmt.Sprintf("%s: %s", r.proto.Version(), err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("RegisterErrors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (n *network) RegisterAtom(a gen.Atom) error {
	regs := n.typeRegistries()
	if len(regs) == 0 {
		return gen.ErrUnsupported
	}
	var errs []string
	for _, r := range regs {
		err := r.registry.RegisterAtom(a)
		if err == nil || err == gen.ErrTaken {
			continue
		}
		errs = append(errs, fmt.Sprintf("%s: %s", r.proto.Version(), err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("RegisterAtom: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (n *network) RegisterAtoms(atoms []gen.Atom) error {
	regs := n.typeRegistries()
	if len(regs) == 0 {
		return gen.ErrUnsupported
	}
	var errs []string
	for _, r := range regs {
		err := r.registry.RegisterAtoms(atoms)
		if err == nil || err == gen.ErrTaken {
			continue
		}
		errs = append(errs, fmt.Sprintf("%s: %s", r.proto.Version(), err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("RegisterAtoms: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (n *network) RegisteredTypes() []gen.RegisteredTypeInfo {
	var all []gen.RegisteredTypeInfo
	for _, r := range n.typeRegistries() {
		all = append(all, r.registry.RegisteredTypes()...)
	}
	return all
}

func (n *network) LookupType(name string) (reflect.Type, bool) {
	for _, r := range n.typeRegistries() {
		if t, ok := r.registry.LookupType(name); ok {
			return t, true
		}
	}
	return nil, false
}

//
// internals
//

// Connection and GetConnection aren't exposed via gen.Network
func (n *network) Connection(name gen.Atom) (gen.Connection, error) {
	v, found := n.connections.Load(name)
	if found == false {
		return nil, gen.ErrNoConnection
	}
	return v.(gen.Connection), nil
}

func (n *network) GetConnection(name gen.Atom) (gen.Connection, error) {
	v, found := n.connections.Load(name)
	if found {
		return v.(gen.Connection), nil
	}

	registrar := n.registrar
	if registrar == nil {
		return nil, gen.ErrNoRoute
	}

	if lib.Verbose() {
		n.node.Log().Trace("trying to make connection with %s", name)
	}
	// check the static routes
	if sroutes, found := n.staticRoutes.lookup(string(name)); found {
		if lib.Verbose() {
			n.node.Log().Trace("found %s static route[s] for %s", len(sroutes), name)
		}
		for i, sroute := range sroutes {
			sroute.InsecureSkipVerify = n.skipverify
			if sroute.Resolver == nil {
				if lib.Verbose() {
					n.node.Log().Trace("use static route to %s (%d)", name, i+1)
				}
				if c, err := n.connect(name, sroute); err == nil {
					return c, nil
				} else {
					if lib.Verbose() {
						n.node.Log().Trace("unable to connect to %s using static route: %s", name, err)
					}
				}
				continue
			}

			if lib.Verbose() {
				n.node.Log().Trace("use static route to %s with resolver (%d)", name, i+1)
			}
			nr, err := sroute.Resolver.Resolve(name)
			if err != nil {
				if lib.Verbose() {
					n.node.Log().Trace("failed to resolve %s: %s", name, err)
				}
				continue
			}

			for _, route := range nr {
				nroute := gen.NetworkRoute{
					Route:              route,
					InsecureSkipVerify: n.skipverify,
				}
				if nroute.Route.TLS && nroute.Cert == nil {
					nroute.Cert = n.node.certmanager
				}
				if nroute.Cookie == "" {
					nroute.Cookie = n.cookie
				}
				if c, err := n.connect(name, nroute); err == nil {
					return c, nil
				} else {
					if lib.Verbose() {
						n.node.Log().Trace("unable to connect to %s using static route (with resolver): %s", name, err)
					}
				}
			}
		}
		return nil, gen.ErrNoRoute
	}

	// check the static proxy routes
	if proutes, found := n.staticProxies.lookup(string(name)); found {
		if lib.Verbose() {
			n.node.Log().Trace("found %d static proxy route[s] for %s", len(proutes), name)
		}
		for i, proute := range proutes {
			if proute.Resolver == nil {
				if lib.Verbose() {
					n.node.Log().Trace("use static proxy route to %s (%d)", name, i+1)
				}
				if c, err := n.connectProxy(name, proute); err == nil {
					return c, nil
				}
				continue
			}

			if lib.Verbose() {
				n.node.Log().Trace("use static proxy route to %s with resolver (%d)", name, i+1)
			}
			pr, err := proute.Resolver.ResolveProxy(name)
			if err != nil {
				if lib.Verbose() {
					n.node.Log().Trace("failed to resolve proxy for %s: %s", name, err)
				}
				continue
			}

			for _, route := range pr {
				nproute := gen.NetworkProxyRoute{
					Route: route,
				}
				if c, err := n.connectProxy(name, nproute); err == nil {
					return c, nil
				} else {
					if lib.Verbose() {
						n.node.Log().Trace("unable to connect to %s using proxy route: %s", name, err)
					}
				}
			}
		}
		return nil, gen.ErrNoRoute
	}

	// resolve it
	if nr, err := registrar.Resolver().Resolve(name); err == nil {
		if lib.Verbose() {
			n.node.Log().Trace("resolved %d route[s] for %s", len(nr), name)
		}

		for _, route := range nr {
			nroute := gen.NetworkRoute{
				Route:              route,
				InsecureSkipVerify: n.skipverify,
				Cookie:             n.cookie,
			}

			if route.TLS {
				nroute.Cert = n.node.certmanager
			}

			if c, err := n.connect(name, nroute); err == nil {
				return c, nil
			} else {
				if lib.Verbose() {
					n.node.Log().Trace("unable to connect to %s: %s", name, err)
				}
			}
		}
		if lib.Verbose() {
			n.node.Log().Trace("unable to connect to %s directly, looking up proxies...", name)
		}
	} else {
		if lib.Verbose() {
			n.node.Log().Trace("attempt to resolve %s failed: %s", name, err)
		}
	}

	// resolve proxy
	if pr, err := registrar.Resolver().ResolveProxy(name); err == nil {
		if lib.Verbose() {
			n.node.Log().Trace("resolved %d proxy routes for %s", len(pr), name)
		}

		// check if we already have connection with the proxy, so use it
		// for the proxy connection
		for _, route := range pr {
			// check if we have connection to the proxy node
			if _, err := n.Connection(route.Proxy); err != nil {
				continue
			}
			// try to use the existing connection to the proxy node
			nproute := gen.NetworkProxyRoute{
				Route: route,
			}
			if c, err := n.connectProxy(name, nproute); err == nil {
				return c, nil
			} else {
				if lib.Verbose() {
					n.node.Log().Trace("unable to connect to %s using resolve proxy: %s", name, err)
				}
			}
		}
	}

	return nil, gen.ErrNoRoute
}

// acquirePending tries to become the goroutine that connects to `name`.
// If another goroutine is already connecting, waits for it to finish and
// checks the result. Retries up to 3 times if the other goroutine fails.
// Returns: (entry, nil) if acquired; (nil, nil) if connection appeared; (nil, err) on failure.
func (n *network) acquirePending(name gen.Atom) (*pendingEntry, error) {
	for attempt := 0; attempt < 3; attempt++ {
		entry := &pendingEntry{ready: make(chan struct{})}
		actual, loaded := n.pending.LoadOrStore(name, entry)
		if loaded == false {
			return entry, nil // acquired the slot
		}

		// another connect in progress, wait for it
		pe := actual.(*pendingEntry)
		select {
		case <-pe.ready:
			// connect finished (success or failure)
		case <-time.After(5 * time.Second):
			return nil, fmt.Errorf("connection to %s: pending timeout", name)
		}

		// check if connection appeared
		if _, ok := n.connections.Load(name); ok {
			return nil, nil // connection exists
		}

		// connect failed and pending was cleared, retry LoadOrStore
	}
	return nil, fmt.Errorf("connection to %s: 3 attempts exhausted", name)
}

func (n *network) connect(name gen.Atom, route gen.NetworkRoute) (gen.Connection, error) {
	var dial func(network, addr string) (net.Conn, error)

	if n.running.Load() == false {
		return nil, gen.ErrNetworkStopped
	}

	vhandshake, found := n.handshakes.Load(route.Route.HandshakeVersion.Str())
	if found == false {
		return nil, fmt.Errorf("no handshake handler for %s", route.Route.HandshakeVersion)
	}
	vproto, found := n.protos.Load(route.Route.ProtoVersion.Str())
	if found == false {
		return nil, fmt.Errorf("no proto handler for %s", route.Route.ProtoVersion)
	}

	hs := vhandshake.(gen.NetworkHandshake)
	proto := vproto.(gen.NetworkProto)

	if route.Route.Host == "" {
		route.Route.Host = name.Host()
	}

	// acquire pending slot (waits for ongoing connect, retries on failure)
	entry, err := n.acquirePending(name)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		// connection appeared while waiting
		v, ok := n.connections.Load(name)
		if ok == false {
			return nil, gen.ErrNoRoute
		}
		return v.(gen.Connection), nil
	}
	defer func() {
		n.pending.Delete(name)
		close(entry.ready) // wake ALL waiting goroutines
	}()

	if lib.Verbose() {
		n.node.Log().Trace("trying to connect to %s (%s:%d, tls:%v)",
			name, route.Route.Host, route.Route.Port, route.Route.TLS)
	}

	dialer := &net.Dialer{
		KeepAlive: gen.DefaultKeepAlivePeriod,
		Timeout:   3 * time.Second, // timeout to establish TCP-connection
	}

	if route.Route.TLS {
		tlsconfig := &tls.Config{
			InsecureSkipVerify: route.InsecureSkipVerify,
			MinVersion:         tls.VersionTLS12,
		}
		// use client certificate if provided
		if route.Cert != nil {
			tlsconfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
				cert := route.Cert.GetCertificate()
				return &cert, nil
			}
			// check for mTLS support (CA pool for server verification, server name)
			if cam, ok := route.Cert.(gen.CertAuthManager); ok {
				tlsconfig.RootCAs = cam.RootCAs()
				if serverName := cam.ServerName(); serverName != "" {
					tlsconfig.ServerName = serverName
				}
			}
		}
		tlsdialer := tls.Dialer{
			NetDialer: dialer,
			Config:    tlsconfig,
		}
		dial = tlsdialer.Dial
	} else {
		dial = dialer.Dial
	}
	dsn := net.JoinHostPort(route.Route.Host, strconv.Itoa(int(route.Route.Port)))
	conn, err := dial("tcp", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(n.handshakeTimeout(route.HandshakeTimeout)))

	hopts := gen.HandshakeOptions{
		Cookie:         route.Cookie,
		Flags:          route.Flags,
		MaxMessageSize: n.maxmessagesize,
		CheckPending: func(peer gen.Atom) bool {
			_, exists := n.pending.Load(peer)
			return exists
		},
	}

	if hopts.Cookie == "" {
		hopts.Cookie = n.cookie
	}
	if hopts.Flags.Enable == false {
		hopts.Flags = n.flags
	}
	// period for keepalive is already in hopts.Flags (from route.Flags or n.flags)

	result, err := hs.Start(n.node, conn, hopts)
	if err != nil {
		conn.Close()
		// on simultaneous connect rejection, check if accept path established connection
		if v, ok := n.connections.Load(name); ok {
			return v.(gen.Connection), nil
		}
		return nil, err
	}

	if result.Peer != name {
		conn.Close()
		return nil, fmt.Errorf("remote node %s introduced itself as %s", name, result.Peer)
	}

	mapping := make(map[gen.Atom]gen.Atom)
	for k, v := range route.AtomMapping {
		mapping[k] = v
	}
	for k, v := range result.AtomMapping {
		mapping[k] = v
	}
	result.AtomMapping = mapping

	if route.LogLevel == gen.LogLevelDefault {
		route.LogLevel = n.node.Log().Level()
	}
	log := createLog(route.LogLevel, n.node.dolog)
	logSource := gen.MessageLogNetwork{
		Node:     n.node.name,
		Peer:     result.Peer,
		Creation: result.PeerCreation,
	}
	log.setSource(logSource)

	// inject options into ConnectionOptions
	if opts, ok := result.Custom.(handshake.ConnectionOptions); ok {
		opts.SoftwareKeepAliveMisses = n.keepAliveMisses(route.SoftwareKeepAliveMisses)
		opts.FragmentSize = n.fragmentSize
		opts.FragmentTimeout = n.fragmentTimeout
		opts.MaxFragmentAssemblies = n.maxFragmentAssemblies
		result.Custom = opts
	}

	pconn, err := proto.NewConnection(n.node.core, result, log)
	if err != nil {
		conn.Close()
		return nil, err
	}

	redial := func(dsn, id string) (net.Conn, []byte, error) {
		c, err := dial("tcp", dsn)
		if err != nil {
			return nil, nil, err
		}
		c.SetDeadline(time.Now().Add(n.handshakeTimeout(route.HandshakeTimeout)))
		tail, err := hs.Join(n.node, c, id, hopts)
		if err != nil {
			c.Close()
			return nil, nil, err
		}
		return c, tail, nil
	}

	// single primary: the connection for connID is the canonical-direction primary
	// (initiated by the smaller-named node). Both ends compute the same survivor, so no
	// cross-kill. The merge decision is serialized (mergeMu) like OTP's net_kernel; the
	// handshake above ran concurrently.
	localIsCanonical := n.node.name < result.Peer

	// the non-canonical outgoing primary stays passive (dial==nil, no self-redial): under a
	// simultaneous connect it is the losing direction and the canonical end closes its TCP;
	// were it joined with a redial it would reconnect as a pool-join and attach to the
	// canonical connection as a TCP the writer does not track (pool overshoot, churn that
	// never settles). The pool is still filled by the dialer (real redial passed to serve);
	// a non-canonical dialer fills only after the acceptor go-ahead, never a superseded one.
	primaryDial := redial
	if localIsCanonical == false {
		primaryDial = nil
	}

	n.mergeMu.Lock()
	owner, loaded := n.connectionsByID.LoadOrStore(result.ConnectionID, pconn)
	if loaded {
		oc := owner.(gen.Connection)
		if localIsCanonical == false {
			// our outgoing is the losing direction: drop ours, adopt the owner
			n.mergeMu.Unlock()
			pconn.Terminate(nil)
			conn.Close()
			return oc, nil
		}
		// our outgoing is the canonical winner but a provisional (losing direction)
		// registered first: take over, replace it, then drop it
		n.connectionsByID.Store(result.ConnectionID, pconn)
		n.registerConnection(result.Peer, pconn)
		if jerr := pconn.Join(conn, result.ConnectionID, primaryDial, result.Tail); jerr != nil {
			n.connectionsByID.CompareAndDelete(result.ConnectionID, pconn)
			n.mergeMu.Unlock()
			pconn.Terminate(nil)
			conn.Close()
			return nil, jerr
		}
		n.mergeMu.Unlock()
		go n.serve(proto, pconn, redial, result.ConnectionID)
		oc.Terminate(nil) // drop the provisional loser
		return pconn, nil
	}

	n.registerConnection(result.Peer, pconn)
	if jerr := pconn.Join(conn, result.ConnectionID, primaryDial, result.Tail); jerr != nil {
		n.connectionsByID.CompareAndDelete(result.ConnectionID, pconn)
		n.mergeMu.Unlock()
		pconn.Terminate(nil)
		conn.Close()
		return nil, jerr
	}
	n.mergeMu.Unlock()
	go n.serve(proto, pconn, redial, result.ConnectionID)
	return pconn, nil
}

func (n *network) serve(proto gen.NetworkProto, conn gen.Connection, redial gen.NetworkDial, connID string) {
	name := conn.Node().Name()
	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				n.node.log.Panic("connection with %s (%s) terminated abnormally: %v", name, name.CRC32(), r)
				n.unregisterConnection(name, conn, connID, gen.TerminateReasonPanic)
				conn.Terminate(gen.TerminateReasonPanic)
			}
		}()
	}

	err := proto.Serve(conn, redial)
	reason := err
	if reason == nil {
		reason = gen.TerminateReasonNormal
	}
	n.unregisterConnection(name, conn, connID, reason)
	conn.Terminate(err)
}

func (n *network) connectProxy(name gen.Atom, route gen.NetworkProxyRoute) (gen.Connection, error) {
	if lib.Verbose() {
		n.node.Log().Trace("trying to connect to %s (via proxy %s)", name, route.Route.Proxy)
	}
	// TODO will be implemented later
	n.node.log.Warning("proxy feature is not implemented yet")

	return nil, gen.ErrUnsupported
}

func (n *network) keepAliveMisses(v int) int {
	if v > 0 {
		return v
	}
	if n.softwareKeepAliveMisses > 0 {
		return n.softwareKeepAliveMisses
	}
	return gen.DefaultSoftwareKeepAliveMisses
}

func (n *network) stop() error {
	if swapped := n.running.CompareAndSwap(true, false); swapped == false {
		return fmt.Errorf("network stack is already stopped")
	}

	n.registrar.Terminate()

	// stop acceptors
	for _, a := range n.acceptors {
		a.l.Close()
	}

	n.connections.Range(func(_, v any) bool {
		c := v.(gen.Connection)
		c.Terminate(gen.TerminateReasonNormal)
		return true
	})

	return nil
}

func (n *network) start(options gen.NetworkOptions) error {
	if swapped := n.running.CompareAndSwap(false, true); swapped == false {
		return fmt.Errorf("network stack is already running")
	}

	n.mode = options.Mode
	if options.Mode == gen.NetworkModeDisabled {
		n.running.Store(false)
		n.node.log.Info("network is disabled")
		return nil
	}

	if lib.Verbose() {
		n.node.log.Trace("starting network...")
	}

	n.skipverify = options.InsecureSkipVerify
	n.registrar = options.Registrar
	if n.registrar == nil {
		n.registrar = registrar.Create(registrar.Options{})
	}

	if options.Cookie == "" {
		n.node.log.Warning("cookie is empty (gen.NetworkOptions), used randomized value")
		options.Cookie = lib.RandomString(16)
	}
	n.cookie = options.Cookie
	n.maxmessagesize = options.MaxMessageSize
	n.handshakeTimeoutDefault = options.HandshakeTimeout

	if options.Flags.Enable == false {
		options.Flags = gen.DefaultNetworkFlags
	}
	n.flags = options.Flags
	n.softwareKeepAliveMisses = options.SoftwareKeepAliveMisses
	if options.FragmentSize > 0 && options.FragmentSize < 4096 {
		return fmt.Errorf("network option FragmentSize (%d) is too small, minimum is 4096 bytes", options.FragmentSize)
	}
	n.fragmentSize = options.FragmentSize
	n.fragmentTimeout = options.FragmentTimeout
	n.maxFragmentAssemblies = options.MaxFragmentAssemblies

	// register our own name so PIDs/refs that carry it encode as a compact atom-cache id
	n.RegisterAtom(n.node.name)

	if options.Mode == gen.NetworkModeHidden {
		static, err := n.registrar.Register(n.node, gen.RegisterRoutes{})
		if err != nil {
			return err
		}

		// add static routes
		for match, route := range static.Routes {
			if err := n.AddRoute(match, route, 0); err != nil {
				n.node.log.Error("unable to add static route %q from the registrar, ignored", match)
			}
		}
		// add static proxy routes
		for match, route := range static.Proxies {
			if err := n.AddProxyRoute(match, route, 0); err != nil {
				n.node.log.Error("unable to add static proxy route %q from the registrar, ignored", match)
			}
		}

		if lib.Verbose() {
			n.node.log.Trace("network started (hidden) with registrar %s", n.registrar.Version())
		}
		return nil
	}

	nodehost := strings.Split(string(n.node.name), "@")

	if len(options.Acceptors) == 0 {
		a := gen.AcceptorOptions{
			Host:           nodehost[1],
			Port:           gen.DefaultPort,
			CertManager:    n.node.CertManager(),
			Cookie:         options.Cookie,
			MaxMessageSize: options.MaxMessageSize,
			Flags:          options.Flags,
		}
		options.Acceptors = append(options.Acceptors, a)
	}

	if options.Handshake != nil {
		n.defaultHandshake = options.Handshake
	}
	if options.Proto != nil {
		n.defaultProto = options.Proto
	}

	appRoutes := []gen.ApplicationRoute{}
	for _, app := range n.node.Applications() {
		info, err := n.node.ApplicationInfo(app)
		if err != nil {
			continue
		}
		r := gen.ApplicationRoute{
			Node:   n.node.Name(),
			Name:   info.Name,
			Weight: info.Weight,
			Tags:   info.Tags,
			Mode:   info.Mode,
		}
		appRoutes = append(appRoutes, r)
	}
	routes := []gen.Route{}

	for _, a := range options.Acceptors {
		if a.Handshake == nil {
			a.Handshake = n.defaultHandshake
		}

		if a.Proto == nil {
			a.Proto = n.defaultProto
		}

		if a.MaxMessageSize == 0 {
			a.MaxMessageSize = options.MaxMessageSize
		}

		if a.Flags.Enable == false {
			a.Flags = a.Handshake.NetworkFlags()
			if a.Flags.Enable == false {
				a.Flags = options.Flags
			}
		}

		switch a.TCP {
		case "tcp":
		case "tcp6":
		default:
			a.TCP = "tcp4"
		}

		if a.Host == "" {
			a.Host = nodehost[1]
		}

		acceptor, err := n.startAcceptor(a)
		if err != nil {
			// stop acceptors
			for i := range n.acceptors {
				n.acceptors[i].l.Close()
			}
			return err
		}

		n.acceptors = append(n.acceptors, acceptor)

		// determine port to advertise in route
		routePort := acceptor.port
		if acceptor.route_port > 0 {
			routePort = acceptor.route_port
		}

		r := gen.Route{
			Host:             acceptor.route_host,
			Port:             routePort,
			TLS:              acceptor.cert_manager != nil,
			HandshakeVersion: acceptor.handshake.Version(),
			ProtoVersion:     acceptor.proto.Version(),
		}

		if a.Registrar == nil {
			acceptor.registrar_info = n.registrar.Info
			routes = append(routes, r)
			continue
		}

		acceptor.registrar_info = a.Registrar.Info
		// custom reistrar for this acceptor
		registerRoutes := gen.RegisterRoutes{
			Routes:            []gen.Route{r},
			ApplicationRoutes: appRoutes,
		}
		registrarInfo := a.Registrar.Info()

		// TODO it returns static routes. they need to be handled
		_, err = a.Registrar.Register(n.node, registerRoutes)
		if err != nil {
			// stop acceptors
			for i := range n.acceptors {
				n.acceptors[i].l.Close()
			}
			return fmt.Errorf(
				"unable to register node on %s (%s): %s",
				registrarInfo.Server,
				registrarInfo.Version,
				err,
			)
		}
		acceptor.registrar_custom = true
	}

	registerRoutes := gen.RegisterRoutes{
		Routes:            routes,
		ApplicationRoutes: appRoutes,
	}

	static, err := n.registrar.Register(n.node, registerRoutes)
	if err != nil {
		return fmt.Errorf("unable to register node: %s", err)
	}

	// add static routes
	for match, route := range static.Routes {
		if err := n.AddRoute(match, route, 0); err != nil {
			n.node.log.Error("unable to add static route %q from the registrar, ignored", match)
		}
	}
	// add static proxy routes
	for match, route := range static.Proxies {
		if err := n.AddProxyRoute(match, route, 0); err != nil {
			n.node.log.Error("unable to add static proxy route %q from the registrar, ignored", match)
		}
	}

	if lib.Verbose() {
		n.node.log.Trace("network started with registrar %s", n.registrar.Version())
	}
	return nil
}

func (n *network) startAcceptor(a gen.AcceptorOptions) (*acceptor, error) {
	lc := net.ListenConfig{
		KeepAlive: gen.DefaultKeepAlivePeriod,
	}

	cert_manager := a.CertManager
	if cert_manager == nil {
		cert_manager = n.node.CertManager()
	}
	bs := a.BufferSize
	if bs < 1 {
		bs = gen.DefaultTCPBufferSize
	}

	pstart := a.Port
	if pstart == 0 {
		pstart = gen.DefaultPort
	}
	pend := uint32(65535)
	if a.PortRange > 1 {
		p := uint32(pstart) + uint32(a.PortRange) - 1
		if p < 65535 {
			pend = p
		}
	} else if a.PortRange == 1 {
		pend = uint32(pstart)
	}

	acceptor := &acceptor{
		bs:                bs,
		proto:             a.Proto,
		handshake:         a.Handshake,
		cert_manager:      cert_manager,
		max_message_size:  a.MaxMessageSize,
		atom_mapping:      make(map[gen.Atom]gen.Atom),
		route_host:        a.RouteHost,
		route_port:        a.RoutePort,
		maxHandshakes:     int32(a.MaxHandshakes),
		handshake_timeout: a.HandshakeTimeout,

		software_keepalive_misses: n.keepAliveMisses(a.SoftwareKeepAliveMisses),
	}
	if a.Cookie == "" {
		acceptor.cookie = n.cookie
	}
	for k, v := range a.AtomMapping {
		acceptor.atom_mapping[k] = v
	}

	for i := uint32(pstart); i <= pend; i++ {
		hp := net.JoinHostPort(a.Host, strconv.Itoa(int(i)))
		lcl, err := lc.Listen(context.Background(), a.TCP, hp)
		if err != nil {
			if e, ok := err.(*net.OpError); ok {
				if _, ok := e.Err.(*net.DNSError); ok {
					return nil, err
				}
			}
			continue
		}

		acceptor.port = uint16(i)
		acceptor.l = lcl
		break
	}

	if acceptor.l == nil {
		return acceptor, fmt.Errorf("unable to assign requested address %s: no available ports in range %d..%d",
			a.Host, pstart, pend)
	}

	if acceptor.cert_manager != nil {
		config := &tls.Config{
			GetCertificate:     acceptor.cert_manager.GetCertificateFunc(),
			InsecureSkipVerify: a.InsecureSkipVerify,
			MinVersion:         tls.VersionTLS12,
		}

		// check for mTLS support
		if cam, ok := acceptor.cert_manager.(gen.CertAuthManager); ok {
			config.ClientAuth = cam.ClientAuth()
			config.ClientCAs = cam.ClientCAs()
		}

		acceptor.l = tls.NewListener(acceptor.l, config)
	}

	acceptor.flags = a.Flags
	if acceptor.flags.Enable == false {
		acceptor.flags = gen.DefaultNetworkFlags
	}

	go n.accept(acceptor)

	if lib.Verbose() {
		n.node.Log().Trace("started acceptor on %s with handshake %s and proto %s (TLS: %t)",
			acceptor.l.Addr(),
			acceptor.handshake.Version(),
			acceptor.proto.Version(), acceptor.cert_manager != nil,
		)
	}

	n.RegisterHandshake(acceptor.handshake)
	n.RegisterProto(acceptor.proto)

	return acceptor, nil
}

func (n *network) accept(a *acceptor) {
	cookie := a.cookie
	if cookie == "" {
		cookie = n.cookie
	}

	hopts := gen.HandshakeOptions{
		Cookie:         cookie,
		Flags:          a.flags,
		MaxMessageSize: a.max_message_size,
		CertManager:    a.cert_manager,
		CheckPending: func(peer gen.Atom) bool {
			_, exists := n.pending.Load(peer)
			return exists
		},
	}
	// period for keepalive is already in hopts.Flags (from a.flags)
	for {
		c, err := a.l.Accept()
		if err != nil {
			if err == io.EOF {
				return
			}
			n.node.Log().Info("acceptor %s terminated (handshake: %s, proto: %s)",
				a.l.Addr(), a.handshake.Version(), a.proto.Version())
			return
		}
		if lib.Verbose() {
			n.node.Log().Trace("accepted new TCP-connection from %s", c.RemoteAddr().String())
		}

		// check concurrency limit
		if a.maxHandshakes > 0 && a.handshaking.Add(1) > a.maxHandshakes {
			a.handshaking.Add(-1)
			c.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
			a.handshake.Reject(c, "busy")
			c.Close()
			continue
		}

		go func() {
			if a.maxHandshakes > 0 {
				defer a.handshaking.Add(-1)
			}
			n.handleAccepted(a, c, hopts)
		}()
	}
}

func (n *network) handleAccepted(a *acceptor, c net.Conn, hopts gen.HandshakeOptions) {
	c.SetDeadline(time.Now().Add(n.handshakeTimeout(a.handshake_timeout)))
	result, err := a.handshake.Negotiate(n.node, c, hopts)
	if err != nil {
		if err != io.EOF {
			n.node.Log().Warning("unable to handshake with %s: %s", c.RemoteAddr().String(), err)
		}
		a.handshakeErrors.Add(1)
		c.Close()
		return
	}

	if result.Peer == "" {
		n.node.Log().Warning("%s is not introduced itself, close connection", c.RemoteAddr().String())
		a.handshakeErrors.Add(1)
		c.Close()
		return
	}

	// update atom mapping: a.atom_mapping + result.AtomMapping
	mapping := make(map[gen.Atom]gen.Atom)
	for k, v := range a.atom_mapping {
		mapping[k] = v
	}
	for k, v := range result.AtomMapping {
		mapping[k] = v
	}
	result.AtomMapping = mapping

	// pool-join: Negotiate did the full short exchange. Attach this TCP to the
	// connection registered by ConnectionID. The owner registered it before
	// sending its introduce, so a pool-join (its initiator dials only after the
	// primary handshake finished) always finds it. No polling.
	if result.PeerCreation == 0 {
		v, ok := n.connectionsByID.Load(result.ConnectionID)
		if ok == false {
			c.Close()
			return
		}
		if err := v.(gen.Connection).Join(c, result.ConnectionID, nil, result.Tail); err != nil {
			c.Close()
		}
		return
	}

	// inject options from acceptor into ConnectionOptions
	if opts, ok := result.Custom.(handshake.ConnectionOptions); ok {
		opts.SoftwareKeepAliveMisses = a.software_keepalive_misses
		opts.FragmentSize = n.fragmentSize
		opts.FragmentTimeout = n.fragmentTimeout
		opts.MaxFragmentAssemblies = n.maxFragmentAssemblies
		result.Custom = opts
	}

	log := createLog(n.node.Log().Level(), n.node.dolog)
	logSource := gen.MessageLogNetwork{
		Node:     n.node.name,
		Peer:     result.Peer,
		Creation: result.PeerCreation,
	}
	log.setSource(logSource)
	conn, err := a.proto.NewConnection(n.node.core, result, log)
	if err != nil {
		n.node.Log().Warning("unable to create new connection: %s", err)
		c.Close()
		return
	}

	// single primary: keep one connection per ConnectionID. The incoming TCP is dropped
	// only when a connection already exists and the local node is canonical (so the
	// incoming is the reverse, losing direction); otherwise it establishes (so a
	// single-direction connect works whatever the initiator's name). Register by
	// ConnectionID BEFORE sending our introduce (Accept): the peer fills its pool only
	// after its connect completes (after it reads our introduce), so registering first
	// guarantees its pool-join TCPs find this connection instead of racing registration,
	// being dropped when the ConnectionID lookup misses, and redialing. Merge decision
	// serialized via mergeMu.
	localIsCanonical := n.node.name < result.Peer
	n.mergeMu.Lock()
	owner, loaded := n.connectionsByID.LoadOrStore(result.ConnectionID, conn)
	if loaded && localIsCanonical {
		// a connection already exists and we are canonical: the incoming is the reverse,
		// losing direction. Finish the peer's handshake so its connect returns promptly
		// and adopts the owner, then drop it.
		n.mergeMu.Unlock()
		if _, err := a.handshake.Accept(n.node, c, hopts, result); err != nil {
			if err != io.EOF {
				n.node.Log().Warning("unable to finish handshake with %s: %s", result.Peer, err)
			}
			a.handshakeErrors.Add(1)
		}
		conn.Terminate(nil)
		c.Close()
		return
	}
	if loaded {
		// we are not canonical: the incoming is the canonical winner, but a provisional
		// registered first. Take over its slot.
		n.connectionsByID.Store(result.ConnectionID, conn)
	}
	n.mergeMu.Unlock()

	// on failure restore the registry: put the provisional back (take-over) or remove
	// our entry (fresh establish).
	result, err = a.handshake.Accept(n.node, c, hopts, result)
	if err != nil {
		if err != io.EOF {
			n.node.Log().Warning("unable to finish handshake with %s: %s", result.Peer, err)
		}
		a.handshakeErrors.Add(1)
		n.mergeMu.Lock()
		if loaded {
			n.connectionsByID.CompareAndSwap(result.ConnectionID, conn, owner)
		} else {
			n.connectionsByID.CompareAndDelete(result.ConnectionID, conn)
		}
		n.mergeMu.Unlock()
		conn.Terminate(nil)
		c.Close()
		return
	}
	if jerr := conn.Join(c, result.ConnectionID, nil, result.Tail); jerr != nil {
		n.mergeMu.Lock()
		if loaded {
			n.connectionsByID.CompareAndSwap(result.ConnectionID, conn, owner)
		} else {
			n.connectionsByID.CompareAndDelete(result.ConnectionID, conn)
		}
		n.mergeMu.Unlock()
		conn.Terminate(nil)
		c.Close()
		return
	}

	// primary attached: announce by name only while we still own this ConnectionID. A
	// canonical-direction connect can take the slot over during the Accept round-trip above;
	// if it did, this direction lost the merge, so drop it rather than route the peer name to
	// (and serve) a superseded connection that no pool-join will ever fill.
	n.mergeMu.Lock()
	if cur, ok := n.connectionsByID.Load(result.ConnectionID); ok == false || cur != conn {
		n.mergeMu.Unlock()
		conn.Terminate(nil)
		return
	}
	n.registerConnection(result.Peer, conn)
	n.mergeMu.Unlock()
	if loaded {
		owner.(gen.Connection).Terminate(nil)
	}
	go n.serve(a.proto, conn, nil, result.ConnectionID)

	// the dialer reached our listener and is the pool filler. When we are canonical the
	// dialer is non-canonical and waits for our go-ahead before filling. Send it only when
	// we are not ourselves dialing the peer: a concurrent canonical-direction connect would
	// supersede this one, and the dialer must not fill a connection that is about to die.
	if localIsCanonical {
		if _, dialing := n.pending.Load(result.Peer); dialing == false {
			if ext, ok := conn.(interface{ Extend() }); ok {
				ext.Extend()
			}
		}
	}
}

// registerConnection sets the routing index by peer name. Dedup is handled by
// connectionsByID; this just points routing at the owner (overwrites a stale
// entry from a previous incarnation).
func (n *network) registerConnection(name gen.Atom, conn gen.Connection) {
	n.connections.Store(name, conn)
	n.connectionsEstablished.Add(1)
	n.node.log.Info("new connection with %s (%s)", name, name.CRC32())
	n.node.RouteNodeUp(name)
}

func (n *network) unregisterConnection(name gen.Atom, conn gen.Connection, connID string, reason error) {
	n.mergeMu.Lock()
	n.connectionsByID.CompareAndDelete(connID, conn) // keep a winner that took over this connID
	routed := n.connections.CompareAndDelete(name, conn)
	n.mergeMu.Unlock()
	n.connectionsLost.Add(1)
	if reason != nil {
		n.node.log.Info("connection with %s (%s) terminated with reason: %s", name, name.CRC32(), reason)
	} else {
		n.node.log.Info("connection with %s (%s) terminated", name, name.CRC32())
	}
	// only signal node-down if this connection still owned the routing entry. A newer
	// incarnation or a simultaneous-connect takeover that already re-registered the name
	// keeps its RouteNodeUp; a stale connection must not tear down the live one's
	// monitors/links by name.
	if routed {
		n.node.RouteNodeDown(name, reason)
	}
}
