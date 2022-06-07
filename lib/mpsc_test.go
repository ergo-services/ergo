package lib

import (
	"fmt"
	"sync"
	"testing"
)

func TestMPSCsequential(t *testing.T) {

	type vv struct {
		v int64
	}
	l := int64(10)
	queue := NewQueueMPSC(l)
	// append to the queue
	for i := int64(0); i < l; i++ {
		v := vv{v: i + 100}
		if queue.Push(v) == false {
			t.Fatal("can't push value into the queue")
		}
		fmt.Println("PUT", v)
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
		fmt.Println("V", v)
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
		fmt.Println("V", v)
		if ok == false || v.v != i+100 {
			t.Fatal("incorrect value. expected", i+100, "got", v)
		}
	}

	// must be empty
	if queue.Len() != 0 {
		t.Fatal("queue length must be 0")
	}

}

func TestMPSCparallel(t *testing.T) {

	type vv struct {
		v int64
	}
	l := int64(100000)
	queue := NewQueueMPSC(l)
	sum := int64(0)
	// append to the queue
	var wg sync.WaitGroup
	for i := int64(0); i < l; i++ {
		v := vv{v: i + 100}
		sum += v.v
		wg.Add(1)
		go func(v vv) {
			if queue.Push(v) == false {
				t.Fatal("can't push value into the queue")
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
