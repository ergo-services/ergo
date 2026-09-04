package edf

import (
	"bytes"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type fuzzInnerStruct struct {
	Value    int
	Name     string
	Flag     bool
	FloatVal float64
}

type fuzzMiddleStruct struct {
	Inner     fuzzInnerStruct
	InnerPtr  *fuzzInnerStruct
	Values    []int
	ValuesPtr []*int
	Data      map[string]int
	DataPtr   map[string]*int
}

type fuzzComplexStruct struct {
	Int8Val   int8
	Int16Val  int16
	Int32Val  int32
	Int64Val  int64
	Uint8Val  uint8
	Uint16Val uint16
	Uint32Val uint32
	Uint64Val uint64
	Float32   float32
	Float64   float64
	Bool      bool
	String    string
	Binary    []byte

	IntPtr    *int
	StringPtr *string
	FloatPtr  *float64
	BoolPtr   *bool

	Atom      gen.Atom
	AtomPtr   *gen.Atom
	PID       gen.PID
	PIDPtr    *gen.PID
	ProcessID gen.ProcessID
	Ref       gen.Ref
	Alias     gen.Alias
	Event     gen.Event

	Middle    fuzzMiddleStruct
	MiddlePtr *fuzzMiddleStruct

	IntSlice       []int
	StringSlice    []string
	StructSlice    []fuzzInnerStruct
	StructPtrSlice []*fuzzInnerStruct
	PtrSlice       []*int

	StringIntMap    map[string]int
	IntStringMap    map[int]string
	StringStructMap map[string]fuzzInnerStruct
	StringPtrMap    map[string]*int
}

func init() {
	RegisterTypeOf(fuzzInnerStruct{})
	RegisterTypeOf(fuzzMiddleStruct{})
	RegisterTypeOf(fuzzComplexStruct{})
}

// compare by re-encoding decoded value and comparing bytes
func compareByEncode(original, decoded any) bool {
	b1 := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b1)
	b2 := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b2)

	if err := Encode(original, b1, Options{}); err != nil {
		return false
	}
	if err := Encode(decoded, b2, Options{}); err != nil {
		return false
	}
	return bytes.Equal(b1.B, b2.B)
}

func FuzzComplexStruct(f *testing.F) {
	// seed corpus - various primitive combinations
	f.Add(
		int8(0), int16(0), int32(0), int64(0),
		uint8(0), uint16(0), uint32(0), uint64(0),
		float32(0), float64(0), false, "", []byte{},
	)
	f.Add(
		int8(42), int16(1000), int32(100000), int64(1000000000),
		uint8(255), uint16(65535), uint32(100000), uint64(1000000000),
		float32(3.14), float64(3.14159), true, "hello", []byte{1, 2, 3},
	)
	f.Add(
		int8(-128), int16(-32768), int32(-2147483648), int64(-9223372036854775808),
		uint8(0), uint16(0), uint32(0), uint64(0),
		float32(-1.5), float64(-1.5), false, "test", []byte{0xff, 0xfe},
	)

	f.Fuzz(func(t *testing.T,
		i8 int8, i16 int16, i32 int32, i64 int64,
		u8 uint8, u16 uint16, u32 uint32, u64 uint64,
		f32 float32, f64 float64, b bool, s string, bin []byte,
	) {
		// limit string for Atom (max 255 bytes)
		if len(s) > 255 {
			s = s[:255]
		}

		// build pointers based on values
		intVal := int(i64)
		floatVal := f64
		boolVal := b

		innerVal := int(i32)
		innerName := s
		innerFloat := f64

		original := fuzzComplexStruct{
			Int8Val:   i8,
			Int16Val:  i16,
			Int32Val:  i32,
			Int64Val:  i64,
			Uint8Val:  u8,
			Uint16Val: u16,
			Uint32Val: u32,
			Uint64Val: u64,
			Float32:   f32,
			Float64:   f64,
			Bool:      b,
			String:    s,
			Binary:    bin,

			IntPtr:    &intVal,
			StringPtr: &s,
			FloatPtr:  &floatVal,
			BoolPtr:   &boolVal,

			Atom:      gen.Atom(s),
			AtomPtr:   nil,
			PID:       gen.PID{Node: gen.Atom(s), ID: u64, Creation: i64},
			PIDPtr:    nil,
			ProcessID: gen.ProcessID{Node: gen.Atom(s), Name: gen.Atom(s)},
			Ref:       gen.Ref{Node: gen.Atom(s), Creation: i64, ID: [3]uint64{u64, u64, u64}},
			Alias:     gen.Alias{Node: gen.Atom(s), Creation: i64, ID: [3]uint64{u64, u64, u64}},
			Event:     gen.Event{Node: gen.Atom(s), Name: gen.Atom(s)},

			Middle: fuzzMiddleStruct{
				Inner: fuzzInnerStruct{
					Value:    innerVal,
					Name:     innerName,
					Flag:     b,
					FloatVal: innerFloat,
				},
				InnerPtr:  &fuzzInnerStruct{Value: innerVal, Name: innerName, Flag: b, FloatVal: innerFloat},
				Values:    []int{int(i64), int(i32), int(i16)},
				ValuesPtr: []*int{&intVal, nil, &innerVal},
				Data:      map[string]int{s: int(i64)},
				DataPtr:   map[string]*int{s: &intVal},
			},
			MiddlePtr: &fuzzMiddleStruct{
				Inner:     fuzzInnerStruct{Value: innerVal, Name: innerName, Flag: b, FloatVal: innerFloat},
				InnerPtr:  nil,
				Values:    []int{int(i64)},
				ValuesPtr: []*int{nil},
				Data:      map[string]int{},
				DataPtr:   map[string]*int{},
			},

			IntSlice:       []int{int(i64), int(i32), int(i16), int(i8)},
			StringSlice:    []string{s, s},
			StructSlice:    []fuzzInnerStruct{{Value: innerVal, Name: innerName, Flag: b, FloatVal: innerFloat}},
			StructPtrSlice: []*fuzzInnerStruct{nil, &fuzzInnerStruct{Value: innerVal, Name: innerName, Flag: b, FloatVal: innerFloat}},
			PtrSlice:       []*int{&intVal, nil, &innerVal},

			StringIntMap:    map[string]int{s: int(i64)},
			IntStringMap:    map[int]string{int(i64): s},
			StringStructMap: map[string]fuzzInnerStruct{s: {Value: innerVal, Name: innerName, Flag: b, FloatVal: innerFloat}},
			StringPtrMap:    map[string]*int{s: &intVal},
		}

		buf := lib.TakeBuffer()
		defer lib.ReleaseBuffer(buf)

		if err := Encode(original, buf, Options{}); err != nil {
			t.Fatalf("encode error: %s", err)
		}

		decoded, _, err := Decode(buf.B, Options{})
		if err != nil {
			t.Fatalf("decode error: %s", err)
		}

		decodedStruct, ok := decoded.(fuzzComplexStruct)
		if ok == false {
			t.Fatalf("expected fuzzComplexStruct, got %T", decoded)
		}

		if compareByEncode(original, decodedStruct) == false {
			t.Fatalf("mismatch after roundtrip")
		}
	})
}

// FuzzDecode - check decoder doesn't panic on random bytes
func FuzzDecode(f *testing.F) {
	f.Add([]byte{edtNil})
	f.Add([]byte{edtBool, 1})
	f.Add([]byte{edtInt, 0, 0, 0, 0, 0, 0, 0, 42})
	f.Add([]byte{edtString, 0, 5, 'h', 'e', 'l', 'l', 'o'})
	f.Add([]byte{edtType, 0, 2, edtPtr, edtInt, edtNil})
	f.Add([]byte{edtType, 0, 2, edtPtr, edtInt, edtPtr, 0, 0, 0, 0, 0, 0, 0, 42})

	f.Fuzz(func(t *testing.T, data []byte) {
		Decode(data, Options{})
	})
}
