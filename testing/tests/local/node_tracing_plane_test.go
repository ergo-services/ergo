package local

import (
	"sync"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type collectingExporter struct {
	mu        sync.Mutex
	spans     []gen.TracingSpan
	terminate int
}

func (e *collectingExporter) HandleSpan(span gen.TracingSpan) {
	e.mu.Lock()
	e.spans = append(e.spans, span)
	e.mu.Unlock()
}

func (e *collectingExporter) Terminate() {
	e.mu.Lock()
	e.terminate++
	e.mu.Unlock()
}

func (e *collectingExporter) terminated() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.terminate
}

func TestNodeTracingExporters(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()

	first := &collectingExporter{}
	second := &collectingExporter{}

	check.ErrorIs(t, nd.TracingExporterAdd("first", nil, gen.TracingFlagSend), gen.ErrIncorrect)
	check.NoError(t, nd.TracingExporterAdd("first", first, gen.TracingFlagSend))
	check.ErrorIs(t, nd.TracingExporterAdd("first", second, gen.TracingFlagSend), gen.ErrTaken)
	check.NoError(t, nd.TracingExporterAdd("second", second, gen.TracingFlagProcs))

	names := nd.TracingExporters()
	check.True(t, contains(names, "first"))
	check.True(t, contains(names, "second"))

	check.Equal(t, gen.TracingFlagSend, nd.TracingExporterFlags("first"))
	check.Equal(t, gen.TracingFlagProcs, nd.TracingExporterFlags("second"))
	check.Equal(t, gen.TracingFlags(0), nd.TracingExporterFlags("missing"))

	nd.TracingExporterDelete("missing")
	nd.TracingExporterDelete("first")
	check.Equal(t, 1, first.terminated())
	check.True(t, contains(nd.TracingExporters(), "first") == false)
	check.True(t, contains(nd.TracingExporters(), "second"))
}

func TestNodeTracingExporterByPID(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()
	pid := n.Spawn(factoryT0, gen.ProcessOptions{})
	other := n.Spawn(factoryT0, gen.ProcessOptions{})
	unknown := gen.PID{Node: n.Name(), ID: 999999}

	check.ErrorIs(t, nd.TracingExporterAddPID(unknown, "bypid", gen.TracingFlagSend), gen.ErrProcessUnknown)
	check.ErrorIs(t, nd.TracingExporterAddPID(pid, "", gen.TracingFlagSend), gen.ErrIncorrect)

	check.NoError(t, nd.TracingExporterAddPID(pid, "bypid", gen.TracingFlagSend))
	check.True(t, contains(nd.TracingExporters(), "bypid"))
	check.Equal(t, gen.TracingFlagSend, nd.TracingExporterFlags("bypid"))

	check.ErrorIs(t, nd.TracingExporterAddPID(pid, "again", gen.TracingFlagSend), gen.ErrNotAllowed)
	check.ErrorIs(t, nd.TracingExporterAddPID(other, "bypid", gen.TracingFlagSend), gen.ErrTaken)

	nd.TracingExporterDeletePID(unknown)
	nd.TracingExporterDeletePID(pid)
	check.True(t, contains(nd.TracingExporters(), "bypid") == false)

	nd.TracingExporterDeletePID(pid)
}

func TestNodeTracingSampler(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()
	pid := n.Spawn(factoryT0, gen.ProcessOptions{})
	unknown := gen.PID{Node: n.Name(), ID: 999999}

	check.Equal(t, "disable", nd.TracingSampler().String())

	check.NoError(t, nd.SetTracingSampler(gen.TracingSamplerAlways))
	check.Equal(t, "always", nd.TracingSampler().String())

	check.NoError(t, nd.SetTracingSampler(gen.TracingSamplerDisable))
	check.Equal(t, "disable", nd.TracingSampler().String())

	check.NoError(t, nd.SetProcessTracingSampler(pid, gen.TracingSamplerAlways))
	check.ErrorIs(t, nd.SetProcessTracingSampler(unknown, gen.TracingSamplerAlways), gen.ErrProcessUnknown)

	nd.Stop()
	nd.Wait()
	check.ErrorIs(t, nd.SetTracingSampler(gen.TracingSamplerAlways), gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.SetProcessTracingSampler(pid, gen.TracingSamplerAlways), gen.ErrNodeTerminated)
	check.ErrorIs(t, nd.TracingExporterAddPID(pid, "late", gen.TracingFlagSend), gen.ErrNodeTerminated)
}

func TestNodeTracingAttributes(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	nd := n.Native()

	nd.SetTracingAttribute("region", "eu")
	nd.SetTracingAttribute("zone", "a")
	nd.SetTracingAttribute("region", "us")
	nd.SetTracingAttribute("ergo.node", "refused")

	info, err := nd.Info()
	check.NoError(t, err)
	attrs := map[string]string{}
	for _, a := range info.Tracing.Attributes {
		attrs[a.Key] = a.Value
	}
	check.Equal(t, "us", attrs["region"])
	check.Equal(t, "a", attrs["zone"])
	check.Equal(t, 2, len(attrs))

	nd.RemoveTracingAttribute("zone")
	nd.RemoveTracingAttribute("missing")

	info, err = nd.Info()
	check.NoError(t, err)
	check.Equal(t, 1, len(info.Tracing.Attributes))
	check.Equal(t, "region", info.Tracing.Attributes[0].Key)
}
