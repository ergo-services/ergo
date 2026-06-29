package unit_test

import (
	"errors"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

// spanActor opens a business span per command. The unit harness records the span
// on close regardless of a sampler (it observes the actor's instrumentation intent,
// like every other egress), so the actor's tracing can be asserted in isolation.
type spanActor struct{ act.Actor }

func factorySpanActor() gen.ProcessBehavior { return &spanActor{} }

func (a *spanActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "work":
		s := a.StartTracingSpan("do-work")
		s.SetAttribute("step", "one")
		s.End()
	case "fail":
		s := a.StartTracingSpan("risky")
		s.EndError(errors.New("nope"))
	}
	return nil
}

// TestUnitBusinessSpan asserts an actor's StartTracingSpan instrumentation through
// the shared ShouldSpan grammar in the in-process unit harness.
func TestUnitBusinessSpan(t *testing.T) {
	actor, err := unit.Spawn(t, factorySpanActor, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	actor.SendMessage(gen.PID{}, "work")
	actor.ShouldSpan().Named("do-work").WithAttribute("step", "one").Once().Assert()

	actor.SendMessage(gen.PID{}, "fail")
	actor.ShouldSpan().Named("risky").Error("nope").Once().Assert()
}
