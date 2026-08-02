package inspect

import "ergo.services/ergo/gen"

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

// process state
type RequestInspectProcessState struct {
	PID   gen.PID
	Items []string
}
type ResponseInspectProcessState struct {
	Event gen.Event
}

type MessageInspectProcessState struct {
	Node  gen.Atom
	PID   gen.PID
	State map[string]string
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

// meta state
type RequestInspectMetaState struct {
	Meta  gen.Alias
	Items []string
}
type ResponseInspectMetaState struct {
	Event gen.Event
}

type MessageInspectMetaState struct {
	Node  gen.Atom
	Meta  gen.Alias
	State map[string]string
}

// do send

type RequestDoSend struct {
	PID      gen.PID
	Priority gen.MessagePriority
	Message  any
}

type ResponseDoSend struct {
	Error error
}

type RequestDoSendMeta struct {
	Meta    gen.Alias
	Message any
}
type ResponseDoSendMeta struct {
	Error error
}

// do send exit

type RequestDoSendExit struct {
	PID    gen.PID
	Reason error
}
type ResponseDoSendExit struct {
	Error error
}

type RequestDoSendExitMeta struct {
	Meta   gen.Alias
	Reason error
}
type ResponseDoSendExitMeta struct {
	Error error
}

// do kill

type RequestDoKill struct {
	PID gen.PID
}
type ResponseDoKill struct {
	Error error
}

// do set log level

// node
type RequestDoSetLogLevel struct {
	Level gen.LogLevel
}
type ResponseDoSetLogLevel struct {
	Error error
}

// do set tracing sampler and flags (node-level)

type RequestDoSetNodeTracingSampler struct {
	Type  string  // "always", "disable", "ratio", "rate_limit"
	Rate  float64 // for ratio
	Limit int     // for rate_limit
}

type RequestDoSetProcessTracingSampler struct {
	PID   gen.PID
	Type  string
	Rate  float64
	Limit int
}

// process
type RequestDoSetProcessLogLevel struct {
	PID   gen.PID
	Level gen.LogLevel
}

// meta
type RequestDoSetMetaLogLevel struct {
	Meta  gen.Alias
	Level gen.LogLevel
}

// do set process settings

type RequestDoSetProcessSendPriority struct {
	PID      gen.PID
	Priority gen.MessagePriority
}

type RequestDoSetProcessCompression struct {
	PID     gen.PID
	Enabled bool
}

type RequestDoSetProcessCompressionType struct {
	PID  gen.PID
	Type gen.CompressionType
}

type RequestDoSetProcessCompressionLevel struct {
	PID   gen.PID
	Level gen.CompressionLevel
}

type RequestDoSetProcessCompressionThreshold struct {
	PID       gen.PID
	Threshold int
}

type RequestDoSetProcessKeepNetworkOrder struct {
	PID   gen.PID
	Order bool
}

type RequestDoSetProcessImportantDelivery struct {
	PID       gen.PID
	Important bool
}

// do set meta settings

type RequestDoSetMetaSendPriority struct {
	Meta     gen.Alias
	Priority gen.MessagePriority
}

// generic response for do-set operations
type ResponseDoSet struct {
	Error error
}

// do app lifecycle

type RequestDoAppStart struct {
	Name gen.Atom
	Mode gen.ApplicationMode
}
type ResponseDoAppStart struct {
	Error error
}

type RequestDoAppStop struct {
	Name  gen.Atom
	Force bool
}
type ResponseDoAppStop struct {
	Error error
}

type RequestDoAppUnload struct {
	Name gen.Atom
}
type ResponseDoAppUnload struct {
	Error error
}

// do one-shot inspect

type RequestDoInspect struct {
	PID   gen.PID
	Items []string
}
type ResponseDoInspect struct {
	State map[string]string
	Error error
}

type RequestDoInspectMeta struct {
	Meta  gen.Alias
	Items []string
}
type ResponseDoInspectMeta struct {
	State map[string]string
	Error error
}

// goroutine dump

type RequestDoGoroutines struct {
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

type ResponseDoGoroutines struct {
	Groups   []GoroutineGroup
	Total    int
	Filtered int
	Error    error
}

// heap profile

type RequestDoHeapProfile struct {
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

type ResponseDoHeapProfile struct {
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

// application list

type RequestInspectApplicationList struct{}
type ResponseInspectApplicationList struct {
	Event gen.Event
}

type MessageInspectApplicationList struct {
	Node         gen.Atom
	Applications map[gen.Atom]gen.ApplicationInfo
}

// application tree

type RequestDoAppTree struct {
	Application gen.Atom
	Limit       int
}
type ResponseDoAppTree struct {
	Node        gen.Atom
	Application gen.Atom
	Processes   []gen.ProcessShortInfo
	// Truncated is the number of application processes omitted because the
	// result hit the limit (0 means the whole tree was returned).
	Truncated int
	Error     error
}

// subtree rooted at a process

type RequestDoSubtree struct {
	PID   gen.PID
	Limit int
}
type ResponseDoSubtree struct {
	Node      gen.Atom
	PID       gen.PID
	Processes []gen.ProcessShortInfo
	Truncated bool
	Error     error
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

type RequestDoTypes struct{}

type ResponseDoTypes struct {
	Types []gen.RegisteredTypeInfo
	Error error
}
