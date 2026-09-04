package lib

import (
	"sync/atomic"
	"testing"
	"time"
)

// benchPayload is a multi-word value (roughly a tracing span in size) so the any-boxing
// cost the MPSC queue pays is representative.
type benchPayload struct {
	a, b, c uint64
	d       [2]uint64
	s       string
	p       gen0PID
}

type gen0PID struct {
	node     string
	id       uint64
	creation int64
}

func TestDispatcherDeliversInOrder(t *testing.T) {
	const n = 2000
	var next atomic.Int64
	bad := make(chan int, 1)
	d := NewDispatcher[int](4096, func(v int) {
		want := int(next.Load())
		if v != want {
			select {
			case bad <- v:
			default:
			}
		}
		next.Add(1)
	})
	for i := 0; i < n; i++ {
		d.Push(i)
	}

	deadline := time.Now().Add(3 * time.Second)
	for next.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d delivered", next.Load(), n)
		}
		time.Sleep(time.Millisecond)
	}
	d.Stop()

	select {
	case v := <-bad:
		t.Fatalf("out-of-order value %d", v)
	default:
	}
	if d.Dropped() != 0 {
		t.Fatalf("unexpected drops: %d", d.Dropped())
	}
}

func TestDispatcherDropsWhenFull(t *testing.T) {
	release := make(chan struct{})
	d := NewDispatcher[int](64, func(int) { <-release })

	for i := 0; i < 64+200; i++ {
		d.Push(i) // worker is stuck on the first value, so the queue fills and the rest drop
	}
	if d.Dropped() == 0 {
		t.Fatal("expected drops once the queue was full")
	}

	close(release)
	d.Stop() // must not hang
}

// A non-positive limit must fall back to a bounded queue, never an unbounded one: a stalled
// handler must make Push drop rather than grow memory without limit.
func TestDispatcherZeroLimitIsBounded(t *testing.T) {
	release := make(chan struct{})
	d := NewDispatcher[int](0, func(int) { <-release })

	for i := 0; i < defaultDispatcherLimit+500; i++ {
		d.Push(i) // worker stuck on the first value; the queue must fill and drop the rest
	}
	if d.Dropped() == 0 {
		t.Fatal("limit 0 produced an unbounded queue (no drops under a stalled handler)")
	}

	close(release)
	d.Stop()
}

// Stop returns the values still queued (not yet handled) so the caller can flush or inspect
// them rather than lose them silently.
func TestDispatcherStopReturnsRemainder(t *testing.T) {
	inHandler := make(chan struct{}, 1)
	release := make(chan struct{})
	d := NewDispatcher[int](1024, func(int) {
		select {
		case inHandler <- struct{}{}:
		default:
		}
		<-release
	})

	const n = 10
	for i := 0; i < n; i++ {
		d.Push(i)
	}
	<-inHandler // the worker is stuck inside the first value; the rest are queued

	got := make(chan []int, 1)
	go func() { got <- d.Stop() }() // sets stopped, then blocks until the worker exits
	time.Sleep(20 * time.Millisecond)
	close(release) // the worker finishes the first value, sees stopped, exits without draining the rest

	rest := <-got
	if len(rest) == 0 {
		t.Fatal("Stop returned no remainder while values were still queued")
	}
	if len(rest) > n {
		t.Fatalf("Stop returned %d values, more than the %d pushed", len(rest), n)
	}
}

// concreteDispatcher mirrors Dispatcher but is hand-written for benchPayload (no generics),
// to measure the cost of the type parameter against a specialized implementation.
type concreteDispatcher struct {
	queue   QueueMPSC
	handle  func(benchPayload)
	state   atomic.Int32
	dropped atomic.Uint64
	done    chan struct{}
}

func newConcreteDispatcher(limit int, handle func(benchPayload)) *concreteDispatcher {
	return &concreteDispatcher{
		queue:  NewQueueLimitMPSC(int64(limit)),
		handle: handle,
		done:   make(chan struct{}),
	}
}

func (d *concreteDispatcher) Push(v benchPayload) bool {
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

func (d *concreteDispatcher) run() {
	for {
		v, ok := d.queue.Pop()
		if ok {
			d.handle(v.(benchPayload))
			if d.state.Load() == dispatcherStopped {
				close(d.done)
				return
			}
			continue
		}
		if d.state.CompareAndSwap(dispatcherRunning, dispatcherIdle) == false {
			close(d.done)
			return
		}
		if d.queue.Len() == 0 {
			return
		}
		if d.state.CompareAndSwap(dispatcherIdle, dispatcherRunning) == false {
			return
		}
	}
}

func (d *concreteDispatcher) Stop() []benchPayload {
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
	var rest []benchPayload
	for {
		v, ok := d.queue.Pop()
		if ok == false {
			return rest
		}
		rest = append(rest, v.(benchPayload))
	}
}

func BenchmarkDispatcherGeneric(b *testing.B) {
	d := NewDispatcher[benchPayload](1<<16, func(benchPayload) {})
	var p benchPayload
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Push(p)
	}
	b.StopTimer()
	d.Stop()
}

func BenchmarkDispatcherConcrete(b *testing.B) {
	d := newConcreteDispatcher(1<<16, func(benchPayload) {})
	var p benchPayload
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Push(p)
	}
	b.StopTimer()
	d.Stop()
}
