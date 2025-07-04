package edf

import (
	"bytes"
	"encoding/gob"
	"testing"

	"ergo.services/ergo/lib"
)

// Test structs for benchmarking
type SimpleStruct struct {
	ID   int64
	Name string
	Flag bool
}

type ComplexStruct struct {
	ID       int64
	Name     string
	Data     []byte
	Numbers  []int64
	Tags     map[string]string
	Metadata SimpleStruct
}

type LargeStruct struct {
	Field1  string
	Field2  int64
	Field3  float64
	Field4  bool
	Field5  []string
	Field6  []int
	Field7  map[string]int
	Field8  SimpleStruct
	Field9  []SimpleStruct
	Field10 map[string]SimpleStruct
}

// Register struct types for EDF
func init() {
	RegisterTypeOf(SimpleStruct{})
	RegisterTypeOf(ComplexStruct{})
	RegisterTypeOf(LargeStruct{})
}

// Benchmark tests to measure performance improvements from optimizations

func BenchmarkOptimizationImpact(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	b.Run("String_Encoding", func(b *testing.B) {
		value := "Ergo Framework Performance Optimization Test String"

		for i := 0; i < b.N; i++ {
			buf.Reset()
			if err := Encode(value, buf, Options{}); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Binary_Encoding", func(b *testing.B) {
		data := make([]byte, 100)
		for i := range data {
			data[i] = byte(i)
		}

		for i := 0; i < b.N; i++ {
			buf.Reset()
			if err := Encode(data, buf, Options{}); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Integer_Slice", func(b *testing.B) {
		slice := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

		for i := 0; i < b.N; i++ {
			buf.Reset()
			if err := Encode(slice, buf, Options{}); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Complex_Map", func(b *testing.B) {
		data := map[string]int{
			"key1": 123,
			"key2": 456,
			"key3": 789,
		}

		for i := 0; i < b.N; i++ {
			buf.Reset()
			if err := Encode(data, buf, Options{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkDecodeOptimizations(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	// Pre-encode test data
	testString := "Performance test string for decode benchmarks"
	testSlice := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	buf.Reset()
	Encode(testString, buf, Options{})
	stringData := make([]byte, len(buf.B))
	copy(stringData, buf.B)

	buf.Reset()
	Encode(testSlice, buf, Options{})
	sliceData := make([]byte, len(buf.B))
	copy(sliceData, buf.B)

	b.Run("String_Decoding", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, err := Decode(stringData, Options{})
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Slice_Decoding", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, err := Decode(sliceData, Options{})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkMemoryAllocationComparison(b *testing.B) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	testData := map[string]any{
		"string": "test string",
		"number": int64(12345),
		"binary": []byte{1, 2, 3, 4, 5},
		"nested": map[string]int{
			"a": 1,
			"b": 2,
		},
	}

	b.Run("Complex_Structure_Encoding", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf.Reset()
			if err := Encode(testData, buf, Options{}); err != nil {
				b.Fatal(err)
			}
		}
	})

	// Pre-encode for decode test
	buf.Reset()
	Encode(testData, buf, Options{})
	encodedData := make([]byte, len(buf.B))
	copy(encodedData, buf.B)

	b.Run("Complex_Structure_Decoding", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, err := Decode(encodedData, Options{})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Test to verify optimizations don't break functionality
func TestOptimizedFunctionality(t *testing.T) {
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	testCases := []any{
		"test string",
		int64(12345),
		[]byte{1, 2, 3, 4, 5},
		[]int{1, 2, 3, 4, 5},
		map[string]int{"a": 1, "b": 2},
	}

	for i, testCase := range testCases {
		buf.Reset()

		// Encode
		err := Encode(testCase, buf, Options{})
		if err != nil {
			t.Fatalf("Test case %d encode failed: %v", i, err)
		}

		// Decode
		decoded, _, err := Decode(buf.B, Options{})
		if err != nil {
			t.Fatalf("Test case %d decode failed: %v", i, err)
		}

		// Basic validation (type-specific comparison would be more thorough)
		if decoded == nil {
			t.Fatalf("Test case %d: decoded value is nil", i)
		}
	}
}

// Benchmark comparison between EDF and gob
func BenchmarkEDFvsGob(b *testing.B) {
	// Test data structures
	testString := "Ergo Framework Performance Comparison Test String with some length to make it meaningful"
	testBinary := make([]byte, 200)
	for i := range testBinary {
		testBinary[i] = byte(i % 256)
	}
	testSlice := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	testMap := map[string]int{
		"key1": 123, "key2": 456, "key3": 789, "key4": 101112,
		"key5": 131415, "key6": 161718, "key7": 192021, "key8": 222324,
	}

	// Complex structure - nested map with consistent types
	testNestedMap := map[string]map[string]string{
		"group1": {"item1": "value1", "item2": "value2"},
		"group2": {"item3": "value3", "item4": "value4"},
		"group3": {"item5": "value5", "item6": "value6"},
	}

	// Test structs
	testSimpleStruct := SimpleStruct{
		ID:   12345,
		Name: "Test Simple Struct for Performance Benchmarking",
		Flag: true,
	}

	testComplexStruct := ComplexStruct{
		ID:      98765,
		Name:    "Test Complex Struct with Multiple Field Types",
		Data:    testBinary[:100],
		Numbers: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Tags: map[string]string{
			"category": "performance",
			"type":     "benchmark",
			"version":  "1.0",
		},
		Metadata: SimpleStruct{
			ID:   999,
			Name: "nested metadata",
			Flag: false,
		},
	}

	testLargeStruct := LargeStruct{
		Field1:  "Large struct with many fields for comprehensive testing",
		Field2:  123456789,
		Field3:  3.141592653589793,
		Field4:  true,
		Field5:  []string{"alpha", "beta", "gamma", "delta", "epsilon"},
		Field6:  []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		Field7:  map[string]int{"one": 1, "two": 2, "three": 3, "four": 4},
		Field8:  testSimpleStruct,
		Field9:  []SimpleStruct{testSimpleStruct, {ID: 111, Name: "second", Flag: false}},
		Field10: map[string]SimpleStruct{"first": testSimpleStruct, "second": {ID: 222, Name: "third", Flag: true}},
	}

	b.Run("String_Encoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			buf := lib.TakeBuffer()
			defer lib.ReleaseBuffer(buf)

			for i := 0; i < b.N; i++ {
				buf.Reset()
				if err := Encode(testString, buf, Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			var buf bytes.Buffer

			for i := 0; i < b.N; i++ {
				buf.Reset()
				enc := gob.NewEncoder(&buf)
				if err := enc.Encode(testString); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Binary_Encoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			buf := lib.TakeBuffer()
			defer lib.ReleaseBuffer(buf)

			for i := 0; i < b.N; i++ {
				buf.Reset()
				if err := Encode(testBinary, buf, Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			var buf bytes.Buffer

			for i := 0; i < b.N; i++ {
				buf.Reset()
				enc := gob.NewEncoder(&buf)
				if err := enc.Encode(testBinary); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Slice_Encoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			buf := lib.TakeBuffer()
			defer lib.ReleaseBuffer(buf)

			for i := 0; i < b.N; i++ {
				buf.Reset()
				if err := Encode(testSlice, buf, Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			var buf bytes.Buffer

			for i := 0; i < b.N; i++ {
				buf.Reset()
				enc := gob.NewEncoder(&buf)
				if err := enc.Encode(testSlice); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Map_Encoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			buf := lib.TakeBuffer()
			defer lib.ReleaseBuffer(buf)

			for i := 0; i < b.N; i++ {
				buf.Reset()
				if err := Encode(testMap, buf, Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			var buf bytes.Buffer

			for i := 0; i < b.N; i++ {
				buf.Reset()
				enc := gob.NewEncoder(&buf)
				if err := enc.Encode(testMap); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Nested_Map_Encoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			buf := lib.TakeBuffer()
			defer lib.ReleaseBuffer(buf)

			for i := 0; i < b.N; i++ {
				buf.Reset()
				if err := Encode(testNestedMap, buf, Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			var buf bytes.Buffer

			for i := 0; i < b.N; i++ {
				buf.Reset()
				enc := gob.NewEncoder(&buf)
				if err := enc.Encode(testNestedMap); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Simple_Struct_Encoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			buf := lib.TakeBuffer()
			defer lib.ReleaseBuffer(buf)

			for i := 0; i < b.N; i++ {
				buf.Reset()
				if err := Encode(testSimpleStruct, buf, Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			var buf bytes.Buffer

			for i := 0; i < b.N; i++ {
				buf.Reset()
				enc := gob.NewEncoder(&buf)
				if err := enc.Encode(testSimpleStruct); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Complex_Struct_Encoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			buf := lib.TakeBuffer()
			defer lib.ReleaseBuffer(buf)

			for i := 0; i < b.N; i++ {
				buf.Reset()
				if err := Encode(testComplexStruct, buf, Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			var buf bytes.Buffer

			for i := 0; i < b.N; i++ {
				buf.Reset()
				enc := gob.NewEncoder(&buf)
				if err := enc.Encode(testComplexStruct); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Large_Struct_Encoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			buf := lib.TakeBuffer()
			defer lib.ReleaseBuffer(buf)

			for i := 0; i < b.N; i++ {
				buf.Reset()
				if err := Encode(testLargeStruct, buf, Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			var buf bytes.Buffer

			for i := 0; i < b.N; i++ {
				buf.Reset()
				enc := gob.NewEncoder(&buf)
				if err := enc.Encode(testLargeStruct); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	// Decoding benchmarks
	// Pre-encode data for decoding tests
	edfBuf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(edfBuf)

	edfBuf.Reset()
	Encode(testString, edfBuf, Options{})
	edfStringData := make([]byte, len(edfBuf.B))
	copy(edfStringData, edfBuf.B)

	edfBuf.Reset()
	Encode(testSlice, edfBuf, Options{})
	edfSliceData := make([]byte, len(edfBuf.B))
	copy(edfSliceData, edfBuf.B)

	edfBuf.Reset()
	Encode(testNestedMap, edfBuf, Options{})
	edfNestedData := make([]byte, len(edfBuf.B))
	copy(edfNestedData, edfBuf.B)

	edfBuf.Reset()
	Encode(testSimpleStruct, edfBuf, Options{})
	edfSimpleStructData := make([]byte, len(edfBuf.B))
	copy(edfSimpleStructData, edfBuf.B)

	edfBuf.Reset()
	Encode(testComplexStruct, edfBuf, Options{})
	edfComplexStructData := make([]byte, len(edfBuf.B))
	copy(edfComplexStructData, edfBuf.B)

	edfBuf.Reset()
	Encode(testLargeStruct, edfBuf, Options{})
	edfLargeStructData := make([]byte, len(edfBuf.B))
	copy(edfLargeStructData, edfBuf.B)

	var gobBuf bytes.Buffer
	gobBuf.Reset()
	gob.NewEncoder(&gobBuf).Encode(testString)
	gobStringData := make([]byte, gobBuf.Len())
	copy(gobStringData, gobBuf.Bytes())

	gobBuf.Reset()
	gob.NewEncoder(&gobBuf).Encode(testSlice)
	gobSliceData := make([]byte, gobBuf.Len())
	copy(gobSliceData, gobBuf.Bytes())

	gobBuf.Reset()
	gob.NewEncoder(&gobBuf).Encode(testNestedMap)
	gobNestedData := make([]byte, gobBuf.Len())
	copy(gobNestedData, gobBuf.Bytes())

	gobBuf.Reset()
	gob.NewEncoder(&gobBuf).Encode(testSimpleStruct)
	gobSimpleStructData := make([]byte, gobBuf.Len())
	copy(gobSimpleStructData, gobBuf.Bytes())

	gobBuf.Reset()
	gob.NewEncoder(&gobBuf).Encode(testComplexStruct)
	gobComplexStructData := make([]byte, gobBuf.Len())
	copy(gobComplexStructData, gobBuf.Bytes())

	gobBuf.Reset()
	gob.NewEncoder(&gobBuf).Encode(testLargeStruct)
	gobLargeStructData := make([]byte, gobBuf.Len())
	copy(gobLargeStructData, gobBuf.Bytes())

	b.Run("String_Decoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := Decode(edfStringData, Options{})
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				buf := bytes.NewReader(gobStringData)
				dec := gob.NewDecoder(buf)
				var result string
				if err := dec.Decode(&result); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Slice_Decoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := Decode(edfSliceData, Options{})
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				buf := bytes.NewReader(gobSliceData)
				dec := gob.NewDecoder(buf)
				var result []int64
				if err := dec.Decode(&result); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Nested_Map_Decoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := Decode(edfNestedData, Options{})
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				buf := bytes.NewReader(gobNestedData)
				dec := gob.NewDecoder(buf)
				var result map[string]map[string]string
				if err := dec.Decode(&result); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Simple_Struct_Decoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := Decode(edfSimpleStructData, Options{})
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				buf := bytes.NewReader(gobSimpleStructData)
				dec := gob.NewDecoder(buf)
				var result SimpleStruct
				if err := dec.Decode(&result); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Complex_Struct_Decoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := Decode(edfComplexStructData, Options{})
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				buf := bytes.NewReader(gobComplexStructData)
				dec := gob.NewDecoder(buf)
				var result ComplexStruct
				if err := dec.Decode(&result); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Large_Struct_Decoding", func(b *testing.B) {
		b.Run("EDF", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := Decode(edfLargeStructData, Options{})
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Gob", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				buf := bytes.NewReader(gobLargeStructData)
				dec := gob.NewDecoder(buf)
				var result LargeStruct
				if err := dec.Decode(&result); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}
