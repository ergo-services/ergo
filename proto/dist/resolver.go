package dist

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

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
	ergoExtraMagic        = 4411
	ergoExtraVersion1     = 1
	ergoExtraEnabledTLS   = 100
	ergoExtraEnabledProxy = 101
)

// epmd implements resolver
type epmdResolver struct {
	node.Resolver

	ctx context.Context

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

	extra []byte
}

func CreateResolver(ctx context.Context) node.Resolver {
	resolver := &epmdResolver{
		ctx:  ctx,
		port: DefaultEPMDPort,
	}
	return resolver
}

func CreateResolverWithEPMD(ctx context.Context, host string, port uint16) node.Resolver {
	if port == 0 {
		port = DefaultEPMDPort
	}
	resolver := &epmdResolver{
		ctx:        ctx,
		enableEPMD: true,
		host:       host,
		port:       port,
	}
	startServerEPMD(ctx, host, port)
	return resolver
}

func (e *epmdResolver) Register(name string, port uint16, options node.ResolverOptions) error {
	n := strings.Split(name, "@")
	if len(n) != 2 {
		return fmt.Errorf("(EMPD) FQDN for node name is required (example: node@hostname)")
	}

	e.name = name
	e.nodeName = n[0]
	e.nodeHost = n[1]
	e.nodePort = port
	e.handshakeVersion = options.HandshakeVersion

	e.composeExtra(options)

	conn, err := e.registerNode(options)
	if err != nil {
		return err
	}
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := conn.Read(buf)
			if err == nil {
				continue
			}
			lib.Log("[%s] EPMD client: closing connection", name)

			// reconnect to the EPMD server
			for {
				if e.ctx.Err() != nil {
					// node is stopped
					return
				}

				// try to start embedded EPMD server
				if e.enableEPMD {
					startServerEPMD(e.ctx, e.host, e.port)
				}

				if c, err := e.registerNode(options); err != nil {
					lib.Log("EPMD client: can't register node %q (%s). Retry in 3 seconds...", name, err)
					time.Sleep(3 * time.Second)
				} else {
					conn = c
					break
				}
			}
		}
	}()

	go func() {
		<-e.ctx.Done()
		conn.Close()
	}()

	return nil
}

func (e *epmdResolver) Resolve(name string) (node.Route, error) {
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

	route.Name = n[0]
	route.Host = n[1]

	err = e.readPortResp(&route, conn)
	if err != nil {
		return node.Route{}, err
	}

	return route, nil

}

func (e *epmdResolver) composeExtra(options node.ResolverOptions) {
	buf := make([]byte, 5)

	// 2 bytes: ergoExtraMagic
	binary.BigEndian.PutUint16(buf[0:2], uint16(ergoExtraMagic))
	// 1 byte Extra version
	buf[3] = ergoExtraVersion1
	// 1 byte flag enabled TLS
	if options.EnabledTLS {
		buf[4] = 1
	}
	// 1 byte flag enabled proxy
	if options.EnabledProxy {
		buf[5] = 1
	}
	e.extra = buf
	return
}

func (e *epmdResolver) readExtra(route *node.Route, buf []byte) {
	if len(buf) < 5 {
		return
	}
	extraLen := int(binary.BigEndian.Uint16(buf[0:2]))
	if extraLen < len(buf)+2 {
		return
	}
	magic := binary.BigEndian.Uint16(buf[2:4])
	if uint16(ergoExtraMagic) != magic {
		return
	}

	if buf[4] != ergoExtraVersion1 {
		return
	}

	if buf[5] == 1 {
		route.EnabledTLS = true
	}

	if buf[6] == 1 {
		route.EnabledProxy = true
	}

	route.IsErgo = true

	return
}

func (e *epmdResolver) registerNode(options node.ResolverOptions) (net.Conn, error) {
	//
	resolverHost := e.host
	if resolverHost == "" {
		resolverHost = e.nodeHost
	}
	dsn := net.JoinHostPort(resolverHost, strconv.Itoa(int(e.port)))
	conn, err := net.Dial("tcp", dsn)
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

func (e *epmdResolver) sendAliveReq(conn net.Conn) error {
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

func (e *epmdResolver) readAliveResp(conn net.Conn) error {
	buf := make([]byte, 16)
	if _, err := conn.Read(buf); err != nil {
		return err
	}
	switch buf[0] {
	case epmdAliveResp, epmdAliveRespX:
	default:
		return fmt.Errorf("Malformed EPMD response %v", buf)
	}
	if buf[1] != 0 {
		return fmt.Errorf("Can't register %q. Code: %v", e.nodeName, buf[1])
	}
	return nil
}

func (e *epmdResolver) sendPortPleaseReq(conn net.Conn, name string) error {
	buflen := uint16(2 + len(name) + 1)
	buf := make([]byte, buflen)
	binary.BigEndian.PutUint16(buf[0:2], uint16(len(buf)-2))
	buf[2] = byte(epmdPortPleaseReq)
	copy(buf[3:buflen], name)
	_, err := conn.Write(buf)
	return err
}

func (e *epmdResolver) readPortResp(route *node.Route, c net.Conn) error {

	buf := make([]byte, 1024)
	_, err := c.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("reading from link - %s", err)
	}

	if buf[0] == epmdPortResp && buf[1] == 0 {
		p := binary.BigEndian.Uint16(buf[2:4])
		// we don't use all the extra info for a while. FIXME (do we need it?)
		nameLen := binary.BigEndian.Uint16(buf[10:12])
		route.Port = p
		extraStart := 12 + int(nameLen)

		e.readExtra(route, buf[extraStart:])
		return nil
	} else if buf[1] > 0 {
		return fmt.Errorf("desired node not found")
	} else {
		return fmt.Errorf("malformed reply - %#v", buf)
	}
}
