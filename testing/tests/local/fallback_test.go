package local

import (
	"errors"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/stage"
)

// fbBlackhole never answers a request, so its caller stays parked in WaitResponse.
type fbBlackhole struct{ act.Actor }

func factoryFBBlackhole() gen.ProcessBehavior { return &fbBlackhole{} }

func (b *fbBlackhole) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

type fbPark struct{ Blackhole gen.PID }

type fbAliasRequest struct{}

type fbCollectedRequest struct{}

// fbVictim parks itself in WaitResponse on demand, so its bounded mailbox fills up
// instead of being drained.
type fbVictim struct {
	act.Actor
	alias gen.Alias
}

func factoryFBVictim() gen.ProcessBehavior { return &fbVictim{} }

func (v *fbVictim) Init(args ...any) error {
	alias, err := v.CreateAlias()
	if err != nil {
		return err
	}
	v.alias = alias
	return nil
}

func (v *fbVictim) HandleMessage(from gen.PID, message any) error {
	if m, ok := message.(fbPark); ok {
		v.CallWithTimeout(m.Blackhole, fbPark{}, 10)
	}
	return nil
}

func (v *fbVictim) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return v.alias, nil
}

// fbCollector records the fallback messages it gets.
type fbCollector struct {
	act.Actor
	got []gen.MessageFallback
}

func factoryFBCollector() gen.ProcessBehavior { return &fbCollector{} }

func (c *fbCollector) HandleMessage(from gen.PID, message any) error {
	if m, ok := message.(gen.MessageFallback); ok {
		c.got = append(c.got, m)
	}
	return nil
}

func (c *fbCollector) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return append([]gen.MessageFallback{}, c.got...), nil
}

// fbStartFull spawns a victim with a one-message mailbox, parks it so it stops draining,
// and fills its mailbox. The returned alias addresses the same process.
func fbStartFull(t *testing.T, n *stage.Node, blackhole gen.PID, name gen.Atom,
	fallback gen.ProcessFallback) (gen.PID, gen.Alias) {
	t.Helper()

	pid := n.SpawnRegister(name, factoryFBVictim, gen.ProcessOptions{
		MailboxSize: 1,
		Fallback:    fallback,
	})

	v, err := n.Call(pid, fbAliasRequest{})
	if err != nil {
		t.Fatalf("unable to get the alias of %s: %s", name, err)
	}
	alias := v.(gen.Alias)

	n.Send(pid, fbPark{Blackhole: blackhole})
	for deadline := time.Now().Add(5 * time.Second); ; {
		info, err := n.Native().ProcessInfo(pid)
		if err == nil && info.State == gen.ProcessStateWaitResponse {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s did not stop draining its mailbox", name)
		}
		time.Sleep(time.Millisecond)
	}

	if err := n.Native().Send(pid, "fill"); err != nil {
		t.Fatalf("unable to fill the mailbox of %s: %s", name, err)
	}
	return pid, alias
}

// fbCollect reads the collector until it holds the expected number of messages.
func fbCollect(t *testing.T, n *stage.Node, collector gen.PID, want int) []gen.MessageFallback {
	t.Helper()

	for deadline := time.Now().Add(5 * time.Second); ; {
		v, err := n.Call(collector, fbCollectedRequest{})
		if err != nil {
			t.Fatalf("unable to read the collector: %s", err)
		}
		got := v.([]gen.MessageFallback)
		if len(got) >= want {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("the collector got %d fallback message(s), expected %d", len(got), want)
		}
		time.Sleep(time.Millisecond)
	}
}

// A full mailbox forwards the message to the fallback process, wrapped once and naming
// the address the sender used, whichever way the target is addressed.
func TestLocalFallbackForwards(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")

	blackhole := n.Spawn(factoryFBBlackhole, gen.ProcessOptions{})
	collector := n.SpawnRegister("fb_collector", factoryFBCollector, gen.ProcessOptions{})
	victim, alias := fbStartFull(t, n, blackhole, "fb_victim", gen.ProcessFallback{
		Enable: true,
		Name:   "fb_collector",
		Tag:    "spill",
	})

	targets := []any{victim, gen.ProcessID{Name: "fb_victim", Node: n.Name()}, alias}
	for i, to := range targets {
		if err := n.Native().Send(to, i); err != nil {
			t.Fatalf("unable to send to %#v: %s", to, err)
		}
	}

	got := fbCollect(t, n, collector, len(targets))
	if len(got) != len(targets) {
		t.Fatalf("the collector got %d fallback message(s), expected %d", len(got), len(targets))
	}
	for i, m := range got {
		if m.PID != victim {
			t.Fatalf("fallback %d names %s, expected %s", i, m.PID, victim)
		}
		if m.Tag != "spill" {
			t.Fatalf("fallback %d carries tag %q, expected %q", i, m.Tag, "spill")
		}
		if m.Target != targets[i] {
			t.Fatalf("fallback %d names target %#v, expected %#v", i, m.Target, targets[i])
		}
		if m.Message != i {
			t.Fatalf("fallback %d carries %#v, expected %d", i, m.Message, i)
		}
	}
}

// A fallback process with a full mailbox of its own does not forward any further: the
// sender gets ErrProcessMailboxFull.
func TestLocalFallbackFullRejects(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")

	blackhole := n.Spawn(factoryFBBlackhole, gen.ProcessOptions{})
	fbStartFull(t, n, blackhole, "fb_spill", gen.ProcessFallback{})
	victim, _ := fbStartFull(t, n, blackhole, "fb_victim", gen.ProcessFallback{
		Enable: true,
		Name:   "fb_spill",
	})

	if err := n.Native().Send(victim, "overflow"); errors.Is(err, gen.ErrProcessMailboxFull) == false {
		t.Fatalf("the send returned %v, expected %v", err, gen.ErrProcessMailboxFull)
	}
}

// A cycle in the fallback configuration ends with ErrProcessMailboxFull instead of
// forwarding until the node dies.
func TestLocalFallbackCycleRejects(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")

	blackhole := n.Spawn(factoryFBBlackhole, gen.ProcessOptions{})
	a, _ := fbStartFull(t, n, blackhole, "fb_a", gen.ProcessFallback{Enable: true, Name: "fb_b"})
	b, _ := fbStartFull(t, n, blackhole, "fb_b", gen.ProcessFallback{Enable: true, Name: "fb_a"})

	for _, to := range []gen.PID{a, b} {
		if err := n.Native().Send(to, "overflow"); errors.Is(err, gen.ErrProcessMailboxFull) == false {
			t.Fatalf("the send to %s returned %v, expected %v", to, err, gen.ErrProcessMailboxFull)
		}
	}
	if n.Native().IsAlive() == false {
		t.Fatal("the node did not survive the fallback cycle")
	}
}

// A fallback process that is not registered leaves the sender with the error of the
// mailbox it could not reach, whichever way the target is addressed.
func TestLocalFallbackUnknownRejects(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")

	blackhole := n.Spawn(factoryFBBlackhole, gen.ProcessOptions{})
	victim, alias := fbStartFull(t, n, blackhole, "fb_victim", gen.ProcessFallback{
		Enable: true,
		Name:   "fb_nobody",
	})

	targets := []any{victim, gen.ProcessID{Name: "fb_victim", Node: n.Name()}, alias}
	for _, to := range targets {
		if err := n.Native().Send(to, "overflow"); errors.Is(err, gen.ErrProcessMailboxFull) == false {
			t.Fatalf("the send to %#v returned %v, expected %v", to, err, gen.ErrProcessMailboxFull)
		}
	}
}

// A process whose fallback names itself does not forward to itself.
func TestLocalFallbackSelfRejects(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")

	blackhole := n.Spawn(factoryFBBlackhole, gen.ProcessOptions{})
	victim, _ := fbStartFull(t, n, blackhole, "fb_self", gen.ProcessFallback{
		Enable: true,
		Name:   "fb_self",
	})

	if err := n.Native().Send(victim, "overflow"); errors.Is(err, gen.ErrProcessMailboxFull) == false {
		t.Fatalf("the send returned %v, expected %v", err, gen.ErrProcessMailboxFull)
	}
}
