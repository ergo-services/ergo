package distributed

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// runMonitorDeath: a monitor on n1 watches a remote target on n2; the target
// terminates with an EDF-registered reason; the monitor receives the matching
// Down carrying the target identity and reason.
func runMonitorDeath(t *testing.T, s *stage.Stage, n1, n2 *stage.Node, addr string, reason error) {
	t.Helper()
	target, pid := newRTarget(t, n2, addr)
	mon := n1.Spawn(factoryRMonitor, gen.ProcessOptions{})

	res, err := n1.Call(mon, monitorTarget{Target: target})
	check.NoError(t, err)
	check.Equal(t, "", res)

	mk := n1.Mark()
	if reason == gen.TerminateReasonKill {
		s.Kill(n2, pid)
	} else {
		check.NoError(t, n2.SendExit(pid, reason))
	}
	downAbout(n1.ShouldReceiveDown().To(mon), target).ReasonIs(reason).
		Since(mk).Once().Within(time.Second).Must()
}

// TestDistMonitor: monitoring a remote process delivers a Down across the wire on
// its termination, by every addressing mode and for EDF-registered reasons.
// ErrUnregistered fires while the target survives; a stale incarnation is rejected
// with ErrProcessIncarnation; a node disconnect fires every monitor at once
// (ErrNoConnection) and MonitorNode yields MessageDownNode.
func TestDistMonitor(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	s.Connect(n1, n2)

	addrs := []string{"pid", "processid", "alias", "event"}

	t.Run("Death", func(t *testing.T) {
		for _, addr := range addrs {
			runMonitorDeath(t, s, n1, n2, addr, gen.TerminateReasonShutdown)
		}
	})

	t.Run("Reasons", func(t *testing.T) {
		runMonitorDeath(t, s, n1, n2, "pid", gen.TerminateReasonKill)
		runMonitorDeath(t, s, n1, n2, "pid", gen.ErrTaken)
	})

	t.Run("Unregistered", func(t *testing.T) {
		target, pid := newRTarget(t, n2, "processid")
		mon := n1.Spawn(factoryRMonitor, gen.ProcessOptions{})
		res, err := n1.Call(mon, monitorTarget{Target: target})
		check.NoError(t, err)
		check.Equal(t, "", res)

		mk := n1.Mark()
		n2.Send(pid, unregisterCmd{Kind: "name"})
		downAbout(n1.ShouldReceiveDown().To(mon), target).ReasonIs(gen.ErrUnregistered).
			Since(mk).Once().Within(time.Second).Must()
		_, err = n2.Native().ProcessInfo(pid)
		check.NoError(t, err)
	})

	t.Run("Incarnation", func(t *testing.T) {
		p := n2.Spawn(factoryRTarget, gen.ProcessOptions{})
		mon := n1.Spawn(factoryRMonitor, gen.ProcessOptions{})
		stale := p
		stale.Creation = p.Creation + 1
		res, err := n1.Call(mon, monitorTarget{Target: stale})
		check.NoError(t, err)
		check.Equal(t, gen.ErrProcessIncarnation.Error(), res)
	})

	// a single node disconnect fires every monitor to that node: per-process
	// monitors with ErrNoConnection, and a MonitorNode monitor with MessageDownNode.
	t.Run("NodeDown", func(t *testing.T) {
		type mt struct {
			mon    gen.PID
			target any
		}
		var mts []mt
		for _, addr := range addrs {
			target, _ := newRTarget(t, n2, addr)
			mon := n1.Spawn(factoryRMonitor, gen.ProcessOptions{})
			res, err := n1.Call(mon, monitorTarget{Target: target})
			check.NoError(t, err)
			check.Equal(t, "", res)
			mts = append(mts, mt{mon: mon, target: target})
		}
		nodeMon := n1.Spawn(factoryRMonitor, gen.ProcessOptions{})
		res, err := n1.Call(nodeMon, monitorNodeCmd{Node: n2.Name()})
		check.NoError(t, err)
		check.Equal(t, "", res)

		mk := n1.Mark()
		remote, err := n1.Native().Network().Node(n2.Name())
		check.NoError(t, err)
		remote.Disconnect()

		for _, x := range mts {
			downAbout(n1.ShouldReceiveDown().To(x.mon), x.target).ReasonIs(gen.ErrNoConnection).
				Since(mk).Once().Within(time.Second).Must()
		}
		n1.ShouldReceiveDown().To(nodeMon).Where(func(d check.Down) bool {
			m, ok := d.Message.(gen.MessageDownNode)
			return ok && m.Name == n2.Name()
		}).Since(mk).Once().Within(time.Second).Must()
	})
}
