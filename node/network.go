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
	AddStaticRoute(name string, port uint16, options RouteOptions) error
	RemoveStaticRoute(name string) bool
	StaticRoutes() []Route
	Connect(peername string) error
	Nodes() []string

	GetConnection(peername string) (ConnectionInterface, error)

	connect(to string) (ConnectionInterface, error)
	stopNetwork()
}

type network struct {
	nodename string
	ctx      context.Context
	listener net.Listener

	resolver          Resolver
	staticOnly        bool
	staticRoutes      map[string]Route
	staticRoutesMutex sync.Mutex

	connections      map[string]ConnectionInterface
	mutexConnections sync.Mutex

	remoteSpawn      map[string]gen.ProcessBehavior
	remoteSpawnMutex sync.Mutex

	tls      TLS
	version  Version
	creation uint32

	router    CoreRouter
	handshake Handshake
	proto     Proto
}

func newNetwork(ctx context.Context, nodename string, options Options, router CoreRouter) (networkInternal, error) {
	n := &network{
		nodename:     nodename,
		ctx:          ctx,
		staticOnly:   options.StaticRoutesOnly,
		staticRoutes: make(map[string]Route),
		connections:  make(map[string]ConnectionInterface),
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

	if err := n.loadTLS(options); err != nil {
		return nil, err
	}

	handshakeVersion, err := n.handshake.Init(n.nodename, n.creation, n.tls.Enabled)
	if err != nil {
		return nil, err
	}

	port, err := n.listen(ctx, options)
	if err != nil {
		return nil, err
	}

	resolverOptions := ResolverOptions{
		NodeVersion:      n.version,
		HandshakeVersion: handshakeVersion,
		EnabledTLS:       n.tls.Enabled,
		EnabledProxy:     options.ProxyMode != ProxyModeDisabled,
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
		Name:         name,
		Port:         port,
		RouteOptions: options,
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
	for k, v := range n.staticRoutes {
		routes = append(routes, v)
	}

	return routes
}

// GetConnection
func (n *network) GetConnection(peername string) (ConnectionInterface, error) {
	n.mutexConnections.Lock()
	connection, ok := n.connections[peername]
	n.mutexConnections.Unlock()
	if ok {
		return connection, nil
	}

	connection, err := n.connect(peername)
	if err != nil {
		lib.Log("[%s] CORE no route to node %q: %s", n.nodename, peername, err)
		return nil, ErrNoRoute
	}

	return connection, nil
}

// Connect
func (n *network) Connect(peername string) error {
	_, err := n.GetConnection(peername)
	return err
}

// Nodes
func (n *network) Nodes() []string {
	list := []string{}
	n.mutexConnections.Lock()
	defer n.mutexConnections.Unlock()

	for name := range n.connections {
		list = append(list, name)
	}
	return list
}

func (n *network) loadTLS(options Options) error {
	switch options.TLSMode {
	case TLSModeAuto:
		cert, err := generateSelfSignedCert(n.version)
		if err != nil {
			return fmt.Errorf("Can't generate certificate: %s\n", err)
		}

		n.tls.Server = cert
		n.tls.Client = cert
		n.tls.Mode = TLSModeAuto
		n.tls.Enabled = true
		n.tls.Config = tls.Config{
			Certificates:       []tls.Certificate{cert},
			InsecureSkipVerify: true,
		}

	case TLSModeStrict:
		certServer, err := tls.LoadX509KeyPair(options.TLSCrtServer, options.TLSKeyServer)
		if err != nil {
			return fmt.Errorf("Can't load server certificate: %s\n", err)
		}
		certClient, err := tls.LoadX509KeyPair(options.TLSCrtClient, options.TLSKeyClient)
		if err != nil {
			return fmt.Errorf("Can't load client certificate: %s\n", err)
		}

		n.tls.Server = certServer
		n.tls.Client = certClient
		n.tls.Mode = TLSModeStrict
		n.tls.Enabled = true
		n.tls.Config = tls.Config{
			Certificates: []tls.Certificate{certServer},
			ServerName:   "localhost",
		}
	}
	return nil
}

func (n *network) listen(ctx context.Context, options Options) (uint16, error) {
	var TLSenabled bool = true

	lc := net.ListenConfig{}
	for p := options.ListenBegin; p <= options.ListenEnd; p++ {
		hostPort := net.JoinHostPort(name, strconv.Itoa(int(p)))
		listener, err := lc.Listen(ctx, "tcp", hostPort)
		if err != nil {
			continue
		}
		if n.tls.Enabled {
			listener = tls.NewListener(l, n.tls.Config)
		}
		n.listener = listener

		go func() {
			for {
				c, err := listener.Accept()
				lib.Log("[%s] Accepted new connection from %s", n.name, c.RemoteAddr().String())

				if err != nil {
					if ctx.Err() == nil {
						continue
					}
					lib.Log(err.Error())
					return
				}

				peername, protoOptions, err := n.handshake.Start(c, n.creation, n.tls.Enabled)
				if err != nil {
					lib.Log("[%s] Can't handshake with %s: %s", n.nodename, c.RemoteAddr().String(), e)
					c.Close()
					continue
				}
				connection, err := n.proto.Init(c, protoOptions, n.router)
				if err != nil {
					c.Close()
					continue
				}
				_, isConnectionObject := connection.(*Connection)
				if isConnectionObject == false {
					c.Close()
					return nil, fmt.Errorf("Proto initialization failed. Not a *node.Connection object")
				}
				if _, err := n.registerConnection(peername, connection); err != nil {
					// Race condition:
					// There must be another goroutine which already created and registered
					// connection to this node.
					// Close this connection and use the already registered connection
					c.Close()
					// call connection callback
					continue
				}
				go func() {
					n.proto.Serve(n.ctx, connection)
					c.Close()
					n.unregisterConnection(peername)
				}()

			}
		}()

		// return port number this node listenig on for the incoming connections
		return p, nil
	}

	// all ports within a given range are taken
	return 0, fmt.Errorf("Can't start listener. Port range is taken")
}

func (n *network) connect(to string) (ConnectionInterface, error) {
	var route Route
	var c net.Conn
	var err error
	var enabledTLS bool

	// resolve the route
	route, err = n.resolver.Resolve(to)
	if err != nil {
		return nil, err
	}

	HostPort := net.JoinHostPort(route.Host, strconv.Itoa(int(route.Port)))

	if route.IsErgo == true {
		// rely on the route TLS settings if they were defined
		if route.EnabledTLS {
			if route.TLSConfig == nil {
				// use the local TLS settings
				tlsdialer := tls.Dialer{
					Config: &n.tls.Config,
				}
				c, err = tlsdialer.DialContext(n.ctx, "tcp", HostPort)
			} else {
				// use the route TLS settings
				tlsdialer := tls.Dialer{
					Config: route.TLSConfig,
				}
				c, err = tlsdialer.DialContext(n.ctx, "tcp", HostPort)
			}

			enabledTLS = true

		} else {
			// TLS disabled on a remote node
			dialer := net.Dialer{}
			c, err = dialer.DialContext(n.ctx, "tcp", HostPort)
		}

	} else {
		// rely on the local TLS settings
		if n.tls.Enabled {
			tlsdialer := tls.Dialer{
				Config: &n.tls.Config,
			}
			c, err = tlsdialer.DialContext(n.ctx, "tcp", HostPort)

			enabledTLS = true

		} else {
			dialer := net.Dialer{}
			c, err = dialer.DialContext(n.ctx, "tcp", HostPort)

		}
	}

	// check if we couldn't establish a connection with the node
	if err != nil {
		return nil, err
	}

	// handshake
	handshake := route.Handshake
	if handshake == nil {
		// use default handshake
		handshake = n.handshake
	}

	protoOptions, err := handshake.Start(c, n.creation, enabledTLS)
	if err != nil {
		c.Close()
		return nil, err
	}

	// proto
	proto := route.Proto
	if proto == nil {
		// use default proto
		proto = n.proto
	}

	connection, err := proto.Init(c, protoOptions, n.router)
	if err != nil {
		c.Close()
		return nil, err
	}

	_, isConnectionObject := connection.(*Connection)
	if isConnectionObject == false {
		c.Close()
		return nil, fmt.Errorf("Proto initialization failed. Not a *node.Connection object")
	}

	if registered, err := n.registerConnection(peername, connection); err != nil {
		// Race condition:
		// There must be another goroutine which already created and registered
		// connection to this node.
		// Close this connection and use the already registered connection
		c.Close()
		// call connection callback
		return registered, nil
	}

	go func() {
		proto.Serve(n.ctx, connection)
		c.Close()
		n.unregisterConnection(peername)
	}()

	return connection, nil
}

func (n *network) registerConnection(peername string, connection ConnectionInterface) (ConnectionInterface, error) {
	lib.Log("[%s] NETWORK registering peer %#v", n.nodename, peername)
	n.mutexConnections.Lock()
	defer n.mutexConnection.Unlock()

	if registered, exist := n.connections[peername]; exist {
		// already registered
		return registered, ErrTaken
	}
	n.connections[peername] = connection
	return connection, nil
}

func (n *network) unregisterConnection(peername string) {
	lib.Log("[%s] NETWORK unregistering peer %v", n.nodename, peername)
	n.mutexConnections.Lock()
	_, exist := n.connections[name]
	delete(n.connections, name)
	n.mutexConnections.Unlock()

	if exist {
		n.router.RouteNodeDown(name)
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

type peer struct {
	name string
	send []chan []etf.Term
	i    int
	n    int

	mutex sync.Mutex
}

func (p *peer) getChannel() chan []etf.Term {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	c := p.send[p.i]

	p.i++
	if p.i < p.n {
		return c
	}

	p.i = 0
	return c
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
func (c *Connection) SendExit(local etf.Pid, remote etf.Pid) error {
	return ErrUnsupported
}
func (c *Connection) Monitor(local gen.Process, remote etf.Pid, ref etf.Ref) error {
	return ErrUnsupported
}
func (c *Connection) MonitorReg(local gen.Process, remote gen.ProcessID, ref etf.Ref) error {
	return ErrUnsupported
}
func (c *Connection) Demonitor(ref etf.Ref) error {
	return ErrUnsupported
}
func (c *Connection) MonitorExitReg(process gen.Process, reason string, ref etf.Ref) error {
	return ErrUnsupported
}
func (c *Connection) MonitorExit(to etf.Pid, terminated etf.Pid, reason string, ref etf.Ref) error {
	return ErrUnsupported
}
func (c *Connection) SpawnRequest() error {
	return ErrUnsupported
}
func (c *Connection) Proxy() error {
	return ErrUnsupported
}
func (c *Connection) ProxyReg() error {
	return ErrUnsupported
}

//
// Handshake interface default callbacks
//

func (h *Handshake) Start(ctx context.Context, c *Connection) (*Connection, error) {
	return nil, ErrUnsupported
}

func (h *Handshake) Accept(ctx context.Context, c *Connection) (*Connection, error) {
	return nil, ErrUnsupported
}
