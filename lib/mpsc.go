// High-performance lock-free implementation of MPSC queue (Multiple Producers Single Consumer)

package lib

import (
	"math"
	"sync/atomic"
	"unsafe"
)

type queueMPSC struct {
	head   *itemMPSC
	tail   *itemMPSC
	length int64
	lock   uint32
}

type queueLimitMPSC struct {
	head   *itemMPSC
	tail   *itemMPSC
	length int64
	limit  int64
	flush  bool
	lock   uint32
}

type QueueMPSC interface {
	Push(value any) bool
	Pop() (any, bool)
	Item() ItemMPSC
	// Len returns the number of items in the queue
	Len() int64
	// Size returns the limit for the queue. -1 - for unlimited
	Size() int64

	Lock() bool
	Unlock() bool
}

func NewQueueMPSC() QueueMPSC {
	emptyItem := &itemMPSC{}
	return &queueMPSC{
		head: emptyItem,
		tail: emptyItem,
	}
}

// NewQueueLimitMPSC creates MPSC queue with limited length. Enabling "flush" options
// makes this queue flush out the tail item if the limit has been reached.
// Warning: enabled "flush" option also makes this queue unusable
// for the concurrent environment
func NewQueueLimitMPSC(limit int64, flush bool) QueueMPSC {
	if limit < 1 {
		limit = math.MaxInt64
	}
	emptyItem := &itemMPSC{}
	return &queueLimitMPSC{
		limit: limit,
		flush: flush,
		head:  emptyItem,
		tail:  emptyItem,
	}
}

type ItemMPSC interface {
	Next() ItemMPSC
	Value() any
	Clear()
}

type itemMPSC struct {
	value any
	next  *itemMPSC
}

func (q *queueMPSC) Push(value any) bool {
	i := &itemMPSC{
		value: value,
	}
	atomic.AddInt64(&q.length, 1)
	old_head := (*itemMPSC)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)), unsafe.Pointer(i)))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&old_head.next)), unsafe.Pointer(i))
	return true
}

func (q *queueLimitMPSC) Push(value any) bool {
	if q.Len()+1 > q.limit {
		if q.flush == false {
			return false
		}
		// flush one item to keep the length within the limit
		q.Pop()
	}

	i := &itemMPSC{
		value: value,
	}
	atomic.AddInt64(&q.length, 1)
	old_head := (*itemMPSC)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)), unsafe.Pointer(i)))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&old_head.next)), unsafe.Pointer(i))
	return true
}

func (q *queueMPSC) Pop() (any, bool) {
	tail_next := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail.next))))
	if tail_next == nil {
		return nil, false
	}

	value := tail_next.value
	tail_next.value = nil // let the GC free this item

	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(tail_next))
	atomic.AddInt64(&q.length, -1)
	return value, true
}

func (q *queueLimitMPSC) Pop() (any, bool) {
	tail_next := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail.next))))
	if tail_next == nil {
		return nil, false
	}

	value := tail_next.value
	tail_next.value = nil // let the GC free this item

	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(tail_next))
	atomic.AddInt64(&q.length, -1)
	return value, true
}

func (q *queueMPSC) Len() int64 {
	return atomic.LoadInt64(&q.length)
}

func (q *queueMPSC) Size() int64 {
	return -1 // unlimited
}

func (q *queueMPSC) Lock() bool {
	return atomic.SwapUint32(&q.lock, 1) == 0
}

func (q *queueMPSC) Unlock() bool {
	return atomic.SwapUint32(&q.lock, 0) == 1
}

// Len returns queue length
func (q *queueLimitMPSC) Len() int64 {
	return atomic.LoadInt64(&q.length)
}

func (q *queueLimitMPSC) Size() int64 {
	return q.limit
}

func (q *queueLimitMPSC) Lock() bool {
	return atomic.SwapUint32(&q.lock, 1) == 0
}

func (q *queueLimitMPSC) Unlock() bool {
	return atomic.SwapUint32(&q.lock, 0) == 1
}

// Item returns the tail item of the queue. Returns nil if queue is empty.
func (q *queueMPSC) Item() ItemMPSC {
	tail := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail))))
	item := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&tail.next))))
	if item == nil {
		return nil
	}
	return item
}

// Item returns the tail item of the queue. Returns nil if queue is empty.
func (q *queueLimitMPSC) Item() ItemMPSC {
	tail := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail))))
	item := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&tail.next))))
	if item == nil {
		return nil
	}
	return item
}

//
// ItemMPSC interface implementation
//

// Next provides walking through the queue. Returns nil if the last item is reached.
func (i *itemMPSC) Next() ItemMPSC {
	next := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&i.next))))
	if next == nil {
		return nil
	}
	return next
}

// Value returns stored value of the queue item
func (i *itemMPSC) Value() any {
	return i.value
}

// Clear sets the value to nil. It doesn't remove this item from the queue. Can be used in a single consumer (goroutine) only.
func (i *itemMPSC) Clear() {
	i.value = nil
}
