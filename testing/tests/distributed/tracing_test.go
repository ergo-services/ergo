package distributed

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// tracingSender installs an always-on sampler, so a send it makes starts (and
// propagates over the wire) a trace.
type tracingSender struct{ act.Actor }

func factoryTracingSender() gen.ProcessBehavior { return &tracingSender{} }

func (s *tracingSender) Init(args ...any) error {
	return s.SetTracingSampler(gen.TracingSamplerAlways)
}

func (s *tracingSender) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	c := request.(sendCmd)
	return errText(s.Send(c.To, c.Msg)), nil
}

// traceRcv is a plain receiver on the remote node.
type traceRcv struct{ act.Actor }

func factoryTraceRcv() gen.ProcessBehavior { return &traceRcv{} }

// TestDistTracingSpanIDPreservedCrossNode: a traced message that crosses a node
// boundary keeps one ergo SpanID. The Sent span on the sender node and the
// Delivered span on the receiver node carry the SAME SpanID (and TraceID) -
// the sender-assigned SpanID travels with the message and the receiver reuses it.
// Minting a fresh SpanID on the receiver would split the span and break both the
// observer waterfall (which groups points by SpanID) and the pulse OTLP mapping
// (Delivered's parent is the same-SpanID Sent).
func TestDistTracingSpanIDPreservedCrossNode(t *testing.T) {
	s := stage.New(t)
	n1 := s.StartNode("n1")
	n2 := s.StartNode("n2")
	s.Connect(n1, n2)

	n2.SpawnRegister("rcv", factoryTraceRcv, gen.ProcessOptions{})
	snd := n1.Spawn(factoryTracingSender, gen.ProcessOptions{})

	// the sender (with a sampler) sends cross-node; only this message is traced.
	res, err := n1.Call(snd, sendCmd{Kind: "send", To: n2.ProcessID("rcv"), Msg: "ping"})
	check.NoError(t, err)
	if res != "" {
		t.Fatalf("cross-node send failed: %v", res)
	}

	const wait = 2 * time.Second
	sent, ok := n1.ShouldSpan().Point(gen.TracingPointSent).From(snd).Within(wait).Capture()
	if ok == false {
		t.Fatal("no Sent span recorded on n1 for the cross-node send")
	}
	delivered, ok := n2.ShouldSpan().Point(gen.TracingPointDelivered).From(snd).Within(wait).Capture()
	if ok == false {
		t.Fatal("no Delivered span recorded on n2 for the cross-node send")
	}

	if sent.SpanID == 0 {
		t.Fatal("Sent span has a zero SpanID")
	}
	if sent.SpanID != delivered.SpanID {
		t.Fatalf("ergo SpanID not preserved across the wire: sent(n1)=%d delivered(n2)=%d", sent.SpanID, delivered.SpanID)
	}
	if sent.TraceID != delivered.TraceID {
		t.Error("Sent and Delivered must share the trace id across the wire")
	}
	if sent.Node != n1.Name() || delivered.Node != n2.Name() {
		t.Errorf("span nodes: sent=%s (want %s), delivered=%s (want %s)",
			sent.Node, n1.Name(), delivered.Node, n2.Name())
	}
}
