package etf

import (
	"context"
	"fmt"
	"math/big"
	"reflect"
	"testing"

	"github.com/halturin/ergo/lib"
)

func TestEncodeBool(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	err := Encode(false, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, []byte{ettSmallAtom, 5, 'f', 'a', 'l', 's', 'e'}) {
		t.Fatal("incorrect value")
	}
}

func TestEncodeBoolWithAtomCache(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writerAtomCache := make(map[Atom]CacheItem)
	encodingAtomCache := TakeListAtomCache()
	defer ReleaseListAtomCache(encodingAtomCache)
	linkAtomCache := NewAtomCache(ctx)
	ci := CacheItem{ID: 499, Encoded: true, Name: "false"}

	writerAtomCache["false"] = ci

	err := Encode(false, b, linkAtomCache, writerAtomCache, encodingAtomCache)
	if err != nil {
		t.Fatal(err)
	}

	if encodingAtomCache.Len() != 1 || encodingAtomCache.L[0] != ci {
		t.Fatal("incorrect cache value")
	}

	if !reflect.DeepEqual(b.B, []byte{ettCacheRef, 0}) {
		t.Fatal("incorrect value")
	}

}

type integerCase struct {
	name     string
	integer  interface{}
	expected []byte
}

func integerCases() []integerCase {
	bigInt := big.Int{}
	bigInt.SetString("9223372036854775807123456789", 10)
	bigIntNegative := big.Int{}
	bigIntNegative.SetString("-9223372036854775807123456789", 10)

	return []integerCase{
		//
		// unsigned integers
		//
		integerCase{"uint8::255", uint8(255), []byte{ettSmallInteger, 255}},
		integerCase{"uint16::255", uint16(255), []byte{ettSmallInteger, 255}},
		integerCase{"uint32::255", uint32(255), []byte{ettSmallInteger, 255}},
		integerCase{"uint64::255", uint64(255), []byte{ettSmallInteger, 255}},
		integerCase{"uint::255", uint(255), []byte{ettSmallInteger, 255}},

		integerCase{"uint16::256", uint16(256), []byte{ettInteger, 0, 0, 1, 0}},

		integerCase{"uint16::65535", uint16(65535), []byte{ettInteger, 0, 0, 255, 255}},
		integerCase{"uint32::65535", uint32(65535), []byte{ettInteger, 0, 0, 255, 255}},
		integerCase{"uint64::65535", uint64(65535), []byte{ettInteger, 0, 0, 255, 255}},

		integerCase{"uint64::65536", uint64(65536), []byte{ettInteger, 0, 1, 0, 0}},

		// treat as an int32
		integerCase{"uint32::2147483647", uint32(2147483647), []byte{ettInteger, 127, 255, 255, 255}},
		integerCase{"uint64::2147483647", uint64(2147483647), []byte{ettInteger, 127, 255, 255, 255}},
		integerCase{"uint64::2147483648", uint64(2147483648), []byte{ettSmallBig, 4, 0, 0, 0, 0, 128}},

		integerCase{"uint32::4294967295", uint32(4294967295), []byte{ettSmallBig, 4, 0, 255, 255, 255, 255}},
		integerCase{"uint64::4294967295", uint64(4294967295), []byte{ettSmallBig, 4, 0, 255, 255, 255, 255}},
		integerCase{"uint64::4294967296", uint64(4294967296), []byte{ettSmallBig, 5, 0, 0, 0, 0, 0, 1}},

		integerCase{"uint64::18446744073709551615", uint64(18446744073709551615), []byte{ettSmallBig, 8, 0, 255, 255, 255, 255, 255, 255, 255, 255}},

		//
		// signed integers
		//

		// negative is always ettInteger for the numbers within the range of int32
		integerCase{"int8::-127", int8(-127), []byte{ettInteger, 255, 255, 255, 129}},
		integerCase{"int16::-127", int16(-127), []byte{ettInteger, 255, 255, 255, 129}},
		integerCase{"int32::-127", int32(-127), []byte{ettInteger, 255, 255, 255, 129}},
		integerCase{"int64::-127", int64(-127), []byte{ettInteger, 255, 255, 255, 129}},
		integerCase{"int::-127", int(-127), []byte{ettInteger, 255, 255, 255, 129}},

		// positive within range of int8 treats as ettSmallInteger
		integerCase{"int8::127", int8(127), []byte{ettSmallInteger, 127}},
		integerCase{"int16::127", int16(127), []byte{ettSmallInteger, 127}},
		integerCase{"int32::127", int32(127), []byte{ettSmallInteger, 127}},
		integerCase{"int64::127", int64(127), []byte{ettSmallInteger, 127}},

		// a positive int[16,32,64] value within the range of uint8 treats as an uint8
		integerCase{"int16::128", int16(128), []byte{ettSmallInteger, 128}},
		integerCase{"int32::128", int32(128), []byte{ettSmallInteger, 128}},
		integerCase{"int64::128", int64(128), []byte{ettSmallInteger, 128}},
		integerCase{"int::128", int(128), []byte{ettSmallInteger, 128}},

		// whether its positive or negative value within the range of int16 its treating as an int32
		integerCase{"int16::-32767", int16(-32767), []byte{ettInteger, 255, 255, 128, 1}},
		integerCase{"int16::32767", int16(32767), []byte{ettInteger, 0, 0, 127, 255}},

		// treat as an int32
		integerCase{"int32::2147483647", int32(2147483647), []byte{ettInteger, 127, 255, 255, 255}},
		integerCase{"int32::-2147483648", int32(-2147483648), []byte{ettInteger, 128, 0, 0, 0}},
		integerCase{"int64::2147483647", int64(2147483647), []byte{ettInteger, 127, 255, 255, 255}},
		integerCase{"int64::-2147483648", int64(-2147483648), []byte{ettInteger, 128, 0, 0, 0}},

		integerCase{"int64::2147483648", int64(2147483648), []byte{ettSmallBig, 4, 0, 0, 0, 0, 128}},

		// int64 treats as ettSmallBig whether its positive or negative
		integerCase{"int64::9223372036854775807", int64(9223372036854775807), []byte{ettSmallBig, 8, 0, 255, 255, 255, 255, 255, 255, 255, 127}},
		integerCase{"int64::-9223372036854775808", int64(-9223372036854775808), []byte{ettSmallBig, 8, 1, 0, 0, 0, 0, 0, 0, 0, 128}},

		integerCase{"big.int::-9223372036854775807123456789", bigIntNegative, []byte{ettSmallBig, 12, 1, 21, 3, 193, 203, 255, 255, 255, 255, 255, 100, 205, 29}},
	}
}

func TestEncodeInteger(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	for _, c := range integerCases() {
		t.Run(c.name, func(t *testing.T) {
			b.Reset()

			err := Encode(c.integer, b, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(b.B, c.expected) {
				fmt.Println("exp ", c.expected)
				fmt.Println("got ", b.B)
				t.Fatal("incorrect value")
			}
		})
	}
}

func TestEncodeFloat(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expected := []byte{ettNewFloat, 64, 9, 30, 184, 81, 235, 133, 31}

	err := Encode(float64(3.14), b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}

	b.Reset()
	err = Encode(float32(3.14), b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// float32 to float64 casting makes some changes, thats why 'expected'
	// has different set of bytes
	expected = []byte{ettNewFloat, 64, 9, 30, 184, 96, 0, 0, 0}
	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodeString(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expected := []byte{ettString, 0, 14, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107}

	err := Encode("Ergo Framework", b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeAtom(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expected := []byte{ettSmallAtomUTF8, 14, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107}

	err := Encode(Atom("Ergo Framework"), b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}

	b.Reset()

	longAtom := Atom("Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework Ergo Framework")
	expected = []byte{ettAtomUTF8, 1, 43, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107, 32, 69, 114, 103, 111, 32, 70, 114, 97, 109, 101, 119,
		111, 114, 107}
	err = Encode(longAtom, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeAtomWithCache(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writerAtomCache := make(map[Atom]CacheItem)
	encodingAtomCache := TakeListAtomCache()
	defer ReleaseListAtomCache(encodingAtomCache)
	linkAtomCache := NewAtomCache(ctx)

	ci := CacheItem{ID: 2020, Encoded: true, Name: "cached atom"}
	writerAtomCache["cached atom"] = ci

	err := Encode(Atom("cached atom"), b, linkAtomCache, writerAtomCache, encodingAtomCache)
	if err != nil {
		t.Fatal(err)
	}

	if encodingAtomCache.Len() != 1 || encodingAtomCache.L[0] != ci {
		t.Fatal("incorrect cache value")
	}

	if !reflect.DeepEqual(b.B, []byte{ettCacheRef, 0}) {
		t.Fatal("incorrect value")
	}

	b.Reset()

	err = Encode(Atom("not cached atom"), b, linkAtomCache, writerAtomCache, encodingAtomCache)
	if err != nil {
		t.Fatal(err)
	}

	if encodingAtomCache.Len() != 1 || encodingAtomCache.L[0] != ci {
		t.Fatal("incorrect cache value")
	}

	expected := []byte{ettSmallAtomUTF8, 15, 110, 111, 116, 32, 99, 97, 99, 104, 101, 100, 32, 97, 116, 111, 109}
	if !reflect.DeepEqual(b.B, expected) {
		t.Fatal("incorrect value")
	}
}

func TestEncodeBinary(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	err := Encode([]byte{1, 2, 3, 4, 5}, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	expected := []byte{ettBinary, 0, 0, 0, 5, 1, 2, 3, 4, 5}
	if !reflect.DeepEqual(b.B, expected) {
		t.Fatal("incorrect value")
	}
}

func TestEncodeList(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expected := []byte{ettList, 0, 0, 0, 3, ettSmallAtomUTF8, 1, 97, ettSmallInteger, 2, ettSmallInteger, 3, ettNil}
	term := List{Atom("a"), 2, 3}
	err := Encode(term, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeSlice(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	expected := []byte{108, 0, 0, 0, 4, 98, 0, 0, 48, 57, 98, 0, 1, 9, 50, 98, 0, 0, 48, 57,
		98, 0, 1, 9, 50, 106}
	//expected := []byte{ettList, 0, 0, 0, 3, ettSmallAtomUTF8, 1, 97, ettSmallInteger, 2, ettSmallInteger, 3, ettNil}
	term := []int{12345, 67890, 12345, 67890}
	err := Encode(term, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}
func TestEncodeListNested(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	expected := []byte{108, 0, 0, 0, 2, 119, 1, 97, 108, 0, 0, 0, 4, 119, 1, 98, 97, 2, 108,
		0, 0, 0, 2, 119, 1, 99, 97, 3, 106, 97, 4, 106, 106}

	term := List{Atom("a"), List{Atom("b"), 2, List{Atom("c"), 3}, 4}}
	err := Encode(term, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeTupleNested(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	expected := []byte{104, 2, 119, 1, 97, 104, 4, 119, 1, 98, 97, 2, 104, 2, 119, 1, 99,
		97, 3, 97, 4}

	term := Tuple{Atom("a"), Tuple{Atom("b"), 2, Tuple{Atom("c"), 3}, 4}}
	err := Encode(term, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeTuple(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expected := []byte{ettSmallTuple, 3, ettSmallAtomUTF8, 1, 97, ettSmallInteger, 2, ettSmallInteger, 3}
	term := Tuple{Atom("a"), 2, 3}
	err := Encode(term, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeMap(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	// map has no guarantee of key order, so the result could be different
	expected := []byte{116, 0, 0, 0, 2, 119, 4, 107, 101, 121, 49, 98, 0, 0, 48, 57, 119, 4,
		107, 101, 121, 50, 107, 0, 11, 104, 101, 108, 108, 111, 32, 119, 111,
		114, 108, 100}
	expected1 := []byte{116, 0, 0, 0, 2, 119, 4, 107, 101, 121, 50, 107, 0, 11, 104, 101,
		108, 108, 111, 32, 119, 111, 114, 108, 100, 119, 4, 107, 101, 121, 49, 98, 0, 0,
		48, 57}
	term := Map{
		Atom("key1"): 12345,
		Atom("key2"): "hello world",
	}

	err := Encode(term, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) && !reflect.DeepEqual(b.B, expected1) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeGoMap(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	// map has no guarantee of key order, so the result could be different
	expected := []byte{116, 0, 0, 0, 2, 119, 4, 107, 101, 121, 49, 98, 0, 0, 48, 57, 119, 4,
		107, 101, 121, 50, 107, 0, 11, 104, 101, 108, 108, 111, 32, 119, 111,
		114, 108, 100}
	expected1 := []byte{116, 0, 0, 0, 2, 119, 4, 107, 101, 121, 50, 107, 0, 11, 104, 101,
		108, 108, 111, 32, 119, 111, 114, 108, 100, 119, 4, 107, 101, 121, 49, 98, 0, 0,
		48, 57}
	term := map[Atom]interface{}{
		Atom("key1"): 12345,
		Atom("key2"): "hello world",
	}

	err := Encode(term, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) && !reflect.DeepEqual(b.B, expected1) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeGoStruct(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expected := []byte{116, 0, 0, 0, 3, 119, 15, 83, 116, 114, 117, 99, 116, 84, 111, 77, 97, 112, 75, 101, 121, 49, 98, 0, 0, 48, 57, 119, 15, 83, 116, 114, 117, 99, 116, 84, 111, 77, 97, 112, 75, 101, 121, 50, 107, 0, 22, 112, 111, 105, 110, 116, 101, 114, 32, 116, 111, 32, 104, 101, 108, 108, 111, 32, 119, 111, 114, 108, 100, 119, 15, 83, 116, 114, 117, 99, 116, 84, 111, 77, 97, 112, 75, 101, 121, 51, 107, 0, 11, 104, 101, 108, 108, 111, 32, 119, 111, 114, 108, 100}

	s := "pointer to hello world"
	term := struct {
		StructToMapKey1 int
		StructToMapKey2 string
		StructToMapKey3 string
	}{
		StructToMapKey1: 12345,
		StructToMapKey2: s,
		StructToMapKey3: "hello world",
	}

	err := Encode(term, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
	b1 := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b1)

	term1 := struct {
		StructToMapKey1 int
		StructToMapKey2 *string
		StructToMapKey3 string
	}{
		StructToMapKey1: 12345,
		StructToMapKey2: &s,
		StructToMapKey3: "hello world",
	}

	err = Encode(term1, b1, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b1.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b1.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeGoStructWithNestedPointers(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	type Nested struct {
		Key1 string
		Key2 *string
		Key3 int
		Key4 *int
		Key5 float64
		Key6 *float64
		Key7 bool
		Key8 *bool
	}
	type Tst struct {
		Nested
		Key9 *Nested
	}
	ValueString := "hello world"
	ValueInt := 123
	ValueFloat := 3.14
	ValueBool := true

	nested := Nested{
		Key1: ValueString,
		Key2: &ValueString,
		Key3: ValueInt,
		Key4: &ValueInt,
		Key5: ValueFloat,
		Key6: &ValueFloat,
		Key7: ValueBool,
		Key8: &ValueBool,
	}
	term := Tst{
		Nested: nested,
		Key9:   &nested,
	}

	expected := []byte{116, 0, 0, 0, 2, 119, 6, 78, 101, 115, 116, 101, 100, 116, 0, 0, 0, 8, 119, 4, 75, 101, 121, 49, 107, 0, 11, 104, 101, 108, 108, 111, 32, 119, 111, 114, 108, 100, 119, 4, 75, 101, 121, 50, 107, 0, 11, 104, 101, 108, 108, 111, 32, 119, 111, 114, 108, 100, 119, 4, 75, 101, 121, 51, 97, 123, 119, 4, 75, 101, 121, 52, 97, 123, 119, 4, 75, 101, 121, 53, 70, 64, 9, 30, 184, 81, 235, 133, 31, 119, 4, 75, 101, 121, 54, 70, 64, 9, 30, 184, 81, 235, 133, 31, 119, 4, 75, 101, 121, 55, 115, 4, 116, 114, 117, 101, 119, 4, 75, 101, 121, 56, 115, 4, 116, 114, 117, 101, 119, 4, 75, 101, 121, 57, 116, 0, 0, 0, 8, 119, 4, 75, 101, 121, 49, 107, 0, 11, 104, 101, 108, 108, 111, 32, 119, 111, 114, 108, 100, 119, 4, 75, 101, 121, 50, 107, 0, 11, 104, 101, 108, 108, 111, 32, 119, 111, 114, 108, 100, 119, 4, 75, 101, 121, 51, 97, 123, 119, 4, 75, 101, 121, 52, 97, 123, 119, 4, 75, 101, 121, 53, 70, 64, 9, 30, 184, 81, 235, 133, 31, 119, 4, 75, 101, 121, 54, 70, 64, 9, 30, 184, 81, 235, 133, 31, 119, 4, 75, 101, 121, 55, 115, 4, 116, 114, 117, 101, 119, 4, 75, 101, 121, 56, 115, 4, 116, 114, 117, 101}

	err := Encode(term, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}

	b1 := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b1)
	termWithNil := Tst{
		Nested: nested,
	}
	expectedWithNil := []byte{116, 0, 0, 0, 2, 119, 6, 78, 101, 115, 116, 101, 100, 116, 0, 0, 0, 8, 119, 4, 75, 101, 121, 49, 107, 0, 11, 104, 101, 108, 108, 111, 32, 119, 111, 114, 108, 100, 119, 4, 75, 101, 121, 50, 107, 0, 11, 104, 101, 108, 108, 111, 32, 119, 111, 114, 108, 100, 119, 4, 75, 101, 121, 51, 97, 123, 119, 4, 75, 101, 121, 52, 97, 123, 119, 4, 75, 101, 121, 53, 70, 64, 9, 30, 184, 81, 235, 133, 31, 119, 4, 75, 101, 121, 54, 70, 64, 9, 30, 184, 81, 235, 133, 31, 119, 4, 75, 101, 121, 55, 115, 4, 116, 114, 117, 101, 119, 4, 75, 101, 121, 56, 115, 4, 116, 114, 117, 101, 119, 4, 75, 101, 121, 57, 106}

	err = Encode(termWithNil, b1, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b1.B, expectedWithNil) {
		fmt.Println("exp", expectedWithNil)
		fmt.Println("got", b1.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodePid(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expected := []byte{103, 119, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64, 49, 50,
		55, 46, 48, 46, 48, 46, 49, 0, 0, 1, 56, 0, 0, 0, 0, 2}
	term := Pid{Node: "erl-demo@127.0.0.1", ID: 312, Serial: 0, Creation: 2}

	err := Encode(term, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodePidWithAtomCache(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expected := []byte{103, 82, 0, 0, 0, 1, 56, 0, 0, 0, 0, 2}
	term := Pid{Node: "erl-demo@127.0.0.1", ID: 312, Serial: 0, Creation: 2}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writerAtomCache := make(map[Atom]CacheItem)
	encodingAtomCache := TakeListAtomCache()
	defer ReleaseListAtomCache(encodingAtomCache)
	linkAtomCache := NewAtomCache(ctx)

	ci := CacheItem{ID: 2020, Encoded: true, Name: "erl-demo@127.0.0.1"}
	writerAtomCache["erl-demo@127.0.0.1"] = ci

	err := Encode(term, b, linkAtomCache, writerAtomCache, encodingAtomCache)
	if err != nil {
		t.Fatal(err)
	}

	if encodingAtomCache.Len() != 1 || encodingAtomCache.L[0] != ci {
		t.Fatal("incorrect cache value")
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodeRef(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expected := []byte{114, 0, 3, 119, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64,
		49, 50, 55, 46, 48, 46, 48, 46, 49, 2, 0, 1, 30, 228, 183, 192, 0, 1, 141,
		122, 203, 35}

	term := Ref{
		Node:     Atom("erl-demo@127.0.0.1"),
		Creation: 2,
		ID:       []uint32{73444, 3082813441, 2373634851},
	}

	err := Encode(term, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}

}

func TestEncodeTupleRefPid(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	expected := []byte{ettSmallTuple, 2, ettNewRef, 0, 3, ettSmallAtomUTF8, 18, 101, 114, 108, 45, 100, 101, 109,
		111, 64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 2, 0, 1, 31, 28, 183, 192, 0,
		1, 141, 122, 203, 35, 103, ettSmallAtomUTF8, 18, 101, 114, 108, 45, 100, 101,
		109, 111, 64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 1, 56, 0, 0, 0, 0,
		2}

	term := Tuple{
		Ref{
			Node:     Atom("erl-demo@127.0.0.1"),
			Creation: 2,
			ID:       []uint32{0x11f1c, 0xb7c00001, 0x8d7acb23}},
		Pid{
			Node:     Atom("erl-demo@127.0.0.1"),
			ID:       312,
			Serial:   0,
			Creation: 2}}

	err := Encode(term, b, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func TestEncodeGoPtrNil(t *testing.T) {
	var x *int
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	err := Encode(x, b, nil, nil, nil)

	if err != nil {
		t.Fatal(err)
	}
	expected := []byte{ettNil}
	if !reflect.DeepEqual(b.B, expected) {
		fmt.Println("exp", expected)
		fmt.Println("got", b.B)
		t.Fatal("incorrect value")
	}
}

func BenchmarkEncodeBool(b *testing.B) {

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(false, buf, nil, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeBoolWithAtomCache(b *testing.B) {

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writerAtomCache := make(map[Atom]CacheItem)
	encodingAtomCache := TakeListAtomCache()
	defer ReleaseListAtomCache(encodingAtomCache)
	linkAtomCache := NewAtomCache(ctx)

	writerAtomCache["false"] = CacheItem{ID: 499, Encoded: true, Name: "false"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(false, buf, linkAtomCache, writerAtomCache, encodingAtomCache)
		encodingAtomCache.Reset()
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeInteger(b *testing.B) {
	for _, c := range integerCases() {
		b.Run(c.name, func(b *testing.B) {
			buf := lib.TakeBuffer()
			defer lib.ReleaseBuffer(buf)

			for i := 0; i < b.N; i++ {
				err := Encode(c.integer, buf, nil, nil, nil)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkEncodeFloat32(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(float32(3.14), buf, nil, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeFloat64(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(float64(3.14), buf, nil, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeString(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode("Ergo Framework", buf, nil, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeAtom(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(Atom("Ergo Framework"), buf, nil, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeAtomWithCache(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writerAtomCache := make(map[Atom]CacheItem)
	encodingAtomCache := TakeListAtomCache()
	defer ReleaseListAtomCache(encodingAtomCache)
	linkAtomCache := NewAtomCache(ctx)

	ci := CacheItem{ID: 2020, Encoded: true, Name: "cached atom"}
	writerAtomCache["cached atom"] = ci

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(Atom("cached atom"), buf, linkAtomCache, writerAtomCache, encodingAtomCache)
		buf.Reset()
		encodingAtomCache.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}

}

func BenchmarkEncodeBinary(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)
	bytes := []byte{1, 2, 3, 4, 5}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(bytes, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeList(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := List{Atom("a"), 2, 3}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}

}

func BenchmarkEncodeListNested(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := List{Atom("a"), List{Atom("b"), 2, List{Atom("c"), 3}, 4}}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}

}

func BenchmarkEncodeTuple(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := Tuple{Atom("a"), 2, 3}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}

}

func BenchmarkEncodeTupleNested(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := Tuple{Atom("a"), Tuple{Atom("b"), 2, Tuple{Atom("c"), 3}, 4}}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}

}

func BenchmarkEncodeSlice(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := []int{12345, 67890, 12345, 67890}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeArray(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := [4]int{12345, 67890, 12345, 67890}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMap(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := Map{
		Atom("key1"): 12345,
		Atom("key2"): "hello world",
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeGoMap(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := map[Atom]interface{}{
		Atom("key1"): 12345,
		Atom("key2"): "hello world",
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeGoStruct(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := struct {
		StructToMapKey1 int
		StructToMapKey2 string
	}{
		StructToMapKey1: 12345,
		StructToMapKey2: "hello world",
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodePid(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := Pid{Node: "erl-demo@127.0.0.1", ID: 312, Serial: 0, Creation: 2}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodePidWithAtomCache(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := Pid{Node: "erl-demo@127.0.0.1", ID: 312, Serial: 0, Creation: 2}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writerAtomCache := make(map[Atom]CacheItem)
	encodingAtomCache := TakeListAtomCache()
	defer ReleaseListAtomCache(encodingAtomCache)
	linkAtomCache := NewAtomCache(ctx)

	ci := CacheItem{ID: 2020, Encoded: true, Name: "erl-demo@127.0.0.1"}
	writerAtomCache["erl-demo@127.0.0.1"] = ci

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, linkAtomCache, writerAtomCache, encodingAtomCache)
		buf.Reset()
		encodingAtomCache.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeRef(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := Ref{
		Node:     Atom("erl-demo@127.0.0.1"),
		Creation: 2,
		ID:       []uint32{73444, 3082813441, 2373634851},
	}

	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeRefWithAtomCache(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := Ref{
		Node:     Atom("erl-demo@127.0.0.1"),
		Creation: 2,
		ID:       []uint32{73444, 3082813441, 2373634851},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writerAtomCache := make(map[Atom]CacheItem)
	encodingAtomCache := TakeListAtomCache()
	defer ReleaseListAtomCache(encodingAtomCache)
	linkAtomCache := NewAtomCache(ctx)

	ci := CacheItem{ID: 2020, Encoded: true, Name: "erl-demo@127.0.0.1"}
	writerAtomCache["erl-demo@127.0.0.1"] = ci

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, linkAtomCache, writerAtomCache, encodingAtomCache)
		buf.Reset()
		encodingAtomCache.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeTupleRefPid(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := Tuple{
		Ref{
			Node:     Atom("erl-demo@127.0.0.1"),
			Creation: 2,
			ID:       []uint32{0x11f1c, 0xb7c00001, 0x8d7acb23}},
		Pid{
			Node:     Atom("erl-demo@127.0.0.1"),
			ID:       312,
			Serial:   0,
			Creation: 2}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, nil, nil, nil)
		buf.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeTupleRefPidWithAtomCache(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	term := Tuple{
		Ref{
			Node:     Atom("erl-demo@127.0.0.1"),
			Creation: 2,
			ID:       []uint32{0x11f1c, 0xb7c00001, 0x8d7acb23}},
		Pid{
			Node:     Atom("erl-demo@127.0.0.1"),
			ID:       312,
			Serial:   0,
			Creation: 2}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writerAtomCache := make(map[Atom]CacheItem)
	encodingAtomCache := TakeListAtomCache()
	defer ReleaseListAtomCache(encodingAtomCache)
	linkAtomCache := NewAtomCache(ctx)

	ci := CacheItem{ID: 2020, Encoded: true, Name: "erl-demo@127.0.0.1"}
	writerAtomCache["erl-demo@127.0.0.1"] = ci

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Encode(term, buf, linkAtomCache, writerAtomCache, encodingAtomCache)
		buf.Reset()
		encodingAtomCache.Reset()
		if err != nil {
			b.Fatal(err)
		}
	}
}
