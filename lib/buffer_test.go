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
