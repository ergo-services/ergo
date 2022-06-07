// this is lock-free MPSC queue (Multiple Producers Single Consumer) implementation

package lib

import (
	"sync/atomic"
	"unsafe"
)

type MPSC struct {
	head *item
	tail *item
}

func NewMPSC() *MPSC {
	return &MPSC{}
}

type Item interface {
	Next() Item
	Prev() Item
	Value() interface{}
}

type item struct {
	value interface{}
	prev  *item
	next  *item
}

// Push place given value in the queue head (FIFO)
func (mpsc *MPSC) Push(value interface{}) {
	i := &item{
		value: value,
	}
	i.prev = (*item)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&mpsc.head)), unsafe.Pointer(i)))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&i.prev.next)), unsafe.Pointer(i))
}

// Pop takes the item from the queue tail. Returns nil if queue is empty
func (mpsc *MPSC) Pop() interface{} {
	item := (*item)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&mpsc.tail.next))))
	if item == nil {
		return nil
	}
	return item.value
}

func (mpsc *MPSC) Head() Item {
	return (*item)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&mpsc.head))))
}
func (mpsc *MPSC) Tail() Item {
	return (*item)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&mpsc.tail))))
}

// Next provides walking through the queue. Returns nil if the last item is reached.
func (i *item) Next() Item {
	return (*item)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&i.next))))

}

// Next provides walking through the queue. Returns nil if the first item is reached.
func (i *item) Prev() Item {
	return (*item)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&i.prev))))
}

// Value returns stored value of the queue item
func (i *item) Value() interface{} {
	return i.value
}
