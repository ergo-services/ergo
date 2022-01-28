package node

import (
	"bytes"
	"context"
	"encoding/pem"
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
	AddStaticPortRoute(node string, port uint16, options RouteOptions) error
	RemoveStaticRoute(node string) bool
	StaticRoutes() []Route

	// add/remove proxy route
	AddProxyRoute(node string, route ProxyRoute) error
	AddProxyTransitRoute(name string, proxy string) error
	RemoveProxyRoute(node string) bool
	ProxyRoutes() []ProxyRoute

	Resolve(peername string) (Route, error)
	Connect(peername string) error
	Disconnect(peername string) error
	Nodes() []string

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
	ctx      context.Context
	listener net.Listener

	resolver          Resolver
	staticOnly        bool
	staticRoutes      map[string]Route
	staticRoutesMutex sync.Mutex

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

	router    coreRouterInternal
	handshake HandshakeInterface
	proto     ProtoInterface

	remoteSpawn      map[string]gen.ProcessBehavior
	remoteSpawnMutex sync.Mutex
}

func newNetwork(ctx context.Context, nodename string, options Options, router coreRouterInternal) (networkInternal, error) {
	n := &network{
		nodename:             nodename,
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

	n.tls = options.TLS
	selfSignedCert, err := generateSelfSignedCert(n.version)
	if n.tls.Server.Certificate == nil {
		n.tls.Server = selfSignedCert
		n.tls.SkipVerify = true
	}
	if n.tls.Client.Certificate == nil {
		n.tls.Client = selfSignedCert
	}

	err = n.handshake.Init(n.nodename, n.creation, options.Flags)
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
	if err := n.resolver.Register(nodename, port, resolverOptions); err != nil {
		return nil, err
	}

	return n, nil
}

func (n *network) stopNetwork() {
	if n.listener != nil {
		n.listener.Close()
	}
}

// AddStaticPortRoute adds a static route to the node with the given name
func (n *network) AddStaticPortRoute(node string, port uint16, options RouteOptions) error {
	ns := strings.Split(node, "@")
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
	if _, err := net.LookupHost(host); err != nil {
		return err
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

	n.staticRoutesMutex.Lock()
	defer n.staticRoutesMutex.Unlock()
	for _, v := range n.staticRoutes {
		routes = append(routes, v)
	}

	return routes
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
		ID: n.router.MakeRef(),
		To: peername,
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
		n.unregisterConnection(node)
		disconnect := ProxyDisconnect{
			From:      n.nodename,
			SessionID: ci.proxySessionID,
			Reason:    "normal",
		}
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

			// proxy feature must be enabled explicitly for the transitional requests
			if n.proxy.Enable == false {
				lib.Log("[%s] NETWORK proxy. Proxy feature is disabled on this node")
				return ErrProxyDisabled
			}
			if request.Hop < 1 {
				lib.Log("[%s] NETWORK proxy. Error: exceeded hop limit")
				return ErrProxyHopExceeded
			}
			request.Hop--

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
			request.Flags = DefaultProxyFlags()
		}

		request.Hop = route.MaxHop
		if request.Hop < 1 {
			request.Hop = DefaultProxyMaxHop
		}
		connectRequest := proxyConnectRequest{
			privateKey: privKey,
			request:    request,
			connection: make(chan ConnectionInterface),
			cancel:     make(chan ProxyConnectCancel),
		}
		request.Path = append([]string{n.nodename}, request.Path...)
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
	if has_route {
		cookie = route.Cookie
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
	flags := n.proxy.Flags
	if flags.Enable == false {
		flags = DefaultProxyFlags()
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
		SessionID: sessionID,
		Path:      request.Path[1:],
	}

	if err := from.ProxyConnectReply(reply); err != nil {
		// can't send reply. ignore this connection request
		lib.Log("[%s] NETWORK proxy. Proxy connect request. Can't send reply: %s ", n.nodename, err)
		n.unregisterConnection(peername)
		return ErrProxyConnect
	}

	session := ProxySession{
		ID:        sessionID,
		NodeFlags: reply.Flags,
		PeerFlags: request.Flags,
		PeerName:  peername,
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
		if n.proxy.Enable == false {
			return ErrProxyDisabled
		}

		if len(reply.Path) == 0 {
			return ErrProxyHopExceeded
		}

		connection, err := n.getConnectionDirect(reply.Path[0], false)
		if err != nil {
			return err
		}
		if connection == from {
			return ErrProxyLoopDetected
		}

		reply.Path = reply.Path[1:]

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
		// closed
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
		lib.Log("[%s] CORE route proxy. Proxy connect reply has wrong digest")
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

	session := ProxySession{
		ID:        reply.SessionID,
		NodeFlags: r.request.Flags,
		PeerFlags: reply.Flags,
		PeerName:  r.request.To,
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

	if cancel.Path[0] != n.nodename {
		connection, err := n.getConnectionDirect(cancel.Path[0], false)
		if err != nil {
			return err
		}

		if connection == from {
			return ErrProxyLoopDetected
		}

		cancel.Path = cancel.Path[1:]

		if err := connection.ProxyConnectCancel(cancel); err != nil {
			return err
		}
		return nil
	}

	n.cancelProxyConnectRequest(cancel)
	return nil
}

func (n *network) RouteProxyDisconnect(from ConnectionInterface, disconnect ProxyDisconnect) error {

	n.proxyTransitSessionsMutex.RLock()
	session, ok := n.proxyTransitSessions[disconnect.SessionID]
	n.proxyTransitSessionsMutex.RUnlock()
	if !ok {
		// check for the proxy connection endpoint
		var peername string
		var found bool
		var ci connectionInternal

		// get peername by session id
		n.connectionsMutex.RLock()
		for p, c := range n.connections {
			if c.proxySessionID == disconnect.SessionID {
				found = true
				peername = p
				ci = c
				break
			}
		}
		if found == false {
			n.connectionsMutex.RUnlock()
			return ErrProxySessionUnknown
		}
		n.connectionsMutex.RUnlock()

		if ci.proxySessionID != disconnect.SessionID || ci.connection != from {
			return ErrProxySessionUnknown
		}

		n.unregisterConnection(peername)
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
		return ErrProxySessionUnknown
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
		return session.a.Proxy(packet)
	case session.a:
		return session.b.Proxy(packet)
	default:
		// TODO there must be another error if none of a and b are not equal the 'from' value
		return ErrProxySessionUnknown
	}
}

func (n *network) AddProxyRoute(node string, route ProxyRoute) error {
	n.proxyRoutesMutex.Lock()
	defer n.proxyRoutesMutex.Unlock()

	if _, exist := n.proxyRoutes[node]; exist {
		return ErrTaken
	}

	n.proxyRoutes[node] = route
	return nil
}

func (n *network) AddProxyTransitRoute(name string, proxy string) error {
	route := ProxyRoute{
		Proxy: name,
	}
	return n.AddProxyRoute(name, route)
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
	return routes
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

				peername, protoFlags, err := n.handshake.Accept(c, n.tls.Enable)
				if err != nil {
					lib.Log("[%s] Can't handshake with %s: %s", n.nodename, c.RemoteAddr().String(), err)
					c.Close()
					continue
				}
				// TODO we need to detect somehow whether to enable software keepalive.
				// Erlang nodes are required to be receiving keepalive messages,
				// but Ergo doesn't need it.
				protoFlags.EnableSoftwareKeepAlive = true
				connection, err := n.proto.Init(n.ctx, c, peername, protoFlags)
				if err != nil {
					c.Close()
					continue
				}

				cInternal := connectionInternal{
					conn:       c,
					connection: connection,
				}

				if _, err := n.registerConnection(peername, cInternal); err != nil {
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
					n.unregisterConnection(peername)
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

func (n *network) connect(peername string) (ConnectionInterface, error) {
	var route Route
	var c net.Conn
	var err error
	var enabledTLS bool

	// resolve the route
	route, err = n.resolver.Resolve(peername)
	if err != nil {
		return nil, err
	}

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

	protoFlags, err := n.handshake.Start(c, enabledTLS)
	if err != nil {
		c.Close()
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
	protoFlags.EnableSoftwareKeepAlive = true
	connection, err := n.proto.Init(n.ctx, c, peername, protoFlags)
	if err != nil {
		c.Close()
		return nil, err
	}
	cInternal := connectionInternal{
		conn:       c,
		connection: connection,
	}

	if registered, err := n.registerConnection(peername, cInternal); err != nil {
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
		n.proto.Serve(ci.connection, n.router)
		n.unregisterConnection(peername)
		n.proto.Terminate(ci.connection)
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

func (n *network) unregisterConnection(peername string) {
	fmt.Printf("[%s] NETWORK unregistering peer %v\n", n.nodename, peername)
	lib.Log("[%s] NETWORK unregistering peer %v", n.nodename, peername)

	n.connectionsMutex.Lock()
	ci, exist := n.connections[peername]
	if exist == false {
		n.connectionsMutex.Unlock()
		return
	}
	delete(n.connections, peername)
	n.router.RouteNodeDown(peername)

	if ci.conn == nil {
		// it was proxy connection
		n.connectionsMutex.Unlock()
		return
	}
	cp, _ := n.connectionsProxy[ci.connection]
	// disconnect proxy connections
	for _, p := range cp {
		lib.Log("[%s] NETWORK unregistering peer (via proxy) %v", n.nodename, p)
		delete(n.connections, p)
		n.router.RouteNodeDown(p)
	}

	// disconnect transit proxy sessions
	ct, exist := n.connectionsTransit[ci.connection]
	if exist == false {
		n.connectionsMutex.Unlock()
		return
	}
	delete(n.connectionsTransit, ci.connection)
	n.connectionsMutex.Unlock()

	for i := range ct {
		disconnect := ProxyDisconnect{
			From:      n.nodename,
			SessionID: ct[i],
			Reason:    "connection with " + peername + " closed on " + n.nodename,
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
func (c *Connection) Proxy(packet *lib.Buffer) error {
	return ErrUnsupported
}

//
// Handshake interface default callbacks
//
func (h *Handshake) Start(c net.Conn) (Flags, error) {
	return Flags{}, ErrUnsupported
}
func (h *Handshake) Accept(c net.Conn) (string, Flags, error) {
	return "", Flags{}, ErrUnsupported
}
func (h *Handshake) Version() HandshakeVersion {
	var v HandshakeVersion
	return v
}

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
