package registrar

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
)

type Options struct {
	Port          uint16
	DisableServer bool
}

func Create(options Options) gen.Registrar {
	if options.Port == 0 {
		options.Port = defaultRegistrarPort
	}
	client := &client{
		options: options,
	}
	client.terminated.Store(true)

	edf.RegisterTypeOf(gen.Version{})
	edf.RegisterTypeOf(gen.Route{})
	edf.RegisterTypeOf(MessageRegisterRoutes{})
	edf.RegisterTypeOf(MessageRegisterReply{})
	edf.RegisterTypeOf(MessageResolveRoutes{})
	edf.RegisterTypeOf(MessageResolveReply{})
	edf.RegisterTypeOf(MessageNodes{})
	edf.RegisterTypeOf(MessageNodesReply{})
	edf.RegisterTypeOf(MessageNodeEvent{})

	return client
}

type client struct {
	node gen.NodeRegistrar

	routes []gen.Route

	options Options

	mu     sync.Mutex
	server *server
	conn   net.Conn

	// nodes cache. Every Nodes() costs one datagram per known peer host.
	nodesCache   []gen.Atom
	nodesCacheAt time.Time

	// event is registered on the first Event() call
	event      gen.Atom
	eventToken gen.Ref

	terminated atomic.Bool
}

//
// gen.Resolver interface implementation
//

func (c *client) Resolve(name gen.Atom) ([]gen.Route, error) {
	if c.terminated.Load() {
		return nil, fmt.Errorf("registrar client terminated")
	}

	c.mu.Lock()
	srv := c.server
	c.mu.Unlock()
	if srv != nil && name.Host() == c.node.Name().Host() {
		c.node.Log().Trace("resolving %s using local registrar server", name)
		return srv.resolve(name, true)
	}

	host := name.Host()
	if host == "" {
		return nil, gen.ErrIncorrect
	}
	dsn := net.JoinHostPort(host, strconv.Itoa(int(c.options.Port)))
	if lib.Verbose() {
		c.node.Log().Trace("resolving %s using registrar %s", name, dsn)
	}
	conn, err := net.Dial("udp", dsn)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// send resolve request
	rbuf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(rbuf)

	rbuf.Allocate(4)
	rbuf.B[0] = protoVersion
	rbuf.B[1] = protoResolve
	resolve := MessageResolveRoutes{
		Node: name,
	}
	if err := edf.Encode(resolve, rbuf, edf.Options{}); err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint16(rbuf.B[2:4], uint16(rbuf.Len()-4))

	if _, err := conn.Write(rbuf.B); err != nil {
		return nil, err
	}

	// wait the answer
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	if n < 4 {
		c.node.Log().Error("malformed data from the registrar")
		return nil, gen.ErrMalformed
	}
	dbuf := buf[:n]

	if dbuf[0] != protoVersion {
		c.node.Log().Error("malformed proto version in the registrar resolve reply")
		return nil, gen.ErrMalformed
	}
	if dbuf[1] != protoResolveReply {
		c.node.Log().Error("malformed resolve reply from the registrar")
		return nil, gen.ErrMalformed
	}
	l := int(binary.BigEndian.Uint16(dbuf[2:4]))
	if 4+l > len(dbuf) {
		c.node.Log().Error("malformed data in the registrar resolve reply (too long)")
		return nil, gen.ErrMalformed
	}
	v, _, err := edf.Decode(dbuf[4:4+l], edf.Options{})
	if err != nil {
		c.node.Log().Error("unable to decode resolve reply message from the registrar:", err)
		return nil, err
	}

	reply, ok := v.(MessageResolveReply)
	if ok == false {
		c.node.Log().Error("incorrect <registrar resolve reply> message: %#v", v)
		return nil, gen.ErrMalformed
	}

	if reply.Error != nil {
		return nil, reply.Error
	}
	return reply.Routes, nil
}

func (c *client) ResolveApplication(name gen.Atom) (gen.ApplicationRoutes, error) {
	return nil, gen.ErrUnsupported
}
func (c *client) ResolveProxy(node gen.Atom) ([]gen.ProxyRoute, error) {
	return nil, gen.ErrUnsupported
}

//
// gen.Registrar interface implementation
//

func (c *client) Resolver() gen.Resolver {
	return c
}

func (c *client) RegisterProxy(to gen.Atom) error {
	return gen.ErrUnsupported
}
func (c *client) UnregisterProxy(to gen.Atom) error {
	return gen.ErrUnsupported
}
func (c *client) RegisterApplicationRoute(route gen.ApplicationRoute) error {
	return gen.ErrUnsupported
}
func (c *client) UnregisterApplicationRoute(name gen.Atom) error {
	return gen.ErrUnsupported
}

// Nodes returns the other nodes known to this registrar: the ones registered on
// the ESRD server of this host, plus the ones registered on the hosts of the
// nodes this node is connected with. ESRD has no state shared between hosts, so
// a host nobody talks to stays invisible.
func (c *client) Nodes() ([]gen.Atom, error) {
	if c.terminated.Load() {
		return nil, gen.ErrRegistrarTerminated
	}

	c.mu.Lock()
	if time.Since(c.nodesCacheAt) < nodesCacheTTL {
		cached := c.nodesCache
		c.mu.Unlock()
		return cached, nil
	}
	c.mu.Unlock()

	self := c.node.Name()
	seen := map[gen.Atom]bool{self: true}
	hosts := make(map[string]bool)
	nodes := []gen.Atom{}

	collect := func(host string) {
		if host == "" || hosts[host] {
			return
		}
		hosts[host] = true
		for _, name := range c.nodesOnHost(host) {
			if seen[name] {
				continue
			}
			seen[name] = true
			nodes = append(nodes, name)
		}
	}

	collect(self.Host())
	for _, peer := range c.node.Peers() {
		collect(peer.Host())
	}

	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })

	c.mu.Lock()
	c.nodesCache = nodes
	c.nodesCacheAt = time.Now()
	c.mu.Unlock()

	return nodes, nil
}

// nodesOnHost lists the nodes registered on the ESRD server of the given host.
func (c *client) nodesOnHost(host string) []gen.Atom {
	c.mu.Lock()
	srv := c.server
	c.mu.Unlock()

	if srv != nil && host == c.node.Name().Host() {
		return srv.nodes()
	}

	dsn := net.JoinHostPort(host, strconv.Itoa(int(c.options.Port)))
	conn, err := net.Dial("udp", dsn)
	if err != nil {
		return nil
	}
	defer conn.Close()

	rbuf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(rbuf)

	rbuf.Allocate(4)
	rbuf.B[0] = protoVersion
	rbuf.B[1] = protoNodes
	if err := edf.Encode(MessageNodes{}, rbuf, edf.Options{}); err != nil {
		return nil
	}
	binary.BigEndian.PutUint16(rbuf.B[2:4], uint16(rbuf.Len()-4))

	if _, err := conn.Write(rbuf.B); err != nil {
		return nil
	}

	conn.SetReadDeadline(time.Now().Add(nodesRequestTimeout))
	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil || n < 4 {
		return nil
	}
	if buf[0] != protoVersion || buf[1] != protoNodesReply {
		return nil
	}
	l := int(binary.BigEndian.Uint16(buf[2:4]))
	if 4+l > n {
		return nil
	}
	v, _, err := edf.Decode(buf[4:4+l], edf.Options{})
	if err != nil {
		return nil
	}
	reply, ok := v.(MessageNodesReply)
	if ok == false {
		return nil
	}
	return reply.Nodes
}
func (c *client) Config(items ...string) (map[string]any, error) {
	return nil, gen.ErrUnsupported
}
func (c *client) ConfigItem(item string) (any, error) {
	return nil, gen.ErrUnsupported
}

// Event returns the event this registrar publishes membership changes to.
//
// The scope is this host: ESRD accepts registrations over loopback only, so
// gen.MessageRegistrarNodeJoined and gen.MessageRegistrarNodeLeft are emitted
// for the nodes of this machine. Nodes on other hosts are discovered by Nodes()
// and by the peer lists of the nodes already known.
func (c *client) Event() (gen.Event, error) {
	if c.terminated.Load() {
		return gen.Event{}, gen.ErrRegistrarTerminated
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.event != "" {
		return gen.Event{Name: c.event, Node: c.node.Name()}, nil
	}

	name := gen.Atom(registrarName + "_event")
	token, err := c.node.RegisterEvent(name, gen.EventOptions{Notify: false})
	if err != nil {
		return gen.Event{}, err
	}
	c.event = name
	c.eventToken = token

	if c.server != nil {
		c.server.setLocalNotify(c.publish)
	}

	return gen.Event{Name: name, Node: c.node.Name()}, nil
}

// publish emits a membership change to the local subscribers.
func (c *client) publish(name gen.Atom, joined bool) {
	c.mu.Lock()
	event := c.event
	token := c.eventToken
	c.mu.Unlock()

	if event == "" {
		return
	}

	var message any = gen.MessageRegistrarNodeLeft{Name: name}
	if joined {
		message = gen.MessageRegistrarNodeJoined{Name: name}
	}

	if err := c.node.SendEvent(event, token, gen.MessageOptions{}, message); err != nil {
		c.node.Log().Trace("(registrar) unable to send node event: %s", err)
	}

	c.mu.Lock()
	c.nodesCacheAt = time.Time{}
	c.mu.Unlock()
}
func (c *client) Info() gen.RegistrarInfo {
	c.mu.Lock()
	server := c.server
	conn := c.conn
	c.mu.Unlock()
	info := gen.RegistrarInfo{
		EmbeddedServer: server != nil,
		SupportEvent:   true,
		Version:        c.Version(),
	}
	if conn != nil {
		info.Server = conn.RemoteAddr().String()
		return info
	}
	if info.EmbeddedServer {
		info.Server = server.lReg.Addr().String()
	}
	return info
}

func (c *client) Register(node gen.NodeRegistrar, routes gen.RegisterRoutes) (gen.StaticRoutes, error) {
	var static gen.StaticRoutes

	c.node = node
	c.routes = routes.Routes

	if c.terminated.Load() == false {
		return static, fmt.Errorf("already started")
	}

	if len(c.routes) == 0 {
		// hidden mode. do not register node
		c.terminated.Store(false)
		return static, nil
	}

	// the registrar owner may be dying at this exact moment (its successor not yet
	// promoted), so the first attempt can hit a transient connection error. retry
	// those for a bounded time: a retry either binds the freed port (promotion) or
	// reconnects to the newly promoted owner. registration rejections (gen.ErrTaken)
	// are not transient and fail the node start immediately, which is how duplicate
	// node names are rejected.
	deadline := time.Now().Add(registerRetryTimeout)
	var rc net.Conn
	for {
		var err error
		rc, err = c.tryRegister()
		if err == nil {
			break
		}
		// retry connection-level failures only (owner dying/promotion window):
		// a net error (dial/read/write, including connection reset) or EOF.
		// registration rejections (gen.ErrTaken etc.) are not net errors and fail
		// the node start immediately, which is how duplicate node names are rejected.
		var ne net.Error
		transient := errors.As(err, &ne) || errors.Is(err, io.EOF)
		if transient == false || time.Now().After(deadline) {
			return static, err
		}
		c.node.Log().Trace("registrar registration transient error, retrying: %s", err)
		time.Sleep(registerRetryInterval)
	}

	c.terminated.Store(false)

	if rc != nil {
		go c.serve(rc)
	}

	return static, nil
}

func (c *client) Terminate() {
	c.terminated.Store(true)
	c.mu.Lock()
	server := c.server
	conn := c.conn
	c.mu.Unlock()
	if server != nil {
		c.node.Log().Trace("terminate registrar server")
		server.terminate()
	}
	if conn != nil {
		conn.Close()
	}
	c.node.Log().Trace("registrar client terminated")
}

// readPush reads one framed message the server pushed over the registration link.
func (c *client) readPush(conn net.Conn) error {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return err
	}
	if header[0] != protoVersion {
		return gen.ErrMalformed
	}

	l := int(binary.BigEndian.Uint16(header[2:4]))
	if l == 0 {
		return nil
	}
	body := make([]byte, l)
	if _, err := io.ReadFull(conn, body); err != nil {
		return err
	}

	if header[1] != protoNodeEvent {
		// a message this version does not know; the link stays usable
		return nil
	}

	v, _, err := edf.Decode(body, edf.Options{})
	if err != nil {
		c.node.Log().Trace("(registrar) unable to decode node event: %s", err)
		return nil
	}
	event, ok := v.(MessageNodeEvent)
	if ok == false {
		return nil
	}

	c.publish(event.Node, event.Joined)
	return nil
}

func (c *client) Version() gen.Version {
	return gen.Version{
		Name:    registrarName,
		Release: registrarRelease,
		License: gen.LicenseMIT,
	}
}

func (c *client) tryRegister() (net.Conn, error) {
	if c.options.DisableServer == false {
		srv := tryStartServer(c.options.Port, c.node.Log())
		c.mu.Lock()
		c.server = srv
		c.mu.Unlock()
		if srv != nil {
			// local registrar is started
			c.mu.Lock()
			event := c.event
			c.mu.Unlock()
			if event != "" {
				srv.setLocalNotify(c.publish)
			}
			srv.register(c.node.Name(), c.routes, nil)
			return nil, nil
		}
		c.node.Log().Trace("unable to start registrar server, run as a client only")
	}

	dialer := net.Dialer{
		KeepAlive: defaultKeepAlive,
	}
	dsn := net.JoinHostPort("localhost", strconv.Itoa(int(c.options.Port)))
	conn, err := dialer.Dial("tcp", dsn)
	if err != nil {
		return nil, err
	}

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	buf.Allocate(4)
	buf.B[0] = protoVersion
	buf.B[1] = protoRegister
	reg := MessageRegisterRoutes{
		Node:   c.node.Name(),
		Routes: c.routes,
	}
	if err := edf.Encode(reg, buf, edf.Options{}); err != nil {
		conn.Close()
		return nil, err
	}
	binary.BigEndian.PutUint16(buf.B[2:4], uint16(buf.Len()-4))

	if _, err := conn.Write(buf.B); err != nil {
		conn.Close()
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(time.Second))

	var rbuf [1024]byte
	n, err := conn.Read(rbuf[:])
	if err != nil {
		return nil, err
	}

	if n < 4 {
		c.node.Log().Error("malformed data from the registrar")
		conn.Close()
		return nil, gen.ErrMalformed
	}
	dbuf := rbuf[:n]

	if dbuf[0] != protoVersion {
		c.node.Log().Error("malformed proto version in the registrar reply")
		conn.Close()
		return nil, gen.ErrMalformed
	}
	if dbuf[1] != protoRegisterReply {
		c.node.Log().Error("malformed reply from the registrar")
		conn.Close()
		return nil, gen.ErrMalformed
	}
	l := int(binary.BigEndian.Uint16(dbuf[2:4]))
	if 4+l > len(dbuf) {
		c.node.Log().Error("malformed data in the registrar reply (too long)")
		conn.Close()
		return nil, gen.ErrMalformed
	}
	v, _, err := edf.Decode(dbuf[4:4+l], edf.Options{})
	if err != nil {
		c.node.Log().Error("unable to decode reply message from the registrar:", err)
		conn.Close()
		return nil, err
	}

	reply, ok := v.(MessageRegisterReply)
	if ok == false {
		c.node.Log().Error("incorrect <registrar reply> message: %#v", v)
		conn.Close()
		return nil, gen.ErrMalformed
	}

	if reply.Error != nil {
		return nil, reply.Error
	}

	conn.SetReadDeadline(time.Time{})
	return conn, nil
}

func (c *client) serve(conn net.Conn) {
	c.mu.Lock()
	if c.terminated.Load() {
		c.mu.Unlock()
		conn.Close()
		return
	}
	c.conn = conn
	c.mu.Unlock()

	for {
		err := c.readPush(conn)
		if c.terminated.Load() {
			return
		}
		if err == nil {
			continue
		}

		// any read error means the connection is broken
		c.node.Log().Warning("lost connection with the registrar: %s", err)
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()

		// trying to reconnect
		for {
			if c.terminated.Load() {
				return
			}
			newconn, err := c.tryRegister()
			if err != nil {
				c.node.Log().Error("unable to register node on the registrar: %s", err)
				time.Sleep(time.Second)
				continue
			}

			if newconn == nil {
				// use the local registrar server
				c.node.Log().Info("registered node on the local registrar")
				return
			}

			c.mu.Lock()
			if c.terminated.Load() {
				c.mu.Unlock()
				newconn.Close()
				return
			}
			c.conn = newconn
			c.mu.Unlock()
			conn = newconn
			c.node.Log().Info("registered node on the registrar")
			break
		}

	}
}
