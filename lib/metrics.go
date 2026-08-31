package lib

import (
	"runtime/metrics"
	"sync"
)

const (
	metricMemoryTotal = "/memory/classes/total:bytes"
	metricHeapObjects = "/memory/classes/heap/objects:bytes"
	metricMemoryLimit = "/gc/gomemlimit:bytes"
	metricHeapLive    = "/gc/heap/live:bytes"
	metricHeapGoal    = "/gc/heap/goal:bytes"
	metricGoroutines  = "/sched/goroutines:goroutines"
	metricGCCycles    = "/gc/cycles/total:gc-cycles"
	metricHeapAllocs  = "/gc/heap/allocs:objects"
	metricHeapFrees   = "/gc/heap/frees:objects"
	metricCPUGC       = "/cpu/classes/gc/total:cpu-seconds"
	metricCPUTotal    = "/cpu/classes/total:cpu-seconds"
)

// RuntimeMetrics contains the runtime counters sampled through runtime/metrics.
// A metric unknown to the running Go version leaves its field zero.
type RuntimeMetrics struct {
	// MemoryTotal is the total memory obtained from the OS, in bytes.
	MemoryTotal uint64

	// MemoryObjects is the memory occupied by live heap objects, in bytes.
	MemoryObjects uint64

	// MemoryLimit is the soft memory limit set via GOMEMLIMIT, in bytes.
	// MaxInt64 means no limit is set.
	MemoryLimit uint64

	// HeapLive is the heap memory occupied as of the last garbage collection, in bytes.
	HeapLive uint64

	// HeapGoal is the heap size that triggers the next garbage collection, in bytes.
	HeapGoal uint64

	// Goroutines is the current number of goroutines.
	Goroutines int64

	// GCCycles is the cumulative number of completed garbage collection cycles.
	GCCycles uint64

	// HeapAllocObjects is the cumulative number of heap objects allocated.
	HeapAllocObjects uint64

	// HeapFreeObjects is the cumulative number of heap objects freed. The difference
	// with HeapAllocObjects is the number of objects currently alive.
	HeapFreeObjects uint64

	// GCCPUFraction is the share of total CPU time spent in garbage collection.
	GCCPUFraction float64

	// CPUTimeGC is the cumulative CPU time spent in garbage collection, in seconds.
	CPUTimeGC float64

	// CPUTimeTotal is the cumulative CPU time available to the process, in seconds,
	// as defined by GOMAXPROCS. Includes idle time.
	CPUTimeTotal float64
}

var runtimeMetrics = []string{
	metricMemoryTotal,
	metricHeapObjects,
	metricMemoryLimit,
	metricHeapLive,
	metricHeapGoal,
	metricGoroutines,
	metricGCCycles,
	metricHeapAllocs,
	metricHeapFrees,
	metricCPUGC,
	metricCPUTotal,
}

// supportedRuntimeMetrics keeps the names the running Go version knows. The
// metric set grows between Go releases, and an unknown name yields KindBad.
var supportedRuntimeMetrics = sync.OnceValue(func() []string {
	known := make(map[string]bool)
	for _, d := range metrics.All() {
		known[d.Name] = true
	}

	supported := make([]string, 0, len(runtimeMetrics))
	for _, name := range runtimeMetrics {
		if known[name] {
			supported = append(supported, name)
		}
	}
	return supported
})

// ReadRuntimeMetrics samples the runtime counters. Unlike runtime.ReadMemStats
// it does not stop the world.
func ReadRuntimeMetrics() RuntimeMetrics {
	var rm RuntimeMetrics

	names := supportedRuntimeMetrics()
	samples := make([]metrics.Sample, len(names))
	for i, name := range names {
		samples[i].Name = name
	}
	metrics.Read(samples)

	var cpuGC, cpuTotal float64

	for _, sample := range samples {
		switch sample.Value.Kind() {
		case metrics.KindUint64:
			value := sample.Value.Uint64()
			switch sample.Name {
			case metricMemoryTotal:
				rm.MemoryTotal = value
			case metricHeapObjects:
				rm.MemoryObjects = value
			case metricMemoryLimit:
				rm.MemoryLimit = value
			case metricHeapLive:
				rm.HeapLive = value
			case metricHeapGoal:
				rm.HeapGoal = value
			case metricGoroutines:
				rm.Goroutines = int64(value)
			case metricGCCycles:
				rm.GCCycles = value
			case metricHeapAllocs:
				rm.HeapAllocObjects = value
			case metricHeapFrees:
				rm.HeapFreeObjects = value
			}

		case metrics.KindFloat64:
			switch sample.Name {
			case metricCPUGC:
				cpuGC = sample.Value.Float64()
			case metricCPUTotal:
				cpuTotal = sample.Value.Float64()
			}
		}
	}

	rm.CPUTimeGC = cpuGC
	rm.CPUTimeTotal = cpuTotal
	if cpuTotal > 0 {
		rm.GCCPUFraction = cpuGC / cpuTotal
	}

	return rm
}
