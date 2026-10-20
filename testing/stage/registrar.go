package stage

import (
	"encoding/binary"
	"io"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
)

// stageRegistrarEvent is the event name the full-mode registrar produces on each
// node; consumers obtain it via Registrar().Event() and never hardcode it.
const stageRegistrarEvent = gen.Atom("$stage_registrar")

// registrarTimeout bounds a request of a node to the registrar of the stage.
const registrarTimeout = 5 * time.Second

// The protocol of the registrar of a stage, the same in the Rust harness: a node
// keeps one TCP link with the server, its requests and their responses go over it
// by id, the registrar events are pushed over it. A frame is the length (4 bytes,
// big endian) and the message in EDF.
type messageHello struct{ Full bool }

type registerRequest struct {
	ID     uint64
	Node   gen.Atom
	Routes []gen.Route
}

type registerResponse struct {
	ID    uint64
	Error error
}

type resolveRequest struct {
	ID   uint64
	Node gen.Atom
}

type resolveResponse struct {
	ID     uint64
	Routes []gen.Route
	Error  error
}

type registerApplicationRequest struct {
	ID    uint64
	Route gen.ApplicationRoute
}

type registerApplicationResponse struct {
	ID    uint64
	Error error
}

type unregisterApplicationRequest struct {
	ID   uint64
	Name gen.Atom
}

type unregisterApplicationResponse struct {
	ID    uint64
	Error error
}

type resolveApplicationRequest struct {
	ID   uint64
	Name gen.Atom
}

type resolveApplicationResponse struct {
	ID     uint64
	Routes []gen.ApplicationRoute
	Error  error
}

type nodesRequest struct{ ID uint64 }

type nodesResponse struct {
	ID    uint64
	Nodes []gen.Atom
	Error error
}

type messageEvent struct{ Message any }

var registrarRegistry = func() *edf.Registry {
	r := edf.NewRegistry()
	if err := r.RegisterTypes([]any{
		messageHello{},
		registerRequest{},
		registerResponse{},
		resolveRequest{},
		resolveResponse{},
		registerApplicationRequest{},
		registerApplicationResponse{},
		unregisterApplicationRequest{},
		unregisterApplicationResponse{},
		resolveApplicationRequest{},
		resolveApplicationResponse{},
		nodesRequest{},
		nodesResponse{},
		messageEvent{},
	}); err != nil {
		panic(err)
	}
	return r
}()

func encodeFrame(message any) ([]byte, error) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	b.Extend(4)
	if err := edf.Encode(message, b, edf.Options{Registry: registrarRegistry}); err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint32(b.B[:4], uint32(b.Len()-4))
	return append([]byte(nil), b.B...), nil
}

func readFrame(conn net.Conn) (any, error) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}
	body := make([]byte, binary.BigEndian.Uint32(header[:]))
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}
	message, _, err := edf.Decode(body, edf.Options{Registry: registrarRegistry})
	return message, err
}

// bound is the local sentinel of the text of an error of a response, if there is one.
func bound(err error) error {
	if err == nil {
		return nil
	}
	if sentinel, found := registrarRegistry.Sentinel(err.Error()); found {
		return sentinel
	}
	return err
}

// appStateMsg maps an application route to the canonical event for its state.
func appStateMsg(route gen.ApplicationRoute) any {
	switch route.State {
	case gen.ApplicationStateLoaded:
		return gen.MessageRegistrarApplicationLoaded{Route: route}
	case gen.ApplicationStateInitializing:
		return gen.MessageRegistrarApplicationInitializing{Route: route}
	case gen.ApplicationStateStopping:
		return gen.MessageRegistrarApplicationStopping{Route: route}
	default: // Running and any other live state advertise as started
		return gen.MessageRegistrarApplicationStarted{Route: route}
	}
}

// registrarServer is the registrar of a stage: in minimal mode it tracks node routes
// only (parity with the embedded registrar), in full mode it also tracks application
// routes, reports the cluster membership and pushes the gen.MessageRegistrar* events,
// the contract etcd and saturn implement. Nodes of any implementation reach it by its
// address, so one stage can span the processes of an interop test.
type registrarServer struct {
	full     bool
	listener net.Listener

	mu     sync.Mutex
	routes map[gen.Atom][]gen.Route
	apps   map[[2]gen.Atom]gen.ApplicationRoute // app name, node
	links  map[*registrarLink]struct{}
}

type registrarLink struct {
	conn net.Conn
	wmu  sync.Mutex
	node gen.Atom
}

func (l *registrarLink) send(message any) {
	frame, err := encodeFrame(message)
	if err != nil {
		return
	}
	l.wmu.Lock()
	l.conn.Write(frame)
	l.wmu.Unlock()
}

func startRegistrarServer(full bool) (*registrarServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &registrarServer{
		full:     full,
		listener: listener,
		routes:   make(map[gen.Atom][]gen.Route),
		apps:     make(map[[2]gen.Atom]gen.ApplicationRoute),
		links:    make(map[*registrarLink]struct{}),
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s, nil
}

func (s *registrarServer) addr() string {
	return s.listener.Addr().String()
}

func (s *registrarServer) stop() {
	s.listener.Close()
	s.mu.Lock()
	for link := range s.links {
		link.conn.Close()
	}
	s.mu.Unlock()
}

func (s *registrarServer) serve(conn net.Conn) {
	link := &registrarLink{conn: conn}
	s.mu.Lock()
	s.links[link] = struct{}{}
	s.mu.Unlock()
	link.send(messageHello{Full: s.full})
	for {
		message, err := readFrame(conn)
		if err != nil {
			break
		}
		s.handle(link, message)
	}
	conn.Close()
	s.mu.Lock()
	delete(s.links, link)
	s.mu.Unlock()
	s.left(link)
}

func (s *registrarServer) broadcast(message any) {
	s.mu.Lock()
	links := make([]*registrarLink, 0, len(s.links))
	for link := range s.links {
		links = append(links, link)
	}
	s.mu.Unlock()
	for _, link := range links {
		link.send(messageEvent{Message: message})
	}
}

func (s *registrarServer) handle(link *registrarLink, message any) {
	switch m := message.(type) {
	case registerRequest:
		s.mu.Lock()
		_, taken := s.routes[m.Node]
		if taken == false {
			s.routes[m.Node] = m.Routes
			link.node = m.Node
		}
		s.mu.Unlock()
		if taken {
			link.send(registerResponse{ID: m.ID, Error: gen.ErrTaken})
			return
		}
		link.send(registerResponse{ID: m.ID})
		if s.full {
			s.broadcast(gen.MessageRegistrarNodeJoined{Name: m.Node})
		}

	case resolveRequest:
		s.mu.Lock()
		routes, found := s.routes[m.Node]
		s.mu.Unlock()
		if found == false {
			link.send(resolveResponse{ID: m.ID, Error: gen.ErrNoRoute})
			return
		}
		link.send(resolveResponse{ID: m.ID, Routes: routes})

	case registerApplicationRequest:
		if s.full == false {
			link.send(registerApplicationResponse{ID: m.ID, Error: gen.ErrUnsupported})
			return
		}
		s.mu.Lock()
		s.apps[[2]gen.Atom{m.Route.Name, m.Route.Node}] = m.Route
		s.mu.Unlock()
		link.send(registerApplicationResponse{ID: m.ID})
		s.broadcast(appStateMsg(m.Route))

	case unregisterApplicationRequest:
		if s.full == false {
			link.send(unregisterApplicationResponse{ID: m.ID, Error: gen.ErrUnsupported})
			return
		}
		s.mu.Lock()
		key := [2]gen.Atom{m.Name, link.node}
		route, found := s.apps[key]
		delete(s.apps, key)
		s.mu.Unlock()
		link.send(unregisterApplicationResponse{ID: m.ID})
		if found {
			s.broadcast(gen.MessageRegistrarApplicationUnloaded{Route: route})
		}

	case resolveApplicationRequest:
		if s.full == false {
			link.send(resolveApplicationResponse{ID: m.ID, Error: gen.ErrUnsupported})
			return
		}
		var routes []gen.ApplicationRoute
		s.mu.Lock()
		for key, route := range s.apps {
			if key[0] == m.Name && route.Weight >= 0 {
				routes = append(routes, route)
			}
		}
		s.mu.Unlock()
		if len(routes) == 0 {
			link.send(resolveApplicationResponse{ID: m.ID, Error: gen.ErrApplicationUnknown})
			return
		}
		sort.Slice(routes, func(i, j int) bool { return routes[i].Node < routes[j].Node })
		link.send(resolveApplicationResponse{ID: m.ID, Routes: routes})

	case nodesRequest:
		if s.full == false {
			link.send(nodesResponse{ID: m.ID, Error: gen.ErrUnsupported})
			return
		}
		var nodes []gen.Atom
		s.mu.Lock()
		for name := range s.routes {
			if name != link.node {
				nodes = append(nodes, name)
			}
		}
		s.mu.Unlock()
		sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })
		link.send(nodesResponse{ID: m.ID, Nodes: nodes})
	}
}

func (s *registrarServer) left(link *registrarLink) {
	if link.node == "" {
		return
	}
	var gone []gen.ApplicationRoute
	s.mu.Lock()
	if s.full {
		for key, route := range s.apps {
			if key[1] == link.node {
				gone = append(gone, route)
				delete(s.apps, key)
			}
		}
	}
	s.mu.Unlock()
	if s.full {
		for _, route := range gone {
			s.broadcast(gen.MessageRegistrarApplicationUnloaded{Route: route})
		}
		s.broadcast(gen.MessageRegistrarNodeLeft{Name: link.node})
	}
	s.mu.Lock()
	delete(s.routes, link.node)
	s.mu.Unlock()
}

// registrarClient is one node's handle onto the registrar of a stage: one TCP link,
// made at the first request, the responses matched by id, the pushed events produced
// on the node's own event in full mode.
type registrarClient struct {
	addr string

	mu      sync.Mutex
	conn    net.Conn
	full    bool
	pending map[uint64]chan any
	next    atomic.Uint64

	node     gen.NodeRegistrar
	event    gen.Event
	eventRef gen.Ref
}

var _ gen.Registrar = (*registrarClient)(nil)

func newRegistrarClient(addr string) *registrarClient {
	return &registrarClient{addr: addr, pending: make(map[uint64]chan any)}
}

func (c *registrarClient) link() (net.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn, nil
	}
	conn, err := net.DialTimeout("tcp", c.addr, registrarTimeout)
	if err != nil {
		return nil, gen.ErrNoConnection
	}
	hello, err := readFrame(conn)
	if err != nil {
		conn.Close()
		return nil, gen.ErrNoConnection
	}
	h, ok := hello.(messageHello)
	if ok == false {
		conn.Close()
		return nil, gen.ErrMalformed
	}
	c.full = h.Full
	c.conn = conn
	go c.read(conn)
	return conn, nil
}

func (c *registrarClient) read(conn net.Conn) {
	for {
		message, err := readFrame(conn)
		if err != nil {
			break
		}
		if e, ok := message.(messageEvent); ok {
			c.mu.Lock()
			node, event, token := c.node, c.event, c.eventRef
			c.mu.Unlock()
			if node != nil && event.Name != "" {
				node.SendEvent(event.Name, token, gen.MessageOptions{}, e.Message)
			}
			continue
		}
		id, found := responseID(message)
		if found == false {
			continue
		}
		c.mu.Lock()
		waiter := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if waiter != nil {
			waiter <- message
		}
	}
	c.mu.Lock()
	if c.conn == conn {
		c.conn = nil
	}
	for id, waiter := range c.pending {
		close(waiter)
		delete(c.pending, id)
	}
	c.mu.Unlock()
}

func responseID(message any) (uint64, bool) {
	switch m := message.(type) {
	case registerResponse:
		return m.ID, true
	case resolveResponse:
		return m.ID, true
	case registerApplicationResponse:
		return m.ID, true
	case unregisterApplicationResponse:
		return m.ID, true
	case resolveApplicationResponse:
		return m.ID, true
	case nodesResponse:
		return m.ID, true
	}
	return 0, false
}

func (c *registrarClient) request(build func(id uint64) any) (any, error) {
	conn, err := c.link()
	if err != nil {
		return nil, err
	}
	id := c.next.Add(1)
	waiter := make(chan any, 1)
	c.mu.Lock()
	c.pending[id] = waiter
	c.mu.Unlock()
	frame, err := encodeFrame(build(id))
	if err == nil {
		c.mu.Lock()
		_, err = conn.Write(frame)
		c.mu.Unlock()
	}
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, gen.ErrNoConnection
	}
	select {
	case message, ok := <-waiter:
		if ok == false {
			return nil, gen.ErrNoConnection
		}
		return message, nil
	case <-time.After(registrarTimeout):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, gen.ErrTimeout
	}
}

func (c *registrarClient) isFull() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.full
}

// gen.Registrar

func (c *registrarClient) Register(node gen.NodeRegistrar, routes gen.RegisterRoutes) (gen.StaticRoutes, error) {
	if len(routes.Routes) == 0 {
		// hidden mode (no acceptor): do not register the node, so peers cannot resolve
		// and dial it back. It still resolves others. Mirrors the embedded registrar client.
		return gen.StaticRoutes{}, nil
	}
	if _, err := c.link(); err != nil {
		return gen.StaticRoutes{}, err
	}
	full := c.isFull()
	var token gen.Ref
	if full {
		ref, err := node.RegisterEvent(stageRegistrarEvent, gen.EventOptions{})
		if err != nil {
			return gen.StaticRoutes{}, err
		}
		token = ref
	}
	c.mu.Lock()
	c.node = node
	if full {
		c.event = gen.Event{Name: stageRegistrarEvent, Node: node.Name()}
		c.eventRef = token
	}
	c.mu.Unlock()
	reply, err := c.request(func(id uint64) any {
		return registerRequest{ID: id, Node: node.Name(), Routes: routes.Routes}
	})
	if err == nil {
		err = bound(reply.(registerResponse).Error)
	}
	if err != nil {
		if full {
			node.UnregisterEvent(stageRegistrarEvent)
		}
		c.mu.Lock()
		c.node = nil
		c.event = gen.Event{}
		c.mu.Unlock()
		return gen.StaticRoutes{}, err
	}
	return gen.StaticRoutes{}, nil
}

func (c *registrarClient) Terminate() {
	c.mu.Lock()
	conn, node, event := c.conn, c.node, c.event
	c.conn = nil
	c.mu.Unlock()
	if conn != nil {
		conn.Close()
	}
	if node != nil && event.Name != "" {
		node.UnregisterEvent(event.Name)
	}
}

func (c *registrarClient) RegisterApplicationRoute(route gen.ApplicationRoute) error {
	reply, err := c.request(func(id uint64) any { return registerApplicationRequest{ID: id, Route: route} })
	if err != nil {
		return err
	}
	return bound(reply.(registerApplicationResponse).Error)
}

func (c *registrarClient) UnregisterApplicationRoute(name gen.Atom) error {
	reply, err := c.request(func(id uint64) any { return unregisterApplicationRequest{ID: id, Name: name} })
	if err != nil {
		return err
	}
	return bound(reply.(unregisterApplicationResponse).Error)
}

func (c *registrarClient) Event() (gen.Event, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.full == false || c.event.Name == "" {
		return gen.Event{}, gen.ErrUnsupported
	}
	return c.event, nil
}

func (c *registrarClient) Resolver() gen.Resolver { return c }

func (c *registrarClient) Info() gen.RegistrarInfo {
	full := c.isFull()
	return gen.RegistrarInfo{
		Server:                     "tcp://" + c.addr,
		SupportRegisterApplication: full,
		SupportEvent:               full,
		Version:                    c.Version(),
	}
}

func (c *registrarClient) Version() gen.Version {
	return gen.Version{Name: "stage-registrar", Release: "net", License: gen.LicenseMIT}
}

func (c *registrarClient) Nodes() ([]gen.Atom, error) {
	reply, err := c.request(func(id uint64) any { return nodesRequest{ID: id} })
	if err != nil {
		return nil, err
	}
	r := reply.(nodesResponse)
	if r.Error != nil {
		return nil, bound(r.Error)
	}
	return r.Nodes, nil
}

func (c *registrarClient) RegisterProxy(gen.Atom) error   { return gen.ErrUnsupported }
func (c *registrarClient) UnregisterProxy(gen.Atom) error { return gen.ErrUnsupported }

func (c *registrarClient) Config(...string) (map[string]any, error) { return nil, gen.ErrUnsupported }
func (c *registrarClient) ConfigItem(string) (any, error)           { return nil, gen.ErrUnsupported }

// gen.Resolver

func (c *registrarClient) Resolve(name gen.Atom) ([]gen.Route, error) {
	reply, err := c.request(func(id uint64) any { return resolveRequest{ID: id, Node: name} })
	if err != nil {
		return nil, err
	}
	r := reply.(resolveResponse)
	if r.Error != nil {
		return nil, bound(r.Error)
	}
	return r.Routes, nil
}

func (c *registrarClient) ResolveProxy(gen.Atom) ([]gen.ProxyRoute, error) {
	return nil, gen.ErrUnsupported
}

func (c *registrarClient) ResolveApplication(name gen.Atom) (gen.ApplicationRoutes, error) {
	reply, err := c.request(func(id uint64) any { return resolveApplicationRequest{ID: id, Name: name} })
	if err != nil {
		return nil, err
	}
	r := reply.(resolveApplicationResponse)
	if r.Error != nil {
		return nil, bound(r.Error)
	}
	return gen.ApplicationRoutes(r.Routes), nil
}
