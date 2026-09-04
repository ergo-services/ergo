package registrar

import (
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
	"ergo.services/ergo/testing/check"
)

// frame builds one protocol packet: version, type, body length, encoded body.
func frame(t *testing.T, version, ptype byte, body any) []byte {
	t.Helper()
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	buf.Allocate(4)
	buf.B[0] = version
	buf.B[1] = ptype
	if err := edf.Encode(body, buf, edf.Options{}); err != nil {
		t.Fatalf("encode %#v: %s", body, err)
	}
	binary.BigEndian.PutUint16(buf.B[2:4], uint16(buf.Len()-4))
	return append([]byte{}, buf.B...)
}

func malformedFrames(t *testing.T, ptype byte, body any) []struct {
	name  string
	bytes []byte
} {
	t.Helper()
	good := frame(t, protoVersion, ptype, body)
	tooLong := append([]byte{}, good...)
	binary.BigEndian.PutUint16(tooLong[2:4], uint16(len(tooLong)))

	return []struct {
		name  string
		bytes []byte
	}{
		{"short packet", []byte{protoVersion, ptype, 0}},
		{"proto version mismatch", frame(t, protoVersion+1, ptype, body)},
		{"unknown packet type", frame(t, protoVersion, 99, body)},
		{"length past the packet", tooLong},
		{"undecodable body", []byte{protoVersion, ptype, 0, 3, 0xff, 0xfe, 0xfd}},
	}
}

func TestServerIgnoresMalformedRegistrations(t *testing.T) {
	port, _, _ := startOwner(t)

	cases := malformedFrames(t, protoRegister, MessageRegisterRoutes{
		Node:   gen.Atom("node2@localhost"),
		Routes: []gen.Route{{Host: "localhost", Port: 7002}},
	})
	cases = append(cases, struct {
		name  string
		bytes []byte
	}{"unexpected message", frame(t, protoVersion, protoRegister, MessageNodes{})})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := net.Dial("tcp", net.JoinHostPort("localhost", strconv.Itoa(int(port))))
			if err != nil {
				t.Fatalf("dial: %s", err)
			}
			defer conn.Close()

			if _, err := conn.Write(tc.bytes); err != nil {
				t.Fatalf("write: %s", err)
			}
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			if _, err := conn.Read(make([]byte, 64)); err != io.EOF {
				t.Fatalf("the server answered %v instead of dropping the connection", err)
			}
		})
	}
}

func TestServerIgnoresMalformedResolveRequests(t *testing.T) {
	port, _, _ := startOwner(t)

	cases := malformedFrames(t, protoResolve, MessageResolveRoutes{Node: "node1@localhost"})
	cases = append(cases, struct {
		name  string
		bytes []byte
	}{"unexpected message", frame(t, protoVersion, protoResolve, MessageRegisterRoutes{Node: "node2@localhost"})})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := net.Dial("udp", net.JoinHostPort("localhost", strconv.Itoa(int(port))))
			if err != nil {
				t.Fatalf("dial: %s", err)
			}
			defer conn.Close()

			if _, err := conn.Write(tc.bytes); err != nil {
				t.Fatalf("write: %s", err)
			}
			conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
			if n, err := conn.Read(make([]byte, 1024)); err == nil {
				t.Fatalf("the server answered %d bytes to a malformed datagram", n)
			}
		})
	}
}

func TestServerStillServesAfterMalformedInput(t *testing.T) {
	port, _, _ := startOwner(t)

	conn, err := net.Dial("udp", net.JoinHostPort("localhost", strconv.Itoa(int(port))))
	if err != nil {
		t.Fatalf("dial: %s", err)
	}
	if _, err := conn.Write([]byte{protoVersion, protoResolve, 0, 3, 0xff, 0xfe, 0xfd}); err != nil {
		t.Fatalf("write: %s", err)
	}
	conn.Close()

	remote, _ := startClient(t, Options{Port: port, DisableServer: true}, "node2@localhost", 7002)
	routes, err := remote.Resolver().Resolve("node1@localhost")
	check.NoError(t, err)
	check.Equal(t, uint16(7001), routes[0].Port)
}

func TestNodesOverTheOwnersServer(t *testing.T) {
	port, owner, _ := startOwner(t)
	startClient(t, Options{Port: port, DisableServer: true}, "node3@localhost", 7003)
	startClient(t, Options{Port: port, DisableServer: true}, "node2@localhost", 7002)

	nodes, err := owner.Nodes()
	check.NoError(t, err)
	if len(nodes) != 2 || nodes[0] != "node2@localhost" || nodes[1] != "node3@localhost" {
		t.Fatalf("the owner lists %v; it must list the other nodes, sorted, without itself", nodes)
	}
}

func TestNodesOverUDP(t *testing.T) {
	port, _, _ := startOwner(t)
	remote, _ := startClient(t, Options{Port: port, DisableServer: true}, "node2@localhost", 7002)

	nodes, err := remote.Nodes()
	check.NoError(t, err)
	if len(nodes) != 1 || nodes[0] != "node1@localhost" {
		t.Fatalf("the client lists %v; it must see node1 over the resolver socket", nodes)
	}
}

func TestNodesAreCachedForTheTTL(t *testing.T) {
	port, owner, _ := startOwner(t)

	first, err := owner.Nodes()
	check.NoError(t, err)
	check.Equal(t, 0, len(first))

	startClient(t, Options{Port: port, DisableServer: true}, "node2@localhost", 7002)

	again, err := owner.Nodes()
	check.NoError(t, err)
	if len(again) != 0 {
		t.Fatalf("the second call answered %v; within the TTL it must reuse the cache", again)
	}
}

// Nodes() also asks the hosts of the peers this node talks to. Each host is asked
// once, and a host with no registrar contributes nothing.
func TestNodesAsksThePeerHostsOnce(t *testing.T) {
	port, owner, node := startOwner(t)
	startClient(t, Options{Port: port, DisableServer: true}, "node2@localhost", 7002)
	node.peers = []gen.Atom{"node2@localhost", "node9@nonexistent.invalid"}

	nodes, err := owner.Nodes()
	check.NoError(t, err)
	if len(nodes) != 1 || nodes[0] != "node2@localhost" {
		t.Fatalf("the owner lists %v; a repeated host and a host with no registrar must add nothing", nodes)
	}
}

func TestServerNodesAndLocalNotify(t *testing.T) {
	s := newTestServer()
	changes := []gen.Atom{}
	s.setLocalNotify(func(name gen.Atom, joined bool) {
		if joined == false {
			name = name + "(left)"
		}
		changes = append(changes, name)
	})

	check.NoError(t, s.register("n1@localhost", []gen.Route{{Port: 1}}, nil))
	check.NoError(t, s.register("n2@localhost", []gen.Route{{Port: 2}}, nil))
	s.unregister("n1@localhost", nil)

	nodes := s.nodes()
	if len(nodes) != 1 || nodes[0] != "n2@localhost" {
		t.Fatalf("the server holds %v after n1 left", nodes)
	}

	want := []gen.Atom{"n1@localhost", "n2@localhost", "n1@localhost(left)"}
	if len(changes) != len(want) {
		t.Fatalf("the owner was notified of %v, not of %v", changes, want)
	}
	for i := range want {
		if changes[i] != want[i] {
			t.Fatalf("the owner was notified of %v, not of %v", changes, want)
		}
	}
}

func TestServerRegisterRejectionIsNotNotified(t *testing.T) {
	s := newTestServer()
	changes := 0
	s.setLocalNotify(func(name gen.Atom, joined bool) { changes++ })

	check.NoError(t, s.register("n@localhost", []gen.Route{{Port: 1}}, nil))
	check.ErrorIs(t, s.register("n@localhost", []gen.Route{{Port: 2}}, nil), gen.ErrTaken)

	check.Equal(t, 1, changes)
}

func TestServerNotifySkipsTheNodeItIsAbout(t *testing.T) {
	port, _, _ := startOwner(t)
	watcher, watcherNode := startClient(t, Options{Port: port, DisableServer: true}, "node2@localhost", 7002)
	if _, err := watcher.Event(); err != nil {
		t.Fatalf("event: %s", err)
	}

	startClient(t, Options{Port: port, DisableServer: true}, "node3@localhost", 7003)
	watcherNode.await(t)

	for _, message := range watcherNode.sent() {
		joined, ok := message.(gen.MessageRegistrarNodeJoined)
		if ok && joined.Name == "node2@localhost" {
			t.Fatal("the client was told about its own registration")
		}
	}
}
