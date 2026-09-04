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
