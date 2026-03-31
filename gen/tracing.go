package gen

import (
	"encoding/json"
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

// TracingSpan represents a single observation point
// in the message lifecycle.
type TracingSpan struct {
	TraceID      [2]uint64
	SpanID       uint64
	ParentSpanID uint64 // SpanID from propagating trace context (0 = root)
	Point        TracingPoint
	Kind         TracingKind
	Timestamp    int64 // wall clock unix nanoseconds
	Node         Atom
	From         PID
	To           any    // PID, ProcessID, or Alias
	Ref          Ref
	Behavior     string // behavior name of the emitting process
	Message      string // type name of the message
	Error        string // empty = no error
}

func (ts TracingSpan) MarshalJSON() ([]byte, error) {
	type alias struct {
		TraceID      string       `json:"TraceID"`
		SpanID       string       `json:"SpanID"`
		ParentSpanID string       `json:"ParentSpanID,omitempty"`
		Point        TracingPoint `json:"Point"`
		Kind         TracingKind  `json:"Kind"`
		Timestamp    int64        `json:"Timestamp"`
		Node         Atom         `json:"Node"`
		From         PID          `json:"From"`
		To           any          `json:"To"`
		Ref          Ref          `json:"Ref"`
		Behavior     string       `json:"Behavior,omitempty"`
		Message      string       `json:"Message"`
		Error        string       `json:"Error,omitempty"`
	}
	a := alias{
		TraceID:   fmt.Sprintf("%016x%016x", ts.TraceID[0], ts.TraceID[1]),
		SpanID:    fmt.Sprintf("%016x", ts.SpanID),
		Point:     ts.Point,
		Kind:      ts.Kind,
		Timestamp: ts.Timestamp,
		Node:      ts.Node,
		From:      ts.From,
		To:        ts.To,
		Ref:       ts.Ref,
		Behavior:  ts.Behavior,
		Message:   ts.Message,
		Error:     ts.Error,
	}
	if ts.ParentSpanID != 0 {
		a.ParentSpanID = fmt.Sprintf("%016x", ts.ParentSpanID)
	}
	return json.Marshal(a)
}

// TracingPoint identifies where in the lifecycle the observation occurred.
type TracingPoint int

const (
	TracingPointSent      TracingPoint = 1
	TracingPointDelivered TracingPoint = 2
	TracingPointProcessed TracingPoint = 3
)

func (tp TracingPoint) String() string {
	switch tp {
	case TracingPointSent:
		return "sent"
	case TracingPointDelivered:
		return "delivered"
	case TracingPointProcessed:
		return "processed"
	}
	return fmt.Sprintf("point#%d", int(tp))
}

func (tp TracingPoint) MarshalJSON() ([]byte, error) {
	return []byte(`"` + tp.String() + `"`), nil
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
	return fmt.Sprintf("kind#%d", int(tk))
}

func (tk TracingKind) MarshalJSON() ([]byte, error) {
	return []byte(`"` + tk.String() + `"`), nil
}

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
