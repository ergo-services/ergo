// this is a lock-free implementation of MPSC queue (Multiple Producers Single Consumer)

package lib

import (
	"sync/atomic"
	"unsafe"
)

type QueueMPSC struct {
	head *itemMPSC
	tail *itemMPSC
}

func NewQueueMPSC() *QueueMPSC {
	return &QueueMPSC{}
}

type ItemMPSC interface {
	Next() ItemMPSC
	Prev() ItemMPSC
	Value() interface{}
}

type itemMPSC struct {
	value interface{}
	prev  *itemMPSC
	next  *itemMPSC
}

// Push place the given value in the queue head (FIFO)
func (q *QueueMPSC) Push(value interface{}) {
	i := &itemMPSC{
		value: value,
	}
	i.prev = (*itemMPSC)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)), unsafe.Pointer(i)))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&i.prev.next)), unsafe.Pointer(i))
}

// Pop takes the item from the queue tail. Returns nil if the queue is empty. Can be used in a single consumer (goroutine) only.
func (q *QueueMPSC) Pop() interface{} {
	item := (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail.next))))
	if item == nil {
		return nil
	}
	return item.value
}

// Head returns the head item of the queue
func (q *QueueMPSC) Head() ItemMPSC {
	return (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head))))
}

// Tail returns the tail item of the queue
func (q *QueueMPSC) Tail() ItemMPSC {
	return (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail))))
}

// Next provides walking through the queue. Returns nil if the last item is reached.
func (i *itemMPSC) Next() ItemMPSC {
	return (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&i.next))))
}

// Next provides walking through the queue. Returns nil if the first item is reached.
func (i *itemMPSC) Prev() ItemMPSC {
	return (*itemMPSC)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&i.prev))))
}

// Value returns stored value of the queue item
func (i *itemMPSC) Value() interface{} {
	return i.value
}
