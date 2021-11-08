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
	RemoveStaticRoute(name string) error
	StaticRoutes() []Route
	connect(to string) (Connection, error)
}

type network struct {
	name string
	ctx  context.Context

	resolver          Resolver
	staticOnly        bool
	staticRoutes      map[string]Route
	staticRoutesMutex sync.Mutex

	remoteSpawn      map[string]gen.ProcessBehavior
	remoteSpawnMutex sync.Mutex

	tls     TLS
	version Version

	handshake Handshake
	proto     Proto
}

func newNetwork(ctx context.Context, name string, options Options, router Router) (networkInternal, error) {
	n := &network{
		name:         name,
		ctx:          ctx,
		staticOnly:   options.StaticRoutesOnly,
		staticRoutes: make(map[string]Route),
		remoteSpawn:  make(map[string]gen.ProcessBehavior),
		resolver:     options.Resolver,
		proto:        options.Proto,
		handshake:    options.Handshake,
		router:       router,
		creation:     options.Creation,
	}

	ns := strings.Split(name, "@")
	if len(ns) != 2 {
		return nil, fmt.Errorf("(EMPD) FQDN for node name is required (example: node@hostname)")
	}

	n.version, _ = options.Env[EnvKeyVersion].(Version)

	if err := n.loadTLS(options); err != nil {
		return nil, err
	}

	handshakeVersion, err := n.handshake.Init(name)
	if err != nil {
		return nil, err
	}

	if err := n.proto.Init(router); err != nil {
		return nil, err
	}

	port, err := n.listen(ctx, ns[1], router)
	if err != nil {
		return nil, err
	}

	resolverOptions := ResolverOptions{
		NodeVersion:      version,
		HandshakeVersion: handshakeVersion,
		EnabledTLS:       n.tls.Enabled,
		EnabledProxy:     options.ProxyMode != ProxyModeDisabled,
	}
	if err := n.resolver.Register(name, port, resolverOptions); err != nil {
		return nil, err
	}

	return n, nil
}

// AddStaticRoute adds a static route to the node with the given name
func (n *network) AddStaticRoute(name string, port uint16, options RouteOptions) error {
	n := strings.Split(name, "@")
	if len(n) != 2 {
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
			return 0, fmt.Errorf("Can't load server certificate: %s\n", err)
		}
		certClient, err := tls.LoadX509KeyPair(options.TLSCrtClient, options.TLSKeyClient)
		if err != nil {
			return 0, fmt.Errorf("Can't load client certificate: %s\n", err)
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
func (n *network) listen(ctx context.Context, name string, router CoreRouter) (uint16, error) {
	var TLSenabled bool = true

	lc := net.ListenConfig{}
	for p := n.opts.ListenRangeBegin; p <= n.opts.ListenRangeEnd; p++ {
		l, err := lc.Listen(ctx, "tcp", net.JoinHostPort(name, strconv.Itoa(int(p))))
		if err != nil {
			continue
		}

		go func() {
			for {
				conn, err := l.Accept()
				lib.Log("[%s] Accepted new connection from %s", n.name, c.RemoteAddr().String())

				if ctx.Err() != nil {
					// Context was canceled
					conn.Close()
					return
				}

				if err != nil {
					lib.Log(err.Error())
					continue
				}
				// wrap handler to catch panic
				handshakeAccept := func(ctx context.Context, conn net.Conn) (c *Connection, err error) {
					defer func() {
						if r := recover(); r != nil {
							c = nil
							err = r
						}
					}()
					c, err = n.opts.Handshake.Accept(ctx, conn)
					return
				}

				connection, err := handshakeAccept(ctx, conn)
				if err != nil {
					lib.Log("[%s] Can't handshake with %s: %s", n.name, conn.RemoteAddr().String(), e)
					conn.Close()
					continue
				}
				// FIXME
				// question: whether to register this peer before the Serve call?
				// what if this name is taken?

				// start serving this link
				go connection.Serve(router)

				// FIXME try to register this connection
				// if err happened close the link
				//p := &peer{
				//	name: link.GetRemoteName(),
				//	send: make([]chan []etf.Term, numHandlers),
				//	n:    numHandlers,
				//}
				//
				//if err := n.registrar.registerPeer(p); err != nil {
				//	// duplicate link?
				//	return err
				//}

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

	// check if we couldn't reach the host
	if err != nil {
		return nil, err
	}

	// handshake
	handshake := route.Handshake
	if handshake == nil {
		// use default handshake
		handshake = n.handshake
	}

	connection, err := handshake.Start(c, n.creation, enabledTLS)
	if err != nil {
		c.Close()
		return nil, err
	}
	_, isConnectionObject := connection.(*Connection)
	if isConnectionObject == false {
		c.Close()
		return nil, fmt.Errorf("Handshake start failed. Not a *node.Connection object")
	}

	// proto
	proto := route.Proto
	if proto == nil {
		// use default proto
		proto = n.proto
	}

	go proto.Serve(connection, protoOptions)

	return connection, nil
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
func (c *Connection) Options() ProtoOptions {
	return ProtoOptions{}
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
