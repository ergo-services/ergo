package stage

import (
	"sync"

	"ergo.services/ergo/gen"
)

// memStore is the per-stage in-memory route registry shared by all nodes of one
// stage. It is plain discovery (node name -> routes); the actual node-to-node
// transport stays real TCP/EDF. Strict consistency: a Register is visible to any
// subsequent Resolve. The mutex here is infrastructure, not an actor callback.
type memStore struct {
	mu     sync.RWMutex
	routes map[gen.Atom][]gen.Route
}

func newMemStore() *memStore {
	return &memStore{routes: make(map[gen.Atom][]gen.Route)}
}

// put registers routes under name, rejecting a duplicate name (gen.ErrTaken) or
// empty routes (gen.ErrIncorrect) exactly like the embedded registrar, so node-name
// uniqueness is enforced. The name is freed by del on the owner's Terminate.
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

// memRegistrar is one node's handle onto the shared store. Each node gets its own
// (so Terminate removes only that node's entry), all sharing one *memStore per
// stage. Mirrors the embedded registrar model: per-node client, shared server -
// here the "server" is just a map.
type memRegistrar struct {
	store *memStore
	node  gen.NodeRegistrar
}

var _ gen.Registrar = (*memRegistrar)(nil)

// gen.Registrar

func (r *memRegistrar) Register(node gen.NodeRegistrar, routes gen.RegisterRoutes) (gen.StaticRoutes, error) {
	if err := r.store.put(node.Name(), routes.Routes); err != nil {
		return gen.StaticRoutes{}, err
	}
	// set node only on success: on a rejected register Terminate must not delete the
	// entry that the existing owner of this name holds.
	r.node = node
	return gen.StaticRoutes{}, nil
}

func (r *memRegistrar) Terminate() {
	if r.node != nil {
		r.store.del(r.node.Name())
	}
}

func (r *memRegistrar) Resolver() gen.Resolver { return r }

func (r *memRegistrar) Info() gen.RegistrarInfo {
	return gen.RegistrarInfo{
		Server:         "(stage in-memory)",
		EmbeddedServer: true,
		Version:        r.Version(),
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

func (r *memRegistrar) ResolveApplication(gen.Atom) (gen.ApplicationRoutes, error) {
	return nil, gen.ErrUnsupported
}

// unsupported optional features (parity with the embedded registrar)

func (r *memRegistrar) RegisterProxy(gen.Atom) error                        { return gen.ErrUnsupported }
func (r *memRegistrar) UnregisterProxy(gen.Atom) error                      { return gen.ErrUnsupported }
func (r *memRegistrar) RegisterApplicationRoute(gen.ApplicationRoute) error { return gen.ErrUnsupported }
func (r *memRegistrar) UnregisterApplicationRoute(gen.Atom) error           { return gen.ErrUnsupported }
func (r *memRegistrar) Nodes() ([]gen.Atom, error)                          { return nil, gen.ErrUnsupported }
func (r *memRegistrar) Config(...string) (map[string]any, error)            { return nil, gen.ErrUnsupported }
func (r *memRegistrar) ConfigItem(string) (any, error)                      { return nil, gen.ErrUnsupported }
func (r *memRegistrar) Event() (gen.Event, error)                           { return gen.Event{}, gen.ErrUnsupported }
