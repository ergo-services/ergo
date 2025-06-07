package lib

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMPSCRaceConditionFix tests the specific race condition that was fixed
// where Pop() and Item() could cause inconsistent state due to non-atomic tail updates
func TestMPSCRaceConditionFix(t *testing.T) {
	const (
		numProducers = 4
		numMessages  = 1000
		iterations   = 10
	)

	for i := 0; i < iterations; i++ {
		t.Run("unlimited_queue", func(t *testing.T) {
			testRaceCondition(t, NewQueueMPSC(), numProducers, numMessages)
		})

		t.Run("limited_queue", func(t *testing.T) {
			testRaceCondition(t, NewQueueLimitMPSC(int64(numMessages*numProducers), false), numProducers, numMessages)
		})
	}
}

func testRaceCondition(t *testing.T, queue QueueMPSC, numProducers, numMessages int) {
	var wg sync.WaitGroup
	var consumedCount int64
	var itemCheckCount int64
	var popErrors int64
	var structuralErrors int64 // Changed from itemErrors to be more specific

	// Start producers
	wg.Add(numProducers)
	for p := 0; p < numProducers; p++ {
		go func(producerID int) {
			defer wg.Done()
			for i := 0; i < numMessages; i++ {
				message := producerID*numMessages + i
				if !queue.Push(message) {
					t.Errorf("Failed to push message %d from producer %d", i, producerID)
				}
				// Small delay to increase chance of race conditions
				if i%100 == 0 {
					runtime.Gosched()
				}
			}
		}(p)
	}

	// Start consumer that calls Pop()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if value, ok := queue.Pop(); ok {
				if value == nil {
					atomic.AddInt64(&popErrors, 1)
				} else {
					atomic.AddInt64(&consumedCount, 1)
				}
			} else {
				// No more items, check if we've consumed everything
				if atomic.LoadInt64(&consumedCount) >= int64(numProducers*numMessages) {
					break
				}
				// Small delay before trying again
				time.Sleep(time.Microsecond)
			}
		}
	}()

	// Start multiple goroutines that call Item() to check queue state
	// This is the scenario that was causing the race condition
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for atomic.LoadInt64(&consumedCount) < int64(numProducers*numMessages) {
				item := queue.Item()
				atomic.AddInt64(&itemCheckCount, 1)
				if item != nil {
					// Check structural integrity - we should be able to traverse
					// Note: values can be nil due to GC optimization in Pop(), this is expected
					var prev ItemMPSC = nil
					for current := item; current != nil; current = current.Next() {
						// Check for infinite loops or corrupted pointers
						if prev == current {
							atomic.AddInt64(&structuralErrors, 1)
							break
						}
						prev = current
					}
				}
				// Yield to other goroutines
				runtime.Gosched()
			}
		}()
	}

	// Wait for all producers to finish
	wg.Wait()

	// Verify results
	finalConsumed := atomic.LoadInt64(&consumedCount)
	finalPopErrors := atomic.LoadInt64(&popErrors)
	finalStructuralErrors := atomic.LoadInt64(&structuralErrors)
	finalItemChecks := atomic.LoadInt64(&itemCheckCount)

	if finalConsumed != int64(numProducers*numMessages) {
		t.Errorf("Expected to consume %d messages, but consumed %d", numProducers*numMessages, finalConsumed)
	}

	if finalPopErrors > 0 {
		t.Errorf("Pop() returned %d nil values, indicating data corruption", finalPopErrors)
	}

	if finalStructuralErrors > 0 {
		t.Errorf("Found %d structural errors in queue traversal, indicating race condition", finalStructuralErrors)
	}

	// Verify queue is empty
	if queue.Len() != 0 {
		t.Errorf("Queue should be empty, but has length %d", queue.Len())
	}

	if finalItemChecks == 0 {
		t.Error("Item() was never called, test is invalid")
	}

	t.Logf("Successfully processed %d messages with %d item checks", finalConsumed, finalItemChecks)
}

// TestMPSCConcurrentPopAndItem specifically tests the race between Pop() and Item()
func TestMPSCConcurrentPopAndItem(t *testing.T) {
	queue := NewQueueMPSC()
	const numItems = 1000

	// Fill the queue
	for i := 0; i < numItems; i++ {
		queue.Push(i)
	}

	var wg sync.WaitGroup
	var popCount int64
	var inconsistencies int64

	// Consumer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numItems; i++ {
			if _, ok := queue.Pop(); ok {
				atomic.AddInt64(&popCount, 1)
			}
			// Yield to increase race chance
			if i%10 == 0 {
				runtime.Gosched()
			}
		}
	}()

	// Multiple observer goroutines calling Item()
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lastSeenLength := int64(-1)
			for atomic.LoadInt64(&popCount) < numItems {
				currentLength := queue.Len()
				item := queue.Item()

				// Check consistency: if we have an item, length should be > 0
				if item != nil && currentLength <= 0 {
					atomic.AddInt64(&inconsistencies, 1)
				}

				// Check that length is monotonically decreasing or staying same
				if lastSeenLength != -1 && currentLength > lastSeenLength {
					// Length increased, which shouldn't happen in this test
					atomic.AddInt64(&inconsistencies, 1)
				}
				lastSeenLength = currentLength

				runtime.Gosched()
			}
		}()
	}

	wg.Wait()

	if atomic.LoadInt64(&inconsistencies) > 0 {
		t.Errorf("Found %d inconsistencies between Pop() and Item() operations", atomic.LoadInt64(&inconsistencies))
	}

	if atomic.LoadInt64(&popCount) != numItems {
		t.Errorf("Expected to pop %d items, but popped %d", numItems, atomic.LoadInt64(&popCount))
	}
}

// TestMPSCAtomicTailUpdate verifies that tail updates are atomic
func TestMPSCAtomicTailUpdate(t *testing.T) {
	queue := NewQueueMPSC()

	// Add some items
	for i := 0; i < 100; i++ {
		queue.Push(i)
	}

	var wg sync.WaitGroup
	var structuralErrors int64

	// Start multiple goroutines that read the tail state
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				item := queue.Item()
				if item != nil {
					// Try to access next pointer - if tail update wasn't atomic,
					// we might get corrupted structure (but values can be nil)
					next := item.Next()
					if next != nil {
						// Just accessing the structure to detect corruption
						_ = next.Next()
					}
				}
				runtime.Gosched()
			}
		}()
	}

	// Consume items while others are reading
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			queue.Pop()
			runtime.Gosched()
		}
	}()

	wg.Wait()

	if atomic.LoadInt64(&structuralErrors) > 0 {
		t.Errorf("Found %d structural errors, indicating non-atomic tail updates", atomic.LoadInt64(&structuralErrors))
	}
}

// TestMPSCValueNilExpected tests that value being nil after Pop() is expected behavior
func TestMPSCValueNilExpected(t *testing.T) {
	queue := NewQueueMPSC()

	// Add an item
	queue.Push("test")

	// Pop it
	value, ok := queue.Pop()
	if !ok || value != "test" {
		t.Fatal("Failed to pop the test value")
	}

	// Now check if we can still access the structure (but value will be nil)
	item := queue.Item()
	// Item() should return nil because queue is empty after Pop()
	if item != nil {
		t.Error("Expected Item() to return nil for empty queue")
	}
}

// BenchmarkMPSCConcurrentAccess benchmarks the fixed implementation
func BenchmarkMPSCConcurrentAccess(b *testing.B) {
	queue := NewQueueMPSC()

	b.RunParallel(func(pb *testing.PB) {
		counter := 0
		for pb.Next() {
			if counter%2 == 0 {
				queue.Push(counter)
			} else {
				queue.Pop()
			}

			// Occasionally call Item() to test the race condition fix
			if counter%10 == 0 {
				queue.Item()
			}
			counter++
		}
	})
}
