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

	cookie         string
	maxmessagesize int

	staticRoutes  *staticRoutes
	staticProxies *staticProxies

	enableSpawn    sync.Map
	enableAppStart sync.Map

	connections sync.Map // gen.Atom (peer name) => gen.Connection
}

func (n *network) Registrar() (gen.Registrar, error) {
	if n.running.Load() == false {
		return nil, gen.ErrNetworkStopped
	}
	return n.registrar, nil
}

func (n *network) Cookie() string {
	return n.cookie
}
func (n *network) SetCookie(cookie string) error {
	n.cookie = cookie
	if lib.Trace() {
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
	if lib.Trace() {
		n.node.Log().Trace("added static route %s with weight %d", match, weight)
	}
	return nil
}

func (n *network) RemoveRoute(match string) error {
	if err := n.staticRoutes.remove(match); err != nil {
		return err
	}
	if lib.Trace() {
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

	if lib.Trace() {
		n.node.Log().Trace("added static proxy route %s with weight %d", match, weight)
	}
	return nil
}

func (n *network) RemoveProxyRoute(match string) error {
	if err := n.staticProxies.remove(match); err != nil {
		return err
	}
	if lib.Trace() {
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
		// allow any node to spawn this process (make nodes map empty)
		enable.nodes = make(map[gen.Atom]bool)
	} else {
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
	allowed := true
	enable.RLock()
	if len(enable.nodes) > 0 {
		allowed = enable.nodes[source]
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
		// allow any node to start this app (make nodes map empty)
		enable.nodes = make(map[gen.Atom]bool)
	} else {
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
	allowed := true
	enable.RLock()
	if len(enable.nodes) > 0 {
		allowed = enable.nodes[source]
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
		delete(enable.nodes, nn)
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
		if lib.Trace() {
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
		if lib.Trace() {
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

	info.EnabledSpawn = n.listEnabledSpawn()
	info.EnabledApplicationStart = n.listEnabledApplicationStart()

	return info, nil
}

func (n *network) Mode() gen.NetworkMode {
	return n.mode
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

	if lib.Trace() {
		n.node.Log().Trace("trying to make connection with %s", name)
	}
	// check the static routes
	if sroutes, found := n.staticRoutes.lookup(string(name)); found {
		if lib.Trace() {
			n.node.Log().Trace("found %s static route[s] for %s", len(sroutes), name)
		}
		for i, sroute := range sroutes {
			sroute.InsecureSkipVerify = n.skipverify
			if sroute.Resolver == nil {
				if lib.Trace() {
					n.node.Log().Trace("use static route to %s (%d)", name, i+1)
				}
				if c, err := n.connect(name, sroute); err == nil {
					return c, nil
				} else {
					if lib.Trace() {
						n.node.Log().Trace("unable to connect to %s using static route: %s", name, err)
					}
				}
				continue
			}

			if lib.Trace() {
				n.node.Log().Trace("use static route to %s with resolver (%d)", name, i+1)
			}
			nr, err := sroute.Resolver.Resolve(name)
			if err != nil {
				if lib.Trace() {
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
					if lib.Trace() {
						n.node.Log().Trace("unable to connect to %s using static route (with resolver): %s", name, err)
					}
				}
			}
		}
		return nil, gen.ErrNoRoute
	}

	// check the static proxy routes
	if proutes, found := n.staticProxies.lookup(string(name)); found {
		if lib.Trace() {
			n.node.Log().Trace("found %d static proxy route[s] for %s", len(proutes), name)
		}
		for i, proute := range proutes {
			if proute.Resolver == nil {
				if lib.Trace() {
					n.node.Log().Trace("use static proxy route to %s (%d)", name, i+1)
				}
				if c, err := n.connectProxy(name, proute); err == nil {
					return c, nil
				}
				continue
			}

			if lib.Trace() {
				n.node.Log().Trace("use static proxy route to %s with resolver (%d)", name, i+1)
			}
			pr, err := proute.Resolver.ResolveProxy(name)
			if err != nil {
				if lib.Trace() {
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
					if lib.Trace() {
						n.node.Log().Trace("unable to connect to %s using proxy route: %s", name, err)
					}
				}
			}
		}
		return nil, gen.ErrNoRoute
	}

	// resolve it
	if nr, err := n.registrar.Resolver().Resolve(name); err == nil {
		if lib.Trace() {
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
				if lib.Trace() {
					n.node.Log().Trace("unable to connect to %s: %s", name, err)
				}
			}
		}
		if lib.Trace() {
			n.node.Log().Trace("unable to connect to %s directly, looking up proxies...", name)
		}
	}

	// resolve proxy
	if pr, err := n.registrar.Resolver().ResolveProxy(name); err == nil {
		if lib.Trace() {
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
				if lib.Trace() {
					n.node.Log().Trace("unable to connect to %s using resolve proxy: %s", name, err)
				}
			}
		}
	}

	return nil, gen.ErrNoRoute
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

	handshake := vhandshake.(gen.NetworkHandshake)
	proto := vproto.(gen.NetworkProto)

	if lib.Trace() {
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

	hopts := gen.HandshakeOptions{
		Cookie:         route.Cookie,
		Flags:          route.Flags,
		MaxMessageSize: n.maxmessagesize,
	}

	if hopts.Cookie == "" {
		hopts.Cookie = n.cookie
	}
	if hopts.Flags.Enable == false {
		hopts.Flags = n.flags
	}

	result, err := handshake.Start(n.node, conn, hopts)
	if err != nil {
		conn.Close()
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

	pconn, err := proto.NewConnection(n.node, result, log)
	if err != nil {
		conn.Close()
		return nil, err
	}

	redial := func(dsn, id string) (net.Conn, []byte, error) {
		c, err := dial("tcp", dsn)
		if err != nil {
			return nil, nil, err
		}
		tail, err := handshake.Join(n.node, c, id, hopts)
		if err != nil {
			return nil, nil, err
		}
		return c, tail, nil
	}

	if c, err := n.registerConnection(result.Peer, pconn); err != nil {
		if err == gen.ErrTaken {
			return c, nil
		}
		pconn.Terminate(err)
		conn.Close()
		return nil, err
	}

	pconn.Join(conn, result.ConnectionID, redial, result.Tail)
	go n.serve(proto, pconn, redial)

	return pconn, nil
}

func (n *network) serve(proto gen.NetworkProto, conn gen.Connection, redial gen.NetworkDial) {
	name := conn.Node().Name()
	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				n.node.log.Panic("connection with %s (%s) terminated abnormally: %v", name, name.CRC32(), r)
				n.unregisterConnection(name, gen.TerminateReasonPanic)
				conn.Terminate(gen.TerminateReasonPanic)
			}
		}()
	}

	err := proto.Serve(conn, redial)
	n.unregisterConnection(name, err)
	conn.Terminate(err)
}

func (n *network) connectProxy(name gen.Atom, route gen.NetworkProxyRoute) (gen.Connection, error) {
	if lib.Trace() {
		n.node.Log().Trace("trying to connect to %s (via proxy %s)", name, route.Route.Proxy)
	}
	// TODO will be implemented later
	n.node.log.Warning("proxy feature is not implemented yet")

	return nil, gen.ErrUnsupported
}

func (n *network) stop() error {
	if swapped := n.running.CompareAndSwap(true, false); swapped == false {
		return fmt.Errorf("network stack is already stopped")
	}

	n.registrar.Terminate()
	n.registrar = nil

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

	if lib.Trace() {
		n.node.log.Trace("starting network...")
	}

	n.skipverify = options.InsecureSkipVerify
	n.registrar = options.Registrar
	if n.registrar == nil {
		n.registrar = registrar.Create(registrar.Options{})
	}

	n.node.validateLicenses(n.registrar.Version())

	if options.Cookie == "" {
		n.node.log.Warning("cookie is empty (gen.NetworkOptions), used randomized value")
		options.Cookie = lib.RandomString(16)
	}
	n.cookie = options.Cookie
	n.maxmessagesize = options.MaxMessageSize

	if options.Flags.Enable == false {
		options.Flags = gen.DefaultNetworkFlags
	}
	n.flags = options.Flags

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

		if lib.Trace() {
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
		r := gen.Route{
			Port:             acceptor.port,
			TLS:              acceptor.tls,
			HandshakeVersion: acceptor.handshake.Version(),
			ProtoVersion:     acceptor.proto.Version(),
		}
		n.node.validateLicenses(r.HandshakeVersion, r.ProtoVersion)
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
			return fmt.Errorf("unable to register node on %s (%s): %s", registrarInfo.Server, registrarInfo.Version, err)
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

	if lib.Trace() {
		n.node.log.Trace("network started with registrar %s", n.registrar.Version())
	}
	return nil
}

func (n *network) startAcceptor(a gen.AcceptorOptions) (*acceptor, error) {
	lc := net.ListenConfig{
		KeepAlive: gen.DefaultKeepAlivePeriod,
	}

	cert := a.CertManager
	if cert == nil {
		cert = n.node.CertManager()
	}
	bs := a.BufferSize
	if bs < 1 {
		bs = gen.DefaultTCPBufferSize
	}

	pstart := a.Port
	if pstart == 0 {
		pstart = gen.DefaultPort
	}
	pend := a.PortRange
	if pend == 0 {
		pend = 50000
	}
	if pend < pstart {
		pend = pstart
	}

	acceptor := &acceptor{
		bs:               bs,
		proto:            a.Proto,
		handshake:        a.Handshake,
		tls:              cert != nil,
		max_message_size: a.MaxMessageSize,
		atom_mapping:     make(map[gen.Atom]gen.Atom),
	}
	if a.Cookie == "" {
		acceptor.cookie = n.cookie
	}
	for k, v := range a.AtomMapping {
		acceptor.atom_mapping[k] = v
	}

	for i := pstart; i < pend+1; i++ {
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

		acceptor.port = i
		acceptor.l = lcl
		break
	}

	if acceptor.l == nil {
		return acceptor, fmt.Errorf("unable to assign requested address %s: no available ports in range %d..%d",
			a.Host, pstart, pend)
	}

	if cert != nil {
		config := &tls.Config{
			GetCertificate:     cert.GetCertificateFunc(),
			InsecureSkipVerify: a.InsecureSkipVerify,
		}
		acceptor.l = tls.NewListener(acceptor.l, config)
	}

	acceptor.flags = a.Flags
	if acceptor.flags.Enable == false {
		acceptor.flags = gen.DefaultNetworkFlags
	}

	go n.accept(acceptor)

	if lib.Trace() {
		n.node.Log().Trace("started acceptor on %s with handshake %s and proto %s (TLS: %t)",
			acceptor.l.Addr(),
			acceptor.handshake.Version(),
			acceptor.proto.Version(), cert != nil,
		)
	}

	n.RegisterHandshake(acceptor.handshake)
	n.RegisterProto(acceptor.proto)

	return acceptor, nil
}

func (n *network) accept(a *acceptor) {
	hopts := gen.HandshakeOptions{
		Cookie:         a.cookie,
		Flags:          a.flags,
		MaxMessageSize: a.max_message_size,
	}
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
		if lib.Trace() {
			n.node.Log().Trace("accepted new TCP-connection from %s", c.RemoteAddr().String())
		}

		if hopts.Cookie == "" {
			hopts.Cookie = n.cookie
		}

		result, err := a.handshake.Accept(n.node, c, hopts)
		if err != nil {
			if err != io.EOF {
				n.node.Log().Warning("unable to handshake with %s: %s", c.RemoteAddr().String(), err)
			}
			c.Close()
			continue
		}

		if result.Peer == "" {
			n.node.Log().Warning("%s is not introduced itself, close connection", c.RemoteAddr().String())
			c.Close()
			continue
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

		// check if we already have connection with this node
		if v, exist := n.connections.Load(result.Peer); exist {
			conn := v.(gen.Connection)
			if err := conn.Join(c, result.ConnectionID, nil, result.Tail); err != nil {
				if err == gen.ErrUnsupported {
					n.node.Log().Warning("unable to accept connection with %s (join is not supported)",
						result.Peer)
				} else {
					n.node.Log().Trace("unable to join %s to the existing connection with %s: %s",
						c.RemoteAddr(), result.Peer, err)
				}
				c.Close()
			}
			continue
		}

		log := createLog(n.node.Log().Level(), n.node.dolog)
		logSource := gen.MessageLogNetwork{
			Node:     n.node.name,
			Peer:     result.Peer,
			Creation: result.PeerCreation,
		}
		log.setSource(logSource)
		conn, err := a.proto.NewConnection(n.node, result, log)
		if err != nil {
			n.node.Log().Warning("unable to create new connection: %s", err)
			c.Close()
			continue
		}

		if _, err := n.registerConnection(result.Peer, conn); err != nil {
			n.node.Log().Warning("unable to register new connection with %s: %s", result.Peer, err)
			c.Close()
			continue
		}
		conn.Join(c, result.ConnectionID, nil, result.Tail)
		go n.serve(a.proto, conn, nil)
	}
}

func (n *network) registerConnection(name gen.Atom, conn gen.Connection) (gen.Connection, error) {
	if v, exist := n.connections.LoadOrStore(name, conn); exist {
		return v.(gen.Connection), gen.ErrTaken
	}
	n.node.log.Info("new connection with %s (%s)", name, name.CRC32())
	// TODO create event gen.MessageNetworkEvent
	return conn, nil
}

func (n *network) unregisterConnection(name gen.Atom, reason error) {
	n.connections.Delete(name)
	if reason != nil {
		n.node.log.Info("connection with %s (%s) terminated with reason: %s", name, name.CRC32(), reason)
	} else {
		n.node.log.Info("connection with %s (%s) terminated", name, name.CRC32())
	}
	n.node.RouteNodeDown(name, reason)
	// TODO create event gen.MessageNetworkEvent
}
