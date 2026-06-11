package distributed

import (
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"
)

// TestT1RemoteSpawnAllowList is a regression for the spawn allow-list inversion:
// disabling one node must not deny every other node, and disabling the last node
// of an explicit allow-list must not silently re-open spawn to all.
func TestT1RemoteSpawnAllowList(t *testing.T) {
	opts1 := gen.NodeOptions{}
	opts1.Network.Cookie = "123"
	opts1.Log.DefaultLogger.Disable = true
	node1, err := ergo.StartNode("distSpawnACLnode1@localhost", opts1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	opts2 := gen.NodeOptions{}
	opts2.Network.Cookie = "123"
	opts2.Log.DefaultLogger.Disable = true
	node2, err := ergo.StartNode("distSpawnACLnode2@localhost", opts2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	node2.Network().EnableSpawn("tst", factoryTestServerRemoteSpawn)

	remote, err := node1.Network().GetNode(node2.Name())
	if err != nil {
		t.Fatal(err)
	}

	// open to all: node1 can spawn
	if _, err := remote.Spawn("tst", gen.ProcessOptions{}); err != nil {
		t.Fatalf("spawn (open to all) must be allowed, got: %s", err)
	}

	// disable a different node: node1 must still be allowed (the inversion would deny it)
	node2.Network().DisableSpawn("tst", gen.Atom("other@localhost"))
	if _, err := remote.Spawn("tst", gen.ProcessOptions{}); err != nil {
		t.Fatalf("disabling another node must not deny node1, got: %s", err)
	}

	// disable node1 explicitly: now denied
	node2.Network().DisableSpawn("tst", node1.Name())
	if _, err := remote.Spawn("tst", gen.ProcessOptions{}); err != gen.ErrNotAllowed {
		t.Fatalf("expected gen.ErrNotAllowed after disabling node1, got: %s", err)
	}

	// switch to an explicit allow-list that excludes node1: still denied
	node2.Network().EnableSpawn("tst", factoryTestServerRemoteSpawn, gen.Atom("other@localhost"))
	if _, err := remote.Spawn("tst", gen.ProcessOptions{}); err != gen.ErrNotAllowed {
		t.Fatalf("explicit allow-list without node1 must deny it, got: %s", err)
	}

	// disable the only allowed node: must not re-open spawn to all
	node2.Network().DisableSpawn("tst", gen.Atom("other@localhost"))
	if _, err := remote.Spawn("tst", gen.ProcessOptions{}); err != gen.ErrNotAllowed {
		t.Fatalf("disabling the last allowed node must not re-open to all, got: %s", err)
	}

	// re-enable open to all: node1 allowed again
	node2.Network().EnableSpawn("tst", factoryTestServerRemoteSpawn)
	if _, err := remote.Spawn("tst", gen.ProcessOptions{}); err != nil {
		t.Fatalf("re-enabling open to all must allow node1, got: %s", err)
	}
}
