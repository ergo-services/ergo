package inspect

import (
	"runtime"
	"sort"
)

func captureHeapProfile(req RequestDoHeapProfile) ResponseDoHeapProfile {
	// force up-to-date stats
	runtime.GC()

	var p []runtime.MemProfileRecord
	n, _ := runtime.MemProfile(nil, true)
	p = make([]runtime.MemProfileRecord, n)
	runtime.MemProfile(p, true)

	var records []HeapRecord
	var totalInuse, totalAlloc, totalObjects int64

	for _, r := range p {
		inuse := r.InUseBytes()
		if req.MinBytes > 0 && inuse < req.MinBytes {
			continue
		}

		frames := runtime.CallersFrames(r.Stack())
		var stack []string
		for {
			frame, more := frames.Next()
			if frame.Function != "" {
				stack = append(stack, frame.Function)
			}
			if more == false {
				break
			}
		}

		rec := HeapRecord{
			InuseBytes:   inuse,
			InuseObjects: r.InUseObjects(),
			AllocBytes:   r.AllocBytes,
			AllocObjects: r.AllocObjects,
			Stack:        stack,
		}
		records = append(records, rec)

		totalInuse += inuse
		totalAlloc += r.AllocBytes
		totalObjects += r.InUseObjects()
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].InuseBytes > records[j].InuseBytes
	})

	return ResponseDoHeapProfile{
		Records:      records,
		TotalInuse:   totalInuse,
		TotalAlloc:   totalAlloc,
		TotalObjects: totalObjects,
	}
}
