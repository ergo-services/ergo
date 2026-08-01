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

// Compact once tombstones reach this absolute floor and at least match live.
const compactMinTombstones = 32

// Lock-free MPSC list. Multiple concurrent Walks allowed; only the
// walkClaim CAS winner splices tombstones, others iterate read-only.
type TargetRelations struct {
	head      atomic.Pointer[relationItem]
	tail      atomic.Pointer[relationItem]
	walkClaim atomic.Uint32
	live      atomic.Int64 // alive items
	dead      atomic.Int64 // tombstones not yet spliced
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
	tr.live.Add(1)
	return it
}

// Returns true if this call flipped alive 1->0. Lets callers decide who
// decrements the external link/monitor counters; internal live/dead are
// maintained here.
func (tr *TargetRelations) MarkDead(it *relationItem) bool {
	if it.alive.CompareAndSwap(1, 0) == false {
		return false
	}
	tr.live.Add(-1)
	tr.dead.Add(1)
	return true
}

// Runs an exclusive Walk to splice tombstones once they pile up. Threshold is
// proportional to live so cost stays amortized O(1) per Unregister.
func (tr *TargetRelations) compact() {
	dead := tr.dead.Load()
	if dead < compactMinTombstones || dead < tr.live.Load() {
		return
	}
	tr.Walk(func(gen.PID, Kind) {})
}

// Drain walks the list once claiming each live relation via MarkDead-CAS,
// calling fn for every relation this call wins. Death fan-out: a concurrent
// Unregister that wins the CAS first is skipped. Entry is dropped after, so
// no splicing.
//
// Push is two-step (head.Swap then old.next.Store), so a relation whose node is
// already published as the head can still have a nil next while its predecessor's
// link is in flight. A plain "stop at next==nil" would miss it. When prev has a nil
// next but is not the head, that link is pending: spin until it lands so the in-flight
// Push is never dropped. prev==head is the only genuine end of the list.
func (tr *TargetRelations) Drain(fn func(pid gen.PID, kind Kind)) {
	prev := tr.tail.Load()
	for {
		curr := prev.next.Load()
		if curr == nil {
			if tr.head.Load() == prev {
				return
			}
			continue
		}
		if curr.alive.CompareAndSwap(1, 0) {
			fn(curr.pid, curr.kind)
		}
		prev = curr
	}
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
			tr.dead.Add(-1)
			continue
		}
		// curr is head; try detaching via head CAS so next Push lands past prev.
		if tr.head.CompareAndSwap(curr, prev) {
			prev.next.CompareAndSwap(curr, nil)
			tr.dead.Add(-1)
			return
		}
		// CAS lost: a Push raced in; loop, next iteration takes mid-list branch.
	}
}
