package inspect

import (
	"fmt"
	"strings"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_tracing() gen.ProcessBehavior {
	return &tracing{}
}

type tracing struct {
	act.Actor
	token gen.Ref
	event gen.Atom

	flags          gen.TracingFlags
	limit          int
	kinds          uint32
	points         uint32
	messagePattern string
	messageExclude bool
	generating     bool
	loopID         uint64

	// ring buffer
	ring     []gen.TracingSpan
	pos      int
	full     bool
	received int64
}

type flushTracing struct{ id uint64 }

const tracingFlushInterval = time.Second
const inspectTracingIdlePeriod = 10 * time.Second

func (it *tracing) Init(args ...any) error {
	it.flags = args[0].(gen.TracingFlags)
	it.limit = args[1].(int)
	if it.limit < 1 {
		it.limit = 500
	}
	it.kinds = args[2].(uint32)
	it.points = args[3].(uint32)
	it.messagePattern = args[4].(string)
	it.messageExclude = args[5].(bool)
	it.ring = make([]gen.TracingSpan, it.limit)
	it.Log().Debug("tracing inspector started (limit: %d)", it.limit)
	it.SetCompression(true)
	it.SetProcessKind(gen.ProcessKindMonitor)

	eopts := gen.EventOptions{
		Notify: true,
	}
	evname := gen.Atom(fmt.Sprintf("%s_%s", string(it.Name()), it.PID()))
	token, err := it.RegisterEvent(evname, eopts)
	if err != nil {
		return err
	}

	it.event = evname
	it.token = token
	it.SendAfter(it.PID(), shutdown{}, inspectTracingIdlePeriod)

	return nil
}

func (it *tracing) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case flushTracing:
		if m.id != it.loopID || it.generating == false {
			break
		}
		if it.received == 0 {
			it.SendAfter(it.PID(), flushTracing{id: it.loopID}, tracingFlushInterval)
			break
		}

		var spans []gen.TracingSpan
		if it.full {
			spans = make([]gen.TracingSpan, it.limit)
			copy(spans, it.ring[it.pos:])
			copy(spans[it.limit-it.pos:], it.ring[:it.pos])
		} else {
			spans = make([]gen.TracingSpan, it.pos)
			copy(spans, it.ring[:it.pos])
		}

		suppressed := it.received - int64(len(spans))
		if suppressed < 0 {
			suppressed = 0
		}

		ev := MessageInspectTracing{
			Node:       it.Node().Name(),
			Spans:      spans,
			Suppressed: suppressed,
		}

		it.pos = 0
		it.full = false
		it.received = 0

		if err := it.SendEvent(it.event, it.token, ev); err != nil {
			return gen.TerminateReasonNormal
		}

		it.SendAfter(it.PID(), flushTracing{id: it.loopID}, tracingFlushInterval)

	case requestInspect:
		response := ResponseInspectTracing{
			Event: gen.Event{
				Name: it.event,
				Node: it.Node().Name(),
			},
		}
		it.SendResponse(m.pid, m.ref, response)

	case shutdown:
		if it.generating {
			break
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart:
		it.Log().Debug("registering as tracing exporter")
		it.Node().TracingExporterAddPID(it.PID(), it.PID().String(), it.flags)
		it.loopID++
		it.generating = true
		it.SendAfter(it.PID(), flushTracing{id: it.loopID}, tracingFlushInterval)

	case gen.MessageEventStop:
		it.Node().TracingExporterDeletePID(it.PID())
		it.Log().Debug("removed as tracing exporter")
		it.generating = false
		it.pos = 0
		it.full = false
		it.received = 0
		it.SendAfter(it.PID(), shutdown{}, inspectTracingIdlePeriod)
	}

	return nil
}

func (it *tracing) HandleSpan(span gen.TracingSpan) error {
	// kind filter: bitmask 1=send, 2=request, 4=response, 8=spawn, 16=terminate
	// business spans (Point=Span) have no message kind - filter them by point only
	if span.Point != gen.TracingPointSpan && it.kinds != 0 && it.kinds != 31 {
		kindBit := uint32(1) << (uint32(span.Kind) - 1)
		if it.kinds&kindBit == 0 {
			return nil
		}
	}

	// point filter: bitmask 1=sent, 2=delivered, 4=processed, 8=span
	if it.points != 0 && it.points != 15 {
		pointBit := uint32(1) << (uint32(span.Point) - 1)
		if it.points&pointBit == 0 {
			return nil
		}
	}

	if it.messagePattern != "" {
		match := strings.Contains(span.Message, it.messagePattern) ||
			strings.Contains(span.Error, it.messagePattern)
		if it.messageExclude == true && match == true {
			return nil
		}
		if it.messageExclude == false && match == false {
			return nil
		}
	}

	it.ring[it.pos] = span
	it.pos++
	if it.pos >= it.limit {
		it.pos = 0
		it.full = true
	}
	it.received++
	return nil
}

func (it *tracing) Terminate(reason error) {
	it.Node().TracingExporterDeletePID(it.PID())
}
