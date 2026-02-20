//go:build latency

// High-performance lock-free MPSC queue with head-of-line latency measurement.
// Latency() returns how long the oldest item has been sitting in the queue.

package lib

import (
	"math"
	"sync/atomic"
	"unsafe"
	_ "unsafe"
)

//go:linkname nanotime runtime.nanotime
func nanotime() int64

type queueMPSCLatency struct {
	head   *itemMPSCLatency
	tail   *itemMPSCLatency
	length int64
	oldest int64 // nanotime of the oldest item (head of queue)
	lock   uint32
}

type queueLimitMPSCLatency struct {
	head   *itemMPSCLatency
	tail   *itemMPSCLatency
	length int64
	oldest int64
	limit  int64
	flush  bool
	lock   uint32
}

type itemMPSCLatency struct {
	value  any
	next   *itemMPSCLatency
	pushed int64
}

func NewQueueMPSC() QueueMPSC {
	emptyItem := &itemMPSCLatency{}
	return &queueMPSCLatency{
		head: emptyItem,
		tail: emptyItem,
	}
}

// NewQueueLimitMPSC creates MPSC queue with limited length and latency measurement.
// Enabling "flush" option makes this queue flush out the tail item if the limit has been reached.
// Warning: enabled "flush" option also makes this queue unusable
// for the concurrent environment
func NewQueueLimitMPSC(limit int64, flush bool) QueueMPSC {
	if limit < 1 {
		limit = math.MaxInt64
	}
	emptyItem := &itemMPSCLatency{}
	return &queueLimitMPSCLatency{
		limit: limit,
		flush: flush,
		head:  emptyItem,
		tail:  emptyItem,
	}
}

//
// queueMPSCLatency
//

func (q *queueMPSCLatency) Push(value any) bool {
	i := &itemMPSCLatency{
		value:  value,
		pushed: nanotime(),
	}
	atomic.AddInt64(&q.length, 1)
	old_head := (*itemMPSCLatency)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)), unsafe.Pointer(i)))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&old_head.next)), unsafe.Pointer(i))
	atomic.CompareAndSwapInt64(&q.oldest, 0, i.pushed)
	return true
}

func (q *queueMPSCLatency) Pop() (any, bool) {
	tail := (*itemMPSCLatency)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail))))
	tail_next := (*itemMPSCLatency)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&tail.next))))
	if tail_next == nil {
		return nil, false
	}

	value := tail_next.value
	tail_next.value = nil

	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(tail_next))
	atomic.AddInt64(&q.length, -1)

	// update oldest: check next item in queue
	next := (*itemMPSCLatency)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&tail_next.next))))
	if next != nil {
		atomic.StoreInt64(&q.oldest, next.pushed)
	} else {
		atomic.StoreInt64(&q.oldest, 0)
	}

	return value, true
}

func (q *queueMPSCLatency) Latency() int64 {
	ts := atomic.LoadInt64(&q.oldest)
	if ts == 0 {
		return 0
	}
	return nanotime() - ts
}

func (q *queueMPSCLatency) Len() int64 {
	return atomic.LoadInt64(&q.length)
}

func (q *queueMPSCLatency) Size() int64 {
	return -1
}

func (q *queueMPSCLatency) Lock() bool {
	return atomic.SwapUint32(&q.lock, 1) == 0
}

func (q *queueMPSCLatency) Unlock() bool {
	return atomic.SwapUint32(&q.lock, 0) == 1
}

func (q *queueMPSCLatency) Item() ItemMPSC {
	tail := (*itemMPSCLatency)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail))))
	item := (*itemMPSCLatency)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&tail.next))))
	if item == nil {
		return nil
	}
	return item
}

//
// queueLimitMPSCLatency
//

func (q *queueLimitMPSCLatency) Push(value any) bool {
	if q.Len()+1 > q.limit {
		if q.flush == false {
			return false
		}
		q.Pop()
	}

	i := &itemMPSCLatency{
		value:  value,
		pushed: nanotime(),
	}
	atomic.AddInt64(&q.length, 1)
	old_head := (*itemMPSCLatency)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)), unsafe.Pointer(i)))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&old_head.next)), unsafe.Pointer(i))
	atomic.CompareAndSwapInt64(&q.oldest, 0, i.pushed)
	return true
}

func (q *queueLimitMPSCLatency) Pop() (any, bool) {
	tail := (*itemMPSCLatency)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail))))
	tail_next := (*itemMPSCLatency)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&tail.next))))
	if tail_next == nil {
		return nil, false
	}

	value := tail_next.value
	tail_next.value = nil

	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(tail_next))
	atomic.AddInt64(&q.length, -1)

	next := (*itemMPSCLatency)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&tail_next.next))))
	if next != nil {
		atomic.StoreInt64(&q.oldest, next.pushed)
	} else {
		atomic.StoreInt64(&q.oldest, 0)
	}

	return value, true
}

func (q *queueLimitMPSCLatency) Latency() int64 {
	ts := atomic.LoadInt64(&q.oldest)
	if ts == 0 {
		return 0
	}
	return nanotime() - ts
}

func (q *queueLimitMPSCLatency) Len() int64 {
	return atomic.LoadInt64(&q.length)
}

func (q *queueLimitMPSCLatency) Size() int64 {
	return q.limit
}

func (q *queueLimitMPSCLatency) Lock() bool {
	return atomic.SwapUint32(&q.lock, 1) == 0
}

func (q *queueLimitMPSCLatency) Unlock() bool {
	return atomic.SwapUint32(&q.lock, 0) == 1
}

func (q *queueLimitMPSCLatency) Item() ItemMPSC {
	tail := (*itemMPSCLatency)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail))))
	item := (*itemMPSCLatency)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&tail.next))))
	if item == nil {
		return nil
	}
	return item
}

//
// itemMPSCLatency implements ItemMPSC
//

func (i *itemMPSCLatency) Next() ItemMPSC {
	next := (*itemMPSCLatency)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&i.next))))
	if next == nil {
		return nil
	}
	return next
}

func (i *itemMPSCLatency) Value() any {
	return i.value
}

func (i *itemMPSCLatency) Clear() {
	i.value = nil
}
