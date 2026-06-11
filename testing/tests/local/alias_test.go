package local

import (
	"errors"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// aliasActor creates two aliases for itself and can delete the first on command.
type aliasActor struct {
	act.Actor
	a1 gen.Alias
	a2 gen.Alias
}

func factoryAliasActor() gen.ProcessBehavior { return &aliasActor{} }

func (a *aliasActor) Init(args ...any) error {
	var err error
	if a.a1, err = a.CreateAlias(); err != nil {
		return err
	}
	if a.a2, err = a.CreateAlias(); err != nil {
		return err
	}
	return nil
}

func (a *aliasActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "aliases":
		return [2]gen.Alias{a.a1, a.a2}, nil
	case "delete1":
		return errText(a.DeleteAlias(a.a1)), nil
	case "ping":
		return "pong", nil
	}
	return "ok", nil
}

// TestLocalAliasRouting: a process is reachable through each of its aliases while
// they are alive; deleting an alias stops it routing (send/call to it returns
// ErrProcessUnknown) while the process's other alias keeps working.
func TestLocalAliasRouting(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	nn := n.Native()
	target := n.Spawn(factoryAliasActor)

	av, err := n.Call(target, "aliases")
	check.NoError(t, err)
	aliases := av.([2]gen.Alias)
	a1, a2 := aliases[0], aliases[1]

	// both aliases route to the process while alive
	v, err := nn.Call(a1, "ping")
	check.NoError(t, err)
	check.Equal(t, "pong", v)
	v, err = nn.Call(a2, "ping")
	check.NoError(t, err)
	check.Equal(t, "pong", v)

	// delete the first alias (synchronously, in the handler)
	dv, err := n.Call(target, "delete1")
	check.NoError(t, err)
	check.Equal(t, "", dv)

	// the deleted alias no longer routes
	_, err = nn.Call(a1, "ping")
	check.True(t, errors.Is(err, gen.ErrProcessUnknown))
	check.True(t, errors.Is(nn.Send(a1, "x"), gen.ErrProcessUnknown))

	// the surviving alias still routes
	v, err = nn.Call(a2, "ping")
	check.NoError(t, err)
	check.Equal(t, "pong", v)
}
