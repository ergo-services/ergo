// Package rust holds the requests of the system application that only a node
// running on the Rust runtime serves. Every node registers these types, so their
// answers decode anywhere; a node of another runtime answers them with
// gen.ErrUnsupported and does not announce their capabilities.
package rust

import (
	"time"

	"ergo.services/ergo/gen"
)

const (
	CapRuntime        = "inspect.rust.runtime"
	CapRuntimeMetrics = "inspect.rust.runtime_metrics"
	CapRuntimeState   = "inspect.rust.runtime_state"
	CapAtomSummary    = "inspect.rust.atom_summary"
	CapAtomTable      = "inspect.rust.atom_table"

	CapSetNodeAtomLimit = "manage.rust.set_node_atom_limit"
	CapSetPeerAtomLimit = "manage.rust.set_peer_atom_limit"
)

// RuntimeMetric describes a metric of the runtime: its name with the unit after
// ':', its kind (u64, f64, histogram, u64s, f64s), whether it only grows. A
// histogram has the lower bounds of its buckets in seconds, the last bucket has
// no upper bound.
type RuntimeMetric struct {
	Name        string
	Description string
	Kind        string
	Cumulative  bool
	Buckets     []float64
}

// RuntimeSample is a value of a metric by its kind: Uint64, Float64, the counts of
// a histogram from the bucket First on (the empty ones around cut), one value per
// worker of the scheduler in Uint64s or Float64s. The kind is empty when the value
// is not available.
type RuntimeSample struct {
	Name     string
	Kind     string
	Uint64   uint64
	Float64  float64
	First    int
	Counts   []uint64
	Uint64s  []uint64
	Float64s []float64
}

// RuntimeRunnable is what runs on a worker: its kind (process, task), its node (a
// task: the one it belongs to, empty if none), the process and its registered
// name, the id of a task, the behavior (a task: its name).
type RuntimeRunnable struct {
	Kind     string
	Node     gen.Atom
	PID      gen.PID
	Name     gen.Atom
	ID       uint64
	Behavior string
}

// RuntimeWorker is a worker of the scheduler now: what it does (idle, running,
// searching, parked, watching, lost, spare), what it runs and for how long, what
// is queued on it, its timers; a lost one: the spare worker holding its license,
// a spare one: the lost worker whose license it holds (-1 - none).
type RuntimeWorker struct {
	Index      int
	Doing      string
	Running    RuntimeRunnable
	RunningFor int64 // ns
	Queued     int
	Inbox      int
	Timers     int
	ReplacedBy int
	Replaces   int
}

// RuntimeLongRun is a run longer than the threshold of the scheduler.
type RuntimeLongRun struct {
	Time     int64 // unix ns, the end
	Worker   int
	Runnable RuntimeRunnable
	Duration int64 // ns
}

// RuntimeLostRun is a run longer than the lost run of the scheduler: its worker
// lost the license.
type RuntimeLostRun struct {
	Time     int64 // unix ns, the start
	Worker   int
	Spare    int // the spare worker that took the license; -1 - none was left
	Runnable RuntimeRunnable
	Duration int64 // ns; -1 - it goes on
}

// RuntimeState is what the runtime does now.
type RuntimeState struct {
	Workers    []RuntimeWorker
	LongRuns   []RuntimeLongRun
	LongRun    int64 // ns
	LostRuns   []RuntimeLostRun
	LostRun    int64 // ns
	Handoffs   uint64
	Licenses   int
	SpareLimit int
}

// RequestInspectRuntime subscribes to the runtime: the response carries the event
// and the catalog of the metrics, MessageInspectRuntime comes every second while
// someone listens.
type RequestInspectRuntime struct{}

type ResponseInspectRuntime struct {
	Event   gen.Event
	Metrics []RuntimeMetric
}

type MessageInspectRuntime struct {
	Node    gen.Atom
	Time    int64 // unix ns
	Samples []RuntimeSample
	State   RuntimeState
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

type RequestGetRuntimeState struct{}

type ResponseGetRuntimeState struct {
	Time  int64 // unix ns
	State RuntimeState
	Error error
}

type AtomLimitInfo struct {
	Atoms   uint64
	Limit   uint64
	Refused uint64
	SetBy   string
}

type AtomPeerInfo struct {
	Peer      gen.Atom
	Atoms     uint64
	Limit     uint64
	Refused   uint64
	Exception bool
}

type AtomGroupInfo struct {
	Key         string
	Atoms       uint64
	Bytes       uint64
	Origin      string
	OriginAtoms uint64
}

type AtomInfo struct {
	Name   string
	Origin string
	Born   time.Time
}

type RequestGetAtomSummary struct{}

type ResponseGetAtomSummary struct {
	Atoms   uint64
	Bytes   uint64
	Long    uint64
	Program uint64
	Network uint64
	Refused uint64
	Runtime AtomLimitInfo
	Node    AtomLimitInfo
	Peer    AtomLimitInfo
	Peers   []AtomPeerInfo
	Groups  []AtomGroupInfo
	Origins []AtomGroupInfo
	Recent  []AtomInfo
	Error   error
}

type RequestGetAtomTable struct {
	Prefix string
	Find   string
	Origin string
	Long   bool
	Limit  int
}

type ResponseGetAtomTable struct {
	Groups    []AtomGroupInfo
	Atoms     []AtomInfo
	Truncated int
	Error     error
}

// RequestDoSetNodeAtomLimit sets the limit of the atoms the node makes.
type RequestDoSetNodeAtomLimit struct {
	Limit uint64
}

// RequestDoSetPeerAtomLimit sets the limit of the atoms the peer makes on the
// node; an empty Peer sets the default for every peer.
type RequestDoSetPeerAtomLimit struct {
	Peer  gen.Atom
	Limit uint64
}

type ResponseDoSetAtomLimit struct {
	Previous uint64
	Atoms    uint64
	Error    error
}

// BuildInfo is the build of a Rust node, the value of
// inspect.ResponseInspectNode.Build.
type BuildInfo struct {
	Target          string // aarch64-apple-darwin
	Host            string
	Profile         string // release, debug
	OptLevel        string // 0..3, s, z
	DebugAssertions bool
	Panic           string   // unwind, abort
	Features        []string // the enabled features of the framework
	RustFlags       []string
}

// Types returns the wire-format types of the package for use in
// gen.ApplicationSpec.Network.RegisterTypes.
func Types() []any {
	return []any{
		RuntimeMetric{}, RuntimeSample{}, RuntimeRunnable{}, RuntimeWorker{},
		RuntimeLongRun{}, RuntimeLostRun{}, RuntimeState{},
		RequestInspectRuntime{}, ResponseInspectRuntime{}, MessageInspectRuntime{},
		RequestGetRuntimeMetrics{}, ResponseGetRuntimeMetrics{},
		RequestGetRuntimeState{}, ResponseGetRuntimeState{},
		AtomLimitInfo{}, AtomPeerInfo{}, AtomGroupInfo{}, AtomInfo{},
		RequestGetAtomSummary{}, ResponseGetAtomSummary{},
		RequestGetAtomTable{}, ResponseGetAtomTable{},
		RequestDoSetNodeAtomLimit{}, RequestDoSetPeerAtomLimit{}, ResponseDoSetAtomLimit{},
		BuildInfo{},
	}
}
