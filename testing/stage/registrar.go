package stage

import (
	"sync"

	"ergo.services/ergo/gen"
)

// stageRegistrarEvent is the event name the full-mode registrar produces on each
// node; consumers obtain it via Registrar().Event() and never hardcode it.
const stageRegistrarEvent = gen.Atom("$stage_registrar")

// memStore is the per-stage in-memory registry shared by all nodes of one stage. It
// is plain discovery (no ports); real node-to-node transport stays TCP/EDF. Strict
// consistency: a Register is visible to any subsequent Resolve. The mutex here is
// infrastructure, not an actor callback.
//
// In minimal mode it tracks node routes only (parity with the embedded registrar:
// ResolveApplication/Event report ErrUnsupported). In full mode it also tracks
// application routes and produces the canonical gen.MessageRegistrar* event stream,
// matching the contract etcd/saturn implement.
type memStore struct {
	mu     sync.RWMutex
	full   bool
	routes map[gen.Atom][]gen.Route
	apps   map[gen.Atom]map[gen.Atom]gen.ApplicationRoute // app name -> node -> route
	regs   []*memRegistrar                                // live registrars, for event fan-out
}

func newMemStore(full bool) *memStore {
	return &memStore{
		full:   full,
		routes: make(map[gen.Atom][]gen.Route),
		apps:   make(map[gen.Atom]map[gen.Atom]gen.ApplicationRoute),
	}
}

// put registers node routes, rejecting a duplicate name (gen.ErrTaken) or empty routes
// (gen.ErrIncorrect) exactly like the embedded registrar, so node-name uniqueness is
// enforced. The name is freed by del on the owner's Terminate.
func (s *memStore) put(name gen.Atom, routes []gen.Route) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, taken := s.routes[name]; taken {
		return gen.ErrTaken
	}
	if len(routes) == 0 {
		return gen.ErrIncorrect
	}
	s.routes[name] = routes
	return nil
}

func (s *memStore) get(name gen.Atom) ([]gen.Route, bool) {
	s.mu.RLock()
	r, ok := s.routes[name]
	s.mu.RUnlock()
	if ok == false {
		return nil, false
	}
	out := make([]gen.Route, len(r)) // copy so the caller cannot mutate the store
	copy(out, r)
	return out, true
}

func (s *memStore) del(name gen.Atom) {
	s.mu.Lock()
	delete(s.routes, name)
	s.mu.Unlock()
}

func (s *memStore) addReg(r *memRegistrar) {
	s.mu.Lock()
	s.regs = append(s.regs, r)
	s.mu.Unlock()
}

func (s *memStore) removeReg(r *memRegistrar) {
	s.mu.Lock()
	for i, x := range s.regs {
		if x == r {
			s.regs = append(s.regs[:i], s.regs[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
}

func (s *memStore) putApp(route gen.ApplicationRoute) {
	s.mu.Lock()
	m := s.apps[route.Name]
	if m == nil {
		m = make(map[gen.Atom]gen.ApplicationRoute)
		s.apps[route.Name] = m
	}
	m[route.Node] = route
	s.mu.Unlock()
}

func (s *memStore) delApp(name, node gen.Atom) (gen.ApplicationRoute, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.apps[name]
	if m == nil {
		return gen.ApplicationRoute{}, false
	}
	route, ok := m[node]
	if ok == false {
		return gen.ApplicationRoute{}, false
	}
	delete(m, node)
	if len(m) == 0 {
		delete(s.apps, name)
	}
	return route, true
}

func (s *memStore) resolveApp(name gen.Atom) gen.ApplicationRoutes {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.apps[name]
	if len(m) == 0 {
		return nil
	}
	out := make(gen.ApplicationRoutes, 0, len(m))
	for _, r := range m {
		out = append(out, r)
	}
	return out
}

// appsOf returns the routes this node currently owns (for teardown announcements).
func (s *memStore) appsOf(node gen.Atom) []gen.ApplicationRoute {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []gen.ApplicationRoute
	for _, m := range s.apps {
		if r, ok := m[node]; ok {
			out = append(out, r)
		}
	}
	return out
}

// memRegistrar is one node's handle onto the shared store. Each node gets its own (so
// Terminate removes only that node's entries), all sharing one *memStore per stage.
// In full mode it also produces the registrar event stream on its own node.
type memRegistrar struct {
	store    *memStore
	node     gen.NodeRegistrar
	event    gen.Event
	eventRef gen.Ref
}

var _ gen.Registrar = (*memRegistrar)(nil)

// gen.Registrar

func (r *memRegistrar) Register(node gen.NodeRegistrar, routes gen.RegisterRoutes) (gen.StaticRoutes, error) {
	if err := r.store.put(node.Name(), routes.Routes); err != nil {
		return gen.StaticRoutes{}, err
	}
	// set node only on success: on a rejected register Terminate must not delete the
	// entry the existing owner of this name holds.
	r.node = node
	if r.store.full {
		ref, err := node.RegisterEvent(stageRegistrarEvent, gen.EventOptions{})
		if err != nil {
			r.store.del(node.Name())
			r.node = nil
			return gen.StaticRoutes{}, err
		}
		r.eventRef = ref
		r.event = gen.Event{Name: stageRegistrarEvent, Node: node.Name()}
		r.store.addReg(r)
		// boot ApplicationRoutes carry no State; the authoritative state-bearing routes
		// arrive via RegisterApplicationRoute as each app transitions, so they are not
		// stored here. Announce only the node joining the cluster.
		r.broadcast(gen.MessageRegistrarNodeJoined{Name: node.Name()})
	}
	return gen.StaticRoutes{}, nil
}

func (r *memRegistrar) Terminate() {
	if r.node == nil {
		return
	}
	if r.store.full {
		for _, route := range r.store.appsOf(r.node.Name()) {
			r.store.delApp(route.Name, route.Node)
			// the node is gone, so its apps are removed entirely (Unloaded), not merely
			// stopped-but-loaded; Route is a frozen snapshot as it was before removal.
			r.broadcast(gen.MessageRegistrarApplicationUnloaded{Route: route})
		}
		r.broadcast(gen.MessageRegistrarNodeLeft{Name: r.node.Name()})
		r.store.removeReg(r)
		r.node.UnregisterEvent(r.event.Name)
	}
	r.store.del(r.node.Name())
}

func (r *memRegistrar) RegisterApplicationRoute(route gen.ApplicationRoute) error {
	if r.store.full == false {
		return gen.ErrUnsupported
	}
	r.store.putApp(route)
	r.broadcast(appStateMsg(route))
	return nil
}

func (r *memRegistrar) UnregisterApplicationRoute(name gen.Atom) error {
	if r.store.full == false {
		return gen.ErrUnsupported
	}
	route, ok := r.store.delApp(name, r.node.Name())
	if ok == false {
		return nil
	}
	// UnregisterApplicationRoute is the node's full-removal signal (unregisterAppRoute),
	// so the app is Unloaded; Route is a frozen snapshot as it was before removal. A
	// plain stop returns the app to Loaded via RegisterApplicationRoute, not here.
	r.broadcast(gen.MessageRegistrarApplicationUnloaded{Route: route})
	return nil
}

func (r *memRegistrar) Event() (gen.Event, error) {
	if r.store.full == false {
		return gen.Event{}, gen.ErrUnsupported
	}
	return r.event, nil
}

func (r *memRegistrar) Resolver() gen.Resolver { return r }

func (r *memRegistrar) Info() gen.RegistrarInfo {
	return gen.RegistrarInfo{
		Server:                     "(stage in-memory)",
		EmbeddedServer:             true,
		SupportRegisterApplication: r.store.full,
		SupportEvent:               r.store.full,
		Version:                    r.Version(),
	}
}

func (r *memRegistrar) Version() gen.Version {
	return gen.Version{Name: "stage-registrar", Release: "mem", License: gen.LicenseMIT}
}

// gen.Resolver

func (r *memRegistrar) Resolve(name gen.Atom) ([]gen.Route, error) {
	if routes, ok := r.store.get(name); ok {
		return routes, nil
	}
	return nil, gen.ErrNoRoute
}

func (r *memRegistrar) ResolveProxy(gen.Atom) ([]gen.ProxyRoute, error) {
	return nil, gen.ErrUnsupported
}

func (r *memRegistrar) ResolveApplication(name gen.Atom) (gen.ApplicationRoutes, error) {
	if r.store.full == false {
		return nil, gen.ErrUnsupported
	}
	if routes := r.store.resolveApp(name); routes != nil {
		return routes, nil
	}
	return nil, gen.ErrApplicationUnknown
}

// unsupported optional features (parity with the embedded registrar)

func (r *memRegistrar) RegisterProxy(gen.Atom) error             { return gen.ErrUnsupported }
func (r *memRegistrar) UnregisterProxy(gen.Atom) error           { return gen.ErrUnsupported }
func (r *memRegistrar) Nodes() ([]gen.Atom, error)               { return nil, gen.ErrUnsupported }
func (r *memRegistrar) Config(...string) (map[string]any, error) { return nil, gen.ErrUnsupported }
func (r *memRegistrar) ConfigItem(string) (any, error)           { return nil, gen.ErrUnsupported }

// event production (full mode)

// broadcast publishes a registrar event to every live node's local subscribers, so a
// cluster change is observed cluster-wide, mirroring how each etcd/saturn client emits
// to its own node from the shared registry state.
func (r *memRegistrar) broadcast(msg any) {
	r.store.mu.RLock()
	regs := append([]*memRegistrar(nil), r.store.regs...)
	r.store.mu.RUnlock()
	for _, x := range regs {
		x.publish(msg)
	}
}

func (r *memRegistrar) publish(msg any) {
	if r.node == nil {
		return
	}
	r.node.SendEvent(r.event.Name, r.eventRef, gen.MessageOptions{}, msg)
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
