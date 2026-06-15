package distributed

import (
	"strings"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// getAlias is a distinct request type asking callPong for its alias (so it does
// not collide with the echoed call values).
type getAlias struct{}

// callPong answers a call by echoing the request; it is addressable by PID,
// registered name and the alias it creates in Init.
type callPong struct {
	act.Actor
	alias gen.Alias
}

func factoryCallPong() gen.ProcessBehavior { return &callPong{} }

func (p *callPong) Init(args ...any) error {
	a, err := p.CreateAlias()
	if err != nil {
		return err
	}
	p.alias = a
	return nil
}

func (p *callPong) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if _, ok := request.(getAlias); ok {
		return p.alias, nil
	}
	return request, nil
}

// callPongDefer replies through SendResponse from inside HandleCall and returns
// (nil, nil) so the actor does not auto-reply: the explicit (deferred) response
// path rather than the return-value path.
type callPongDefer struct{ act.Actor }

func factoryCallPongDefer() gen.ProcessBehavior { return &callPongDefer{} }

func (p *callPongDefer) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if err := p.SendResponse(from, ref, request); err != nil {
		return nil, err
	}
	return nil, nil
}

// TestDistCall: a synchronous call to a process on another node, by PID,
// registered name and alias (plain and important delivery), round-trips the
// request and returns the echoed response. Important delivery to a non-existent
// target reports ErrProcessUnknown.
func TestDistCall(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	s.Connect(n1, n2)

	t.Run("PID", func(t *testing.T) {
		p := n2.Spawn(factoryCallPong, gen.ProcessOptions{})
		v, err := n1.Call(p, "ping")
		check.NoError(t, err)
		check.Equal(t, "ping", v)
	})

	t.Run("ImportantPID", func(t *testing.T) {
		p := n2.Spawn(factoryCallPong, gen.ProcessOptions{})
		v, err := n1.Native().CallImportant(p, "ping")
		check.NoError(t, err)
		check.Equal(t, "ping", v)

		bad := p
		bad.ID = 100000
		_, err = n1.Native().CallImportant(bad, "ping")
		check.True(t, err == gen.ErrProcessUnknown)
	})

	t.Run("ProcessID", func(t *testing.T) {
		n2.SpawnRegister("call_pong", factoryCallPong, gen.ProcessOptions{})
		v, err := n1.Call(n2.ProcessID("call_pong"), "ping")
		check.NoError(t, err)
		check.Equal(t, "ping", v)
	})

	t.Run("ImportantProcessID", func(t *testing.T) {
		n2.SpawnRegister("call_pong_imp", factoryCallPong, gen.ProcessOptions{})
		v, err := n1.Native().CallImportant(n2.ProcessID("call_pong_imp"), "ping")
		check.NoError(t, err)
		check.Equal(t, "ping", v)

		_, err = n1.Native().CallImportant(n2.ProcessID("unknown_name"), "ping")
		check.True(t, err == gen.ErrProcessUnknown)
	})

	// the responder defers: it replies via SendResponse and returns (nil, nil)
	t.Run("Deferred", func(t *testing.T) {
		p := n2.Spawn(factoryCallPongDefer, gen.ProcessOptions{})
		mk := n2.Mark()
		v, err := n1.Call(p, "ping")
		check.NoError(t, err)
		check.Equal(t, "ping", v)
		n2.ShouldSendResponse().From(p).Message("ping").Since(mk).Once().Within(time.Second).Must()
	})

	t.Run("Incarnation", func(t *testing.T) {
		p := n2.Spawn(factoryCallPong, gen.ProcessOptions{})
		// a call to a pid from a different node incarnation is rejected
		stale := p
		stale.Creation = p.Creation + 1
		_, err := n1.Call(stale, "ping")
		check.True(t, err == gen.ErrProcessIncarnation)
		_, err = n1.Native().CallImportant(stale, "ping")
		check.True(t, err == gen.ErrProcessIncarnation)
	})

	t.Run("UnregisteredType", func(t *testing.T) {
		p := n2.Spawn(factoryCallPong, gen.ProcessOptions{})
		// a request whose type is not registered in EDF cannot be serialized
		_, err := n1.Call(p, unregisteredValue{X: 7})
		check.True(t, err != nil && strings.Contains(err.Error(), "encoder"))
	})

	t.Run("Alias", func(t *testing.T) {
		p := n2.Spawn(factoryCallPong, gen.ProcessOptions{})
		av, err := n2.Call(p, getAlias{})
		check.NoError(t, err)
		alias := av.(gen.Alias)
		v, err := n1.Call(alias, "ping")
		check.NoError(t, err)
		check.Equal(t, "ping", v)
	})

	t.Run("ImportantAlias", func(t *testing.T) {
		p := n2.Spawn(factoryCallPong, gen.ProcessOptions{})
		av, err := n2.Call(p, getAlias{})
		check.NoError(t, err)
		alias := av.(gen.Alias)
		v, err := n1.Native().CallImportant(alias, "ping")
		check.NoError(t, err)
		check.Equal(t, "ping", v)

		bad := alias
		bad.ID[1] = 0
		_, err = n1.Native().CallImportant(bad, "ping")
		check.True(t, err == gen.ErrProcessUnknown)
	})
}
