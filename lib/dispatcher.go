package lib

import "sync/atomic"

const (
	dispatcherIdle    int32 = 0
	dispatcherRunning int32 = 1
	dispatcherStopped int32 = 2

	// defaultDispatcherLimit bounds the queue when the caller passes a non-positive limit,
	// so a dispatcher is never accidentally unbounded (that would let a stalled handler grow
	// memory without limit instead of dropping).
	defaultDispatcherLimit = 1024
)

// Dispatcher decouples producers from a single handler through a bounded MPSC queue and a
// worker that runs only while there is work: it is spawned on Push when idle and exits once
// the queue drains, so an idle dispatcher holds no goroutine (like a sleeping process). The
// queue grows lazily; nothing is pre-allocated. Push is non-blocking - a full queue drops
// the value and counts it. The handler is never called concurrently with itself.
type Dispatcher[T any] struct {
	queue   QueueMPSC
	handle  func(T)
	state   atomic.Int32
	dropped atomic.Uint64
	done    chan struct{} // closed by the worker once it observes Stop
}

// NewDispatcher builds a dispatcher whose worker passes every pushed value to handle. limit
// bounds the queue length; a non-positive limit uses defaultDispatcherLimit. The queue is
// always bounded, so a stalled handler makes Push drop rather than grow memory unbounded.
func NewDispatcher[T any](limit int, handle func(T)) *Dispatcher[T] {
	if limit < 1 {
		limit = defaultDispatcherLimit
	}
	return &Dispatcher[T]{
		queue:  NewQueueLimitMPSC(int64(limit)),
		handle: handle,
		done:   make(chan struct{}),
	}
}

// Push hands a value to the worker without blocking and reports whether it was queued. A
// full queue or a stopped dispatcher drops the value, counts it, and returns false, so a
// producer that cares can back off, retry or surface an error instead of losing it silently.
func (d *Dispatcher[T]) Push(v T) bool {
	if d.state.Load() == dispatcherStopped {
		return false
	}
	if d.queue.Push(v) == false {
		d.dropped.Add(1)
		return false
	}
	if d.state.CompareAndSwap(dispatcherIdle, dispatcherRunning) {
		go d.run()
	}
	return true
}

func (d *Dispatcher[T]) run() {
	for {
		v, ok := d.queue.Pop()
		if ok {
			d.handle(v.(T))
			// stop between spans: finish the current one, then drop the rest
			if d.state.Load() == dispatcherStopped {
				close(d.done)
				return
			}
			continue
		}
		if d.state.CompareAndSwap(dispatcherRunning, dispatcherIdle) == false {
			// Stop() flipped us to stopped while we held running
			close(d.done)
			return
		}
		// re-check for values pushed between the last Pop and going idle
		if d.queue.Len() == 0 {
			return
		}
		if d.state.CompareAndSwap(dispatcherIdle, dispatcherRunning) == false {
			// a concurrent Push spawned a fresh worker, or Stop took over
			return
		}
	}
}

// Dropped returns how many values have been dropped so far because the queue was full.
func (d *Dispatcher[T]) Dropped() uint64 {
	return d.dropped.Load()
}

// Stop prevents further work, waits for any in-flight handler to finish, and returns the
// values still queued so the caller can flush, inspect or discard them instead of losing
// them silently. After Stop returns the handler is guaranteed not to be running. Best-effort:
// a Push that raced Stop may or may not be included. Idempotent; a later Stop returns nil.
func (d *Dispatcher[T]) Stop() []T {
	for {
		old := d.state.Load()
		if old == dispatcherStopped {
			return nil
		}
		if d.state.CompareAndSwap(old, dispatcherStopped) {
			if old == dispatcherRunning {
				<-d.done
			}
			break
		}
	}
	// the worker has exited; drain the remainder as the sole consumer
	var rest []T
	for {
		v, ok := d.queue.Pop()
		if ok == false {
			return rest
		}
		rest = append(rest, v.(T))
	}
}
