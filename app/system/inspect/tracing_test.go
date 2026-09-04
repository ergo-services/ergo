package inspect

import (
	"fmt"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

func spawnTracingInspector(t *testing.T, pattern string, exclude bool) *unit.Subject {
	t.Helper()

	node := unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
	node.OnTracingExporterAddPID(func(gen.PID, string, gen.TracingFlags) error { return nil })
	node.OnTracingExporterDeletePID(func(gen.PID) {})

	sub, err := node.SpawnRegister(gen.Atom(inspectTracing), factory_tracing, gen.ProcessOptions{},
		gen.TracingFlagSend, 10, uint32(0), uint32(0), pattern, exclude)
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}
	return sub
}

func tracingEvent(sub *unit.Subject) gen.Atom {
	return gen.Atom(fmt.Sprintf("%s_%s", inspectTracing, sub.PID()))
}

func TestTracingInspectorRegistersItsEventOnInit(t *testing.T) {
	sub := spawnTracingInspector(t, "", false)

	sub.ShouldRegisterEvent().Name(tracingEvent(sub)).Once().Assert()
}

func TestTracingInspectorBecomesAnExporterOnlyWhileWatched(t *testing.T) {
	added := 0
	deleted := 0

	node := unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
	node.OnTracingExporterAddPID(func(gen.PID, string, gen.TracingFlags) error { added++; return nil })
	node.OnTracingExporterDeletePID(func(gen.PID) { deleted++ })
	sub, err := node.SpawnRegister(gen.Atom(inspectTracing), factory_tracing, gen.ProcessOptions{},
		gen.TracingFlagSend, 10, uint32(0), uint32(0), "", false)
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}

	if added != 0 {
		t.Fatal("the inspector became an exporter before anyone subscribed, taxing every message on the node")
	}

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: tracingEvent(sub)})
	if added != 1 {
		t.Fatalf("registered %d times on the first subscriber, want once", added)
	}

	sub.SendMessage(gen.PID{}, gen.MessageEventStop{Name: tracingEvent(sub)})
	if deleted != 1 {
		t.Fatalf("unregistered %d times when the last subscriber left, want once", deleted)
	}
}

func TestTracingInspectorPublishesWhatItCollected(t *testing.T) {
	sub := spawnTracingInspector(t, "", false)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: tracingEvent(sub)})
	sub.DeliverSpan(gen.TracingSpan{SpanID: 1, Message: "Query"})
	sub.FireTimers()
	sub.Drain()

	sub.ShouldSendEvent().Name(tracingEvent(sub)).AtLeast(1).Assert()
}

func TestTracingInspectorPublishesNothingWithoutSpans(t *testing.T) {
	sub := spawnTracingInspector(t, "", false)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: tracingEvent(sub)})
	mark := sub.Mark()
	sub.FireTimers()
	sub.Drain()

	sub.ShouldSendEvent().Since(mark).None().Assert()
}

func TestTracingInspectorKeepsOnlyWhatMatchesThePattern(t *testing.T) {
	sub := spawnTracingInspector(t, "query", false)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: tracingEvent(sub)})
	sub.DeliverSpan(gen.TracingSpan{SpanID: 1, Message: "Heartbeat"})
	mark := sub.Mark()
	sub.FireTimers()
	sub.Drain()

	sub.ShouldSendEvent().Since(mark).None().Assert()
}

func TestTracingInspectorDropsWhatMatchesAnExcludingPattern(t *testing.T) {
	sub := spawnTracingInspector(t, "heartbeat", true)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: tracingEvent(sub)})
	sub.DeliverSpan(gen.TracingSpan{SpanID: 1, Message: "Heartbeat"})
	mark := sub.Mark()
	sub.FireTimers()
	sub.Drain()

	sub.ShouldSendEvent().Since(mark).None().Assert()
}

func TestTracingInspectorAnswersAnInspectRequest(t *testing.T) {
	sub := spawnTracingInspector(t, "", false)

	client := gen.PID{Node: "inspect@localhost", ID: 100}
	sub.SendMessage(gen.PID{}, requestInspect{pid: client, ref: gen.Ref{}})

	sub.ShouldSendResponse().To(client).Once().Assert()
}

func TestTracingInspectorShutsDownWhenNobodyEverSubscribed(t *testing.T) {
	sub := spawnTracingInspector(t, "", false)

	sub.SendMessage(gen.PID{}, shutdown{})

	if sub.Terminated() == false {
		t.Fatal("an inspector nobody subscribed to stayed alive, holding its event forever")
	}
}

func TestTracingInspectorIgnoresShutdownWhileWatched(t *testing.T) {
	sub := spawnTracingInspector(t, "", false)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: tracingEvent(sub)})
	sub.SendMessage(gen.PID{}, shutdown{})

	if sub.Terminated() {
		t.Fatal("the inspector shut down under a live subscriber")
	}
}
