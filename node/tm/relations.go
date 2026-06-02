package tm

import (
	"sync/atomic"

	"ergo.services/ergo/gen"
)

type Kind uint8

const (
	KindLink    Kind = 1
	KindMonitor Kind = 2
)

type relationItem struct {
	pid   gen.PID
	kind  Kind
	alive atomic.Uint32
	next  atomic.Pointer[relationItem]
}

// Lock-free MPSC list. Multiple concurrent Walks allowed; only the
// walkClaim CAS winner splices tombstones, others iterate read-only.
type TargetRelations struct {
	head      atomic.Pointer[relationItem]
	tail      atomic.Pointer[relationItem]
	walkClaim atomic.Uint32
}

func NewTargetRelations() *TargetRelations {
	tr := &TargetRelations{}
	sentinel := &relationItem{}
	tr.head.Store(sentinel)
	tr.tail.Store(sentinel)
	return tr
}

func (tr *TargetRelations) Push(pid gen.PID, kind Kind) *relationItem {
	it := &relationItem{pid: pid, kind: kind}
	it.alive.Store(1)
	old := tr.head.Swap(it)
	old.next.Store(it)
	return it
}

// Returns true if this call flipped alive 1->0. Lets callers decide
// who decrements associated counters when multiple paths can tombstone.
func (tr *TargetRelations) MarkDead(it *relationItem) bool {
	return it.alive.CompareAndSwap(1, 0)
}

func (tr *TargetRelations) Walk(fn func(pid gen.PID, kind Kind)) {
	exclusive := tr.walkClaim.CompareAndSwap(0, 1)
	if exclusive {
		defer tr.walkClaim.Store(0)
	}

	prev := tr.tail.Load()
	for {
		curr := prev.next.Load()
		if curr == nil {
			return
		}
		if curr.alive.Load() == 1 {
			fn(curr.pid, curr.kind)
			prev = curr
			continue
		}
		if exclusive == false {
			prev = curr
			continue
		}
		next := curr.next.Load()
		if next != nil {
			prev.next.Store(next)
			continue
		}
		// curr is head; try detaching via head CAS so next Push lands past prev.
		if tr.head.CompareAndSwap(curr, prev) {
			prev.next.CompareAndSwap(curr, nil)
			return
		}
		// CAS lost: a Push raced in; loop, next iteration takes mid-list branch.
	}
}
