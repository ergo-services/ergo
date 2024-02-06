package lib

import (
	"math/rand"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

func TestMPSCsequential(t *testing.T) {

	type vv struct {
		v int64
	}
	l := int64(10)
	queue := NewQueueLimitMPSC(l, false)
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
	queue := NewQueueLimitMPSC(l, false)
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

type chanQueue struct {
	q chan interface{}
}

type testQueue interface {
	Push(v interface{}) bool
	Pop() (interface{}, bool)
}

func newChanQueue() *chanQueue {
	chq := &chanQueue{
		q: make(chan interface{}, 100000000),
	}
	return chq
}

// Enqueue puts the given value v at the tail of the queue.
func (cq *chanQueue) Push(v interface{}) bool {
	select {
	case cq.q <- v:
		return true
	default:
		panic("channel is full")
	}
}

func (cq *chanQueue) Pop() (interface{}, bool) {
	v := <-cq.q
	return v, true
}
func BenchmarkMPSC(b *testing.B) {
	queues := map[string]testQueue{
		"Chan queue           ": newChanQueue(),
		"MPSC queue           ": NewQueueMPSC(),
		"MPSC with limit queue": NewQueueLimitMPSC(0, false),
	}

	length := 1 << 12
	inputs := make([]int, length)
	for i := 0; i < length; i++ {
		inputs = append(inputs, rand.Int())
	}

	for _, cpus := range []int{4, 32, 1024} {
		runtime.GOMAXPROCS(cpus)
		for name, q := range queues {
			b.Run(name+"#"+strconv.Itoa(cpus), func(b *testing.B) {
				b.ResetTimer()

				var c int64
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						i := int(atomic.AddInt64(&c, 1)-1) % length
						v := inputs[i]
						if v >= 0 {
							q.Push(v)
						} else {
							q.Pop()
						}
					}
				})
			})
		}
	}
}
