package local

import (
	"slices"
	"strings"
	"testing"

	"ergo.services/ergo/app/system/inspect"
	"ergo.services/ergo/app/system/manage"
	"ergo.services/ergo/app/system/runtime/golang"
	"ergo.services/ergo/app/system/runtime/rust"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// TestSystemRuntimeCapabilities: a Go node announces the requests of its own
// runtime and none of the Rust runtime, whose types it still registers.
func TestSystemRuntimeCapabilities(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	caps := capabilitiesOf(t, n)
	for _, c := range golang.Capabilities() {
		if slices.Contains(caps.Capabilities, c) == false {
			t.Errorf("a Go node does not announce %s", c)
		}
	}
	for _, c := range caps.Capabilities {
		if strings.HasPrefix(c, "inspect.rust.") || strings.HasPrefix(c, "manage.rust.") {
			t.Errorf("a Go node announces %s of the Rust runtime", c)
		}
	}
}

// TestSystemRuntimeRustRequestsAreUnsupported: a request of the Rust runtime
// decodes on a Go node and is answered with gen.ErrUnsupported.
func TestSystemRuntimeRustRequestsAreUnsupported(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	metrics, ok := inspectGet(t, n, rust.RequestGetRuntimeMetrics{}).(rust.ResponseGetRuntimeMetrics)
	check.True(t, ok)
	check.ErrorIs(t, metrics.Error, gen.ErrUnsupported)

	state, ok := inspectGet(t, n, rust.RequestGetRuntimeState{}).(rust.ResponseGetRuntimeState)
	check.True(t, ok)
	check.ErrorIs(t, state.Error, gen.ErrUnsupported)

	summary, ok := inspectGet(t, n, rust.RequestGetAtomSummary{}).(rust.ResponseGetAtomSummary)
	check.True(t, ok)
	check.ErrorIs(t, summary.Error, gen.ErrUnsupported)

	table, ok := inspectGet(t, n, rust.RequestGetAtomTable{}).(rust.ResponseGetAtomTable)
	check.True(t, ok)
	check.ErrorIs(t, table.Error, gen.ErrUnsupported)

	check.ErrorIs(t, callError(t, n, inspect.Name, rust.RequestInspectRuntime{}), gen.ErrUnsupported)
	check.ErrorIs(t, callError(t, n, manage.Name, rust.RequestDoSetNodeAtomLimit{Limit: 1}), gen.ErrUnsupported)
	check.ErrorIs(t, callError(t, n, manage.Name, rust.RequestDoSetPeerAtomLimit{Limit: 1}), gen.ErrUnsupported)
}

// callError is the error a call ends with: the error itself or the error it
// was answered with.
func callError(t *testing.T, n *stage.Node, process gen.Atom, request any) error {
	t.Helper()

	result, err := n.Native().Call(gen.ProcessID{Name: process, Node: n.Name()}, request)
	if err != nil {
		return err
	}
	answer, ok := result.(error)
	if ok == false {
		t.Fatalf("%T answered with %#v, not an error", request, result)
	}
	return answer
}

// TestSystemRuntimeInspectNode: the node answers with the version of its
// runtime and its build as golang.BuildInfo.
func TestSystemRuntimeInspectNode(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	response, ok := inspectGet(t, n, inspect.RequestInspectNode{}).(inspect.ResponseInspectNode)
	if ok == false {
		t.Fatal("unexpected response to RequestInspectNode")
	}
	if strings.HasPrefix(response.RuntimeVersion, "go") == false {
		t.Errorf("RuntimeVersion of a Go node is %q", response.RuntimeVersion)
	}
	if _, ok := response.Build.(golang.BuildInfo); ok == false {
		t.Errorf("Build of a Go node is %T", response.Build)
	}
}

// TestSystemRuntimeMetrics: the counters the profiler draws its GC pressure
// from come with the metrics of the Go runtime: cumulative, and nothing can have
// been freed that was not allocated first.
func TestSystemRuntimeMetrics(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	const allocs, frees = "/gc/heap/allocs:objects", "/gc/heap/frees:objects"
	response, ok := inspectGet(t, n, golang.RequestGetRuntimeMetrics{
		Names: []string{allocs, frees},
	}).(golang.ResponseGetRuntimeMetrics)
	if ok == false {
		t.Fatal("unexpected response to RequestGetRuntimeMetrics")
	}
	check.NoError(t, response.Error)

	values := map[string]uint64{}
	for _, sample := range response.Samples {
		values[sample.Name] = sample.Uint64
	}
	check.True(t, values[allocs] > 0)
	check.True(t, values[frees] <= values[allocs])
}
