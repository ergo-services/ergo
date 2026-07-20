package edf

import (
	"encoding/binary"
	"reflect"
	"testing"

	"ergo.services/ergo/lib"
)

// A crafted type descriptor declaring a huge fixed-size array must be rejected
// at type construction instead of driving reflect.New to a multi-gigabyte
// allocation (an OOM that recover() cannot catch).
func TestDecodeArrayLengthAttack(t *testing.T) {
	fold := []byte{edtArray, 0, 0, 0, 0, edtUint64}
	binary.BigEndian.PutUint32(fold[1:5], 0xFFFFFFFF) // [4294967295]uint64 ~ 34 GB

	packet := append([]byte{edtType, 0, byte(len(fold))}, fold...)

	_, _, err := Decode(packet, Options{})
	if err == nil {
		t.Fatal("expected an error for an oversized array descriptor, got nil")
	}
}

// A map type whose key is non-comparable (here []uint8) must be rejected with
// an error rather than panicking inside reflect.MapOf.
func TestDecodeMapNonComparableKey(t *testing.T) {
	fold := []byte{edtMap, edtSlice, edtUint8, edtUint8} // map[[]uint8]uint8

	packet := append([]byte{edtType, 0, byte(len(fold))}, fold...)

	_, _, err := Decode(packet, Options{})
	if err == nil {
		t.Fatal("expected an error for a non-comparable map key, got nil")
	}
}

// A valid fixed-size array still round-trips.
func TestDecodeArrayRoundTrip(t *testing.T) {
	expect := [3]uint64{10, 20, 30}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	if err := Encode(expect, b, Options{}); err != nil {
		t.Fatal(err)
	}

	value, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(value, expect) == false {
		t.Fatalf("array mismatch: got %#v, want %#v", value, expect)
	}
}

// Nested slices round-trip: exercises the capped-then-grown allocation path
// where the outer element size (a slice header) does not predict the count.
func TestDecodeNestedSliceRoundTrip(t *testing.T) {
	expect := [][]uint64{{1, 2, 3}, {}, {4}, {5, 6}}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	if err := Encode(expect, b, Options{}); err != nil {
		t.Fatal(err)
	}

	value, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(value, expect) == false {
		t.Fatalf("nested slice mismatch: got %#v, want %#v", value, expect)
	}
}

// A slice whose declared element count is inflated far beyond the bytes present
// must fail cleanly (bounded allocation) rather than reserving count*elemSize.
func TestDecodeSliceInflatedCount(t *testing.T) {
	packet := []byte{edtType, 0, 2,
		edtSlice, edtUint64,
		edtSlice,
		0xFF, 0xFF, 0xFF, 0xF0, // ~4.29e9 elements
		0, 0, 0, 0, 0, 0, 0, 1, // only one element's worth of bytes
	}

	_, _, err := Decode(packet, Options{})
	if err == nil {
		t.Fatal("expected an error for an inflated slice length, got nil")
	}
}
