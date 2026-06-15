package registrar

import (
	"net"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

// testNode is a minimal gen.NodeRegistrar: the registrar client only reads Name()
// and Log() from it; everything else is a safe no-op.
type testNode struct {
	name gen.Atom
	log  gen.Log
}

func (n *testNode) Name() gen.Atom                                             { return n.name }
func (n *testNode) Creation() int64                                            { return 1 }
func (n *testNode) SetEnv(gen.Env, any)                                        {}
func (n *testNode) RegisterEvent(gen.Atom, gen.EventOptions) (gen.Ref, error)  { return gen.Ref{}, nil }
func (n *testNode) UnregisterEvent(gen.Atom) error                             { return nil }
func (n *testNode) SendEvent(gen.Atom, gen.Ref, gen.MessageOptions, any) error { return nil }
func (n *testNode) Log() gen.Log                                               { return n.log }
func (n *testNode) Stop()                                                      {}
func (n *testNode) StopWithTimeout(time.Duration)                              {}
func (n *testNode) StopForce()                                                 {}

func freePort(t *testing.T) uint16 {
	t.Helper()
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("reserve free port: %s", err)
	}
	defer l.Close()
	return uint16(l.Addr().(*net.TCPAddr).Port)
}

// startOwner starts a registrar that owns the embedded server and has node1
// registered locally. Returns the server port and the owner registrar.
func startOwner(t *testing.T) (uint16, gen.Registrar) {
	t.Helper()
	port := freePort(t)
	owner := Create(Options{Port: port})
	node := &testNode{name: "node1@localhost", log: mock.NewLog()}
	if _, err := owner.Register(node, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: 7001}}}); err != nil {
		t.Fatalf("owner register: %s", err)
	}
	t.Cleanup(owner.Terminate)
	return port, owner
}

func TestRegistrarOwnerResolvesLocally(t *testing.T) {
	_, owner := startOwner(t)

	routes, err := owner.Resolver().Resolve("node1@localhost")
	check.NoError(t, err)
	check.Equal(t, 1, len(routes))
	check.Equal(t, uint16(7001), routes[0].Port)
}

func TestRegistrarRemoteRegisterAndResolve(t *testing.T) {
	port, owner := startOwner(t)

	// a second node registers over TCP (exercises serveConn on the server)
	remote := Create(Options{Port: port, DisableServer: true})
	remoteNode := &testNode{name: "node2@localhost", log: mock.NewLog()}
	_, err := remote.Register(remoteNode, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: 7002}}})
	check.NoError(t, err)
	t.Cleanup(remote.Terminate)

	// owner sees the wire-registered node2 through its local server
	r2, err := owner.Resolver().Resolve("node2@localhost")
	check.NoError(t, err)
	check.Equal(t, uint16(7002), r2[0].Port)

	// the remote resolves node1 over UDP (exercises serveResolve and the client UDP path)
	r1, err := remote.Resolver().Resolve("node1@localhost")
	check.NoError(t, err)
	check.Equal(t, uint16(7001), r1[0].Port)
}

func TestRegistrarDuplicateNameRejected(t *testing.T) {
	port, _ := startOwner(t)

	dup := Create(Options{Port: port, DisableServer: true})
	dupNode := &testNode{name: "node1@localhost", log: mock.NewLog()}
	_, err := dup.Register(dupNode, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: 7003}}})
	// the reply error crosses the wire by message, not sentinel identity (edf encodes
	// the error field without an error cache), so match on the message.
	check.ErrorContains(t, err, gen.ErrTaken.Error())
}

func TestRegistrarResolveUnknownOverUDP(t *testing.T) {
	port, _ := startOwner(t)

	remote := Create(Options{Port: port, DisableServer: true})
	remoteNode := &testNode{name: "node2@localhost", log: mock.NewLog()}
	_, err := remote.Register(remoteNode, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: 7002}}})
	check.NoError(t, err)
	t.Cleanup(remote.Terminate)

	_, err = remote.Resolver().Resolve("nobody@localhost")
	// wire errors carry the message, not the sentinel identity (see the duplicate test).
	check.ErrorContains(t, err, gen.ErrUnknown.Error())
}
