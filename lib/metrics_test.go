package lib

import (
	"runtime"
	"testing"
)

// TestReadRuntimeMetricsHeapObjects: the two object counters must actually be sampled.
// A metric name the running Go version does not know is dropped silently and leaves its
// field zero, so a typo would go unnoticed until a chart drawn from it stayed flat.
func TestReadRuntimeMetricsHeapObjects(t *testing.T) {
	before := ReadRuntimeMetrics()
	if before.HeapAllocObjects == 0 {
		t.Fatal("HeapAllocObjects is zero, the metric name is not sampled")
	}

	// allocate enough that the counter has to move, and keep the objects reachable
	// until after the second reading so the compiler cannot drop them
	kept := make([][]byte, 0, 10000)
	for i := 0; i < 10000; i++ {
		kept = append(kept, make([]byte, 64))
	}

	after := ReadRuntimeMetrics()
	if after.HeapAllocObjects <= before.HeapAllocObjects {
		t.Errorf("HeapAllocObjects did not grow across 10000 allocations: %d -> %d",
			before.HeapAllocObjects, after.HeapAllocObjects)
	}
	if after.HeapFreeObjects < before.HeapFreeObjects {
		t.Errorf("HeapFreeObjects went backwards: %d -> %d",
			before.HeapFreeObjects, after.HeapFreeObjects)
	}
	if after.HeapFreeObjects > after.HeapAllocObjects {
		t.Errorf("more objects freed than allocated: %d > %d",
			after.HeapFreeObjects, after.HeapAllocObjects)
	}
	runtime.KeepAlive(kept)

	// freeing them has to show up on the other counter once a collection has run
	kept = nil
	runtime.GC()

	freed := ReadRuntimeMetrics()
	if freed.HeapFreeObjects <= before.HeapFreeObjects {
		t.Errorf("HeapFreeObjects did not grow after a collection: %d -> %d",
			before.HeapFreeObjects, freed.HeapFreeObjects)
	}
}
