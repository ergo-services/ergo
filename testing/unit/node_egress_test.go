package unit_test

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

type neActor struct {
	act.Actor

	child     gen.PID
	killErr   error
	regErr    error
	unregPID  gen.PID
	loadName  gen.Atom
	callErr   error
	forwarded error
}

func factoryNEActor() gen.ProcessBehavior { return &neActor{} }

func (a *neActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "applications":
		a.Node().ApplicationStart("orders", gen.ApplicationOptions{})
		a.Node().ApplicationStartTemporary("audit", gen.ApplicationOptions{})
		a.Node().ApplicationStartTransient("billing", gen.ApplicationOptions{})
		a.Node().ApplicationStartPermanent("core", gen.ApplicationOptions{})
		a.Node().ApplicationStop("orders")
		a.Node().ApplicationStopForce("audit")
		a.Node().ApplicationStopWithTimeout("billing", 3*time.Second)
		a.Node().ApplicationUnload("core")

	case "load":
		name, err := a.Node().ApplicationLoad(nil)
		a.loadName = name
		a.callErr = err

	case "kill":
		child, err := a.Spawn(factoryNEActor, gen.ProcessOptions{})
		if err != nil {
			return err
		}
		a.child = child
		a.killErr = a.Node().Kill(child)

	case "names":
		a.regErr = a.RegisterName("worker")
		pid, _ := a.Node().UnregisterName("worker")
		a.unregPID = pid

	case "cron":
		a.Node().Cron().AddJob(gen.CronJob{Name: "nightly", Spec: "0 0 * * *"})
		a.Node().Cron().DisableJob("nightly")
		a.Node().Cron().EnableJob("nightly")
		a.Node().Cron().EnableJob("unknown")

	case "calls":
		a.CallWithTimeout("db", "q1", 30)
		a.CallWithPriority("db", "q2", gen.MessagePriorityHigh)
		a.CallImportant("db", "q3")

	case "forward":
		a.forwarded = a.Forward(a.PID(), &gen.MailboxMessage{From: from, Message: "fwd"}, gen.MessagePriorityMax)
	}
	return nil
}

func TestNodeApplicationEgressIsRecorded(t *testing.T) {
	sub, err := unit.Spawn(t, factoryNEActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "applications")

	sub.ShouldApplicationStart().From(sub.PID()).Name("orders").Mode(0).Once().Assert()
	sub.ShouldApplicationStart().Name("audit").Mode(gen.ApplicationModeTemporary).Once().Assert()
	sub.ShouldApplicationStart().Name("billing").Mode(gen.ApplicationModeTransient).Once().Assert()
	sub.ShouldApplicationStart().Name("core").Mode(gen.ApplicationModePermanent).Once().Assert()

	sub.ShouldApplicationStop().Name("orders").Force(false).Once().Assert()
	sub.ShouldApplicationStop().Name("audit").Force(true).Once().Assert()
	sub.ShouldApplicationStop().Name("billing").Timeout(3 * time.Second).Once().Assert()
	sub.ShouldApplicationUnload().Name("core").Once().Assert()
}

func TestNodeApplicationLoadIsStubbed(t *testing.T) {
	n := unit.StartNode(t, "unit@localhost", gen.NodeOptions{})
	n.OnApplicationLoad(func(app gen.ApplicationBehavior, args ...any) (gen.Atom, error) {
		return "orders", nil
	})
	sub, err := n.Spawn(factoryNEActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "load")

	check.Equal(t, gen.Atom("orders"), sub.Behavior().(*neActor).loadName)
	sub.ShouldApplicationLoad().Name("orders").Once().Assert()
}

func TestNodeKillIsRecorded(t *testing.T) {
	sub, err := unit.Spawn(t, factoryNEActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "kill")

	a := sub.Behavior().(*neActor)
	check.NoError(t, a.killErr)
	sub.ShouldKill().From(sub.PID()).Target(a.child).Once().Assert()
}

func TestNameRegistryIsRecorded(t *testing.T) {
	sub, err := unit.Spawn(t, factoryNEActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "names")

	a := sub.Behavior().(*neActor)
	check.NoError(t, a.regErr)
	check.Equal(t, sub.PID(), a.unregPID)

	sub.ShouldRegisterName().Name("worker").PID(sub.PID()).Once().Assert()
	sub.ShouldUnregisterName().Name("worker").PID(sub.PID()).Once().Assert()
}

func TestCronToggleIsRecorded(t *testing.T) {
	sub, err := unit.Spawn(t, factoryNEActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "cron")

	sub.ShouldAddCronJob().Name("nightly").Once().Assert()
	sub.ShouldDisableCronJob().Name("nightly").Once().Assert()
	sub.ShouldEnableCronJob().Name("nightly").Once().Assert()
	sub.ShouldEnableCronJob().Name("unknown").ErrorIs(gen.ErrUnknown).Once().Assert()
}

func TestCallOptionsAreRecorded(t *testing.T) {
	sub, err := unit.Spawn(t, factoryNEActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.OnCall("db").Respond("ok")
	sub.SendMessage(sub.PID(), "calls")

	sub.ShouldCall().To("db").Request("q1").Timeout(30).Once().Assert()
	sub.ShouldCall().To("db").Request("q2").Priority(gen.MessagePriorityHigh).Once().Assert()
	sub.ShouldCall().To("db").Request("q3").Important(true).Once().Assert()
	sub.ShouldCall().To("db").Request("q1").Priority(gen.MessagePriorityHigh).None().Assert()
}

func TestForwardPriorityIsRecorded(t *testing.T) {
	sub, err := unit.Spawn(t, factoryNEActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "forward")

	check.NoError(t, sub.Behavior().(*neActor).forwarded)
	sub.ShouldForward().To(sub.PID()).Priority(gen.MessagePriorityMax).Once().Assert()
}

func TestNodeAppliesProcessSettingsToTheSubject(t *testing.T) {
	sub, err := unit.Spawn(t, factoryNEActor, gen.ProcessOptions{})
	check.NoError(t, err)

	node := sub.Node()
	check.NoError(t, node.SetProcessSendPriority(sub.PID(), gen.MessagePriorityHigh))
	check.NoError(t, node.SetProcessImportantDelivery(sub.PID(), true))
	check.NoError(t, node.SetProcessKeepNetworkOrder(sub.PID(), false))
	check.NoError(t, node.SetProcessLogLevel(sub.PID(), gen.LogLevelError))

	sub.SendMessage(sub.PID(), "forward")
	sub.ShouldForward().Priority(gen.MessagePriorityMax).Once().Assert()

	err = node.SetProcessSendPriority(gen.PID{Node: "unit@localhost", ID: 9999}, gen.MessagePriorityHigh)
	check.ErrorIs(t, err, gen.ErrProcessUnknown)
}
