package inspect

import (
	"runtime/metrics"
	"strings"
	"sync"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/app/system/runtime/golang"
	"ergo.services/ergo/gen"
)

// runtimeCatalog is the catalog of runtime/metrics. The set of metrics and the
// buckets of the histograms do not change for the life of the process.
var runtimeCatalog = sync.OnceValue(func() []golang.RuntimeMetric {
	all := metrics.All()
	samples := make([]metrics.Sample, len(all))
	for i := range all {
		samples[i].Name = all[i].Name
	}
	metrics.Read(samples)

	catalog := make([]golang.RuntimeMetric, 0, len(all))
	for i, d := range all {
		m := golang.RuntimeMetric{
			Name:        d.Name,
			Description: d.Description,
			Kind:        runtimeKind(d.Kind),
			Cumulative:  d.Cumulative,
		}
		if samples[i].Value.Kind() == metrics.KindFloat64Histogram {
			// N+1 boundaries of N buckets: the lower bounds stay, the last +Inf goes
			b := samples[i].Value.Float64Histogram().Buckets
			if len(b) > 0 {
				m.Buckets = append([]float64(nil), b[:len(b)-1]...)
			}
		}
		catalog = append(catalog, m)
	}
	return catalog
})

func runtimeKind(kind metrics.ValueKind) string {
	switch kind {
	case metrics.KindUint64:
		return "uint64"
	case metrics.KindFloat64:
		return "float64"
	case metrics.KindFloat64Histogram:
		return "histogram"
	}
	return ""
}

func readRuntimeSamples(catalog []golang.RuntimeMetric) []golang.RuntimeSample {
	samples := make([]metrics.Sample, len(catalog))
	for i := range catalog {
		samples[i].Name = catalog[i].Name
	}
	metrics.Read(samples)

	out := make([]golang.RuntimeSample, len(samples))
	for i, s := range samples {
		out[i].Name = s.Name
		out[i].Kind = runtimeKind(s.Value.Kind())
		switch s.Value.Kind() {
		case metrics.KindUint64:
			out[i].Uint64 = s.Value.Uint64()
		case metrics.KindFloat64:
			out[i].Float64 = s.Value.Float64()
		case metrics.KindFloat64Histogram:
			out[i].First, out[i].Counts = histogramCounts(s.Value.Float64Histogram().Counts)
		}
	}
	return out
}

// histogramCounts cuts the empty buckets around: the first non-empty bucket and
// the counts from it to the last non-empty one.
func histogramCounts(counts []uint64) (int, []uint64) {
	first, last := 0, len(counts)
	for first < last && counts[first] == 0 {
		first++
	}
	for last > first && counts[last-1] == 0 {
		last--
	}
	if first == last {
		return 0, nil
	}
	return first, append([]uint64(nil), counts[first:last]...)
}

// selectRuntimeMetrics picks the metrics by their names or by the prefixes of
// their names (ending with '/'); all of them if none.
func selectRuntimeMetrics(catalog []golang.RuntimeMetric, names []string) []golang.RuntimeMetric {
	if len(names) == 0 {
		return catalog
	}
	var selected []golang.RuntimeMetric
	for _, m := range catalog {
		for _, n := range names {
			if m.Name == n || (strings.HasSuffix(n, "/") && strings.HasPrefix(m.Name, n)) {
				selected = append(selected, m)
				break
			}
		}
	}
	return selected
}

func readRuntimeMetrics(req golang.RequestGetRuntimeMetrics) golang.ResponseGetRuntimeMetrics {
	selected := selectRuntimeMetrics(runtimeCatalog(), req.Names)
	return golang.ResponseGetRuntimeMetrics{
		Time:    time.Now().UnixNano(),
		Metrics: selected,
		Samples: readRuntimeSamples(selected),
	}
}

func factory_runtime() gen.ProcessBehavior {
	return &goRuntime{}
}

// goRuntime sends the values of the metrics of the Go runtime every second while
// the event has subscribers.
type goRuntime struct {
	act.Actor
	token gen.Ref

	generating bool
	loopID     uint64
}

func (ir *goRuntime) Init(args ...any) error {
	ir.Log().SetLogger("default")
	ir.SetProcessKind(gen.ProcessKindMonitor)
	ir.Log().Debug("go runtime inspector started")

	eopts := gen.EventOptions{
		Notify: true,
		Buffer: 1, // keep the last event
	}
	token, err := ir.RegisterEvent(inspectRuntime, eopts)
	if err != nil {
		ir.Log().Error("unable to register event: %s", err)
		return err
	}
	ir.token = token
	ir.SendAfter(ir.PID(), shutdown{}, inspectRuntimeIdlePeriod)
	return nil
}

func (ir *goRuntime) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if m.id != ir.loopID || ir.generating == false {
			break // cancelled
		}
		ev := golang.MessageInspectRuntime{
			Node:    ir.Node().Name(),
			Time:    time.Now().UnixNano(),
			Samples: readRuntimeSamples(runtimeCatalog()),
		}
		if err := ir.SendEvent(inspectRuntime, ir.token, ev); err != nil {
			ir.Log().Error("unable to send event %q: %s", inspectRuntime, err)
			return gen.TerminateReasonNormal
		}
		ir.SendAfter(ir.PID(), generate{id: ir.loopID}, inspectRuntimePeriod)

	case requestInspect:
		response := golang.ResponseInspectRuntime{
			Event: gen.Event{
				Name: inspectRuntime,
				Node: ir.Node().Name(),
			},
			Metrics: runtimeCatalog(),
		}
		ir.SendResponse(m.pid, m.ref, response)

	case shutdown:
		if ir.generating {
			break
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart:
		ir.loopID++
		ir.Send(ir.PID(), generate{id: ir.loopID})
		ir.generating = true

	case gen.MessageEventStop:
		if ir.generating {
			ir.generating = false
			ir.SendAfter(ir.PID(), shutdown{}, inspectRuntimeIdlePeriod)
		}

	default:
		ir.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (ir *goRuntime) Terminate(reason error) {
	ir.Log().Debug("go runtime inspector terminated: %s", reason)
}
