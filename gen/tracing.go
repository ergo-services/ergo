package gen

// Tracing carries trace identity through message chains.
// Zero value (ID == [2]uint64{}) means no active tracing.
type Tracing struct {
	ID     [2]uint64
	SpanID uint64
}

// TracingFlags controls tracing granularity.
type TracingFlags uint32

const (
	TracingFlagSend    TracingFlags = 1 << 0 // trace send/call/response
	TracingFlagReceive TracingFlags = 1 << 1 // trace delivered to mailbox
	TracingFlagProcs   TracingFlags = 1 << 2 // trace spawn/terminate
	TracingFlagInherit TracingFlags = 1 << 3 // children inherit tracing
)

// TracingSpan represents a single observation point
// in the message lifecycle.
type TracingSpan struct {
	TraceID   [2]uint64
	SpanID    uint64
	Point     TracingPoint
	Kind      TracingKind
	Timestamp int64 // wall clock unix nanoseconds
	Node      Atom
	From      PID
	To        any    // PID, ProcessID, or Alias
	Ref       Ref
	Message   string // type name of the message
	Error     string // empty = no error
}

// TracingPoint identifies where in the lifecycle the observation occurred.
type TracingPoint int

const (
	TracingPointSent      TracingPoint = 1
	TracingPointDelivered TracingPoint = 2
	TracingPointProcessed TracingPoint = 3
)

// TracingKind identifies the type of operation being traced.
type TracingKind int

const (
	TracingKindSend      TracingKind = 1
	TracingKindRequest   TracingKind = 2
	TracingKindResponse  TracingKind = 3
	TracingKindSpawn     TracingKind = 4
	TracingKindTerminate TracingKind = 5
)

// TracingBehavior interface for tracing exporters.
type TracingBehavior interface {
	HandleSpan(TracingSpan)
	Terminate()
}

// TracingExporter defines a named exporter with its flags.
type TracingExporter struct {
	Name     string
	Exporter TracingBehavior
	Flags    TracingFlags
}

// TracingOptions configures tracing at node startup.
type TracingOptions struct {
	Exporters []TracingExporter
}
