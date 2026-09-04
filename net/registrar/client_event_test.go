package registrar

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

func TestClientEventIsRegisteredOnce(t *testing.T) {
	_, owner, _ := startOwner(t)

	first, err := owner.Event()
	check.NoError(t, err)
	check.Equal(t, gen.Atom(registrarName+"_event"), first.Name)
	check.Equal(t, gen.Atom("node1@localhost"), first.Node)

	second, err := owner.Event()
	check.NoError(t, err)
	check.Equal(t, first, second)
}

func TestClientEventReportsARegistrationFailure(t *testing.T) {
	owner := Create(Options{}).(*client)
	owner.options.Port = 0
	failure := errors.New("no event for you")
	node := &testNode{name: "node1@localhost", log: mock.NewLog(), regErr: failure}
	if _, err := owner.Register(node, gen.RegisterRoutes{Routes: []gen.Route{{Host: "localhost", Port: 7001}}}); err != nil {
		t.Fatalf("owner register: %s", err)
	}
	t.Cleanup(owner.Terminate)

	_, err := owner.Event()
	if errors.Is(err, failure) == false {
		t.Fatalf("Event answered %v, not the failure the node reported", err)
	}
	if owner.event != "" {
		t.Fatal("a failed registration still left the event name behind")
	}
}

func TestOwnerPublishesLocalMembershipChanges(t *testing.T) {
	port, owner, ownerNode := startOwner(t)
	if _, err := owner.Event(); err != nil {
		t.Fatalf("event: %s", err)
	}

	joiner, _ := startClient(t, Options{Port: port, DisableServer: true}, "node2@localhost", 7002)

	joined, ok := ownerNode.await(t).(gen.MessageRegistrarNodeJoined)
	if ok == false {
		t.Fatalf("the owner published %#v on a registration", ownerNode.sent())
	}
	check.Equal(t, gen.Atom("node2@localhost"), joined.Name)

	joiner.Terminate()

	left, ok := ownerNode.await(t).(gen.MessageRegistrarNodeLeft)
	if ok == false {
		t.Fatalf("the owner published %#v when the link dropped", ownerNode.sent())
	}
	check.Equal(t, gen.Atom("node2@localhost"), left.Name)
}

func TestClientReceivesMembershipChangesOverTheRegistrationLink(t *testing.T) {
	port, _, _ := startOwner(t)
	remote, remoteNode := startClient(t, Options{Port: port, DisableServer: true}, "node2@localhost", 7002)
	if _, err := remote.Event(); err != nil {
		t.Fatalf("event: %s", err)
	}

	third, _ := startClient(t, Options{Port: port, DisableServer: true}, "node3@localhost", 7003)

	joined, ok := remoteNode.await(t).(gen.MessageRegistrarNodeJoined)
	if ok == false {
		t.Fatalf("the client published %#v on a registration", remoteNode.sent())
	}
	check.Equal(t, gen.Atom("node3@localhost"), joined.Name)

	third.Terminate()

	left, ok := remoteNode.await(t).(gen.MessageRegistrarNodeLeft)
	if ok == false {
		t.Fatalf("the client published %#v when node3 left", remoteNode.sent())
	}
	check.Equal(t, gen.Atom("node3@localhost"), left.Name)
}

// When the owner dies its clients race for the freed port. The promoted one runs
// the server from then on, and must keep publishing membership changes to the
// event it already handed out.
func TestPromotedClientKeepsPublishing(t *testing.T) {
	port, owner, _ := startOwner(t)
	successor, successorNode := startClient(t, Options{Port: port}, "node2@localhost", 7002)
	if _, err := successor.Event(); err != nil {
		t.Fatalf("event: %s", err)
	}

	owner.Terminate()

	promoted, ok := successorNode.await(t).(gen.MessageRegistrarNodeJoined)
	if ok == false {
		t.Fatalf("the successor published %#v when the owner died", successorNode.sent())
	}
	check.Equal(t, gen.Atom("node2@localhost"), promoted.Name)

	joiner, _ := startClient(t, Options{Port: port, DisableServer: true}, "node3@localhost", 7003)
	defer joiner.Terminate()

	joined, ok := successorNode.await(t).(gen.MessageRegistrarNodeJoined)
	if ok == false {
		t.Fatalf("the promoted client published %#v on a registration", successorNode.sent())
	}
	check.Equal(t, gen.Atom("node3@localhost"), joined.Name)
}

func TestMembershipChangeDropsTheNodesCache(t *testing.T) {
	port, owner, ownerNode := startOwner(t)
	if _, err := owner.Event(); err != nil {
		t.Fatalf("event: %s", err)
	}

	before, err := owner.Nodes()
	check.NoError(t, err)
	check.Equal(t, 0, len(before))

	startClient(t, Options{Port: port, DisableServer: true}, "node2@localhost", 7002)
	ownerNode.await(t)

	after, err := owner.Nodes()
	check.NoError(t, err)
	if len(after) != 1 || after[0] != "node2@localhost" {
		t.Fatalf("Nodes answered %v; a membership change must drop the cache", after)
	}
}
