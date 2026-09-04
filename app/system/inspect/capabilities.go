package inspect

import (
	"sort"

	"ergo.services/ergo/gen"
)

const (
	// EnvManage tells the inspector whether the mutating plane is configured.
	EnvManage gen.Env = "system_manage_enabled"

	// EnvManageProcess carries the registered name of the mutating plane.
	EnvManageProcess gen.Env = "system_manage_process"

	// EnvManageCapabilities carries the capability names of the mutating plane.
	// The inspector reports them without importing it.
	EnvManageCapabilities gen.Env = "system_manage_capabilities"
)

// Capability names of the read plane. Same vocabulary as the mutating plane:
// feature detection, the caller's ceiling and the audit record share it.
const (
	CapCapabilities = "inspect.capabilities"

	CapNode            = "inspect.node"
	CapNodeShort       = "inspect.node_short"
	CapNetwork         = "inspect.network"
	CapConnection      = "inspect.connection"
	CapConnectionList  = "inspect.connection_list"
	CapProcessList     = "inspect.process_list"
	CapProcessRange    = "inspect.process_range"
	CapProcess         = "inspect.process"
	CapMeta            = "inspect.meta"
	CapApplicationList = "inspect.application_list"
	CapEventList       = "inspect.event_list"
	CapEvent           = "inspect.event"
	CapEventStream     = "inspect.event_stream"
	CapLog             = "inspect.log"
	CapTracing         = "inspect.tracing"

	CapGetProcessState  = "inspect.process_state"
	CapGetProcessLookup = "inspect.process_lookup"
	CapGetMetaState     = "inspect.meta_state"
	CapGetAppTree       = "inspect.app_tree"
	CapGetSubtree       = "inspect.subtree"
	CapGetGoroutines    = "inspect.goroutines"
	CapGetHeapProfile   = "inspect.heap_profile"
	CapGetTypes         = "inspect.types"
	CapGetErrors        = "inspect.errors"
	CapGetAtoms         = "inspect.atoms"

	CapGetCronInfo     = "inspect.cron_info"
	CapGetCronSchedule = "inspect.cron_schedule"

	CapGetRegistrarNodes             = "inspect.registrar_nodes"
	CapGetRegistrarRoutes            = "inspect.registrar_routes"
	CapGetRegistrarProxyRoutes       = "inspect.registrar_proxy_routes"
	CapGetRegistrarApplicationRoutes = "inspect.registrar_application_routes"
)

// buildTags is filled by the tag-guarded files of this package, so it reports
// what was actually compiled rather than what the build recorded.
var buildTags []string

type RequestGetCapabilities struct{}

type ResponseGetCapabilities struct {
	Node     gen.Atom
	Creation int64

	Version   gen.Version
	Framework gen.Version

	// Manage reports that the mutating plane is configured and answering.
	Manage bool

	// Capabilities are the names this node supports. The manage.* ones appear
	// only while the mutating plane is up.
	Capabilities []string

	// Build lists the enabled build tags. Without "latency" every latency field
	// reports -1, which a consumer has to know before charting it.
	Build []string
}

// Capabilities returns every capability of the read plane.
func Capabilities() []string {
	return []string{
		CapCapabilities,
		CapNode,
		CapNodeShort,
		CapNetwork,
		CapConnection,
		CapConnectionList,
		CapProcessList,
		CapProcessRange,
		CapProcess,
		CapMeta,
		CapApplicationList,
		CapEventList,
		CapEvent,
		CapEventStream,
		CapLog,
		CapTracing,
		CapGetProcessState,
		CapGetProcessLookup,
		CapGetMetaState,
		CapGetAppTree,
		CapGetSubtree,
		CapGetGoroutines,
		CapGetHeapProfile,
		CapGetTypes,
		CapGetErrors,
		CapGetAtoms,
		CapGetCronInfo,
		CapGetCronSchedule,
		CapGetRegistrarNodes,
		CapGetRegistrarRoutes,
		CapGetRegistrarProxyRoutes,
		CapGetRegistrarApplicationRoutes,
	}
}

// capabilities builds the static part of the answer. Called once per worker at
// init: the set changes only with a node restart, which the caller keys its
// cache on through Node and Creation.
func (i *inspect) capabilities() ResponseGetCapabilities {
	build := append([]string(nil), buildTags...)
	sort.Strings(build)

	return ResponseGetCapabilities{
		Node:         i.Node().Name(),
		Creation:     i.Node().Creation(),
		Version:      i.Node().Version(),
		Framework:    i.Node().FrameworkVersion(),
		Capabilities: Capabilities(),
		Build:        build,
	}
}

// manageCapabilities reads what the mutating plane offers, if it is configured.
func (i *inspect) manageCapabilities() (gen.Atom, []string) {
	if enabled, _ := i.Env(EnvManage); enabled != true {
		return "", nil
	}

	name, _ := i.Env(EnvManageProcess)
	process, ok := name.(gen.Atom)
	if ok == false || process == "" {
		return "", nil
	}

	v, _ := i.Env(EnvManageCapabilities)
	caps, ok := v.([]string)
	if ok == false {
		return "", nil
	}

	return process, caps
}

// responseCapabilities adds what is true right now: Env says the plane is
// configured, a lookup says it is actually there.
func (i *inspect) responseCapabilities() ResponseGetCapabilities {
	response := i.caps
	if i.manageProcess == "" {
		return response
	}
	if _, err := i.Node().ProcessPID(i.manageProcess); err != nil {
		return response
	}

	response.Manage = true
	response.Capabilities = append(append([]string(nil), i.caps.Capabilities...), i.manageCaps...)
	return response
}
