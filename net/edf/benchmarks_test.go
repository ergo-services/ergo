package edf

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"sync"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

func BenchmarkEncodeString(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Encode("Ergo Framework", buf, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkEncodeStringGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)
	gob.Register(benchEncodeStruct{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode("Ergo Framework"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeAtom(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Encode(gen.Atom("Ergo Framework"), buf, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeAtomGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)
	gob.Register(benchEncodeStruct{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode(gen.Atom("Ergo Framework")); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeBinary(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)
	value := []byte{1, 2, 3, 4, 5}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeBinaryGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)

	value := []byte{1, 2, 3, 4, 5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode(value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeSlice(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := []int{12345, 67890, 12345, 67890}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeSliceGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)

	value := []int{12345, 67890, 12345, 67890}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode(value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeArray(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := [4]int{12345, 67890, 12345, 67890}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeArrayGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)

	value := [4]int{12345, 67890, 12345, 67890}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode(value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMap(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := map[string]int{
		"key1": 1234,
		"key2": 5678,
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMapGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)

	value := map[string]int{
		"key1": 1234,
		"key2": 5678,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode(value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodePID(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := gen.PID{Node: "demo@127.0.0.1", ID: 312, Creation: 2}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodePIDGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)

	value := gen.PID{Node: "demo@127.0.0.1", ID: 312, Creation: 2}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode(value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeProcessID(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := gen.ProcessID{Node: "demo@127.0.0.1", Name: "example"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeProcessIDGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)

	value := gen.ProcessID{Node: "demo@127.0.0.1", Name: "example"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode(value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeRef(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := gen.Ref{
		Node:     "demo@127.0.0.1",
		Creation: 2,
		ID:       [3]uint32{73444, 3082813441, 2373634851},
	}

	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeRefGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)

	value := gen.Ref{
		Node:     "demo@127.0.0.1",
		Creation: 2,
		ID:       [3]uint32{73444, 3082813441, 2373634851},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode(value); err != nil {
			b.Fatal(err)
		}
	}
}

type benchEncodeStruct struct {
	A float32
	B float64
	C any
}

func BenchmarkEncodeStructEDF(b *testing.B) {
	var regCache *sync.Map

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := benchEncodeStruct{
		A: 3.14,
		B: 3.15,
		C: float64(3.18),
	}

	RegisterTypeOf(benchEncodeStruct{})
	regCache = new(sync.Map)
	regCache.Store(reflect.TypeOf(value), []byte{edtReg, 0x13, 0x88})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, Options{RegCache: regCache}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeStructGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)
	gob.Register(benchEncodeStruct{})

	value := benchEncodeStruct{
		A: 3.14,
		B: 3.15,
		C: float64(3.18),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode(value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeSliceStructEDF(b *testing.B) {
	var regCache *sync.Map

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := []benchEncodeStruct{
		{
			A: 3.14,
			B: 3.15,
			C: float64(3.18),
		},
		{
			A: 3.15,
			B: 3.14,
			C: float64(3.19),
		},
	}

	RegisterTypeOf(benchEncodeStruct{})
	regCache = new(sync.Map)
	regCache.Store(reflect.TypeOf(value), []byte{edtReg, 0x13, 0x88})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, Options{RegCache: regCache}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeSliceStructGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)
	gob.Register(benchEncodeStruct{})

	value := []benchEncodeStruct{
		{
			A: 3.14,
			B: 3.15,
			C: float64(3.18),
		},
		{
			A: 3.15,
			B: 3.14,
			C: float64(3.19),
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode(value); err != nil {
			b.Fatal(err)
		}
	}
}

// decoding

func BenchmarkDecodeString(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)
	if err := Encode("Ergo Framework", buf, Options{}); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeAtom(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	if err := Encode(gen.Atom("Ergo Framework"), buf, Options{}); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeBinary(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := []byte{1, 2, 3, 4, 5}
	if err := Encode(value, buf, Options{}); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeSlice(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := []int{12345, 67890, 12345, 67890}
	if err := Encode(value, buf, Options{}); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeArray(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := [4]int{12345, 67890, 12345, 67890}
	if err := Encode(value, buf, Options{}); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeMap(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := map[string]int{
		"key1": 1234,
		"key2": 5678,
	}
	if err := Encode(value, buf, Options{}); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodePID(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := gen.PID{Node: "demo@127.0.0.1", ID: 312, Creation: 2}
	if err := Encode(value, buf, Options{}); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeProcessID(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := gen.ProcessID{Node: "demo@127.0.0.1", Name: "example"}
	if err := Encode(value, buf, Options{}); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeRef(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := gen.Ref{
		Node:     "demo@127.0.0.1",
		Creation: 2,
		ID:       [3]uint32{73444, 3082813441, 2373634851},
	}
	if err := Encode(value, buf, Options{}); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeStructEDF(b *testing.B) {
	var regCache *sync.Map

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := benchEncodeStruct{
		A: 3.14,
		B: 3.15,
		C: float64(3.18),
	}

	RegisterTypeOf(benchEncodeStruct{})
	//regCache = new(sync.Map)
	//regCache.Store(reflect.TypeOf(value), []byte{edtReg, 0x13, 0x88})
	if err := Encode(value, buf, Options{RegCache: regCache}); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeStructGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)
	gob.Register(benchEncodeStruct{})

	value := benchEncodeStruct{
		A: 3.14,
		B: 3.15,
		C: float64(3.18),
	}
	if err := enc.Encode(value); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var x benchEncodeStruct
		decbuf := bytes.NewBuffer(buf.Bytes())
		dec := gob.NewDecoder(decbuf)
		if err := dec.Decode(&x); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeSliceStructEDF(b *testing.B) {
	var regCache *sync.Map

	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := []benchEncodeStruct{
		{
			A: 3.14,
			B: 3.15,
			C: float64(3.18),
		},
		{
			A: 3.15,
			B: 3.14,
			C: float64(3.19),
		},
	}

	RegisterTypeOf(benchEncodeStruct{})
	//regCache = new(sync.Map)
	//regCache.Store(reflect.TypeOf(value), []byte{edtReg, 0x13, 0x88})
	if err := Encode(value, buf, Options{RegCache: regCache}); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, Options{})
		if err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkDecodeSliceStructGob(b *testing.B) {
	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)
	gob.Register(benchEncodeStruct{})

	value := []benchEncodeStruct{
		{
			A: 3.14,
			B: 3.15,
			C: float64(3.18),
		},
		{
			A: 3.15,
			B: 3.14,
			C: float64(3.19),
		},
	}
	if err := enc.Encode(value); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var x []benchEncodeStruct
		decbuf := bytes.NewBuffer(buf.Bytes())
		dec := gob.NewDecoder(decbuf)
		if err := dec.Decode(&x); err != nil {
			b.Fatal(err)
		}
	}
}
