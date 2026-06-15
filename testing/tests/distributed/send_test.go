package distributed

import (
	"strings"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// pong is the remote target: it creates an alias in Init (so it is addressable by
// PID, registered name and alias) and exposes it. Incoming deliveries are observed
// via the node recorder, so its handlers are no-ops.
type pong struct {
	act.Actor
	alias gen.Alias
}

func factoryPong() gen.ProcessBehavior { return &pong{} }

func (p *pong) Init(args ...any) error {
	a, err := p.CreateAlias()
	if err != nil {
		return err
	}
	p.alias = a
	return nil
}

func (p *pong) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "alias" {
		return p.alias, nil
	}
	return "ok", nil
}

func (p *pong) HandleMessage(from gen.PID, message any) error { return nil }

// sendCmd drives the sender to perform a send operation toward a target.
type sendCmd struct {
	Kind string // send | important | exit | compress
	To   any
	Msg  any
}

// sender performs cross-node sends on command (a process is needed for the
// process-only operations SendImportant / SetCompression / SendExit).
type sender struct{ act.Actor }

func factorySender() gen.ProcessBehavior { return &sender{} }

func (s *sender) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	c := request.(sendCmd)
	switch c.Kind {
	case "send":
		return errText(s.Send(c.To, c.Msg)), nil
	case "important":
		return errText(s.SendImportant(c.To, c.Msg)), nil
	case "exit":
		return errText(s.SendExit(c.To.(gen.PID), c.Msg.(error))), nil
	case "compress":
		return errText(s.SetCompression(true)), nil
	}
	return "ok", nil
}

// TestDistSend: a process on one node sends to a process on another node by PID,
// registered name and alias (plain and important delivery); the message crosses
// the wire intact (observed as Delivered on the receiver). Important delivery to a
// gone target reports ErrProcessUnknown. Oversized messages are rejected with
// ErrTooLarge (even with compression, which is bounded by the original size). An
// exit signal carries its reason across the wire.
func TestDistSend(t *testing.T) {
	const maxMsg = 765

	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2", stage.NodeOptions{MaxMessageSize: maxMsg})
	s.Connect(n1, n2)

	t.Run("PID", func(t *testing.T) {
		snd := n1.Spawn(factorySender, gen.ProcessOptions{})
		p := n2.Spawn(factoryPong, gen.ProcessOptions{})
		mk := n2.Mark()
		res, err := n1.Call(snd, sendCmd{Kind: "send", To: p, Msg: "byPID"})
		check.NoError(t, err)
		check.Equal(t, "", res)
		n2.ShouldDeliver().To(p).Message("byPID").Since(mk).Once().Within(time.Second).Must()
		n1.ShouldSend().From(snd).Message("byPID").Once().Within(time.Second).Must()
	})

	t.Run("ProcessID", func(t *testing.T) {
		snd := n1.Spawn(factorySender, gen.ProcessOptions{})
		n2.SpawnRegister("pong_pid", factoryPong, gen.ProcessOptions{})
		target := n2.ProcessID("pong_pid")
		mk := n2.Mark()
		res, err := n1.Call(snd, sendCmd{Kind: "send", To: target, Msg: "byProcessID"})
		check.NoError(t, err)
		check.Equal(t, "", res)
		n2.ShouldDeliver().ToProcessID(target).Message("byProcessID").Since(mk).Once().Within(time.Second).Must()
	})

	t.Run("Alias", func(t *testing.T) {
		snd := n1.Spawn(factorySender, gen.ProcessOptions{})
		p := n2.Spawn(factoryPong, gen.ProcessOptions{})
		av, err := n2.Call(p, "alias")
		check.NoError(t, err)
		alias := av.(gen.Alias)
		mk := n2.Mark()
		res, err := n1.Call(snd, sendCmd{Kind: "send", To: alias, Msg: "byAlias"})
		check.NoError(t, err)
		check.Equal(t, "", res)
		n2.ShouldDeliver().ToAlias(alias).Message("byAlias").Since(mk).Once().Within(time.Second).Must()
	})

	t.Run("ImportantPID", func(t *testing.T) {
		snd := n1.Spawn(factorySender, gen.ProcessOptions{})
		p := n2.Spawn(factoryPong, gen.ProcessOptions{})
		mk := n2.Mark()
		res, err := n1.Call(snd, sendCmd{Kind: "important", To: p, Msg: "imp"})
		check.NoError(t, err)
		check.Equal(t, "", res)
		n2.ShouldDeliver().To(p).Message("imp").Since(mk).Once().Within(time.Second).Must()

		// important delivery to a non-existent pid on the (live, connected) remote
		// node reports ErrProcessUnknown: the ack mechanism detects the miss, where
		// a plain fire-and-forget send would silently return nil
		bad := p
		bad.ID = 100000
		res, err = n1.Call(snd, sendCmd{Kind: "important", To: bad, Msg: "imp"})
		check.NoError(t, err)
		check.Equal(t, gen.ErrProcessUnknown.Error(), res)
	})

	t.Run("ImportantProcessID", func(t *testing.T) {
		snd := n1.Spawn(factorySender, gen.ProcessOptions{})
		n2.SpawnRegister("pong_imp", factoryPong, gen.ProcessOptions{})
		target := n2.ProcessID("pong_imp")
		mk := n2.Mark()
		res, err := n1.Call(snd, sendCmd{Kind: "important", To: target, Msg: "imp"})
		check.NoError(t, err)
		check.Equal(t, "", res)
		n2.ShouldDeliver().ToProcessID(target).Message("imp").Since(mk).Once().Within(time.Second).Must()

		// unknown name -> ErrProcessUnknown
		res, err = n1.Call(snd, sendCmd{Kind: "important", To: n2.ProcessID("unknown_name"), Msg: "imp"})
		check.NoError(t, err)
		check.Equal(t, gen.ErrProcessUnknown.Error(), res)
	})

	t.Run("ImportantAlias", func(t *testing.T) {
		snd := n1.Spawn(factorySender, gen.ProcessOptions{})
		p := n2.Spawn(factoryPong, gen.ProcessOptions{})
		av, err := n2.Call(p, "alias")
		check.NoError(t, err)
		alias := av.(gen.Alias)
		mk := n2.Mark()
		res, err := n1.Call(snd, sendCmd{Kind: "important", To: alias, Msg: "imp"})
		check.NoError(t, err)
		check.Equal(t, "", res)
		n2.ShouldDeliver().ToAlias(alias).Message("imp").Since(mk).Once().Within(time.Second).Must()

		// a mangled alias -> ErrProcessUnknown
		bad := alias
		bad.ID[1] = 0
		res, err = n1.Call(snd, sendCmd{Kind: "important", To: bad, Msg: "imp"})
		check.NoError(t, err)
		check.Equal(t, gen.ErrProcessUnknown.Error(), res)
	})

	t.Run("UnregisteredType", func(t *testing.T) {
		snd := n1.Spawn(factorySender, gen.ProcessOptions{})
		p := n2.Spawn(factoryPong, gen.ProcessOptions{})
		mk := n2.Mark()
		// a value whose type is not registered in EDF cannot be serialized for the
		// wire (it would work locally without serialization)
		res, err := n1.Call(snd, sendCmd{Kind: "send", To: p, Msg: unregisteredValue{X: 7}})
		check.NoError(t, err)
		check.True(t, strings.Contains(res.(string), "encoder"))
		n2.ShouldDeliver().To(p).Since(mk).None().Assert()
	})

	t.Run("Incarnation", func(t *testing.T) {
		snd := n1.Spawn(factorySender, gen.ProcessOptions{})
		p := n2.Spawn(factoryPong, gen.ProcessOptions{})
		// a pid carrying a Creation from a different node incarnation is rejected
		// with ErrProcessIncarnation (distinct from ErrProcessUnknown, which is a
		// live incarnation but an unknown id)
		stale := p
		stale.Creation = p.Creation + 1
		res, err := n1.Call(snd, sendCmd{Kind: "send", To: stale, Msg: "v"})
		check.NoError(t, err)
		check.Equal(t, gen.ErrProcessIncarnation.Error(), res)

		res, err = n1.Call(snd, sendCmd{Kind: "important", To: stale, Msg: "v"})
		check.NoError(t, err)
		check.Equal(t, gen.ErrProcessIncarnation.Error(), res)
	})

	t.Run("TooLarge", func(t *testing.T) {
		snd := n1.Spawn(factorySender, gen.ProcessOptions{})
		big := lib.RandomString(maxMsg + 1)
		res, err := n1.Call(snd, sendCmd{Kind: "send", To: n2.ProcessID("whatever"), Msg: big})
		check.NoError(t, err)
		check.Equal(t, gen.ErrTooLarge.Error(), res)
	})

	t.Run("Compress", func(t *testing.T) {
		snd := n1.Spawn(factorySender, gen.ProcessOptions{})
		p := n2.Spawn(factoryPong, gen.ProcessOptions{})

		// original size exceeds the limit -> ErrTooLarge, even with compression on
		big := lib.RandomString(maxMsg + maxMsg/2)
		res, err := n1.Call(snd, sendCmd{Kind: "send", To: p, Msg: big})
		check.NoError(t, err)
		check.Equal(t, gen.ErrTooLarge.Error(), res)

		_, err = n1.Call(snd, sendCmd{Kind: "compress"})
		check.NoError(t, err)
		res, err = n1.Call(snd, sendCmd{Kind: "send", To: p, Msg: big})
		check.NoError(t, err)
		check.Equal(t, gen.ErrTooLarge.Error(), res)

		// a message within the limit is delivered (compression enabled)
		small := lib.RandomString(maxMsg / 2)
		mk := n2.Mark()
		res, err = n1.Call(snd, sendCmd{Kind: "send", To: p, Msg: small})
		check.NoError(t, err)
		check.Equal(t, "", res)
		n2.ShouldDeliver().To(p).Message(small).Since(mk).Once().Within(time.Second).Must()
	})

	t.Run("Exit", func(t *testing.T) {
		snd := n1.Spawn(factorySender, gen.ProcessOptions{})
		p := n2.Spawn(factoryPong, gen.ProcessOptions{})
		w := n2.Spawn(factoryWatcher, gen.ProcessOptions{})
		n2.Send(w, monitorCmd{Target: p})
		n2.ShouldMonitor().From(w).Target(p).Once().Within(time.Second).Must()

		// the exit reason (an EDF-registered sentinel) crosses the wire intact
		mk := n2.Mark()
		res, err := n1.Call(snd, sendCmd{Kind: "exit", To: p, Msg: gen.ErrTaken})
		check.NoError(t, err)
		check.Equal(t, "", res)
		n2.ShouldReceiveDown().To(w).About(p).ReasonIs(gen.ErrTaken).Since(mk).Once().Within(time.Second).Must()
	})
}
