package edf

import (
	//  "encoding/binary"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type testRegBool bool
type testRegString string
type testRegFloat32 float32
type testRegFloat64 float64
type testRegInt int
type testRegInt8 int8
type testRegInt16 int16
type testRegInt32 int32
type testRegInt64 int64
type testRegUint uint
type testRegUint8 uint8
type testRegUint16 uint16
type testRegUint32 uint32
type testRegUint64 uint64
type testRegBin []byte
type testRegMap map[bool]string

type testRegStruct struct{ A bool }
type testRegSlice []bool
type testRegArray [3]bool

type testRegStructUnexported struct {
	A bool
	b bool // unexported field
}

type regCases struct {
	name  string
	value any
}

func registerCases() []regCases {

	return []regCases{
		{"bool", testRegBool(true)},
		{"string", testRegString("string")},
		{"float32", testRegFloat32(3.12)},
		{"float64", testRegFloat64(3.14)},
		{"int", testRegInt(10)},
		{"int8", testRegInt8(11)},
		{"int16", testRegInt16(12)},
		{"int32", testRegInt32(13)},
		{"int64", testRegInt64(14)},
		{"uint", testRegUint(15)},
		{"uint8", testRegUint8(16)},
		{"uint16", testRegUint16(17)},
		{"uint32", testRegUint32(18)},
		{"uint64", testRegUint64(19)},
		{"[]byte", testRegBin([]byte{1, 2, 3, 4, 5})},
		{"struct", testRegStruct{A: true}},
		{"slice", testRegSlice{false, true, false, true, false}},
		{"array", testRegArray{false, true, false}},
		{"map", testRegMap{false: "string1", true: "string2"}},
	}
}

func TestRegTypes(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	cache := new(sync.Map)
	for _, c := range registerCases() {
		t.Run(c.name, func(t *testing.T) {
			b.Reset()
			if err := RegisterTypeOf(c.value); err != nil {
				if err != gen.ErrTaken {
					t.Fatal(err)
				}
			}

			if err := Encode(c.value, b, Options{Cache: cache}); err != nil {
				t.Fatal(err)
			}
			value, _, err := Decode(b.B, Options{})
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(c.value, value) {
				fmt.Printf("exp (%T) %#v\n", c.value, c.value)
				fmt.Printf("got (%T) %#v\n", value, value)
				t.Fatal("incorrect value")
			}
		})
	}
}

func TestRegCacheTypes(t *testing.T) {
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	cache := new(sync.Map)
	for _, c := range registerCases() {
		t.Run(c.name, func(t *testing.T) {
			b.Reset()
			if err := RegisterTypeOf(c.value); err != nil {
				if err != gen.ErrTaken {
					t.Fatal(err)
				}
			}
			rcDec := new(sync.Map)
			names := []string{}
			for k, v := range GetRegCache() {
				names = append(names, v)
				rcDec.Store(k, v)
			}

			if len(names) == 0 {
				t.Fatal("decoding reg cache is empty")
			}
			rcEnc := MakeEncodeRegTypeCache(names)
			if rcEnc == nil {
				t.Fatal("encoding reg cache is nil")
			}

			if err := Encode(c.value, b, Options{RegCache: rcEnc, Cache: cache}); err != nil {
				t.Fatal(err)
			}
			value, _, err := Decode(b.B, Options{RegCache: rcDec, Cache: cache})
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(c.value, value) {
				fmt.Printf("exp (%T) %#v\n", c.value, c.value)
				fmt.Printf("got (%T) %#v\n", value, value)
				t.Fatal("incorrect value")
			}
		})
	}
}

type regErrorCases struct {
	name        string
	value       any
	expectedErr string
}

func registerErrorCases() []regErrorCases {
	return []regErrorCases{
		{
			name:        "struct with unexported fields",
			value:       testRegStructUnexported{A: true, b: false},
			expectedErr: fmt.Sprintf("struct %s has unexported field(s)", "testRegStructUnexported"),
		},
		{
			name:        "regular type bool",
			value:       true,
			expectedErr: "unable to register a regular type",
		},
		{
			name:        "regular type string",
			value:       "test",
			expectedErr: "unable to register a regular type",
		},
		{
			name:        "pointer type",
			value:       &testRegStruct{},
			expectedErr: "pointer type is not supported",
		},
		{
			name:        "ergo framework type",
			value:       gen.Atom("test"),
			expectedErr: "unable to register a type of Ergo Framework",
		},
	}
}

func TestRegTypeErrors(t *testing.T) {
	for _, c := range registerErrorCases() {
		t.Run(c.name, func(t *testing.T) {
			err := RegisterTypeOf(c.value)
			if err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
			if err.Error() != c.expectedErr {
				t.Fatalf("expected error message '%s', got '%s'", c.expectedErr, err.Error())
			}
		})
	}
}
