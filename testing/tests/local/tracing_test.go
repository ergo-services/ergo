package local

import (
	"errors"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/stage"
)

// commands for the business-span tracer.
type (
	cmdRoot       struct{}
	cmdNested     struct{}
	cmdSequential struct{}
	cmdError      struct{}
	cmdAttr       struct{}
	cmdSelfLeak   struct{} // does a sampled self-send so the leak runs inside a trace
	cmdLeak       struct{} // opens a span and returns without closing it
)

// bizTracer exercises StartTracingSpan from its handler. With sample=true it
// installs an always-on sampler, so a span opened in an untraced handler starts a
// fresh trace (self-start / initiator). With sample=false it never initiates a
// trace: StartTracingSpan only annotates a trace the handler already runs in, and
// is a no-op otherwise (passive instrumentation).
type bizTracer struct {
	act.Actor
	sample bool
}

func factoryBizTracer() gen.ProcessBehavior    { return &bizTracer{sample: true} }
func factoryBizTracerOff() gen.ProcessBehavior { return &bizTracer{sample: false} }

func (b *bizTracer) Init(args ...any) error {
	if b.sample {
		return b.SetTracingSampler(gen.TracingSamplerAlways)
	}
	return nil
}

func (b *bizTracer) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case cmdRoot:
		s := b.StartTracingSpan("root")
		s.End()
	case cmdNested:
		outer := b.StartTracingSpan("outer")
		inner := b.StartTracingSpan("inner")
		inner.End()
		outer.End()
	case cmdSequential:
		a := b.StartTracingSpan("seqA")
		a.End()
		c := b.StartTracingSpan("seqB")
		c.End()
	case cmdError:
		s := b.StartTracingSpan("failed")
		s.EndError(errors.New("boom"))
	case cmdAttr:
		s := b.StartTracingSpan("attr")
		s.SetAttribute("key", "value")
		s.End()
	case cmdSelfLeak:
		b.Send(b.PID(), cmdLeak{})
	case cmdLeak:
		b.StartTracingSpan("leaked") // intentionally left open
	}
	return nil
}

// sendCmd tells the sampling sender to forward Msg to To (carrying a trace).
type sendCmd struct {
	To  gen.PID
	Msg any
}

// samplingSender has its own sampler, so a message it sends starts a trace; used
// to give a passive actor an already-active trace to annotate.
type samplingSender struct{ act.Actor }

func factorySamplingSender() gen.ProcessBehavior { return &samplingSender{} }

func (s *samplingSender) Init(args ...any) error {
	return s.SetTracingSampler(gen.TracingSamplerAlways)
}

func (s *samplingSender) HandleMessage(from gen.PID, message any) error {
	if c, ok := message.(sendCmd); ok {
		return s.Send(c.To, c.Msg)
	}
	return nil
}

// TestBusinessSpans covers Process.StartTracingSpan via the shared ShouldSpan
// grammar: self-start at an initiator, nesting, per-callback isolation, EndError,
// attributes, auto-close of a span left open in a traced handler, passive
// annotation of an inherited trace, and the no-trace no-op.
func TestBusinessSpans(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	pid := n.Spawn(factoryBizTracer, gen.ProcessOptions{}) // initiator (has a sampler)
	const wait = 2 * time.Second

	t.Run("SelfStartRoot", func(t *testing.T) {
		mk := n.Mark()
		n.Send(pid, cmdRoot{})
		root, ok := n.ShouldSpan().Named("root").From(pid).Since(mk).Within(wait).Capture()
		if ok == false {
			t.Fatal("root span not recorded")
		}
		if root.TraceID == [2]uint64{} {
			t.Error("self-started span must carry a trace id")
		}
		if root.ParentSpanID != 0 {
			t.Errorf("self-started root must have no parent, got %d", root.ParentSpanID)
		}
		// a business span carries an interval (EndTimestamp set, not before start);
		// a point observation would have EndTimestamp == 0. Zero duration is fine.
		if root.Timestamp == 0 || root.EndTimestamp < root.Timestamp {
			t.Errorf("span must carry an interval: start=%d end=%d", root.Timestamp, root.EndTimestamp)
		}
	})

	t.Run("Nested", func(t *testing.T) {
		mk := n.Mark()
		n.Send(pid, cmdNested{})
		outer, ok1 := n.ShouldSpan().Named("outer").Since(mk).Within(wait).Capture()
		inner, ok2 := n.ShouldSpan().Named("inner").Since(mk).Within(wait).Capture()
		if ok1 == false || ok2 == false {
			t.Fatal("nested spans not recorded")
		}
		if inner.ParentSpanID != outer.SpanID {
			t.Errorf("inner.parent=%d, want outer=%d", inner.ParentSpanID, outer.SpanID)
		}
		if outer.ParentSpanID != 0 {
			t.Errorf("outer must be root, parent=%d", outer.ParentSpanID)
		}
		if inner.TraceID != outer.TraceID {
			t.Error("nested spans must share the trace id")
		}
	})

	t.Run("SequentialDoNotMerge", func(t *testing.T) {
		mk := n.Mark()
		n.Send(pid, cmdSequential{})
		a, ok1 := n.ShouldSpan().Named("seqA").Since(mk).Within(wait).Capture()
		b, ok2 := n.ShouldSpan().Named("seqB").Since(mk).Within(wait).Capture()
		if ok1 == false || ok2 == false {
			t.Fatal("sequential spans not recorded")
		}
		if a.TraceID == b.TraceID {
			t.Error("sequential self-started spans must be separate traces")
		}
		if a.ParentSpanID != 0 || b.ParentSpanID != 0 {
			t.Error("sequential top-level spans must each be a root")
		}
	})

	t.Run("EndError", func(t *testing.T) {
		mk := n.Mark()
		n.Send(pid, cmdError{})
		n.ShouldSpan().Named("failed").Error("boom").Since(mk).Within(wait).Once().Must()
	})

	t.Run("SetAttribute", func(t *testing.T) {
		mk := n.Mark()
		n.Send(pid, cmdAttr{})
		n.ShouldSpan().Named("attr").WithAttribute("key", "value").Since(mk).Within(wait).Once().Must()
	})

	t.Run("AutoCloseLeftoverInTracedHandler", func(t *testing.T) {
		mk := n.Mark()
		n.Send(pid, cmdSelfLeak{})
		n.ShouldSpan().Named("leaked").WithAttribute("ergo.span.unended", "true").
			Since(mk).Within(wait).Once().Must()
	})

	t.Run("PassiveAnnotatesInheritedTrace", func(t *testing.T) {
		off := n.Spawn(factoryBizTracerOff, gen.ProcessOptions{}) // no sampler of its own
		snd := n.Spawn(factorySamplingSender, gen.ProcessOptions{})
		mk := n.Mark()
		// snd has a sampler, so its send to off carries a trace; off, with no
		// sampler, opens a span under that inherited trace without initiating one.
		n.Send(snd, sendCmd{To: off, Msg: cmdRoot{}})
		sp, ok := n.ShouldSpan().Named("root").From(off).Since(mk).Within(wait).Capture()
		if ok == false {
			t.Fatal("passive span not recorded under inherited trace")
		}
		if sp.TraceID == [2]uint64{} {
			t.Error("passive span must carry the inherited trace id")
		}
	})

	t.Run("NoTraceNoSpan", func(t *testing.T) {
		off := n.Spawn(factoryBizTracerOff, gen.ProcessOptions{}) // no sampler
		mk := n.Mark()
		n.Send(off, cmdRoot{}) // untraced delivery, no sampler -> no trace, no span
		n.ShouldDeliver().To(off).Message(cmdRoot{}).Since(mk).Within(wait).Once().Must()
		n.ShouldSpan().Named("root").From(off).Since(mk).None().Assert()
	})
}
