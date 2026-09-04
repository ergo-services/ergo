package act_test

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

type faulty struct{ act.Actor }

func factoryFaulty() gen.ProcessBehavior { return &faulty{} }

func (f *faulty) Init(args ...any) error {
	if len(args) > 0 && args[0] == "init" {
		var p *int
		_ = *p
	}
	return nil
}

func (f *faulty) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "divide":
		a, b := 1, 0
		_ = a / b
	case "nil":
		var p *int
		_ = *p
	case "bounds":
		s := []int{}
		_ = s[3]
	case "assert":
		var v any = "s"
		_ = v.(int)
	case "explicit":
		panic("boom")
	}
	return nil
}

func TestActorPanicIsReportedAtTheFaultingLine(t *testing.T) {
	for _, kind := range []string{"divide", "nil", "bounds", "assert", "explicit"} {
		t.Run(kind, func(t *testing.T) {
			sub, err := unit.Spawn(t, factoryFaulty, gen.ProcessOptions{})
			check.NoError(t, err)

			sub.SendMessage(gen.PID{}, kind)

			sub.ShouldLog().Level(gen.LogLevelPanic).
				Containing("act_test.(*faulty).HandleMessage[").
				Containing("panic_origin_test.go:").
				Once().Assert()
		})
	}
}

func TestActorInitPanicIsReportedAtTheFaultingLine(t *testing.T) {
	sub := unit.StartNode(t, "unit@localhost", gen.NodeOptions{}).
		Prepare(factoryFaulty, gen.ProcessOptions{}, "init")
	check.Error(t, sub.Run())

	sub.ShouldLog().Level(gen.LogLevelPanic).
		Containing("act_test.(*faulty).Init[").
		Containing("panic_origin_test.go:").
		Once().Assert()
}
