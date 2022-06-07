// this is a lock-free implementation of MPSC queue (Multiple Producers Single Consumer)

package lib

import (
	"math"
	"sync/atomic"
	"unsafe"
)

type QueueMPSC struct {
	head   *itemMPSC
	tail   *itemMPSC
	length int64
	limit  int64
}

func NewQueueMPSC(limit int64) *QueueMPSC {
	if limit < 1 {
		limit = math.MaxInt64
	}
	emptyItem := &itemMPSC{}
	return &QueueMPSC{
		limit: limit,
		head:  emptyItem,
		tail:  emptyItem,
	}
}

type ItemMPSC interface {
	Next() ItemMPSC
	Value() interface{}
}

type itemMPSC struct {
	value interface{}
	next  *itemMPSC
}

// Push place the given value in the queue head (FIFO). Returns false if exceeded the limit
func (q *QueueMPSC) Push(value interface{}) bool {
	if q.Len()+1 > q.limit {
		return false
	}
	i := &itemMPSC{
		value: value,
	}
	prev := (*itemMPSC)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)), unsafe.Pointer(i)))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&prev.next)), unsafe.Pointer(i))
	atomic.AddInt64(&q.length, 1)
	return true
}

// Pop takes the item from the queue tail. Returns false if the queue is empty. Can be used in a single consumer (goroutine) only.
func (q *QueueMPSC) Pop() (interface{}, bool) {
	if q.Len() == 0 {
		return nil, false
	}
	next := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail.next))))

	value := next.value
	next.value = nil
	atomic.AddInt64(&q.length, -1)
	q.tail = next
	return value, true
}

// Len returns the number of items in the queue
func (q *QueueMPSC) Len() int64 {
	return atomic.LoadInt64(&q.length)
}

// Tail returns the tail item of the queue
func (q *QueueMPSC) Item() ItemMPSC {
	item := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail.next))))
	if item == nil {
		return nil
	}
	return item
}

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
