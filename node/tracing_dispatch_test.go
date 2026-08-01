package node

import (
	"sync/atomic"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

// blockingExporter stalls in HandleSpan until released, modelling a slow/blocked object
// exporter (I/O to a dead sink).
type blockingExporter struct {
	release chan struct{}
}

func (b *blockingExporter) HandleSpan(gen.TracingSpan) { <-b.release }
func (b *blockingExporter) Terminate()                 {}

// A slow object tracing exporter must not stall the routing goroutine (sendTracingSpan),
// and spans that overflow its queue are dropped and counted rather than blocking. Guards
// the tracing-decouple change.
func TestTracingExporterDoesNotStallRouting(t *testing.T) {
	n := &node{}
	atomic.StoreInt64(&n.creation, 1) // isRunning()
	n.log = createLog(gen.LogLevelDisabled, nil)

	ex := &blockingExporter{release: make(chan struct{})}
	if err := n.TracingExporterAdd("blk", ex, gen.TracingFlagSend); err != nil {
		t.Fatal(err)
	}

	span := gen.TracingSpan{Kind: gen.TracingKindSend, Point: gen.TracingPointSent}

	// emit well past the queue capacity while the exporter is stuck on the first span
	done := make(chan struct{})
	go func() {
		for i := 0; i < tracingExporterQueue+200; i++ {
			n.sendTracingSpan(span)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		close(ex.release)
		t.Fatal("sendTracingSpan blocked on a stuck object exporter")
	}

	v, _ := n.tracingExporters.Load("blk")
	entry := v.(tracingExporterEntry)
	if entry.dispatcher.Dropped() == 0 {
		t.Fatal("overflow spans were not dropped/counted")
	}

	close(ex.release)              // unblock the worker so teardown can drain
	n.TracingExporterDelete("blk") // Stop + Terminate; must not hang
}
