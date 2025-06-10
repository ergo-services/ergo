package edf

import (
	"bytes"
	"encoding/gob"
	"sync"
	"testing"

	"ergo.services/ergo/lib"
)

// ENCODING BENCHMARKS WITHOUT TYPE CACHING

func BenchmarkEncodeString(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode("Ergo Framework", buf, Options{}); err != nil {
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

func BenchmarkEncodeSlice(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := []int64{12345, 67890, 12345, 67890}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

// ENCODING BENCHMARKS WITH TYPE CACHING

func BenchmarkEncodeStringCached(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	cache := &sync.Map{}
	options := Options{Cache: cache}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode("Ergo Framework", buf, options); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMapCached(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := map[string]int{
		"key1": 1234,
		"key2": 5678,
	}

	cache := &sync.Map{}
	options := Options{Cache: cache}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, options); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeSliceCached(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := []int64{12345, 67890, 12345, 67890}

	cache := &sync.Map{}
	options := Options{Cache: cache}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, options); err != nil {
			b.Fatal(err)
		}
	}
}

// COMPLEX TYPES TO SHOW BIGGER IMPACT

func BenchmarkEncodeComplexMap(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := map[string]map[int][]string{
		"group1": {
			1: {"item1", "item2", "item3"},
			2: {"item4", "item5"},
		},
		"group2": {
			3: {"item6", "item7", "item8", "item9"},
			4: {"item10"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeComplexMapCached(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := map[string]map[int][]string{
		"group1": {
			1: {"item1", "item2", "item3"},
			2: {"item4", "item5"},
		},
		"group2": {
			3: {"item6", "item7", "item8", "item9"},
			4: {"item10"},
		},
	}

	cache := &sync.Map{}
	options := Options{Cache: cache}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := Encode(value, buf, options); err != nil {
			b.Fatal(err)
		}
	}
}

// GOB COMPARISONS

func BenchmarkEncodeStringGob(b *testing.B) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode("Ergo Framework"); err != nil {
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

func BenchmarkEncodeSliceGob(b *testing.B) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	value := []int64{12345, 67890, 12345, 67890}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := enc.Encode(value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeComplexMapGob(b *testing.B) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	value := map[string]map[int][]string{
		"group1": {
			1: {"item1", "item2", "item3"},
			2: {"item4", "item5"},
		},
		"group2": {
			3: {"item6", "item7", "item8", "item9"},
			4: {"item10"},
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

// DECODING BENCHMARKS WITHOUT TYPE CACHING

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

// DECODING BENCHMARKS WITH TYPE CACHING

func BenchmarkDecodeStringCached(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	cache := &sync.Map{}
	options := Options{Cache: cache}

	if err := Encode("Ergo Framework", buf, options); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, options)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeMapCached(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := map[string]int{
		"key1": 1234,
		"key2": 5678,
	}

	cache := &sync.Map{}
	options := Options{Cache: cache}

	if err := Encode(value, buf, options); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, options)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeSliceCached(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	value := []int{12345, 67890, 12345, 67890}

	cache := &sync.Map{}
	options := Options{Cache: cache}

	if err := Encode(value, buf, options); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(buf.B, options)
		if err != nil {
			b.Fatal(err)
		}
	}
}
