package lib

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestOptimizedBasic(t *testing.T) {
	queue := NewQueueMPSC()

	// Test basic push/pop
	if !queue.Push(1) {
		t.Fatal("Failed to push")
	}

	if queue.Len() != 1 {
		t.Fatal("Length should be 1")
	}

	value, ok := queue.Pop()
	if !ok || value != 1 {
		t.Fatal("Failed to pop correct value")
	}

	if queue.Len() != 0 {
		t.Fatal("Length should be 0")
	}
}

func TestOptimizedRapidPushPop(t *testing.T) {
	queue := NewQueueMPSC()

	// Test rapid push/pop to stress the pool
	for i := 0; i < 100; i++ {
		if !queue.Push(i) {
			t.Fatalf("Failed to push at iteration %d", i)
		}

		value, ok := queue.Pop()
		if !ok || value != i {
			t.Fatalf("Failed to pop correct value at iteration %d, got %v", i, value)
		}
	}
}

func TestOptimizedDeadlockCheck(t *testing.T) {
	queue := NewQueueMPSC()

	// Test with timeout to detect deadlocks
	done := make(chan bool, 1)

	go func() {
		for i := 0; i < 1000; i++ {
			queue.Push(i)
			queue.Pop()
		}
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Deadlock detected - test timed out")
	}
}

func TestOptimizedSimpleMultiProducer(t *testing.T) {
	queue := NewQueueMPSC()
	const numProducers = 4
	const itemsPerProducer = 100

	done := make(chan bool, 1)
	var wg sync.WaitGroup

	go func() {
		// Producers
		for i := 0; i < numProducers; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < itemsPerProducer; j++ {
					queue.Push(id*1000 + j)
					if j%10 == 0 {
						runtime.Gosched() // Yield occasionally
					}
				}
			}(i)
		}

		// Consumer
		wg.Add(1)
		go func() {
			defer wg.Done()
			consumed := 0
			target := numProducers * itemsPerProducer

			for consumed < target {
				if _, ok := queue.Pop(); ok {
					consumed++
				} else {
					runtime.Gosched()
				}
			}
		}()

		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Success
		t.Logf("Successfully processed %d items", numProducers*itemsPerProducer)
	case <-time.After(5 * time.Second):
		t.Fatal("Multi-producer test timed out - possible deadlock")
	}
}
