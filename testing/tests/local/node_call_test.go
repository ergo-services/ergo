package local

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// ncaAnswer carries the request it answers, so a call that got the answer meant
// for another call is visible.
type ncaAnswer struct {
	request int
	kind    string
}

// ncaResponder answers every request twice: once by hand, once by returning a
// result from the callback.
type ncaResponder struct {
	act.Actor
}

func factoryNCAResponder() gen.ProcessBehavior { return &ncaResponder{} }

func (r *ncaResponder) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	r.SendResponse(from, ref, ncaAnswer{request: request.(int), kind: "first"})
	return ncaAnswer{request: request.(int), kind: "second"}, nil
}

// A node level call keeps the answer that reached it first, and the answer that
// lost cannot reach any of the calls that follow.
func TestLocalNodeCallAnsweredTwice(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	pid := n.Spawn(factoryNCAResponder, gen.ProcessOptions{})

	for request := 0; request < 200; request++ {
		value, err := n.Call(pid, request)
		check.NoError(t, err)
		answer := value.(ncaAnswer)
		check.Equal(t, request, answer.request)
		check.Equal(t, "first", answer.kind)
	}
}
