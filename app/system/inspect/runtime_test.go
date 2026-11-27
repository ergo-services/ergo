package inspect

import (
	"math"
	"strings"
	"testing"

	"ergo.services/ergo/app/system/runtime/golang"
	"ergo.services/ergo/gen"
)

func TestGoRuntimeInspectorContract(t *testing.T) {
	runInspectorContract(t, inspectorCase{
		name:    "golang_runtime",
		factory: factory_runtime,
		event:   gen.Atom(inspectRuntime),
	})
}

func TestGoRuntimeInspectorAnswersWithTheCatalog(t *testing.T) {
	sub := inspectorCase{factory: factory_runtime}.spawn(t)

	client := gen.PID{Node: "inspect@localhost", ID: 100}
	sub.SendMessage(gen.PID{}, requestInspect{pid: client, ref: gen.Ref{}})

	sub.ShouldSendResponse().To(client).Once().Assert()
}

func TestRuntimeCatalogIsTheWholeOfRuntimeMetrics(t *testing.T) {
	catalog := runtimeCatalog()
	if len(catalog) == 0 {
		t.Fatal("the catalog is empty")
	}

	histograms := 0
	for _, m := range catalog {
		if strings.Contains(m.Name, ":") == false {
			t.Errorf("%q carries no unit", m.Name)
		}
		switch m.Kind {
		case "uint64", "float64":
			if len(m.Buckets) != 0 {
				t.Errorf("%s of kind %s has buckets", m.Name, m.Kind)
			}
		case "histogram":
			histograms++
			if len(m.Buckets) == 0 {
				t.Errorf("histogram %s has no buckets", m.Name)
			}
			if math.IsInf(m.Buckets[len(m.Buckets)-1], 1) {
				t.Errorf("histogram %s keeps the upper bound +Inf among its lower bounds", m.Name)
			}
		default:
			t.Errorf("%s has an unknown kind %q", m.Name, m.Kind)
		}
	}
	if histograms == 0 {
		t.Error("runtime/metrics has histograms, the catalog has none")
	}
}

func TestReadRuntimeMetricsByNameAndPrefix(t *testing.T) {
	const allocs = "/gc/heap/allocs:objects"

	one := readRuntimeMetrics(golang.RequestGetRuntimeMetrics{Names: []string{allocs}})
	if one.Time == 0 {
		t.Error("the answer carries no time")
	}
	if len(one.Metrics) != 1 || len(one.Samples) != 1 {
		t.Fatalf("a name selected %d metrics and %d samples", len(one.Metrics), len(one.Samples))
	}
	if one.Samples[0].Name != allocs || one.Samples[0].Kind != "uint64" || one.Samples[0].Uint64 == 0 {
		t.Errorf("unexpected sample of %s: %+v", allocs, one.Samples[0])
	}

	gc := readRuntimeMetrics(golang.RequestGetRuntimeMetrics{Names: []string{"/gc/"}})
	if len(gc.Metrics) < 2 {
		t.Fatalf("the prefix /gc/ selected %d metrics", len(gc.Metrics))
	}
	for _, m := range gc.Metrics {
		if strings.HasPrefix(m.Name, "/gc/") == false {
			t.Errorf("the prefix /gc/ selected %s", m.Name)
		}
	}

	none := readRuntimeMetrics(golang.RequestGetRuntimeMetrics{Names: []string{"/gc"}})
	if len(none.Metrics) != 0 {
		t.Errorf("a name without '/' at the end is a whole name, yet it selected %d metrics", len(none.Metrics))
	}

	all := readRuntimeMetrics(golang.RequestGetRuntimeMetrics{})
	if len(all.Metrics) != len(runtimeCatalog()) || len(all.Samples) != len(all.Metrics) {
		t.Errorf("no names selected %d metrics and %d samples of %d", len(all.Metrics), len(all.Samples),
			len(runtimeCatalog()))
	}
}

func TestHistogramCountsCutTheEmptyBuckets(t *testing.T) {
	first, counts := histogramCounts([]uint64{0, 0, 3, 0, 5, 0})
	if first != 2 || len(counts) != 3 || counts[0] != 3 || counts[2] != 5 {
		t.Errorf("got first %d, counts %v", first, counts)
	}
	if first, counts = histogramCounts([]uint64{0, 0}); first != 0 || counts != nil {
		t.Errorf("an empty histogram: got first %d, counts %v", first, counts)
	}
}
