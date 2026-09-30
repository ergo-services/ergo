package stage_test

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type nodeEgressActor struct {
	act.Actor

	child gen.PID
}

func factoryNodeEgressActor() gen.ProcessBehavior { return &nodeEgressActor{} }

func (a *nodeEgressActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "application":
		a.Node().ApplicationStart("unknown_app", gen.ApplicationOptions{})
		a.Node().ApplicationStopForce("unknown_app")

	case "kill":
		child, err := a.Spawn(factoryNodeEgressActor, gen.ProcessOptions{})
		if err != nil {
			return err
		}
		a.child = child
		a.Node().Kill(child)

	case "names":
		a.RegisterName("worker")
		a.UnregisterName()

	case "cron":
		a.Node().Cron().AddJob(gen.CronJob{
			Name:   "nightly",
			Spec:   "0 0 * * *",
			Action: gen.CreateCronActionMessage(a.PID(), gen.MessagePriorityNormal),
		})
		a.Node().Cron().DisableJob("nightly")
		a.Node().Cron().EnableJob("nightly")
		a.Node().Cron().RemoveJob("nightly")
	}
	return nil
}

func (a *nodeEgressActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "child" {
		return a.child, nil
	}
	if request == "ask" {
		return a.CallWithTimeout(gen.Atom("responder"), "ping", 30)
	}
	return "pong", nil
}

func TestStageRecordsNodeApplicationEgress(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("apps")
	pid := n.Spawn(factoryNodeEgressActor, gen.ProcessOptions{})

	n.Send(pid, "application")

	n.ShouldApplicationStart().From(pid).Name("unknown_app").
		ErrorIs(gen.ErrApplicationUnknown).Once().Within(time.Second).Must()
	n.ShouldApplicationStop().From(pid).Name("unknown_app").Force(true).
		Once().Within(time.Second).Must()
}

func TestStageRecordsKill(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("kill")
	pid := n.Spawn(factoryNodeEgressActor, gen.ProcessOptions{})

	n.Send(pid, "kill")
	n.ShouldKill().From(pid).Once().Within(time.Second).Must()

	child, err := n.Call(pid, "child")
	check.NoError(t, err)
	n.ShouldKill().From(pid).Target(child.(gen.PID)).Once().Within(time.Second).Must()
}

func TestStageRecordsNameRegistry(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("names")
	pid := n.Spawn(factoryNodeEgressActor, gen.ProcessOptions{})

	n.Send(pid, "names")

	n.ShouldRegisterName().From(pid).Name("worker").PID(pid).Once().Within(time.Second).Must()
	n.ShouldUnregisterName().From(pid).Name("worker").Once().Within(time.Second).Must()
}

func TestStageRecordsCron(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("cron")
	pid := n.Spawn(factoryNodeEgressActor, gen.ProcessOptions{})

	n.Send(pid, "cron")

	n.ShouldAddCronJob().From(pid).Name("nightly").Spec("0 0 * * *").Once().Within(time.Second).Must()
	n.ShouldDisableCronJob().From(pid).Name("nightly").Once().Within(time.Second).Must()
	n.ShouldEnableCronJob().From(pid).Name("nightly").Once().Within(time.Second).Must()
	n.ShouldRemoveCronJob().From(pid).Name("nightly").Once().Within(time.Second).Must()
}

func TestStageRecordsCallOptions(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("calls")
	n.SpawnRegister("responder", factoryNodeEgressActor, gen.ProcessOptions{})
	caller := n.Spawn(factoryNodeEgressActor, gen.ProcessOptions{})

	response, err := n.Call(caller, "ask")
	check.NoError(t, err)
	check.Equal(t, "pong", response)

	n.ShouldCall().From(caller).To(gen.Atom("responder")).Request("ping").
		Timeout(30).Once().Within(time.Second).Must()
}
