package node

import (
	"bytes"
	"context"
	"encoding/pem"
	"math/big"
	"sync"
	"time"

	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
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
	// static route methods
	AddStaticRoute(name string, port uint16, options RouteOptions) error
	RemoveStaticRoute(name string) bool
	StaticRoutes() []Route

	// proxy route methods
	//AddProxyRoute(name string, proxy string) error
	//RemoveProxyRoute(name string) bool
	//ProxyRoutes() []string

	Resolve(peername string) (Route, error)
	Connect(peername string) error
	Disconnect(peername string) error
	Nodes() []string

	GetConnection(peername string) (ConnectionInterface, error)
	getConnection(peername string) (ConnectionInterface, error)

	connect(to string) (ConnectionInterface, error)
	stopNetwork()
}

type connectionInternal struct {
	conn       net.Conn
	connection ConnectionInterface
	proxy      string
}

type proxyRoute struct {
	ProxyRoute
	Flags
}

type network struct {
	nodename string
	ctx      context.Context
	listener net.Listener

	resolver          Resolver
	staticOnly        bool
	staticRoutes      map[string]Route
	staticRoutesMutex sync.Mutex

	proxyRoutes      map[string]proxyRoute
	proxyRoutesMutex sync.RWMutex

	connections      map[string]connectionInternal
	connectionsMutex sync.RWMutex

	remoteSpawn      map[string]gen.ProcessBehavior
	remoteSpawnMutex sync.Mutex

	tls      TLS
	proxy    Proxy
	version  Version
	creation uint32

	router    CoreRouter
	handshake HandshakeInterface
	proto     ProtoInterface
}

func newNetwork(ctx context.Context, nodename string, options Options, router CoreRouter) (networkInternal, error) {
	n := &network{
		nodename:     nodename,
		ctx:          ctx,
		staticOnly:   options.StaticRoutesOnly,
		staticRoutes: make(map[string]Route),
		proxyRoutes:  make(map[string]proxyRoute),
		connections:  make(map[string]connectionInternal),
		remoteSpawn:  make(map[string]gen.ProcessBehavior),
		resolver:     options.Resolver,
		handshake:    options.Handshake,
		proto:        options.Proto,
		router:       router,
		creation:     options.Creation,
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

// AddStaticRoute adds a static route to the node with the given name
func (n *network) AddStaticRoute(name string, port uint16, options RouteOptions) error {
	ns := strings.Split(name, "@")
	if len(ns) != 2 {
		return fmt.Errorf("wrong FQDN")
	}
	if _, err := net.LookupHost(ns[1]); err != nil {
		return err
	}

	route := Route{
		Name:    name,
		Host:    ns[1],
		Port:    port,
		Options: options,
	}

	n.staticRoutesMutex.Lock()
	defer n.staticRoutesMutex.Unlock()

	_, exist := n.staticRoutes[name]
	if exist {
		return ErrTaken
	}
	n.staticRoutes[name] = route

	return nil
}

// RemoveStaticRoute removes static route record. Returns false if it doesn't exist.
func (n *network) RemoveStaticRoute(name string) bool {
	n.staticRoutesMutex.Lock()
	defer n.staticRoutesMutex.Unlock()
	_, exist := n.staticRoutes[name]
	if exist {
		delete(n.staticRoutes, name)
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

func (n *network) getConnection(peername string) (ConnectionInterface, error) {
	n.connectionsMutex.RLock()
	ci, ok := n.connections[peername]
	n.connectionsMutex.RUnlock()
	if ok {
		return ci.connection, nil
	}

	n.proxyRoutesMutex.RLock()
	proxyRoute, is_proxy := n.proxyRoutes[peername]
	n.proxyRoutesMutex.RUnlock()

	if is_proxy == false {
		connection, err := n.connect(peername)
		if err != nil {
			lib.Log("[%s] CORE no route to node %q: %s", n.nodename, peername, err)
			return nil, ErrNoRoute
		}
		return connection, nil
	}

	connection, err := n.getConnection(proxyRoute.Name)
	if err != nil {
		return nil, err
	}

}

// GetConnection
func (n *network) GetConnection(peername string) (ConnectionInterface, error) {
	if peername == n.nodename {
		return nil, fmt.Errorf("can't connect to itself")
	}
	n.connectionsMutex.RLock()
	ci, ok := n.connections[peername]
	n.connectionsMutex.RUnlock()
	if ok {
		return ci.connection, nil
	}

	ci, err := n.getConnection(peername)
	if err != nil {
		return nil, err
	}

	var salt [32]byte
	rand.Read(salt[:])
	digest := generateProxyDigest(proxyRoute.Cookie, peername, salt[:])
	if err := connection.ProxyConnect(peername, digest[:], salt[:], proxyRoute.Encryption); err != nil {
		return nil, err
	}

	// FIXME wait reply with Flags

	ci = connectionInternal{
		proxy:      proxyRoute.Name,
		connection: connection,
	}

	if registered, err := n.registerConnection(peername, ci); err != nil {
		// Race condition:
		// There must be another goroutine which already created and registered
		// proxy connection to this node.
		// Close this proxy connection and use the already registered one
		connection.ProxyDisconnect(peername)
		return registered.connection, nil
	}

	return connection, nil
}

// Resolve
func (n *network) Resolve(peername string) (Route, error) {
	n.staticRoutesMutex.Lock()
	defer n.staticRoutesMutex.Unlock()

	if r, ok := n.staticRoutes[peername]; ok {
		return r, nil
	}

	if n.staticOnly {
		return Route{}, ErrNoRoute
	}

	return n.resolver.Resolve(peername)
}

// Connect
func (n *network) Connect(peername string) error {
	_, err := n.GetConnection(peername)
	return err
}

// Disconnect
func (n *network) Disconnect(peername string) error {
	n.connectionsMutex.RLock()
	connectionInternal, ok := n.connections[peername]
	n.connectionsMutex.RUnlock()
	if !ok {
		return ErrNoRoute
	}

	if connectionInternal.proxy != "" {
		n.unregisterConnection(peername)
		return connectionInternal.connection.ProxyDisconnect(peername)
	}

	connectionInternal.conn.Close()
	return nil
}

// Nodes
func (n *network) Nodes() []string {
	list := []string{}
	n.connectionsMutex.RLock()
	defer n.connectionsMutex.RUnlock()

	for name := range n.connections {
		list = append(list, name)
	}
	return list
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
		return registered.connection, nil
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

func (n *network) registerConnection(peername string, ci connectionInternal) (connectionInternal, error) {
	lib.Log("[%s] NETWORK registering peer %#v", n.nodename, peername)
	n.connectionsMutex.Lock()
	defer n.connectionsMutex.Unlock()

	if registered, exist := n.connections[peername]; exist {
		// already registered
		return registered, ErrTaken
	}
	n.connections[peername] = ci
	return ci, nil
}

func (n *network) unregisterConnection(peername string) {
	lib.Log("[%s] NETWORK unregistering peer %v", n.nodename, peername)
	n.connectionsMutex.Lock()
	_, exist := n.connections[peername]
	delete(n.connections, peername)
	n.connectionsMutex.Unlock()

	if exist {
		n.router.RouteNodeDown(peername)
	}
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
func (c *Connection) ProxyConnect(node string, digest string, salt string) error {
	return ErrUnsupported
}
func (c *Connection) ProxyDisconnect(node string) error {
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

///
func generateProxyDigest(cookie string, peer string, salt []byte) [16]byte {
	// md5(md5(md5(cookie)+peer)+salt)
	digest1 := md5.Sum(cookie)
	digest2 := md5.Sum(append(digest1, node))
	digest3 := md5.Sum(append(digest2, salt))
	return digest3
}
