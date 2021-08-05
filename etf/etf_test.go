package etf

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/halturin/ergo/lib"
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
		D String
		E Atom
		F []byte
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
					D: String("test"),
					E: Atom("value"),
					F: []byte{97, 127, 255},
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
					"test",                                // D String (string -> String)
					"value",                               // E Atom (string -> Atom)
					string([]byte{97, 127, 255}),          // F []byte (string -> []byte)
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
		D String `etf:"d"`
		E Atom   `etf:"e"`
		F []byte `etf:"f"`
	}

	dest := testStruct{}

	want := testStruct{
		A: []bool{false, true, true},
		B: 3233,
		C: "hello world",
		D: String("hello"),
		E: Atom("world"),
		F: []byte{97, 127, 255},
	}

	term := Map{
		Atom("a"): List{false, true, true},
		"b":       3233,
		Atom("c"): "hello world",
		Atom("d"): String("hello"),
		Atom("e"): Atom("world"),
		Atom("f"): []byte{97, 127, 255},
	}

	if err := TermMapIntoStruct(term, &dest); err != nil {
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
		D String `etf:"d"`
		E Atom   `etf:"e"`
		F []byte `etf:"f"`
	}

	dest := testStruct{}

	want := testStruct{
		A: []bool{false, true, true},
		B: 3233,
		C: "hello world",
		D: String("hello"),
		E: Atom("world"),
		F: []byte{97, 127, 255},
	}
	termList := List{
		Tuple{Atom("a"), List{false, true, true}},
		Tuple{"b", 3233},
		Tuple{Atom("c"), "hello world"},
		Tuple{Atom("d"), String("hello")},
		Tuple{Atom("e"), Atom("world")},
		Tuple{Atom("f"), []byte{97, 127, 255}},
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

func TestTermIntoStruct_WithTags(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	type Nested struct {
		NestedKey1 string
		NestedKey2 map[string]*string `etf:"field charlist"`
	}
	type StructWithTags struct {
		Key1 string
		Key2 []*string `etf:"custom_field_name charlist"`
		Key3 *Nested
		Key4 [][]*string `etf:"charlist"`
	}

	nestedMap := make(map[string]*string)
	value1 := "Hello World! 你好世界! Привет Мир! 🚀"
	nestedMap["map_key"] = &value1

	nested := Nested{
		NestedKey1: value1,
		NestedKey2: nestedMap,
	}

	value2 := "你好世界! 🚀"
	value3 := "Привет Мир! 🚀"
	value4 := "Hello World! 🚀"
	term_source := StructWithTags{
		Key1: "Hello World!",
		Key2: []*string{&value2, &value3, &value4},
		Key3: &nested,
		Key4: [][]*string{[]*string{&value2, &value3, &value4}, []*string{&value2, &value3, &value4}},
	}
	if err := Encode(term_source, b, nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	term_Term, _, err := Decode(b.B, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	term_dest := StructWithTags{}
	if err := TermMapIntoStruct(term_Term, &term_dest); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(term_source, term_dest) {
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
