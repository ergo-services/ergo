package edf

// Verifies that Encode/Decode roundtrip works correctly when MarshalEDF
// writes enough data to trigger lib.Buffer reallocation. A small initial
// buffer capacity (64 bytes) guarantees that MarshalEDF's Write call
// triggers append-reallocation for any non-trivial payload.

import (
	"bytes"
	"io"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type largePayload struct {
	Data []byte
}

func (lp largePayload) MarshalEDF(w io.Writer) error {
	_, err := w.Write(lp.Data)
	return err
}

func (lp *largePayload) UnmarshalEDF(data []byte) error {
	lp.Data = make([]byte, len(data))
	copy(lp.Data, data)
	return nil
}

func TestMarshalerBufferReallocation(t *testing.T) {
	if err := RegisterTypeOf(largePayload{}); err != nil && err != gen.ErrTaken {
		t.Fatal(err)
	}

	// Small payload that fits without reallocation, and a large payload
	// that forces the buffer to grow.
	sizes := []struct {
		name string
		n    int
	}{
		{"small", 100},
		{"large", lib.DefaultBufferLength * 2},
	}

	for _, tc := range sizes {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, tc.n)
			for i := range data {
				data[i] = byte(i % 251)
			}
			original := largePayload{Data: data}

			// Small capacity forces MarshalEDF to trigger buffer reallocation.
			buf := &lib.Buffer{B: make([]byte, 0, 64)}

			if err := Encode(original, buf, Options{}); err != nil {
				t.Fatalf("Encode: %v", err)
			}

			decoded, _, err := Decode(buf.B, Options{})
			if err != nil {
				t.Fatalf("Decode: %v (encoded %d bytes)", err, buf.Len())
			}

			got, ok := decoded.(largePayload)
			if !ok {
				t.Fatalf("expected largePayload, got %T", decoded)
			}
			if !bytes.Equal(got.Data, original.Data) {
				t.Fatalf("roundtrip mismatch: got %d bytes, want %d bytes",
					len(got.Data), len(original.Data))
			}
		})
	}
}
