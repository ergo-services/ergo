package lib

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

// countingWriter records how many times it was written to, safe for the flusher's
// timer goroutine.
type countingWriter struct {
	mu sync.Mutex
	n  int
}

func (w *countingWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	w.n++
	w.mu.Unlock()
	return len(b), nil
}

func (w *countingWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.n
}

// Stop must flush buffered data rather than drop it: a connection that closes
// right after a write must still deliver that write.
func TestFlusherStopFlushesPending(t *testing.T) {
	var buf bytes.Buffer
	f := NewFlusher(&buf)
	if _, err := f.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	f.Stop()
	if buf.String() != "data" {
		t.Fatalf("Stop must flush pending data, got %q", buf.String())
	}
}

// After Stop() the keepalive timer must never fire again: a callback that had
// already fired (or a re-armed timer) must not write a keepalive into the closed
// conn. Regression for the stopped-flag guard - here a stray timer firing after
// Stop is simulated directly.
func TestFlusherKeepAliveNoWriteAfterStop(t *testing.T) {
	w := &countingWriter{}
	f := NewFlusherWithKeepAlive(w, []byte("KA"), 5*time.Millisecond).(*flusher)

	time.Sleep(20 * time.Millisecond) // let a few keepalives fire
	f.Stop()
	time.Sleep(5 * time.Millisecond) // let any in-flight callback settle
	before := w.count()

	// simulate a stray/re-armed keepalive timer firing after Stop
	f.Lock()
	f.timer.Reset(time.Nanosecond)
	f.Unlock()
	time.Sleep(20 * time.Millisecond)

	if after := w.count(); after != before {
		t.Fatalf("flusher wrote to the conn after Stop: %d writes before, %d after", before, after)
	}
}
