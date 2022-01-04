package node

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

	"github.com/ergo-services/ergo/lib"
)

const (
	EPMD_ALIVE2_REQ  = 120
	EPMD_ALIVE2_RESP = 121

	EPMD_PORT_PLEASE2_REQ = 122
	EPMD_PORT2_RESP       = 119

	EPMD_NAMES_REQ = 110 // $n

	EPMD_DUMP_REQ = 100 // $d
	EPMD_KILL_REQ = 107 // $k
	EPMD_STOP_REQ = 115 // $s
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

	staticOnly   bool
	staticRoutes map[string]NetworkRoute
	mtx          sync.RWMutex

	response chan interface{}
}

func (e *epmd) Init(ctx context.Context, name string, port uint16, opts Options) error {
	ns := strings.Split(name, "@")
	if len(ns) != 2 {
		return fmt.Errorf("(EMPD) FQDN for node name is required (example: node@hostname)")
	}

	e.Name = ns[0]
	e.Domain = ns[1]
	e.NodePort = port
	e.Port = opts.EPMDPort

	// http://erlang.org/doc/reference_manual/distributed.html (section 13.5)
	// // 77 — regular public node, 72 — hidden
	if opts.Hidden {
		e.Type = 72
	} else {
		e.Type = 77
	}

	e.Protocol = 0
	e.HighVsn = uint16(opts.HandshakeVersion)
	e.LowVsn = 5
	// FIXME overflows value opts.creation is uint32
	e.Creation = uint16(opts.creation)

	e.staticOnly = opts.DisableEPMD
	e.staticRoutes = make(map[string]NetworkRoute)

	ready := make(chan error)

	go func(e *epmd) {
		defer close(ready)
		for {
			if !opts.DisableEPMDServer {
				// trying to start embedded EPMD before we go further
				Server(ctx, e.Port)
			}
			dialer := net.Dialer{
				KeepAlive: 15 * time.Second,
			}
			dsn := net.JoinHostPort("", strconv.Itoa(int(e.Port)))
			conn, err := dialer.Dial("tcp", dsn)
			if err != nil {
				ready <- err
				return
			}

			conn.Write(compose_ALIVE2_REQ(e))

			for {
				buf := make([]byte, 1024)
				_, err := conn.Read(buf)
				if err != nil {
					lib.Log("EPMD: closing connection")
					conn.Close()
					break
				}

				if buf[0] == EPMD_ALIVE2_RESP {
					creation := read_ALIVE2_RESP(buf)
					switch creation {
					case false:
						ready <- fmt.Errorf("Duplicate name '%s'", e.Name)
						return
					default:
						e.Creation = creation.(uint16)
					}
					ready <- nil
				} else {
					lib.Log("Malformed EPMD reply")
					conn.Close()
					break
				}
			}

		}
	}(e)

	return <-ready
}

func (e *epmd) AddStaticRoute(name string, port uint16, cookie string, tls bool) error {
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

func (e *epmd) RemoveStaticRoute(name string) {
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
	reply = make([]byte, 2+14+len(e.Name)+len(e.Extra))
	binary.BigEndian.PutUint16(reply[0:2], uint16(len(reply)-2))
	reply[2] = byte(EPMD_ALIVE2_REQ)
	binary.BigEndian.PutUint16(reply[3:5], e.NodePort)
	reply[5] = e.Type
	reply[6] = e.Protocol
	binary.BigEndian.PutUint16(reply[7:9], e.HighVsn)
	binary.BigEndian.PutUint16(reply[9:11], e.LowVsn)
	nLen := len(e.Name)
	binary.BigEndian.PutUint16(reply[11:13], uint16(nLen))
	offset := (13 + nLen)
	copy(reply[13:offset], e.Name)
	nELen := len(e.Extra)
	binary.BigEndian.PutUint16(reply[offset:offset+2], uint16(nELen))
	copy(reply[offset+2:offset+2+nELen], e.Extra)
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

	lc := net.ListenConfig{}
	epmd, err := lc.Listen(ctx, "tcp", net.JoinHostPort("", strconv.Itoa(int(port))))
	if err != nil {
		lib.Log("Can't start embedded EPMD service: %s", err)
		return fmt.Errorf("Can't start embedded EPMD service: %s", err)

	}

	epmdServer := &embeddedEPMDserver{
		portmap: make(map[string]*nodeinfo),
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
							epmdServer.Leave(name)
						}
						return
					}
					// buf[0:1] - length
					if uint16(n-2) != binary.BigEndian.Uint16(buf[0:2]) {
						continue
					}

					switch buf[2] {
					case EPMD_ALIVE2_REQ:
						reply, registered := epmdServer.compose_ALIVE2_RESP(buf[3:n])
						c.Write(reply)
						if registered == "" {
							return
						}
						name = registered
						if tcp, ok := c.(*net.TCPConn); !ok {
							tcp.SetKeepAlive(true)
							tcp.SetKeepAlivePeriod(15 * time.Second)
							tcp.SetNoDelay(true)
						}
						continue
					case EPMD_PORT_PLEASE2_REQ:
						c.Write(epmdServer.compose_EPMD_PORT2_RESP(buf[3:n]))
						return
					case EPMD_NAMES_REQ:
						c.Write(epmdServer.compose_EPMD_NAMES_RESP(port, buf[3:n]))
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
