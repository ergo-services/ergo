package local

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type netRouteType struct {
	Field string
}

var errNetRouteTest = errors.New("net route test error")

func TestNetworkStaticRoutes(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	net := n.Native().Network()

	_, err := net.Route("worker@localhost")
	check.ErrorIs(t, err, gen.ErrNoRoute)

	route := gen.NetworkRoute{Route: gen.Route{Host: "localhost", Port: 4444}}
	check.NoError(t, net.AddRoute("worker@localhost", route, 10))
	check.ErrorIs(t, net.AddRoute("worker@localhost", route, 10), gen.ErrTaken)

	routes, err := net.Route("worker@localhost")
	check.NoError(t, err)
	check.Equal(t, 1, len(routes))
	check.Equal(t, uint16(4444), routes[0].Route.Port)
	check.True(t, routes[0].Route.HandshakeVersion.Name != "")
	check.True(t, routes[0].Route.ProtoVersion.Name != "")

	check.ErrorIs(t, net.RemoveRoute("nosuchmatch"), gen.ErrUnknown)
	check.NoError(t, net.RemoveRoute("worker@localhost"))

	_, err = net.Route("worker@localhost")
	check.ErrorIs(t, err, gen.ErrNoRoute)
}

func TestNetworkStaticProxyRoutes(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	net := n.Native().Network()

	_, err := net.ProxyRoute("worker@localhost")
	check.ErrorIs(t, err, gen.ErrNoRoute)

	proxy := gen.NetworkProxyRoute{Route: gen.ProxyRoute{To: "worker@localhost", Proxy: "gate@localhost"}}
	check.NoError(t, net.AddProxyRoute("worker@localhost", proxy, 5))
	check.ErrorIs(t, net.AddProxyRoute("worker@localhost", proxy, 5), gen.ErrTaken)

	routes, err := net.ProxyRoute("worker@localhost")
	check.NoError(t, err)
	check.Equal(t, 1, len(routes))
	check.Equal(t, gen.Atom("gate@localhost"), routes[0].Route.Proxy)

	check.ErrorIs(t, net.RemoveProxyRoute("nosuchmatch"), gen.ErrUnknown)
	check.NoError(t, net.RemoveProxyRoute("worker@localhost"))

	_, err = net.ProxyRoute("worker@localhost")
	check.ErrorIs(t, err, gen.ErrNoRoute)
}

func TestNetworkRegistration(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	net := n.Native().Network()
	typeName := "#ergo.services/ergo/testing/tests/local/netRouteType"

	check.NoError(t, net.RegisterType(netRouteType{}))
	check.NoError(t, net.RegisterType(netRouteType{}))
	check.NoError(t, net.RegisterTypes([]any{netRouteType{}}))

	check.NoError(t, net.RegisterError(errNetRouteTest))
	check.NoError(t, net.RegisterErrors([]error{errNetRouteTest}))

	check.NoError(t, net.RegisterAtom("net_route_test_atom"))
	check.NoError(t, net.RegisterAtoms([]gen.Atom{"net_route_test_atom"}))

	found := false
	for _, info := range net.RegisteredTypes() {
		if info.Name == typeName {
			found = true
		}
	}
	check.True(t, found)

	found = false
	for _, info := range net.RegisteredErrors() {
		if info.Text == errNetRouteTest.Error() {
			found = true
		}
	}
	check.True(t, found)

	found = false
	for _, info := range net.RegisteredAtoms() {
		if info.Name == "net_route_test_atom" {
			found = true
		}
	}
	check.True(t, found)

	typ, ok := net.LookupType(typeName)
	check.True(t, ok)
	check.Equal(t, "local.netRouteType", typ.String())

	_, ok = net.LookupType("nothing.registered.under.this.name")
	check.True(t, ok == false)
}

func TestNetworkInfoAndProtos(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	net := n.Native().Network()

	check.NoError(t, net.AddRoute("worker@localhost", gen.NetworkRoute{Route: gen.Route{Host: "localhost", Port: 4444}}, 1))
	check.NoError(t, net.EnableSpawn("spawnable", factoryT0, "peer@localhost"))
	check.NoError(t, net.EnableApplicationStart("startable", "peer@localhost"))

	info, err := net.Info()
	check.NoError(t, err)
	check.Equal(t, net.Mode(), info.Mode)
	check.Equal(t, net.MaxMessageSize(), info.MaxMessageSize)
	check.Equal(t, 1, len(info.Routes))
	check.True(t, info.HandshakeVersion.Name != "")
	check.True(t, info.ProtoVersion.Name != "")

	spawnFound := false
	for _, e := range info.EnabledSpawn {
		if e.Name == "spawnable" && contains(e.Nodes, gen.Atom("peer@localhost")) {
			spawnFound = true
		}
	}
	check.True(t, spawnFound)

	startFound := false
	for _, e := range info.EnabledApplicationStart {
		if e.Name == "startable" && contains(e.Nodes, gen.Atom("peer@localhost")) {
			startFound = true
		}
	}
	check.True(t, startFound)

	protos := net.Protos()
	check.True(t, len(protos) > 0)
}

func TestNetworkRemoteSpawnAndApplicationStartACL(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	net := n.Native().Network()

	check.ErrorIs(t, net.DisableSpawn("never_enabled"), gen.ErrUnknown)
	check.ErrorIs(t, net.DisableSpawn("never_enabled", "peer@localhost"), gen.ErrUnknown)
	check.ErrorIs(t, net.DisableApplicationStart("never_enabled"), gen.ErrUnknown)
	check.ErrorIs(t, net.DisableApplicationStart("never_enabled", "peer@localhost"), gen.ErrUnknown)

	check.NoError(t, net.EnableSpawn("worker", factoryT0, "a@localhost", "b@localhost"))
	check.NoError(t, net.DisableSpawn("worker", "a@localhost"))

	info, err := net.Info()
	check.NoError(t, err)
	for _, e := range info.EnabledSpawn {
		if e.Name != "worker" {
			continue
		}
		check.True(t, contains(e.Nodes, gen.Atom("a@localhost")) == false)
		check.True(t, contains(e.Nodes, gen.Atom("b@localhost")))
	}
	check.NoError(t, net.DisableSpawn("worker"))

	check.NoError(t, net.EnableApplicationStart("worker_app", "a@localhost", "b@localhost"))
	check.NoError(t, net.DisableApplicationStart("worker_app", "a@localhost"))

	info, err = net.Info()
	check.NoError(t, err)
	for _, e := range info.EnabledApplicationStart {
		if e.Name != "worker_app" {
			continue
		}
		check.True(t, contains(e.Nodes, gen.Atom("a@localhost")) == false)
		check.True(t, contains(e.Nodes, gen.Atom("b@localhost")))
	}
	check.NoError(t, net.DisableApplicationStart("worker_app"))
}

func TestNetworkResolveApplicationUnsupported(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	net := n.Native().Network()

	_, err := net.ResolveApplication("worker_app")
	check.ErrorIs(t, err, gen.ErrUnsupported)
}

func TestNetworkNodeUnknown(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	net := n.Native().Network()

	_, err := net.Node("nobody@localhost")
	check.ErrorIs(t, err, gen.ErrNoConnection)

	check.Equal(t, 0, len(net.Nodes()))
}
