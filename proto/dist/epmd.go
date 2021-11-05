package dist

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

const (
	DefaultEPMDPort uint16 = 4369

	epmdAliveReq       = 120
	epmdAliveResp      = 121
	epmdPortPlease2Req = 122
	epmdPort2Resp      = 119
	epmdNamesReq       = 110 // $n
	epmdDumpReq        = 100 // $d
	epmdKillReq        = 107 // $k
	epmdStopReq        = 115 // $s

	ergoExtraMagic        = 4411
	ergoExtraVersion      = 1
	ergoExtraEnabledTLS   = 100
	ergoExtraEnabledProxy = 101
)

type epmd struct {
	Name   string
	Domain string

	// Listening port for incoming connections
	NodePort uint16

	// EPMD port for the cluster
	Port uint16
	Type uint8

	Protocol uint8
	HighVsn  uint16
	LowVsn   uint16
	Extra    []byte
	Creation uint16

	response chan interface{}
}

type EPMD struct {
	node.Resolver

	ctx context.Context

	enableServer  bool
	host          string
	port          uint16
	serverPortMap map[string]node
	serverMutex   sync.Mutex

	nodePort         uint16
	nodeName         string
	nodeHost         string
	handshakeVersion HandshakeVersion

	staticOnly   bool
	staticRoutes map[string]gen.Route
	staticMutex  sync.Mutex

	extra []byte
}

func CreateResolver(ctx context.Context, enableServer bool, host string, port uint16) *EPMD {
	epmd := &EPMD{
		ctx:          ctx,
		enableServer: enableServer,
		host:         host,
		port:         port,
	}
	epmd.startServer()
	return epmd
}

func (e *EPMD) Register(name string, port uint16, options node.ResolverOptions) error {
	n := strings.Split(name, "@")
	if len(n) != 2 {
		return fmt.Errorf("(EMPD) FQDN for node name is required (example: node@hostname)")
	}

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
				e.startServer()

				if c, err := e.registerNode(options); err != nil {
					lib.Log("[%s] EPMD client: can't register node %q (%s). Retry in 3 seconds...", name, err)
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

func (e *EPMD) Resolve(name string) (gen.Route, error) {
	return gen.Route{}, nil
}

func (e *EPMD) AddStaticRoute(name string, port uint16, options gen.RouteOptions) error {
	return nil
}

func (e *EPMD) RemoveStaticRoute(name string) error {
	return nil
}

func (e *EPMD) startServer() {
	if e.enableServer == false {
		return
	}

	lc := net.ListenConfig{}
	epmd, err := lc.Listen(ctx, "tcp", net.JoinHostPort(e.host, strconv.Itoa(int(e.port))))
	if err != nil {
		lib.Log("Can't start embedded EPMD service: %s", err)
		return
	}

	lib.Log("Started embedded EMPD service and listen port: %d", port)

	go func() {
		for {
			c, err := epmd.Accept()
			if err != nil {
				lib.Log(err.Error())
				continue
			}

			lib.Log("EPMD accepted new connection from %s", c.RemoteAddr().String())

			//epmd connection handler loop
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				name := ""
				for {
					n, err := c.Read(buf)
					lib.Log("Request from EPMD client: %v", buf[:n])
					if err != nil {
						if name != "" {
							e.nodeLeave(name)
						}
						return
					}
					// buf[0:1] - length
					if uint16(n-2) != binary.BigEndian.Uint16(buf[0:2]) {
						continue
					}

					switch buf[2] {
					case epmdAliveReq:
						name, route, err = e.readAliveReq(c)
						if err != nil {
							// send error and close connection
							e.sendAliveResp(c, 1)
							return
						}

						// check if node with this name is already registered
						e.serverMutex.Lock()
						_, exist := e.serverPortMap[name]
						e.serverMutex.Unlock()
						if exist {
							// send error and close connection
							e.sendAliveResp(c, 1)
							return
						}

						// register new node as a route
						e.serverMutex.Lock()
						e.serverPortMap[name] = route
						e.serverMutex.Unlock()

						if err := e.sendAliveResp(c, 0); err != nil {
							return
						}
						if tcp, ok := c.(*net.TCPConn); ok {
							tcp.SetKeepAlive(true)
							tcp.SetKeepAlivePeriod(15 * time.Second)
							tcp.SetNoDelay(true)
						}
						continue
					case epmdPortPlease2Req:
						e.sendPortPlease2Resp(c, buf[3:n])
						return
					case epmdNamesReq:
						e.sendNamesResp(c, port, buf[3:n])
						return
					default:
						lib.Log("unknown EPMD request")
						return
					}

				}
			}(c)

		}
	}()

	return nil
}

func (e *EPMD) composeExtraVersion1(options node.ResolverOptions) {
	buf := make([]byte, 5)

	// 2 bytes: ergoExtraMagic
	binary.BigEndian.PutUint16(buff[0:2], uint16(ergoExtraMagic))
	// 1 byte Extra version
	buf[3] = ergoExtraVersion
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

func (e *EPMD) readExtra(buf []byte, route *gen.Route) {
	if len(buf) < 5 {
		return
	}
	magic := binary.BigEndian.Uint16(buf[0:2])
	if uint16(ergoExtraMagic) != magic {
		return
	}

	if buf[3] != ergoExtraVersion {
		return
	}

	if buf[4] == 1 {
		route.EnabledTLS = true
	}

	if buf[5] == 1 {
		route.EnabledProxy = true
	}

	route.IsErgo = true

	return
}

func (e *EPMD) registerNode(options node.ResolverOptions) (net.Conn, error) {
	dsn := net.JoinHostPort(options.ServerHost, strconv.Itoa(int(options.ServerPort)))
	conn, err := net.Dial("tcp", dsn)
	if err != nil {
		return nil, err
	}

	if _, err := e.sendAliveReq(conn); err != nil {
		conn.Close()
		return nil, err
	}

	if err := e.readAliveResp(conn); err != nil {
		conn.Close()
		return nil, err
	}

	lib.Log("[%s] EPMD client: node registered", name)
	return conn, nil
}

func (e *EPMD) sendAliveReq(conn net.Conn) error {
	buf := make([]byte, 2+14+len(e.nodeName)+len(e.Extra))
	binary.BigEndian.PutUint16(buf[0:2], uint16(len(buf)-2))
	buf[2] = byte(epmdAlive2Req)
	binary.BigEndian.PutUint16(buf[3:5], e.nodePort)
	// http://erlang.org/doc/reference_manual/distributed.html (section 13.5)
	// 77 — regular public node, 72 — hidden
	// We use a regular one
	buf[5] = 77
	// Protocol TCP
	buf[6] = 0
	// HighestVersion
	binary.BigEndian.PutUint16(buf[7:9], uint16(DistHandshakeVersion6))
	// LowestVersion
	binary.BigEndian.PutUint16(buf[9:11], uint16(DistHandshakeVersion5))
	// length Node name
	l := len(e.nodeName)
	binary.BigEndian.PutUint16(reply[11:13], uint16(l))
	// Node name
	offset := (13 + l)
	copy(buf[13:offset], e.nodeName)
	// Extra data
	l = len(e.Extra)
	binary.BigEndian.PutUint16(buf[offset:offset+2], uint16(l))
	copy(buf[offset+2:offset+2+l], e.Extra)
	// Send
	if _, err := conn.Write(buf); err != nil {
		return err
	}
	return nil
}

func (e *EMPD) readAliveResp(conn net.Conn) error {
	buf := make([]byte, 16)
	if _, err := conn.Read(buf); err != nil {
		return err
	}
	if buf[0] != epmdAlive2Resp {
		return fmt.Errorf("Malformed EMPD response")
	}
	if buf[1] != 0 {
		return fmt.Errorf("Can't register. Code: %d", e.nodeName, buf[1])
	}
	return nil
}

func (e *EPMD) readAliveReq(req []byte) (string, gen.Route, error) {
	// Name length
	l := binary.BigEndian.Uint16(req[8:10])
	// Name
	name := string(req[10 : 10+l])
	// Port
	route := gen.Route{
		Port: binary.BigEndian.Uint16(req[0:2]),
	}

	if err := e.readExtra(req[10+l], &route); err != nil {
		return name, route, err
	}

	return name, route, nil
}

func (e *EPMD) sendAliveResp(c net.Conn, code int) error {
	buf := make([]byte, 4)
	reply[0] = epmdAliveResp
	reply[1] = byte(code)

	// Creation. Ergo doesn't use it. Just for Erlang nodes.
	binary.BigEndian.PutUint16(buf[2:], uint16(1))
	_, err := c.Write(buf)
	return err
}

func (e *epmd) addStaticRoute(name string, port uint16, cookie string, tls bool) error {
	ns := strings.Split(name, "@")
	if len(ns) == 1 {
		ns = append(ns, "localhost")
	}
	if len(ns) != 2 {
		return fmt.Errorf("wrong FQDN")
	}
	if _, err := net.LookupHost(ns[1]); err != nil {
		return err
	}

	if e.staticOnly && port == 0 {
		return fmt.Errorf("EMPD is disabled. Port must be > 0")
	}

	e.mtx.Lock()
	defer e.mtx.Unlock()
	if _, ok := e.staticRoutes[name]; ok {
		// already exist
		return fmt.Errorf("already exist")
	}
	e.staticRoutes[name] = NetworkRoute{int(port), cookie, tls}

	return nil
}

// RemoveStaticRoute
func (e *epmd) removeStaticRoute(name string) {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	delete(e.staticRoutes, name)
	return
}

func (e *epmd) resolve(name string) (NetworkRoute, error) {
	// chech static routes first
	e.mtx.RLock()
	defer e.mtx.RUnlock()
	nr, ok := e.staticRoutes[name]
	if ok && nr.Port > 0 {
		return nr, nil
	}

	if e.staticOnly {
		return nr, fmt.Errorf("Can't resolve %s", name)
	}

	// no static route for the given name. go the regular way
	port, err := e.resolvePort(name)
	if err != nil {
		return nr, err
	}
	return NetworkRoute{port, nr.Cookie, nr.TLS}, nil
}

func (e *epmd) resolvePort(name string) (int, error) {
	ns := strings.Split(name, "@")
	if len(ns) != 2 {
		return 0, fmt.Errorf("incorrect FQDN node name (example: node@localhost)")
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(ns[1], fmt.Sprintf("%d", e.Port)))
	if err != nil {
		return 0, err
	}

	defer conn.Close()

	buf := compose_PORT_PLEASE2_REQ(ns[0])
	_, err = conn.Write(buf)
	if err != nil {
		return -1, fmt.Errorf("initiate connection - %s", err)
	}

	buf = make([]byte, 1024)
	_, err = conn.Read(buf)
	if err != nil && err != io.EOF {
		return -1, fmt.Errorf("reading from link - %s", err)
	}

	if buf[0] == EPMD_PORT2_RESP && buf[1] == 0 {
		p := binary.BigEndian.Uint16(buf[2:4])
		// we don't use all the extra info for a while. FIXME (do we need it?)
		return int(p), nil
	} else if buf[1] > 0 {
		return -1, fmt.Errorf("desired node not found")
	} else {
		return -1, fmt.Errorf("malformed reply - %#v", buf)
	}
}

func compose_ALIVE2_REQ(e *epmd) (reply []byte) {
	return
}

func read_ALIVE2_RESP(reply []byte) interface{} {
	if reply[1] == 0 {
		return binary.BigEndian.Uint16(reply[2:4])
	}
	return false
}

func compose_PORT_PLEASE2_REQ(name string) (reply []byte) {
	replylen := uint16(2 + len(name) + 1)
	reply = make([]byte, replylen)
	binary.BigEndian.PutUint16(reply[0:2], uint16(len(reply)-2))
	reply[2] = byte(EPMD_PORT_PLEASE2_REQ)
	copy(reply[3:replylen], name)
	return
}

/// empd server implementation

type nodeinfo struct {
	Port      uint16
	Hidden    bool
	HiVersion uint16
	LoVersion uint16
	Extra     []byte
}

type embeddedEPMDserver struct {
	portmap map[string]*nodeinfo
	mtx     sync.RWMutex
}

func (e *embeddedEPMDserver) Join(name string, info *nodeinfo) bool {

	e.mtx.Lock()
	defer e.mtx.Unlock()
	if _, ok := e.portmap[name]; ok {
		// already registered
		return false
	}
	lib.Log("EPMD registering node: '%s' port:%d hidden:%t", name, info.Port, info.Hidden)
	e.portmap[name] = info

	return true
}

func (e *embeddedEPMDserver) Get(name string) *nodeinfo {
	e.mtx.RLock()
	defer e.mtx.RUnlock()
	if info, ok := e.portmap[name]; ok {
		return info
	}
	return nil
}

func (e *embeddedEPMDserver) Leave(name string) {
	lib.Log("EPMD unregistering node: '%s'", name)

	e.mtx.Lock()
	delete(e.portmap, name)
	e.mtx.Unlock()
}

func (e *embeddedEPMDserver) ListAll() map[string]uint16 {
	e.mtx.Lock()
	lst := make(map[string]uint16)
	for k, v := range e.portmap {
		lst[k] = v.Port
	}
	e.mtx.Unlock()
	return lst
}

func Server(ctx context.Context, port uint16) error {

}

func (e *embeddedEPMDserver) compose_ALIVE2_RESP(req []byte) ([]byte, string) {

	hidden := false //
	if req[2] == 72 {
		hidden = true
	}

	namelen := binary.BigEndian.Uint16(req[8:10])
	name := string(req[10 : 10+namelen])

	info := nodeinfo{
		Port:      binary.BigEndian.Uint16(req[0:2]),
		Hidden:    hidden,
		HiVersion: binary.BigEndian.Uint16(req[4:6]),
		LoVersion: binary.BigEndian.Uint16(req[6:8]),
	}

	reply := make([]byte, 4)
	reply[0] = EPMD_ALIVE2_RESP

	registered := ""
	if e.Join(name, &info) {
		reply[1] = 0
		registered = name
	} else {
		reply[1] = 1
	}

	binary.BigEndian.PutUint16(reply[2:], uint16(1))
	lib.Log("Made reply for ALIVE2_REQ: (%s) %#v", name, reply)
	return reply, registered
}

func (e *embeddedEPMDserver) compose_EPMD_PORT2_RESP(req []byte) []byte {
	name := string(req)
	info := e.Get(name)

	if info == nil {
		// not found
		lib.Log("EPMD: looking for '%s'. Not found", name)
		return []byte{EPMD_PORT2_RESP, 1}
	}

	reply := make([]byte, 12+len(name)+2+len(info.Extra))
	reply[0] = EPMD_PORT2_RESP
	reply[1] = 0
	binary.BigEndian.PutUint16(reply[2:4], uint16(info.Port))
	if info.Hidden {
		reply[4] = 72
	} else {
		reply[4] = 77
	}
	reply[5] = 0 // protocol tcp
	binary.BigEndian.PutUint16(reply[6:8], uint16(info.HiVersion))
	binary.BigEndian.PutUint16(reply[8:10], uint16(info.LoVersion))
	binary.BigEndian.PutUint16(reply[10:12], uint16(len(name)))
	offset := 12 + len(name)
	copy(reply[12:offset], name)
	nELen := len(info.Extra)
	binary.BigEndian.PutUint16(reply[offset:offset+2], uint16(nELen))
	copy(reply[offset+2:offset+2+nELen], info.Extra)

	lib.Log("Made reply for EPMD_PORT_PLEASE2_REQ: %#v", reply)

	return reply
}

func (e *embeddedEPMDserver) compose_EPMD_NAMES_RESP(port uint16, req []byte) []byte {
	// io:format("name ~ts at port ~p~n", [NodeName, Port]).
	var str strings.Builder
	var s string
	var portbuf [4]byte
	binary.BigEndian.PutUint32(portbuf[0:4], uint32(port))
	str.WriteString(string(portbuf[0:]))
	for h, p := range e.ListAll() {
		s = fmt.Sprintf("name %s at port %d\n", h, p)
		str.WriteString(s)
	}

	return []byte(str.String())
}
