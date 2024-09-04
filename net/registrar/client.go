package registrar

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
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
		options:    options,
		terminated: true,
	}

	edf.RegisterTypeOf(gen.Version{})
	edf.RegisterTypeOf(gen.Route{})
	edf.RegisterTypeOf(MessageRegisterRoutes{})
	edf.RegisterTypeOf(MessageRegisterReply{})
	edf.RegisterTypeOf(MessageResolveRoutes{})
	edf.RegisterTypeOf(MessageResolveReply{})

	return client
}

type client struct {
	node gen.NodeRegistrar

	routes []gen.Route

	options Options

	server *server
	conn   net.Conn

	terminated bool
}

//
// gen.Resolver interface implementation
//

func (c *client) Resolve(name gen.Atom) ([]gen.Route, error) {
	if c.terminated {
		return nil, fmt.Errorf("registrar client terminated")
	}

	srv := c.server
	if srv != nil {
		c.node.Log().Trace("resolving %s using local registrar server", name)
		return srv.resolve(name, true)
	}

	host := name.Host()
	if host == "" {
		return nil, gen.ErrIncorrect
	}
	dsn := net.JoinHostPort(host, strconv.Itoa(int(c.options.Port)))
	c.node.Log().Trace("resolving %s using registrar %s", name, dsn)
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
	v, _, err := edf.Decode(dbuf[4:], edf.Options{})
	if err != nil {
		c.node.Log().Error("unable to decode resolve reply message from the registrar:", err)
		return nil, err
	}

	reply, ok := v.(MessageResolveReply)
	if ok == false {
		c.node.Log().Error("incorrect <registrar resolve reply> message: %#v", v)
		return nil, err
	}

	if reply.Error != nil {
		return nil, reply.Error
	}
	return reply.Routes, nil
}

func (c *client) ResolveApplication(name gen.Atom) ([]gen.ApplicationRoute, error) {
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
func (c *client) Nodes() ([]gen.Atom, error) {
	return nil, gen.ErrUnsupported
}
func (c *client) Config(items ...string) (map[string]any, error) {
	return nil, gen.ErrUnsupported
}
func (c *client) ConfigItem(item string) (any, error) {
	return nil, gen.ErrUnsupported
}
func (c *client) Event() (gen.Event, error) {
	return gen.Event{}, gen.ErrUnsupported
}
func (c *client) Info() gen.RegistrarInfo {
	info := gen.RegistrarInfo{
		EmbeddedServer: c.server != nil,
		Version:        c.Version(),
	}
	conn := c.conn
	if conn != nil {
		info.Server = conn.RemoteAddr().String()
		return info
	}
	if info.EmbeddedServer {
		info.Server = c.server.lReg.Addr().String()
	}
	return info
}

func (c *client) Register(node gen.NodeRegistrar, routes gen.RegisterRoutes) (gen.StaticRoutes, error) {
	var static gen.StaticRoutes

	c.node = node
	c.routes = routes.Routes

	if c.terminated == false {
		return static, fmt.Errorf("already started")
	}

	if len(c.routes) == 0 {
		// hidden mode. do not register node
		c.terminated = false
		return static, nil
	}

	rc, err := c.tryRegister()
	if err != nil {
		return static, err
	}

	if rc != nil {
		go c.serve(rc)
	}

	c.terminated = false
	return static, nil
}

func (c *client) Terminate() {
	if c.server != nil {
		c.node.Log().Trace("terminate registrar server")
		c.server.terminate()
	}
	if c.conn != nil {
		c.conn.Close()
	}

	c.terminated = true
	c.node.Log().Trace("registrar client terminated")
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
		c.server = tryStartServer(c.options.Port, c.node.Log())
		if c.server != nil {
			// local registrar is started
			c.server.registerNode(c.node.Name(), c.routes, nil)
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
	v, _, err := edf.Decode(dbuf[4:], edf.Options{})
	if err != nil {
		c.node.Log().Error("unable to decode reply message from the registrar:", err)
		conn.Close()
		return nil, err
	}

	reply, ok := v.(MessageRegisterReply)
	if ok == false {
		c.node.Log().Error("incorrect <registrar reply> message: %#v", v)
		conn.Close()
		return nil, err
	}

	if reply.Error != nil {
		return nil, reply.Error
	}

	conn.SetReadDeadline(time.Time{})
	return conn, nil
}

func (c *client) serve(conn net.Conn) {
	var buf [16]byte
	c.conn = conn

	for {
		_, err := c.conn.Read(buf[:])
		if c.terminated {
			return
		}
		if err != io.EOF {
			continue
		}

		// disconnected
		c.node.Log().Warning("lost connection with the registrar")
		c.conn = nil

		// trying to reconnect
		for {
			if c.terminated {
				return
			}
			conn, err := c.tryRegister()
			if err != nil {
				c.node.Log().Error("unable to register node on the registrar: %s", err)
				time.Sleep(time.Second)
				continue
			}

			if conn == nil {
				// use the local registrar server
				c.node.Log().Info("registered node on the local registrar")
				return
			}
			c.conn = conn
			c.node.Log().Info("registered node on the registrar")
			break
		}

	}
}
