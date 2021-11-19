package dist

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ergo-services/ergo/lib"
)

type registeredNode struct {
	port   uint16
	hidden bool
	hi     uint16
	lo     uint16
	extra  []byte
}

type epmd struct {
	port       uint16
	nodes      map[string]registeredNode
	nodesMutex sync.Mutex
}

func startServerEPMD(ctx context.Context, host string, port uint16) error {
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(int(port))))
	if err != nil {
		lib.Log("Can't start embedded EPMD service: %s", err)
		return err
	}

	epmd := epmd{
		port:  port,
		nodes: make(map[string]registeredNode),
	}
	go epmd.serve(listener)
	lib.Log("Started embedded EMPD service and listen port: %d", port)

	return nil
}

func (e *epmd) serve(l net.Listener) {
	for {
		c, err := l.Accept()
		if err != nil {
			lib.Log("EPMD server stopped: %s", err.Error())
			return
		}
		lib.Log("EPMD accepted new connection from %s", c.RemoteAddr().String())
		go e.handle(c)
	}
}

func (e *epmd) handle(c net.Conn) {
	var name string
	var node registeredNode
	buf := make([]byte, 1024)

	defer c.Close()
	for {
		n, err := c.Read(buf)
		lib.Log("Request from EPMD client: %v", buf[:n])
		if err != nil {
			lib.Log("EPMD unregistering node: '%s'", name)
			e.nodesMutex.Lock()
			delete(e.nodes, name)
			e.nodesMutex.Unlock()
			return
		}
		// buf[0:1] - length
		if uint16(n-2) != binary.BigEndian.Uint16(buf[0:2]) {
			continue
		}

		switch buf[2] {
		case epmdAliveReq:
			name, node, err = e.readAliveReq(buf[3:])
			if err != nil {
				// send error and close connection
				e.sendAliveResp(c, 1)
				return
			}

			// check if node with this name is already registered
			e.nodesMutex.Lock()
			_, exist := e.nodes[name]
			e.nodesMutex.Unlock()
			if exist {
				// send error and close connection
				e.sendAliveResp(c, 1)
				return
			}

			// send alive response
			if err := e.sendAliveResp(c, 0); err != nil {
				return
			}

			// register new node
			e.nodesMutex.Lock()
			e.nodes[name] = node
			e.nodesMutex.Unlock()

			// enable keep alive on this connection
			if tcp, ok := c.(*net.TCPConn); ok {
				tcp.SetKeepAlive(true)
				tcp.SetKeepAlivePeriod(15 * time.Second)
				tcp.SetNoDelay(true)
			}
			continue
		case epmdPortPleaseReq:
			requestedName := string(buf[3:n])

			e.nodesMutex.Lock()
			node, exist := e.nodes[requestedName]
			e.nodesMutex.Unlock()

			if exist == false {
				lib.Log("EPMD: looking for '%s'. Not found", name)
				c.Write([]byte{epmdPortResp, 1})
				return
			}
			e.sendPortPleaseResp(c, requestedName, node)
			return
		case epmdNamesReq:
			e.sendNamesResp(c, buf[3:n])
			return
		default:
			lib.Log("unknown EPMD request")
			return
		}

	}
}

func (e *epmd) readAliveReq(req []byte) (string, registeredNode, error) {
	if len(req) < 10 {
		return "", registeredNode{}, fmt.Errorf("Malformed EPMD request")
	}
	// Name length
	l := binary.BigEndian.Uint16(req[8:10])
	// Name
	name := string(req[10 : 10+l])
	// Hidden
	hidden := false
	if req[2] == 72 {
		hidden = true
	}
	// node
	node := registeredNode{
		port:   binary.BigEndian.Uint16(req[0:2]),
		hidden: hidden,
		hi:     binary.BigEndian.Uint16(req[4:6]),
		lo:     binary.BigEndian.Uint16(req[6:8]),
		extra:  req[10+l:],
	}

	return name, node, nil
}

func (e *epmd) sendAliveResp(c net.Conn, code int) error {
	buf := make([]byte, 4)
	buf[0] = epmdAliveResp
	buf[1] = byte(code)

	// Creation. Ergo doesn't use it. Just for Erlang nodes.
	binary.BigEndian.PutUint16(buf[2:], uint16(1))
	_, err := c.Write(buf)
	return err
}

func (e *epmd) sendPortPleaseResp(c net.Conn, name string, node registeredNode) {
	buf := make([]byte, 12+len(name)+2+len(node.extra))
	buf[0] = epmdPortResp

	// Result 0
	buf[1] = 0
	// Port
	binary.BigEndian.PutUint16(buf[2:4], uint16(node.port))
	// Hidden
	if node.hidden {
		buf[4] = 72
	} else {
		buf[4] = 77
	}
	// Protocol TCP
	buf[5] = 0
	// Highest version
	binary.BigEndian.PutUint16(buf[6:8], uint16(node.hi))
	// Lowest version
	binary.BigEndian.PutUint16(buf[8:10], uint16(node.lo))
	// Name
	binary.BigEndian.PutUint16(buf[10:12], uint16(len(name)))
	offset := 12 + len(name)
	copy(buf[12:offset], name)
	// Extra
	l := len(node.extra)
	binary.BigEndian.PutUint16(buf[offset:offset+2], uint16(l))
	copy(buf[offset+2:offset+2+l], node.extra)
	// send
	c.Write(buf)
	return
}

func (e *epmd) sendNamesResp(c net.Conn, req []byte) {
	var str strings.Builder
	var s string
	var buf [4]byte

	binary.BigEndian.PutUint32(buf[0:4], uint32(e.port))
	str.WriteString(string(buf[0:]))

	e.nodesMutex.Lock()
	for k, v := range e.nodes {
		// io:format("name ~ts at port ~p~n", [NodeName, Port]).
		s = fmt.Sprintf("name %s at port %d\n", k, v.port)
		str.WriteString(s)
	}
	e.nodesMutex.Unlock()

	c.Write([]byte(str.String()))
	return
}
