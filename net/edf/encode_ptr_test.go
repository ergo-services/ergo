package edf

import (
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo/lib"
)

// Basic pointer tests

func TestEncodePtrInt(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := 42
	ptr := &v

	// type descriptor: [edtType][len=2][edtPtr][edtInt]
	// value: [edtPtr][8 bytes for int64]
	expect := []byte{
		edtType, 0, 2, // type header
		edtPtr, edtInt, // type descriptor
		edtPtr,                          // non-nil marker
		0, 0, 0, 0, 0, 0, 0, 42, // int value (big endian)
	}

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(b.B, expect) == false {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodePtrIntNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *int = nil

	// type descriptor: [edtType][len=2][edtPtr][edtInt]
	// value: [edtNil]
	expect := []byte{
		edtType, 0, 2, // type header
		edtPtr, edtInt, // type descriptor
		edtNil, // nil marker
	}

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(b.B, expect) == false {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodePtrString(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	s := "hello"
	ptr := &s

	// type descriptor: [edtType][len=2][edtPtr][edtString]
	// value: [edtPtr][len][string bytes]
	expect := []byte{
		edtType, 0, 2, // type header
		edtPtr, edtString, // type descriptor
		edtPtr,       // non-nil marker
		0, 5,         // string len
		'h', 'e', 'l', 'l', 'o',
	}

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(b.B, expect) == false {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodePtrStringNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *string = nil

	expect := []byte{
		edtType, 0, 2,
		edtPtr, edtString,
		edtNil,
	}

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(b.B, expect) == false {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodePtrFloat64(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	f := 3.14
	ptr := &f

	expect := []byte{
		edtType, 0, 2,
		edtPtr, edtFloat64,
		edtPtr,
		0x40, 0x09, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // 3.14 in IEEE 754
	}

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(b.B, expect) == false {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodePtrFloat64Nil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *float64 = nil

	expect := []byte{
		edtType, 0, 2,
		edtPtr, edtFloat64,
		edtNil,
	}

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(b.B, expect) == false {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodePtrBool(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := true
	ptr := &v

	expect := []byte{
		edtType, 0, 2,
		edtPtr, edtBool,
		edtPtr,
		1, // true
	}

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(b.B, expect) == false {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodePtrBoolNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *bool = nil

	expect := []byte{
		edtType, 0, 2,
		edtPtr, edtBool,
		edtNil,
	}

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(b.B, expect) == false {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

// Nested pointer (error case)

func TestEncodePtrPtrInt(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := 42
	ptr := &v
	ptrptr := &ptr

	err := Encode(ptrptr, b, Options{})
	if err == nil {
		t.Fatal("expected error for nested pointer")
	}
}

// Slice of pointers

func TestEncodeSlicePtr(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	a, b2, c := 1, 2, 3
	slice := []*int{&a, nil, &b2, nil, &c}

	// type: [edtType][len=3][edtSlice][edtPtr][edtInt]
	// value: [edtSlice][count=5][ptr][1][nil][ptr][2][nil][ptr][3]
	expect := []byte{
		edtType, 0, 3,
		edtSlice, edtPtr, edtInt,
		edtSlice,
		0, 0, 0, 5, // count
		edtPtr, 0, 0, 0, 0, 0, 0, 0, 1, // &a
		edtNil,                         // nil
		edtPtr, 0, 0, 0, 0, 0, 0, 0, 2, // &b2
		edtNil,                         // nil
		edtPtr, 0, 0, 0, 0, 0, 0, 0, 3, // &c
	}

	if err := Encode(slice, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(b.B, expect) == false {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSlicePtrEmpty(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	slice := []*int{}

	expect := []byte{
		edtType, 0, 3,
		edtSlice, edtPtr, edtInt,
		edtSlice,
		0, 0, 0, 0, // count = 0
	}

	if err := Encode(slice, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(b.B, expect) == false {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSlicePtrNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var slice []*int = nil

	expect := []byte{
		edtType, 0, 3,
		edtSlice, edtPtr, edtInt,
		edtNil, // nil slice
	}

	if err := Encode(slice, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(b.B, expect) == false {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

// Map with pointer values

func TestEncodeMapPtrValue(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := 42
	m := map[string]*int{"key": &v}

	if err := Encode(m, b, Options{}); err != nil {
		t.Fatal(err)
	}

	// verify by decoding
	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	dm, ok := decoded.(map[string]*int)
	if ok == false {
		t.Fatalf("expected map[string]*int, got %T", decoded)
	}
	if dm["key"] == nil || *dm["key"] != 42 {
		t.Fatal("incorrect decoded value")
	}
}

func TestEncodeMapPtrValueNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := 42
	m := map[string]*int{"a": &v, "b": nil}

	if err := Encode(m, b, Options{}); err != nil {
		t.Fatal(err)
	}

	// verify by decoding
	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	dm, ok := decoded.(map[string]*int)
	if ok == false {
		t.Fatalf("expected map[string]*int, got %T", decoded)
	}
	if dm["a"] == nil || *dm["a"] != 42 {
		t.Fatal("incorrect decoded value for key 'a'")
	}
	if dm["b"] != nil {
		t.Fatal("expected nil for key 'b'")
	}
}

// Max depth protection test

// createNestedPointer creates N levels of pointer nesting without cycles
// e.g., depth=3 creates: *any -> *any -> *any -> int(42)
func createNestedPointer(depth int) any {
	if depth == 0 {
		return 42
	}
	inner := createNestedPointer(depth - 1)
	return &inner
}

func TestEncodePtrMaxDepthError(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	// Build deeply nested pointer chain: 101 levels of *any
	// This exceeds maxEncodeDepth (100)
	val := createNestedPointer(101)

	err := Encode(val, b, Options{})
	if err == nil {
		t.Fatal("expected error for max depth exceeded")
	}
	if err != ErrMaxDepthExceeded {
		t.Fatalf("expected ErrMaxDepthExceeded, got: %v", err)
	}
}

func TestEncodePtrCustomMaxDepth(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	// Build 10 levels of nesting (no cycles)
	val := createNestedPointer(10)

	// Should fail with MaxDepth=5
	err := Encode(val, b, Options{MaxDepth: 5})
	if err != ErrMaxDepthExceeded {
		t.Fatalf("expected ErrMaxDepthExceeded with MaxDepth=5, got: %v", err)
	}

	// Should succeed with MaxDepth=15
	b.Reset()
	err = Encode(val, b, Options{MaxDepth: 15})
	if err != nil {
		t.Fatalf("unexpected error with MaxDepth=15: %v", err)
	}
}

func TestEncodePtrCyclicReference(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	// Create cyclic reference: val points to itself
	var val any = 42
	val = &val // val now contains *any pointing to val (cycle!)

	err := Encode(val, b, Options{})
	if err != ErrMaxDepthExceeded {
		t.Fatalf("expected ErrMaxDepthExceeded for cyclic reference, got: %v", err)
	}
}
