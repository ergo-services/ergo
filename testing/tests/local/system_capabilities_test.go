package local

import (
	"slices"
	"testing"

	"ergo.services/ergo/app/system/inspect"
	"ergo.services/ergo/app/system/manage"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/stage"
)

func capabilitiesOf(t *testing.T, n *stage.Node) inspect.ResponseGetCapabilities {
	t.Helper()

	target := gen.ProcessID{Name: inspect.Name, Node: n.Name()}
	result, err := n.Native().Call(target, inspect.RequestGetCapabilities{})
	if err != nil {
		t.Fatalf("capabilities request: %s", err)
	}
	response, ok := result.(inspect.ResponseGetCapabilities)
	if ok == false {
		t.Fatalf("unexpected response %T", result)
	}
	return response
}

// TestSystemCapabilities: the node reports what it can do, and the report names
// the mutating plane only while that plane is actually up. A consumer keys its
// cache on Node plus Creation, so both must be filled.
func TestSystemCapabilities(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	caps := capabilitiesOf(t, n)

	if caps.Node != n.Name() {
		t.Errorf("report belongs to %s, expected %s", caps.Node, n.Name())
	}
	if caps.Creation == 0 {
		t.Error("creation is zero, so a consumer cannot key its cache")
	}
	if caps.Framework.Release == "" {
		t.Error("framework version is empty")
	}

	if caps.Manage == false {
		t.Error("the mutating plane is up by default and must be reported")
	}
	if slices.Contains(caps.Capabilities, inspect.CapNodeShort) == false {
		t.Errorf("read capabilities miss %s: %v", inspect.CapNodeShort, caps.Capabilities)
	}
	if slices.Contains(caps.Capabilities, manage.CapKill) == false {
		t.Errorf("mutating capabilities miss %s while the plane is up", manage.CapKill)
	}
	if slices.Contains(caps.Capabilities, inspect.CapCapabilities) == false {
		t.Error("the report does not list itself")
	}

	// the report must claim only tags the framework actually defines, whether or
	// not this suite was built with them
	known := []string{"pprof", "latency", "verbose", "norecover"}
	for _, tag := range caps.Build {
		if slices.Contains(known, tag) == false {
			t.Errorf("unknown build tag %q reported", tag)
		}
	}
}

// TestSystemCapabilitiesManageDisabled: with the mutating plane down the report
// says so and drops every manage.* name, which is what a caller gates on before
// offering an action.
func TestSystemCapabilitiesManageDisabled(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{
		EnableSystemApp:     true,
		DisableSystemManage: true,
	})

	caps := capabilitiesOf(t, n)

	if caps.Manage {
		t.Error("the mutating plane is disabled but reported as up")
	}
	for _, name := range caps.Capabilities {
		if len(name) >= 7 && name[:7] == "manage." {
			t.Errorf("%s is offered while the plane is down", name)
		}
	}
	if slices.Contains(caps.Capabilities, inspect.CapNode) == false {
		t.Error("reading must stay available with the mutating plane down")
	}

	// the process itself must not be there either
	if _, err := n.Native().ProcessPID(manage.Name); err == nil {
		t.Errorf("%s is running while disabled", manage.Name)
	}
}

// TestSystemCapabilitiesMutationRefused: a mutation sent to a node with the plane
// down fails instead of silently doing nothing.
func TestSystemCapabilitiesMutationRefused(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{
		EnableSystemApp:     true,
		DisableSystemManage: true,
	})

	target := gen.ProcessID{Name: manage.Name, Node: n.Name()}
	if _, err := n.Native().Call(target, manage.RequestDoSetLogLevel{Level: gen.LogLevelDebug}); err == nil {
		t.Error("a mutation went through with the plane down")
	}
}
