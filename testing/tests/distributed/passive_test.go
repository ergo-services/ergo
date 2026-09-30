package distributed

import (
	"net"
	"strconv"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// staticRouteTo builds a static route to the node's first acceptor.
func staticRouteTo(t *testing.T, n *stage.Node) gen.NetworkRoute {
	t.Helper()
	info, err := n.Native().Network().Info()
	check.NoError(t, err)
	if len(info.Acceptors) == 0 {
		t.Fatalf("%s runs no acceptor", n.Name())
	}
	_, sport, err := net.SplitHostPort(info.Acceptors[0].Interface)
	check.NoError(t, err)
	port, err := strconv.Atoi(sport)
	check.NoError(t, err)
	return gen.NetworkRoute{Route: gen.Route{Host: n.Name().Host(), Port: uint16(port)}}
}

// TestDistPassiveNode: a node in gen.NetworkModePassive accepts incoming connections
// like an enabled one, but announces nothing on the registrar. A peer therefore cannot
// resolve it by name and reaches it only through a static route; once connected, traffic
// flows in both directions.
func TestDistPassiveNode(t *testing.T) {
	s := stage.New(t)
	passive := s.StartNode("aaa", stage.NodeOptions{Mode: gen.NetworkModePassive})
	peer := s.StartNode("bbb")

	info, err := passive.Native().Network().Info()
	check.NoError(t, err)
	check.Equal(t, gen.NetworkModePassive, info.Mode)
	check.Equal(t, 1, len(info.Acceptors))
	check.Equal(t, "passive", info.Mode.String())

	_, err = peer.Native().Network().Route(passive.Name())
	check.ErrorIs(t, err, gen.ErrNoRoute)
	if _, err := peer.Native().Network().GetNode(passive.Name()); err == nil {
		t.Fatal("the peer reached the passive node without a static route")
	}

	check.NoError(t, peer.Native().Network().AddRoute(string(passive.Name()), staticRouteTo(t, passive), 1))
	remote, err := peer.Native().Network().GetNode(passive.Name())
	check.NoError(t, err)
	check.Equal(t, passive.Name(), remote.Name())

	pong := passive.SpawnRegister("pong", factoryCallPong, gen.ProcessOptions{})
	v, err := peer.Native().Call(gen.ProcessID{Name: "pong", Node: passive.Name()}, "ping")
	check.NoError(t, err)
	check.Equal(t, "ping", v)

	echo := peer.SpawnRegister("echo", factoryCallPong, gen.ProcessOptions{})
	v, err = passive.Native().Call(gen.ProcessID{Name: "echo", Node: peer.Name()}, "pong")
	check.NoError(t, err)
	check.Equal(t, "pong", v)
	check.True(t, pong != echo)
}
