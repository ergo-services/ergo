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

// poolWorker is a pool member. It answers calls with its own pid (so a forwarded
// call returns the worker that handled it) and ignores async messages (the
// forward itself is observed via the Forward record, not a worker side effect).
type poolWorker struct{ act.Actor }

func factoryPoolWorker() gen.ProcessBehavior { return &poolWorker{} }

func (w *poolWorker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return w.PID(), nil
}
func (w *poolWorker) HandleMessage(from gen.PID, message any) error { return nil }

// poolPool is an act.Pool of poolWorker. Normal-priority traffic is forwarded
// round-robin to workers; High/Max-priority traffic is handled here. An int
// request (High priority) grows (n>0) or shrinks (n<0) the pool; any other
// priority call returns the pool's own pid; a priority message is reported to
// the collector so "handled by the pool itself" is observable.
type poolPool struct {
	act.Pool
	collector gen.PID
}

func factoryPoolPool() gen.ProcessBehavior { return &poolPool{} }

func (p *poolPool) Init(args ...any) (act.PoolOptions, error) {
	p.collector = args[0].(gen.PID)
	var o act.PoolOptions
	o.WorkerFactory = factoryPoolWorker
	o.PoolSize = 5
	return o, nil
}

func (p *poolPool) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch v := request.(type) {
	case int:
		var nw int64
		var err error
		if v > 0 {
			nw, err = p.AddWorkers(v)
		} else {
			nw, err = p.RemoveWorkers(-v)
		}
		// surface the error as a value, never as the terminate reason
		if err != nil {
			return err, nil
		}
		return nw, nil
	}
	return p.PID(), nil
}

func (p *poolPool) HandleMessage(from gen.PID, message any) error {
	return p.Send(p.collector, p.PID())
}

// poolWorkers returns, in spawn order, the count workers spawned by pool since mark.
func poolWorkers(n *stage.Node, pool gen.PID, mark, count int) []gen.PID {
	sp := n.ShouldSpawn().From(pool).Since(mark).Times(count).Within(time.Second).Collect()
	out := make([]gen.PID, len(sp))
	for i, s := range sp {
		out[i] = s.Child
	}
	return out
}

// forwardTargets returns, in forward order, the count workers pool forwarded to since mark.
func forwardTargets(n *stage.Node, pool gen.PID, mark, count int) []gen.PID {
	fwd := n.ShouldForward().By(pool).Since(mark).Times(count).Within(time.Second).Collect()
	out := make([]gen.PID, len(fwd))
	for i, f := range fwd {
		out[i] = f.To
	}
	return out
}

// assertRoundRobin checks that to[] cycles over exactly `period` distinct workers
// with that period (to[i] == to[i%period]).
func assertRoundRobin(t *testing.T, to []gen.PID, period int) {
	t.Helper()
	check.Equal(t, period*2, len(to))
	seen := map[gen.PID]bool{}
	for i := 0; i < period; i++ {
		seen[to[i]] = true
	}
	check.Equal(t, period, len(seen))
	for i := range to {
		check.True(t, to[i] == to[i%period])
	}
}

func sameSet(a, b []gen.PID) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[gen.PID]int{}
	for _, p := range a {
		m[p]++
	}
	for _, p := range b {
		m[p]--
	}
	for _, c := range m {
		if c != 0 {
			return false
		}
	}
	return true
}

// TestLocalPool: an act.Pool forwards normal-priority Call/Send round-robin over
// its workers, while High/Max-priority traffic is handled by the pool itself
// (never forwarded). AddWorkers grows the ring (new workers appended), and
// RemoveWorkers shrinks it by dropping the oldest workers; in both cases the
// updated worker set keeps serving round-robin. Verified deterministically:
// forwards are observed in order via Forward records, the worker set/order via
// Spawn records.
func TestLocalPool(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nn := n.Native()

	collector := n.Spawn(factoryEcho, gen.ProcessOptions{})

	mkPool := n.Mark()
	pool := n.Spawn(factoryPoolPool, gen.ProcessOptions{}, collector)
	initial := poolWorkers(n, pool, mkPool, 5)
	check.Equal(t, 5, len(initial))

	high, max := gen.MessagePriorityHigh, gen.MessagePriorityMax

	// S1: a priority call is handled by the pool itself, not forwarded
	for _, pr := range []gen.MessagePriority{high, max} {
		mk := n.Mark()
		v, err := nn.CallWithPriority(pool, "ping", pr)
		check.NoError(t, err)
		check.Equal(t, pool, v)
		n.ShouldForward().By(pool).Since(mk).None().Assert()
	}

	// S2: a priority message is handled by the pool itself (reports to collector),
	// not forwarded
	for _, pr := range []gen.MessagePriority{high, max} {
		mk := n.Mark()
		check.NoError(t, nn.SendWithPriority(pool, "hi", pr))
		n.ShouldSend().From(pool).Message(pool).Since(mk).Once().Within(time.Second).Must()
		n.ShouldForward().By(pool).Since(mk).None().Assert()
	}

	// S3: normal-priority calls are forwarded round-robin over the 5 workers; the
	// call's response is the worker that handled it, matching the forward target
	mkCall := n.Mark()
	resp := make([]gen.PID, 10)
	for i := 0; i < 10; i++ {
		v, err := n.Call(pool, "ping")
		check.NoError(t, err)
		resp[i] = v.(gen.PID)
	}
	callTo := forwardTargets(n, pool, mkCall, 10)
	assertRoundRobin(t, callTo, 5)
	check.True(t, sameSet(callTo[:5], initial))
	for i := 0; i < 10; i++ {
		check.True(t, resp[i] == callTo[i])
	}

	// S4: normal-priority sends are forwarded round-robin over the same 5 workers
	mkSend := n.Mark()
	for i := 0; i < 10; i++ {
		n.Send(pool, "hi")
	}
	sendTo := forwardTargets(n, pool, mkSend, 10)
	assertRoundRobin(t, sendTo, 5)
	check.True(t, sameSet(sendTo[:5], initial))

	// S5: AddWorkers grows the pool to 8; forwarding now cycles over all 8, the 3
	// new workers appended to the original 5
	mkAdd := n.Mark()
	total, err := nn.CallWithPriority(pool, 3, high)
	check.NoError(t, err)
	check.Equal(t, int64(8), total)
	added := poolWorkers(n, pool, mkAdd, 3)
	check.Equal(t, 3, len(added))

	mkCall8 := n.Mark()
	for i := 0; i < 16; i++ {
		_, err := n.Call(pool, "ping")
		check.NoError(t, err)
	}
	to8 := forwardTargets(n, pool, mkCall8, 16)
	assertRoundRobin(t, to8, 8)
	check.True(t, sameSet(to8[:8], append(append([]gen.PID{}, initial...), added...)))

	// S6: RemoveWorkers shrinks the pool to 3 by dropping the 5 oldest workers;
	// the 3 survivors are exactly the ones AddWorkers appended, still serving
	total, err = nn.CallWithPriority(pool, -5, high)
	check.NoError(t, err)
	check.Equal(t, int64(3), total)

	mkCall3 := n.Mark()
	for i := 0; i < 6; i++ {
		_, err := n.Call(pool, "ping")
		check.NoError(t, err)
	}
	to3 := forwardTargets(n, pool, mkCall3, 6)
	assertRoundRobin(t, to3, 3)
	check.True(t, sameSet(to3[:3], added))

	// N1 (negative): removing more workers than present drains the pool and
	// returns ErrPoolEmpty as a value; the pool itself keeps running
	v, err := nn.CallWithPriority(pool, -10, high)
	check.NoError(t, err)
	e, ok := v.(error)
	check.True(t, ok)
	check.True(t, errors.Is(e, act.ErrPoolEmpty))
	// the pool itself did not terminate: still registered (N2 then proves it works)
	_, err = nn.ProcessInfo(pool)
	check.NoError(t, err)

	// N2 (recovery after the empty-pool error): the drained pool still accepts
	// AddWorkers, and forwarding resumes round-robin over the new workers. (No
	// normal traffic is sent before the add, so there is no priority-inversion
	// race; the empty-pool drop itself emits no observable signal, so it is not
	// asserted rather than faked with a time window.)
	mkRecover := n.Mark()
	cnt, err := nn.CallWithPriority(pool, 2, high)
	check.NoError(t, err)
	check.Equal(t, int64(2), cnt)
	recov := poolWorkers(n, pool, mkRecover, 2)
	check.Equal(t, 2, len(recov))

	mkFwd := n.Mark()
	for i := 0; i < 4; i++ {
		_, err := n.Call(pool, "ping")
		check.NoError(t, err)
	}
	to2 := forwardTargets(n, pool, mkFwd, 4)
	assertRoundRobin(t, to2, 2)
	check.True(t, sameSet(to2[:2], recov))

	// N3 (negative): a worker dies. The pool is not linked-from-child nor
	// monitoring workers, so it gets no notification; it discovers the dead worker
	// only on the next forward that lands on it, then lazily respawns a
	// replacement and keeps serving. The pool itself never terminates.
	victim := recov[0]
	survivor := recov[1]
	watcher := n.Spawn(factoryWatcher, gen.ProcessOptions{})
	n.Send(watcher, monitorCmd{Target: victim})
	n.ShouldMonitor().From(watcher).Target(victim).Once().Within(time.Second).Must()

	mkKill := n.Mark()
	check.NoError(t, n.SendExit(victim, gen.TerminateReasonKill))
	// confirm the worker is actually dead before forwarding (no async race)
	n.ShouldReceiveDown().To(watcher).About(victim).Since(mkKill).Once().Within(time.Second).Must()

	// a full cycle: one forward lands on the dead slot and triggers exactly one
	// respawn (the failed forward to the dead worker emits no Forward)
	mkResp := n.Mark()
	for i := 0; i < 2; i++ {
		_, err := n.Call(pool, "ping")
		check.NoError(t, err)
	}
	respawned := n.ShouldSpawn().From(pool).Since(mkResp).Times(1).Within(time.Second).Collect()
	check.Equal(t, 1, len(respawned))
	newPid := respawned[0].Child
	check.True(t, newPid != victim)

	// forwarding now cycles over the survivor and the respawned worker; the dead
	// worker never receives a forward again
	mkHeal := n.Mark()
	for i := 0; i < 4; i++ {
		_, err := n.Call(pool, "ping")
		check.NoError(t, err)
	}
	healed := forwardTargets(n, pool, mkHeal, 4)
	assertRoundRobin(t, healed, 2)
	for _, p := range healed {
		check.True(t, p != victim)
	}
	check.True(t, sameSet(healed[:2], []gen.PID{survivor, newPid}))
}
