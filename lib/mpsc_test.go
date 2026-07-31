package lib

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestMPSCsequential(t *testing.T) {

	type vv struct {
		v int64
	}
	l := int64(10)
	queue := NewQueueLimitMPSC(l)
	// append to the queue
	for i := int64(0); i < l; i++ {
		v := vv{v: i + 100}
		if queue.Push(v) == false {
			t.Fatal("can't push value into the queue")
		}
	}
	if queue.Len() != l {
		t.Fatal("queue length must be 10")
	}

	if queue.Push("must be failed") == true {
		t.Fatal("must be false: exceeded the limit", queue.Len())
	}

	// walking through the queue
	item := queue.Item()
	for i := int64(0); i < l; i++ {
		v, ok := item.Value().(vv)
		if ok == false || v.v != i+100 {
			t.Fatal("incorrect value. expected", i+100, "got", v)
		}

		item = item.Next()

	}
	if item != nil {
		t.Fatal("there is something else in the queue", item.Value())
	}

	// popping from the queue
	for i := int64(0); i < l; i++ {
		value, ok := queue.Pop()
		if ok == false {
			t.Fatal("there must be value")
		}
		v, ok := value.(vv)
		if ok == false || v.v != i+100 {
			t.Fatal("incorrect value. expected", i+100, "got", v)
		}
	}

	// must be empty
	if queue.Len() != 0 {
		t.Fatal("queue length must be 0")
	}

	// check Clear method
	if ok := queue.Push(vv{v: 100}); ok == false {
		t.Fatal("must be true here")
	}

	item = queue.Item()
	if item == nil {
		t.Fatal("item is nil")
	}
	item.Clear()
	value, ok := queue.Pop()
	if ok == false {
		t.Fatal("must be true here")
	}
	if value != nil {
		t.Fatal("must be nil here")
	}
}

func TestMPSCparallel(t *testing.T) {

	type vv struct {
		v int64
	}
	l := int64(100000)
	queue := NewQueueLimitMPSC(l)
	sum := int64(0)
	// append to the queue
	var wg sync.WaitGroup
	for i := int64(0); i < l; i++ {
		v := vv{v: i + 100}
		sum += v.v
		wg.Add(1)
		go func(v vv) {
			if queue.Push(v) == false {
				panic("can't push value into the queue")
			}
			wg.Done()
		}(v)
	}
	wg.Wait()
	if x := queue.Len(); x != l {
		t.Fatal("queue length must be", l, "have", x)
	}

	if queue.Push("must be failed") == true {
		t.Fatal("must be false: exceeded the limit", queue.Len())
	}

	// walking through the queue
	item := queue.Item()
	sum1 := int64(0)
	for i := int64(0); i < l; i++ {
		v, ok := item.Value().(vv)
		sum1 += v.v
		if ok == false {
			t.Fatal("incorrect value. got", v)
		}

		item = item.Next()

	}
	if item != nil {
		t.Fatal("there is something else in the queue", item.Value())
	}
	if sum != sum1 {
		t.Fatal("wrong value. exp", sum, "got", sum1)
	}

	sum1 = 0
	// popping from the queue
	for i := int64(0); i < l; i++ {
		value, ok := queue.Pop()
		if ok == false {
			t.Fatal("there must be value")
		}
		v, ok := value.(vv)
		sum1 += v.v
		if ok == false {
			t.Fatal("incorrect value. got", v)
		}
	}

	// must be empty
	if queue.Len() != 0 {
		t.Fatal("queue length must be 0")
	}
	if sum != sum1 {
		t.Fatal("wrong value. exp", sum, "got", sum1)
	}
}

// TestQueueLimitMPSCConcurrentLimit releases many producers at once against a bounded
// (non-flush) queue of limit 1 with no consumer: exactly one push may succeed and the
// queue must hold exactly one item. The start barrier piles every producer onto the very
// first slot, so they all read Len()==0 together - the old Len()-then-link check let them
// all pass and overshoot, which this reliably exposes.
func TestQueueLimitMPSCConcurrentLimit(t *testing.T) {
	const limit = 1
	const producers = 256

	q := NewQueueLimitMPSC(limit)

	var ready, done sync.WaitGroup
	ready.Add(producers)
	done.Add(producers)
	release := make(chan struct{})
	var pushed int64
	for p := 0; p < producers; p++ {
		go func() {
			defer done.Done()
			ready.Done()
			<-release // all producers unblock together and hammer the boundary
			if q.Push(0) {
				atomic.AddInt64(&pushed, 1)
			}
		}()
	}
	ready.Wait()
	close(release)
	done.Wait()

	if pushed != limit {
		t.Fatalf("successful pushes = %d, want exactly %d (limit not enforced under concurrency)", pushed, limit)
	}
	if q.Len() != limit {
		t.Fatalf("Len() = %d, want %d", q.Len(), limit)
	}

	// the queue must actually hold exactly `limit` items
	var drained int64
	for {
		if _, ok := q.Pop(); ok == false {
			break
		}
		drained++
	}
	if drained != limit {
		t.Fatalf("drained %d items, want %d", drained, limit)
	}
}

// TestQueueLimitMPSCUnbounded: a limit below 1 yields an unbounded queue (Size() == -1)
// that accepts far more items than any positive limit would.
func TestQueueLimitMPSCUnbounded(t *testing.T) {
	q := NewQueueLimitMPSC(0)
	if q.Size() != -1 {
		t.Fatalf("Size() = %d, want -1 (unbounded)", q.Size())
	}
	const n = 10000
	for i := 0; i < n; i++ {
		if q.Push(i) == false {
			t.Fatalf("unbounded queue rejected push %d", i)
		}
	}
	if q.Len() != n {
		t.Fatalf("Len() = %d, want %d", q.Len(), n)
	}
}
