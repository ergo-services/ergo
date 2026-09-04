package registrar

import (
	"net"
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

// testNode is a minimal gen.NodeRegistrar: the registrar client reads Name(), Log()
// and Peers() from it and publishes membership changes through RegisterEvent and
// SendEvent. Published messages are collected, and also offered on a buffered
// channel so a test can wait for one without polling.
type testNode struct {
	name   gen.Atom
	log    gen.Log
	peers  []gen.Atom
	regErr error

	mu        sync.Mutex
	events    []any
	published chan any
}

func (n *testNode) Name() gen.Atom      { return n.name }
func (n *testNode) Creation() int64     { return 1 }
func (n *testNode) Peers() []gen.Atom   { return n.peers }
func (n *testNode) SetEnv(gen.Env, any) {}
func (n *testNode) RegisterEvent(gen.Atom, gen.EventOptions) (gen.Ref, error) {
	if n.regErr != nil {
		return gen.Ref{}, n.regErr
	}
	return gen.Ref{ID: [3]uint64{7, 0, 0}}, nil
}
func (n *testNode) UnregisterEvent(gen.Atom) error { return nil }
func (n *testNode) SendEvent(name gen.Atom, token gen.Ref, options gen.MessageOptions, message any) error {
	n.mu.Lock()
	n.events = append(n.events, message)
	ch := n.published
	n.mu.Unlock()
	if ch != nil {
		select {
		case ch <- message:
		default:
		}
	}
	return nil
}
func (n *testNode) Log() gen.Log                  { return n.log }
func (n *testNode) Stop()                         {}
func (n *testNode) StopWithTimeout(time.Duration) {}
func (n *testNode) StopForce()                    {}

// sent returns the messages published so far.
func (n *testNode) sent() []any {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]any{}, n.events...)
}

// await returns the next published message, or fails the test if none arrives.
func (n *testNode) await(t *testing.T) any {
	t.Helper()
	select {
	case message := <-n.published:
		return message
	case <-time.After(5 * time.Second):
		t.Fatal("no membership change was published")
		return nil
	}
}

// startOwner starts a registrar that owns the embedded server and has node1
// registered locally. Returns the server port, the owner registrar and the node
// it registered.
func startOwner(t *testing.T) (uint16, *client, *testNode) {
	t.Helper()
	owner := Create(Options{}).(*client)
	owner.options.Port = 0
	node := &testNode{name: "node1@localhost", log: mock.NewLog(), published: make(chan any, 16)}
	if _, err := owner.Register(node, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: 7001}}}); err != nil {
		t.Fatalf("owner register: %s", err)
	}
	t.Cleanup(owner.Terminate)
	return uint16(owner.server.lReg.Addr().(*net.TCPAddr).Port), owner, node
}

func TestRegistrarOwnerResolvesLocally(t *testing.T) {
	_, owner, _ := startOwner(t)

	routes, err := owner.Resolver().Resolve("node1@localhost")
	check.NoError(t, err)
	check.Equal(t, 1, len(routes))
	check.Equal(t, uint16(7001), routes[0].Port)
}

func TestRegistrarRemoteRegisterAndResolve(t *testing.T) {
	port, owner, _ := startOwner(t)

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
	port, _, _ := startOwner(t)

	dup := Create(Options{Port: port, DisableServer: true})
	dupNode := &testNode{name: "node1@localhost", log: mock.NewLog()}
	_, err := dup.Register(dupNode, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: 7003}}})
	// the reply error crosses the wire by message, not sentinel identity (edf encodes
	// the error field without an error cache), so match on the message.
	check.ErrorContains(t, err, gen.ErrTaken.Error())
}

func TestRegistrarResolveUnknownOverUDP(t *testing.T) {
	port, _, _ := startOwner(t)

	remote := Create(Options{Port: port, DisableServer: true})
	remoteNode := &testNode{name: "node2@localhost", log: mock.NewLog()}
	_, err := remote.Register(remoteNode, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: 7002}}})
	check.NoError(t, err)
	t.Cleanup(remote.Terminate)

	_, err = remote.Resolver().Resolve("nobody@localhost")
	// wire errors carry the message, not the sentinel identity (see the duplicate test).
	check.ErrorContains(t, err, gen.ErrUnknown.Error())
}
