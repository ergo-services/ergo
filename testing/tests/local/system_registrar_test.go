package local

import (
	"testing"

	"ergo.services/ergo/app/system/inspect"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/stage"
)

func registrarCall(t *testing.T, n *stage.Node, request any) any {
	t.Helper()

	target := gen.ProcessID{Name: inspect.Name, Node: n.Name()}
	result, err := n.Native().Call(target, request)
	if err != nil {
		t.Fatalf("registrar request %T: %s", request, err)
	}
	return result
}

// TestSystemRegistrarNodes: asked over the wire, a node reports its own registrar's
// picture of the cluster. That is what lets a caller map a cluster it is not part of.
func TestSystemRegistrarNodes(t *testing.T) {
	s := stage.New(t, stage.StageOptions{RegistrarFull: true})
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})
	peer := s.StartNode("peer", stage.NodeOptions{})

	result := registrarCall(t, n, inspect.RequestGetRegistrarNodes{})
	response, ok := result.(inspect.ResponseGetRegistrarNodes)
	if ok == false {
		t.Fatalf("unexpected response %T", result)
	}
	if response.Error != nil {
		t.Fatalf("listing nodes: %s", response.Error)
	}

	// whether the answer includes the asking node is up to the registrar: the
	// stage one skips itself, etcd and saturn need not. Only the peer is pinned.
	seen := map[gen.Atom]bool{}
	for _, name := range response.Nodes {
		seen[name] = true
	}
	if seen[peer.Name()] == false {
		t.Errorf("the peer is missing from %v", response.Nodes)
	}
}

// TestSystemRegistrarRoutes: resolving a peer yields the routes to reach it, and
// resolving nothing is an error rather than an empty answer.
func TestSystemRegistrarRoutes(t *testing.T) {
	s := stage.New(t, stage.StageOptions{RegistrarFull: true})
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})
	peer := s.StartNode("peer", stage.NodeOptions{})

	result := registrarCall(t, n, inspect.RequestGetRegistrarRoutes{Node: peer.Name()})
	response, ok := result.(inspect.ResponseGetRegistrarRoutes)
	if ok == false {
		t.Fatalf("unexpected response %T", result)
	}
	if response.Error != nil {
		t.Fatalf("resolving %s: %s", peer.Name(), response.Error)
	}
	if len(response.Routes) == 0 {
		t.Fatalf("no routes to %s", peer.Name())
	}
	if response.Routes[0].Port == 0 {
		t.Error("route carries no port")
	}

	empty := registrarCall(t, n, inspect.RequestGetRegistrarRoutes{})
	if empty.(inspect.ResponseGetRegistrarRoutes).Error == nil {
		t.Error("an empty node name resolved without an error")
	}
}

// TestSystemRegistrarUnsupported: a registrar that cannot list nodes must say so.
// The caller has to tell "cannot enumerate" from "cluster is empty", otherwise a
// narrowed map reads as the whole cluster.
func TestSystemRegistrarUnsupported(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	result := registrarCall(t, n, inspect.RequestGetRegistrarNodes{})
	response, ok := result.(inspect.ResponseGetRegistrarNodes)
	if ok == false {
		t.Fatalf("unexpected response %T", result)
	}
	if response.Error == nil {
		t.Errorf("a registrar without enumeration answered %v and no error", response.Nodes)
	}
}
