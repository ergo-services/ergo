package distributed

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// TestDistConnect: two nodes establish a connection; each side's RemoteNode view
// reports the peer's name, version, network flags and negotiated MaxMessageSize,
// matching the peer's acceptor. Resolving an unknown node is ErrNoRoute.
func TestDistConnect(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1", stage.NodeOptions{MaxMessageSize: 567})
	n2 := s.Node("n2", stage.NodeOptions{MaxMessageSize: 765})

	// an unknown node does not resolve
	_, err := n1.Native().Network().GetNode("unknown@node")
	check.True(t, err == gen.ErrNoRoute)

	// connect n1 -> n2 (waits until both sides registered)
	remote1 := s.Connect(n1, n2)
	remote2, err := n2.Native().Network().Node(n1.Name())
	check.NoError(t, err)

	info1 := remote1.Info() // n1's view of n2
	info2 := remote2.Info() // n2's view of n1

	// each side sees the peer's identity and version
	check.Equal(t, n2.Name(), info1.Node)
	check.Equal(t, n1.Name(), info2.Node)
	check.Equal(t, n2.Native().Version(), info1.Version)
	check.Equal(t, n1.Native().Version(), info2.Version)

	// network flags match the peer's acceptor
	acc1, err := n1.Native().Network().Acceptors()
	check.NoError(t, err)
	check.Equal(t, acc1[0].NetworkFlags(), info2.NetworkFlags)
	acc2, err := n2.Native().Network().Acceptors()
	check.NoError(t, err)
	check.Equal(t, acc2[0].NetworkFlags(), info1.NetworkFlags)

	// MaxMessageSize: each node keeps its own option and sees the peer's
	check.Equal(t, 567, n1.Native().Network().MaxMessageSize())
	check.Equal(t, 765, n2.Native().Network().MaxMessageSize())
	check.Equal(t, n1.Native().Network().MaxMessageSize(), info2.MaxMessageSize)
	check.Equal(t, n2.Native().Network().MaxMessageSize(), info1.MaxMessageSize)

	// handshake / proto versions agree across the connection
	check.Equal(t, acc2[0].Info().HandshakeVersion, info1.HandshakeVersion)
	check.Equal(t, acc2[0].Info().ProtoVersion, info1.ProtoVersion)
}
