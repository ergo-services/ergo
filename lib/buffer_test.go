package lib

import (
	"bytes"
	"testing"
)

func TestBufferAppend(t *testing.T) {
	b := TakeBuffer()
	defer ReleaseBuffer(b)

	b.AppendByte('h')
	b.Append([]byte("el"))
	b.AppendString("lo")

	if b.String() != "hello" {
		t.Fatalf("String = %q, want hello", b.String())
	}
	if b.Len() != 5 {
		t.Fatalf("Len = %d, want 5", b.Len())
	}
}

func TestBufferAllocateExtend(t *testing.T) {
	b := TakeBuffer()
	defer ReleaseBuffer(b)

	b.Allocate(10)
	if b.Len() != 10 {
		t.Fatalf("after Allocate(10): Len = %d", b.Len())
	}

	tail := b.Extend(4)
	if len(tail) != 4 {
		t.Fatalf("Extend(4) returned %d bytes", len(tail))
	}
	if b.Len() != 14 {
		t.Fatalf("after Extend(4): Len = %d, want 14", b.Len())
	}

	copy(tail, "abcd")
	if string(b.B[10:14]) != "abcd" {
		t.Fatal("the Extend slice does not write through to the buffer")
	}
}

// Allocate beyond the capacity grows to the exact size and keeps the content.
func TestBufferAllocateGrow(t *testing.T) {
	b := TakeBuffer()
	defer ReleaseBuffer(b)

	b.AppendString("head")
	n := DefaultBufferLength*10 + 7
	b.Allocate(n)
	if b.Len() != n || b.Cap() != n {
		t.Fatalf("after Allocate(%d): Len = %d, Cap = %d", n, b.Len(), b.Cap())
	}
	if string(b.B[:4]) != "head" {
		t.Fatal("Allocate lost the existing content")
	}
}

func TestBufferSetReset(t *testing.T) {
	b := TakeBuffer()
	defer ReleaseBuffer(b)

	b.Set([]byte("data"))
	if b.String() != "data" {
		t.Fatalf("after Set: %q", b.String())
	}

	b.Reset()
	if b.Len() != 0 {
		t.Fatalf("after Reset: Len = %d", b.Len())
	}
}

func TestBufferWriteDataToReadDataFrom(t *testing.T) {
	w := TakeBuffer()
	defer ReleaseBuffer(w)
	w.Append([]byte("payload"))

	var sink bytes.Buffer
	if err := w.WriteDataTo(&sink); err != nil {
		t.Fatalf("WriteDataTo: %s", err)
	}
	if sink.String() != "payload" {
		t.Fatalf("WriteDataTo wrote %q", sink.String())
	}

	r := TakeBuffer()
	defer ReleaseBuffer(r)
	n, err := r.ReadDataFrom(bytes.NewReader([]byte("incoming")), 0)
	if err != nil {
		t.Fatalf("ReadDataFrom: %s", err)
	}
	if string(r.B[:n]) != "incoming" {
		t.Fatalf("ReadDataFrom read %q", string(r.B[:n]))
	}
}

// shortWriter takes at most chunk bytes per call and reports no error for the rest.
type shortWriter struct {
	chunk int
	got   bytes.Buffer
	calls int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	w.calls++
	n := len(p)
	if n > w.chunk {
		n = w.chunk
	}
	w.got.Write(p[:n])
	return n, nil
}

// A writer that takes the data in pieces gets every piece once, in order.
func TestBufferWriteDataToShortWrites(t *testing.T) {
	b := TakeBuffer()
	defer ReleaseBuffer(b)
	b.AppendString("abcdefghij")

	w := &shortWriter{chunk: 3}
	if err := b.WriteDataTo(w); err != nil {
		t.Fatalf("WriteDataTo: %s", err)
	}
	if w.got.String() != "abcdefghij" {
		t.Fatalf("the writer got %q, expected abcdefghij", w.got.String())
	}
	if w.calls != 4 {
		t.Fatalf("WriteDataTo made %d call(s), expected 4", w.calls)
	}
}
