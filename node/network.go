package node

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"sync"

	//"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"runtime"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/lib"
	"github.com/halturin/ergo/node/dist"

	"math/big"

	"net"

	//	"net/http"
	"strconv"
	"strings"
	"time"
)

type networkInternal interface {
	Network
	connect(to etf.Atom) error
}

type network struct {
	registrar        registrarInternal
	name             string
	opts             Options
	ctx              context.Context
	remoteSpawnMutex sync.Mutex
	remoteSpawn      map[string]gen.ProcessBehavior
	epmd             *epmd
	tlscertServer    tls.Certificate
	tlscertClient    tls.Certificate
}

func newNetwork(ctx context.Context, name string, opts Options, r registrarInternal) (networkInternal, error) {
	n := &network{
		name:      name,
		opts:      opts,
		ctx:       ctx,
		registrar: r,
	}
	ns := strings.Split(name, "@")
	if len(ns) != 2 {
		return nil, fmt.Errorf("(EMPD) FQDN for node name is required (example: node@hostname)")
	}

	port, err := n.listen(ctx, ns[1])
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
	return n.epmd.AddStaticRoute(name, port, n.opts.cookie, tlsEnabled)
}

func (n *network) AddStaticRouteExt(name string, port uint16, cookie string, tls bool) error {
	return n.epmd.AddStaticRoute(name, port, cookie, tls)
}

// RemoveStaticRoute removes static route record from the EPMD client
func (n *network) RemoveStaticRoute(name string) {
	n.epmd.RemoveStaticRoute(name)
}

func (n *network) ProvideRemoteSpawn(name string, object gen.ProcessBehavior) {
	n.remoteSpawnMutex.Lock()
	n.remoteSpawn[name] = object
	n.remoteSpawnMutex.Unlock()
	return
}

func (n *network) RevokeRemoteSpawn(name string) bool {
	n.remoteSpawnMutex.Lock()
	defer n.remoteSpawnMutex.Unlock()
	if _, ok := n.remoteSpawn[name]; ok {
		delete(n.remoteSpawn, name)
		return true
	}
	return false
}

func (n *network) listen(ctx context.Context, name string) (uint16, error) {
	var TLSenabled bool = true
	versions := ctx.Value("versions").(map[string]interface{})

	lc := net.ListenConfig{}
	for p := n.opts.ListenRangeBegin; p <= n.opts.ListenRangeEnd; p++ {
		l, err := lc.Listen(ctx, "tcp", net.JoinHostPort(name, strconv.Itoa(int(p))))
		if err != nil {
			continue
		}

		switch n.opts.TLSMode {
		case TLSModeAuto:
			cert, err := generateSelfSignedCert(versions)
			if err != nil {
				return 0, fmt.Errorf("Can't generate certificate: %s\n", err)
			}

			n.tlscertServer = cert
			n.tlscertClient = cert

			TLSconfig := &tls.Config{
				Certificates:       []tls.Certificate{cert},
				InsecureSkipVerify: true,
			}
			l = tls.NewListener(l, TLSconfig)

		case TLSModeStrict:
			certServer, err := tls.LoadX509KeyPair(n.opts.TLScrtServer, n.opts.TLSkeyServer)
			if err != nil {
				return 0, fmt.Errorf("Can't load server certificate: %s\n", err)
			}
			certClient, err := tls.LoadX509KeyPair(n.opts.TLScrtServer, n.opts.TLSkeyServer)
			if err != nil {
				return 0, fmt.Errorf("Can't load client certificate: %s\n", err)
			}

			n.tlscertServer = certServer
			n.tlscertClient = certClient

			TLSconfig := &tls.Config{
				Certificates: []tls.Certificate{certServer},
				ServerName:   "localhost",
			}
			l = tls.NewListener(l, TLSconfig)

		default:
			TLSenabled = false
		}

		go func() {
			for {
				c, err := l.Accept()
				lib.Log("[%s] Accepted new connection from %s", n.name, c.RemoteAddr().String())

				if ctx.Err() != nil {
					// Context was canceled
					c.Close()
					return
				}

				if err != nil {
					lib.Log(err.Error())
					continue
				}
				handshakeOptions := dist.HandshakeOptions{
					Name:     n.name,
					Cookie:   n.opts.cookie,
					TLS:      TLSenabled,
					Hidden:   n.opts.Hidden,
					Creation: n.opts.creation,
					Version:  n.opts.HandshakeVersion,
				}

				link, e := dist.HandshakeAccept(c, handshakeOptions)
				if e != nil {
					lib.Log("[%s] Can't handshake with %s: %s", n.name, c.RemoteAddr().String(), e)
					c.Close()
					continue
				}

				// start serving this link
				if err := n.serve(ctx, link); err != nil {
					lib.Log("Can't serve connection link due to: %s", err)
					c.Close()
				}

			}
		}()

		// return port number this node listenig on for the incoming connections
		return p, nil
	}

	// all the ports within a given range are taken
	return 0, fmt.Errorf("Can't start listener. Port range is taken")
}
func (n *network) serve(ctx context.Context, link *dist.Link) error {
	// define the total number of reader/writer goroutines
	numHandlers := runtime.GOMAXPROCS(n.opts.ConnectionHandlers)

	// do not use shared channels within intencive code parts, impacts on a performance
	receivers := struct {
		recv []chan *lib.Buffer
		n    int
		i    int
	}{
		recv: make([]chan *lib.Buffer, n.opts.RecvQueueLength),
		n:    numHandlers,
	}

	p := &peer{
		name: link.GetRemoteName(),
		send: make([]chan []etf.Term, numHandlers),
		n:    numHandlers,
	}

	if err := n.registrar.registerPeer(p); err != nil {
		// duplicate link?
		return err
	}

	// run readers for incoming messages
	for i := 0; i < numHandlers; i++ {
		// run packet reader/handler routines (decoder)
		recv := make(chan *lib.Buffer, n.opts.RecvQueueLength)
		receivers.recv[i] = recv
		go link.ReadHandlePacket(ctx, recv, n.handleMessage)
	}

	cacheIsReady := make(chan bool)

	// run link reader routine
	go func() {
		var err error
		var packetLength int
		var recv chan *lib.Buffer

		linkctx, cancel := context.WithCancel(ctx)
		defer cancel()

		go func() {
			select {
			case <-linkctx.Done():
				// if node's context is done
				link.Close()
			}
		}()

		// initializing atom cache if its enabled
		if !n.opts.DisableHeaderAtomCache {
			link.SetAtomCache(etf.NewAtomCache(linkctx))
		}
		cacheIsReady <- true

		defer func() {
			link.Close()
			n.registrar.unregisterPeer(link.GetRemoteName())

			// close handlers channel
			for i := 0; i < numHandlers; i++ {
				if p.send[i] != nil {
					close(p.send[i])
				}
				if receivers.recv[i] != nil {
					close(receivers.recv[i])
				}
			}
		}()

		b := lib.TakeBuffer()
		for {
			packetLength, err = link.Read(b)
			if err != nil || packetLength == 0 {
				// link was closed or got malformed data
				if err != nil {
					fmt.Println("link was closed", link.GetPeerName(), "error:", err)
				}
				lib.ReleaseBuffer(b)
				return
			}

			// take new buffer for the next reading and append the tail (part of the next packet)
			b1 := lib.TakeBuffer()
			b1.Set(b.B[packetLength:])
			// cut the tail and send it further for handling.
			// buffer b has to be released by the reader of
			// recv channel (link.ReadHandlePacket)
			b.B = b.B[:packetLength]
			recv = receivers.recv[receivers.i]
			recv <- b

			// set new buffer as a current for the next reading
			b = b1

			// round-robin switch to the next receiver
			receivers.i++
			if receivers.i < receivers.n {
				continue
			}
			receivers.i = 0

		}
	}()

	// we should make sure if the cache is ready before we start writers
	<-cacheIsReady

	// run readers/writers for incoming/outgoing messages
	for i := 0; i < numHandlers; i++ {
		// run writer routines (encoder)
		send := make(chan []etf.Term, n.opts.SendQueueLength)
		p.mutex.Lock()
		p.send[i] = send
		p.mutex.Unlock()
		go link.Writer(send, n.opts.FragmentationUnit)
	}

	return nil
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
				n.registrar.Route(t.Element(2).(etf.Pid), t.Element(4), message)

			case distProtoSEND:
				// {2, Unused, ToPid}
				// SEND has no sender pid
				lib.Log("[%s] CONTROL SEND [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
				n.registrar.Route(etf.Pid{}, t.Element(3), message)

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
				n.registrar.processTerminated(terminated, "", string(reason))

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
					n.registrar.processTerminated(terminated, "", string(reason))
				case etf.Atom:
					pid := fakeMonitorPidFromName(string(terminated), fromNode)
					n.registrar.processTerminated(pid, "", string(reason))
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
			case distProtoALIAS_SEND:
				// {33, FromPid, Alias}
				lib.Log("[%s] CONTROL ALIAS_SEND [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
				alias := etf.Alias(t.Element(3).(etf.Ref))
				n.registrar.Route(t.Element(2).(etf.Pid), alias, message)

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
				var args etf.List
				if str, ok := message.(string); !ok {
					args = message.(etf.List)
				} else {
					// stupid Erlang's strings :). [1,2,3,4,5] sends as a string.
					// args can't be anything but etf.List.
					for i := range []byte(str) {
						args = append(args, str[i])
					}
				}

				n.remoteSpawnMutex.Lock()
				object, provided := n.remoteSpawn[string(module)]
				n.remoteSpawnMutex.Unlock()
				if !provided {
					message := etf.Tuple{distProtoSPAWN_REPLY, ref, from, 0, etf.Atom("not_provided")}
					n.registrar.RouteRaw(from.Node, message)
					return
				}

				process, err_spawn := n.registrar.spawn(registerName, processOptions{}, object, args...)
				if err_spawn != nil {
					message := etf.Tuple{distProtoSPAWN_REPLY, ref, from, 0, etf.Atom(err_spawn.Error())}
					n.registrar.RouteRaw(from.Node, message)
					return
				}
				message := etf.Tuple{distProtoSPAWN_REPLY, ref, from, 0, process.Self()}
				n.registrar.RouteRaw(from.Node, message)

			case distProtoSPAWN_REPLY:
				// {31, ReqId, To, Flags, Result}
				lib.Log("[%s] CONTROL SPAWN_REPLY [from %s]: %#v", n.registrar.NodeName(), fromNode, control)

				to := t.Element(3).(etf.Pid)
				process := n.registrar.GetProcessByPid(to)
				if process == nil {
					return
				}
				ref := t.Element(2).(etf.Ref)
				//flags := t.Element(4)
				process.PutSyncReply(ref, t.Element(5))

			default:
				lib.Log("[%s] CONTROL unknown command [from %s]: %#v", n.registrar.NodeName(), fromNode, control)
			}
		default:
			err = fmt.Errorf("unsupported message %#v", control)
		}
	}

	return
}

func (n *network) connect(to etf.Atom) error {
	var pc portcookietls
	var err error
	var c net.Conn
	if pc, err = n.epmd.resolve(string(to)); err != nil {
		return fmt.Errorf("Can't resolve port for %s: %s", to, err)
	}
	if pc.cookie == "" {
		pc.cookie = n.opts.cookie
	}
	ns := strings.Split(string(to), "@")

	TLSenabled := false

	switch n.opts.TLSMode {
	case TLSModeAuto:
		tlsdialer := tls.Dialer{
			Config: &tls.Config{
				Certificates:       []tls.Certificate{n.tlscertClient},
				InsecureSkipVerify: true,
			},
		}
		c, err = tlsdialer.DialContext(n.ctx, "tcp", net.JoinHostPort(ns[1], strconv.Itoa(pc.port)))
		TLSenabled = true

	case TLSModeStrict:
		tlsdialer := tls.Dialer{
			Config: &tls.Config{
				Certificates: []tls.Certificate{n.tlscertClient},
			},
		}
		c, err = tlsdialer.DialContext(n.ctx, "tcp", net.JoinHostPort(ns[1], strconv.Itoa(pc.port)))
		TLSenabled = true

	default:
		dialer := net.Dialer{}
		c, err = dialer.DialContext(n.ctx, "tcp", net.JoinHostPort(ns[1], strconv.Itoa(pc.port)))
	}

	if err != nil {
		lib.Log("Error calling net.Dialer.DialerContext : %s", err.Error())
		return err
	}

	handshakeOptions := dist.HandshakeOptions{
		Name:     n.name,
		Cookie:   pc.cookie,
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

func generateSelfSignedCert(versions map[string]interface{}) (tls.Certificate, error) {
	var cert = tls.Certificate{}
	var prefix string = "ergo"
	var version string = ""
	p, ok := versions["prefix"]
	if ok {
		prefix, _ = p.(string)
	}
	p, ok = versions["version"]
	if ok {
		version, _ = p.(string)
	}

	org := fmt.Sprintf("%s %s", prefix, version)

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