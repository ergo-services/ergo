package edf

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// Basic pointer decode tests

func TestDecodePtrInt(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := 42
	ptr := &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*int)
	if ok == false {
		t.Fatalf("expected *int, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *decodedPtr != 42 {
		t.Fatalf("expected 42, got %d", *decodedPtr)
	}
}

func TestDecodePtrIntNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *int = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*int)
	if ok == false {
		t.Fatalf("expected *int, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatalf("expected nil, got %v", decodedPtr)
	}
}

func TestDecodePtrString(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	s := "hello"
	ptr := &s

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*string)
	if ok == false {
		t.Fatalf("expected *string, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *decodedPtr != "hello" {
		t.Fatalf("expected 'hello', got '%s'", *decodedPtr)
	}
}

func TestDecodePtrStringNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *string = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*string)
	if ok == false {
		t.Fatalf("expected *string, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatalf("expected nil, got %v", decodedPtr)
	}
}

func TestDecodePtrFloat64(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	f := 3.14
	ptr := &f

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*float64)
	if ok == false {
		t.Fatalf("expected *float64, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *decodedPtr != 3.14 {
		t.Fatalf("expected 3.14, got %f", *decodedPtr)
	}
}

func TestDecodePtrFloat64Nil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *float64 = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*float64)
	if ok == false {
		t.Fatalf("expected *float64, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatalf("expected nil, got %v", decodedPtr)
	}
}

func TestDecodePtrBool(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := true
	ptr := &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*bool)
	if ok == false {
		t.Fatalf("expected *bool, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *decodedPtr != true {
		t.Fatalf("expected true, got %v", *decodedPtr)
	}
}

func TestDecodePtrBoolNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *bool = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*bool)
	if ok == false {
		t.Fatalf("expected *bool, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatalf("expected nil, got %v", decodedPtr)
	}
}

// Slice of pointers decode tests

func TestDecodeSlicePtr(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	a, b2, c := 1, 2, 3
	slice := []*int{&a, nil, &b2, nil, &c}

	if err := Encode(slice, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedSlice, ok := decoded.([]*int)
	if ok == false {
		t.Fatalf("expected []*int, got %T", decoded)
	}
	if len(decodedSlice) != 5 {
		t.Fatalf("expected len 5, got %d", len(decodedSlice))
	}

	// check values
	if decodedSlice[0] == nil || *decodedSlice[0] != 1 {
		t.Fatal("incorrect value at index 0")
	}
	if decodedSlice[1] != nil {
		t.Fatal("expected nil at index 1")
	}
	if decodedSlice[2] == nil || *decodedSlice[2] != 2 {
		t.Fatal("incorrect value at index 2")
	}
	if decodedSlice[3] != nil {
		t.Fatal("expected nil at index 3")
	}
	if decodedSlice[4] == nil || *decodedSlice[4] != 3 {
		t.Fatal("incorrect value at index 4")
	}
}

func TestDecodeSlicePtrEmpty(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	slice := []*int{}

	if err := Encode(slice, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedSlice, ok := decoded.([]*int)
	if ok == false {
		t.Fatalf("expected []*int, got %T", decoded)
	}
	if len(decodedSlice) != 0 {
		t.Fatalf("expected len 0, got %d", len(decodedSlice))
	}
}

func TestDecodeSlicePtrNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var slice []*int = nil

	if err := Encode(slice, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	// nil slice decodes to nil interface, not typed nil
	if decoded != nil {
		// if it decoded to a typed value, check it
		decodedSlice, ok := decoded.([]*int)
		if ok == false {
			t.Fatalf("expected []*int or nil, got %T", decoded)
		}
		if decodedSlice != nil {
			t.Fatal("expected nil slice")
		}
	}
}

// Pointer to slice

func TestDecodePtrSlice(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	slice := []int{1, 2, 3}
	ptr := &slice

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*[]int)
	if ok == false {
		t.Fatalf("expected *[]int, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if reflect.DeepEqual(*decodedPtr, []int{1, 2, 3}) == false {
		t.Fatalf("expected [1,2,3], got %v", *decodedPtr)
	}
}

func TestDecodePtrSliceNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *[]int = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*[]int)
	if ok == false {
		t.Fatalf("expected *[]int, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatalf("expected nil, got %v", decodedPtr)
	}
}

// Pointer to map

func TestDecodePtrMap(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	m := map[string]int{"a": 1, "b": 2}
	ptr := &m

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*map[string]int)
	if ok == false {
		t.Fatalf("expected *map[string]int, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if (*decodedPtr)["a"] != 1 || (*decodedPtr)["b"] != 2 {
		t.Fatalf("incorrect map values: %v", *decodedPtr)
	}
}

func TestDecodePtrMapNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *map[string]int = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*map[string]int)
	if ok == false {
		t.Fatalf("expected *map[string]int, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatalf("expected nil, got %v", decodedPtr)
	}
}

// Map with pointer values

func TestDecodeMapPtrValue(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v1, v2 := 10, 20
	m := map[string]*int{"a": &v1, "b": nil, "c": &v2}

	if err := Encode(m, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	dm, ok := decoded.(map[string]*int)
	if ok == false {
		t.Fatalf("expected map[string]*int, got %T", decoded)
	}

	if dm["a"] == nil || *dm["a"] != 10 {
		t.Fatal("incorrect value for key 'a'")
	}
	if dm["b"] != nil {
		t.Fatal("expected nil for key 'b'")
	}
	if dm["c"] == nil || *dm["c"] != 20 {
		t.Fatal("incorrect value for key 'c'")
	}
}

// Pointer in any field (struct)

type testPtrStructWithAny struct {
	Value any
}

func init() {
	RegisterTypeOf(testPtrStructWithAny{})
}

func TestDecodePtrInAny(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := 123
	s := testPtrStructWithAny{Value: &v}

	if err := Encode(s, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	ds, ok := decoded.(testPtrStructWithAny)
	if ok == false {
		t.Fatalf("expected testPtrStructWithAny, got %T", decoded)
	}

	ptr, ok := ds.Value.(*int)
	if ok == false {
		t.Fatalf("expected *int in any, got %T", ds.Value)
	}
	if ptr == nil || *ptr != 123 {
		t.Fatalf("expected 123, got %v", ptr)
	}
}

func TestDecodePtrInAnyNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	s := testPtrStructWithAny{Value: (*int)(nil)}

	if err := Encode(s, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	ds, ok := decoded.(testPtrStructWithAny)
	if ok == false {
		t.Fatalf("expected testPtrStructWithAny, got %T", decoded)
	}

	// nil pointer in any becomes typed nil (*int)(nil)
	ptr, ok := ds.Value.(*int)
	if ok == false {
		// might also be untyped nil
		if ds.Value != nil {
			t.Fatalf("expected *int or nil in any, got %T", ds.Value)
		}
	} else if ptr != nil {
		t.Fatal("expected nil pointer")
	}
}

func TestDecodeSlicePtrInAny(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	a, c := 1, 3
	slice := []*int{&a, nil, &c}
	s := testPtrStructWithAny{Value: slice}

	if err := Encode(s, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	ds, ok := decoded.(testPtrStructWithAny)
	if ok == false {
		t.Fatalf("expected testPtrStructWithAny, got %T", decoded)
	}

	decodedSlice, ok := ds.Value.([]*int)
	if ok == false {
		t.Fatalf("expected []*int in any, got %T", ds.Value)
	}

	if len(decodedSlice) != 3 {
		t.Fatalf("expected len 3, got %d", len(decodedSlice))
	}
	if decodedSlice[0] == nil || *decodedSlice[0] != 1 {
		t.Fatal("incorrect value at index 0")
	}
	if decodedSlice[1] != nil {
		t.Fatal("expected nil at index 1")
	}
	if decodedSlice[2] == nil || *decodedSlice[2] != 3 {
		t.Fatal("incorrect value at index 2")
	}
}

// Pointer to nil slice (not nil pointer, pointer to nil slice value)

func TestDecodePtrToNilSlice(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var slice []int = nil
	ptr := &slice // pointer to nil slice

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*[]int)
	if ok == false {
		t.Fatalf("expected *[]int, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *decodedPtr != nil {
		t.Fatalf("expected nil slice, got %v", *decodedPtr)
	}
}

// Pointer to nil map

func TestDecodePtrToNilMap(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var m map[string]int = nil
	ptr := &m // pointer to nil map

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*map[string]int)
	if ok == false {
		t.Fatalf("expected *map[string]int, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *decodedPtr != nil {
		t.Fatalf("expected nil map, got %v", *decodedPtr)
	}
}

// Pointer to empty slice

func TestDecodePtrToEmptySlice(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	slice := []int{}
	ptr := &slice

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*[]int)
	if ok == false {
		t.Fatalf("expected *[]int, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *decodedPtr == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(*decodedPtr) != 0 {
		t.Fatalf("expected empty slice, got len %d", len(*decodedPtr))
	}
}

// Corner case: pointer to pointer to slice element (nested through slice)

func TestDecodeSlicePtrString(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	s1, s2 := "hello", "world"
	slice := []*string{&s1, nil, &s2}

	if err := Encode(slice, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedSlice, ok := decoded.([]*string)
	if ok == false {
		t.Fatalf("expected []*string, got %T", decoded)
	}

	if len(decodedSlice) != 3 {
		t.Fatalf("expected len 3, got %d", len(decodedSlice))
	}
	if decodedSlice[0] == nil || *decodedSlice[0] != "hello" {
		t.Fatal("incorrect value at index 0")
	}
	if decodedSlice[1] != nil {
		t.Fatal("expected nil at index 1")
	}
	if decodedSlice[2] == nil || *decodedSlice[2] != "world" {
		t.Fatal("incorrect value at index 2")
	}
}

// Test malformed data

func TestDecodePtrMalformedMissingValue(t *testing.T) {
	// type descriptor for *int but no value bytes
	packet := []byte{
		edtType, 0, 2,
		edtPtr, edtInt,
		// missing edtPtr or edtNil marker
	}

	_, _, err := Decode(packet, Options{})
	if err == nil {
		t.Fatal("expected error for malformed data")
	}
}

func TestDecodePtrMalformedInvalidMarker(t *testing.T) {
	// type descriptor for *int with invalid marker
	packet := []byte{
		edtType, 0, 2,
		edtPtr, edtInt,
		edtString, // wrong marker
	}

	_, _, err := Decode(packet, Options{})
	if err == nil {
		t.Fatal("expected error for invalid marker")
	}
}

// Large value through pointer

func TestDecodePtrLargeInt(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := int64(9223372036854775807) // max int64
	ptr := &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*int64)
	if ok == false {
		t.Fatalf("expected *int64, got %T", decoded)
	}
	if *decodedPtr != 9223372036854775807 {
		t.Fatalf("expected max int64, got %d", *decodedPtr)
	}
}

// Slice of slices containing pointers

func TestDecodeNestedSlicePtr(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	a, b2 := 1, 2
	c, d := 3, 4
	nested := [][]*int{
		{&a, nil, &b2},
		{&c, &d},
	}

	if err := Encode(nested, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	dn, ok := decoded.([][]*int)
	if ok == false {
		t.Fatalf("expected [][]*int, got %T", decoded)
	}

	if len(dn) != 2 {
		t.Fatalf("expected 2 outer slices, got %d", len(dn))
	}

	// first inner slice
	if len(dn[0]) != 3 {
		t.Fatalf("expected 3 elements in first slice, got %d", len(dn[0]))
	}
	if *dn[0][0] != 1 || dn[0][1] != nil || *dn[0][2] != 2 {
		fmt.Printf("got: %v %v %v\n", dn[0][0], dn[0][1], dn[0][2])
		t.Fatal("incorrect values in first slice")
	}

	// second inner slice
	if len(dn[1]) != 2 {
		t.Fatalf("expected 2 elements in second slice, got %d", len(dn[1]))
	}
	if *dn[1][0] != 3 || *dn[1][1] != 4 {
		t.Fatal("incorrect values in second slice")
	}
}

// Pointer to binary ([]byte)

func TestDecodePtrBinary(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	bin := []byte{1, 2, 3, 4, 5}
	ptr := &bin

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*[]byte)
	if ok == false {
		t.Fatalf("expected *[]byte, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if reflect.DeepEqual(*decodedPtr, []byte{1, 2, 3, 4, 5}) == false {
		t.Fatalf("expected [1,2,3,4,5], got %v", *decodedPtr)
	}
}

func TestDecodePtrBinaryNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *[]byte = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*[]byte)
	if ok == false {
		t.Fatalf("expected *[]byte, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatal("expected nil pointer")
	}
}

// Pointer to Ergo types

func TestDecodePtrAtom(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	atom := gen.Atom("test_atom")
	ptr := &atom

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*gen.Atom)
	if ok == false {
		t.Fatalf("expected *gen.Atom, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *decodedPtr != "test_atom" {
		t.Fatalf("expected 'test_atom', got '%s'", *decodedPtr)
	}
}

func TestDecodePtrAtomNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *gen.Atom = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*gen.Atom)
	if ok == false {
		t.Fatalf("expected *gen.Atom, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatal("expected nil pointer")
	}
}

func TestDecodePtrTime(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	tm := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	ptr := &tm

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*time.Time)
	if ok == false {
		t.Fatalf("expected *time.Time, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if decodedPtr.Equal(tm) == false {
		t.Fatalf("expected %v, got %v", tm, *decodedPtr)
	}
}

func TestDecodePtrTimeNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *time.Time = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*time.Time)
	if ok == false {
		t.Fatalf("expected *time.Time, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatal("expected nil pointer")
	}
}

// Array of pointers

func TestDecodeArrayPtr(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	a, c := 1, 3
	arr := [3]*int{&a, nil, &c}

	if err := Encode(arr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedArr, ok := decoded.([3]*int)
	if ok == false {
		t.Fatalf("expected [3]*int, got %T", decoded)
	}

	if decodedArr[0] == nil || *decodedArr[0] != 1 {
		t.Fatal("incorrect value at index 0")
	}
	if decodedArr[1] != nil {
		t.Fatal("expected nil at index 1")
	}
	if decodedArr[2] == nil || *decodedArr[2] != 3 {
		t.Fatal("incorrect value at index 2")
	}
}

// Pointer to array

func TestDecodePtrArray(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	arr := [3]int{1, 2, 3}
	ptr := &arr

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*[3]int)
	if ok == false {
		t.Fatalf("expected *[3]int, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *decodedPtr != [3]int{1, 2, 3} {
		t.Fatalf("expected [1,2,3], got %v", *decodedPtr)
	}
}

func TestDecodePtrArrayNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *[3]int = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*[3]int)
	if ok == false {
		t.Fatalf("expected *[3]int, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatal("expected nil pointer")
	}
}

// Registered struct with pointer fields

type testStructWithPtrField struct {
	Name  string
	Value *int
	Data  *string
}

func init() {
	RegisterTypeOf(testStructWithPtrField{})
}

func TestDecodeStructWithPtrField(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := 42
	s := "hello"
	st := testStructWithPtrField{
		Name:  "test",
		Value: &v,
		Data:  &s,
	}

	if err := Encode(st, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	dst, ok := decoded.(testStructWithPtrField)
	if ok == false {
		t.Fatalf("expected testStructWithPtrField, got %T", decoded)
	}

	if dst.Name != "test" {
		t.Fatalf("expected Name='test', got '%s'", dst.Name)
	}
	if dst.Value == nil || *dst.Value != 42 {
		t.Fatal("incorrect Value field")
	}
	if dst.Data == nil || *dst.Data != "hello" {
		t.Fatal("incorrect Data field")
	}
}

func TestDecodeStructWithPtrFieldNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	st := testStructWithPtrField{
		Name:  "test",
		Value: nil,
		Data:  nil,
	}

	if err := Encode(st, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	dst, ok := decoded.(testStructWithPtrField)
	if ok == false {
		t.Fatalf("expected testStructWithPtrField, got %T", decoded)
	}

	if dst.Name != "test" {
		t.Fatalf("expected Name='test', got '%s'", dst.Name)
	}
	if dst.Value != nil {
		t.Fatal("expected nil Value field")
	}
	if dst.Data != nil {
		t.Fatal("expected nil Data field")
	}
}

func TestDecodeStructWithPtrFieldMixed(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := 99
	st := testStructWithPtrField{
		Name:  "mixed",
		Value: &v,
		Data:  nil, // one nil, one non-nil
	}

	if err := Encode(st, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	dst, ok := decoded.(testStructWithPtrField)
	if ok == false {
		t.Fatalf("expected testStructWithPtrField, got %T", decoded)
	}

	if dst.Name != "mixed" {
		t.Fatalf("expected Name='mixed', got '%s'", dst.Name)
	}
	if dst.Value == nil || *dst.Value != 99 {
		t.Fatal("incorrect Value field")
	}
	if dst.Data != nil {
		t.Fatal("expected nil Data field")
	}
}

// Additional integer pointer types

func TestDecodePtrInt8(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := int8(127)
	ptr := &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*int8)
	if ok == false {
		t.Fatalf("expected *int8, got %T", decoded)
	}
	if *decodedPtr != 127 {
		t.Fatalf("expected 127, got %d", *decodedPtr)
	}
}

func TestDecodePtrInt16(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := int16(32767)
	ptr := &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*int16)
	if ok == false {
		t.Fatalf("expected *int16, got %T", decoded)
	}
	if *decodedPtr != 32767 {
		t.Fatalf("expected 32767, got %d", *decodedPtr)
	}
}

func TestDecodePtrInt32(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := int32(2147483647)
	ptr := &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*int32)
	if ok == false {
		t.Fatalf("expected *int32, got %T", decoded)
	}
	if *decodedPtr != 2147483647 {
		t.Fatalf("expected 2147483647, got %d", *decodedPtr)
	}
}

func TestDecodePtrUint(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := uint(18446744073709551615)
	ptr := &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*uint)
	if ok == false {
		t.Fatalf("expected *uint, got %T", decoded)
	}
	if *decodedPtr != 18446744073709551615 {
		t.Fatalf("expected max uint, got %d", *decodedPtr)
	}
}

func TestDecodePtrUint8(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := uint8(255)
	ptr := &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*uint8)
	if ok == false {
		t.Fatalf("expected *uint8, got %T", decoded)
	}
	if *decodedPtr != 255 {
		t.Fatalf("expected 255, got %d", *decodedPtr)
	}
}

func TestDecodePtrUint16(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := uint16(65535)
	ptr := &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*uint16)
	if ok == false {
		t.Fatalf("expected *uint16, got %T", decoded)
	}
	if *decodedPtr != 65535 {
		t.Fatalf("expected 65535, got %d", *decodedPtr)
	}
}

func TestDecodePtrUint32(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := uint32(4294967295)
	ptr := &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*uint32)
	if ok == false {
		t.Fatalf("expected *uint32, got %T", decoded)
	}
	if *decodedPtr != 4294967295 {
		t.Fatalf("expected 4294967295, got %d", *decodedPtr)
	}
}

func TestDecodePtrUint64(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := uint64(18446744073709551615)
	ptr := &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*uint64)
	if ok == false {
		t.Fatalf("expected *uint64, got %T", decoded)
	}
	if *decodedPtr != 18446744073709551615 {
		t.Fatalf("expected max uint64, got %d", *decodedPtr)
	}
}

func TestDecodePtrFloat32(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := float32(3.14)
	ptr := &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*float32)
	if ok == false {
		t.Fatalf("expected *float32, got %T", decoded)
	}
	if *decodedPtr != float32(3.14) {
		t.Fatalf("expected 3.14, got %f", *decodedPtr)
	}
}

// Ergo types pointers

func TestDecodePtrPID(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	pid := gen.PID{Node: "test@localhost", ID: 12345, Creation: 1}
	ptr := &pid

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*gen.PID)
	if ok == false {
		t.Fatalf("expected *gen.PID, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if decodedPtr.Node != "test@localhost" || decodedPtr.ID != 12345 {
		t.Fatalf("incorrect PID: %v", *decodedPtr)
	}
}

func TestDecodePtrPIDNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *gen.PID = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*gen.PID)
	if ok == false {
		t.Fatalf("expected *gen.PID, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatal("expected nil pointer")
	}
}

func TestDecodePtrProcessID(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	pid := gen.ProcessID{Node: "test@localhost", Name: "myprocess"}
	ptr := &pid

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*gen.ProcessID)
	if ok == false {
		t.Fatalf("expected *gen.ProcessID, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if decodedPtr.Node != "test@localhost" || decodedPtr.Name != "myprocess" {
		t.Fatalf("incorrect ProcessID: %v", *decodedPtr)
	}
}

func TestDecodePtrProcessIDNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *gen.ProcessID = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*gen.ProcessID)
	if ok == false {
		t.Fatalf("expected *gen.ProcessID, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatal("expected nil pointer")
	}
}

func TestDecodePtrRef(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	ref := gen.Ref{Node: "test@localhost", Creation: 1, ID: [3]uint64{1, 2, 3}}
	ptr := &ref

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*gen.Ref)
	if ok == false {
		t.Fatalf("expected *gen.Ref, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if decodedPtr.Node != "test@localhost" || decodedPtr.ID != [3]uint64{1, 2, 3} {
		t.Fatalf("incorrect Ref: %v", *decodedPtr)
	}
}

func TestDecodePtrRefNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *gen.Ref = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*gen.Ref)
	if ok == false {
		t.Fatalf("expected *gen.Ref, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatal("expected nil pointer")
	}
}

func TestDecodePtrAlias(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	alias := gen.Alias{Node: "test@localhost", Creation: 1, ID: [3]uint64{4, 5, 6}}
	ptr := &alias

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*gen.Alias)
	if ok == false {
		t.Fatalf("expected *gen.Alias, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if decodedPtr.Node != "test@localhost" || decodedPtr.ID != [3]uint64{4, 5, 6} {
		t.Fatalf("incorrect Alias: %v", *decodedPtr)
	}
}

func TestDecodePtrAliasNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *gen.Alias = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*gen.Alias)
	if ok == false {
		t.Fatalf("expected *gen.Alias, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatal("expected nil pointer")
	}
}

func TestDecodePtrEvent(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	event := gen.Event{Node: "test@localhost", Name: "myevent"}
	ptr := &event

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*gen.Event)
	if ok == false {
		t.Fatalf("expected *gen.Event, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if decodedPtr.Node != "test@localhost" || decodedPtr.Name != "myevent" {
		t.Fatalf("incorrect Event: %v", *decodedPtr)
	}
}

func TestDecodePtrEventNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *gen.Event = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*gen.Event)
	if ok == false {
		t.Fatalf("expected *gen.Event, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatal("expected nil pointer")
	}
}

// Pointer to registered struct

type testRegisteredStruct struct {
	X int
	Y string
}

func init() {
	RegisterTypeOf(testRegisteredStruct{})
}

func TestDecodePtrToRegisteredStruct(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	st := testRegisteredStruct{X: 42, Y: "hello"}
	ptr := &st

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*testRegisteredStruct)
	if ok == false {
		t.Fatalf("expected *testRegisteredStruct, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if decodedPtr.X != 42 || decodedPtr.Y != "hello" {
		t.Fatalf("incorrect struct: %v", *decodedPtr)
	}
}

func TestDecodePtrToRegisteredStructNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr *testRegisteredStruct = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*testRegisteredStruct)
	if ok == false {
		t.Fatalf("expected *testRegisteredStruct, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatal("expected nil pointer")
	}
}

// Slice of pointers to registered struct

func TestDecodeSlicePtrToRegisteredStruct(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	st1 := testRegisteredStruct{X: 1, Y: "one"}
	st2 := testRegisteredStruct{X: 2, Y: "two"}
	slice := []*testRegisteredStruct{&st1, nil, &st2}

	if err := Encode(slice, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedSlice, ok := decoded.([]*testRegisteredStruct)
	if ok == false {
		t.Fatalf("expected []*testRegisteredStruct, got %T", decoded)
	}
	if len(decodedSlice) != 3 {
		t.Fatalf("expected len 3, got %d", len(decodedSlice))
	}
	if decodedSlice[0] == nil || decodedSlice[0].X != 1 {
		t.Fatal("incorrect value at index 0")
	}
	if decodedSlice[1] != nil {
		t.Fatal("expected nil at index 1")
	}
	if decodedSlice[2] == nil || decodedSlice[2].X != 2 {
		t.Fatal("incorrect value at index 2")
	}
}

// Type alias for pointer: type myPtrType *bool

type myPtrBool *bool

func TestDecodeTypePtrAlias(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := true
	var ptr myPtrBool = &v

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	// Note: decoded type will be *bool, not myPtrBool
	// because EDF doesn't preserve type aliases
	decodedPtr, ok := decoded.(*bool)
	if ok == false {
		t.Fatalf("expected *bool, got %T", decoded)
	}
	if decodedPtr == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *decodedPtr != true {
		t.Fatal("expected true")
	}
}

func TestDecodeTypePtrAliasNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var ptr myPtrBool = nil

	if err := Encode(ptr, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	decodedPtr, ok := decoded.(*bool)
	if ok == false {
		t.Fatalf("expected *bool, got %T", decoded)
	}
	if decodedPtr != nil {
		t.Fatal("expected nil pointer")
	}
}

// Pointer to type alias (*myPtrBool = **bool) - should be rejected

func TestDecodePtrToTypePtrAlias(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := true
	var ptr myPtrBool = &v
	ptrptr := &ptr // this is *myPtrBool which is **bool

	err := Encode(ptrptr, b, Options{})
	if err == nil {
		t.Fatal("expected error for pointer to pointer type alias")
	}
	// Should get "nested pointer type is not supported" error
}

// Pointer as map key

func TestDecodeMapWithPtrKey(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	k := "key"
	m := map[*string]int{&k: 42}

	if err := Encode(m, b, Options{}); err != nil {
		t.Fatal(err)
	}

	decoded, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	dm, ok := decoded.(map[*string]int)
	if ok == false {
		t.Fatalf("expected map[*string]int, got %T", decoded)
	}

	if len(dm) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(dm))
	}

	for ptr, v := range dm {
		if ptr == nil || *ptr != "key" {
			t.Fatal("incorrect key")
		}
		if v != 42 {
			t.Fatal("incorrect value")
		}
	}
}
