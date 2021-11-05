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
	"github.com/ergo-services/ergo/node/dist"

	"net"

	"strconv"
	"strings"
)

type networkInternal interface {
	// AddStaticRoute
	AddStaticRoute(name string, port uint16) error
	// AddStaticRouteExt
	AddStaticRouteExt(name string, port uint16, cookie string, tls bool) error
	// RemoveStaticRoute
	RemoveStaticRoute(name string)
	// Resolve
	Resolve(name string) (NetworkRoute, error)
	// Connect creates connection to the node with given name
	Connect(to string) error
}

type network struct {
	name             string
	ctx              context.Context
	remoteSpawnMutex sync.Mutex
	remoteSpawn      map[string]gen.ProcessBehavior
	epmd             *epmd

	tls TLS

	version   Version
	handshake Handshake
	proto     Proto
}

func newNetwork(ctx context.Context, name string, options Options, router Router) (networkInternal, error) {
	n := &network{
		name:      name,
		ctx:       ctx,
		proto:     options.Proto,
		handshake: options.Handshake,
		router:    router,
	}

	ns := strings.Split(name, "@")
	if len(ns) != 2 {
		return nil, fmt.Errorf("(EMPD) FQDN for node name is required (example: node@hostname)")
	}

	if err := n.loadTLS(options); err != nil {
		return nil, err
	}

	if err := n.handshake.Init(name); err != nil {
		return nil, err
	}

	if err := n.proto.Init(router); err != nil {
		return nil, err
	}

	port, err := n.listen(ctx, ns[1], router)
	if err != nil {
		return nil, err
	}

	n.epmd = &epmd{}
	if err := n.epmd.Init(ctx, name, port, opts); err != nil {
		return nil, err
	}
	return n, nil
}

// AddStaticRoute adds static route record into the EPMD client
func (n *network) AddStaticRoute(name string, port uint16) error {
	tlsEnabled := n.opts.TLSMode != TLSModeDisabled
	return n.epmd.addStaticRoute(name, port, n.opts.cookie, tlsEnabled)
}

func (n *network) AddStaticRouteExt(name string, port uint16, cookie string, tls bool) error {
	return n.epmd.addStaticRoute(name, port, cookie, tls)
}

// RemoveStaticRoute removes static route record from the EPMD client
func (n *network) RemoveStaticRoute(name string) {
	n.epmd.removeStaticRoute(name)
}

func (n *network) loadTLS(options Options) error {
	version, _ = options.Env[EnvKeyVersion].(Version)
	switch options.TLSMode {
	case TLSModeAuto:
		cert, err := generateSelfSignedCert(version)
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
		certServer, err := tls.LoadX509KeyPair(options.TLScrtServer, options.TLSkeyServer)
		if err != nil {
			return 0, fmt.Errorf("Can't load server certificate: %s\n", err)
		}
		certClient, err := tls.LoadX509KeyPair(options.TLScrtServer, options.TLSkeyServer)
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

// Resolve
func (n *network) Resolve(name string) (NetworkRoute, error) {
	return n.epmd.resolve(name)
}

func (n *network) handleMessage(fromNode string, control, message etf.Term) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s", r)
		}
	}()

	switch t := control.(type) {
	case etf.Tuple:
		switch act := t.Element(1).(type) {
		case int:
			switch act {
			case distProtoREG_SEND:
				// {6, FromPid, Unused, ToName}
				lib.Log("[%s] CONTROL REG_SEND [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
				n.registrar.route(t.Element(2).(etf.Pid), t.Element(4), message)

			case distProtoSEND:
				// {2, Unused, ToPid}
				// SEND has no sender pid
				lib.Log("[%s] CONTROL SEND [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
				n.registrar.route(etf.Pid{}, t.Element(3), message)

			case distProtoLINK:
				// {1, FromPid, ToPid}
				lib.Log("[%s] CONTROL LINK [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
				n.registrar.link(t.Element(2).(etf.Pid), t.Element(3).(etf.Pid))

			case distProtoUNLINK:
				// {4, FromPid, ToPid}
				lib.Log("[%s] CONTROL UNLINK [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
				n.registrar.unlink(t.Element(2).(etf.Pid), t.Element(3).(etf.Pid))

			case distProtoNODE_LINK:
				lib.Log("[%s] CONTROL NODE_LINK [from %s]: %#v", n.registrar.NodeName(), fromNode, control)

			case distProtoEXIT:
				// {3, FromPid, ToPid, Reason}
				lib.Log("[%s] CONTROL EXIT [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
				terminated := t.Element(2).(etf.Pid)
				reason := fmt.Sprint(t.Element(4))
				n.registrar.handleTerminated(terminated, "", string(reason))

			case distProtoEXIT2:
				lib.Log("[%s] CONTROL EXIT2 [from %s]: %#v", n.registrar.NodeName(), fromNode, control)

			case distProtoMONITOR:
				// {19, FromPid, ToProc, Ref}, where FromPid = monitoring process
				// and ToProc = monitored process pid or name (atom)
				lib.Log("[%s] CONTROL MONITOR [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
				n.registrar.monitorProcess(t.Element(2).(etf.Pid), t.Element(3), t.Element(4).(etf.Ref))

			case distProtoDEMONITOR:
				// {20, FromPid, ToProc, Ref}, where FromPid = monitoring process
				// and ToProc = monitored process pid or name (atom)
				lib.Log("[%s] CONTROL DEMONITOR [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
				n.registrar.demonitorProcess(t.Element(4).(etf.Ref))

			case distProtoMONITOR_EXIT:
				// {21, FromProc, ToPid, Ref, Reason}, where FromProc = monitored process
				// pid or name (atom), ToPid = monitoring process, and Reason = exit reason for the monitored process
				lib.Log("[%s] CONTROL MONITOR_EXIT [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
				reason := fmt.Sprint(t.Element(5))
				switch terminated := t.Element(2).(type) {
				case etf.Pid:
					n.registrar.handleTerminated(terminated, "", string(reason))
				case etf.Atom:
					vpid := virtualPid(gen.ProcessID{string(terminated), fromNode})
					n.registrar.handleTerminated(vpid, "", string(reason))
				}

			// Not implemented yet, just stubs. TODO.
			case distProtoSEND_SENDER:
				lib.Log("[%s] CONTROL SEND_SENDER unsupported [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
			case distProtoPAYLOAD_EXIT:
				lib.Log("[%s] CONTROL PAYLOAD_EXIT unsupported [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
			case distProtoPAYLOAD_EXIT2:
				lib.Log("[%s] CONTROL PAYLOAD_EXIT2 unsupported [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
			case distProtoPAYLOAD_MONITOR_P_EXIT:
				lib.Log("[%s] CONTROL PAYLOAD_MONITOR_P_EXIT unsupported [from %s]: %#v", n.registrar.NodeName(), fromNode, control)

			// alias support
			case distProtoALIAS_SEND:
				// {33, FromPid, Alias}
				lib.Log("[%s] CONTROL ALIAS_SEND [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
				alias := etf.Alias(t.Element(3).(etf.Ref))
				n.registrar.route(t.Element(2).(etf.Pid), alias, message)

			case distProtoSPAWN_REQUEST:
				// {29, ReqId, From, GroupLeader, {Module, Function, Arity}, OptList}
				lib.Log("[%s] CONTROL SPAWN_REQUEST [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
				registerName := ""
				for _, option := range t.Element(6).(etf.List) {
					name, ok := option.(etf.Tuple)
					if !ok {
						break
					}
					if name.Element(1).(etf.Atom) == etf.Atom("name") {
						registerName = string(name.Element(2).(etf.Atom))
					}
				}

				from := t.Element(3).(etf.Pid)
				ref := t.Element(2).(etf.Ref)

				mfa := t.Element(5).(etf.Tuple)
				module := mfa.Element(1).(etf.Atom)
				function := mfa.Element(2).(etf.Atom)
				var args etf.List
				if str, ok := message.(string); !ok {
					args, _ = message.(etf.List)
				} else {
					// stupid Erlang's strings :). [1,2,3,4,5] sends as a string.
					// args can't be anything but etf.List.
					for i := range []byte(str) {
						args = append(args, str[i])
					}
				}

				rb, err_behavior := n.registrar.RegisteredBehavior(remoteBehaviorGroup, string(module))
				if err_behavior != nil {
					message := etf.Tuple{distProtoSPAWN_REPLY, ref, from, 0, etf.Atom("not_provided")}
					n.registrar.routeRaw(from.Node, message)
					return
				}
				remote_request := gen.RemoteSpawnRequest{
					Ref:      ref,
					From:     from,
					Function: string(function),
				}
				process_opts := processOptions{}
				process_opts.Env = map[string]interface{}{"ergo:RemoteSpawnRequest": remote_request}

				process, err_spawn := n.registrar.spawn(registerName, process_opts, rb.Behavior, args...)
				if err_spawn != nil {
					message := etf.Tuple{distProtoSPAWN_REPLY, ref, from, 0, etf.Atom(err_spawn.Error())}
					n.registrar.routeRaw(from.Node, message)
					return
				}
				message := etf.Tuple{distProtoSPAWN_REPLY, ref, from, 0, process.Self()}
				n.registrar.routeRaw(from.Node, message)

			case distProtoSPAWN_REPLY:
				// {31, ReqId, To, Flags, Result}
				lib.Log("[%s] CONTROL SPAWN_REPLY [from %s]: %#v", n.registrar.NodeName(), fromNode, control)

				to := t.Element(3).(etf.Pid)
				process := n.registrar.ProcessByPid(to)
				if process == nil {
					return
				}
				ref := t.Element(2).(etf.Ref)
				//flags := t.Element(4)
				process.PutSyncReply(ref, t.Element(5))

			case distProtoProxy:
				// FIXME

			default:
				lib.Log("[%s] CONTROL unknown command [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
			}
		default:
			err = fmt.Errorf("unsupported message %#v", control)
		}
	}

	return
}

func (n *network) Connect(to string) error {
	var nr NetworkRoute
	var err error
	var c net.Conn
	if nr, err = n.epmd.resolve(string(to)); err != nil {
		return fmt.Errorf("Can't resolve port for %s: %s", to, err)
	}
	if nr.Cookie == "" {
		nr.Cookie = n.opts.cookie
	}
	ns := strings.Split(to, "@")

	TLSenabled := false

	switch n.opts.TLSMode {
	case TLSModeAuto:
		tlsdialer := tls.Dialer{
			Config: &tls.Config{
				Certificates:       []tls.Certificate{n.tlscertClient},
				InsecureSkipVerify: true,
			},
		}
		c, err = tlsdialer.DialContext(n.ctx, "tcp", net.JoinHostPort(ns[1], strconv.Itoa(nr.Port)))
		TLSenabled = true

	case TLSModeStrict:
		tlsdialer := tls.Dialer{
			Config: &tls.Config{
				Certificates: []tls.Certificate{n.tlscertClient},
			},
		}
		c, err = tlsdialer.DialContext(n.ctx, "tcp", net.JoinHostPort(ns[1], strconv.Itoa(nr.Port)))
		TLSenabled = true

	default:
		dialer := net.Dialer{}
		c, err = dialer.DialContext(n.ctx, "tcp", net.JoinHostPort(ns[1], strconv.Itoa(nr.Port)))
	}

	if err != nil {
		lib.Log("Error calling net.Dialer.DialerContext : %s", err.Error())
		return err
	}

	handshakeOptions := dist.HandshakeOptions{
		Name:     n.name,
		Cookie:   nr.Cookie,
		TLS:      TLSenabled,
		Hidden:   false,
		Creation: n.opts.creation,
		Version:  n.opts.HandshakeVersion,
	}
	link, e := dist.Handshake(c, handshakeOptions)
	if e != nil {
		return e
	}

	if err := n.serve(n.ctx, link); err != nil {
		c.Close()
		return err
	}
	return nil
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
