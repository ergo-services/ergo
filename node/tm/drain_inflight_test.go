package tm

import (
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

// A relation whose Push is caught mid two-step (head.Swap done, old.next.Store still in
// flight) must not be dropped by Drain: Drain waits for the pending link instead of stopping
// at the first nil next. Verify-to-fail: the old "stop at next==nil" Drain returns with only
// the fully-linked relation.
func TestDrainWaitsForInFlightPush(t *testing.T) {
	tr := NewTargetRelations()
	tr.Push(pid(1), KindMonitor) // fully linked

	// stage an in-flight Push: publish the node as head, defer its predecessor's link
	it2 := &relationItem{pid: pid(2), kind: KindMonitor}
	it2.alive.Store(1)
	old := tr.head.Swap(it2)

	go func() {
		time.Sleep(10 * time.Millisecond)
		old.next.Store(it2) // complete the pending link a beat later
	}()

	var drained []gen.PID
	tr.Drain(func(p gen.PID, _ Kind) { drained = append(drained, p) })

	if len(drained) != 2 {
		t.Fatalf("drained=%v, want pid(1) and pid(2): Drain must not miss an in-flight Push", drained)
	}
}

// Many concurrent wait-free Pushes racing a Drain: every relation is delivered exactly once
// across the racing Drain and a final sweep (none lost in the two-step window). Run with -race.
func TestDrainConcurrentPushExactlyOnce(t *testing.T) {
	const (
		iterations = 100
		N          = 64
	)
	for iter := 0; iter < iterations; iter++ {
		tr := NewTargetRelations()
		var delivered int64
		var mu sync.Mutex

		var wg sync.WaitGroup
		wg.Add(N + 1)
		for i := 0; i < N; i++ {
			go func(i int) {
				defer wg.Done()
				tr.Push(pid(uint64(i+1)), KindMonitor)
			}(i)
		}
		go func() {
			defer wg.Done()
			tr.Drain(func(gen.PID, Kind) {
				mu.Lock()
				delivered++
				mu.Unlock()
			})
		}()
		wg.Wait()

		// sweep survivors pushed after the racing Drain passed the head
		tr.Drain(func(gen.PID, Kind) {
			mu.Lock()
			delivered++
			mu.Unlock()
		})

		if delivered != N {
			t.Fatalf("iter %d: delivered=%d, want %d (a relation was lost or double-delivered)", iter, delivered, N)
		}
	}
}

// RemoveTarget (Drain) racing the compaction Walk run by the last Unregister on the same
// target must return. Verify-to-fail: without walkClaim in Drain, Walk detaches the dead
// head Drain stands on and Drain spins on next==nil forever (~0.3% of iterations).
func TestDrainReturnsWhileCompactionWalkDetachesHead(t *testing.T) {
	const (
		iterations = 3000
		N          = 224
	)
	tgt := target(1)
	for iter := 0; iter < iterations; iter++ {
		s := NewStorage()
		for i := 0; i < N; i++ {
			s.Register(tgt, pid(uint64(i+1)), KindLink)
		}
		// all but the last consumer already gone, compaction armed but not yet run
		v, _ := s.targets.Load(tgt)
		entry := v.(*targetEntry)
		for i := 0; i < N-1; i++ {
			iv, _ := entry.index.LoadAndDelete(indexKey{consumer: pid(uint64(i + 1)), kind: KindLink})
			entry.relations.MarkDead(iv.(*relationItem))
		}

		var start sync.WaitGroup
		start.Add(1)
		drained := make(chan struct{})
		go func() { start.Wait(); s.RemoveTarget(tgt); close(drained) }()
		go func() { start.Wait(); s.Unregister(tgt, pid(N), KindLink) }()
		start.Done()

		select {
		case <-drained:
		case <-time.After(2 * time.Second):
			t.Fatalf("iter %d: Drain did not return while racing a compaction Walk", iter)
		}
	}
}
