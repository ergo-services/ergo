// Package golang holds the requests of the system application that only a node
// running on the Go runtime serves. Every node registers these types, so their
// answers decode anywhere; a node of another runtime answers them with
// gen.ErrUnsupported and does not announce their capabilities.
package golang

import (
	"ergo.services/ergo/gen"
)

const (
	CapGoroutines     = "inspect.golang.goroutines"
	CapHeapProfile    = "inspect.golang.heap_profile"
	CapRuntime        = "inspect.golang.runtime"
	CapRuntimeMetrics = "inspect.golang.runtime_metrics"
)

// Capabilities returns the capabilities a Go node announces for this package.
func Capabilities() []string {
	return []string{
		CapGoroutines,
		CapHeapProfile,
		CapRuntime,
		CapRuntimeMetrics,
	}
}

// RequestGetGoroutines groups the goroutines of the node by their stacks.
type RequestGetGoroutines struct {
	Stack   string // substring of the stack text
	State   string // exact state: running, chan receive...
	MinWait int64  // seconds; 0 - any
	Limit   int    // groups; 0 - 1000
}

// GoroutineGroup is the goroutines with the same state and stack. The Go runtime
// reports a wait from one minute on and in minutes, so WaitSec is a multiple of 60.
type GoroutineGroup struct {
	Count   int
	State   string
	WaitSec int64  // the longest wait in the group
	Origin  string // the bottom function of the stack
	Current string // the top function of the stack
	Stack   string // the full text of the first goroutine of the group
	IDs     []int  // the first 100
}

type ResponseGetGoroutines struct {
	Groups    []GoroutineGroup
	Total     int // goroutines on the node
	Filtered  int // passed the filters
	Truncated int // groups over Limit
	Error     error
}

// RequestGetHeapProfile reads the heap profile of the Go runtime. The values are
// scaled as pprof does, by the chance of a sample to be taken.
type RequestGetHeapProfile struct {
	MinBytes int64 // InuseBytes of a record at least; 0 - any
	Limit    int   // records; 0 - 1000
}

type HeapRecord struct {
	InuseBytes   int64
	InuseObjects int64
	AllocBytes   int64
	AllocObjects int64
	FreeObjects  int64
	Stack        []string // function names
}

type ResponseGetHeapProfile struct {
	Records      []HeapRecord // by InuseBytes, descending
	TotalInuse   int64        // over all records
	TotalAlloc   int64
	TotalObjects int64
	Truncated    int
	Error        error
}

// RuntimeMetric describes a metric of runtime/metrics: its name with the unit
// after ':', its kind (uint64, float64, histogram), whether it only grows. A
// histogram has the lower bounds of its buckets in the unit of the metric.
type RuntimeMetric struct {
	Name        string
	Description string
	Kind        string
	Cumulative  bool
	Buckets     []float64
}

// RuntimeSample is a value of a metric by its kind; a histogram has the counts of
// its buckets from the bucket First on, the empty ones around cut. The kind is
// empty when the value is not available.
type RuntimeSample struct {
	Name    string
	Kind    string
	Uint64  uint64
	Float64 float64
	First   int
	Counts  []uint64
}

// RequestInspectRuntime subscribes to the metrics of the Go runtime: the response
// carries the event and the catalog, MessageInspectRuntime comes every second
// while someone listens.
type RequestInspectRuntime struct{}

type ResponseInspectRuntime struct {
	Event   gen.Event
	Metrics []RuntimeMetric
}

type MessageInspectRuntime struct {
	Node    gen.Atom
	Time    int64 // unix ns
	Samples []RuntimeSample
}

// RequestGetRuntimeMetrics reads the metrics by their names or by the prefixes of
// their names (ending with '/'); all of them if none.
type RequestGetRuntimeMetrics struct {
	Names []string
}

type ResponseGetRuntimeMetrics struct {
	Time    int64 // unix ns
	Metrics []RuntimeMetric
	Samples []RuntimeSample
	Error   error
}

// BuildInfo is the build of a Go node, the value of inspect.ResponseInspectNode.Build.
type BuildInfo struct {
	Main     string   // path@version of the main module
	Revision string   // vcs.revision
	Modified bool     // vcs.modified
	Settings []string // key=value: GOOS, -tags, -ldflags, vcs.time...
	Deps     []string // path@version
	Replaces []string // path => target@version
}

// Types returns the wire-format types of the package for use in
// gen.ApplicationSpec.Network.RegisterTypes.
func Types() []any {
	return []any{
		RequestGetGoroutines{}, GoroutineGroup{}, ResponseGetGoroutines{},
		RequestGetHeapProfile{}, HeapRecord{}, ResponseGetHeapProfile{},
		RuntimeMetric{}, RuntimeSample{},
		RequestInspectRuntime{}, ResponseInspectRuntime{}, MessageInspectRuntime{},
		RequestGetRuntimeMetrics{}, ResponseGetRuntimeMetrics{},
		BuildInfo{},
	}
}
