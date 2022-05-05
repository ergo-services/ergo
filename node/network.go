package node

import (
	"bytes"
	"context"
	"encoding/pem"
	"io"
	"math/big"
	"sync"
	"time"

	"crypto/aes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"

	"net"

	"strconv"
	"strings"
)

type networkInternal interface {
	// add/remove static route
	AddStaticRoute(node string, host string, port uint16, options RouteOptions) error
	AddStaticRoutePort(node string, port uint16, options RouteOptions) error
	AddStaticRouteOptions(node string, options RouteOptions) error
	RemoveStaticRoute(node string) bool
	StaticRoutes() []Route
	StaticRoute(name string) (Route, bool)

	// add/remove proxy route
	AddProxyRoute(node string, route ProxyRoute) error
	RemoveProxyRoute(node string) bool
	ProxyRoutes() []ProxyRoute
	ProxyRoute(name string) (ProxyRoute, bool)

	Resolve(peername string) (Route, error)
	Connect(peername string) error
	Disconnect(peername string) error
	Nodes() []string
	NodesIndirect() []string

	// core router methods
	RouteProxyConnectRequest(from ConnectionInterface, request ProxyConnectRequest) error
	RouteProxyConnectReply(from ConnectionInterface, reply ProxyConnectReply) error
	RouteProxyConnectCancel(from ConnectionInterface, cancel ProxyConnectCancel) error
	RouteProxyDisconnect(from ConnectionInterface, disconnect ProxyDisconnect) error
	RouteProxy(from ConnectionInterface, sessionID string, packet *lib.Buffer) error

	getConnection(peername string) (ConnectionInterface, error)
	stopNetwork()
}

type connectionInternal struct {
	// conn. has nil value for the proxy connection
	conn net.Conn
	// connection interface of the network connection
	connection ConnectionInterface
	//
	proxySessionID string
}

type network struct {
	nodename string
	cookie   string
	ctx      context.Context
	listener net.Listener

	resolver          Resolver
	staticOnly        bool
	staticRoutes      map[string]Route
	staticRoutesMutex sync.RWMutex

	proxyRoutes      map[string]ProxyRoute
	proxyRoutesMutex sync.RWMutex

	connections        map[string]connectionInternal
	connectionsProxy   map[ConnectionInterface][]string // peers via proxy
	connectionsTransit map[ConnectionInterface][]string // transit session IDs
	connectionsMutex   sync.RWMutex

	proxyTransitSessions      map[string]proxyTransitSession
	proxyTransitSessionsMutex sync.RWMutex

	proxyConnectRequest      map[etf.Ref]proxyConnectRequest
	proxyConnectRequestMutex sync.RWMutex

	tls      TLS
	proxy    Proxy
	version  Version
	creation uint32
	flags    Flags

	router    coreRouterInternal
	handshake HandshakeInterface
	proto     ProtoInterface

	remoteSpawn      map[string]gen.ProcessBehavior
	remoteSpawnMutex sync.Mutex
}

func newNetwork(ctx context.Context, nodename string, cookie string, options Options, router coreRouterInternal) (networkInternal, error) {
	n := &network{
		nodename:             nodename,
		cookie:               cookie,
		ctx:                  ctx,
		staticOnly:           options.StaticRoutesOnly,
		staticRoutes:         make(map[string]Route),
		proxyRoutes:          make(map[string]ProxyRoute),
		connections:          make(map[string]connectionInternal),
		connectionsProxy:     make(map[ConnectionInterface][]string),
		connectionsTransit:   make(map[ConnectionInterface][]string),
		proxyTransitSessions: make(map[string]proxyTransitSession),
		proxyConnectRequest:  make(map[etf.Ref]proxyConnectRequest),
		remoteSpawn:          make(map[string]gen.ProcessBehavior),
		flags:                options.Flags,
		proxy:                options.Proxy,
		resolver:             options.Resolver,
		handshake:            options.Handshake,
		proto:                options.Proto,
		router:               router,
		creation:             options.Creation,
	}

	nn := strings.Split(nodename, "@")
	if len(nn) != 2 {
		return nil, fmt.Errorf("(EMPD) FQDN for node name is required (example: node@hostname)")
	}

	n.version, _ = options.Env[EnvKeyVersion].(Version)
	if n.proxy.Flags.Enable == false {
		n.proxy.Flags = DefaultProxyFlags()
	}

	n.tls = options.TLS
	selfSignedCert, err := generateSelfSignedCert(n.version)
	if n.tls.Server.Certificate == nil {
		n.tls.Server = selfSignedCert
		n.tls.SkipVerify = true
	}
	if n.tls.Client.Certificate == nil {
		n.tls.Client = selfSignedCert
	}

	err = n.handshake.Init(n.nodename, n.creation, n.flags)
	if err != nil {
		return nil, err
	}

	port, err := n.listen(ctx, nn[1], options.ListenBegin, options.ListenEnd)
	if err != nil {
		return nil, err
	}

	resolverOptions := ResolverOptions{
		NodeVersion:       n.version,
		HandshakeVersion:  n.handshake.Version(),
		EnableTLS:         n.tls.Enable,
		EnableProxy:       options.Flags.EnableProxy,
		EnableCompression: options.Flags.EnableCompression,
	}
	if err := n.resolver.Register(n.ctx, nodename, port, resolverOptions); err != nil {
		return nil, err
	}

	return n, nil
}

func (n *network) stopNetwork() {
	if n.listener != nil {
		n.listener.Close()
	}
	n.connectionsMutex.RLock()
	defer n.connectionsMutex.RUnlock()
	for _, ci := range n.connections {
		if ci.conn == nil {
			continue
		}
		ci.conn.Close()
	}
}

// AddStaticRouteOptions adds static options for the given node.
func (n *network) AddStaticRouteOptions(node string, options RouteOptions) error {
	if n.staticOnly {
		return fmt.Errorf("can't be used if enabled StaticRoutesOnly")
	}
	return n.AddStaticRoute(node, "", 0, options)
}

// AddStaticRoutePort adds a static route to the node with the given name
func (n *network) AddStaticRoutePort(node string, port uint16, options RouteOptions) error {
	ns := strings.Split(node, "@")
	if port < 1 {
		return fmt.Errorf("port must be greater 0")
	}
	if len(ns) != 2 {
		return fmt.Errorf("wrong FQDN")
	}
	return n.AddStaticRoute(node, ns[1], port, options)

}

// AddStaticRoute adds a static route to the node with the given name
func (n *network) AddStaticRoute(node string, host string, port uint16, options RouteOptions) error {
	if len(strings.Split(node, "@")) != 2 {
		return fmt.Errorf("wrong FQDN")
	}

	if port > 0 {
		if _, err := net.LookupHost(host); err != nil {
			return err
		}
	}

	route := Route{
		Node:    node,
		Host:    host,
		Port:    port,
		Options: options,
	}

	n.staticRoutesMutex.Lock()
	defer n.staticRoutesMutex.Unlock()

	_, exist := n.staticRoutes[node]
	if exist {
		return ErrTaken
	}

	if options.Handshake != nil {
		if err := options.Handshake.Init(n.nodename, n.creation, n.flags); err != nil {
			return err
		}
	}
	n.staticRoutes[node] = route

	return nil
}

// RemoveStaticRoute removes static route record. Returns false if it doesn't exist.
func (n *network) RemoveStaticRoute(node string) bool {
	n.staticRoutesMutex.Lock()
	defer n.staticRoutesMutex.Unlock()
	_, exist := n.staticRoutes[node]
	if exist {
		delete(n.staticRoutes, node)
		return true
	}
	return false
}

// StaticRoutes returns list of static routes added with AddStaticRoute
func (n *network) StaticRoutes() []Route {
	var routes []Route

	n.staticRoutesMutex.RLock()
	defer n.staticRoutesMutex.RUnlock()
	for _, v := range n.staticRoutes {
		routes = append(routes, v)
	}

	return routes
}

func (n *network) StaticRoute(name string) (Route, bool) {
	n.staticRoutesMutex.RLock()
	defer n.staticRoutesMutex.RUnlock()
	route, exist := n.staticRoutes[name]
	return route, exist
}

func (n *network) getConnectionDirect(peername string, connect bool) (ConnectionInterface, error) {
	n.connectionsMutex.RLock()
	ci, ok := n.connections[peername]
	n.connectionsMutex.RUnlock()
	if ok {
		return ci.connection, nil
	}

	if connect == false {
		return nil, ErrNoRoute
	}

	connection, err := n.connect(peername)
	if err != nil {
		lib.Log("[%s] CORE no route to node %q: %s", n.nodename, peername, err)
		return nil, ErrNoRoute
	}
	return connection, nil

}

// getConnection
func (n *network) getConnection(peername string) (ConnectionInterface, error) {
	if peername == n.nodename {
		// can't connect to itself
		return nil, ErrNoRoute
	}
	n.connectionsMutex.RLock()
	ci, ok := n.connections[peername]
	n.connectionsMutex.RUnlock()
	if ok {
		return ci.connection, nil
	}

	// try to connect via proxy if there ProxyRoute was presented for this peer
	request := ProxyConnectRequest{
		ID:       n.router.MakeRef(),
		To:       peername,
		Creation: n.creation,
	}

	if err := n.RouteProxyConnectRequest(nil, request); err != nil {
		if err != ErrProxyNoRoute {
			return nil, err
		}

		// there wasn't proxy presented. try to connect directly.
		connection, err := n.getConnectionDirect(peername, true)
		return connection, err
	}

	connection, err := n.waitProxyConnection(request.ID, 5)
	if err != nil {
		return nil, err
	}

	return connection, nil
}

// Resolve
func (n *network) Resolve(node string) (Route, error) {
	n.staticRoutesMutex.Lock()
	defer n.staticRoutesMutex.Unlock()

	if r, ok := n.staticRoutes[node]; ok {
		if r.Port == 0 {
			// use static option for this route
			route, err := n.resolver.Resolve(node)
			route.Options = r.Options
			return route, err
		}
		return r, nil
	}

	if n.staticOnly {
		return Route{}, ErrNoRoute
	}

	return n.resolver.Resolve(node)
}

// Connect
func (n *network) Connect(node string) error {
	_, err := n.getConnection(node)
	return err
}

// Disconnect
func (n *network) Disconnect(node string) error {
	n.connectionsMutex.RLock()
	ci, ok := n.connections[node]
	n.connectionsMutex.RUnlock()
	if !ok {
		return ErrNoRoute
	}

	if ci.conn == nil {
		// this is proxy connection
		disconnect := ProxyDisconnect{
			Node:      n.nodename,
			Proxy:     n.nodename,
			SessionID: ci.proxySessionID,
			Reason:    "normal",
		}
		n.unregisterConnection(node, &disconnect)
		return ci.connection.ProxyDisconnect(disconnect)
	}

	ci.conn.Close()
	return nil
}

// Nodes
func (n *network) Nodes() []string {
	list := []string{}
	n.connectionsMutex.RLock()
	defer n.connectionsMutex.RUnlock()

	for node := range n.connections {
		list = append(list, node)
	}
	return list
}

func (n *network) NodesIndirect() []string {
	list := []string{}
	n.connectionsMutex.RLock()
	defer n.connectionsMutex.RUnlock()

	for node, ci := range n.connections {
		if ci.conn == nil {
			list = append(list, node)
		}
	}
	return list

}

// RouteProxyConnectRequest
func (n *network) RouteProxyConnectRequest(from ConnectionInterface, request ProxyConnectRequest) error {
	// check if we have proxy route
	n.proxyRoutesMutex.RLock()
	route, has_route := n.proxyRoutes[request.To]
	n.proxyRoutesMutex.RUnlock()

	if request.To != n.nodename {
		var connection ConnectionInterface
		var err error

		if from != nil {
			//
			// transit request
			//

			lib.Log("[%s] NETWORK transit proxy connection to %q via %q", n.nodename, request.To, route.Proxy)
			// proxy feature must be enabled explicitly for the transitional requests
			if n.proxy.Transit == false {
				lib.Log("[%s] NETWORK proxy. Proxy feature is disabled on this node", n.nodename)
				return ErrProxyTransitDisabled
			}
			if request.Hop < 1 {
				lib.Log("[%s] NETWORK proxy. Error: exceeded hop limit", n.nodename)
				return ErrProxyHopExceeded
			}
			request.Hop--

			if len(request.Path) > defaultProxyPathLimit {
				return ErrProxyPathTooLong
			}

			for i := range request.Path {
				if n.nodename != request.Path[i] {
					continue
				}
				lib.Log("[%s] NETWORK proxy. Error: loop detected in proxy path %#v", n.nodename, request.Path)
				return ErrProxyLoopDetected
			}

			// try to connect to the next-hop node
			if has_route == false {
				connection, err = n.getConnectionDirect(request.To, true)
			} else {
				connection, err = n.getConnectionDirect(route.Proxy, true)
			}

			if err != nil {
				return err
			}

			if from == connection {
				lib.Log("[%s] NETWORK proxy. Error: proxy route points to the connection this request came from", n.nodename)
				return ErrProxyLoopDetected
			}
			request.Path = append([]string{n.nodename}, request.Path...)
			return connection.ProxyConnectRequest(request)
		}

		if has_route == false {
			// if it was invoked from getConnection ('from' == nil) there will
			// be attempt to make direct connection using getConnectionDirect
			return ErrProxyNoRoute
		}

		//
		// initiating proxy connection
		//
		lib.Log("[%s] NETWORK initiate proxy connection to %q via %q", n.nodename, request.To, route.Proxy)
		connection, err = n.getConnectionDirect(route.Proxy, true)
		if err != nil {
			return err
		}

		privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		pubKey := x509.MarshalPKCS1PublicKey(&privKey.PublicKey)
		request.PublicKey = pubKey

		// create digest using nodename, cookie, peername and pubKey
		request.Digest = generateProxyDigest(n.nodename, route.Cookie, request.To, pubKey)

		request.Flags = route.Flags
		if request.Flags.Enable == false {
			request.Flags = n.proxy.Flags
		}

		request.Hop = route.MaxHop
		if request.Hop < 1 {
			request.Hop = DefaultProxyMaxHop
		}
		request.Creation = n.creation
		connectRequest := proxyConnectRequest{
			privateKey: privKey,
			request:    request,
			connection: make(chan ConnectionInterface),
			cancel:     make(chan ProxyConnectCancel),
		}
		request.Path = []string{n.nodename}
		if err := connection.ProxyConnectRequest(request); err != nil {
			return err
		}
		n.putProxyConnectRequest(connectRequest)
		return nil
	}

	//
	// handle proxy connect request
	//

	// check digest
	// use the last item in the request.Path as a peername
	if len(request.Path) < 2 {
		// reply error. there must be atleast 2 nodes - initiating and transit nodes
		lib.Log("[%s] NETWORK proxy. Proxy connect request has wrong path (too short)", n.nodename)
		return ErrProxyConnect
	}
	peername := request.Path[len(request.Path)-1]
	cookie := n.proxy.Cookie
	flags := n.proxy.Flags
	if has_route {
		cookie = route.Cookie
		if route.Flags.Enable == true {
			flags = route.Flags
		}
	}
	checkDigest := generateProxyDigest(peername, cookie, n.nodename, request.PublicKey)
	if bytes.Equal(request.Digest, checkDigest) == false {
		// reply error. digest mismatch
		lib.Log("[%s] NETWORK proxy. Proxy connect request has wrong digest", n.nodename)
		return ErrProxyConnect
	}

	// do some encryption magic
	pk, err := x509.ParsePKCS1PublicKey(request.PublicKey)
	if err != nil {
		lib.Log("[%s] NETWORK proxy. Proxy connect request has wrong public key", n.nodename)
		return ErrProxyConnect
	}
	hash := sha256.New()
	key := make([]byte, 32)
	rand.Read(key)
	cipherkey, err := rsa.EncryptOAEP(hash, rand.Reader, pk, key, nil)
	if err != nil {
		lib.Log("[%s] NETWORK proxy. Proxy connect request. Can't encrypt: %s ", n.nodename, err)
		return ErrProxyConnect
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	sessionID := lib.RandomString(32)
	digest := generateProxyDigest(n.nodename, n.proxy.Cookie, peername, key)
	if flags.Enable == false {
		flags = DefaultProxyFlags()
	}

	// if one of the nodes want to use encryption then it must be used by both nodes
	if request.Flags.EnableEncryption || flags.EnableEncryption {
		request.Flags.EnableEncryption = true
		flags.EnableEncryption = true
	}

	cInternal := connectionInternal{
		connection:     from,
		proxySessionID: sessionID,
	}
	if _, err := n.registerConnection(peername, cInternal); err != nil {
		return ErrProxySessionDuplicate
	}

	reply := ProxyConnectReply{
		ID:        request.ID,
		To:        peername,
		Digest:    digest,
		Cipher:    cipherkey,
		Flags:     flags,
		Creation:  n.creation,
		SessionID: sessionID,
		Path:      request.Path[1:],
	}

	if err := from.ProxyConnectReply(reply); err != nil {
		// can't send reply. ignore this connection request
		lib.Log("[%s] NETWORK proxy. Proxy connect request. Can't send reply: %s ", n.nodename, err)
		n.unregisterConnection(peername, nil)
		return ErrProxyConnect
	}

	session := ProxySession{
		ID:        sessionID,
		NodeFlags: reply.Flags,
		PeerFlags: request.Flags,
		PeerName:  peername,
		Creation:  request.Creation,
		Block:     block,
	}

	// register proxy session
	from.ProxyRegisterSession(session)
	return nil
}

func (n *network) RouteProxyConnectReply(from ConnectionInterface, reply ProxyConnectReply) error {

	n.proxyTransitSessionsMutex.RLock()
	_, duplicate := n.proxyTransitSessions[reply.SessionID]
	n.proxyTransitSessionsMutex.RUnlock()

	if duplicate {
		return ErrProxySessionDuplicate
	}

	if from == nil {
		// from value can't be nil
		return ErrProxyUnknownRequest
	}

	if reply.To != n.nodename {
		// send this reply further and register this session
		if n.proxy.Transit == false {
			return ErrProxyTransitDisabled
		}

		if len(reply.Path) == 0 {
			return ErrProxyUnknownRequest
		}
		if len(reply.Path) > defaultProxyPathLimit {
			return ErrProxyPathTooLong
		}

		next := reply.Path[0]
		connection, err := n.getConnectionDirect(next, false)
		if err != nil {
			return err
		}
		if connection == from {
			return ErrProxyLoopDetected
		}

		reply.Path = reply.Path[1:]
		// check for the looping
		for i := range reply.Path {
			if reply.Path[i] == next {
				return ErrProxyLoopDetected
			}
		}

		if err := connection.ProxyConnectReply(reply); err != nil {
			return err
		}

		// register transit proxy session
		n.proxyTransitSessionsMutex.Lock()
		session := proxyTransitSession{
			a: from,
			b: connection,
		}
		n.proxyTransitSessions[reply.SessionID] = session
		n.proxyTransitSessionsMutex.Unlock()

		// keep session id for both connections in order
		// to handle connection closing (we should
		// send ProxyDisconnect if one of the connection
		// was closed)
		n.connectionsMutex.Lock()
		sessions, _ := n.connectionsTransit[session.a]
		sessions = append(sessions, reply.SessionID)
		n.connectionsTransit[session.a] = sessions
		sessions, _ = n.connectionsTransit[session.b]
		sessions = append(sessions, reply.SessionID)
		n.connectionsTransit[session.b] = sessions
		n.connectionsMutex.Unlock()
		return nil
	}

	// look up for the request we made earlier
	r, found := n.getProxyConnectRequest(reply.ID)
	if found == false {
		return ErrProxyUnknownRequest
	}

	// decrypt cipher key using private key
	hash := sha256.New()
	key, err := rsa.DecryptOAEP(hash, rand.Reader, r.privateKey, reply.Cipher, nil)
	if err != nil {
		lib.Log("[%s] CORE route proxy. Proxy connect reply has invalid cipher", n.nodename)
		return ErrProxyConnect
	}

	cookie := n.proxy.Cookie
	// check if we should use proxy route cookie
	n.proxyRoutesMutex.RLock()
	route, has_route := n.proxyRoutes[r.request.To]
	n.proxyRoutesMutex.RUnlock()
	if has_route {
		cookie = route.Cookie
	}
	// check digest
	checkDigest := generateProxyDigest(r.request.To, cookie, n.nodename, key)
	if bytes.Equal(checkDigest, reply.Digest) == false {
		lib.Log("[%s] CORE route proxy. Proxy connect reply has wrong digest", n.nodename)
		return ErrProxyConnect
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	cInternal := connectionInternal{
		connection:     from,
		proxySessionID: reply.SessionID,
	}
	if registered, err := n.registerConnection(r.request.To, cInternal); err != nil {
		select {
		case r.connection <- registered:
		}
		return ErrProxySessionDuplicate
	}
	// if one of the nodes want to use encryption then it must be used by both nodes
	if r.request.Flags.EnableEncryption || reply.Flags.EnableEncryption {
		r.request.Flags.EnableEncryption = true
		reply.Flags.EnableEncryption = true
	}

	session := ProxySession{
		ID:        reply.SessionID,
		NodeFlags: r.request.Flags,
		PeerFlags: reply.Flags,
		PeerName:  r.request.To,
		Creation:  reply.Creation,
		Block:     block,
	}

	// register proxy session
	from.ProxyRegisterSession(session)

	select {
	case r.connection <- from:
	}

	return nil
}

func (n *network) RouteProxyConnectCancel(from ConnectionInterface, cancel ProxyConnectCancel) error {
	if from == nil {
		// from value can not be nil
		return ErrProxyConnect
	}
	if len(cancel.Path) == 0 {
		n.cancelProxyConnectRequest(cancel)
		return nil
	}

	next := cancel.Path[0]
	if next != n.nodename {
		if len(cancel.Path) > defaultProxyPathLimit {
			return ErrProxyPathTooLong
		}
		connection, err := n.getConnectionDirect(next, false)
		if err != nil {
			return err
		}

		if connection == from {
			return ErrProxyLoopDetected
		}

		cancel.Path = cancel.Path[1:]
		// check for the looping
		for i := range cancel.Path {
			if cancel.Path[i] == next {
				return ErrProxyLoopDetected
			}
		}

		if err := connection.ProxyConnectCancel(cancel); err != nil {
			return err
		}
		return nil
	}

	return ErrProxyUnknownRequest
}

func (n *network) RouteProxyDisconnect(from ConnectionInterface, disconnect ProxyDisconnect) error {

	n.proxyTransitSessionsMutex.RLock()
	session, isTransitSession := n.proxyTransitSessions[disconnect.SessionID]
	n.proxyTransitSessionsMutex.RUnlock()
	if isTransitSession == false {
		// check for the proxy connection endpoint
		var peername string
		var found bool
		var ci connectionInternal

		// get peername by session id
		n.connectionsMutex.RLock()
		for p, c := range n.connections {
			if c.proxySessionID != disconnect.SessionID {
				continue
			}
			found = true
			peername = p
			ci = c
			break
		}
		if found == false {
			n.connectionsMutex.RUnlock()
			return ErrProxySessionUnknown
		}
		n.connectionsMutex.RUnlock()

		if ci.proxySessionID != disconnect.SessionID || ci.connection != from {
			return ErrProxySessionUnknown
		}

		n.unregisterConnection(peername, &disconnect)
		return nil
	}

	n.proxyTransitSessionsMutex.Lock()
	delete(n.proxyTransitSessions, disconnect.SessionID)
	n.proxyTransitSessionsMutex.Unlock()

	// remove this session from the connections
	n.connectionsMutex.Lock()
	sessions, ok := n.connectionsTransit[session.a]
	if ok {
		for i := range sessions {
			if sessions[i] == disconnect.SessionID {
				sessions[i] = sessions[0]
				sessions = sessions[1:]
				n.connectionsTransit[session.a] = sessions
				break
			}
		}
	}
	sessions, ok = n.connectionsTransit[session.b]
	if ok {
		for i := range sessions {
			if sessions[i] == disconnect.SessionID {
				sessions[i] = sessions[0]
				sessions = sessions[1:]
				n.connectionsTransit[session.b] = sessions
				break
			}
		}
	}
	n.connectionsMutex.Unlock()

	// send this message further
	switch from {
	case session.b:
		return session.a.ProxyDisconnect(disconnect)
	case session.a:
		return session.b.ProxyDisconnect(disconnect)
	default:
		// shouldn't happen
		panic("internal error")
	}
}

func (n *network) RouteProxy(from ConnectionInterface, sessionID string, packet *lib.Buffer) error {
	// check if this session is present on this node
	n.proxyTransitSessionsMutex.RLock()
	session, ok := n.proxyTransitSessions[sessionID]
	n.proxyTransitSessionsMutex.RUnlock()

	if !ok {
		return ErrProxySessionUnknown
	}

	switch from {
	case session.b:
		return session.a.ProxyPacket(packet)
	case session.a:
		return session.b.ProxyPacket(packet)
	default:
		// shouldn't happen
		panic("internal error")
	}
}

func (n *network) AddProxyRoute(node string, route ProxyRoute) error {
	n.proxyRoutesMutex.Lock()
	defer n.proxyRoutesMutex.Unlock()
	if route.MaxHop > defaultProxyPathLimit {
		return ErrProxyPathTooLong
	}
	if route.MaxHop < 1 {
		route.MaxHop = DefaultProxyMaxHop
	}

	if route.Flags.Enable == false {
		route.Flags = n.proxy.Flags
	}

	if _, exist := n.proxyRoutes[node]; exist {
		return ErrTaken
	}

	n.proxyRoutes[node] = route
	return nil
}

func (n *network) RemoveProxyRoute(node string) bool {
	n.proxyRoutesMutex.Lock()
	defer n.proxyRoutesMutex.Unlock()
	if _, exist := n.proxyRoutes[node]; exist == false {
		return false
	}
	delete(n.proxyRoutes, node)
	return true
}

func (n *network) ProxyRoutes() []ProxyRoute {
	var routes []ProxyRoute
	n.proxyRoutesMutex.RLock()
	defer n.proxyRoutesMutex.RUnlock()
	for _, v := range n.proxyRoutes {
		routes = append(routes, v)
	}
	return routes
}

func (n *network) ProxyRoute(name string) (ProxyRoute, bool) {
	n.proxyRoutesMutex.RLock()
	defer n.proxyRoutesMutex.RUnlock()
	route, exist := n.proxyRoutes[name]
	return route, exist
}

func (n *network) listen(ctx context.Context, hostname string, begin uint16, end uint16) (uint16, error) {

	lc := net.ListenConfig{
		KeepAlive: defaultKeepAlivePeriod * time.Second,
	}
	for port := begin; port <= end; port++ {
		hostPort := net.JoinHostPort(hostname, strconv.Itoa(int(port)))
		listener, err := lc.Listen(ctx, "tcp", hostPort)
		if err != nil {
			continue
		}
		if n.tls.Enable {
			config := tls.Config{
				Certificates:       []tls.Certificate{n.tls.Server},
				InsecureSkipVerify: n.tls.SkipVerify,
			}
			listener = tls.NewListener(listener, &config)
		}
		n.listener = listener

		go func() {
			for {
				c, err := listener.Accept()
				if err != nil {
					if ctx.Err() == nil {
						continue
					}
					lib.Log(err.Error())
					return
				}
				lib.Log("[%s] NETWORK accepted new connection from %s", n.nodename, c.RemoteAddr().String())

				details, err := n.handshake.Accept(c, n.tls.Enable, n.cookie)
				if err != nil {
					lib.Log("[%s] Can't handshake with %s: %s", n.nodename, c.RemoteAddr().String(), err)
					c.Close()
					continue
				}
				// TODO we need to detect somehow whether to enable software keepalive.
				// Erlang nodes are required to be receiving keepalive messages,
				// but Ergo doesn't need it.
				details.Flags.EnableSoftwareKeepAlive = true
				connection, err := n.proto.Init(n.ctx, c, n.nodename, details)
				if err != nil {
					c.Close()
					continue
				}

				cInternal := connectionInternal{
					conn:       c,
					connection: connection,
				}

				if _, err := n.registerConnection(details.Name, cInternal); err != nil {
					// Race condition:
					// There must be another goroutine which already created and registered
					// connection to this node.
					// Close this connection and use the already registered connection
					c.Close()
					continue
				}

				// run serving connection
				go func(ctx context.Context, ci connectionInternal) {
					n.proto.Serve(ci.connection, n.router)
					n.unregisterConnection(details.Name, nil)
					n.proto.Terminate(ci.connection)
					ci.conn.Close()
				}(ctx, cInternal)

			}
		}()

		// return port number this node listenig on for the incoming connections
		return port, nil
	}

	// all ports within a given range are taken
	return 0, fmt.Errorf("Can't start listener. Port range is taken")
}

func (n *network) connect(node string) (ConnectionInterface, error) {
	var route Route
	var c net.Conn
	var err error
	var enabledTLS bool
	lib.Log("[%s] NETWORK trying to connect to %#v", n.nodename, node)

	// resolve the route
	route, err = n.Resolve(node)
	if err != nil {
		return nil, err
	}
	customHandshake := route.Options.Handshake != nil
	lib.Log("[%s] NETWORK resolved %#v to %s:%d (custom handshake: %t)", n.nodename, node, route.Host, route.Port, customHandshake)

	HostPort := net.JoinHostPort(route.Host, strconv.Itoa(int(route.Port)))
	dialer := net.Dialer{
		KeepAlive: defaultKeepAlivePeriod * time.Second,
	}

	if route.Options.IsErgo == true {
		// rely on the route TLS settings if they were defined
		if route.Options.EnableTLS {
			if route.Options.Cert.Certificate == nil {
				// use the local TLS settings
				config := tls.Config{
					Certificates:       []tls.Certificate{n.tls.Client},
					InsecureSkipVerify: n.tls.SkipVerify,
				}
				tlsdialer := tls.Dialer{
					NetDialer: &dialer,
					Config:    &config,
				}
				c, err = tlsdialer.DialContext(n.ctx, "tcp", HostPort)
			} else {
				// use the route TLS settings
				config := tls.Config{
					Certificates: []tls.Certificate{route.Options.Cert},
				}
				tlsdialer := tls.Dialer{
					NetDialer: &dialer,
					Config:    &config,
				}
				c, err = tlsdialer.DialContext(n.ctx, "tcp", HostPort)
			}
			enabledTLS = true

		} else {
			// TLS disabled on a remote node
			c, err = dialer.DialContext(n.ctx, "tcp", HostPort)
		}

	} else {
		// rely on the local TLS settings
		if n.tls.Enable {
			config := tls.Config{
				Certificates:       []tls.Certificate{n.tls.Client},
				InsecureSkipVerify: n.tls.SkipVerify,
			}
			tlsdialer := tls.Dialer{
				NetDialer: &dialer,
				Config:    &config,
			}
			c, err = tlsdialer.DialContext(n.ctx, "tcp", HostPort)
			enabledTLS = true

		} else {
			c, err = dialer.DialContext(n.ctx, "tcp", HostPort)
		}
	}

	// check if we couldn't establish a connection with the node
	if err != nil {
		return nil, err
	}

	// handshake
	handshake := route.Options.Handshake
	if handshake == nil {
		// use default handshake
		handshake = n.handshake
	}

	cookie := n.cookie
	if route.Options.Cookie != "" {
		cookie = route.Options.Cookie
	}

	details, err := handshake.Start(c, enabledTLS, cookie)
	if err != nil {
		c.Close()
		return nil, err
	}
	if details.Name != node {
		err := fmt.Errorf("node %q introduced itself as %q", node, details.Name)
		lib.Warning("%s", err)
		return nil, err
	}

	// proto
	proto := route.Options.Proto
	if proto == nil {
		// use default proto
		proto = n.proto
	}

	// TODO we need to detect somehow whether to enable software keepalive.
	// Erlang nodes are required to be receiving keepalive messages,
	// but Ergo doesn't need it.
	details.Flags.EnableSoftwareKeepAlive = true
	connection, err := proto.Init(n.ctx, c, n.nodename, details)
	if err != nil {
		c.Close()
		return nil, err
	}
	cInternal := connectionInternal{
		conn:       c,
		connection: connection,
	}

	if registered, err := n.registerConnection(details.Name, cInternal); err != nil {
		// Race condition:
		// There must be another goroutine which already created and registered
		// connection to this node.
		// Close this connection and use the already registered one
		c.Close()
		if err == ErrTaken {
			return registered, nil
		}
		return nil, err
	}

	// run serving connection
	go func(ctx context.Context, ci connectionInternal) {
		proto.Serve(ci.connection, n.router)
		n.unregisterConnection(details.Name, nil)
		proto.Terminate(ci.connection)
		ci.conn.Close()
	}(n.ctx, cInternal)

	return connection, nil
}

func (n *network) registerConnection(peername string, ci connectionInternal) (ConnectionInterface, error) {
	lib.Log("[%s] NETWORK registering peer %#v", n.nodename, peername)
	n.connectionsMutex.Lock()
	defer n.connectionsMutex.Unlock()

	if registered, exist := n.connections[peername]; exist {
		// already registered
		return registered.connection, ErrTaken
	}
	n.connections[peername] = ci
	if ci.conn == nil {
		// this is proxy connection
		p, _ := n.connectionsProxy[ci.connection]
		p = append(p, peername)
		n.connectionsProxy[ci.connection] = p
	}
	return ci.connection, nil
}

func (n *network) unregisterConnection(peername string, disconnect *ProxyDisconnect) {
	lib.Log("[%s] NETWORK unregistering peer %v", n.nodename, peername)

	n.connectionsMutex.Lock()
	ci, exist := n.connections[peername]
	if exist == false {
		n.connectionsMutex.Unlock()
		return
	}
	delete(n.connections, peername)
	n.connectionsMutex.Unlock()

	n.router.RouteNodeDown(peername, disconnect)

	if ci.conn == nil {
		// it was proxy connection
		ci.connection.ProxyUnregisterSession(ci.proxySessionID)
		return
	}

	n.connectionsMutex.Lock()
	cp, _ := n.connectionsProxy[ci.connection]
	for _, p := range cp {
		lib.Log("[%s] NETWORK unregistering peer (via proxy) %v", n.nodename, p)
		delete(n.connections, p)
	}

	ct, _ := n.connectionsTransit[ci.connection]
	delete(n.connectionsTransit, ci.connection)
	n.connectionsMutex.Unlock()

	// send disconnect for the proxy sessions
	for _, p := range cp {
		disconnect := ProxyDisconnect{
			Node:   peername,
			Proxy:  n.nodename,
			Reason: "noconnection",
		}
		n.router.RouteNodeDown(p, &disconnect)
	}

	// disconnect for the transit proxy sessions
	for i := range ct {
		disconnect := ProxyDisconnect{
			Node:      peername,
			Proxy:     n.nodename,
			SessionID: ct[i],
			Reason:    "noconnection",
		}
		n.RouteProxyDisconnect(ci.connection, disconnect)
	}

}

//
// Connection interface default callbacks
//
func (c *Connection) Send(from gen.Process, to etf.Pid, message etf.Term) error {
	return ErrUnsupported
}
func (c *Connection) SendReg(from gen.Process, to gen.ProcessID, message etf.Term) error {
	return ErrUnsupported
}
func (c *Connection) SendAlias(from gen.Process, to etf.Alias, message etf.Term) error {
	return ErrUnsupported
}
func (c *Connection) Link(local gen.Process, remote etf.Pid) error {
	return ErrUnsupported
}
func (c *Connection) Unlink(local gen.Process, remote etf.Pid) error {
	return ErrUnsupported
}
func (c *Connection) LinkExit(local etf.Pid, remote etf.Pid, reason string) error {
	return ErrUnsupported
}
func (c *Connection) Monitor(local gen.Process, remote etf.Pid, ref etf.Ref) error {
	return ErrUnsupported
}
func (c *Connection) MonitorReg(local gen.Process, remote gen.ProcessID, ref etf.Ref) error {
	return ErrUnsupported
}
func (c *Connection) Demonitor(by etf.Pid, process etf.Pid, ref etf.Ref) error {
	return ErrUnsupported
}
func (c *Connection) DemonitorReg(by etf.Pid, process gen.ProcessID, ref etf.Ref) error {
	return ErrUnsupported
}
func (c *Connection) MonitorExitReg(process gen.Process, reason string, ref etf.Ref) error {
	return ErrUnsupported
}
func (c *Connection) MonitorExit(to etf.Pid, terminated etf.Pid, reason string, ref etf.Ref) error {
	return ErrUnsupported
}
func (c *Connection) SpawnRequest(nodeName string, behaviorName string, request gen.RemoteSpawnRequest, args ...etf.Term) error {
	return ErrUnsupported
}
func (c *Connection) SpawnReply(to etf.Pid, ref etf.Ref, pid etf.Pid) error {
	return ErrUnsupported
}
func (c *Connection) SpawnReplyError(to etf.Pid, ref etf.Ref, err error) error {
	return ErrUnsupported
}
func (c *Connection) ProxyConnectRequest(connect ProxyConnectRequest) error {
	return ErrUnsupported
}
func (c *Connection) ProxyConnectReply(reply ProxyConnectReply) error {
	return ErrUnsupported
}
func (c *Connection) ProxyDisconnect(disconnect ProxyDisconnect) error {
	return ErrUnsupported
}
func (c *Connection) ProxyRegisterSession(session ProxySession) error {
	return ErrUnsupported
}
func (c *Connection) ProxyUnregisterSession(id string) error {
	return ErrUnsupported
}
func (c *Connection) ProxyPacket(packet *lib.Buffer) error {
	return ErrUnsupported
}

//
// Handshake interface default callbacks
//
func (h *Handshake) Start(conn io.ReadWriter, tls bool, cookie string) (HandshakeDetails, error) {
	return HandshakeDetails{}, ErrUnsupported
}
func (h *Handshake) Accept(conn io.ReadWriter, tls bool, cookie string) (HandshakeDetails, error) {
	return HandshakeDetails{}, ErrUnsupported
}
func (h *Handshake) Version() HandshakeVersion {
	var v HandshakeVersion
	return v
}

// internals

func (n *network) putProxyConnectRequest(r proxyConnectRequest) {
	n.proxyConnectRequestMutex.Lock()
	defer n.proxyConnectRequestMutex.Unlock()
	n.proxyConnectRequest[r.request.ID] = r
}

func (n *network) cancelProxyConnectRequest(cancel ProxyConnectCancel) {
	n.proxyConnectRequestMutex.Lock()
	defer n.proxyConnectRequestMutex.Unlock()

	r, found := n.proxyConnectRequest[cancel.ID]
	if found == false {
		return
	}

	delete(n.proxyConnectRequest, cancel.ID)
	select {
	case r.cancel <- cancel:
	default:
	}
	return
}

func (n *network) waitProxyConnection(id etf.Ref, timeout int) (ConnectionInterface, error) {
	n.proxyConnectRequestMutex.RLock()
	r, found := n.proxyConnectRequest[id]
	n.proxyConnectRequestMutex.RUnlock()

	if found == false {
		return nil, ErrProxyUnknownRequest
	}

	defer func(id etf.Ref) {
		n.proxyConnectRequestMutex.Lock()
		delete(n.proxyConnectRequest, id)
		n.proxyConnectRequestMutex.Unlock()
	}(id)

	timer := lib.TakeTimer()
	defer lib.ReleaseTimer(timer)
	timer.Reset(time.Second * time.Duration(timeout))

	for {
		select {
		case connection := <-r.connection:
			return connection, nil
		case err := <-r.cancel:
			return nil, fmt.Errorf("[%s] %s", err.From, err.Reason)
		case <-timer.C:
			return nil, ErrTimeout
		case <-n.ctx.Done():
			// node is on the way to terminate, it means connection is closed
			// so it doesn't matter what kind of error will be returned
			return nil, ErrProxyUnknownRequest
		}
	}
}

func (n *network) getProxyConnectRequest(id etf.Ref) (proxyConnectRequest, bool) {
	n.proxyConnectRequestMutex.RLock()
	defer n.proxyConnectRequestMutex.RUnlock()
	r, found := n.proxyConnectRequest[id]
	return r, found
}

//
// internals
//

func generateProxyDigest(node string, cookie string, peer string, pubkey []byte) []byte {
	// md5(md5(md5(md5(node)+cookie)+peer)+pubkey)
	digest1 := md5.Sum([]byte(node))
	digest2 := md5.Sum(append(digest1[:], []byte(cookie)...))
	digest3 := md5.Sum(append(digest2[:], []byte(peer)...))
	digest4 := md5.Sum(append(digest3[:], pubkey...))
	return digest4[:]
}

func generateSelfSignedCert(version Version) (tls.Certificate, error) {
	var cert = tls.Certificate{}
	org := fmt.Sprintf("%s %s", version.Prefix, version.Release)
	certPrivKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return cert, err
	}
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return cert, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{org},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 365),
		//IsCA:        true,

		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	template.IPAddresses = append(template.IPAddresses, net.ParseIP("127.0.0.1"))

	certBytes, err1 := x509.CreateCertificate(rand.Reader, &template, &template,
		&certPrivKey.PublicKey, certPrivKey)
	if err1 != nil {
		return cert, err1
	}

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	certPrivKeyPEM := new(bytes.Buffer)
	x509Encoded, _ := x509.MarshalECPrivateKey(certPrivKey)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509Encoded,
	})

	return tls.X509KeyPair(certPEM.Bytes(), certPrivKeyPEM.Bytes())
}
