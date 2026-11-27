package inspect

import (
	"math"
	"runtime"
	"sort"

	"ergo.services/ergo/app/system/runtime/golang"
)

const heapRecordsLimit = 1000

type heapSample struct {
	record runtime.MemProfileRecord
	golang.HeapRecord
}

func captureHeapProfile(req golang.RequestGetHeapProfile) golang.ResponseGetHeapProfile {
	// force up-to-date stats
	runtime.GC()

	var p []runtime.MemProfileRecord
	n, _ := runtime.MemProfile(nil, true)
	for {
		p = make([]runtime.MemProfileRecord, n+64)
		var ok bool
		n, ok = runtime.MemProfile(p, true)
		if ok {
			p = p[:n]
			break
		}
	}

	var response golang.ResponseGetHeapProfile
	rate := int64(runtime.MemProfileRate)
	samples := make([]heapSample, 0, len(p))

	for _, r := range p {
		var s heapSample
		s.record = r
		s.InuseObjects, s.InuseBytes = scaleHeapSample(r.InUseObjects(), r.InUseBytes(), rate)
		s.AllocObjects, s.AllocBytes = scaleHeapSample(r.AllocObjects, r.AllocBytes, rate)
		s.FreeObjects, _ = scaleHeapSample(r.FreeObjects, r.FreeBytes, rate)

		response.TotalInuse += s.InuseBytes
		response.TotalAlloc += s.AllocBytes
		response.TotalObjects += s.InuseObjects

		if req.MinBytes > 0 && s.InuseBytes < req.MinBytes {
			continue
		}
		samples = append(samples, s)
	}

	sort.Slice(samples, func(i, j int) bool {
		return samples[i].InuseBytes > samples[j].InuseBytes
	})

	limit := req.Limit
	if limit <= 0 {
		limit = heapRecordsLimit
	}
	if len(samples) > limit {
		response.Truncated = len(samples) - limit
		samples = samples[:limit]
	}

	response.Records = make([]golang.HeapRecord, 0, len(samples))
	for _, s := range samples {
		frames := runtime.CallersFrames(s.record.Stack())
		for {
			frame, more := frames.Next()
			if frame.Function != "" {
				s.Stack = append(s.Stack, frame.Function)
			}
			if more == false {
				break
			}
		}
		response.Records = append(response.Records, s.HeapRecord)
	}

	return response
}

// scaleHeapSample turns the sampled values of a record into an estimate of the
// whole: a record of objects of the average size is sampled with the chance
// 1 - exp(-size/rate). The same as scaleHeapSample of runtime/pprof.
func scaleHeapSample(count, size, rate int64) (int64, int64) {
	if count == 0 || size == 0 {
		return 0, 0
	}
	if rate <= 1 {
		return count, size
	}
	avgSize := float64(size) / float64(count)
	scale := 1 / (1 - math.Exp(-avgSize/float64(rate)))
	return int64(float64(count) * scale), int64(float64(size) * scale)
}
