package inspect

import (
	"time"

	"ergo.services/ergo/gen"
)

type RequestInspectNode struct{}
type ResponseInspectNode struct {
	CRC32     string
	Event     gen.Event
	OS        string
	Arch      string
	Cores     int
	Timezone  string
	GoVersion string
	Version   gen.Version
	Creation  int64

	// build information from debug.ReadBuildInfo() of the running node binary
	BuildMain     string   // main module "path@version"
	BuildRevision string   // VCS commit of the build (if available)
	BuildModified bool     // VCS working tree was dirty at build time
	BuildSettings []string // build settings as "key=value" (GOOS, GOARCH, -tags, -ldflags, vcs.time, ...)
	BuildDeps     []string // dependency modules as "path@version", sorted
	BuildReplaces []string // replace directives as "path => target@version" (e.g. local "=> ../foo")
}

type MessageInspectNode struct {
	Node gen.Atom
	Info gen.NodeInfo
}

type RequestGetNode struct{}
type ResponseGetNode struct {
	Node  gen.Atom
	Info  gen.NodeInfo
	Error error
}

// node short

type RequestInspectNodeShort struct {
	// Period between the published snapshots. Zero applies the default of 3s,
	// and anything below 100ms is raised to it. Consumers asking for different
	// periods get their own inspector and their own event.
	Period time.Duration
}
type ResponseInspectNodeShort struct {
	Event gen.Event
	Info  gen.NodeShortInfo
	Error error
}

type MessageInspectNodeShort struct {
	Node gen.Atom
	Info gen.NodeShortInfo
}

// network

type RequestInspectNetwork struct{}
type ResponseInspectNetwork struct {
	Event   gen.Event
	Stopped bool
	Info    gen.NetworkInfo
}

type MessageInspectNetwork struct {
	Node    gen.Atom
	Stopped bool
	Info    gen.NetworkInfo
}

type RequestGetNetwork struct{}
type ResponseGetNetwork struct {
	Node  gen.Atom
	Info  gen.NetworkInfo
	Error error
}

type RequestInspectConnection struct {
	RemoteNode gen.Atom
}
type ResponseInspectConnection struct {
	Event        gen.Event
	Disconnected bool
	Info         gen.RemoteNodeInfo
}

type MessageInspectConnection struct {
	Node         gen.Atom
	Disconnected bool
	Info         gen.RemoteNodeInfo
}

type RequestGetConnection struct {
	RemoteNode gen.Atom
}
type ResponseGetConnection struct {
	Node  gen.Atom
	Info  gen.RemoteNodeInfo
	Error error
}

// connection list (scoped)

type RequestInspectConnectionList struct {
	Limit int
	Name  string
}
type ResponseInspectConnectionList struct {
	Event gen.Event
}

type MessageInspectConnectionList struct {
	Node        gen.Atom
	Connections []gen.RemoteNodeInfo
}

type RequestGetConnectionList struct {
	Limit int
	Name  string
}
type ResponseGetConnectionList struct {
	Node        gen.Atom
	Connections []gen.RemoteNodeInfo
	Error       error
}

// process list

type RequestInspectProcessList struct {
	Start       int
	Limit       int
	Name        string
	Behavior    string
	Application string
	State       string
	MinMailbox  uint64
}
type ResponseInspectProcessList struct {
	Event gen.Event
}

type MessageInspectProcessList struct {
	Node      gen.Atom
	Processes []gen.ProcessShortInfo
}

// Start walks forward from that process id, negative walks back from the newest. Zero applies
// the default of 1000, the lowest id a request may name.
type RequestGetProcessList struct {
	Start       int
	Limit       int
	Name        string
	Behavior    string
	Application string
	State       string
	MinMailbox  uint64
}
type ResponseGetProcessList struct {
	Node      gen.Atom
	Processes []gen.ProcessShortInfo
	Error     error
}

// node logs

type RequestInspectLog struct {
	Levels         []gen.LogLevel
	Limit          int
	MessagePattern string
	MessageExclude bool
}
type ResponseInspectLog struct {
	Event gen.Event
}

type InspectLogEntry struct {
	Source    string // "node", "process", "network", "meta", "application"
	Name      gen.Atom
	PID       gen.PID
	Behavior  string
	Peer      gen.Atom
	Parent    gen.PID
	Meta      gen.Alias
	Creation  int64
	Timestamp int64
	Level     gen.LogLevel
	Message   string
	Fields    []gen.LogField
}

type MessageInspectLog struct {
	Node       gen.Atom
	Entries    []InspectLogEntry
	Suppressed int64
}

// process

type RequestInspectProcess struct {
	PID gen.PID
}
type ResponseInspectProcess struct {
	Event gen.Event
	Info  gen.ProcessInfo
}

type MessageInspectProcess struct {
	Node gen.Atom
	Info gen.ProcessInfo
}

type RequestGetProcess struct {
	PID gen.PID
}
type ResponseGetProcess struct {
	Node  gen.Atom
	Info  gen.ProcessInfo
	Error error
}

// meta

type RequestInspectMeta struct {
	Meta gen.Alias
}
type ResponseInspectMeta struct {
	Event gen.Event
	Info  gen.MetaInfo
}

type MessageInspectMeta struct {
	Node gen.Atom
	Info gen.MetaInfo
}

type RequestGetMeta struct {
	Meta gen.Alias
}
type ResponseGetMeta struct {
	Node  gen.Atom
	Info  gen.MetaInfo
	Error error
}

// one-shot process and meta state

type RequestGetProcessState struct {
	PID   gen.PID
	Items []string
}
type ResponseGetProcessState struct {
	State map[string]string
	Error error
}

type RequestGetMetaState struct {
	Meta  gen.Alias
	Items []string
}
type ResponseGetMetaState struct {
	State map[string]string
	Error error
}

// process lookup

// RequestGetProcessLookup resolves a process either way: set Name to look it up
// by its registered name, or PID to get the name it is registered under.
type RequestGetProcessLookup struct {
	Name gen.Atom
	PID  gen.PID
}

type ResponseGetProcessLookup struct {
	PID   gen.PID
	Name  gen.Atom // empty when the process is not registered under a name
	State gen.ProcessState
	Error error
}

// goroutine dump

type RequestGetGoroutines struct {
	Stack   string // substring match in stack text
	State   string // exact state match (running, chan receive, etc.)
	MinWait int64  // minimum wait duration in seconds (0 = any)
}

type GoroutineInfo struct {
	ID       int
	State    string
	Wait     string
	Frames   []string
	FullText string
}

type GoroutineGroup struct {
	Count   int
	State   string
	WaitSec int64
	Origin  string
	Current string
	Stack   string
	IDs     []int
}

type ResponseGetGoroutines struct {
	Groups   []GoroutineGroup
	Total    int
	Filtered int
	Error    error
}

// heap profile

type RequestGetHeapProfile struct {
	MinBytes int64
}

type HeapRecord struct {
	InuseBytes   int64
	InuseObjects int64
	AllocBytes   int64
	AllocObjects int64
	FreeObjects  int64
	Stack        []string
}

type HeapStats struct {
	TotalInuse   int64
	TotalObjects int64
	TotalAlloc   int64
	TotalFree    int64
}

type ResponseGetHeapProfile struct {
	Records      []HeapRecord
	TotalInuse   int64
	TotalAlloc   int64
	TotalObjects int64
	Error        error
}

// heap inspector (event-based)

type RequestInspectHeap struct {
	Limit int
	Name  string
}
type ResponseInspectHeap struct {
	Event gen.Event
}

type MessageInspectHeap struct {
	Node          gen.Atom
	Records       []HeapRecord
	TotalInuse    int64
	TotalObjects  int64
	TotalAlloc    int64
	TotalFree     int64
	GCCPUFraction float64
}

// process range (full scan with filters)

type RequestInspectProcessRange struct {
	Name        string
	Behavior    string
	Application string
	State       string
	MinMailbox  uint64
	Limit       int
}

type RequestGetProcessRange struct {
	Name        string
	Behavior    string
	Application string
	State       string
	MinMailbox  uint64
	Limit       int
}
type ResponseGetProcessRange struct {
	Node      gen.Atom
	Processes []gen.ProcessShortInfo
	Error     error
}
type ResponseInspectProcessRange struct {
	Event gen.Event
}

// event list

type RequestInspectEventList struct {
	Timestamp      int64 // 0=oldest first, -1=newest first, >0=from this unix nanos
	Limit          int
	Name           string
	Notify         int // 0=any, 1=yes, -1=no
	Buffered       int // 0=any, 1=yes, -1=no
	Open           int // 0=any, 1=yes, -1=no
	MinSubscribers int64
}
type ResponseInspectEventList struct {
	Event gen.Event
}

type MessageInspectEventList struct {
	Node   gen.Atom
	Events []gen.EventInfo
}

type RequestGetEventList struct {
	Timestamp      int64
	Limit          int
	Name           string
	Notify         int
	Buffered       int
	Open           int
	MinSubscribers int64
}
type ResponseGetEventList struct {
	Node   gen.Atom
	Events []gen.EventInfo
	Error  error
}

// event

type RequestInspectEvent struct {
	Name gen.Atom
}
type ResponseInspectEvent struct {
	Event gen.Event
	Info  gen.EventInfo
}

type RequestInspectEventStream struct {
	Name           gen.Atom
	Limit          int
	TypePattern    string
	MessagePattern string
	MessageExclude bool
	Force          bool
	Verbose        bool
}
type ResponseInspectEventStream struct {
	Event       gen.Event           // inspector's own event to monitor
	Target      gen.Event           // observed event, for client-side routing
	Buffer      []InspectEventEntry // backlog snapshot returned to every subscriber
	Watching    bool
	WatchReason string
}

type InspectEventEntry struct {
	Timestamp int64
	Type      string // %T
	Message   string // %+v
	Verbose   string // %#v, set only when stream Verbose is on and differs from Message
}

type MessageInspectEvent struct {
	Node        gen.Atom
	Info        gen.EventInfo
	Entries     []InspectEventEntry
	Suppressed  int64 // entries dropped this tick by the storm cap (rate/s, not cumulative)
	Closed      bool
	Reason      string
	Watching    bool
	WatchReason string
}

type RequestGetEvent struct {
	Name gen.Atom
}
type ResponseGetEvent struct {
	Node  gen.Atom
	Info  gen.EventInfo
	Error error
}

// application list

type RequestInspectApplicationList struct{}
type ResponseInspectApplicationList struct {
	Event gen.Event
}

type MessageInspectApplicationList struct {
	Node         gen.Atom
	Applications map[gen.Atom]gen.ApplicationInfo
}

type RequestGetApplicationList struct{}
type ResponseGetApplicationList struct {
	Node         gen.Atom
	Applications map[gen.Atom]gen.ApplicationInfo
	Error        error
}

// application tree

type RequestGetAppTree struct {
	Application gen.Atom
	Limit       int
}
type ResponseGetAppTree struct {
	Node        gen.Atom
	Application gen.Atom
	Processes   []gen.ProcessShortInfo
	// Truncated is the number of application processes omitted because the
	// result hit the limit (0 means the whole tree was returned).
	Truncated int
	Error     error
}

// subtree rooted at a process

type RequestGetSubtree struct {
	PID   gen.PID
	Limit int
}
type ResponseGetSubtree struct {
	Node      gen.Atom
	PID       gen.PID
	Processes []gen.ProcessShortInfo
	Truncated bool
	Error     error
}

// cron

// RequestGetCronSchedule previews the upcoming firings. Only the node can compute
// them: the schedule is evaluated against its clock and each job's timezone.
type RequestGetCronSchedule struct {
	// Job narrows the preview to one job. Empty covers every job.
	Job gen.Atom

	// Since is where the preview starts. Zero means the node's current time.
	Since time.Time

	// Duration is how far ahead to look. Zero applies 24h.
	Duration time.Duration

	// Limit caps the returned entries. Zero applies 1000: a per-minute job over a
	// long window would otherwise answer with thousands of timestamps.
	Limit int
}

type ResponseGetCronSchedule struct {
	Schedule  []gen.CronSchedule
	Truncated bool
	Error     error
}

// RequestGetCronInfo reads the scheduler, or one job when Job is set. The same
// data is part of the node snapshot, but a caller after a single job should not
// have to pull the whole of it.
type RequestGetCronInfo struct {
	Job gen.Atom
}

type ResponseGetCronInfo struct {
	// Next and Spool are filled for the whole scheduler only.
	Next  time.Time
	Spool []gen.Atom

	Jobs  []gen.CronJobInfo
	Error error
}

// registrar

type RequestGetRegistrarNodes struct{}
type ResponseGetRegistrarNodes struct {
	Nodes []gen.Atom
	Error error
}

type RequestGetRegistrarRoutes struct {
	Node gen.Atom
}
type ResponseGetRegistrarRoutes struct {
	Routes []gen.Route
	Error  error
}

type RequestGetRegistrarProxyRoutes struct {
	Node gen.Atom
}
type ResponseGetRegistrarProxyRoutes struct {
	Routes []gen.ProxyRoute
	Error  error
}

type RequestGetRegistrarApplicationRoutes struct {
	Name gen.Atom
}
type ResponseGetRegistrarApplicationRoutes struct {
	Routes []gen.ApplicationRoute
	Error  error
}

// tracing

type RequestInspectTracing struct {
	Flags          gen.TracingFlags
	Limit          int
	Kinds          uint32 // bitmask: 1=send, 2=request, 4=response, 8=spawn, 16=terminate
	Points         uint32 // bitmask: 1=sent, 2=delivered, 4=processed, 8=span
	MessagePattern string
	MessageExclude bool
}

type ResponseInspectTracing struct {
	Event gen.Event
}

type MessageInspectTracing struct {
	Node       gen.Atom
	Spans      []gen.TracingSpan
	Suppressed int64
}

// types

type RequestGetTypes struct{}

type ResponseGetTypes struct {
	Types []gen.RegisteredTypeInfo
	Error error
}
