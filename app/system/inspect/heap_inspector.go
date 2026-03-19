package inspect

import (
	"fmt"
	"runtime"
	"sort"
	"strings"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_heap() gen.ProcessBehavior {
	return &heap_inspector{}
}

type heap_inspector struct {
	act.Actor
	token gen.Ref

	limit int
	name  string

	generating bool
	loopID     uint64
	event      gen.Atom
}

func (h *heap_inspector) Init(args ...any) error {
	h.limit = args[0].(int)
	h.name = args[1].(string)

	h.Log().SetLogger("default")
	h.SetCompression(true)

	eopts := gen.EventOptions{Notify: true, Buffer: 1}
	hash := filterHash(h.name, "", "", "", 0, h.limit)
	h.event = gen.Atom(fmt.Sprintf("%s_%s", inspectHeap, hash))
	token, err := h.RegisterEvent(h.event, eopts)
	if err != nil {
		h.Log().Error("unable to register event: %s", err)
		return err
	}
	h.token = token
	h.SendAfter(h.PID(), shutdown{}, inspectHeapIdlePeriod)

	return nil
}

func (h *heap_inspector) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if m.id != h.loopID || h.generating == false {
			break
		}

		records, totalAlloc, totalFree := h.captureTop()

		var totalInuse, totalObjects int64
		for _, r := range records {
			totalInuse += r.InuseBytes
			totalObjects += r.InuseObjects
		}

		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)

		ev := MessageInspectHeap{
			Node:          h.Node().Name(),
			Records:       records,
			TotalInuse:    totalInuse,
			TotalObjects:  totalObjects,
			TotalAlloc:    totalAlloc,
			TotalFree:     totalFree,
			GCCPUFraction: ms.GCCPUFraction,
		}

		if err := h.SendEvent(h.event, h.token, ev); err != nil {
			h.Log().Error("unable to send event %q: %s", h.event, err)
			return gen.TerminateReasonNormal
		}

		h.SendAfter(h.PID(), generate{id: h.loopID}, inspectHeapPeriod)

	case requestInspect:
		response := ResponseInspectHeap{
			Event: gen.Event{
				Name: h.event,
				Node: h.Node().Name(),
			},
		}
		h.SendResponse(m.pid, m.ref, response)

	case shutdown:
		if h.generating {
			break
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart:
		h.loopID++
		h.Send(h.PID(), generate{id: h.loopID})
		h.generating = true

	case gen.MessageEventStop:
		if h.generating {
			h.generating = false
			h.SendAfter(h.PID(), shutdown{}, inspectHeapIdlePeriod)
		}
	}

	return nil
}

func (h *heap_inspector) Terminate(reason error) {}

func (h *heap_inspector) captureTop() ([]HeapRecord, int64, int64) {
	var p []runtime.MemProfileRecord
	n, _ := runtime.MemProfile(nil, false) // false = include freed records
	p = make([]runtime.MemProfileRecord, n)
	runtime.MemProfile(p, false)

	nameLower := strings.ToLower(h.name)

	type entry struct {
		inuse   int64
		objects int64
		alloc   int64
		allocN  int64
		freeN   int64
		stack   []string
	}

	var entries []entry
	var totalAlloc, totalFree int64

	for _, r := range p {
		totalAlloc += r.AllocObjects
		totalFree += r.FreeObjects

		inuse := r.InUseBytes()
		if inuse <= 0 {
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

		if nameLower != "" {
			matched := false
			for _, f := range stack {
				if strings.Contains(strings.ToLower(f), nameLower) {
					matched = true
					break
				}
			}
			if matched == false {
				continue
			}
		}

		entries = append(entries, entry{
			inuse:   inuse,
			objects: r.InUseObjects(),
			alloc:   r.AllocBytes,
			allocN:  r.AllocObjects,
			freeN:   r.FreeObjects,
			stack:   stack,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].inuse > entries[j].inuse
	})

	if len(entries) > h.limit {
		entries = entries[:h.limit]
	}

	records := make([]HeapRecord, len(entries))
	for i, e := range entries {
		records[i] = HeapRecord{
			InuseBytes:   e.inuse,
			InuseObjects: e.objects,
			AllocBytes:   e.alloc,
			AllocObjects: e.allocN,
			FreeObjects:  e.freeN,
			Stack:        e.stack,
		}
	}
	return records, totalAlloc, totalFree
}
