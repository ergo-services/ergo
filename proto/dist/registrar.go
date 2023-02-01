package dist

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

const (
	DefaultEPMDPort uint16 = 4369

	epmdAliveReq      = 120
	epmdAliveResp     = 121
	epmdAliveRespX    = 118
	epmdPortPleaseReq = 122
	epmdPortResp      = 119
	epmdNamesReq      = 110

	// wont be implemented
	// epmdDumpReq = 100
	// epmdKillReq = 107
	// epmdStopReq = 115

	// Extra data
	ergoExtraMagic    = 4411
	ergoExtraVersion1 = 1
)

// epmd implements registrar interface
type epmdRegistrar struct {
	// EPMD server
	enableEPMD bool
	host       string
	port       uint16

	// Node
	name             string
	nodePort         uint16
	nodeName         string
	nodeHost         string
	handshakeVersion node.HandshakeVersion

	running int32
	extra   []byte
}

func CreateRegistrar() node.Registrar {
	registrar := &epmdRegistrar{
		port: DefaultEPMDPort,
	}
	return registrar
}

func CreateRegistrarWithLocalEPMD(host string, port uint16) node.Registrar {
	if port == 0 {
		port = DefaultEPMDPort
	}
	registrar := &epmdRegistrar{
		enableEPMD: true,
		host:       host,
		port:       port,
	}
	return registrar
}

func CreateRegistrarWithRemoteEPMD(host string, port uint16) node.Registrar {
	if port == 0 {
		port = DefaultEPMDPort
	}
	registrar := &epmdRegistrar{
		host: host,
		port: port,
	}
	return registrar
}

func (e *epmdRegistrar) Register(ctx context.Context, name string, options node.RegisterOptions) error {
	if atomic.CompareAndSwapInt32(&e.running, 0, 1) == false {
		return fmt.Errorf("registrar is already running")
	}

	n := strings.Split(name, "@")
	if len(n) != 2 {
		return fmt.Errorf("(EMPD) FQDN for node name is required (example: node@hostname)")
	}

	e.name = name
	e.nodeName = n[0]
	e.nodeHost = n[1]
	e.nodePort = options.Port
	e.handshakeVersion = options.HandshakeVersion

	e.composeExtra(options)
	ready := make(chan error)

	go func() {
		defer atomic.StoreInt32(&e.running, 0)
		buf := make([]byte, 16)
		var reconnecting bool

		for {
			if ctx.Err() != nil {
				// node is stopped
				return
			}

			// try to start embedded EPMD server
			if e.enableEPMD {
				startServerEPMD(ctx, e.host, e.port)
			}

			// register this node on EPMD server
			conn, err := e.registerNode(options)
			if err != nil {
				if reconnecting == false {
					ready <- err
					break
				}
				lib.Warning("EPMD client: can't register node %q (%s). Retry in 3 seconds...", name, err)
				time.Sleep(3 * time.Second)
				continue
			}

			go func() {
				<-ctx.Done()
				conn.Close()
			}()

			if reconnecting == false {
				ready <- nil
				reconnecting = true
			}

			for {
				_, err := conn.Read(buf)
				if err == nil {
					continue
				}
				break
			}
			lib.Log("[%s] EPMD client: closing connection", name)
		}
	}()

	defer close(ready)
	return <-ready
}

func (e *epmdRegistrar) Resolve(name string) (node.Route, error) {
	var route node.Route

	n := strings.Split(name, "@")
	if len(n) != 2 {
		return node.Route{}, fmt.Errorf("incorrect FQDN node name (example: node@localhost)")
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(n[1], fmt.Sprintf("%d", e.port)))
	if err != nil {
		return node.Route{}, err
	}

	defer conn.Close()

	if err := e.sendPortPleaseReq(conn, n[0]); err != nil {
		return node.Route{}, err
	}

	route.Node = n[0]
	route.Host = n[1]

	err = e.readPortResp(&route, conn)
	if err != nil {
		return node.Route{}, err
	}

	return route, nil
}

func (e *epmdRegistrar) ResolveProxy(name string) (node.ProxyRoute, error) {
	var route node.ProxyRoute
	return route, lib.ErrProxyNoRoute
}
func (e *epmdRegistrar) RegisterProxy(name string, maxhop int, flags node.ProxyFlags) error {
	return lib.ErrUnsupported
}
func (e *epmdRegistrar) UnregisterProxy(name string) error {
	return lib.ErrUnsupported
}
func (e *epmdRegistrar) Config() (node.RegistrarConfig, error) {
	return node.RegistrarConfig{}, lib.ErrUnsupported
}
func (e *epmdRegistrar) ConfigItem(name string) (etf.Term, error) {
	return nil, lib.ErrUnsupported
}

// just stub
func (e *epmdRegistrar) SetConfigUpdateCallback(func(string, etf.Term) error) error {
	return lib.ErrUnsupported
}

func (e *epmdRegistrar) composeExtra(options node.RegisterOptions) {
	buf := make([]byte, 4)

	// 2 bytes: ergoExtraMagic
	binary.BigEndian.PutUint16(buf[0:2], uint16(ergoExtraMagic))
	// 1 byte Extra version
	buf[2] = ergoExtraVersion1
	// 1 byte flag enabled TLS
	if options.EnableTLS {
		buf[3] = 1
	}
	e.extra = buf
	return
}

func (e *epmdRegistrar) readExtra(route *node.Route, buf []byte) {
	if len(buf) < 4 {
		return
	}
	magic := binary.BigEndian.Uint16(buf[0:2])
	if uint16(ergoExtraMagic) != magic {
		return
	}

	if buf[2] != ergoExtraVersion1 {
		return
	}

	if buf[3] == 1 {
		route.Options.TLS = &tls.Config{}
	}

	route.Options.IsErgo = true
	return
}

func (e *epmdRegistrar) registerNode(options node.RegisterOptions) (net.Conn, error) {
	//
	registrarHost := e.host
	if registrarHost == "" {
		registrarHost = e.nodeHost
	}
	dialer := net.Dialer{
		KeepAlive: 15 * time.Second,
	}
	dsn := net.JoinHostPort(registrarHost, strconv.Itoa(int(e.port)))
	conn, err := dialer.Dial("tcp", dsn)
	if err != nil {
		return nil, err
	}

	if err := e.sendAliveReq(conn); err != nil {
		conn.Close()
		return nil, err
	}

	if err := e.readAliveResp(conn); err != nil {
		conn.Close()
		return nil, err
	}

	lib.Log("[%s] EPMD client: node registered", e.name)
	return conn, nil
}

func (e *epmdRegistrar) sendAliveReq(conn net.Conn) error {
	buf := make([]byte, 2+14+len(e.nodeName)+len(e.extra))
	binary.BigEndian.PutUint16(buf[0:2], uint16(len(buf)-2))
	buf[2] = byte(epmdAliveReq)
	binary.BigEndian.PutUint16(buf[3:5], e.nodePort)
	// http://erlang.org/doc/reference_manual/distributed.html (section 13.5)
	// 77 — regular public node, 72 — hidden
	// We use a regular one
	buf[5] = 77
	// Protocol TCP
	buf[6] = 0
	// HighestVersion
	binary.BigEndian.PutUint16(buf[7:9], uint16(HandshakeVersion6))
	// LowestVersion
	binary.BigEndian.PutUint16(buf[9:11], uint16(HandshakeVersion5))
	// length Node name
	l := len(e.nodeName)
	binary.BigEndian.PutUint16(buf[11:13], uint16(l))
	// Node name
	offset := (13 + l)
	copy(buf[13:offset], e.nodeName)
	// Extra data
	l = len(e.extra)
	binary.BigEndian.PutUint16(buf[offset:offset+2], uint16(l))
	copy(buf[offset+2:offset+2+l], e.extra)
	// Send
	if _, err := conn.Write(buf); err != nil {
		return err
	}
	return nil
}

func (e *epmdRegistrar) readAliveResp(conn net.Conn) error {
	buf := make([]byte, 16)
	if _, err := conn.Read(buf); err != nil {
		return err
	}
	switch buf[0] {
	case epmdAliveResp, epmdAliveRespX:
	default:
		return fmt.Errorf("malformed EPMD response %v", buf)
	}
	if buf[1] != 0 {
		if buf[1] == 1 {
			return fmt.Errorf("can not register node with %q, name is taken", e.nodeName)
		}
		return fmt.Errorf("can not register %q, code: %v", e.nodeName, buf[1])
	}
	return nil
}

func (e *epmdRegistrar) sendPortPleaseReq(conn net.Conn, name string) error {
	buflen := uint16(2 + len(name) + 1)
	buf := make([]byte, buflen)
	binary.BigEndian.PutUint16(buf[0:2], uint16(len(buf)-2))
	buf[2] = byte(epmdPortPleaseReq)
	copy(buf[3:buflen], name)
	_, err := conn.Write(buf)
	return err
}

func (e *epmdRegistrar) readPortResp(route *node.Route, c net.Conn) error {

	buf := make([]byte, 1024)
	n, err := c.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("reading from link - %s", err)
	}
	buf = buf[:n]

	if buf[0] == epmdPortResp && buf[1] == 0 {
		p := binary.BigEndian.Uint16(buf[2:4])
		nameLen := binary.BigEndian.Uint16(buf[10:12])
		route.Port = p
		extraStart := 12 + int(nameLen)
		// read extra data
		buf = buf[extraStart:]
		extraLen := binary.BigEndian.Uint16(buf[:2])
		buf = buf[2 : extraLen+2]
		e.readExtra(route, buf)
		return nil
	} else if buf[1] > 0 {
		return fmt.Errorf("desired node not found")
	} else {
		return fmt.Errorf("malformed reply - %#v", buf)
	}
}
