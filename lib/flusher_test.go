package lib

import (
	"bytes"
	"testing"
)

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
