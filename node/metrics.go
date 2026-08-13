package node

import (
	"runtime/metrics"
	"sync"

	"ergo.services/ergo/gen"
)

// Runtime metrics sampled for gen.NodeShortInfo.
const (
	metricMemoryTotal = "/memory/classes/total:bytes"
	metricHeapObjects = "/memory/classes/heap/objects:bytes"
	metricMemoryLimit = "/gc/gomemlimit:bytes"
	metricHeapLive    = "/gc/heap/live:bytes"
	metricHeapGoal    = "/gc/heap/goal:bytes"
	metricGoroutines  = "/sched/goroutines:goroutines"
	metricGCCycles    = "/gc/cycles/total:gc-cycles"
	metricCPUGC       = "/cpu/classes/gc/total:cpu-seconds"
	metricCPUTotal    = "/cpu/classes/total:cpu-seconds"
)

var runtimeMetrics = []string{
	metricMemoryTotal,
	metricHeapObjects,
	metricMemoryLimit,
	metricHeapLive,
	metricHeapGoal,
	metricGoroutines,
	metricGCCycles,
	metricCPUGC,
	metricCPUTotal,
}

// supportedRuntimeMetrics keeps the names this Go runtime knows. The metric set
// grows between Go releases, and reading an unknown name yields KindBad.
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

// readRuntimeMetrics fills the runtime part of info. Metrics missing on this
// runtime leave their fields zero.
func readRuntimeMetrics(info *gen.NodeShortInfo) {
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
				info.MemoryUsed = value
			case metricHeapObjects:
				info.MemoryAlloc = value
			case metricMemoryLimit:
				info.MemoryLimit = value
			case metricHeapLive:
				info.HeapLive = value
			case metricHeapGoal:
				info.HeapGoal = value
			case metricGoroutines:
				info.Goroutines = int64(value)
			case metricGCCycles:
				info.GCCycles = value
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

	if cpuTotal > 0 {
		info.GCCPUFraction = cpuGC / cpuTotal
	}
}
