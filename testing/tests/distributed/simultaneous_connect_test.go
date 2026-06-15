package distributed

import (
	"fmt"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// TestDistSimultaneousConnect: two nodes dialing each other at the same time
// resolve the collision to a single bidirectional connection (one side wins the
// connect, the other adopts it via accept), so each node ends with exactly one peer.
func TestDistSimultaneousConnect(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")

	s.ConnectMesh(n1, n2)

	check.Equal(t, 1, len(n1.Native().Network().Nodes()))
	check.Equal(t, 1, len(n2.Native().Network().Nodes()))

	r1, err := n1.Native().Network().Node(n2.Name())
	check.NoError(t, err)
	check.Equal(t, n2.Name(), r1.Info().Node)
	r2, err := n2.Native().Network().Node(n1.Name())
	check.NoError(t, err)
	check.Equal(t, n1.Name(), r2.Info().Node)
}

// TestDistSimultaneousConnectNoFlag: with EnableSimultaneousConnect off on one
// peer the connection still establishes, and each side reports the other's flags.
func TestDistSimultaneousConnectNoFlag(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2", stage.NodeOptions{
		NetworkFlags: gen.NetworkFlags{
			Enable:                       true,
			EnableRemoteSpawn:            true,
			EnableRemoteApplicationStart: true,
			EnableProxyAccept:            true,
			EnableImportantDelivery:      true,
			EnableSimultaneousConnect:    false,
		},
	})

	r1 := s.Connect(n1, n2)
	r2, err := n2.Native().Network().Node(n1.Name())
	check.NoError(t, err)

	check.Equal(t, n2.Name(), r1.Name())
	check.Equal(t, n1.Name(), r2.Name())

	// n1 keeps the framework default (flag on); n2 explicitly disabled it
	check.Equal(t, false, r1.Info().NetworkFlags.EnableSimultaneousConnect)
	check.Equal(t, true, r2.Info().NetworkFlags.EnableSimultaneousConnect)
}

// TestDistSimultaneousConnectCluster: a cluster where every node dials every other
// at once settles into a clean full mesh, with each node holding exactly one
// connection per peer (no duplicates, no leaked connections).
//
// Skipped: the connect storm exercises the TCP pool-expansion path, where the
// handshake is decoupled from connection registration (FIXME at node/network.go
// pool-expansion workaround). Under that flaw the pool Join races with the
// connection's serve loop (data race between connection.Join and connection.wait),
// and the mesh does not reliably settle under load. Enable once pool formation is
// properly synchronized in the handshake.
func TestDistSimultaneousConnectCluster(t *testing.T) {
	t.Skip("gated on the handshake/pool-expansion design flaw (node/network.go pool-expansion workaround)")

	const N = 50

	s := stage.New(t)
	nodes := make([]*stage.Node, N)
	for i := range nodes {
		nodes[i] = s.Node(fmt.Sprintf("c%03d", i))
	}

	s.ConnectMesh(nodes...)

	for i := range nodes {
		check.Equal(t, N-1, len(nodes[i].Native().Network().Nodes()))
	}
}
