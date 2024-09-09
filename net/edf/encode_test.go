package edf

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

func TestEncodeBool(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	if err := Encode(false, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, []byte{edtBool, 0}) {
		t.Fatal("incorrect value")
	}

	b.Reset()
	if err := Encode(true, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, []byte{edtBool, 1}) {
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceBool(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []bool{false, true, false}
	expect := []byte{edtType, 0, 2,
		edtSlice, edtBool,
		edtSlice,
		0, 0, 0, 3,
		0, 1, 0,
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyBool(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{false, true, false}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtBool, 0,
		edtBool, 1,
		edtBool, 0,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeAtom(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := gen.Atom("hello world")
	expect := []byte{edtAtom,
		0, 0x0b, // len
		0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x77, 0x6f, 0x72, 0x6c, 0x64, // "hello world"
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeAtomCache(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := gen.Atom("hello world")
	expect := []byte{edtAtom,
		0x01, 0x2c, // cached "hello world" => 300
	}

	atomCache := new(sync.Map)
	atomCache.Store(value, uint16(300))

	if err := Encode(value, b, Options{AtomCache: atomCache}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeAtomMapping(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := gen.Atom("hello world")
	mapped := gen.Atom("hi")
	expect := []byte{edtAtom,
		0, 0x02, // len
		0x68, 0x69, // "hi"
	}

	atomMapping := new(sync.Map)
	atomMapping.Store(value, mapped)

	if err := Encode(value, b, Options{AtomMapping: atomMapping}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeAtomMappingCache(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := gen.Atom("hello world")
	mapped := gen.Atom("hi")
	expect := []byte{edtAtom,
		0x01, 0x2c, // mapped "hello world" => "hi", cached "hi" => 300
	}

	atomMapping := new(sync.Map)
	atomMapping.Store(value, mapped)
	atomCache := new(sync.Map)
	atomCache.Store(mapped, uint16(300))

	if err := Encode(value, b, Options{AtomCache: atomCache, AtomMapping: atomMapping}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAtom(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := gen.Atom("hello world")
	value := []gen.Atom{
		v, v, v,
	}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAtom,
		edtSlice,
		0, 0, 0, 3,
		0, 0x0b, // len
		0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x77, 0x6f, 0x72, 0x6c, 0x64, // "hello world"
		0, 0x0b, // len
		0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x77, 0x6f, 0x72, 0x6c, 0x64, // "hello world"
		0, 0x0b, // len
		0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x77, 0x6f, 0x72, 0x6c, 0x64, // "hello world"
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAtomCache(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := gen.Atom("hello world")
	value := []gen.Atom{
		v, v, v,
	}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAtom,
		edtSlice,
		0, 0, 0, 3,
		0x01, 0x2c, // cached "hello world" => 300
		0x01, 0x2c, // cached "hello world" => 300
		0x01, 0x2c, // cached "hello world" => 300
	}

	atomCache := new(sync.Map)
	atomCache.Store(v, uint16(300))

	if err := Encode(value, b, Options{AtomCache: atomCache}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyAtom(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := gen.Atom("hello world")
	value := []any{
		v, nil, v,
	}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtAtom, 0, 0x0b, // len
		0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x77, 0x6f, 0x72, 0x6c, 0x64, // "hello world"
		edtNil,
		edtAtom, 0, 0x0b, // len
		0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x77, 0x6f, 0x72, 0x6c, 0x64, // "hello world"
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyAtomCache(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := gen.Atom("hello world")
	value := []any{
		v, nil, v,
	}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtAtom, 0x01, 0x2c, // cached "hello world" => 300
		edtNil,
		edtAtom, 0x01, 0x2c, // cached "hello world" => 300
	}

	atomCache := new(sync.Map)
	atomCache.Store(v, uint16(300))

	if err := Encode(value, b, Options{AtomCache: atomCache}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeString(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := "abc"
	expect := []byte{edtString, 0, 3, 97, 98, 99}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceString(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := []string{"abc", "def", "ghi"}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtString,
		edtSlice,
		0, 0, 0, 3,
		0, 3, 97, 98, 99, // "abc"
		0, 3, 100, 101, 102, // "def"
		0, 3, 103, 104, 105, // "ghi"
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyString(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := []any{"abc", "def", "ghi"}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtString, 0, 3, 97, 98, 99, // "abc"
		edtString, 0, 3, 100, 101, 102, // "def"
		edtString, 0, 3, 103, 104, 105, // "ghi"
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeBinary(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []byte{1, 2, 3, 4, 5}
	expect := []byte{edtBinary,
		0x0, 0x0, 0x0, 0x05, // len
		0x1, 0x2, 0x3, 0x4, 0x5,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceBinary(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := [][]byte{{1, 2, 3, 4, 5}, {6, 7, 8}, {9}}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtBinary,
		edtSlice,
		0, 0, 0, 3,
		0x0, 0x0, 0x0, 0x05, // len
		0x1, 0x2, 0x3, 0x4, 0x5,
		0x0, 0x0, 0x0, 0x03, // len
		0x6, 0x7, 0x8,
		0x0, 0x0, 0x0, 0x01, // len
		0x9,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyBinary(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{[]byte{1, 2, 3, 4, 5}, []byte{6, 7, 8}, []byte{9}}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtBinary, 0x0, 0x0, 0x0, 0x05, // len
		0x1, 0x2, 0x3, 0x4, 0x5,
		edtBinary, 0x0, 0x0, 0x0, 0x03, // len
		0x6, 0x7, 0x8,
		edtBinary, 0x0, 0x0, 0x0, 0x01, // len
		0x9,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeFloat32(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	if err := Encode(float32(3.14), b, Options{}); err != nil {
		t.Fatal(err)
	}

	expect := []byte{edtFloat32, 64, 72, 245, 195}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceFloat32(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []float32{3.14, 3.15, 3.16}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtFloat32,
		edtSlice,
		0, 0, 0, 3,
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x4a, 0x3d, 0x71, // 3.16
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyFloat32(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{float32(3.14), float32(3.15), float32(3.16)}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtFloat32, 0x40, 0x48, 0xf5, 0xc3, // 3.14
		edtFloat32, 0x40, 0x49, 0x99, 0x9a, // 3.15
		edtFloat32, 0x40, 0x4a, 0x3d, 0x71, // 3.16
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeFloat64(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtFloat64, 64, 9, 30, 184, 81, 235, 133, 31}

	if err := Encode(float64(3.14), b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Println("exp", expect)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceFloat64(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []float64{3.14, 3.15, 3.16}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtFloat64,
		edtSlice,
		0, 0, 0, 3,
		0x40, 0x9, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // 3.14
		0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15
		0x40, 0x9, 0x47, 0xae, 0x14, 0x7a, 0xe1, 0x48, // 3.16

	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyFloat64(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{float64(3.14), float64(3.15), float64(3.16)}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtFloat64, 0x40, 0x9, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // 3.14
		edtFloat64, 0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15
		edtFloat64, 0x40, 0x9, 0x47, 0xae, 0x14, 0x7a, 0xe1, 0x48, // 3.16
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeInteger(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	for _, c := range integerCases() {
		t.Run(c.name, func(t *testing.T) {
			b.Reset()

			if err := Encode(c.integer, b, Options{}); err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(b.B, c.bin) {
				fmt.Printf("exp %#v\n", c.bin)
				fmt.Printf("got %#v\n", b.B)
				t.Fatal("incorrect value")
			}
		})
	}
}

func TestEncodeSliceInt(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []int{1, 2, 3}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtInt,
		edtSlice,
		0, 0, 0, 3,
		0, 0, 0, 0, 0, 0, 0, 1,
		0, 0, 0, 0, 0, 0, 0, 2,
		0, 0, 0, 0, 0, 0, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Println("exp", expect)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyInt(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{int(1), int(2), int(3)}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtInt, 0, 0, 0, 0, 0, 0, 0, 1,
		edtInt, 0, 0, 0, 0, 0, 0, 0, 2,
		edtInt, 0, 0, 0, 0, 0, 0, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Println("exp", expect)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceInt8(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []int8{1, 2, 3, 4, 5}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtInt8,
		edtSlice,
		0, 0, 0, 5,
		1, 2, 3, 4, 5,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyInt8(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{int8(1), int8(2), int8(3)}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtInt8, 1,
		edtInt8, 2,
		edtInt8, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceInt16(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []int16{1, 2, 3}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtInt16,
		edtSlice,
		0, 0, 0, 3,
		0, 1,
		0, 2,
		0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyInt16(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{int16(1), int16(2), int16(3)}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtInt16, 0, 1,
		edtInt16, 0, 2,
		edtInt16, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceInt32(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []int32{1, 2, 3}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtInt32,
		edtSlice,
		0, 0, 0, 3,
		0, 0, 0, 1,
		0, 0, 0, 2,
		0, 0, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyInt32(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{int32(1), int32(2), int32(3)}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtInt32, 0, 0, 0, 1,
		edtInt32, 0, 0, 0, 2,
		edtInt32, 0, 0, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceInt64(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []int64{1, 2, 3}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtInt64,
		edtSlice,
		0, 0, 0, 3,
		0, 0, 0, 0, 0, 0, 0, 1,
		0, 0, 0, 0, 0, 0, 0, 2,
		0, 0, 0, 0, 0, 0, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyInt64(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{int64(1), int64(2), int64(3)}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtInt64, 0, 0, 0, 0, 0, 0, 0, 1,
		edtInt64, 0, 0, 0, 0, 0, 0, 0, 2,
		edtInt64, 0, 0, 0, 0, 0, 0, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceUint(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []uint{1, 2, 3}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtUint,
		edtSlice,
		0, 0, 0, 3,
		0, 0, 0, 0, 0, 0, 0, 1,
		0, 0, 0, 0, 0, 0, 0, 2,
		0, 0, 0, 0, 0, 0, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyUint(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{uint(1), uint(2), uint(3)}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtUint, 0, 0, 0, 0, 0, 0, 0, 1,
		edtUint, 0, 0, 0, 0, 0, 0, 0, 2,
		edtUint, 0, 0, 0, 0, 0, 0, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceUint8(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []uint8{1, 2, 3, 4, 5}
	// since the byte type is the alias to the uint8
	// []byte is the same as []uint8
	expect := []byte{edtBinary,
		0, 0, 0, 5, // len
		1, 2, 3, 4, 5}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceUint16(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []uint16{1, 2, 3}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtUint16,
		edtSlice,
		0, 0, 0, 3,
		0, 1,
		0, 2,
		0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyUint16(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{uint16(1), uint16(2), uint16(3)}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtUint16, 0, 1,
		edtUint16, 0, 2,
		edtUint16, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceUint32(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []uint32{1, 2, 3}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtUint32,
		edtSlice,
		0, 0, 0, 3,
		0, 0, 0, 1,
		0, 0, 0, 2,
		0, 0, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyUint32(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{uint32(1), uint32(2), uint32(3)}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtUint32, 0, 0, 0, 1,
		edtUint32, 0, 0, 0, 2,
		edtUint32, 0, 0, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceUint64(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []uint64{1, 2, 3}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtUint64,
		edtSlice,
		0, 0, 0, 3,
		0, 0, 0, 0, 0, 0, 0, 1,
		0, 0, 0, 0, 0, 0, 0, 2,
		0, 0, 0, 0, 0, 0, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyUint64(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{uint64(1), uint64(2), uint64(3)}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtUint64, 0, 0, 0, 0, 0, 0, 0, 1,
		edtUint64, 0, 0, 0, 0, 0, 0, 0, 2,
		edtUint64, 0, 0, 0, 0, 0, 0, 0, 3,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyInteger(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{
		int(1), nil, int8(2), nil, int16(3), nil, int32(4), nil, int64(5), nil,
		uint(6), nil, uint8(7), nil, uint16(8), nil, uint32(9), nil, uint64(10),
	}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 19,
		edtInt, 0, 0, 0, 0, 0, 0, 0, 1,
		edtNil,
		edtInt8, 2,
		edtNil,
		edtInt16, 0, 3,
		edtNil,
		edtInt32, 0, 0, 0, 4,
		edtNil,
		edtInt64, 0, 0, 0, 0, 0, 0, 0, 5,
		edtNil,
		edtUint, 0, 0, 0, 0, 0, 0, 0, 6,
		edtNil,
		edtUint8, 7,
		edtNil,
		edtUint16, 0, 8,
		edtNil,
		edtUint32, 0, 0, 0, 9,
		edtNil,
		edtUint64, 0, 0, 0, 0, 0, 0, 0, 10,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnySlice(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []any{
		[]int{4},
		nil,
		[]float32{3.14, 3.15, 3.16},
		nil,
	}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 4,

		edtType, 0, 2,
		edtSlice, edtInt,
		edtSlice, 0, 0, 0, 1,
		0, 0, 0, 0, 0, 0, 0, 4,

		edtNil,

		edtType, 0, 2,
		edtSlice, edtFloat32,
		edtSlice, 0, 0, 0, 3,
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x4a, 0x3d, 0x71, // 3.16

		edtNil,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeTime(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := time.Date(1399, time.January, 26, 0, 0, 0, 0, time.UTC)
	expect := []byte{edtTime,
		0xf, // len
		0x1, 0x0, 0x0, 0x0, 0xa, 0x45, 0xaf, 0x1f, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff,
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceTime(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := time.Date(1399, time.January, 26, 0, 0, 0, 0, time.UTC)
	value := []time.Time{
		v, v, v,
	}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtTime,
		edtSlice,
		0, 0, 0, 3,
		0xf, // len
		0x1, 0x0, 0x0, 0x0, 0xa, 0x45, 0xaf, 0x1f, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff,
		0xf, // len
		0x1, 0x0, 0x0, 0x0, 0xa, 0x45, 0xaf, 0x1f, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff,
		0xf, // len
		0x1, 0x0, 0x0, 0x0, 0xa, 0x45, 0xaf, 0x1f, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff,
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyTime(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	v := time.Date(1399, time.January, 26, 0, 0, 0, 0, time.UTC)
	value := []any{
		v, v, v,
	}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtTime, 0xf, // len
		0x1, 0x0, 0x0, 0x0, 0xa, 0x45, 0xaf, 0x1f, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff,
		edtTime, 0xf, // len
		0x1, 0x0, 0x0, 0x0, 0xa, 0x45, 0xaf, 0x1f, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff,
		edtTime, 0xf, // len
		0x1, 0x0, 0x0, 0x0, 0xa, 0x45, 0xaf, 0x1f, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff,
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeReg(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	type MyRegF1 float32
	var value MyRegF1
	value = 3.14
	expect := []byte{edtReg, 0, 35,
		// name: #ergo.services/ergo/net/edf/MyRegF1
		0x23, 0x65, 0x72, 0x67, 0x6f, 0x2e, 0x73, 0x65,
		0x72, 0x76, 0x69, 0x63, 0x65, 0x73, 0x2f, 0x65,
		0x72, 0x67, 0x6f, 0x2f, 0x6e, 0x65, 0x74, 0x2f,
		0x65, 0x64, 0x66, 0x2f, 0x4d, 0x79, 0x52, 0x65,
		0x67, 0x46, 0x31,
		0x40, 0x48, 0xf5, 0xc3, // 3.14
	}

	RegisterTypeOf(value)

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceReg(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	type MyFloattt float32
	var x MyFloattt

	value := []MyFloattt{3.14, 3.15, 3.16}
	expect := []byte{edtType, 0, 41,
		edtSlice,
		edtReg, 0, 37,
		// name: #ergo.services/ergo/net/edf/MyFloat
		0x23, 0x65, 0x72, 0x67, 0x6f, 0x2e, 0x73, 0x65,
		0x72, 0x76, 0x69, 0x63, 0x65, 0x73, 0x2f, 0x65,
		0x72, 0x67, 0x6f, 0x2f, 0x6e, 0x65, 0x74, 0x2f,
		0x65, 0x64, 0x66, 0x2f, 0x4d, 0x79, 0x46, 0x6c,
		0x6f, 0x61, 0x74, 0x74, 0x74,
		edtSlice,
		0, 0, 0, 3, // len
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x4a, 0x3d, 0x71, // 3.16
	}

	RegisterTypeOf(x)

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceRegCache(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	type MyFloat12333 float32
	var x MyFloat12333

	value := []MyFloat12333{3.14, 3.15, 3.16}
	expect := []byte{edtType, 0, 4,
		edtSlice,
		edtReg, 0x13, 0x88, // name: #ergo.services/ergo/net/proto/edf/MyFloat12333 => cache id 5000
		edtSlice,
		0, 0, 0, 3, // len
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x4a, 0x3d, 0x71, // 3.16
	}
	RegisterTypeOf(x)

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(x), []byte{edtReg, 0x13, 0x88})

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeRegSlice(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	type MySlice99 []float32

	x := MySlice99{3.14, 3.15, 3.16}
	expect := []byte{edtReg, 0x13, 0x88,
		edtReg,
		0, 0, 0, 3, // len
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x4a, 0x3d, 0x71, // 3.16
	}
	if err := RegisterTypeOf(x); err != nil {
		t.Fatal(err)
	}

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(x), []byte{edtReg, 0x13, 0x88})

	if err := Encode(x, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodeRegSliceRegSlice(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	type MySliceFloat []float32
	type MySliceOfSlice []MySliceFloat

	x := MySliceOfSlice{
		{3.14, 3.15, 3.16},
		nil,
		{3.14},
	}
	expect := []byte{edtReg, 0x13, 0x88,
		edtReg,
		0x0, 0x0, 0x0, 0x3,
		edtSlice,
		0x0, 0x0, 0x0, 0x3,
		0x40, 0x48, 0xf5, 0xc3,
		0x40, 0x49, 0x99, 0x9a,
		0x40, 0x4a, 0x3d, 0x71,
		edtNil,
		edtSlice,
		0x0, 0x0, 0x0, 0x1,
		0x40, 0x48, 0xf5, 0xc3,
	}

	if err := RegisterTypeOf(MySliceOfSlice{}); err != nil {
		t.Fatal(err)
	}
	if err := RegisterTypeOf(MySliceFloat{}); err != nil {
		t.Fatal(err)
	}

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(x), []byte{edtReg, 0x13, 0x88})

	if err := Encode(x, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodePID(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtPID,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7f, 0xff, // id
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
	}
	value := gen.PID{Node: "abc@def", ID: 32767, Creation: 2}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSlicePID(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtPID,
		edtSlice,
		0, 0, 0, 3,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7f, 0xff, // id
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7f, 0xff, // id
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7f, 0xff, // id
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
	}
	v := gen.PID{Node: "abc@def", ID: 32767, Creation: 2}
	value := []gen.PID{v, v, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyPID(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtPID, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7f, 0xff, // id
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		edtPID, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7f, 0xff, // id
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		edtPID, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7f, 0xff, // id
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
	}
	v := gen.PID{Node: "abc@def", ID: 32767, Creation: 2}
	value := []any{v, v, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeProcessID(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtProcessID,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (node name)
		0x67, 0x68, 0x69,
	}
	value := gen.ProcessID{Node: "abc@def", Name: "ghi"}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceProcessID(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtProcessID,
		edtSlice,
		0, 0, 0, 3,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (process name)
		0x67, 0x68, 0x69,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (process name)
		0x67, 0x68, 0x69,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (process name)
		0x67, 0x68, 0x69,
	}
	v := gen.ProcessID{Node: "abc@def", Name: "ghi"}
	value := []gen.ProcessID{v, v, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyProcessID(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtProcessID, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (process name)
		0x67, 0x68, 0x69,
		edtProcessID, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (node name)
		0x67, 0x68, 0x69,
		edtProcessID, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (node name)
		0x67, 0x68, 0x69,
	}
	v := gen.ProcessID{Node: "abc@def", Name: "ghi"}
	value := []any{v, v, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeEvent(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtEvent,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (node name)
		0x67, 0x68, 0x69,
	}
	value := gen.Event{Node: "abc@def", Name: "ghi"}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceEvent(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtEvent,
		edtSlice,
		0, 0, 0, 3,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (process name)
		0x67, 0x68, 0x69,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (process name)
		0x67, 0x68, 0x69,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (process name)
		0x67, 0x68, 0x69,
	}
	v := gen.Event{Node: "abc@def", Name: "ghi"}
	value := []gen.Event{v, v, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyEvent(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtEvent, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (process name)
		0x67, 0x68, 0x69,
		edtEvent, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (node name)
		0x67, 0x68, 0x69,
		edtEvent, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x3, // len atom (node name)
		0x67, 0x68, 0x69,
	}
	v := gen.Event{Node: "abc@def", Name: "ghi"}
	value := []any{v, v, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeRef(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtRef,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
	}
	value := gen.Ref{Node: "abc@def", ID: [3]uint64{4, 5, 6}, Creation: 2}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceRef(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtRef,
		edtSlice,
		0, 0, 0, 3,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
	}
	v := gen.Ref{Node: "abc@def", ID: [3]uint64{4, 5, 6}, Creation: 2}
	value := []gen.Ref{v, v, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyRef(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtRef, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
		edtRef, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
		edtRef, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
	}
	v := gen.Ref{Node: "abc@def", ID: [3]uint64{4, 5, 6}, Creation: 2}
	value := []any{v, v, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeAlias(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtAlias,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
	}
	value := gen.Alias{Node: "abc@def", ID: [3]uint64{4, 5, 6}, Creation: 2}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAlias(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAlias,
		edtSlice,
		0, 0, 0, 3,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
		0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
	}
	v := gen.Alias{Node: "abc@def", ID: [3]uint64{4, 5, 6}, Creation: 2}
	value := []gen.Alias{v, v, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyAlias(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtAlias, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
		edtAlias, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
		edtAlias, 0x0, 0x7, // len atom (node name)
		0x61, 0x62, 0x63, 0x40, 0x64, 0x65, 0x66,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, // creation
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6,
	}
	v := gen.Alias{Node: "abc@def", ID: [3]uint64{4, 5, 6}, Creation: 2}
	value := []any{v, v, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeError(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtError,
		0, 3, // len
		97, 98, 99, // "abc"
	}
	value := errors.New("abc")

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceError(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtError,
		edtSlice,
		0, 0, 0, 3,
		0, 4, // len
		97, 98, 99, 100, // "abcd"
		0, 4, // len
		97, 98, 99, 100, // "abcd"
		0, 4, // len
		97, 98, 99, 100, // "abcd"
	}
	v := errors.New("abcd")
	value := []error{v, v, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceErrorNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtError,
		edtSlice,
		0, 0, 0, 3,
		0, 4, // len
		97, 98, 99, 100, // "abcd"
		0xff, 0xff, // nil error
		0, 4, // len
		97, 98, 99, 100, // "abcd"
	}
	v := errors.New("abcd")
	value := []error{v, nil, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeRegError(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := errors.New("abc")
	errCache := new(sync.Map)
	errCache.Store(value, uint16(35000))

	expect := []byte{edtError,
		0x88, 0xb8, // 35000 => error "abc"
	}

	if err := Encode(value, b, Options{ErrCache: errCache}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyError(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtError, 0, 4, // len
		97, 98, 99, 100, // "abcd"
		edtNil,
		edtError, 0, 4, // len
		97, 98, 99, 100, // "abcd"
	}
	v := errors.New("abcd")
	value := []any{v, nil, v}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceType(t *testing.T) {
	b := lib.TakeBuffer()

	value := []float32{3.14, 3.15, 3.16}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtFloat32,
		edtSlice,
		0, 0, 0, 3, // len
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x4a, 0x3d, 0x71, // 3.16
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

	lib.ReleaseBuffer(b)
}

func TestEncodeSliceTypeReg(t *testing.T) {
	type MyFloaaa float32
	var x MyFloaaa

	b := lib.TakeBuffer()
	value := []MyFloaaa{3.14, 3.15, 3.16}
	expect := []byte{edtType, 0, 40,
		edtSlice,
		edtReg, 0, 36,
		// name: #ergo.services/ergo/net/edf/MyFloa
		0x23, 0x65, 0x72, 0x67, 0x6f, 0x2e, 0x73, 0x65,
		0x72, 0x76, 0x69, 0x63, 0x65, 0x73, 0x2f, 0x65,
		0x72, 0x67, 0x6f, 0x2f, 0x6e, 0x65, 0x74, 0x2f,
		0x65, 0x64, 0x66, 0x2f, 0x4d, 0x79, 0x46, 0x6c,
		0x6f, 0x61, 0x61, 0x61,
		edtSlice,
		0, 0, 0, 3, // len
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x4a, 0x3d, 0x71, // 3.16
	}

	RegisterTypeOf(x)

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

	lib.ReleaseBuffer(b)
}

func TestEncodeSliceTypeRegCache(t *testing.T) {
	type MyFloatE123 float32
	var x MyFloatE123

	b := lib.TakeBuffer()
	value := []MyFloatE123{3.14, 3.15, 3.16}
	expect := []byte{edtType, 0, 4,
		edtSlice,
		edtReg, 0x13, 0x88, // cache id uint16(5000) => name: #ergo.services/ergo/net/proto/edf/MyFloatE123
		edtSlice,
		0, 0, 0, 3, // len
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x4a, 0x3d, 0x71, // 3.16
	}

	RegisterTypeOf(x)

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(x), []byte{edtReg, 0x13, 0x88})

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

	lib.ReleaseBuffer(b)
}

func TestEncodeSliceRegTypeReg(t *testing.T) {
	type MyFloatE19 float32
	type MySliceE19 []MyFloatE19
	var x MyFloatE19

	b := lib.TakeBuffer()
	value := MySliceE19{3.14, 3.15, 3.16}
	expect := []byte{edtReg, 0x13, 0x88,
		edtReg,
		0, 0, 0, 3, // len
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x4a, 0x3d, 0x71, // 3.16
	}

	if err := RegisterTypeOf(x); err != nil {
		t.Fatal(err)
	}

	if err := RegisterTypeOf(value); err != nil {
		t.Fatal(err)
	}
	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(value), []byte{edtReg, 0x13, 0x88})
	regCache.Store(reflect.TypeOf(x), []byte{edtReg, 0x13, 0x89})

	opts := Options{
		RegCache: regCache,
	}
	if err := Encode(value, b, opts); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

	lib.ReleaseBuffer(b)
}

func TestEncodeSliceAny(t *testing.T) {

	b := lib.TakeBuffer()
	value := []any{float32(3.14), float64(3.15), float32(3.16)}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtFloat32, 0x40, 0x48, 0xf5, 0xc3, // 3.14
		edtFloat64, 0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15
		edtFloat32, 0x40, 0x4a, 0x3d, 0x71, // 3.16
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

	lib.ReleaseBuffer(b)
}

func TestEncodeSliceNil(t *testing.T) {
	b := lib.TakeBuffer()
	value := []any{nil, nil, nil}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 3,
		edtNil,
		edtNil,
		edtNil,
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodeSliceNil2(t *testing.T) {
	b := lib.TakeBuffer()
	value := []any{}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 0,
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodeSliceNest(t *testing.T) {
	b := lib.TakeBuffer()
	value := []any{
		[]any{float32(3.15)},
		float32(3.14),
		float32(3.16),
		[]any{float64(3.15)},
	}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 4,
		edtType, 0, 2,
		edtSlice, edtAny,
		edtSlice,
		0, 0, 0, 1, edtFloat32, 0x40, 0x49, 0x99, 0x9a, // 3.15
		edtFloat32, 0x40, 0x48, 0xf5, 0xc3, // 3.14
		edtFloat32, 0x40, 0x4a, 0x3d, 0x71, // 3.16
		edtType, 0, 2,
		edtSlice, edtAny,
		edtSlice,
		0, 0, 0, 1, edtFloat64, 0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodeSliceSlice(t *testing.T) {
	b := lib.TakeBuffer()
	value := [][]float32{
		{3.14, 3.15, 3.16},
		{3.16},
		nil,
		{3.14, 3.15},
		{},
	}
	expect := []byte{edtType, 0, 3,
		edtSlice,
		edtSlice,
		edtFloat32,
		edtSlice,
		0, 0, 0, 5,
		edtSlice,
		0, 0, 0, 3, // first slice with 3 items
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x4a, 0x3d, 0x71, // 3.16
		edtSlice,
		0, 0, 0, 1, // second slice with 1 item
		0x40, 0x4a, 0x3d, 0x71, // 3.16
		edtNil, // third one
		edtSlice,
		0, 0, 0, 2, // 4th
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x49, 0x99, 0x9a, // 3.15
		edtSlice,
		0, 0, 0, 0, // 5th
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodeSliceSliceAny(t *testing.T) {
	b := lib.TakeBuffer()
	value := [][]any{
		{float32(3.14), float32(3.16), float64(3.15)},
		{float64(3.15)},
		nil,
		{float32(3.14), float32(3.16)},
		{},
	}
	expect := []byte{edtType, 0, 3,
		edtSlice,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 5,
		edtSlice,
		0, 0, 0, 3, // first slice with 3 items
		edtFloat32, 0x40, 0x48, 0xf5, 0xc3, // 3.14
		edtFloat32, 0x40, 0x4a, 0x3d, 0x71, // 3.16
		edtFloat64, 0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15
		edtSlice,
		0, 0, 0, 1, // second slice with 1 item
		edtFloat64, 0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15
		edtNil, // third one
		edtSlice,
		0, 0, 0, 2, // 4th
		edtFloat32, 0x40, 0x48, 0xf5, 0xc3, // 3.14
		edtFloat32, 0x40, 0x4a, 0x3d, 0x71, // 3.16
		edtSlice,
		0, 0, 0, 0, // 5th
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodeSliceSliceNil(t *testing.T) {
	b := lib.TakeBuffer()
	value := [][]any{nil, []any{}, nil, nil}
	expect := []byte{edtType, 0, 3,
		edtSlice,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 4,
		edtNil,
		edtSlice,
		0, 0, 0, 0,
		edtNil,
		edtNil,
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodeSliceSliceReg(t *testing.T) {
	b := lib.TakeBuffer()

	type MySlice1555 []float32

	if err := RegisterTypeOf(MySlice1555{}); err != nil {
		if err != gen.ErrTaken {
			t.Fatal(err)
		}
	}

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(MySlice1555{}), []byte{edtReg, 0x13, 0x88})

	value := []MySlice1555{
		MySlice1555{3.14, 3.16, 3.15},
		MySlice1555{3.15},
		nil,
		MySlice1555{3.14, 3.16},
		MySlice1555{},
	}
	expect := []byte{edtType, 0, 4,
		edtSlice,
		edtReg, 0x13, 0x88,
		edtSlice,
		0, 0, 0, 5,
		edtReg,
		0, 0, 0, 3, // first slice with 3 items
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x4a, 0x3d, 0x71, // 3.16
		0x40, 0x49, 0x99, 0x9a, // 3.15
		edtReg,
		0, 0, 0, 1, // second slice with 1 item
		0x40, 0x49, 0x99, 0x9a, // 3.15
		edtNil,
		edtReg,
		0, 0, 0, 2, // 4th
		0x40, 0x48, 0xf5, 0xc3, // 3.14
		0x40, 0x4a, 0x3d, 0x71, // 3.16
		edtReg,
		0, 0, 0, 0, // third one
	}

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodeSlice3DZero(t *testing.T) {

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := [][][]float32{}
	expect := []byte{edtType, 0, 4,
		edtSlice,
		edtSlice,
		edtSlice,
		edtFloat32,
		edtSlice,
		0, 0, 0, 0,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodeSlice3D(t *testing.T) {
	b := lib.TakeBuffer()

	value := [][][]float32{ /* len 3 */
		{ /* len 5 */
			{ /* len 7 */ 2.21018848, 2.94523878, 1.67807658, 1.30014748, 1.1873558, 8.1819557, 3.2368748},
			{ /* len 10 */ 2.17948558, 2.95483828, 3.29734688, 2.72996818, 2.50011478, 2.98767788, 1.31364818, 8.06395757, 2.53354848, 2.38570578},
			{ /* len 4 */ 2.9838078, 1.61728128, 1.8756628, 1.5756598},
			{ /* len 10 */ 8.5187367, 2.79348, 4.3456557, 1.29794587, 3.38391948, 1.4460748, 5.0206397, 2.02001097, 1.77825548, 2.33810328},
			{ /* len 8 */ 3.15617888, 2.21068618, 3.01507718, 7.0342597, 2.12085158, 7.9914467, 2.92003388, 3.19992137},
		}, { /* len 6 */
			{ /* len 3 */ 3.3188187, 2.82300078, 7.3257346},
			{ /* len 10 */ 1.47951058, 1.47638718, 3.1678068, 1.24334058, 1.48100658, 1.8274938, 2.07265258, 1.83188888, 5.8776197, 1.64099568},
			{ /* len 6 */ 2.26154558, 9.5987497, 3.24544727, 1.34864688, 2.47839448, 2.0456888},
			{ /* len 5 */ 9.0369537, 3.69528477, 3.04563028, 1.4488858, 3.80179227},
			{ /* len 5 */ 1.53326348, 2.77105168, 1.05977548, 2.75297638, 8.9171847},
			{ /* len 10 */ 1.65367358, 9.4070457, 3.06440548, 2.4763148, 2.22120158, 2.3734938, 3.37481478, 2.22900497, 6.2138987, 2.80613798},
		}, { /* len 1 */
			{ /* len 10 */ 8.03434337, 2.55059418, 2.20168828, 2.86517478, 4.38993137, 8.6655217, 2.22159657, 3.0119788, 1.19758818, 2.58799087},
		},
	}

	expect := []byte{edtType, 0, 4,
		edtSlice,
		edtSlice,
		edtSlice,
		edtFloat32,

		edtSlice,
		0x0, 0x0, 0x0, 0x3, // len 3 { x, x, x}
		edtSlice,
		0x0, 0x0, 0x0, 0x5, // len 5 { {y, y, y, y, y}, x, x}
		edtSlice,
		0x0, 0x0, 0x0, 0x7, // len 7 { { {z, z, z, z, z, z, z}, y, y, y, y}, x, x}
		0x40, 0xd, 0x73, 0xba, // z
		0x40, 0x3c, 0x7e, 0xcb, // z
		0x3f, 0xd6, 0xcb, 0x37, // z
		0x3f, 0xa6, 0x6b, 0x3c, // z
		0x3f, 0x97, 0xfb, 0x46, // z
		0x41, 0x2, 0xe9, 0x4a, // z
		0x40, 0x4f, 0x28, 0xf5, // z
		edtSlice,
		0x0, 0x0, 0x0, 0xa, // len 10
		0x40, 0xb, 0x7c, 0xb1,
		0x40, 0x3d, 0x1c, 0x12,
		0x40, 0x53, 0x7, 0xbb,
		0x40, 0x2e, 0xb7, 0xcc,
		0x40, 0x20, 0x1, 0xe1,
		0x40, 0x3f, 0x36, 0x1d,
		0x3f, 0xa8, 0x25, 0xa0,
		0x41, 0x1, 0x5, 0xf8,
		0x40, 0x22, 0x25, 0xa9,
		0x40, 0x18, 0xaf, 0x67,
		edtSlice,
		0x0, 0x0, 0x0, 0x4, // len 4
		0x40, 0x3e, 0xf6, 0xb5,
		0x3f, 0xcf, 0x3, 0x13,
		0x3f, 0xf0, 0x15, 0xb8,
		0x3f, 0xc9, 0xaf, 0x38,
		edtSlice,
		0x0, 0x0, 0x0, 0xa, // len 10
		0x41, 0x8, 0x4c, 0xbf,
		0x40, 0x32, 0xc8, 0x60,
		0x40, 0x8b, 0xf, 0x9d,
		0x3f, 0xa6, 0x23, 0x17,
		0x40, 0x58, 0x92, 0x23,
		0x3f, 0xb9, 0x18, 0xfb,
		0x40, 0xa0, 0xa9, 0x15,
		0x40, 0x1, 0x47, 0xdc,
		0x3f, 0xe3, 0x9d, 0xe0,
		0x40, 0x15, 0xa3, 0x7c,
		edtSlice,
		0x0, 0x0, 0x0, 0x8, // len 8
		0x40, 0x49, 0xfe, 0xd6,
		0x40, 0xd, 0x7b, 0xe2,
		0x40, 0x40, 0xf7, 0x6,
		0x40, 0xe1, 0x18, 0xa8,
		0x40, 0x7, 0xbc, 0x8,
		0x40, 0xff, 0xb9, 0xee,
		0x40, 0x3a, 0xe1, 0xd6,
		0x40, 0x4c, 0xcb, 0x83,
		edtSlice,
		0x0, 0x0, 0x0, 0x6, // len 6
		edtSlice,
		0x0, 0x0, 0x0, 0x3, // len 3
		0x40, 0x54, 0x67, 0x87,
		0x40, 0x34, 0xac, 0xb,
		0x40, 0xea, 0x6c, 0x6b,
		edtSlice,
		0x0, 0x0, 0x0, 0xa, // len 10
		0x3f, 0xbd, 0x60, 0x9a,
		0x3f, 0xbc, 0xfa, 0x41,
		0x40, 0x4a, 0xbd, 0x59,
		0x3f, 0x9f, 0x25, 0xc9,
		0x3f, 0xbd, 0x91, 0xa0,
		0x3f, 0xe9, 0xeb, 0x51,
		0x40, 0x4, 0xa6, 0x57,
		0x3f, 0xea, 0x7b, 0x56,
		0x40, 0xbc, 0x15, 0x76,
		0x3f, 0xd2, 0xc, 0x25,
		edtSlice,
		0x0, 0x0, 0x0, 0x6, // len 6
		0x40, 0x10, 0xbd, 0x2a,
		0x41, 0x19, 0x94, 0x7b,
		0x40, 0x4f, 0xb5, 0x68,
		0x3f, 0xac, 0xa0, 0x76,
		0x40, 0x1e, 0x9e, 0x4,
		0x40, 0x2, 0xec, 0x91,
		edtSlice,
		0x0, 0x0, 0x0, 0x5, // len 5
		0x41, 0x10, 0x97, 0x5d,
		0x40, 0x6c, 0x7f, 0x8c,
		0x40, 0x42, 0xeb, 0x9b,
		0x3f, 0xb9, 0x75, 0x17,
		0x40, 0x73, 0x50, 0x91,
		edtSlice,
		0x0, 0x0, 0x0, 0x5, // len 5
		0x3f, 0xc4, 0x41, 0xfa,
		0x40, 0x31, 0x58, 0xe9,
		0x3f, 0x87, 0xa6, 0xb9,
		0x40, 0x30, 0x30, 0xc4,
		0x41, 0xe, 0xac, 0xca,
		edtSlice,
		0x0, 0x0, 0x0, 0xa, // len 10
		0x3f, 0xd3, 0xab, 0x93,
		0x41, 0x16, 0x83, 0x42,
		0x40, 0x44, 0x1f, 0x38,
		0x40, 0x1e, 0x7b, 0xf1,
		0x40, 0xe, 0x28, 0x2b,
		0x40, 0x17, 0xe7, 0x53,
		0x40, 0x57, 0xfc, 0xf7,
		0x40, 0xe, 0xa8, 0x4,
		0x40, 0xc6, 0xd8, 0x42,
		0x40, 0x33, 0x97, 0xc4,
		edtSlice,
		0x0, 0x0, 0x0, 0x1, // len 1
		edtSlice,
		0x0, 0x0, 0x0, 0xa, // len 10
		0x41, 0x0, 0x8c, 0xac,
		0x40, 0x23, 0x3c, 0xef,
		0x40, 0xc, 0xe8, 0x76,
		0x40, 0x37, 0x5f, 0x6,
		0x40, 0x8c, 0x7a, 0x51,
		0x41, 0xa, 0xa5, 0xfa,
		0x40, 0xe, 0x2e, 0xa3,
		0x40, 0x40, 0xc4, 0x43,
		0x3f, 0x99, 0x4a, 0x92,
		0x40, 0x25, 0xa1, 0xa4,
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

	lib.ReleaseBuffer(b)
}

type testMarshal struct{}

func (testMarshal) MarshalEDF(w io.Writer) error {
	w.Write([]byte{10, 20, 30, 40})
	return nil
}

func (*testMarshal) UnmarshalEDF(b []byte) error {
	return nil
}

func TestEncodeMarshal(t *testing.T) {
	var value testMarshal

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	if err := Encode(value, b, Options{}); err == nil {
		t.Fatal("incorrect value")
	}
	b.Reset()

	if err := RegisterTypeOf(value); err != nil {
		t.Fatal(err)
	}
	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(value), []byte{edtReg, 0x13, 0x88})

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	expect := []byte{edtReg, 0x13, 0x88,
		0, 0, 0, 4,
		10, 20, 30, 40}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceMarshal(t *testing.T) {
	x := testMarshal{}
	value := []testMarshal{x, x}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	RegisterTypeOf(x)

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(x), []byte{edtReg, 0x13, 0x88})

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	expect := []byte{edtType, 0, 4,
		edtSlice,
		edtReg, 0x13, 0x88,
		edtSlice,
		0, 0, 0, 2, // num of elements
		0, 0, 0, 4, // len
		10, 20, 30, 40,
		0, 0, 0, 4, // len
		10, 20, 30, 40,
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

type testStruct struct {
	A float32
	B float64
}

func TestEncodeStruct(t *testing.T) {

	if err := RegisterTypeOf(testStruct{}); err != nil {
		if err != gen.ErrTaken {
			t.Fatal(err)
		}
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := testStruct{3.16, 3.15}

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(value), []byte{edtReg, 0x13, 0x88})

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	expect := []byte{edtReg, 0x13, 0x88,
		0x40, 0x4a, 0x3d, 0x71, // 3.16
		0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceStruct(t *testing.T) {

	if err := RegisterTypeOf(testStruct{}); err != nil {
		if err != gen.ErrTaken {
			t.Fatal(err)
		}
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := []testStruct{{3.16, 3.15}, {3.15, 3.14}}
	expect := []byte{edtType, 0, 4,
		edtSlice,
		edtReg, 0x13, 0x88,
		edtSlice,
		0, 0, 0, 2,
		0x40, 0x4a, 0x3d, 0x71, // 3.16
		0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x9, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // 3.14
	}

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(testStruct{}), []byte{edtReg, 0x13, 0x88})

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

type testStructWithAny struct {
	A float32
	B float64
	C any
}

func TestEncodeStructWithAny(t *testing.T) {
	if err := RegisterTypeOf(testStructWithAny{}); err != nil {
		if err != gen.ErrTaken {
			t.Fatal(err)
		}
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := testStructWithAny{3.16, 3.15, nil}
	expect := []byte{edtReg, 0x13, 0x88,
		0x40, 0x4a, 0x3d, 0x71, // 3.16
		0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15
		edtNil,
	}

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(testStructWithAny{}), []byte{edtReg, 0x13, 0x88})

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}

	b.Reset()

	value = testStructWithAny{3.15, 3.14, float64(3.14)}
	expect = []byte{edtReg, 0x13, 0x88,
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x9, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // 3.14
		edtFloat64, 0x40, 0x9, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // 3.14
	}

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

type regSliceString []string
type testStructWithSlice struct {
	A float32
	B float64
	C []bool
	D regSliceString
	E []int
}

func TestEncodeStructWithSlice(t *testing.T) {
	if err := RegisterTypeOf(regSliceString{}); err != nil {
		if err != gen.ErrTaken {
			t.Fatal(err)
		}
	}

	if err := RegisterTypeOf(testStructWithSlice{}); err != nil {
		if err != gen.ErrTaken {
			t.Fatal(err)
		}
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := testStructWithSlice{
		3.16,
		3.15,
		[]bool{true, false},
		regSliceString{"true", "false"},
		nil,
	}

	expect := []byte{edtReg, 0x13, 0x88,
		0x40, 0x4a, 0x3d, 0x71, // 3.16 (float32)
		0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15 (float64)
		edtSlice,
		0x0, 0x0, 0x0, 0x2, // len of []bool
		0x1, 0x0, // true, false
		edtReg,             // regSliceString
		0x0, 0x0, 0x0, 0x2, // len of regSliceString
		0x0, 0x4, // len of "true"
		0x74, 0x72, 0x75, 0x65, // "true"
		0x0, 0x5, // len of "false"
		0x66, 0x61, 0x6c, 0x73, 0x65, // "false"
		edtNil, // nil value of []int
	}

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(testStructWithSlice{}), []byte{edtReg, 0x13, 0x88})

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceStructWithAny(t *testing.T) {

	if err := RegisterTypeOf(testStructWithAny{}); err != nil {
		if err != gen.ErrTaken {
			t.Fatal(err)
		}
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(testStructWithAny{}), []byte{edtReg, 0x13, 0x88})

	value := []testStructWithAny{
		{3.16, 3.15, nil},
		{3.16, 3.15, float32(3.16)},
		{3.15, 3.14, float64(3.14)},
	}

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	expect := []byte{edtType, 0, 4,
		edtSlice,
		edtReg, 0x13, 0x88,
		edtSlice,
		0, 0, 0, 3,
		0x40, 0x4a, 0x3d, 0x71, // 3.16
		0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15
		edtNil,
		0x40, 0x4a, 0x3d, 0x71, // 3.16
		0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15
		edtFloat32, 0x40, 0x4a, 0x3d, 0x71, // 3.16
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x9, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // 3.14
		edtFloat64, 0x40, 0x9, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // 3.14
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyWithStruct(t *testing.T) {

	if err := RegisterTypeOf(testStruct{}); err != nil {
		if err != gen.ErrTaken {
			t.Fatal(err)
		}
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(testStruct{}), []byte{edtReg, 0x13, 0x88})

	value := []any{
		nil,
		testStruct{3.16, 3.15},
		nil,
		testStruct{3.15, 3.14},
	}

	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,
		edtSlice,
		0, 0, 0, 4,
		edtNil,
		edtReg, 0x13, 0x88,
		0x40, 0x4a, 0x3d, 0x71, // 3.16
		0x40, 0x9, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, // 3.15
		edtNil,
		edtReg, 0x13, 0x88,
		0x40, 0x49, 0x99, 0x9a, // 3.15
		0x40, 0x9, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // 3.14
	}

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeMap(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := map[int16]string{
		8: "hello",
		9: "world",
	}

	expect := []byte{edtType, 0, 3,
		edtMap,
		edtInt16,
		edtString,
		edtMap,
		0, 0, 0, 2,
		0, 8, // key 8
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		0, 9, // key 9
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
	}
	expect2 := []byte{edtType, 0, 3,
		edtMap,
		edtInt16,
		edtString,
		edtMap,
		0, 0, 0, 2,
		0, 9, // key 9
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		0, 8, // key 8
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		if !reflect.DeepEqual(b.B, expect2) {
			fmt.Printf("exp %#v\n", expect)
			fmt.Printf("got %#v\n", b.B)
			t.Fatal("incorrect value")
		}
	}
}

func TestEncodeMapAnyString(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := map[any]string{
		nil:      "hello",
		int16(9): "world",
	}
	expect := []byte{edtType, 0, 3,
		edtMap,
		edtAny,
		edtString,
		edtMap,
		0, 0, 0, 2,
		edtNil,
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		edtInt16, 0, 9, // key 9
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
	}
	expect2 := []byte{edtType, 0, 3,
		edtMap,
		edtAny,
		edtString,
		edtMap,
		0, 0, 0, 2,
		edtInt16, 0, 9, // key 9
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		edtNil,
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		if !reflect.DeepEqual(b.B, expect2) {
			fmt.Printf("exp1 %#v\n", expect)
			fmt.Printf("exp2 %#v\n", expect2)
			fmt.Printf("got  %#v\n", b.B)
			t.Fatal("incorrect value")
		}
	}
}

func TestEncodeMapStringAny(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := map[string]any{
		"hello": nil,
		"helloo": map[float32]any{
			3.16: uint16(3),
		},
	}
	expect := []byte{edtType, 0, 3,
		edtMap,
		edtString,
		edtAny,
		edtMap, 0, 0, 0, 2,

		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		edtNil,

		0, 6, // len of value "helloo"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x6f, // "helloo"
		edtType, 0, 3,
		edtMap, edtFloat32, edtAny,
		edtMap, 0, 0, 0, 1,
		0x40, 0x4a, 0x3d, 0x71, // 3.16
		edtUint16, 0, 3,
	}

	expect2 := []byte{edtType, 0, 3,
		edtMap,
		edtString,
		edtAny,
		edtMap, 0, 0, 0, 2,

		0, 6, // len of value "helloo"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x6f, // "helloo"
		edtType, 0, 3,
		edtMap, edtFloat32, edtAny,
		edtMap, 0, 0, 0, 1,
		0x40, 0x4a, 0x3d, 0x71, // 3.16
		edtUint16, 0, 3,

		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		edtNil,
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		if !reflect.DeepEqual(b.B, expect2) {
			fmt.Printf("exp1 %#v\n", expect)
			fmt.Printf("exp2 %#v\n", expect2)
			fmt.Printf("got  %#v\n", b.B)
			t.Fatal("incorrect value")
		}
	}

}

func TestEncodeMapStringMapNilZero(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := map[string]map[any]int16{
		"hello": nil,
		"world": {},
	}

	expect := []byte{edtType, 0, 5,
		edtMap,
		edtString,
		edtMap,
		edtAny,
		edtInt16,
		edtMap, 0, 0, 0, 2,

		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		edtNil,

		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		edtMap, 0, 0, 0, 0,
	}

	expect2 := []byte{edtType, 0, 5,
		edtMap,
		edtString,
		edtMap,
		edtAny,
		edtInt16,
		edtMap, 0, 0, 0, 2,

		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		edtMap, 0, 0, 0, 0,

		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		edtNil,
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		if !reflect.DeepEqual(b.B, expect2) {
			fmt.Printf("exp1 %#v\n", expect)
			fmt.Printf("exp2 %#v\n", expect2)
			fmt.Printf("got  %#v\n", b.B)
			t.Fatal("incorrect value")
		}
	}
}

func TestEncodeMap3DZero(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := map[int16]map[string]map[float32]int{}

	expect := []byte{edtType, 0, 7,
		edtMap,
		edtInt16,
		edtMap,
		edtString,
		edtMap,
		edtFloat32,
		edtInt,
		edtMap, 0, 0, 0, 0,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeMapZero(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := map[int16]string{}

	expect := []byte{edtType, 0, 3,
		edtMap,
		edtInt16, edtString,
		edtMap, 0, 0, 0, 0,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceMap(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := []map[int16]string{
		{
			8: "hello",
		}, {

			10: "helloo",
		},
		{
			12: "hellooo",
		},
	}

	expect := []byte{edtType, 0, 4,
		edtSlice,
		edtMap,
		edtInt16,
		edtString,
		edtSlice,
		0, 0, 0, 3,
		edtMap,
		0, 0, 0, 1,
		0, 8, // key 8
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		edtMap,
		0, 0, 0, 1, // len of second map
		0, 0xa, // key 10
		0, 6, // len of value "helloo"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x6f, // "helloo"
		edtMap,
		0, 0, 0, 1, // len of 3rd map
		0, 0xc, // key 12
		0, 7, // len of value "helloo"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x6f, 0x6f, // "hellooo"
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeMapValueSliceNil(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := map[int16][]any{
		int16(8): nil,
		int16(9): []any{"world"},
	}

	expect := []byte{edtType, 0, 4,
		edtMap,
		edtInt16,
		edtSlice,
		edtAny,
		edtMap,
		0, 0, 0, 2,

		0, 8, // key 8
		edtNil,

		0, 9, // key 9
		edtSlice,
		0, 0, 0, 1,
		edtString, 0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
	}
	expect2 := []byte{edtType, 0, 4,
		edtMap,
		edtInt16,
		edtSlice,
		edtAny,
		edtMap,
		0, 0, 0, 2,

		0, 9, // key 9
		edtSlice,
		0, 0, 0, 1,
		edtString, 0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"

		0, 8, // key 8
		edtNil,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		if !reflect.DeepEqual(b.B, expect2) {
			fmt.Printf("exp %#v\n", expect)
			fmt.Printf("got %#v\n", b.B)
			t.Fatal("incorrect value")
		}
	}
}

func TestEncodeMapValueMap(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := map[int16]map[string]int{
		int16(8): nil,
		int16(9): {
			"world": 10,
		},
	}

	expect := []byte{edtType, 0, 5,
		edtMap,
		edtInt16,
		edtMap,
		edtString,
		edtInt,
		edtMap,
		0, 0, 0, 2,

		0, 8, // key
		edtNil,

		0, 9, // 9 => map
		edtMap,
		0, 0, 0, 1,
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		0, 0, 0, 0, 0, 0, 0, 0xa, // key 10
	}
	expect2 := []byte{edtType, 0, 5,
		edtMap,
		edtInt16,
		edtMap,
		edtString,
		edtInt,
		edtMap,
		0, 0, 0, 2,

		0, 9, // 9 => map
		edtMap,
		0, 0, 0, 1,
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		0, 0, 0, 0, 0, 0, 0, 0xa, // key 10

		0, 8, // key
		edtNil,
	}
	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		if !reflect.DeepEqual(b.B, expect2) {
			fmt.Printf("exp1 %#v\n", expect)
			fmt.Printf("exp2 %#v\n", expect2)
			fmt.Printf("got  %#v\n", b.B)
			t.Fatal("incorrect value")
		}
	}
}

func TestEncodeMapValueMapRegKey(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var x testMapKey

	if err := RegisterTypeOf(x); err != nil {
		if err != gen.ErrTaken {
			t.Fatal(err)
		}
	}

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(x), []byte{edtReg, 0x13, 0x88})

	value := map[int16]map[testMapKey]int{
		int16(8): nil,
		int16(9): {
			"world": 10,
		},
	}

	expect := []byte{edtType, 0, 7,
		edtMap,
		edtInt16,
		edtMap,
		edtReg, 0x13, 0x88,
		edtInt,

		edtMap,
		0, 0, 0, 2,

		0, 8, // 8 => map
		edtNil,

		0, 9, // 9 => map
		edtMap,
		0, 0, 0, 1,
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		0, 0, 0, 0, 0, 0, 0, 0xa, // value 10
	}

	expect2 := []byte{edtType, 0, 7,
		edtMap,
		edtInt16,
		edtMap,
		edtReg, 0x13, 0x88,
		edtInt,

		edtMap,
		0, 0, 0, 2,

		0, 9, // 9 => map
		edtMap,
		0, 0, 0, 1,
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		0, 0, 0, 0, 0, 0, 0, 0xa, // value 10

		0, 8, // 8 => map
		edtNil,
	}

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		if !reflect.DeepEqual(b.B, expect2) {
			fmt.Printf("exp1 %#v\n", expect)
			fmt.Printf("exp2 %#v\n", expect2)
			fmt.Printf("got  %#v\n", b.B)
			t.Fatal("incorrect value")
		}
	}
}

func TestEncodeMapValueMapAnyWithRegKey(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var x testMapKey = "world"

	if err := RegisterTypeOf(x); err != nil {
		if err != gen.ErrTaken {
			t.Fatal(err)
		}
	}

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(x), []byte{edtReg, 0x13, 0x88})

	value := map[int16]map[any]int{
		int16(8): nil,
		int16(9): {
			x: 10,
		},
	}

	expect := []byte{edtType, 0, 5,
		edtMap,
		edtInt16,
		edtMap,
		edtAny,
		edtInt,

		edtMap,
		0, 0, 0, 2,

		0, 8, // 8 => map
		edtNil,

		0, 9, // 9 => map
		edtMap,
		0, 0, 0, 1,
		edtReg, 0x13, 0x88, 0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		0, 0, 0, 0, 0, 0, 0, 0xa, // value 10
	}

	expect2 := []byte{edtType, 0, 5,
		edtMap,
		edtInt16,
		edtMap,
		edtAny,
		edtInt,

		edtMap,
		0, 0, 0, 2,

		0, 9, // 9 => map
		edtMap,
		0, 0, 0, 1,
		edtReg, 0x13, 0x88, 0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		0, 0, 0, 0, 0, 0, 0, 0xa, // value 10

		0, 8, // 8 => map
		edtNil,
	}

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		if !reflect.DeepEqual(b.B, expect2) {
			fmt.Printf("exp1 %#v\n", expect)
			fmt.Printf("exp2 %#v\n", expect2)
			fmt.Printf("got  %#v\n", b.B)
			t.Fatal("incorrect value")
		}
	}
}

func TestEncodeRegMap(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var x MyMap

	if err := RegisterTypeOf(x); err != nil {
		if err != gen.ErrTaken {
			t.Fatal(err)
		}
	}

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(x), []byte{edtReg, 0x13, 0x88})

	value := MyMap{
		"hello": true,
		"world": false,
	}

	expect := []byte{edtReg, 0x13, 0x88,
		edtReg,
		0, 0, 0, 2,
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		0,    // false
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		1, // true
	}

	expect2 := []byte{edtReg, 0x13, 0x88,
		edtReg,
		0, 0, 0, 2,
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		1,    // true
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		0, // false
	}

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		if !reflect.DeepEqual(b.B, expect2) {
			fmt.Printf("exp1 %#v\n", expect)
			fmt.Printf("exp2 %#v\n", expect2)
			fmt.Printf("got  %#v\n", b.B)
			t.Fatal("incorrect value")
		}
	}
}

func TestEncodeRegMapRegSlice(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	type mySlice90 []bool
	type myMap90 map[string]mySlice90

	RegisterTypeOf(mySlice90{})
	RegisterTypeOf(myMap90{})

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(myMap90{}), []byte{edtReg, 0x13, 0x88})

	value := myMap90{
		"world": nil,
		"hello": {true, false, true},
	}

	expect := []byte{edtReg, 0x13, 0x88,
		edtReg,
		0, 0, 0, 2,
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		edtReg,
		0, 0, 0, 3,
		1, 0, 1, // true, false, true
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		edtNil,
	}

	expect2 := []byte{edtReg, 0x13, 0x88,
		edtReg,
		0, 0, 0, 2,
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		edtNil,
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		edtReg,
		0, 0, 0, 3,
		1, 0, 1, // true, false, true
	}

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		if !reflect.DeepEqual(b.B, expect2) {
			fmt.Printf("exp1 %#v\n", expect)
			fmt.Printf("exp2 %#v\n", expect2)
			fmt.Printf("got  %#v\n", b.B)
			t.Fatal("incorrect value")
		}
	}
}

func TestEncodeArray(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := [2]string{
		"hello", "world",
	}
	expect := []byte{edtType, 0, 6,
		edtArray, 0, 0, 0, 2,
		edtString,
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeArrayZero(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := [0]string{}
	expect := []byte{edtType, 0, 6,
		edtArray, 0, 0, 0, 0,
		edtString,
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceArray(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := [][2]string{
		{"hello", "world"},
	}
	expect := []byte{edtType, 0, 7,
		edtSlice,
		edtArray, 0, 0, 0, 2,
		edtString,
		edtSlice,
		0, 0, 0, 1,
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSliceAnyArray(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	value := []any{
		nil,
		[2]string{"hello", "world"},
		nil,
	}
	expect := []byte{edtType, 0, 2,
		edtSlice,
		edtAny,

		edtSlice,
		0, 0, 0, 3,

		edtNil,

		edtType, 0, 6,
		edtArray, 0, 0, 0, 2,
		edtString,
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"

		edtNil,
	}

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

type myArrayEnc [2]string

func TestEncodeRegArray(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := myArrayEnc{"hello", "world"}

	expect := []byte{edtReg,
		0, 38, // len of the type name #ergo.services/ergo/net/edf/myArrayEnc
		0x23, 0x65, 0x72, 0x67, 0x6f, 0x2e, 0x73, 0x65,
		0x72, 0x76, 0x69, 0x63, 0x65, 0x73, 0x2f, 0x65,
		0x72, 0x67, 0x6f, 0x2f, 0x6e, 0x65, 0x74, 0x2f,
		0x65, 0x64, 0x66, 0x2f, 0x6d, 0x79, 0x41, 0x72,
		0x72, 0x61, 0x79, 0x45, 0x6e, 0x63,

		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
	}

	RegisterTypeOf(value)

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

type myArrayStr string

func TestEncodeArrayReg(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	value := [2]myArrayStr{"hello", "world"}

	expect := []byte{edtType, 0, 46,
		edtArray, 0, 0, 0, 2,
		edtReg,
		0, 38, // len of the type name #ergo.services/ergo/net/edf/myArrayStr
		0x23, 0x65, 0x72, 0x67, 0x6f, 0x2e, 0x73, 0x65,
		0x72, 0x76, 0x69, 0x63, 0x65, 0x73, 0x2f, 0x65,
		0x72, 0x67, 0x6f, 0x2f, 0x6e, 0x65, 0x74, 0x2f,
		0x65, 0x64, 0x66, 0x2f, 0x6d, 0x79, 0x41, 0x72,
		0x72, 0x61, 0x79, 0x53, 0x74, 0x72,

		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
	}

	RegisterTypeOf(value[0])

	if err := Encode(value, b, Options{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}
func TestEncodeRegArrayRegArray(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	type myArrayMyStr1 [2]string
	type myArrayArray1 [3]myArrayMyStr1

	value := myArrayArray1{
		{"hello", "world"},
		{"", ""},
		{"world", "hello"},
	}

	expect := []byte{edtReg, 0x13, 0x88,
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		0, 0,
		0, 0,
		0, 5, // len of value "world"
		0x77, 0x6f, 0x72, 0x6c, 0x64, // "world"
		0, 5, // len of value "hello"
		0x68, 0x65, 0x6c, 0x6c, 0x6f, // "hello"
	}

	regCache := new(sync.Map)
	regCache.Store(reflect.TypeOf(myArrayArray1{}), []byte{edtReg, 0x13, 0x88})

	RegisterTypeOf(myArrayMyStr1{})
	RegisterTypeOf(myArrayArray1{})

	if err := Encode(value, b, Options{RegCache: regCache}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expect) {
		fmt.Printf("exp %#v\n", expect)
		fmt.Printf("got %#v\n", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeStructWithMap(t *testing.T) {
	// there was a bug with such kind of data
	type BugInfo struct {
		Env     map[gen.Env]any
		Loggers []gen.LoggerInfo
	}
	in := BugInfo{
		Env: map[gen.Env]any{
			"x": "y",
		},
		Loggers: []gen.LoggerInfo{
			gen.LoggerInfo{},
		},
	}
	if err := RegisterTypeOf(in); err != nil {
		panic(err)
	}

	b := lib.TakeBuffer()
	if err := Encode(in, b, Options{}); err != nil {
		t.Fatal(err)
	}
	value, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(value, in) {
		fmt.Println("exp", in)
		fmt.Println("got", value)
		t.Fatal("incorrect value")
	}
}
