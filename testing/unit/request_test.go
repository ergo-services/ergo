package unit_test

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

type reqActor struct {
	act.Actor

	refs    []gen.Ref
	answers []gen.MessageResponse
}

func factoryReqActor() gen.ProcessBehavior { return &reqActor{} }

func (a *reqActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "ask":
		ref, err := a.SendRequestWithLabel(gen.Atom("db"), "q", "orders")
		if err != nil {
			return err
		}
		a.refs = append(a.refs, ref)

	case "ask-many":
		for _, node := range []gen.Atom{"a", "b", "c"} {
			ref, err := a.SendRequestWithLabel(gen.ProcessID{Name: "inspector", Node: node}, "info", node)
			if err != nil {
				return err
			}
			a.refs = append(a.refs, ref)
		}

	case "ask-self":
		if _, err := a.SendRequest(a.PID(), "q"); err != gen.ErrNotAllowed {
			return gen.ErrIncorrect
		}

	case "ask-and-cancel":
		ref, err := a.SendRequest(gen.Atom("db"), "q")
		if err != nil {
			return err
		}
		return a.CancelRequest(ref)
	}
	return nil
}

func (a *reqActor) HandleResponse(response gen.MessageResponse) error {
	a.answers = append(a.answers, response)
	return nil
}

func TestRequestIsRecordedAndAnswered(t *testing.T) {
	sub, err := unit.Spawn(t, factoryReqActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "ask")

	sub.ShouldSendRequest().From(sub.PID()).To(gen.Atom("db")).Request("q").
		Label("orders").Timeout(gen.DefaultRequestTimeout).Once().Assert()

	a := sub.Behavior().(*reqActor)
	sub.DeliverResponse(gen.PID{Node: "unit@localhost", ID: 7}, a.refs[0], "answer", nil)

	check.Equal(t, 1, len(a.answers))
	check.Equal(t, "answer", a.answers[0].Result)
	check.Equal(t, "orders", a.answers[0].Label)
	check.NoError(t, a.answers[0].Error)
}

func TestRequestExpires(t *testing.T) {
	sub, err := unit.Spawn(t, factoryReqActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "ask")
	a := sub.Behavior().(*reqActor)
	sub.ExpireRequest(a.refs[0])

	check.Equal(t, 1, len(a.answers))
	check.ErrorIs(t, a.answers[0].Error, gen.ErrTimeout)
	check.Equal(t, "orders", a.answers[0].Label)
}

func TestRequestFanOutKeepsLabels(t *testing.T) {
	sub, err := unit.Spawn(t, factoryReqActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "ask-many")
	sub.ShouldSendRequest().Times(3).Assert()
	sub.ShouldSendRequest().Label(gen.Atom("b")).Once().Assert()

	a := sub.Behavior().(*reqActor)
	sub.DeliverResponse(gen.PID{}, a.refs[1], "info-b", nil)
	sub.ExpireRequest(a.refs[0])

	check.Equal(t, 2, len(a.answers))
	check.Equal(t, gen.Atom("b"), a.answers[0].Label)
	check.Equal(t, "info-b", a.answers[0].Result)
	check.Equal(t, gen.Atom("a"), a.answers[1].Label)
	check.ErrorIs(t, a.answers[1].Error, gen.ErrTimeout)
}

func TestRequestToSelfRefused(t *testing.T) {
	sub, err := unit.Spawn(t, factoryReqActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "ask-self")
	check.Equal(t, false, sub.Terminated())
	sub.ShouldSendRequest().None().Assert()
}

func TestRequestCancelledIsUnknown(t *testing.T) {
	sub, err := unit.Spawn(t, factoryReqActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "ask-and-cancel")
	check.Equal(t, false, sub.Terminated())

	a := sub.Behavior().(*reqActor)
	check.Equal(t, 0, len(a.answers))
	check.ErrorIs(t, a.cancelAgain(), gen.ErrUnknown)
}

func (a *reqActor) cancelAgain() error { return a.CancelRequest(gen.Ref{}) }
