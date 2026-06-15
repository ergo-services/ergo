package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// echo is a passive receiver; deliveries to it are observed by the recorder.
type echo struct{ act.Actor }

func factoryEcho() gen.ProcessBehavior { return &echo{} }

func (e *echo) HandleMessage(from gen.PID, message any) error { return nil }

// sendTo tells the sender to send Msg to To.
type sendTo struct {
	To  any
	Msg any
}

// sender sends on command (so the send is real process egress, recorded).
type sender struct{ act.Actor }

func factorySender() gen.ProcessBehavior { return &sender{} }

func (s *sender) HandleMessage(from gen.PID, message any) error {
	if c, ok := message.(sendTo); ok {
		return s.Send(c.To, c.Msg)
	}
	return nil
}

// TestLocalSend: a process sends a message addressed by PID, registered name or
// alias; the send is observed as egress at the sender and as delivery into the
// target's mailbox. Sending to an unregistered name is rejected.
func TestLocalSend(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	from := n.Spawn(factorySender, gen.ProcessOptions{})

	t.Run("PID", func(t *testing.T) {
		target := n.Spawn(factoryEcho, gen.ProcessOptions{})
		mk := n.Mark()
		n.Send(from, sendTo{To: target, Msg: "byPID"})
		n.ShouldSend().From(from).Message("byPID").Since(mk).Once().Within(time.Second).Must()
		n.ShouldDeliver().To(target).Message("byPID").Since(mk).Once().Within(time.Second).Must()
	})

	t.Run("ProcessID", func(t *testing.T) {
		n.SpawnRegister("echo-name", factoryEcho, gen.ProcessOptions{})
		pid := gen.ProcessID{Name: "echo-name", Node: n.Name()}
		mk := n.Mark()
		n.Send(from, sendTo{To: pid, Msg: "byName"})
		n.ShouldSend().From(from).Message("byName").Since(mk).Once().Within(time.Second).Must()
		n.ShouldDeliver().ToProcessID(pid).Message("byName").Since(mk).Once().Within(time.Second).Must()
	})

	t.Run("Alias", func(t *testing.T) {
		target := n.Spawn(factoryTarget, gen.ProcessOptions{})
		info, err := n.Call(target, "info")
		check.NoError(t, err)
		alias := info.(targetInfo).Alias
		mk := n.Mark()
		n.Send(from, sendTo{To: alias, Msg: "byAlias"})
		n.ShouldSend().From(from).Message("byAlias").Since(mk).Once().Within(time.Second).Must()
		n.ShouldDeliver().ToAlias(alias).Message("byAlias").Since(mk).Once().Within(time.Second).Must()
	})

	t.Run("Unknown", func(t *testing.T) {
		err := n.Native().Send(gen.Atom("no_such_process"), 1)
		check.ErrorIs(t, err, gen.ErrProcessUnknown)
	})
}
