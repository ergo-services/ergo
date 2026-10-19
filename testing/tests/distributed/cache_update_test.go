package distributed

import (
	"errors"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

var errRegisteredLate = errors.New("registered after connect")

// TestDistCacheUpdateErrorIdentity: an error registered on both nodes after
// they connected is sent by cache id in both directions from the first message,
// so the caller gets its own sentinel back.
func TestDistCacheUpdateErrorIdentity(t *testing.T) {
	s := stage.New(t)
	n1 := s.StartNode("n1")
	n2 := s.StartNode("n2")
	s.Connect(n1, n2)

	check.NoError(t, n1.Native().Network().RegisterError(errRegisteredLate))
	check.NoError(t, n2.Native().Network().RegisterError(errRegisteredLate))

	pong := n2.Spawn(factoryCallPong, gen.ProcessOptions{})
	res, err := n1.Call(pong, errRegisteredLate)
	check.NoError(t, err)
	check.True(t, res == errRegisteredLate, "the echo is the local sentinel")
}

// TestDistCacheUpdateDisabled: without the flag on one side the same error goes
// as text: delivered, but without its identity.
func TestDistCacheUpdateDisabled(t *testing.T) {
	flags := gen.DefaultNetworkFlags
	flags.EnableCacheUpdate = false

	s := stage.New(t)
	n1 := s.StartNode("n1")
	n2 := s.StartNode("n2", stage.NodeOptions{NetworkFlags: flags})
	s.Connect(n1, n2)

	check.NoError(t, n1.Native().Network().RegisterError(errRegisteredLate))
	check.NoError(t, n2.Native().Network().RegisterError(errRegisteredLate))

	pong := n2.Spawn(factoryCallPong, gen.ProcessOptions{})
	res, err := n1.Call(pong, errRegisteredLate)
	check.NoError(t, err)
	check.True(t, res != errRegisteredLate)
	check.Equal(t, errRegisteredLate.Error(), res.(error).Error())
}

var errRegisteredLater = errors.New("registered on the receiver later")

type errCheck struct{ act.Actor }

func factoryErrCheck() gen.ProcessBehavior { return &errCheck{} }

func (e *errCheck) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return request == errRegisteredLater, nil
}

// TestDistCacheUpdateLateSentinel: the receiver gets the cache id before it
// registers the sentinel; once it does, the same id decodes to the sentinel.
func TestDistCacheUpdateLateSentinel(t *testing.T) {
	s := stage.New(t)
	n1 := s.StartNode("n1")
	n2 := s.StartNode("n2")
	s.Connect(n1, n2)

	check.NoError(t, n1.Native().Network().RegisterError(errRegisteredLater))
	checker := n2.Spawn(factoryErrCheck, gen.ProcessOptions{})

	res, err := n1.Call(checker, errRegisteredLater)
	check.NoError(t, err)
	check.Equal(t, false, res)

	check.NoError(t, n2.Native().Network().RegisterError(errRegisteredLater))
	res, err = n1.Call(checker, errRegisteredLater)
	check.NoError(t, err)
	check.Equal(t, true, res)
}
