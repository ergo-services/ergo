package node

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// newCallNode builds a node able to route responses on node level calls.
func newCallNode() *node {
	n := &node{name: "n@localhost"}
	n.creation = 1
	n.corePID = gen.PID{Node: n.name, ID: 1, Creation: 1}
	n.log = createLog(gen.LogLevelInfo, func(gen.MessageLog, string) {})
	return n
}

// A node level call takes the first response, a second one has nowhere to go
func TestNodeCallAnsweredTwice(t *testing.T) {
	n := newCallNode()
	from := gen.PID{Node: n.name, ID: 2, Creation: 1}

	answers := []struct {
		name    string
		deliver func(ref gen.Ref, answer string) error
		take    func(result nodeCallResult) string
	}{
		{
			name: "response",
			deliver: func(ref gen.Ref, answer string) error {
				return n.RouteSendResponse(from, n.corePID, gen.MessageOptions{Ref: ref}, answer)
			},
			take: func(result nodeCallResult) string { return result.response.(string) },
		},
		{
			name: "response error",
			deliver: func(ref gen.Ref, answer string) error {
				return n.RouteSendResponseError(from, n.corePID, gen.MessageOptions{Ref: ref}, errors.New(answer))
			},
			take: func(result nodeCallResult) string { return result.err.Error() },
		},
	}

	for _, answer := range answers {
		t.Run(answer.name, func(t *testing.T) {
			ref := n.MakeRef()
			done := takeNodeCall()
			n.calls.Store(ref, done)

			check.NoError(t, answer.deliver(ref, "first"))

			if _, found := n.calls.Load(ref); found {
				t.Fatal("the answered call is still registered")
			}
			check.ErrorIs(t, answer.deliver(ref, "second"), gen.ErrResponseIgnored)

			check.Equal(t, "first", answer.take(<-done))
			select {
			case extra := <-done:
				t.Fatalf("the call got a second result %v", extra)
			default:
			}
		})
	}
}

// A response for a ref no call waits for is ignored.
func TestNodeCallUnknownRef(t *testing.T) {
	n := newCallNode()
	from := gen.PID{Node: n.name, ID: 2, Creation: 1}
	options := gen.MessageOptions{Ref: n.MakeRef()}

	check.ErrorIs(t, n.RouteSendResponse(from, n.corePID, options, "answer"), gen.ErrResponseIgnored)
	check.ErrorIs(t, n.RouteSendResponseError(from, n.corePID, options, errors.New("answer")), gen.ErrResponseIgnored)
}
