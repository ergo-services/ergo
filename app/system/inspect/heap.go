package inspect

import (
	"runtime"
	"sort"
)

func captureHeapProfile(req RequestGetHeapProfile) ResponseGetHeapProfile {
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

	truncated := 0
	if req.Limit > 0 && len(records) > req.Limit {
		truncated = len(records) - req.Limit
		records = records[:req.Limit]
	}

	return ResponseGetHeapProfile{
		Records:      records,
		TotalInuse:   totalInuse,
		TotalAlloc:   totalAlloc,
		TotalObjects: totalObjects,
		Truncated:    truncated,
	}
}
