package edf

import (
	//  "encoding/binary"
	"fmt"
	"math"
	"reflect"
	"sync"
	"sync/atomic"
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

type testRegTagSkipExported struct {
	ID   int64
	Name string
	Skip *int `edf:"-"`
}

type testRegTagSkipUnexported struct {
	ID    int64
	Name  string
	cache map[string]int `edf:"-"`
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

func TestRegTagSkipExportedField(t *testing.T) {
	if err := RegisterTypeOf(testRegTagSkipExported{}); err != nil && err != gen.ErrTaken {
		t.Fatalf("register failed for struct with edf:\"-\" on exported field: %v", err)
	}
}

func TestRegTagSkipUnexportedField(t *testing.T) {
	if err := RegisterTypeOf(testRegTagSkipUnexported{}); err != nil && err != gen.ErrTaken {
		t.Fatalf("register failed for struct with edf:\"-\" on unexported field: %v", err)
	}
}

type edfOverflowType int

// Registering a type once the 16-bit reg-cache id space is exhausted must
// surface an error, not silently register with a full-name fallback. Regression
// for the discarded addRegCache error (#257): the overflow now propagates.
func TestRegisterTypeCacheOverflow(t *testing.T) {
	old := atomic.LoadUint32(&regCacheID)
	atomic.StoreUint32(&regCacheID, math.MaxUint16)
	defer atomic.StoreUint32(&regCacheID, old)

	if err := registerType(reflect.TypeOf(edfOverflowType(0))); err == nil {
		t.Fatal("registering past the 65535-type cache limit must return an error")
	}
}
