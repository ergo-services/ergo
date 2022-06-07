// this is a lock-free implementation of MPSC queue (Multiple Producers Single Consumer)

package lib

import (
	"math"
	"sync/atomic"
	"unsafe"
)

type queueMPSC struct {
	head *itemMPSC
	tail *itemMPSC
}

type queueLimitMPSC struct {
	head   *itemMPSC
	tail   *itemMPSC
	length int64
	limit  int64
}

type QueueMPSC interface {
	Push(value interface{}) bool
	Pop() (interface{}, bool)
	Item() ItemMPSC
	// Len returns the number of items in the queue
	Len() int64
}

func NewQueueMPSC() QueueMPSC {
	emptyItem := &itemMPSC{}
	return &queueMPSC{
		head: emptyItem,
		tail: emptyItem,
	}
}

func NewQueueLimitMPSC(limit int64) QueueMPSC {
	if limit < 1 {
		limit = math.MaxInt64
	}
	emptyItem := &itemMPSC{}
	return &queueLimitMPSC{
		limit: limit,
		head:  emptyItem,
		tail:  emptyItem,
	}
}

type ItemMPSC interface {
	Next() ItemMPSC
	Value() interface{}
	Clear()
}

type itemMPSC struct {
	value interface{}
	next  *itemMPSC
}

// Push place the given value in the queue head (FIFO). Returns false if exceeded the limit
func (q *queueMPSC) Push(value interface{}) bool {
	i := &itemMPSC{
		value: value,
	}
	old_head := (*itemMPSC)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)), unsafe.Pointer(i)))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&old_head.next)), unsafe.Pointer(i))
	return true
}

// Push place the given value in the queue head (FIFO). Returns false if exceeded the limit
func (q *queueLimitMPSC) Push(value interface{}) bool {
	if q.Len()+1 > q.limit {
		return false
	}
	atomic.AddInt64(&q.length, 1)
	i := &itemMPSC{
		value: value,
	}
	old_head := (*itemMPSC)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)), unsafe.Pointer(i)))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&old_head.next)), unsafe.Pointer(i))
	return true
}

// Pop takes the item from the queue tail. Returns false if the queue is empty. Can be used in a single consumer (goroutine) only.
func (q *queueMPSC) Pop() (interface{}, bool) {
	tail_next := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail.next))))
	if tail_next == nil {
		return nil, false
	}

	value := tail_next.value
	tail_next.value = nil // let the GC free this item
	q.tail = tail_next
	return value, true
}

func (q *queueLimitMPSC) Pop() (interface{}, bool) {
	tail_next := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail.next))))
	if tail_next == nil {
		return nil, false
	}

	value := tail_next.value
	tail_next.value = nil // let the GC free this item
	q.tail = tail_next
	atomic.AddInt64(&q.length, -1)
	return value, true
}

func (q *queueMPSC) Len() int64 {
	return -1
}

func (q *queueLimitMPSC) Len() int64 {
	return atomic.LoadInt64(&q.length)
}

// Item returns the tail item of the queue. Returns nil if queue is empty.
func (q *queueMPSC) Item() ItemMPSC {
	item := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail.next))))
	if item == nil {
		return nil
	}
	return item
}

// Item returns the tail item of the queue. Returns nil if queue is empty.
func (q *queueLimitMPSC) Item() ItemMPSC {
	item := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail.next))))
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
func (i *itemMPSC) Value() interface{} {
	return i.value
}

// Clear sets the value to nil. It doesn't remove this item from the queue. Can be used in a signle consumer (goroutine) only.
func (i *itemMPSC) Clear() {
	i.value = nil
}
