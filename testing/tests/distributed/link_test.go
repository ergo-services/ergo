package distributed

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// runLinkDeath: a linker on n1 links a remote target on n2; the target terminates
// with reason (EDF-registered, so it crosses the wire); a trapping linker receives
// the exit carrying that reason, a non-trapping one cascades with it.
func runLinkDeath(t *testing.T, s *stage.Stage, n1, n2 *stage.Node, trap bool, addr string, reason error) {
	t.Helper()
	target, pid := newRTarget(t, n2, addr)
	linker := n1.Spawn(factoryRLinker, gen.ProcessOptions{}, trap)

	var w gen.PID
	if trap == false {
		w = n1.Spawn(factoryWatcher, gen.ProcessOptions{})
		n1.Send(w, monitorCmd{Target: linker})
		n1.ShouldMonitor().From(w).Target(linker).Once().Within(time.Second).Must()
	}

	res, err := n1.Call(linker, linkTarget{Target: target})
	check.NoError(t, err)
	check.Equal(t, "", res)

	mk := n1.Mark()
	if reason == gen.TerminateReasonKill {
		s.Kill(n2, pid)
	} else {
		check.NoError(t, n2.SendExit(pid, reason))
	}

	if trap {
		exitAbout(n1.ShouldReceiveExit().To(linker), target).Reason(reason).
			Since(mk).Once().Within(time.Second).Must()
	} else {
		n1.ShouldReceiveDown().To(w).About(linker).ReasonIs(reason).
			Since(mk).Once().Within(time.Second).Must()
	}
}

// TestDistLink: linking a remote process delivers an exit signal across the wire
// carrying the dead partner's identity and reason. Covers all addressing modes,
// trap true/false (message vs cascade), EDF-registered reasons, ErrUnregistered
// (identity unregistered while the process survives), node disconnect
// (ErrNoConnection) and LinkNode (MessageExitNode).
func TestDistLink(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	s.Connect(n1, n2)

	addrs := []string{"pid", "processid", "alias", "event"}

	// remote death delivers the exit across all addressing modes (trap=true)
	t.Run("Death", func(t *testing.T) {
		for _, addr := range addrs {
			runLinkDeath(t, s, n1, n2, true, addr, gen.TerminateReasonShutdown)
		}
	})

	// a non-trapping linker cascades cross-node
	t.Run("Cascade", func(t *testing.T) {
		runLinkDeath(t, s, n1, n2, false, "pid", gen.TerminateReasonShutdown)
	})

	// EDF-registered reasons cross the wire intact
	t.Run("Reasons", func(t *testing.T) {
		runLinkDeath(t, s, n1, n2, true, "pid", gen.TerminateReasonKill)
		runLinkDeath(t, s, n1, n2, true, "pid", gen.ErrTaken)
	})

	// unregistering the remote identity breaks the link with ErrUnregistered while
	// the remote process survives
	t.Run("Unregistered", func(t *testing.T) {
		target, pid := newRTarget(t, n2, "processid")
		linker := n1.Spawn(factoryRLinker, gen.ProcessOptions{}, true)
		res, err := n1.Call(linker, linkTarget{Target: target})
		check.NoError(t, err)
		check.Equal(t, "", res)

		mk := n1.Mark()
		n2.Send(pid, unregisterCmd{Kind: "name"})
		exitAbout(n1.ShouldReceiveExit().To(linker), target).Reason(gen.ErrUnregistered).
			Since(mk).Once().Within(time.Second).Must()
		// the remote process is still alive
		_, err = n2.Native().ProcessInfo(pid)
		check.NoError(t, err)
	})

	// linking a remote pid from a different node incarnation is rejected
	t.Run("Incarnation", func(t *testing.T) {
		p := n2.Spawn(factoryRTarget, gen.ProcessOptions{})
		linker := n1.Spawn(factoryRLinker, gen.ProcessOptions{}, true)
		stale := p
		stale.Creation = p.Creation + 1
		res, err := n1.Call(linker, linkTarget{Target: stale})
		check.NoError(t, err)
		check.Equal(t, gen.ErrProcessIncarnation.Error(), res)
	})

	// a single node disconnect fires every link to that node at once: per-process
	// links with ErrNoConnection, and a LinkNode link with MessageExitNode. (Run
	// last: it drops the connection and is not reconnected.)
	t.Run("NodeDown", func(t *testing.T) {
		type lt struct {
			linker gen.PID
			target any
		}
		var lts []lt
		for _, addr := range addrs {
			target, _ := newRTarget(t, n2, addr)
			linker := n1.Spawn(factoryRLinker, gen.ProcessOptions{}, true)
			res, err := n1.Call(linker, linkTarget{Target: target})
			check.NoError(t, err)
			check.Equal(t, "", res)
			lts = append(lts, lt{linker: linker, target: target})
		}
		nodeLinker := n1.Spawn(factoryRLinker, gen.ProcessOptions{}, true)
		res, err := n1.Call(nodeLinker, linkNodeCmd{Node: n2.Name()})
		check.NoError(t, err)
		check.Equal(t, "", res)

		mk := n1.Mark()
		remote, err := n1.Native().Network().Node(n2.Name())
		check.NoError(t, err)
		remote.Disconnect()

		for _, x := range lts {
			exitAbout(n1.ShouldReceiveExit().To(x.linker), x.target).Reason(gen.ErrNoConnection).
				Since(mk).Once().Within(time.Second).Must()
		}
		n1.ShouldReceiveExit().To(nodeLinker).Where(func(e check.Exit) bool {
			m, ok := e.Message.(gen.MessageExitNode)
			return ok && m.Name == n2.Name()
		}).Since(mk).Once().Within(time.Second).Must()
	})
}
