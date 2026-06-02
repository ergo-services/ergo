package tm

import (
	"sort"
	"sync"
	"sync/atomic"
	"testing"

	"ergo.services/ergo/gen"
)

func target(id uint64) gen.PID {
	return gen.PID{Node: "target", ID: id, Creation: 1}
}

func storageWalkPIDs(s *Storage, t any) []gen.PID {
	out := []gen.PID{}
	s.Walk(t, func(p gen.PID, _ Kind) {
		out = append(out, p)
	})
	return out
}

func TestStorageRegisterWalk(t *testing.T) {
	s := NewStorage()
	tgt := target(1)

	if s.Register(tgt, pid(1), KindLink) == false {
		t.Fatal("first Register should succeed")
	}
	if s.Register(tgt, pid(2), KindMonitor) == false {
		t.Fatal("second Register (different kind/consumer) should succeed")
	}

	got := storageWalkPIDs(s, tgt)
	want := []gen.PID{pid(1), pid(2)}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestStorageDuplicateReturnsFalse(t *testing.T) {
	s := NewStorage()
	tgt := target(1)

	if s.Register(tgt, pid(1), KindLink) == false {
		t.Fatal("first Register should succeed")
	}
	if s.Register(tgt, pid(1), KindLink) == true {
		t.Fatal("duplicate Register should return false")
	}

	got := storageWalkPIDs(s, tgt)
	if len(got) != 1 || got[0] != pid(1) {
		t.Fatalf("expected exactly one entry, got %v", got)
	}
}

func TestStorageLinkAndMonitorSamePIDCoexist(t *testing.T) {
	s := NewStorage()
	tgt := target(1)

	if s.Register(tgt, pid(1), KindLink) == false {
		t.Fatal("link Register should succeed")
	}
	if s.Register(tgt, pid(1), KindMonitor) == false {
		t.Fatal("monitor Register (same consumer, different kind) should succeed")
	}

	links := 0
	monitors := 0
	s.Walk(tgt, func(_ gen.PID, k Kind) {
		switch k {
		case KindLink:
			links++
		case KindMonitor:
			monitors++
		}
	})
	if links != 1 || monitors != 1 {
		t.Fatalf("links=%d monitors=%d; want 1/1", links, monitors)
	}
}

func TestStorageUnregisterTombstones(t *testing.T) {
	s := NewStorage()
	tgt := target(1)

	s.Register(tgt, pid(1), KindLink)
	s.Register(tgt, pid(2), KindLink)
	s.Register(tgt, pid(3), KindLink)

	if s.Unregister(tgt, pid(2), KindLink) == false {
		t.Fatal("Unregister of existing relation should return true")
	}

	got := storageWalkPIDs(s, tgt)
	want := []gen.PID{pid(1), pid(3)}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v want %v", got, want)
	}

	// Re-register after unregister works (uses fresh item).
	if s.Register(tgt, pid(2), KindLink) == false {
		t.Fatal("re-Register after Unregister should succeed")
	}
	got = storageWalkPIDs(s, tgt)
	if len(got) != 3 {
		t.Fatalf("expected 3 after re-register, got %v", got)
	}
}

func TestStorageUnregisterNonexistent(t *testing.T) {
	s := NewStorage()
	tgt := target(1)

	if s.Unregister(tgt, pid(1), KindLink) == true {
		t.Fatal("Unregister on empty storage should return false")
	}
	s.Register(tgt, pid(1), KindLink)
	if s.Unregister(tgt, pid(1), KindMonitor) == true {
		t.Fatal("Unregister with wrong kind should return false")
	}
	if s.Unregister(tgt, pid(2), KindLink) == true {
		t.Fatal("Unregister with wrong consumer should return false")
	}
}

func TestStorageWalkUnknownTarget(t *testing.T) {
	s := NewStorage()
	calls := 0
	s.Walk(target(999), func(gen.PID, Kind) { calls++ })
	if calls != 0 {
		t.Fatalf("expected 0 walk calls on unknown target, got %d", calls)
	}
}

func TestStorageMultipleTargetsIsolated(t *testing.T) {
	s := NewStorage()
	tgtA := target(1)
	tgtB := target(2)

	s.Register(tgtA, pid(10), KindLink)
	s.Register(tgtB, pid(20), KindLink)
	s.Register(tgtA, pid(30), KindMonitor)

	gotA := storageWalkPIDs(s, tgtA)
	gotB := storageWalkPIDs(s, tgtB)

	if len(gotA) != 2 {
		t.Fatalf("target A: got %v want 2 entries", gotA)
	}
	if len(gotB) != 1 || gotB[0] != pid(20) {
		t.Fatalf("target B: got %v want [%v]", gotB, pid(20))
	}

	// Unregister on B does not affect A.
	s.Unregister(tgtB, pid(20), KindLink)
	gotA = storageWalkPIDs(s, tgtA)
	if len(gotA) != 2 {
		t.Fatalf("target A after B unregister: got %v want 2 entries", gotA)
	}
}

func TestStorageRemoveTargetReturnsAlive(t *testing.T) {
	s := NewStorage()
	tgt := target(1)

	s.Register(tgt, pid(1), KindLink)
	s.Register(tgt, pid(2), KindMonitor)
	s.Register(tgt, pid(3), KindLink)
	s.Unregister(tgt, pid(2), KindMonitor) // tombstoned before RemoveTarget

	got := s.RemoveTarget(tgt)
	if len(got) != 2 {
		t.Fatalf("RemoveTarget returned %d, want 2 alive", len(got))
	}
	// Order is unspecified (iterates per-target index, a sync.Map);
	// validate as a set of (Consumer, Kind) pairs.
	want := map[Relation]struct{}{
		{Consumer: pid(1), Kind: KindLink}: {},
		{Consumer: pid(3), Kind: KindLink}: {},
	}
	for _, r := range got {
		if _, ok := want[r]; ok == false {
			t.Fatalf("unexpected relation %+v", r)
		}
		delete(want, r)
	}
	if len(want) != 0 {
		t.Fatalf("missing relations: %+v", want)
	}

	// After RemoveTarget, target is gone.
	if storageWalkPIDs(s, tgt) != nil && len(storageWalkPIDs(s, tgt)) != 0 {
		t.Fatal("expected empty walk after RemoveTarget")
	}
	if s.RemoveTarget(tgt) != nil {
		t.Fatal("second RemoveTarget should return nil")
	}

	// Re-register on same target works (fresh entry).
	if s.Register(tgt, pid(7), KindLink) == false {
		t.Fatal("re-Register after RemoveTarget should succeed")
	}
	got2 := storageWalkPIDs(s, tgt)
	if len(got2) != 1 || got2[0] != pid(7) {
		t.Fatalf("after re-register: got %v want [%v]", got2, pid(7))
	}
}

func TestStorageConcurrentRegisterDuplicates(t *testing.T) {
	s := NewStorage()
	tgt := target(1)

	const workers = 16
	var trueCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			if s.Register(tgt, pid(1), KindLink) {
				trueCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if trueCount.Load() != 1 {
		t.Fatalf("exactly one Register should return true, got %d", trueCount.Load())
	}
	got := storageWalkPIDs(s, tgt)
	if len(got) != 1 || got[0] != pid(1) {
		t.Fatalf("walker should see one entry, got %v", got)
	}
}

func TestStorageConcurrentRegisterUnregister(t *testing.T) {
	s := NewStorage()
	tgt := target(1)

	const consumers = 1000
	const workers = 32

	// Register all consumers concurrently.
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(base int) {
			defer wg.Done()
			for i := 0; i < consumers/workers; i++ {
				s.Register(tgt, pid(uint64(base*consumers+i+1)), KindLink)
			}
		}(w)
	}
	wg.Wait()

	// Unregister every odd-id concurrently.
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(base int) {
			defer wg.Done()
			for i := 0; i < consumers/workers; i++ {
				id := uint64(base*consumers+i+1)
				if id%2 == 1 {
					s.Unregister(tgt, pid(id), KindLink)
				}
			}
		}(w)
	}
	wg.Wait()

	got := storageWalkPIDs(s, tgt)
	expectAlive := 0
	for w := 0; w < workers; w++ {
		for i := 0; i < consumers/workers; i++ {
			id := uint64(w*consumers+i+1)
			if id%2 == 0 {
				expectAlive++
			}
		}
	}
	if len(got) != expectAlive {
		t.Fatalf("walker saw %d alive, want %d", len(got), expectAlive)
	}
	for _, p := range got {
		if p.ID%2 == 1 {
			t.Fatalf("walker saw odd-id %d after Unregister", p.ID)
		}
	}
}

func TestStorageReverseIndexBasic(t *testing.T) {
	s := NewStorage()
	consumer := pid(1)
	tgt1 := target(10)
	tgt2 := target(11)
	tgt3 := target(12)

	s.Register(tgt1, consumer, KindLink)
	s.Register(tgt2, consumer, KindLink)
	s.Register(tgt3, consumer, KindMonitor)

	links := s.LinksFor(consumer)
	if len(links) != 2 {
		t.Fatalf("LinksFor = %v, want 2 entries", links)
	}
	seen := map[any]bool{links[0]: true, links[1]: true}
	if seen[tgt1] == false || seen[tgt2] == false {
		t.Fatalf("LinksFor missing tgt1 or tgt2: %v", links)
	}

	monitors := s.MonitorsFor(consumer)
	if len(monitors) != 1 || monitors[0] != tgt3 {
		t.Fatalf("MonitorsFor = %v, want [%v]", monitors, tgt3)
	}
}

func TestStorageReverseIndexUnregister(t *testing.T) {
	s := NewStorage()
	consumer := pid(1)
	tgt := target(10)

	s.Register(tgt, consumer, KindLink)
	if len(s.LinksFor(consumer)) != 1 {
		t.Fatal("expected one link before unregister")
	}
	s.Unregister(tgt, consumer, KindLink)
	if len(s.LinksFor(consumer)) != 0 {
		t.Fatal("LinksFor should be empty after Unregister")
	}
}

func TestStorageReverseIndexRemoveTarget(t *testing.T) {
	s := NewStorage()
	tgt := target(10)
	a := pid(1)
	b := pid(2)
	s.Register(tgt, a, KindLink)
	s.Register(tgt, b, KindMonitor)

	s.RemoveTarget(tgt)

	if len(s.LinksFor(a)) != 0 {
		t.Fatalf("a's links should be empty after RemoveTarget, got %v", s.LinksFor(a))
	}
	if len(s.MonitorsFor(b)) != 0 {
		t.Fatalf("b's monitors should be empty after RemoveTarget, got %v", s.MonitorsFor(b))
	}
}

func TestStorageReverseIndexRemoveConsumersOnNode(t *testing.T) {
	s := NewStorage()
	tgt := target(10)
	localC := pid(1)
	remoteC := remotePID(1)
	s.Register(tgt, localC, KindLink)
	s.Register(tgt, remoteC, KindLink)

	s.RemoveConsumersOnNode(tgt, remoteC.Node)

	if len(s.LinksFor(remoteC)) != 0 {
		t.Fatalf("remote consumer's links should be empty, got %v", s.LinksFor(remoteC))
	}
	if len(s.LinksFor(localC)) != 1 {
		t.Fatalf("local consumer's links should still hold tgt, got %v", s.LinksFor(localC))
	}
}

func TestStorageClearConsumer(t *testing.T) {
	s := NewStorage()
	consumer := pid(1)
	tgt1 := target(10)
	s.Register(tgt1, consumer, KindLink)
	s.Unregister(tgt1, consumer, KindLink)

	// after Unregister LinksFor reports empty, but the inner sync.Map entry
	// remains until ClearConsumer.
	s.ClearConsumer(consumer)
	if _, ok := s.reverse.Load(consumer); ok {
		t.Fatal("reverse entry should be gone after ClearConsumer")
	}
}

func TestStorageCountsTrack(t *testing.T) {
	s := NewStorage()
	if l, mn := s.Counts(); l != 0 || mn != 0 {
		t.Fatalf("empty counts: %d/%d want 0/0", l, mn)
	}

	s.Register(target(1), pid(1), KindLink)
	s.Register(target(1), pid(2), KindMonitor)
	s.Register(target(2), pid(3), KindLink)
	if l, mn := s.Counts(); l != 2 || mn != 1 {
		t.Fatalf("after registers: %d/%d want 2/1", l, mn)
	}

	s.Unregister(target(1), pid(1), KindLink)
	if l, mn := s.Counts(); l != 1 || mn != 1 {
		t.Fatalf("after unregister: %d/%d want 1/1", l, mn)
	}

	s.RemoveTarget(target(2))
	if l, mn := s.Counts(); l != 0 || mn != 1 {
		t.Fatalf("after RemoveTarget: %d/%d want 0/1", l, mn)
	}

	s.RemoveConsumersOnNode(target(1), pid(2).Node)
	if l, mn := s.Counts(); l != 0 || mn != 0 {
		t.Fatalf("after RemoveConsumersOnNode: %d/%d want 0/0", l, mn)
	}
}

func TestStorageConcurrentRegisterUnregisterReregister(t *testing.T) {
	s := NewStorage()
	tgt := target(1)

	const N = 500
	pids := make([]gen.PID, N)
	for i := range pids {
		pids[i] = pid(uint64(i + 1))
	}

	// Each PID toggles: register, unregister, register again.
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			s.Register(tgt, pids[idx], KindLink)
			s.Unregister(tgt, pids[idx], KindLink)
			s.Register(tgt, pids[idx], KindLink)
		}(i)
	}
	wg.Wait()

	got := storageWalkPIDs(s, tgt)
	if len(got) != N {
		t.Fatalf("expected %d active after register/unregister/reregister, got %d", N, len(got))
	}
	// Verify exactly the right PIDs are present.
	gotIDs := make([]uint64, len(got))
	for i, p := range got {
		gotIDs[i] = p.ID
	}
	sort.Slice(gotIDs, func(i, j int) bool { return gotIDs[i] < gotIDs[j] })
	for i, id := range gotIDs {
		if id != uint64(i+1) {
			t.Fatalf("missing pid at index %d: got id=%d", i, id)
		}
	}
}
