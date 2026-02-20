package lib

type QueueMPSC interface {
	Push(value any) bool
	Pop() (any, bool)
	Item() ItemMPSC
	// Len returns the number of items in the queue
	Len() int64
	// Size returns the limit for the queue. -1 - for unlimited
	Size() int64
	// Latency returns how long the oldest item has been in the queue (nanoseconds).
	// Safe to call from any goroutine.
	// Returns -1 if built without -tags=latency (measurement disabled).
	// Returns 0 if the queue is empty.
	Latency() int64

	Lock() bool
	Unlock() bool
}

type ItemMPSC interface {
	Next() ItemMPSC
	Value() any
	Clear()
}
