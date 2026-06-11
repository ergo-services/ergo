package local

import (
	"errors"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// silentActor never answers a call: returning (nil, nil) defers the response, and
// it never sends one, so a caller must time out.
type silentActor struct{ act.Actor }

func factorySilentActor() gen.ProcessBehavior { return &silentActor{} }

func (s *silentActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

// callEcho answers a call with the request value.
type callEcho struct{ act.Actor }

func factoryCallEcho() gen.ProcessBehavior { return &callEcho{} }

func (e *callEcho) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return request, nil
}

// TestLocalCallWithTimeout: a call that is answered within the timeout returns the
// response; a call to a process that never answers returns gen.ErrTimeout after
// the timeout elapses.
func TestLocalCallWithTimeout(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	silent := n.Spawn(factorySilentActor)
	echo := n.Spawn(factoryCallEcho)

	// answered within the timeout
	v, err := n.Native().CallWithTimeout(echo, "ping", 2)
	check.NoError(t, err)
	check.Equal(t, "ping", v)

	// never answered -> ErrTimeout after waiting ~the timeout (1s)
	start := time.Now()
	_, err = n.Native().CallWithTimeout(silent, "x", 1)
	check.True(t, errors.Is(err, gen.ErrTimeout))
	check.True(t, time.Since(start) >= 900*time.Millisecond)
}
