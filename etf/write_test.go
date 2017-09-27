package etf

import (
	"bytes"
	"math"
	"math/big"
	"reflect"
	"testing"
)

func TestWriteAtom(t *testing.T) {
	c := new(Context)
	test := func(in Atom, shouldFail bool) {
		w := new(bytes.Buffer)
		if err := c.writeAtom(w, in); err != nil {
			if !shouldFail {
				t.Error(in, err)
			}
		} else if shouldFail {
			t.Error("err == nil (%v)", in)
		} else if v, err := c.Read(w); err != nil {
			t.Error(in, err)
		} else if l := w.Len(); l != 0 {
			t.Errorf("%v: buffer len %d", in, l)
		} else if v != in {
			t.Errorf("expected %v, got %v", in, v)
		}
	}

	test(Atom(""), false)
	test(Atom(bytes.Repeat([]byte{'a'}, math.MaxUint8)), false)
	test(Atom(bytes.Repeat([]byte{'a'}, math.MaxUint8+1)), false)
	test(Atom(bytes.Repeat([]byte{'a'}, math.MaxUint16)), false)
	test(Atom(bytes.Repeat([]byte{'a'}, math.MaxUint16+1)), true)
}

func TestWriteBinary(t *testing.T) {
	c := new(Context)
	test := func(in []byte) {
		w := new(bytes.Buffer)
		if err := c.writeBinary(w, in); err != nil {
			t.Error(in, err)
		} else if v, err := c.Read(w); err != nil {
			t.Error(in, err)
		} else if l := w.Len(); l != 0 {
			t.Errorf("%v: buffer len %d", in, l)
		} else if bytes.Compare(v.([]byte), in) != 0 {
			t.Errorf("expected %v, got %v", in, v)
		}
	}

	test([]byte{})
	test(bytes.Repeat([]byte{231}, 65535))
	test(bytes.Repeat([]byte{123}, 65536))
}

// func TestWriteBool(t *testing.T) {
// 	c := new(Context)
// 	test := func(in bool) {
// 		w := new(bytes.Buffer)
// 		if err := c.writeBool(w, in); err != nil {
// 			t.Error(in, err)
// 		} else if v, err := c.Read(w); err != nil {
// 			t.Error(in, err)
// 		} else if l := w.Len(); l != 0 {
// 			t.Errorf("%v: buffer len %d", in, l)
// 		} else if v != in {
// 			t.Errorf("expected %v, got %v", in, v)
// 		}
// 	}

// 	test(true)
// 	test(false)
// }

func TestWriteFloat(t *testing.T) {
	c := new(Context)
	test := func(in float64) {
		w := new(bytes.Buffer)
		if err := c.writeFloat(w, in); err != nil {
			t.Error(in, err)
		} else if v, err := c.Read(w); err != nil {
			t.Error(in, err)
		} else if l := w.Len(); l != 0 {
			t.Errorf("%v: buffer len %d", in, l)
		} else if v != in {
			t.Errorf("expected %v, got %v", in, v)
		}
	}

	test(0.0)
	test(-12345.6789)
	test(math.SmallestNonzeroFloat64)
	test(math.MaxFloat64)
}

func TestWriteInt(t *testing.T) {
	c := new(Context)
	test := func(in int64) {
		w := new(bytes.Buffer)
		if err := c.writeInt(w, in); err != nil {
			t.Error(in, err)
		} else if v, err := c.Read(w); err != nil {
			t.Error(in, err)
		} else if l := w.Len(); l != 0 {
			t.Errorf("%v: buffer len %d", in, l)
		} else if reflect.ValueOf(v).Int() != in {
			t.Errorf("expected %v, got %v", in, v)
		}
	}

	test(0)
	test(-1)
	test(math.MaxInt8)
	test(math.MaxInt8 + 1)
	test(math.MaxInt32)
	test(math.MaxInt32 + 1)
	test(math.MinInt32)
	test(math.MinInt32 - 1)
	test(math.MinInt64)
	test(math.MaxInt64)
}

func TestWriteUint(t *testing.T) {
	c := new(Context)
	test := func(in uint64) {
		w := new(bytes.Buffer)
		if err := c.writeUint(w, in); err != nil {
			t.Error(in, err)
		} else if v, err := c.Read(w); err != nil {
			t.Error(in, err)
		} else if l := w.Len(); l != 0 {
			t.Errorf("%v: buffer len %d", in, l)
		} else {
			var uv uint64
			switch v := v.(type) {
			case int:
				uv = uint64(v)
			case int64:
				uv = uint64(v)
			case *big.Int:
				uv = v.Uint64()
			}
			if uv != in {
				t.Errorf("expected %v, got %v", in, v)
			}
		}
	}

	test(0)
	test(math.MaxUint8)
	test(math.MaxUint8 + 1)
	test(math.MaxUint32)
	test(math.MaxUint32 + 1)
	test(math.MaxUint64)
}

func TestWritePid(t *testing.T) {
	c := new(Context)
	test := func(in Pid) {
		w := new(bytes.Buffer)
		if err := c.writePid(w, in); err != nil {
			t.Error(in, err)
		} else if v, err := c.Read(w); err != nil {
			t.Error(in, err)
		} else if l := w.Len(); l != 0 {
			t.Errorf("%v: buffer len %d", in, l)
		} else if v != in {
			t.Errorf("expected %v, got %v", in, v)
		}
	}

	test(Pid{Atom("omg@lol"), 38, 0, 3})
	test(Pid{Atom("self@localhost"), 32, 1, 9})
}

func TestWriteString(t *testing.T) {
	c := new(Context)
	test := func(in string, shouldFail bool) {
		w := new(bytes.Buffer)
		if err := c.writeString(w, in); err != nil {
			if !shouldFail {
				t.Error(in, err)
			}
		} else if shouldFail {
			t.Error("err == nil (%v)", in)
		} else if v, err := c.Read(w); err != nil {
			t.Error(in, err)
		} else if l := w.Len(); l != 0 {
			t.Errorf("%v: buffer len %d", in, l)
		} else if v != in {
			t.Errorf("expected %v, got %v", in, v)
		}
	}

	test(string(bytes.Repeat([]byte{'a'}, math.MaxUint16)), false)
	test("", false)
	test(string(bytes.Repeat([]byte{'a'}, math.MaxUint16+1)), true)
}

func TestWriteTerm(t *testing.T) {
	c := new(Context)
	type s1 struct {
		L []interface{}
		F float64
	}
	type s2 struct {
		Atom
		S  string
		I  int
		S1 s1
		B  byte
	}
	in := s2{
		Atom("lol"),
		"omg",
		13666,
		s1{
			[]interface{}{
				256,
				"1",
				13.0,
			},
			13.13,
		},
		1,
	}

	w := new(bytes.Buffer)
	if err := c.Write(w, in); err != nil {
		t.Error(in, err)
	} else {
		if term, err := c.Read(w); err != nil {
			t.Error(in, err)
		} else if l := w.Len(); l != 0 {
			t.Errorf("%v: buffer len %d", in, l)
		} else if err := c.Write(w, term); err != nil {
			t.Error(term, err)
		} else if term, err := c.Read(w); err != nil {
			t.Error(in, err)
		} else if l := w.Len(); l != 0 {
			t.Errorf("%v: buffer len %d", term, l)
		}
	}
}
