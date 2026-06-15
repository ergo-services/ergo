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

// spawnLinker asks the host to spawn a linker child (so its parent is the host,
// not the node core, otherwise act.Actor treats a link-death exit, which the
// node core relays, as a parent exit and ignores trap).
type spawnLinker struct{ Trap bool }

type host struct{ act.Actor }

func factoryHost() gen.ProcessBehavior { return &host{} }

func (h *host) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if c, ok := request.(spawnLinker); ok {
		return h.Spawn(factoryLinkerC, gen.ProcessOptions{}, c.Trap)
	}
	return nil, nil
}

// linkCmd tells the linker to link Target (PID/ProcessID/Alias/Event).
type linkCmd struct{ Target any }

// linkerC links on command; with trap set it survives a partner's death and
// receives the exit as a message, otherwise it terminates with the reason.
type linkerC struct{ act.Actor }

func factoryLinkerC() gen.ProcessBehavior { return &linkerC{} }

func (l *linkerC) Init(args ...any) error {
	l.SetTrapExit(args[0].(bool))
	return nil
}

func (l *linkerC) HandleMessage(from gen.PID, message any) error {
	if c, ok := message.(linkCmd); ok {
		switch tgt := c.Target.(type) {
		case gen.PID:
			_ = l.LinkPID(tgt)
		case gen.ProcessID:
			_ = l.LinkProcessID(tgt)
		case gen.Alias:
			_ = l.LinkAlias(tgt)
		case gen.Event:
			_, _ = l.LinkEvent(tgt)
		}
	}
	return nil
}

// linksOf returns the count and presence of target in the matching Links* list.
func linksOf(info gen.ProcessInfo, target any) (int, bool) {
	switch t := target.(type) {
	case gen.PID:
		return len(info.LinksPID), contains(info.LinksPID, t)
	case gen.ProcessID:
		return len(info.LinksProcessID), contains(info.LinksProcessID, t)
	case gen.Alias:
		return len(info.LinksAlias), contains(info.LinksAlias, t)
	case gen.Event:
		return len(info.LinksEvent), contains(info.LinksEvent, t)
	}
	return -1, false
}

func exitAbout(a *check.ExitAssert, target any) *check.ExitAssert {
	switch t := target.(type) {
	case gen.PID:
		return a.About(t)
	case gen.ProcessID:
		return a.AboutProcessID(t)
	case gen.Alias:
		return a.AboutAlias(t)
	case gen.Event:
		return a.AboutEvent(t)
	}
	return a
}

// runLink exercises one (addressing, trap, reason) combination: a linker links a
// target, the link is registered, the target dies, and the linker either receives
// the exit (trap) or terminates with the reason (no trap).
func runLink(t *testing.T, s *stage.Stage, n *stage.Node, host, w gen.PID, trap bool, reason error, bPID gen.PID, linkTarget any) {
	t.Helper()

	cAny, err := n.Call(host, spawnLinker{Trap: trap})
	check.NoError(t, err)
	c := cAny.(gen.PID)

	// observe the linker's fate
	n.Send(w, monitorCmd{Target: c})
	n.ShouldMonitor().From(w).Target(c).Once().Within(time.Second).Must()

	// before linking the linker has no links
	info, err := n.Native().ProcessInfo(c)
	check.NoError(t, err)
	cnt, has := linksOf(info, linkTarget)
	check.Equal(t, 0, cnt)
	check.True(t, has == false)

	mk := n.Mark()
	n.Send(c, linkCmd{Target: linkTarget})
	n.ShouldLink().From(c).Target(linkTarget).Since(mk).Once().Within(time.Second).Must()

	// after linking the linker has exactly that one link
	info, err = n.Native().ProcessInfo(c)
	check.NoError(t, err)
	cnt, has = linksOf(info, linkTarget)
	check.Equal(t, 1, cnt)
	check.True(t, has)

	if reason == gen.ErrUnregistered {
		// break the link by unregistering the linked name/alias/event; the target
		// process itself stays alive
		unregisterFor(n, bPID, linkTarget)
	} else if reason == gen.TerminateReasonKill {
		n.Kill(bPID)
	} else if err := n.SendExit(bPID, reason); err != nil {
		t.Fatalf("send exit: %s", err)
	}

	if trap {
		// linker survives and receives the exit signal carrying the target identity
		exitAbout(n.ShouldReceiveExit().To(c), linkTarget).Reason(reason).
			Since(mk).Once().Within(time.Second).Must()
	} else {
		// linker cascades: it terminates, wrapping the partner's reason
		n.ShouldReceiveDown().To(w).About(c).ReasonIs(reason).
			Since(mk).Once().Within(time.Second).Must()
	}

	if reason == gen.ErrUnregistered {
		// the break was the identity being unregistered, not a termination
		_, err := n.Native().ProcessInfo(bPID)
		check.NoError(t, err)
	}
}

// TestLocalLink: a bidirectional link delivers an exit signal carrying the dead
// partner's identity. A trapping process receives it as a message and survives;
// a non-trapping process terminates with the reason. Covers addressing by PID,
// registered name, alias and event; the link is visible in ProcessInfo; linking
// an unknown target fails.
func TestLocalLink(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	host := n.Spawn(factoryHost, gen.ProcessOptions{})
	w := n.Spawn(factoryWatcher, gen.ProcessOptions{})

	custom := errors.New("custom")
	base := []error{gen.TerminateReasonKill, custom, gen.TerminateReasonShutdown}
	withUnreg := append([]error{}, append(base, gen.ErrUnregistered)...)

	t.Run("PID", func(t *testing.T) {
		for _, reason := range base {
			for _, trap := range []bool{false, true} {
				b := n.Spawn(factoryTarget, gen.ProcessOptions{})
				runLink(t, s, n, host, w, trap, reason, b, b)
			}
		}
	})

	t.Run("ProcessID", func(t *testing.T) {
		for i, reason := range withUnreg {
			for j, trap := range []bool{false, true} {
				name := gen.Atom("lnk-" + string(rune('a'+i*2+j)))
				b := n.SpawnRegister(name, factoryTarget, gen.ProcessOptions{})
				runLink(t, s, n, host, w, trap, reason, b, gen.ProcessID{Name: name, Node: n.Name()})
			}
		}
	})

	t.Run("Alias", func(t *testing.T) {
		for _, reason := range withUnreg {
			for _, trap := range []bool{false, true} {
				b := n.Spawn(factoryTarget, gen.ProcessOptions{})
				info, err := n.Call(b, "info")
				check.NoError(t, err)
				runLink(t, s, n, host, w, trap, reason, b, info.(targetInfo).Alias)
			}
		}
	})

	t.Run("Event", func(t *testing.T) {
		for _, reason := range withUnreg {
			for _, trap := range []bool{false, true} {
				b := n.Spawn(factoryTarget, gen.ProcessOptions{})
				info, err := n.Call(b, "info")
				check.NoError(t, err)
				runLink(t, s, n, host, w, trap, reason, b, info.(targetInfo).Event)
			}
		}
	})

	t.Run("Unknown", func(t *testing.T) {
		cAny, err := n.Call(host, spawnLinker{Trap: true})
		check.NoError(t, err)
		c := cAny.(gen.PID)

		ghost := gen.PID{Node: n.Name(), ID: 999999}
		mk := n.Mark()
		n.Send(c, linkCmd{Target: ghost})
		n.ShouldLink().From(c).Target(ghost).Error(gen.ErrProcessUnknown).Since(mk).Once().Within(time.Second).Must()

		unknownName := gen.ProcessID{Name: "no_such", Node: n.Name()}
		mk = n.Mark()
		n.Send(c, linkCmd{Target: unknownName})
		n.ShouldLink().From(c).Target(unknownName).Error(gen.ErrProcessUnknown).Since(mk).Once().Within(time.Second).Must()

		unknownAlias := gen.Alias{Node: n.Name()}
		mk = n.Mark()
		n.Send(c, linkCmd{Target: unknownAlias})
		n.ShouldLink().From(c).Target(unknownAlias).Error(gen.ErrAliasUnknown).Since(mk).Once().Within(time.Second).Must()

		unknownEvent := gen.Event{Name: "no_such", Node: n.Name()}
		mk = n.Mark()
		n.Send(c, linkCmd{Target: unknownEvent})
		n.ShouldLink().From(c).Target(unknownEvent).Error(gen.ErrEventUnknown).Since(mk).Once().Within(time.Second).Must()
	})
}
