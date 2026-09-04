package registrar

import (
	"net"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

func TestClientReportsTheUnsupportedSurface(t *testing.T) {
	_, owner, _ := startOwner(t)

	_, err := owner.ResolveApplication("worker_app")
	check.ErrorIs(t, err, gen.ErrUnsupported)

	_, err = owner.ResolveProxy("node2@localhost")
	check.ErrorIs(t, err, gen.ErrUnsupported)

	check.ErrorIs(t, owner.RegisterProxy("node2@localhost"), gen.ErrUnsupported)
	check.ErrorIs(t, owner.UnregisterProxy("node2@localhost"), gen.ErrUnsupported)
	check.ErrorIs(t, owner.RegisterApplicationRoute(gen.ApplicationRoute{Name: "worker_app"}), gen.ErrUnsupported)
	check.ErrorIs(t, owner.UnregisterApplicationRoute("worker_app"), gen.ErrUnsupported)

	_, err = owner.Config()
	check.ErrorIs(t, err, gen.ErrUnsupported)

	_, err = owner.ConfigItem("anything")
	check.ErrorIs(t, err, gen.ErrUnsupported)
}

func TestClientVersion(t *testing.T) {
	_, owner, _ := startOwner(t)

	version := owner.Version()
	check.Equal(t, registrarName, version.Name)
	check.Equal(t, registrarRelease, version.Release)
	check.Equal(t, gen.LicenseMIT, version.License)
}

func TestClientInfoOnTheOwner(t *testing.T) {
	_, owner, _ := startOwner(t)

	info := owner.Info()
	if info.EmbeddedServer == false {
		t.Fatal("the owner does not report its embedded server")
	}
	if info.SupportEvent == false {
		t.Fatal("the owner does not report event support")
	}
	check.Equal(t, owner.server.lReg.Addr().String(), info.Server)
	check.Equal(t, registrarName, info.Version.Name)
}

// startClient registers one more node against the owner's server over TCP. With
// DisableServer cleared the client also tries to own the server, which is how a
// successor is promoted once the current owner dies.
func startClient(t *testing.T, options Options, name gen.Atom, route uint16) (*client, *testNode) {
	t.Helper()
	c := Create(options).(*client)
	node := &testNode{name: name, log: mock.NewLog(), published: make(chan any, 16)}
	if _, err := c.Register(node, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: route}}}); err != nil {
		t.Fatalf("register %s: %s", name, err)
	}
	t.Cleanup(c.Terminate)
	return c, node
}

func TestClientInfoOnAConnectedClient(t *testing.T) {
	port, _, _ := startOwner(t)
	remote, node := startClient(t, Options{Port: port, DisableServer: true}, "node2@localhost", 7002)

	if _, err := remote.Event(); err != nil {
		t.Fatalf("event: %s", err)
	}
	startClient(t, Options{Port: port, DisableServer: true}, "node3@localhost", 7003)
	node.await(t)

	info := remote.Info()
	if info.EmbeddedServer {
		t.Fatal("a client with the server disabled reports an embedded server")
	}
	if _, _, err := net.SplitHostPort(info.Server); err != nil {
		t.Fatalf("the client reports the server address %q, which is not an address: %s", info.Server, err)
	}
}

func TestClientInfoBeforeRegistration(t *testing.T) {
	c := Create(Options{}).(*client)

	info := c.Info()
	if info.EmbeddedServer {
		t.Fatal("an unregistered client reports an embedded server")
	}
	check.Equal(t, "", info.Server)
}

func TestClientRegistersInHiddenModeWithoutRoutes(t *testing.T) {
	port, owner, _ := startOwner(t)

	hidden := Create(Options{Port: port, DisableServer: true}).(*client)
	node := &testNode{name: "node2@localhost", log: mock.NewLog()}
	if _, err := hidden.Register(node, gen.RegisterRoutes{}); err != nil {
		t.Fatalf("hidden register: %s", err)
	}
	t.Cleanup(hidden.Terminate)

	if _, err := owner.Resolver().Resolve("node2@localhost"); err == nil {
		t.Fatal("a hidden node is resolvable, so it was registered after all")
	}
}

func TestClientRefusesASecondRegistration(t *testing.T) {
	_, owner, node := startOwner(t)

	_, err := owner.Register(node, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: 7009}}})
	check.ErrorContains(t, err, "already started")
}

func TestClientRefusesWorkAfterTerminate(t *testing.T) {
	_, owner, _ := startOwner(t)
	owner.Terminate()

	if _, err := owner.Resolver().Resolve("node1@localhost"); err == nil {
		t.Error("a terminated client still resolves")
	}

	_, err := owner.Nodes()
	check.ErrorIs(t, err, gen.ErrRegistrarTerminated)

	_, err = owner.Event()
	check.ErrorIs(t, err, gen.ErrRegistrarTerminated)
}

func TestClientRefusesANameWithoutHost(t *testing.T) {
	_, owner, _ := startOwner(t)

	_, err := owner.Resolver().Resolve("nohost")
	check.ErrorIs(t, err, gen.ErrIncorrect)
}
