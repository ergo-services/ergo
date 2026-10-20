package stage

import (
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

type registrarNode struct {
	*mock.Node
	name gen.Atom
	mu   sync.Mutex
	sent []any
}

func newRegistrarNode(name gen.Atom) *registrarNode {
	n := &registrarNode{Node: mock.NewNode(), name: name}
	n.OnName(func() gen.Atom { return name })
	n.OnSendEvent(func(_ gen.Atom, _ gen.Ref, _ gen.MessageOptions, message any) error {
		n.mu.Lock()
		n.sent = append(n.sent, message)
		n.mu.Unlock()
		return nil
	})
	return n
}

func (n *registrarNode) saw(f func(any) bool) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n.mu.Lock()
		for _, m := range n.sent {
			if f(m) {
				n.mu.Unlock()
				return true
			}
		}
		n.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func registrarRoutes(port uint16) gen.RegisterRoutes {
	return gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: port}}}
}

func startTestRegistrar(t *testing.T, full bool) *registrarServer {
	s, err := startRegistrarServer(full)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.stop)
	return s
}

func registered(t *testing.T, s *registrarServer, node *registrarNode, port uint16) *registrarClient {
	c := newRegistrarClient(s.addr())
	if _, err := c.Register(node, registrarRoutes(port)); err != nil {
		t.Fatalf("register %s: %v", node.name, err)
	}
	return c
}

func appRoute(name gen.Atom, node *registrarNode, weight int) gen.ApplicationRoute {
	return gen.ApplicationRoute{Name: name, Node: node.name, Weight: weight, State: gen.ApplicationStateRunning}
}

// A name is taken by one node at a time: free again once that node is gone.
func TestRegistrarNamesAreUnique(t *testing.T) {
	s := startTestRegistrar(t, false)
	a := registered(t, s, newRegistrarNode("a@localhost"), 1)
	again := newRegistrarClient(s.addr())
	if _, err := again.Register(newRegistrarNode("a@localhost"), registrarRoutes(2)); err != gen.ErrTaken {
		t.Fatalf("duplicate name: got %v want ErrTaken", err)
	}

	b := newRegistrarClient(s.addr())
	routes, err := b.Resolve("a@localhost")
	if err != nil || routes[0].Port != 1 {
		t.Fatalf("resolve: got %v %v want port 1", routes, err)
	}
	a.Terminate()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := b.Resolve("a@localhost"); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the name was not freed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := again.Register(newRegistrarNode("a@localhost"), registrarRoutes(2)); err != nil {
		t.Fatalf("register after the owner left: %v", err)
	}
}

// The registrar of a stage without RegistrarFull: the routes of the nodes only,
// as the embedded one.
func TestRegistrarMinimalResolvesNodesOnly(t *testing.T) {
	s := startTestRegistrar(t, false)
	c := registered(t, s, newRegistrarNode("a@localhost"), 1)
	if c.Info().SupportRegisterApplication {
		t.Fatal("minimal registrar reports application support")
	}
	if _, err := c.ResolveApplication("app"); err != gen.ErrUnsupported {
		t.Fatalf("ResolveApplication: got %v want ErrUnsupported", err)
	}
	if _, err := c.Nodes(); err != gen.ErrUnsupported {
		t.Fatalf("Nodes: got %v want ErrUnsupported", err)
	}
	if _, err := c.Event(); err != gen.ErrUnsupported {
		t.Fatalf("Event: got %v want ErrUnsupported", err)
	}
	if _, err := c.Resolve("nobody@localhost"); err != gen.ErrNoRoute {
		t.Fatalf("Resolve unknown: got %v want ErrNoRoute", err)
	}
}

// The full registrar of a stage: the application routes in rotation (a negative
// weight out), the cluster without the node asking, the registrar events on every
// node, the routes of a node gone with it.
func TestRegistrarFullServesApplicationsAndEvents(t *testing.T) {
	s := startTestRegistrar(t, true)
	na := newRegistrarNode("a@localhost")
	nb := newRegistrarNode("b@localhost")
	a := registered(t, s, na, 1)
	b := registered(t, s, nb, 2)
	info := a.Info()
	if info.SupportRegisterApplication == false || info.SupportEvent == false {
		t.Fatalf("full registrar info: %+v", info)
	}
	if event, err := a.Event(); err != nil || event.Name != stageRegistrarEvent {
		t.Fatalf("Event: got %v %v", event, err)
	}
	if nodes, err := a.Nodes(); err != nil || len(nodes) != 1 || nodes[0] != "b@localhost" {
		t.Fatalf("Nodes: got %v %v want [b@localhost]", nodes, err)
	}
	if na.saw(func(m any) bool {
		j, ok := m.(gen.MessageRegistrarNodeJoined)
		return ok && j.Name == "b@localhost"
	}) == false {
		t.Fatal("a did not see b joined")
	}

	if err := a.RegisterApplicationRoute(appRoute("app", na, 10)); err != nil {
		t.Fatal(err)
	}
	if err := b.RegisterApplicationRoute(appRoute("app", nb, -1)); err != nil {
		t.Fatal(err)
	}
	resolved, err := b.ResolveApplication("app")
	if err != nil || len(resolved) != 1 || resolved[0].Node != na.name {
		t.Fatalf("ResolveApplication: got %v %v want one route on a", resolved, err)
	}
	if nb.saw(func(m any) bool {
		s, ok := m.(gen.MessageRegistrarApplicationStarted)
		return ok && s.Route.Node == na.name
	}) == false {
		t.Fatal("b did not see the application started on a")
	}

	if err := a.UnregisterApplicationRoute("app"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ResolveApplication("app"); err != gen.ErrApplicationUnknown {
		t.Fatalf("ResolveApplication after unregister: got %v want ErrApplicationUnknown", err)
	}

	b.Terminate()
	if na.saw(func(m any) bool {
		u, ok := m.(gen.MessageRegistrarApplicationUnloaded)
		return ok && u.Route.Node == nb.name
	}) == false {
		t.Fatal("a did not see the application of b unloaded")
	}
	if na.saw(func(m any) bool {
		l, ok := m.(gen.MessageRegistrarNodeLeft)
		return ok && l.Name == nb.name
	}) == false {
		t.Fatal("a did not see b left")
	}
}
