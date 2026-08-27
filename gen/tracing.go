package gen

import (
	"fmt"
)

// Tracing carries trace identity through message chains.
// Zero value (ID == [2]uint64{}) means no active tracing.
type Tracing struct {
	ID       [2]uint64
	SpanID   uint64
	Behavior string
}

// TracingFlags controls tracing granularity.
type TracingFlags uint32

const (
	TracingFlagSend    TracingFlags = 1 << 0 // trace send/call/response
	TracingFlagReceive TracingFlags = 1 << 1 // trace delivered to mailbox
	TracingFlagProcs   TracingFlags = 1 << 2 // trace spawn/terminate
	TracingFlagInherit TracingFlags = 1 << 3 // children inherit tracing
)

// Names lists the flags that are set, in declaration order. A bit this build does not know is
// reported as flag#N rather than dropped, so reading a newer node loses nothing.
func (tf TracingFlags) Names() []string {
	out := []string{}
	rest := tf
	for _, known := range []struct {
		flag TracingFlags
		name string
	}{
		{TracingFlagSend, "send"},
		{TracingFlagReceive, "receive"},
		{TracingFlagProcs, "procs"},
		{TracingFlagInherit, "inherit"},
	} {
		if tf&known.flag != 0 {
			out = append(out, known.name)
			rest &^= known.flag
		}
	}
	for bit := 0; bit < 32; bit++ {
		if rest&(TracingFlags(1)<<uint(bit)) != 0 {
			out = append(out, fmt.Sprintf("flag#%d", bit))
		}
	}
	return out
}

// String joins the set flags, so a log line reads "send|procs" instead of a number.
func (tf TracingFlags) String() string {
	names := tf.Names()
	if len(names) == 0 {
		return ""
	}
	out := names[0]
	for _, name := range names[1:] {
		out += "|" + name
	}
	return out
}

// TracingSpan represents a single observation point
// in the message lifecycle.
type TracingSpan struct {
	TraceID      [2]uint64
	SpanID       uint64
	ParentSpanID uint64       // SpanID from propagating trace context (0 = root)
	ParentPoint  TracingPoint // point of the parent observation (business spans only)
	Point        TracingPoint
	Kind         TracingKind
	Timestamp    int64 // wall clock unix nanoseconds
	EndTimestamp int64 // interval end (business spans); 0 = point observation
	Node         Atom
	From         PID
	To           any // PID, ProcessID, or Alias
	Ref          Ref
	Behavior     string             // behavior name of the emitting process
	Message      string             // type name of the message
	Error        string             // empty = no error
	Attributes   []TracingAttribute // custom attributes, nil = none
}

// TracingAttribute is a key-value pair attached to a tracing span.
type TracingAttribute struct {
	Key   string
	Value string
}

// TracingPoint identifies where in the lifecycle the observation occurred.
type TracingPoint int

const (
	TracingPointSent      TracingPoint = 1
	TracingPointDelivered TracingPoint = 2
	TracingPointProcessed TracingPoint = 3
	TracingPointSpan      TracingPoint = 4 // business span opened with StartTracingSpan
)

func (tp TracingPoint) String() string {
	switch tp {
	case TracingPointSent:
		return "sent"
	case TracingPointDelivered:
		return "delivered"
	case TracingPointProcessed:
		return "processed"
	case TracingPointSpan:
		return "span"
	}
	return fmt.Sprintf("point#%d", int(tp))
}

// TracingKind identifies the type of operation being traced.
type TracingKind int

const (
	TracingKindSend      TracingKind = 1
	TracingKindRequest   TracingKind = 2
	TracingKindResponse  TracingKind = 3
	TracingKindSpawn     TracingKind = 4
	TracingKindTerminate TracingKind = 5
)

func (tk TracingKind) String() string {
	switch tk {
	case TracingKindSend:
		return "send"
	case TracingKindRequest:
		return "request"
	case TracingKindResponse:
		return "response"
	case TracingKindSpawn:
		return "spawn"
	case TracingKindTerminate:
		return "terminate"
	}
	if tk == 0 {
		return "" // business span - no message kind
	}
	return fmt.Sprintf("kind#%d", int(tk))
}

// TracingInfo contains tracing configuration for a process or node.
type TracingInfo struct {
	Sampler    string
	Attributes []TracingAttribute
}

// TracingBehavior interface for tracing exporters.
type TracingBehavior interface {
	// HandleSpan processes one span. The node calls it from a dedicated per-exporter
	// worker (serially, so it need not be goroutine-safe), decoupled from the routing
	// path - it may do I/O without stalling message delivery. Spans are buffered; a
	// persistently slow exporter drops spans once the buffer fills (see
	// TracingExporterInfo.DroppedSpans), so drain promptly for lossless export.
	HandleSpan(TracingSpan)

	// Terminate is called when the exporter is removed or the node stops, after the
	// worker has finished; use it to flush and clean up.
	Terminate()
}

// TracingSpanScope is an open business span started with Process.StartTracingSpan.
// Close it with End or EndError (defer it). All methods are no-ops if the scope
// was created without an active trace.
type TracingSpanScope interface {
	// SetAttribute attaches a key/value to the span. The "ergo." prefix is reserved.
	SetAttribute(key, value string)
	// End closes the span successfully.
	End()
	// EndError closes the span with an error.
	EndError(err error)
}

type tracingSpanScopeNoop struct{}

func (tracingSpanScopeNoop) SetAttribute(key, value string) {}
func (tracingSpanScopeNoop) End()                           {}
func (tracingSpanScopeNoop) EndError(err error)             {}

// TracingSpanScopeNoop is a TracingSpanScope that does nothing. Returned by
// StartTracingSpan when no trace is active.
var TracingSpanScopeNoop TracingSpanScope = tracingSpanScopeNoop{}

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
