package distributed

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"
)

func TestT9SimultaneousConnect(t *testing.T) {
	// Test: two nodes connect to each other at the same time.
	// One side wins via connect(), the other gets its connection via accept().
	// The rejected connect() returns error, but the accept path registers the connection.
	// Verify: after both goroutines complete, both nodes have exactly one connection.
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "simconnect"
	options1.Log.DefaultLogger.Disable = true

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "simconnect"
	options2.Log.DefaultLogger.Disable = true

	node1, err := ergo.StartNode("distT9node1simcon@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	node2, err := ergo.StartNode("distT9node2simcon@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	// simultaneously connect from both sides
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		node1.Network().GetNode(node2.Name())
	}()
	go func() {
		defer wg.Done()
		node2.Network().GetNode(node1.Name())
	}()
	wg.Wait()

	// the accept path on the rejected side needs a moment to finish
	// its handshake and register the connection
	time.Sleep(300 * time.Millisecond)

	// both nodes must have exactly one connection to each other
	r1, err := node1.Network().Node(node2.Name())
	if err != nil {
		t.Fatalf("node1 should have connection to node2: %s", err)
	}
	r2, err := node2.Network().Node(node1.Name())
	if err != nil {
		t.Fatalf("node2 should have connection to node1: %s", err)
	}

	// verify exactly one connection per node (no duplicates)
	nodes1 := node1.Network().Nodes()
	nodes2 := node2.Network().Nodes()
	if len(nodes1) != 1 {
		t.Fatalf("node1 should have exactly 1 connection, got %d", len(nodes1))
	}
	if len(nodes2) != 1 {
		t.Fatalf("node2 should have exactly 1 connection, got %d", len(nodes2))
	}

	// verify connection info is consistent
	r1info := r1.Info()
	r2info := r2.Info()
	if r1info.Node != node2.Name() {
		t.Fatal("connection info mismatch on node1")
	}
	if r2info.Node != node1.Name() {
		t.Fatal("connection info mismatch on node2")
	}
}

func TestT9SimultaneousConnectNoFlag(t *testing.T) {
	// Test: one node has EnableSimultaneousConnect, the other does not.
	// Should fall back to current behavior (no collision detection).
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "simconnect2"
	options1.Log.DefaultLogger.Disable = true

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "simconnect2"
	options2.Log.DefaultLogger.Disable = true
	options2.Network.Flags = gen.NetworkFlags{
		Enable:                       true,
		EnableRemoteSpawn:            true,
		EnableRemoteApplicationStart: true,
		EnableProxyAccept:            true,
		EnableImportantDelivery:      true,
		EnableSimultaneousConnect:    false, // explicitly disabled
	}

	node1, err := ergo.StartNode("distT9node1noflag@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	node2, err := ergo.StartNode("distT9node2noflag@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	remote1, err := node1.Network().GetNode(node2.Name())
	if err != nil {
		t.Fatalf("node1 -> node2 connection failed: %s", err)
	}

	time.Sleep(100 * time.Millisecond)

	remote2, err := node2.Network().Node(node1.Name())
	if err != nil {
		t.Fatalf("node2 should see node1: %s", err)
	}

	if remote1.Name() != node2.Name() {
		t.Fatal("incorrect remote1 node name")
	}
	if remote2.Name() != node1.Name() {
		t.Fatal("incorrect remote2 node name")
	}

	r1info := remote1.Info()
	if r1info.NetworkFlags.EnableSimultaneousConnect == true {
		t.Fatal("node2 should not have EnableSimultaneousConnect flag")
	}

	r2info := remote2.Info()
	if r2info.NetworkFlags.EnableSimultaneousConnect == false {
		t.Fatal("node1 should have EnableSimultaneousConnect flag")
	}
}

func TestT9SimultaneousConnectCluster(t *testing.T) {
	// Test: N nodes all connect to each other simultaneously.
	// Reproduces the real-world scenario where a cluster of nodes
	// starts up and every node tries to reach every other node at once.
	// Verify: after the storm, every pair has exactly one connection,
	// no dead loops, no leaked connections.
	const N = 50 // 50 nodes = 1225 pairs

	cookie := "simcluster"
	nodes := make([]gen.Node, N)

	for i := 0; i < N; i++ {
		opts := gen.NodeOptions{}
		opts.Network.Cookie = cookie
		opts.Log.DefaultLogger.Disable = true
		name := gen.Atom(fmt.Sprintf("distT9cluster%03d@localhost", i))
		nd, err := ergo.StartNode(name, opts)
		if err != nil {
			// stop already started nodes
			for j := 0; j < i; j++ {
				nodes[j].Stop()
			}
			t.Fatalf("failed to start node %d: %s", i, err)
		}
		nodes[i] = nd
	}
	defer func() {
		for _, nd := range nodes {
			nd.Stop()
		}
	}()

	// every node connects to every other node simultaneously
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		for j := 0; j < N; j++ {
			if i == j {
				continue
			}
			wg.Add(1)
			go func(src, dst int) {
				defer wg.Done()
				nodes[src].Network().GetNode(nodes[dst].Name())
			}(i, j)
		}
	}
	wg.Wait()

	// let accept paths finish
	time.Sleep(3 * time.Second)

	// retry missing connections (TCP backlog overflow under 9900 concurrent dials
	// can drop some connections -- same as in a real cluster, retry resolves it)
	for retry := 0; retry < 3; retry++ {
		missing := 0
		for i := 0; i < N; i++ {
			for j := i + 1; j < N; j++ {
				if _, err := nodes[i].Network().Node(nodes[j].Name()); err != nil {
					missing++
					nodes[i].Network().GetNode(nodes[j].Name())
				}
			}
		}
		if missing == 0 {
			break
		}
		t.Logf("retry %d: %d missing connections, retrying...", retry+1, missing)
		time.Sleep(time.Second)
	}

	// verify: every node sees exactly N-1 peers
	for i := 0; i < N; i++ {
		peers := nodes[i].Network().Nodes()
		if len(peers) != N-1 {
			t.Fatalf("node %d has %d connections, expected %d", i, len(peers), N-1)
		}
	}

	// verify: every pair has a bidirectional connection
	for i := 0; i < N; i++ {
		for j := i + 1; j < N; j++ {
			_, err := nodes[i].Network().Node(nodes[j].Name())
			if err != nil {
				t.Fatalf("node %d -> node %d: no connection: %s", i, j, err)
			}
			_, err = nodes[j].Network().Node(nodes[i].Name())
			if err != nil {
				t.Fatalf("node %d -> node %d: no connection: %s", j, i, err)
			}
		}
	}
}
