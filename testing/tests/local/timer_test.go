package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// timerActor arms a SendAfter timer on command and keeps its CancelFunc so the
// test can cancel it and observe the returned bool.
type timerActor struct {
	act.Actor
	cancel gen.CancelFunc
}

func factoryTimerActor() gen.ProcessBehavior { return &timerActor{} }

type armReq struct {
	To    gen.PID
	Msg   any
	After time.Duration
}

func (a *timerActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch r := request.(type) {
	case armReq:
		c, err := a.SendAfter(r.To, r.Msg, r.After)
		if err != nil {
			return err, nil
		}
		a.cancel = c
		return "ok", nil
	case string:
		if r == "cancel" {
			return a.cancel(), nil
		}
	}
	return "ok", nil
}

// TestLocalTimerSendAfter: SendAfter delivers the message to the target after the
// delay; its CancelFunc stops a pending timer (returning true) so the message is
// never delivered; and cancelling after the timer has already fired returns false.
func TestLocalTimerSendAfter(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	collector := n.Spawn(factoryEcho, gen.ProcessOptions{})
	timer := n.Spawn(factoryTimerActor, gen.ProcessOptions{})

	// fires: the message is delivered after the delay
	mk := n.Mark()
	_, err := n.Call(timer, armReq{To: collector, Msg: "tick", After: 30 * time.Millisecond})
	check.NoError(t, err)
	n.ShouldDeliver().To(collector).Message("tick").Since(mk).Once().Within(time.Second).Must()

	// cancel before firing: CancelFunc returns true and the message never arrives
	mk = n.Mark()
	_, err = n.Call(timer, armReq{To: collector, Msg: "cancelled", After: 100 * time.Millisecond})
	check.NoError(t, err)
	stopped, err := n.Call(timer, "cancel")
	check.NoError(t, err)
	check.Equal(t, true, stopped)
	// a later timer fires as a barrier: once it arrives, the 100ms deadline has
	// long passed, so "cancelled" would be present if it had not been cancelled
	_, err = n.Call(timer, armReq{To: collector, Msg: "barrier", After: 300 * time.Millisecond})
	check.NoError(t, err)
	n.ShouldDeliver().To(collector).Message("barrier").Since(mk).Once().Within(2 * time.Second).Must()
	n.ShouldDeliver().To(collector).Message("cancelled").Since(mk).None().Assert()

	// cancel after firing: CancelFunc returns false
	mk = n.Mark()
	_, err = n.Call(timer, armReq{To: collector, Msg: "fired", After: 30 * time.Millisecond})
	check.NoError(t, err)
	n.ShouldDeliver().To(collector).Message("fired").Since(mk).Once().Within(time.Second).Must()
	late, err := n.Call(timer, "cancel")
	check.NoError(t, err)
	check.Equal(t, false, late)
}
