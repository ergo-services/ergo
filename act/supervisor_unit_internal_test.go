package act

// Internal-package supervisor unit tests: these need access to unexported message
// types (supMessageChildStart/Terminate) that the external act_test suite cannot
// construct. They use the public testing/unit harness all the same.

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

type supPlain struct{ Supervisor }

func (s *supPlain) Init(args ...any) (SupervisorSpec, error) {
	return args[0].(SupervisorSpec), nil
}

func factorySupPlain() gen.ProcessBehavior { return &supPlain{} }

// With EnableHandleChild, delivering the child-start / child-terminate notifications
// routes to the default HandleChildStart / HandleChildTerminate (both return nil, so
// the supervisor survives).
func TestSupervisorUnitDefaultHandleChild(t *testing.T) {
	spec := SupervisorSpec{
		Type:                SupervisorTypeOneForOne,
		EnableHandleChild:   true,
		DisableAutoShutdown: true,
		Children:            []SupervisorChildSpec{{Name: "a", Factory: func() gen.ProcessBehavior { return &Actor{} }}},
	}
	s, err := unit.Spawn(t, factorySupPlain, gen.ProcessOptions{}, spec)
	if err != nil {
		t.Fatal(err)
	}

	pid := gen.PID{Node: "unit@localhost", ID: 1001, Creation: 1}
	s.SendMessage(gen.PID{}, supMessageChildStart{name: "a", pid: pid})
	s.SendMessage(gen.PID{}, supMessageChildTerminate{name: "a", pid: pid, reason: errors.New("x")})

	if s.Terminated() {
		t.Error("supervisor must survive the child notifications")
	}
}
