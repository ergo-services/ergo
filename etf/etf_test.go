package etf

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/ergo-services/ergo/lib"
)

func TestTermIntoStruct_Slice(t *testing.T) {
	dest := []byte{}

	tests := []struct {
		want []byte
		term Term
	}{
		{[]byte{1, 2, 3}, List{1, 2, 3}},
	}

	for _, tt := range tests {
		if err := TermIntoStruct(tt.term, &dest); err != nil {
			t.Errorf("%#v: conversion failed: %v", tt.term, err)
		}

		if !bytes.Equal(dest, tt.want) {
			t.Errorf("%#v: got %v, want %v", tt.term, dest, tt.want)
		}
	}
	tests1 := []struct {
		want [][]float32
		term Term
	}{
		{[][]float32{[]float32{1.23, 2.34, 3.45}, []float32{4.56, 5.67, 6.78}, []float32{7.89, 8.91, 9.12}}, List{List{1.23, 2.34, 3.45}, List{4.56, 5.67, 6.78}, List{7.89, 8.91, 9.12}}},
	}
	dest1 := [][]float32{}

	for _, tt := range tests1 {
		if err := TermIntoStruct(tt.term, &dest1); err != nil {
			t.Errorf("%#v: conversion failed: %v", tt.term, err)
		}

		if !reflect.DeepEqual(dest1, tt.want) {
			t.Errorf("%#v: got %v, want %v", tt.term, dest1, tt.want)
		}
	}
}

func TestTermIntoStruct_Array(t *testing.T) {
	dest := [3]byte{}

	tests := []struct {
		want [3]byte
		term Term
	}{
		{[...]byte{1, 2, 3}, List{1, 2, 3}},
	}

	for _, tt := range tests {
		if err := TermIntoStruct(tt.term, &dest); err != nil {
			t.Errorf("%#v: conversion failed: %v", tt.term, err)
		}

		if dest != tt.want {
			t.Errorf("%#v: got %v, want %v", tt.term, dest, tt.want)
		}
	}

	tests1 := []struct {
		want [3][3]float64
		term Term
	}{
		{[3][3]float64{[...]float64{1.23, 2.34, 3.45}, [...]float64{4.56, 5.67, 6.78}, [...]float64{7.89, 8.91, 9.12}}, List{List{1.23, 2.34, 3.45}, List{4.56, 5.67, 6.78}, List{7.89, 8.91, 9.12}}},
	}
	dest1 := [3][3]float64{}

	for _, tt := range tests1 {
		if err := TermIntoStruct(tt.term, &dest1); err != nil {
			t.Errorf("%#v: conversion failed: %v", tt.term, err)
		}

		if !reflect.DeepEqual(dest1, tt.want) {
			t.Errorf("%#v: got %v, want %v", tt.term, dest1, tt.want)
		}
	}
}

func TestTermIntoStruct_Struct(t *testing.T) {
	type testAA struct {
		A []bool
		B uint32
		C string
	}

	type testStruct struct {
		AA testAA
		BB float64
		CC *testStruct
	}
	type testItem struct {
		Want testStruct
		Term Term
	}

	dest := testStruct{}
	tests := []testItem{
		testItem{
			Want: testStruct{
				AA: testAA{
					A: []bool{true, false, false, true, false},
					B: 8765,
					C: "test value",
				},
				BB: 3.13,
				CC: &testStruct{
					BB: 4.14,
					CC: &testStruct{
						AA: testAA{
							A: []bool{false, true},
							B: 5,
						},
					},
				},
			},
			Term: Tuple{ //testStruct
				Tuple{ // AA testAA
					List{true, false, false, true, false}, // A []bool
					8765,                                  // B uint32
					"test value",                          // C string
				},
				3.13, // BB float64
				Tuple{ // CC *testStruct
					Tuple{}, // AA testAA (empty)
					4.14,    // BB float64
					Tuple{ // CC *testStruct
						Tuple{ // AA testAA
							List{false, true}, // A []bool
							5,                 // B uint32
							// C string (empty)
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		if err := TermIntoStruct(tt.Term, &dest); err != nil {
			t.Errorf("%#v: conversion failed %v", tt.Term, err)
		}

		if !reflect.DeepEqual(dest, tt.Want) {
			t.Errorf("%#v: got %#v, want %#v", tt.Term, dest, tt.Want)
		}
	}
}

func TestTermIntoStruct_Map(t *testing.T) {
	type St struct {
		A uint16
		B float32
	}
	var destIS map[int]string
	var destSI map[string]int
	var destFlSt map[float64]St
	var destSliceSI []map[bool][]int8

	wantIS := map[int]string{
		888: "hello",
		777: "world",
	}
	termIS := Map{
		888: "hello",
		777: Atom("world"),
	}
	if err := TermIntoStruct(termIS, &destIS); err != nil {
		t.Errorf("%#v: conversion failed %v", termIS, err)
	}

	if !reflect.DeepEqual(destIS, wantIS) {
		t.Errorf("%#v: got %#v, want %#v", termIS, destIS, wantIS)
	}

	wantSI := map[string]int{
		"hello": 888,
		"world": 777,
	}
	termSI := Map{
		"hello":       888,
		Atom("world"): 777,
	}
	if err := TermIntoStruct(termSI, &destSI); err != nil {
		t.Errorf("%#v: conversion failed %v", termSI, err)
	}

	if !reflect.DeepEqual(destSI, wantSI) {
		t.Errorf("%#v: got %#v, want %#v", termSI, destSI, wantSI)
	}

	wantFlSt := map[float64]St{
		3.45: St{67, 8.91},
		7.65: St{43, 2.19},
	}
	termFlSt := Map{
		3.45: Tuple{67, 8.91},
		7.65: Tuple{43, 2.19},
	}
	if err := TermIntoStruct(termFlSt, &destFlSt); err != nil {
		t.Errorf("%#v: conversion failed %v", termFlSt, err)
	}

	if !reflect.DeepEqual(destFlSt, wantFlSt) {
		t.Errorf("%#v: got %#v, want %#v", termFlSt, destFlSt, wantFlSt)
	}

	wantSliceSI := []map[bool][]int8{
		map[bool][]int8{
			true:  []int8{1, 2, 3, 4, 5},
			false: []int8{11, 22, 33, 44, 55},
		},
		map[bool][]int8{
			true:  []int8{21, 22, 23, 24, 25},
			false: []int8{-11, -22, -33, -44, -55},
		},
	}
	termSliceSI := List{
		Map{
			true:  List{1, 2, 3, 4, 5},
			false: List{11, 22, 33, 44, 55},
		},
		Map{
			true:  List{21, 22, 23, 24, 25},
			false: List{-11, -22, -33, -44, -55},
		},
	}
	if err := TermIntoStruct(termSliceSI, &destSliceSI); err != nil {
		t.Errorf("%#v: conversion failed %v", termSliceSI, err)
	}

	if !reflect.DeepEqual(destSliceSI, wantSliceSI) {
		t.Errorf("%#v: got %#v, want %#v", termSliceSI, destSliceSI, wantSliceSI)
	}
}

func TestTermMapIntoStruct_Struct(t *testing.T) {
	type testStruct struct {
		A []bool `etf:"a"`
		B uint32 `etf:"b"`
		C string `etf:"c"`
	}

	dest := testStruct{}

	want := testStruct{
		A: []bool{false, true, true},
		B: 3233,
		C: "hello world",
	}

	term := Map{
		Atom("a"): List{false, true, true},
		"b":       3233,
		Atom("c"): "hello world",
	}

	if err := TermIntoStruct(term, &dest); err != nil {
		t.Errorf("%#v: conversion failed %v", term, err)
	}

	if !reflect.DeepEqual(dest, want) {
		t.Errorf("%#v: got %#v, want %#v", term, dest, want)
	}

}
func TestTermMapIntoMap(t *testing.T) {
	type testMap map[string]int

	var dest testMap

	want := testMap{
		"a": 123,
		"b": 456,
		"c": 789,
	}

	term := Map{
		Atom("a"): 123,
		"b":       456,
		Atom("c"): 789,
	}

	if err := TermIntoStruct(term, &dest); err != nil {
		t.Errorf("%#v: conversion failed %v", term, err)
	}

	if !reflect.DeepEqual(dest, want) {
		t.Errorf("%#v: got %#v, want %#v", term, dest, want)
	}

}

func TestTermProplistIntoStruct(t *testing.T) {
	type testStruct struct {
		A []bool `etf:"a"`
		B uint32 `etf:"b"`
		C string `etf:"c"`
	}

	dest := testStruct{}

	want := testStruct{
		A: []bool{false, true, true},
		B: 3233,
		C: "hello world",
	}
	termList := List{
		Tuple{Atom("a"), List{false, true, true}},
		Tuple{"b", 3233},
		Tuple{Atom("c"), "hello world"},
	}

	if err := TermProplistIntoStruct(termList, &dest); err != nil {
		t.Errorf("%#v: conversion failed %v", termList, err)
	}

	if !reflect.DeepEqual(dest, want) {
		t.Errorf("%#v: got %#v, want %#v", termList, dest, want)
	}

	termSliceProplistElements := []ProplistElement{
		ProplistElement{Atom("a"), List{false, true, true}},
		ProplistElement{"b", 3233},
		ProplistElement{Atom("c"), "hello world"},
	}

	if err := TermProplistIntoStruct(termSliceProplistElements, &dest); err != nil {
		t.Errorf("%#v: conversion failed %v", termList, err)
	}

	if !reflect.DeepEqual(dest, want) {
		t.Errorf("%#v: got %#v, want %#v", termSliceProplistElements, dest, want)
	}
}

func TestTermIntoStructCharlistString(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	type Nested struct {
		NestedKey1 String
		NestedKey2 map[string]*Charlist `etf:"field"`
	}
	type StructCharlistString struct {
		Key1 string
		Key2 []*Charlist `etf:"custom_field_name"`
		Key3 *Nested
		Key4 [][]*Charlist
	}

	nestedMap := make(map[string]*Charlist)
	value1 := Charlist("Hello World! 你好世界! Привет Мир! 🚀")
	value11 := String("Hello World! 你好世界! Привет Мир! 🚀")
	nestedMap["map_key"] = &value1

	nested := Nested{
		NestedKey1: value11,
		NestedKey2: nestedMap,
	}

	value2 := Charlist("你好世界! 🚀")
	value3 := Charlist("Привет Мир! 🚀")
	value4 := Charlist("Hello World! 🚀")
	term := StructCharlistString{
		Key1: "Hello World!",
		Key2: []*Charlist{&value2, &value3, &value4},
		Key3: &nested,
		Key4: [][]*Charlist{[]*Charlist{&value2, &value3, &value4}, []*Charlist{&value2, &value3, &value4}},
	}
	err := Encode(term, b, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}

	term_Term, _, err := Decode(b.B, []Atom{}, DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}

	term_dest := StructCharlistString{}
	if err := TermIntoStruct(term_Term, &term_dest); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(term, term_dest) {
		t.Fatal("result != expected")
	}
}

func TestCharlistToString(t *testing.T) {
	l := List{72, 101, 108, 108, 111, 32, 87, 111, 114, 108, 100, 33, 32, 20320, 22909, 19990, 30028, 33, 32, 1055, 1088, 1080, 1074, 1077, 1090, 32, 1052, 1080, 1088, 33, 32, 128640}
	s, err := convertCharlistToString(l)
	if err != nil {
		t.Fatal(err)
	}
	expected := "Hello World! 你好世界! Привет Мир! 🚀"
	if s != expected {
		t.Error("want", expected)
		t.Error("got", s)
		t.Fatal("incorrect result")
	}

}

func TestEncodeDecodePid(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	pidIn := Pid{Node: "erl-demo@127.0.0.1", ID: 32767, Creation: 2}

	err := Encode(pidIn, b, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}

	term, _, err := Decode(b.B, []Atom{}, DecodeOptions{})
	pidOut, ok := term.(Pid)
	if !ok {
		t.Fatal("incorrect result")
	}

	if pidIn != pidOut {
		t.Error("want", pidIn)
		t.Error("got", pidOut)
		t.Fatal("incorrect result")
	}

	// enable BigCreation
	b.Reset()
	encodeOptions := EncodeOptions{
		FlagBigCreation: true,
	}
	err = Encode(pidIn, b, encodeOptions)
	if err != nil {
		t.Fatal(err)
	}

	decodeOptions := DecodeOptions{}
	term, _, err = Decode(b.B, []Atom{}, decodeOptions)
	pidOut, ok = term.(Pid)
	if !ok {
		t.Fatal("incorrect result")
	}

	if pidIn != pidOut {
		t.Error("want", pidIn)
		t.Error("got", pidOut)
		t.Fatal("incorrect result")
	}

	// enable FlagBigPidRef
	b.Reset()
	encodeOptions = EncodeOptions{
		FlagBigPidRef: true,
	}
	err = Encode(pidIn, b, encodeOptions)
	if err != nil {
		t.Fatal(err)
	}

	decodeOptions = DecodeOptions{
		FlagBigPidRef: true,
	}
	term, _, err = Decode(b.B, []Atom{}, decodeOptions)
	pidOut, ok = term.(Pid)
	if !ok {
		t.Fatal("incorrect result")
	}

	if pidIn != pidOut {
		t.Error("want", pidIn)
		t.Error("got", pidOut)
		t.Fatal("incorrect result")
	}

	// enable BigCreation and FlagBigPidRef
	b.Reset()
	encodeOptions = EncodeOptions{
		FlagBigPidRef:   true,
		FlagBigCreation: true,
	}
	err = Encode(pidIn, b, encodeOptions)
	if err != nil {
		t.Fatal(err)
	}

	decodeOptions = DecodeOptions{
		FlagBigPidRef: true,
	}
	term, _, err = Decode(b.B, []Atom{}, decodeOptions)
	pidOut, ok = term.(Pid)
	if !ok {
		t.Fatal("incorrect result")
	}

	if pidIn != pidOut {
		t.Error("want", pidIn)
		t.Error("got", pidOut)
		t.Fatal("incorrect result")
	}
}

type myTime struct {
	Time time.Time
}

func (m myTime) MarshalETF() ([]byte, error) {
	s := fmt.Sprintf("%s", m.Time.Format(time.RFC3339))
	return []byte(s), nil
}

func (m *myTime) UnmarshalETF(b []byte) error {
	t, e := time.Parse(
		time.RFC3339, string(b))
	m.Time = t
	return e
}

func TestTermIntoStructUnmarshal(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	var src, dest myTime

	now := time.Now()
	s := fmt.Sprintf("%s", now.Format(time.RFC3339))
	now, _ = time.Parse(time.RFC3339, s)

	src.Time = now
	err := Encode(src, b, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	term, _, err := Decode(b.B, []Atom{}, DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if err := TermIntoStruct(term, &dest); err != nil {
		t.Errorf("%#v: conversion failed %v", term, err)
	}

	if src != dest {
		t.Fatal("wrong value")
	}

	type aaa struct {
		A1 myTime
		A2 *myTime
	}

	var src1 aaa
	var dst1 aaa

	src1.A1.Time = now
	src1.A2 = &myTime{now}

	b.Reset()

	err = Encode(src1, b, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	term, _, err = Decode(b.B, []Atom{}, DecodeOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if err = TermIntoStruct(term, &dst1); err != nil {
		t.Errorf("%#v: conversion failed %v", term, err)
	}
	if !reflect.DeepEqual(src1, dst1) {
		t.Errorf("got %v, want %v", dst1, src1)
	}
}

func TestRegisterSlice(t *testing.T) {
	type sliceString []string
	type sliceInt []int
	type sliceInt8 []int8
	type sliceInt16 []int16
	type sliceInt32 []int32
	type sliceInt64 []int64
	type sliceUint []uint
	type sliceUint8 []uint8
	type sliceUint16 []uint16
	type sliceUint32 []uint32
	type sliceUint64 []uint64
	type sliceFloat32 []float32
	type sliceFloat64 []float64

	type allInOneSlices struct {
		A sliceString
		B sliceInt
		C sliceInt8
		D sliceInt16
		E sliceInt32
		F sliceInt64
		G sliceUint
		H sliceUint8
		I sliceUint16
		K sliceUint32
		L sliceUint64
		M sliceFloat32
		O sliceFloat64
	}
	types := []interface{}{
		sliceString{},
		sliceInt{},
		sliceInt8{},
		sliceInt16{},
		sliceInt32{},
		sliceInt64{},
		sliceUint{},
		sliceUint8{},
		sliceUint16{},
		sliceUint32{},
		sliceUint64{},
		sliceFloat32{},
		sliceFloat64{},
		allInOneSlices{},
	}
	if err := registerTypes(types); err != nil {
		t.Fatal(err)
	}

	src := allInOneSlices{
		A: sliceString{"Hello", "World"},
		B: sliceInt{-1, 2, -3, 4, -5},
		C: sliceInt8{-1, 2, -3, 4, -5},
		D: sliceInt16{-1, 2, -3, 4, -5},
		E: sliceInt32{-1, 2, -3, 4, -5},
		F: sliceInt64{-1, 2, -3, 4, -5},
		G: sliceUint{1, 2, 3, 4, 5},
		H: sliceUint8{1, 2, 3, 4, 5},
		I: sliceUint16{1, 2, 3, 4, 5},
		K: sliceUint32{1, 2, 3, 4, 5},
		L: sliceUint64{1, 2, 3, 4, 5},
		M: sliceFloat32{1.1, -2.2, 3.3, -4.4, 5.5},
		O: sliceFloat64{1.1, -2.2, 3.3, -4.4, 5.5},
	}

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	encodeOptions := EncodeOptions{
		FlagBigPidRef:   true,
		FlagBigCreation: true,
	}
	if err := Encode(src, buf, encodeOptions); err != nil {
		t.Fatal(err)
	}
	decodeOptions := DecodeOptions{
		FlagBigPidRef: true,
	}
	dst, _, err := Decode(buf.B, []Atom{}, decodeOptions)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := dst.(allInOneSlices); !ok {
		t.Fatalf("wrong term result: %#v\n", dst)
	}

	if !reflect.DeepEqual(src, dst) {
		t.Errorf("got:\n%#v\n\nwant:\n%#v\n", dst, src)
	}
}

func TestRegisterMap(t *testing.T) {
	type mapIntString map[int]string
	type mapStringInt map[string]int
	type mapInt8Int map[int8]int
	type mapFloat32Int32 map[float32]int32
	type mapFloat64Int32 map[float64]int32
	type mapInt32Float32 map[int32]float32
	type mapInt32Float64 map[int32]float64

	type allInOne struct {
		A mapIntString
		B mapStringInt
		C mapInt8Int
		D mapFloat32Int32
		E mapFloat64Int32
		F mapInt32Float32
		G mapInt32Float64
	}

	types := []interface{}{
		mapIntString{},
		mapStringInt{},
		mapInt8Int{},
		mapFloat32Int32{},
		mapFloat64Int32{},
		mapInt32Float32{},
		mapInt32Float64{},
		allInOne{},
	}
	if err := registerTypes(types); err != nil {
		t.Fatal(err)
	}

	src := allInOne{
		A: make(mapIntString),
		B: make(mapStringInt),
		C: make(mapInt8Int),
		D: make(mapFloat32Int32),
		E: make(mapFloat64Int32),
		F: make(mapInt32Float32),
		G: make(mapInt32Float64),
	}

	src.A[1] = "Hello"
	src.B["Hello"] = 1
	src.C[1] = 1
	src.D[3.14] = 1
	src.E[3.15] = 1
	src.F[1] = 3.15
	src.G[1] = 3.15

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	encodeOptions := EncodeOptions{
		FlagBigPidRef:   true,
		FlagBigCreation: true,
	}
	if err := Encode(src, buf, encodeOptions); err != nil {
		t.Fatal(err)
	}
	decodeOptions := DecodeOptions{
		FlagBigPidRef: true,
	}
	dst, _, err := Decode(buf.B, []Atom{}, decodeOptions)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := dst.(allInOne); !ok {
		t.Fatalf("wrong term result: %#v\n", dst)
	}

	if !reflect.DeepEqual(src, dst) {
		t.Errorf("got:\n%#v\n\nwant:\n%#v\n", dst, src)
	}

}

func TestRegisterType(t *testing.T) {
	type ccc []string
	type ddd [3]bool
	type aaa struct {
		A   string
		B   int
		B8  int8
		B16 int16
		B32 int32
		B64 int64

		BU   uint
		BU8  uint8
		BU16 uint16
		BU32 uint32
		BU64 uint64

		C   float32
		C64 float64
		D   ddd

		T1 Pid
		T2 Ref
		T3 Alias
	}
	type bbb map[aaa]ccc

	src := bbb{
		aaa{
			A:    "aa",
			B:    -11,
			B8:   -18,
			B16:  -1116,
			B32:  -1132,
			B64:  -1164,
			BU:   0xb,
			BU8:  0x12,
			BU16: 0x45c,
			BU32: 0x46c,
			BU64: 0x48c,
			C:    -11.22,
			C64:  1164.22,
			D:    ddd{true, false, false},
			T1:   Pid{Node: Atom("nodepid11"), ID: 123, Creation: 456},
			T2:   Ref{Node: Atom("noderef11"), Creation: 123, ID: [5]uint32{4, 5, 6, 0, 0}},
			T3:   Alias{Node: Atom("nodealias11"), Creation: 123, ID: [5]uint32{4, 5, 6, 0, 0}},
		}: ccc{"a1", "a2", "a3"},
		aaa{
			A:    "bb",
			B:    -22,
			B8:   -28,
			B16:  -2216,
			B32:  -2232,
			B64:  -2264,
			BU:   0x16,
			BU8:  0x1c,
			BU16: 0x8a8,
			BU32: 0x8b8,
			BU64: 0x8d8,
			C:    -22.22,
			C64:  2264.22,
			D:    ddd{false, true, false},
			T1:   Pid{Node: Atom("nodepid22"), ID: 123, Creation: 456},
			T2:   Ref{Node: Atom("noderef22"), Creation: 123, ID: [5]uint32{4, 5, 6, 0, 0}},
			T3:   Alias{Node: Atom("nodealias22"), Creation: 123, ID: [5]uint32{4, 5, 6, 0, 0}},
		}: ccc{"b1", "b2", "b3"},
		aaa{
			A:    "cc",
			B:    -33,
			B8:   -38,
			B16:  -3316,
			B32:  -3332,
			B64:  -3364,
			BU:   0x21,
			BU8:  0x26,
			BU16: 0xcf4,
			BU32: 0xd04,
			BU64: 0xd24,
			C:    -33.22,
			C64:  3364.22,
			D:    ddd{false, false, true},
			T1:   Pid{Node: Atom("nodepid33"), ID: 123, Creation: 456},
			T2:   Ref{Node: Atom("noderef33"), Creation: 123, ID: [5]uint32{4, 5, 6, 0, 0}},
			T3:   Alias{Node: Atom("nodealias33"), Creation: 123, ID: [5]uint32{4, 5, 6, 0, 0}},
		}: ccc{}, //test empty list
	}

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	if _, err := RegisterType(Pid{}, RegisterTypeOptions{Strict: true}); err == nil {
		t.Fatal("shouldn't be registered")
	}
	if _, err := RegisterType(Ref{}, RegisterTypeOptions{Strict: true}); err == nil {
		t.Fatal("shouldn't be registered")
	}
	if _, err := RegisterType(Alias{}, RegisterTypeOptions{Strict: true}); err == nil {
		t.Fatal("shouldn't be registered")
	}

	types := []interface{}{
		ddd{},
		aaa{},
		ccc{},
		bbb{},
	}
	if err := registerTypes(types); err != nil {
		t.Fatal(err)
	}

	encodeOptions := EncodeOptions{
		FlagBigPidRef:   true,
		FlagBigCreation: true,
	}
	if err := Encode(src, buf, encodeOptions); err != nil {
		t.Fatal(err)
	}
	decodeOptions := DecodeOptions{
		FlagBigPidRef: true,
	}
	dst, _, err := Decode(buf.B, []Atom{}, decodeOptions)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := dst.(bbb); !ok {
		t.Fatal("wrong term result")
	}

	if !reflect.DeepEqual(src, dst) {
		t.Errorf("got:\n%#v\n\nwant:\n%#v\n", dst, src)
	}
}

func registerTypes(types []interface{}) error {
	rtOpts := RegisterTypeOptions{Strict: true}

	for _, t := range types {
		if _, err := RegisterType(t, rtOpts); err != nil && err != lib.ErrTaken {
			return err
		}
	}
	return nil
}
