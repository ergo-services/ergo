package inspect

import (
	"fmt"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

func TestNetworkInspectorContract(t *testing.T) {
	runInspectorContract(t, inspectorCase{
		name:    "network",
		factory: factory_network,
		event:   gen.Atom(inspectNetwork),
	})
}

func TestApplicationListInspectorContract(t *testing.T) {
	runInspectorContract(t, inspectorCase{
		name:    "application_list",
		factory: factory_application_list,
		event:   gen.Atom(inspectApplicationList),
		node: func(t *testing.T) *unit.MockNode {
			t.Helper()
			node := plainNode(t)
			node.OnApplications(func() []gen.Atom { return []gen.Atom{"system_app"} })
			node.OnApplicationInfo(func(name gen.Atom) (gen.ApplicationInfo, error) {
				return gen.ApplicationInfo{Name: name}, nil
			})
			return node
		},
	})
}

func TestProcessInspectorContract(t *testing.T) {
	pid := gen.PID{Node: "inspect@localhost", ID: 1001}

	runInspectorContract(t, inspectorCase{
		name:    "process",
		factory: factory_process,
		event:   gen.Atom(fmt.Sprintf("%s_%s", inspectProcess, pid)),
		args:    []any{pid},
		node: func(t *testing.T) *unit.MockNode {
			t.Helper()
			node := plainNode(t)
			node.OnProcessInfo(func(gen.PID) (gen.ProcessInfo, error) {
				return gen.ProcessInfo{PID: pid}, nil
			})
			return node
		},
	})
}

func TestMetaInspectorContract(t *testing.T) {
	alias := gen.Alias{Node: "inspect@localhost", ID: [3]uint64{1, 2, 3}}

	runInspectorContract(t, inspectorCase{
		name:    "meta",
		factory: factory_meta,
		event:   gen.Atom(fmt.Sprintf("%s_%s", inspectMeta, alias)),
		args:    []any{alias},
		node: func(t *testing.T) *unit.MockNode {
			t.Helper()
			node := plainNode(t)
			node.OnMetaInfo(func(gen.Alias) (gen.MetaInfo, error) {
				return gen.MetaInfo{ID: alias}, nil
			})
			return node
		},
	})
}

func TestConnectionInspectorContract(t *testing.T) {
	remote := gen.Atom("peer@localhost")

	runInspectorContract(t, inspectorCase{
		name:    "connection",
		factory: factory_connection,
		event:   gen.Atom(fmt.Sprintf("%s_%s", inspectConnection, remote)),
		args:    []any{remote},
		node: func(t *testing.T) *unit.MockNode {
			t.Helper()
			node := plainNode(t)
			node.Network().OnGetNode(remote)
			return node
		},
	})
}

func TestConnectionListInspectorContract(t *testing.T) {
	runInspectorContract(t, inspectorCase{
		name:    "connection_list",
		factory: factory_connection_list,
		event:   gen.Atom(fmt.Sprintf("%s_%s", inspectConnectionList, "h1")),
		args:    []any{"", 10, "h1"},
	})
}

func TestProcessListInspectorContract(t *testing.T) {
	hash := filterHash("", "", "", "", uint64(0), 10)

	runInspectorContract(t, inspectorCase{
		name:    "process_list",
		factory: factory_process_list,
		event:   gen.Atom(fmt.Sprintf("%s_%d_%s", inspectProcessList, 1000, hash)),
		args:    []any{1000, 10, "", "", "", "", uint64(0)},
		node: func(t *testing.T) *unit.MockNode {
			t.Helper()
			node := plainNode(t)
			node.OnProcessListShortInfo(func(int, int, ...func(gen.ProcessShortInfo) bool) ([]gen.ProcessShortInfo, error) {
				return nil, nil
			})
			return node
		},
	})
}

func TestProcessRangeInspectorContract(t *testing.T) {
	runInspectorContract(t, inspectorCase{
		name:    "process_range",
		factory: factory_process_range,
		event:   gen.Atom(fmt.Sprintf("%s_%s", inspectProcessRange, "h2")),
		args:    []any{"", "", "", "", uint64(0), 10, "h2"},
		node: func(t *testing.T) *unit.MockNode {
			t.Helper()
			node := plainNode(t)
			node.OnProcessRangeShortInfo(func(func(gen.ProcessShortInfo) bool) error {
				return nil
			})
			return node
		},
	})
}

func TestEventListInspectorContract(t *testing.T) {
	runInspectorContract(t, inspectorCase{
		name:    "event_list",
		factory: factory_event_list,
		event:   gen.Atom(fmt.Sprintf("%s_%s", inspectEventList, "h3")),
		args:    []any{int64(0), "", 0, 0, 0, int64(0), 10, "h3"},
		node: func(t *testing.T) *unit.MockNode {
			t.Helper()
			node := plainNode(t)
			node.OnEventListInfo(func(int64, int, ...func(gen.EventInfo) bool) ([]gen.EventInfo, error) {
				return nil, nil
			})
			return node
		},
	})
}

func TestEventInspectorContract(t *testing.T) {
	target := gen.Atom("prices")

	runInspectorContract(t, inspectorCase{
		name:    "event",
		factory: factory_event,
		event:   gen.Atom(fmt.Sprintf("%s_%s", inspectEvent, "h4")),
		args:    []any{eventArgs{Name: target, Hash: "h4"}},
		node: func(t *testing.T) *unit.MockNode {
			t.Helper()
			node := plainNode(t)
			node.OnEventInfo(func(gen.Event) (gen.EventInfo, error) {
				return gen.EventInfo{Event: gen.Event{Name: target, Node: "inspect@localhost"}}, nil
			})
			return node
		},
	})
}
