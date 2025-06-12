package unit

import (
	"sync"

	"ergo.services/ergo/gen"
)

// TestRegistrar implements gen.Registrar interface for testing
type TestRegistrar struct {
	test     *TestActor
	resolver *TestResolver
	nodes    map[gen.Atom][]gen.Route
	proxies  map[gen.Atom][]gen.ProxyRoute
	apps     map[gen.Atom][]gen.ApplicationRoute
	config   map[string]any
	event    gen.Event
	eventRef gen.Ref
	version  gen.Version
	info     gen.RegistrarInfo
	node     gen.NodeRegistrar
	mutex    sync.RWMutex
	active   bool
}

// TestResolver implements gen.Resolver interface for testing
type TestResolver struct {
	registrar *TestRegistrar
}

func newTestRegistrar(test *TestActor) *TestRegistrar {
	registrar := &TestRegistrar{
		test:    test,
		nodes:   make(map[gen.Atom][]gen.Route),
		proxies: make(map[gen.Atom][]gen.ProxyRoute),
		apps:    make(map[gen.Atom][]gen.ApplicationRoute),
		config:  make(map[string]any),
		version: gen.Version{Name: "test-registrar", Release: "1.0", License: gen.LicenseBSL1},
		info: gen.RegistrarInfo{
			Server:                     "test-server:4499",
			EmbeddedServer:             true,
			SupportRegisterProxy:       true,
			SupportRegisterApplication: true,
			SupportConfig:              true,
			SupportEvent:               true,
			Version:                    gen.Version{Name: "test-registrar", Release: "1.0", License: gen.LicenseBSL1},
		},
		active: true,
	}
	registrar.resolver = &TestResolver{registrar: registrar}
	return registrar
}

// Registrar interface implementation

func (r *TestRegistrar) Register(node gen.NodeRegistrar, routes gen.RegisterRoutes) (gen.StaticRoutes, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.active {
		return gen.StaticRoutes{}, gen.ErrRegistrarTerminated
	}

	r.node = node
	r.nodes[node.Name()] = routes.Routes

	// Register application routes
	for _, appRoute := range routes.ApplicationRoutes {
		if existing, found := r.apps[appRoute.Name]; found {
			r.apps[appRoute.Name] = append(existing, appRoute)
		} else {
			r.apps[appRoute.Name] = []gen.ApplicationRoute{appRoute}
		}
	}

	// Register proxy routes
	for _, proxyRoute := range routes.ProxyRoutes {
		if existing, found := r.proxies[proxyRoute.To]; found {
			r.proxies[proxyRoute.To] = append(existing, proxyRoute)
		} else {
			r.proxies[proxyRoute.To] = []gen.ProxyRoute{proxyRoute}
		}
	}

	// Set up default config
	r.config["test.enabled"] = true
	r.config["test.timeout"] = 30
	r.config["test.cluster"] = "test-cluster"

	// Return empty static routes for testing
	return gen.StaticRoutes{
		Routes:  make(map[string]gen.NetworkRoute),
		Proxies: make(map[string]gen.NetworkProxyRoute),
	}, nil
}

func (r *TestRegistrar) Resolver() gen.Resolver {
	return r.resolver
}

func (r *TestRegistrar) RegisterProxy(to gen.Atom) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.active {
		return gen.ErrRegistrarTerminated
	}

	// Mock proxy registration
	proxyRoute := gen.ProxyRoute{
		To:    to,
		Proxy: r.node.Name(),
	}

	if existing, found := r.proxies[to]; found {
		r.proxies[to] = append(existing, proxyRoute)
	} else {
		r.proxies[to] = []gen.ProxyRoute{proxyRoute}
	}

	return nil
}

func (r *TestRegistrar) UnregisterProxy(to gen.Atom) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.active {
		return gen.ErrRegistrarTerminated
	}

	// Remove proxy routes for this node
	if routes, found := r.proxies[to]; found {
		var filtered []gen.ProxyRoute
		for _, route := range routes {
			if route.Proxy != r.node.Name() {
				filtered = append(filtered, route)
			}
		}
		if len(filtered) == 0 {
			delete(r.proxies, to)
		} else {
			r.proxies[to] = filtered
		}
	}

	return nil
}

func (r *TestRegistrar) RegisterApplicationRoute(route gen.ApplicationRoute) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.active {
		return gen.ErrRegistrarTerminated
	}

	if existing, found := r.apps[route.Name]; found {
		// Update existing route or add new one
		updated := false
		for i, existingRoute := range existing {
			if existingRoute.Node == route.Node {
				existing[i] = route
				updated = true
				break
			}
		}
		if !updated {
			r.apps[route.Name] = append(existing, route)
		}
	} else {
		r.apps[route.Name] = []gen.ApplicationRoute{route}
	}

	return nil
}

func (r *TestRegistrar) UnregisterApplicationRoute(name gen.Atom) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.active {
		return gen.ErrRegistrarTerminated
	}

	// Remove application routes for this node
	if routes, found := r.apps[name]; found {
		var filtered []gen.ApplicationRoute
		for _, route := range routes {
			if route.Node != r.node.Name() {
				filtered = append(filtered, route)
			}
		}
		if len(filtered) == 0 {
			delete(r.apps, name)
		} else {
			r.apps[name] = filtered
		}
	}

	return nil
}

func (r *TestRegistrar) Nodes() ([]gen.Atom, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if !r.active {
		return nil, gen.ErrRegistrarTerminated
	}

	var nodes []gen.Atom
	for name := range r.nodes {
		nodes = append(nodes, name)
	}

	return nodes, nil
}

func (r *TestRegistrar) Config(items ...string) (map[string]any, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if !r.active {
		return nil, gen.ErrRegistrarTerminated
	}

	result := make(map[string]any)

	if len(items) == 0 {
		// Return all config items
		for k, v := range r.config {
			result[k] = v
		}
	} else {
		// Return specific items
		for _, item := range items {
			if value, found := r.config[item]; found {
				result[item] = value
			}
		}
	}

	return result, nil
}

func (r *TestRegistrar) ConfigItem(item string) (any, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if !r.active {
		return nil, gen.ErrRegistrarTerminated
	}

	if value, found := r.config[item]; found {
		return value, nil
	}

	return nil, gen.ErrUnknown
}

func (r *TestRegistrar) Event() (gen.Event, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if !r.active {
		return gen.Event{}, gen.ErrRegistrarTerminated
	}

	// Create a mock event if not already set
	if r.event.Name == "" {
		r.event = gen.Event{
			Name: "test.registrar.event",
			Node: r.node.Name(),
		}
	}

	return r.event, nil
}

func (r *TestRegistrar) Info() gen.RegistrarInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return r.info
}

func (r *TestRegistrar) Terminate() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.active = false

	// Clean up resources
	r.nodes = make(map[gen.Atom][]gen.Route)
	r.proxies = make(map[gen.Atom][]gen.ProxyRoute)
	r.apps = make(map[gen.Atom][]gen.ApplicationRoute)
	r.config = make(map[string]any)
}

func (r *TestRegistrar) Version() gen.Version {
	return r.version
}

// Helper methods for testing

func (r *TestRegistrar) SetConfig(key string, value any) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.config[key] = value
}

func (r *TestRegistrar) AddNode(name gen.Atom, routes []gen.Route) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.nodes[name] = routes
}

func (r *TestRegistrar) RemoveNode(name gen.Atom) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.nodes, name)
}

// Resolver interface implementation

func (res *TestResolver) Resolve(node gen.Atom) ([]gen.Route, error) {
	res.registrar.mutex.RLock()
	defer res.registrar.mutex.RUnlock()

	if !res.registrar.active {
		return nil, gen.ErrRegistrarTerminated
	}

	if routes, found := res.registrar.nodes[node]; found {
		// Return a copy to avoid modification
		result := make([]gen.Route, len(routes))
		copy(result, routes)
		return result, nil
	}

	return nil, gen.ErrNoRoute
}

func (res *TestResolver) ResolveProxy(node gen.Atom) ([]gen.ProxyRoute, error) {
	res.registrar.mutex.RLock()
	defer res.registrar.mutex.RUnlock()

	if !res.registrar.active {
		return nil, gen.ErrRegistrarTerminated
	}

	if routes, found := res.registrar.proxies[node]; found {
		// Return a copy to avoid modification
		result := make([]gen.ProxyRoute, len(routes))
		copy(result, routes)
		return result, nil
	}

	return nil, gen.ErrNoRoute
}

func (res *TestResolver) ResolveApplication(name gen.Atom) ([]gen.ApplicationRoute, error) {
	res.registrar.mutex.RLock()
	defer res.registrar.mutex.RUnlock()

	if !res.registrar.active {
		return nil, gen.ErrRegistrarTerminated
	}

	if routes, found := res.registrar.apps[name]; found {
		// Return a copy to avoid modification
		result := make([]gen.ApplicationRoute, len(routes))
		copy(result, routes)
		return result, nil
	}

	return nil, gen.ErrNoRoute
}
