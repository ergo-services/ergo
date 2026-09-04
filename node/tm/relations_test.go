package tm

import (
	"sync"
	"sync/atomic"
	"testing"

	"ergo.services/ergo/gen"
)

func pid(id uint64) gen.PID {
	return gen.PID{Node: "test", ID: id, Creation: 1}
}

// walkCollect drains the active relations into a slice in walk order.
func walkCollect(tr *TargetRelations) []gen.PID {
	out := []gen.PID{}
	tr.Walk(func(p gen.PID, _ Kind) {
		out = append(out, p)
	})
	return out
}

// listLen counts items currently linked in the queue (alive + tombstoned).
// Used to verify splice actually unhooks items.
func listLen(tr *TargetRelations) int {
	n := 0
	for it := tr.tail.Load().next.Load(); it != nil; it = it.next.Load() {
		n++
	}
	return n
}

func TestWalkEmpty(t *testing.T) {
	tr := NewTargetRelations()
	got := walkCollect(tr)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestPushWalkOrder(t *testing.T) {
	tr := NewTargetRelations()
	want := []gen.PID{pid(1), pid(2), pid(3)}
	for _, p := range want {
		tr.Push(p, KindLink)
	}
	got := walkCollect(tr)
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order[%d]: got %v want %v", i, got[i], want[i])
		}
	}
}

func TestMarkDeadMiddleIsSpliced(t *testing.T) {
	tr := NewTargetRelations()
	a := tr.Push(pid(1), KindLink)
	_ = a
	b := tr.Push(pid(2), KindLink)
	c := tr.Push(pid(3), KindLink)
	_ = c

	tr.MarkDead(b)

	got := walkCollect(tr)
	want := []gen.PID{pid(1), pid(3)}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v want %v", got, want)
	}
	if listLen(tr) != 2 {
		t.Fatalf("list still has %d nodes; expected 2 after splice", listLen(tr))
	}
}

func TestMarkDeadHeadIsDetached(t *testing.T) {
	tr := NewTargetRelations()
	a := tr.Push(pid(1), KindLink)
	b := tr.Push(pid(2), KindLink)
	_ = a

	// b is the head (most recent). Tombstone it.
	tr.MarkDead(b)
	got := walkCollect(tr)
	if len(got) != 1 || got[0] != pid(1) {
		t.Fatalf("got %v want [%v]", got, pid(1))
	}
	if listLen(tr) != 1 {
		t.Fatalf("list still has %d nodes; expected 1 after head detach", listLen(tr))
	}

	// Push another item; it should land after `a` (the surviving prev).
	tr.Push(pid(3), KindLink)
	got = walkCollect(tr)
	want := []gen.PID{pid(1), pid(3)}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("after re-push got %v want %v", got, want)
	}
}

func TestMarkDeadOnlyItemEmptiesList(t *testing.T) {
	tr := NewTargetRelations()
	a := tr.Push(pid(1), KindLink)
	tr.MarkDead(a)

	got := walkCollect(tr)
	if len(got) != 0 {
		t.Fatalf("expected empty after head detach, got %v", got)
	}
	if listLen(tr) != 0 {
		t.Fatalf("list still has %d nodes; expected 0", listLen(tr))
	}

	// New push must still work.
	tr.Push(pid(2), KindLink)
	got = walkCollect(tr)
	if len(got) != 1 || got[0] != pid(2) {
		t.Fatalf("after re-push got %v want [%v]", got, pid(2))
	}
}

func TestMarkDeadAllInOrder(t *testing.T) {
	tr := NewTargetRelations()
	items := make([]*relationItem, 0, 100)
	for i := 0; i < 100; i++ {
		items = append(items, tr.Push(pid(uint64(i+1)), KindLink))
	}
	for _, it := range items {
		tr.MarkDead(it)
	}
	got := walkCollect(tr)
	if len(got) != 0 {
		t.Fatalf("expected empty after mark-all-dead, got len=%d", len(got))
	}
	if listLen(tr) != 0 {
		t.Fatalf("list still has %d nodes; expected 0", listLen(tr))
	}
}

func TestKindPreserved(t *testing.T) {
	tr := NewTargetRelations()
	tr.Push(pid(1), KindLink)
	tr.Push(pid(2), KindMonitor)

	var links, monitors []gen.PID
	tr.Walk(func(p gen.PID, k Kind) {
		switch k {
		case KindLink:
			links = append(links, p)
		case KindMonitor:
			monitors = append(monitors, p)
		}
	})
	if len(links) != 1 || links[0] != pid(1) {
		t.Fatalf("links: got %v want [%v]", links, pid(1))
	}
	if len(monitors) != 1 || monitors[0] != pid(2) {
		t.Fatalf("monitors: got %v want [%v]", monitors, pid(2))
	}
}

func TestConcurrentPushAllVisible(t *testing.T) {
	tr := NewTargetRelations()
	const perWorker = 312
	const workers = 16
	const total = perWorker * workers

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(base int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				tr.Push(pid(uint64(base*perWorker+i+1)), KindLink)
			}
		}(w)
	}
	wg.Wait()

	got := walkCollect(tr)
	if len(got) != total {
		t.Fatalf("expected %d items, walker saw %d", total, len(got))
	}
}

func TestConcurrentPushWithLiveWalker(t *testing.T) {
	tr := NewTargetRelations()
	const total = 4000
	const workers = 8

	var pushed atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < total/workers; i++ {
				tr.Push(pid(uint64(pushed.Add(1))), KindLink)
			}
		}()
	}

	// Walker spins concurrently with pushers; keeps last seen count.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	for {
		select {
		case <-done:
			final := walkCollect(tr)
			if len(final) != total {
				t.Fatalf("final walk saw %d, want %d", len(final), total)
			}
			return
		default:
			// Walker (single, target-owner contract) keeps spinning.
			tr.Walk(func(gen.PID, Kind) {})
		}
	}
}

func TestConcurrentWalkersOnlyOneSplices(t *testing.T) {
	tr := NewTargetRelations()
	const n = 1000

	items := make([]*relationItem, n)
	for i := 0; i < n; i++ {
		items[i] = tr.Push(pid(uint64(i+1)), KindLink)
	}
	// Mark every even item dead.
	for i := 0; i < n; i++ {
		if items[i].pid.ID%2 == 0 {
			tr.MarkDead(items[i])
		}
	}

	// 32 concurrent walkers. Each should see all alive entries exactly once
	// in its own iteration. The splicing walker reclaims tombstones; non-
	// splicers walk read-only and skip dead entries without mutation.
	const walkers = 32
	var wg sync.WaitGroup
	wg.Add(walkers)
	for w := 0; w < walkers; w++ {
		go func() {
			defer wg.Done()
			count := 0
			tr.Walk(func(gen.PID, Kind) { count++ })
			if count != n/2 {
				t.Errorf("walker saw %d alive, want %d", count, n/2)
			}
		}()
	}
	wg.Wait()

	// After concurrent walks, the splicer (whichever won) should have
	// cleaned up tombstones. List length must equal alive count.
	if listLen(tr) != n/2 {
		t.Fatalf("list len after concurrent walks = %d, want %d", listLen(tr), n/2)
	}
}

func TestConcurrentPushAndMarkDead(t *testing.T) {
	tr := NewTargetRelations()
	const total = 4000
	const workers = 8

	items := make(chan *relationItem, total)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(base int) {
			defer wg.Done()
			for i := 0; i < total/workers; i++ {
				id := uint64(base*total+i) + 1
				it := tr.Push(pid(id), KindLink)
				if id%2 == 0 {
					items <- it
				}
			}
		}(w)
	}
	wg.Wait()
	close(items)

	// Mark every even-id item dead.
	for it := range items {
		tr.MarkDead(it)
	}

	got := walkCollect(tr)
	if len(got) != total/2 {
		t.Fatalf("expected %d alive, walker saw %d", total/2, len(got))
	}
	for _, p := range got {
		if p.ID%2 == 0 {
			t.Fatalf("walker saw tombstoned id %d", p.ID)
		}
	}
	if listLen(tr) != total/2 {
		t.Fatalf("list still has %d nodes after splice; expected %d", listLen(tr), total/2)
	}
}
