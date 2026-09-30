package unit_test

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

var trace = gen.Tracing{ID: [2]uint64{7, 8}, SpanID: 9}

type trActor struct {
	act.Actor

	seen    gen.Tracing
	attrs   []gen.TracingAttribute
	sampler gen.TracingSampler
}

func factoryTRActor() gen.ProcessBehavior { return &trActor{} }

func (a *trActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "propagate":
		a.SetPropagatingTrace(trace)
		a.seen = a.PropagatingTrace()
		a.Send(gen.Atom("downstream"), "payload")

	case "attributes":
		a.SetTracingAttribute("tenant", "acme")
		a.SetTracingAttribute("tenant", "globex")
		a.SetTracingAttribute("region", "eu")
		a.SetTracingAttribute("ergo.reserved", "no")
		a.RemoveTracingAttribute("region")
		a.attrs = a.TracingAttributes()
		a.attrs = append(a.attrs, gen.TracingAttribute{Key: "local", Value: "only"})

	case "span-attributes":
		a.SetTracingAttribute("tenant", "acme")
		s := a.StartTracingSpan("checkout")
		a.SetTracingSpanAttribute("step", "one")
		s.End()

	case "span-cleared":
		s := a.StartTracingSpan("cleared")
		a.SetTracingSpanAttribute("step", "one")
		a.ClearTracingSpanAttributes()
		s.End()

	case "span-left-open":
		a.StartTracingSpan("forgotten")
		a.CloseTracingSpans()

	case "hand-built-span":
		a.SendTracingSpan(gen.TracingSpan{
			From: a.PID(), Message: "manual", Point: gen.TracingPointSpan,
			TraceID: trace.ID, SpanID: 42,
		})

	case "sampler":
		a.SetTracingSampler(nil)
		a.sampler = a.TracingSampler()
	}
	return nil
}

func TestPropagatingTraceIsKeptAndRidesTheSend(t *testing.T) {
	sub, err := unit.Spawn(t, factoryTRActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "propagate")

	check.Equal(t, trace, sub.Behavior().(*trActor).seen)

	sent, ok := sub.ShouldSend().To(gen.Atom("downstream")).Capture()
	check.True(t, ok)
	check.Equal(t, trace, sent.Options.Tracing)
}

func TestTracingAttributesAreKept(t *testing.T) {
	sub, err := unit.Spawn(t, factoryTRActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "attributes")

	a := sub.Behavior().(*trActor)
	check.Equal(t, []gen.TracingAttribute{
		{Key: "tenant", Value: "globex"},
		{Key: "local", Value: "only"},
	}, a.attrs)
	check.Equal(t, []gen.TracingAttribute{{Key: "tenant", Value: "globex"}}, a.TracingAttributes())
}

func TestSpanCarriesProcessAndSpanAttributes(t *testing.T) {
	sub, err := unit.Spawn(t, factoryTRActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "span-attributes")

	sub.ShouldSpan().Named("checkout").
		WithAttribute("step", "one").WithAttribute("tenant", "acme").Once().Assert()
}

func TestSpanAttributesCleared(t *testing.T) {
	sub, err := unit.Spawn(t, factoryTRActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "span-cleared")

	sub.ShouldSpan().Named("cleared").Once().Assert()
	sub.ShouldSpan().Named("cleared").WithAttribute("step", "one").None().Assert()
}

func TestCloseTracingSpansClosesWhatIsOpen(t *testing.T) {
	sub, err := unit.Spawn(t, factoryTRActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "span-left-open")

	sub.ShouldSpan().Named("forgotten").Once().Assert()
}

func TestHandBuiltSpanIsRecorded(t *testing.T) {
	sub, err := unit.Spawn(t, factoryTRActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "hand-built-span")

	span, ok := sub.ShouldSpan().Named("manual").Capture()
	check.True(t, ok)
	check.Equal(t, trace.ID, span.TraceID)
	check.Equal(t, uint64(42), span.SpanID)
}
